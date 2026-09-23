package optimizer

import (
	"context"
	"math"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
	"google.golang.org/protobuf/encoding/protojson"
	goproto "google.golang.org/protobuf/proto"
)

// An old request or replay fixture has no equipped gear, so results score against the seed as before.
func TestPrepareRequestEquipped(t *testing.T) {
	r, err := PrepareRequest(testRequest())
	if err != nil {
		t.Fatal(err)
	}
	if r.Equipped != r.Seed {
		t.Errorf("equipped = %+v, want the seed %+v", r.Equipped, r.Seed)
	}

	req := testRequest()
	req.Equipped = testEquipment(map[proto.ItemSlot]*proto.ItemSpec{
		proto.ItemSlot_ItemSlotHead:    {Id: testHeadID},
		proto.ItemSlot_ItemSlotFinger1: {Id: testPoolRingID},
	})
	if r, err = PrepareRequest(req); err != nil {
		t.Fatal(err)
	}
	if r.Equipped.Items[proto.ItemSlot_ItemSlotFinger1].ItemID != testPoolRingID || r.Equipped.RacialTraits != r.Seed.RacialTraits {
		t.Errorf("equipped = %+v, want the request's with the seed's racial traits", r.Equipped)
	}
	if r.Seed.Items[proto.ItemSlot_ItemSlotFinger1].ItemID != testRingID {
		t.Errorf("seed finger 1 = %d, want the target's %d", r.Seed.Items[proto.ItemSlot_ItemSlotFinger1].ItemID, testRingID)
	}

	req.Equipped.Items[proto.ItemSlot_ItemSlotNeck] = &proto.ItemSpec{Id: 9400999}
	if _, err := PrepareRequest(req); err == nil || !strings.Contains(err.Error(), "equipped gear") {
		t.Errorf("err = %v, want one about the equipped gear's unknown item", err)
	}
}

// kaLaterPhaseRequest is kaRequest with a trinket from a later phase equipped: the pool doesn't have
// it, so the seed goes in without it.
func kaLaterPhaseRequest() *proto.OptimizeGearRequest {
	req := kaRequest(proto.OptimizerEffort_OptimizerEffortQuick)
	target := req.Base.Raid.Parties[0].Players[0]
	req.Equipped = goproto.Clone(target.Equipment).(*proto.EquipmentSpec)
	req.Equipped.Items[proto.ItemSlot_ItemSlotTrinket2] = &proto.ItemSpec{Id: kaTrinketA}
	target.Equipment.Items[proto.ItemSlot_ItemSlotTrinket2] = &proto.ItemSpec{}
	for _, sp := range req.Pool.Slots {
		// both trinket slots share one slice
		sp.ItemIds = slices.DeleteFunc(slices.Clone(sp.ItemIds), func(id int32) bool { return id == kaTrinketA })
	}
	return req
}

// A later-phase trinket still leaves the pick, but the result's seed is the gear as equipped and every
// delta is against it, not against the trimmed seed the search started from.
func TestDeltaAgainstEquippedGear(t *testing.T) {
	r, err := PrepareRequest(kaLaterPhaseRequest())
	if err != nil {
		t.Fatal(err)
	}
	var progress []*proto.OptimizerProgress
	result := optimize(context.Background(), r, r, newKnownEvaluator(kaMetrics), func(p *proto.OptimizerProgress) {
		progress = append(progress, p)
	}, time.Now())
	if result.ErrorResult != "" {
		t.Fatal(result.ErrorResult)
	}

	seed, err := LoadoutFromProto(result.Seed.GetEquipment(), result.Seed.GetRacialTraits())
	if err != nil || seed != r.Equipped {
		t.Fatalf("result seed = %+v (%v), want the gear as equipped %+v", seed, err, r.Equipped)
	}
	if result.Seed.ScoreDelta != 0 || result.Seed.ScoreDeltaSe != 0 {
		t.Errorf("seed delta = %v ± %v, want 0 against itself", result.Seed.ScoreDelta, result.Seed.ScoreDeltaSe)
	}
	dps := func(l Loadout) float64 { return kaDPS(gearStats(l), l) }
	if got, want := result.Seed.GetMetrics().GetDps(), dps(r.Equipped); math.Abs(got-want) > 5 {
		t.Errorf("seed DPS = %.1f, want the equipped gear's %.1f", got, want)
	}

	best, err := LoadoutFromProto(result.Best.GetEquipment(), result.Best.GetRacialTraits())
	if err != nil {
		t.Fatal(err)
	}
	if !result.Improved || best.Items[proto.ItemSlot_ItemSlotTrinket1].ItemID == kaTrinketA || best.Items[proto.ItemSlot_ItemSlotTrinket2].ItemID == kaTrinketA {
		t.Fatalf("improved = %v, best = %+v; want a pick without the later-phase trinket", result.Improved, best)
	}
	// kaDPS gains 1 DPS per attack power, so J is DPS and its deltas are DPS deltas
	got, se := result.Best.ScoreDelta, result.Best.ScoreDeltaSe
	if want := dps(best) - dps(r.Equipped); math.Abs(got-want) > 4*se+1 {
		t.Errorf("best delta = %.1f ± %.1f, want %.1f against the gear as equipped", got, se, want)
	}
	if fromSeed := dps(best) - dps(r.Seed); math.Abs(got-fromSeed) < 50 {
		t.Errorf("best delta = %.1f is the %.1f from the trimmed seed", got, fromSeed)
	}
	for i, top := range result.Top {
		l, _ := LoadoutFromProto(top.GetEquipment(), top.GetRacialTraits())
		if want := dps(l) - dps(r.Equipped); math.Abs(top.ScoreDelta-want) > 4*top.ScoreDeltaSe+1 {
			t.Errorf("top %d delta = %.1f ± %.1f, want %.1f", i, top.ScoreDelta, top.ScoreDeltaSe, want)
		}
	}
	if last := progress[len(progress)-1]; math.Abs(last.BestScoreDelta-got) > 4*se+1 {
		t.Errorf("last progress reports %.1f, want the result's %.1f", last.BestScoreDelta, got)
	}

	if !hasWarning(result, "Trinket 2: the search started without item 9700041, since the P1 pool doesn't have it") {
		t.Errorf("warnings = %q, want one naming the left-out trinket", result.Warnings)
	}
	if !slices.Equal(result.ScoreStats, []proto.Stat{proto.Stat_StatAttackPower}) {
		t.Errorf("score stats = %v, want attack power", result.ScoreStats)
	}
}

// A run cancelled before anything was simmed still names the gear as equipped as its seed.
func TestCancelledKeepsEquippedGear(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	req := kaLaterPhaseRequest()
	result := Optimize(ctx, req, nil)
	if !result.Cancelled {
		t.Fatalf("result = %v, want a cancelled one", result)
	}
	if !goproto.Equal(result.Seed.GetEquipment(), mustLoadout(t, req.Equipped).Equipment()) {
		t.Errorf("seed = %v, want the gear as equipped", result.Seed.GetEquipment())
	}
	if result.Best.GetEquipment().GetItems()[proto.ItemSlot_ItemSlotTrinket2].GetId() != 0 {
		t.Errorf("best = %v, want the trimmed seed", result.Best.GetEquipment())
	}
}

func mustLoadout(t *testing.T, es *proto.EquipmentSpec) Loadout {
	t.Helper()
	l, err := LoadoutFromProto(es, 0)
	if err != nil {
		t.Fatal(err)
	}
	return l
}

// Each left-out item gets its own warning with why it's out, and an item the trimmer only moved to
// another slot gets none.
func TestWarnLeftOut(t *testing.T) {
	req := kaRequest(proto.OptimizerEffort_OptimizerEffortQuick)
	target := req.Base.Raid.Parties[0].Players[0]
	req.Equipped = goproto.Clone(target.Equipment).(*proto.EquipmentSpec)
	req.Settings.ExcludedItemIds = []int32{kaHeadPlain}
	items := target.Equipment.Items
	items[proto.ItemSlot_ItemSlotHead] = &proto.ItemSpec{}
	items[proto.ItemSlot_ItemSlotMainHand] = &proto.ItemSpec{}
	items[proto.ItemSlot_ItemSlotTrinket1], items[proto.ItemSlot_ItemSlotTrinket2] = items[proto.ItemSlot_ItemSlotTrinket2], &proto.ItemSpec{}
	r, err := PrepareRequest(req)
	if err != nil {
		t.Fatal(err)
	}
	run := &run{r: r}
	run.warnLeftOut()

	if len(run.warnings) != 3 {
		t.Fatalf("warnings = %q, want the head, the main hand and trinket 1", run.warnings)
	}
	for _, want := range []string{
		"Head: the search started without item 9700003, since it's excluded.",
		"Main Hand: the search started without item 9700051, since it breaks an equip rule there.",
		"Trinket 1: the search started without item 9700043",
	} {
		if !slices.ContainsFunc(run.warnings, func(w string) bool { return strings.HasPrefix(w, want) }) {
			t.Errorf("warnings = %q, want one starting %q", run.warnings, want)
		}
	}
	for _, w := range run.warnings {
		if weapon := strings.HasPrefix(w, "Main Hand"); weapon != strings.Contains(w, "a weapon short") {
			t.Errorf("%q: only the weapon slot should say what an empty weapon slot does", w)
		}
	}
}

// Slot names match slotNames in ui/core/proto_utils/names.ts, and only a hunter's gun counts as a
// weapon the seed can't do without.
func TestLeftOutWording(t *testing.T) {
	names := []string{"Head", "Neck", "Shoulders", "Back", "Chest", "Wrist", "Hands", "Waist", "Legs", "Feet",
		"Finger 1", "Finger 2", "Trinket 1", "Trinket 2", "Main Hand", "Off Hand", "Ranged"}
	for slot, want := range names {
		if got := slotLabel(proto.ItemSlot(slot)); got != want {
			t.Errorf("slotLabel(%s) = %q, want %q", proto.ItemSlot(slot), got, want)
		}
	}
	gun := core.Item{Type: proto.ItemType_ItemTypeRanged, RangedWeaponType: proto.RangedWeaponType_RangedWeaponTypeGun}
	if isWeapon(gun, false) || !isWeapon(gun, true) {
		t.Errorf("a gun counts as a weapon for a warrior: %v, for a hunter: %v; want false, true", isWeapon(gun, false), isWeapon(gun, true))
	}
}

// Replay fixtures from the pool builder: an equipped item the phase and faction allow stays in the
// seed whatever the sources say, and one they don't (a later phase, PvP) leaves it. One the sources
// alone rule out sits in one pool or locked slot per copy worn.
func TestReplayEquippedGear(t *testing.T) {
	var files []string
	for _, pattern := range []string{"testdata/replay/*.json", "testdata/replay/*.json.gz"} {
		matches, err := filepath.Glob(pattern)
		if err != nil {
			t.Fatal(err)
		}
		files = append(files, matches...)
	}
	for _, file := range files {
		t.Run(filepath.Base(file), func(t *testing.T) {
			data, err := readFixture(file)
			if err != nil {
				t.Fatal(err)
			}
			req := &proto.OptimizeGearRequest{}
			if err := protojson.Unmarshal(data, req); err != nil {
				t.Fatal(err)
			}
			// a tab export from before equipped existed replays against its seed
			if req.Equipped == nil && !strings.HasSuffix(file, ".gz") {
				t.Skip("no equipped gear")
			}
			if req.Equipped == nil {
				t.Fatal("the fixture has no equipped gear: rerun the fixture driver")
			}
			r, err := PrepareRequest(req)
			if err != nil {
				t.Fatal(err)
			}
			phase := r.Settings.GetContentPhase()
			faction := playerFaction(r.Target().Race)
			// offered: slot pools with the item, plus locked slots holding it, since those keep it too
			kept, copies, offered := map[int32]int{}, map[int32]int{}, map[int32]int{}
			for slot := range r.Seed.Items {
				kept[r.Seed.Items[slot].ItemID]++
				copies[r.Equipped.Items[slot].ItemID]++
			}
			for _, sp := range r.Pool.Slots {
				for _, id := range sp.ItemIds {
					offered[id]++
				}
			}
			lockedOwned := 0
			for _, slot := range r.Settings.LockedSlots {
				if id := r.Seed.Items[slot].ItemID; id != 0 {
					offered[id]++
					if row := r.Catalog[id]; phaseAllows(row, phase, faction, nil) && !phaseAllows(row, phase, faction, r.Settings.Sources) {
						lockedOwned++
					}
				}
			}
			ownedOnly, laterPhase := 0, 0
			for slot, c := range r.Equipped.Items {
				if c.ItemID == 0 {
					continue
				}
				row := r.Catalog[c.ItemID]
				allowed := phaseAllows(row, phase, faction, nil) && !slices.Contains(r.Settings.ExcludedItemIds, c.ItemID)
				if stays := kept[c.ItemID] > 0; stays != allowed {
					t.Errorf("%s: item %d stays in the seed: %v, want %v", proto.ItemSlot(slot), c.ItemID, stays, allowed)
				}
				kept[c.ItemID]--
				switch {
				case allowed && !phaseAllows(row, phase, faction, r.Settings.Sources):
					ownedOnly++
					// in both rings' or hands' pools, the one copy could come back as a pair
					if offered[c.ItemID] > copies[c.ItemID] {
						t.Errorf("%s: item %d is in %d slot pools or locked slots, but only %d is equipped", proto.ItemSlot(slot), c.ItemID, offered[c.ItemID], copies[c.ItemID])
					}
				case !allowed && row.GetProgressionTier() > 12+phase:
					laterPhase++
				}
			}
			if filepath.Base(file) == "ret_p2_owned.json.gz" && (ownedOnly == 0 || laterPhase == 0 || lockedOwned == 0) {
				t.Errorf("%d items only the owned rule keeps, %d of them locked, %d from a later phase; want some of each", ownedOnly, lockedOwned, laterPhase)
			}
		})
	}
}

// phaseAllows mirrors isAvailable in ui/core/optimizer/catalog.ts. nil sources counts every kind.
func phaseAllows(row *proto.CatalogItem, phase int32, faction proto.Faction, sources []proto.CatalogSourceKind) bool {
	if row == nil || row.Pvp {
		return false
	}
	if row.Faction != proto.Faction_Unknown && row.Faction != faction {
		return false
	}
	if len(row.Sources) == 0 {
		return row.ProgressionTier <= 12+phase
	}
	return slices.ContainsFunc(row.Sources, func(s *proto.CatalogSource) bool {
		return s.ProgressionTier <= 12+phase && (sources == nil || s.Kind == proto.CatalogSourceKind_CatalogSourceUnknown || slices.Contains(sources, s.Kind))
	})
}
