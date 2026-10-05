package optimizer

import (
	"context"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/wowsims/wotlk/sim/core/proto"
	"github.com/wowsims/wotlk/sim/core/stats"
	"google.golang.org/protobuf/encoding/protojson"
)

// Holiday loot the outclassed cut has to keep, all in Ret's pools: Ahune's cloaks (tier 8), Coren
// Direbrew's trinkets and the Headless Horseman's rings (tier 0), Love is in the Air's necks (tier 0).
// AtlasLoot files every one of them under vanilla or TBC.
var holidayLoot = []int32{
	54801, 54802, 54803, 54804, 54805,
	49074, 49076, 49078, 49080, 49116, 49118, 49121, 49123, 49124,
	51804, 51805, 51806, 51807, 51808,
}

func loadReplay(tb testing.TB, name string) *proto.OptimizeGearRequest {
	tb.Helper()
	data, err := readFixture(filepath.Join("testdata/replay", name+".json.gz"))
	if err != nil {
		tb.Fatal(err)
	}
	req := &proto.OptimizeGearRequest{}
	if err := protojson.Unmarshal(data, req); err != nil {
		tb.Fatal(err)
	}
	return req
}

// poolSlots is each item's slot pools.
func poolSlots(req *proto.OptimizeGearRequest) map[int32][]proto.ItemSlot {
	out := map[int32][]proto.ItemSlot{}
	for _, sp := range req.GetPool().GetSlots() {
		for _, id := range sp.ItemIds {
			out[id] = append(out[id], sp.Slot)
		}
	}
	return out
}

func TestOutclassed(t *testing.T) {
	northrend := &proto.CatalogItem{ProgressionTier: 13}
	cases := []struct {
		name string
		item *proto.UIItem
		row  *proto.CatalogItem
		want bool
	}{
		{"plain Northrend under the cut", &proto.UIItem{Ilvl: 183}, northrend, true},
		{"plain Northrend at the cut", &proto.UIItem{Ilvl: outclassedItemLevel}, northrend, false},
		{"Northrend with an effect", &proto.UIItem{Ilvl: 174}, &proto.CatalogItem{ProgressionTier: 13, HasEffect: true}, false},
		{"Northrend set piece", &proto.UIItem{Ilvl: 115, SetName: "Blessed Battlegear of Undead Slaying"}, northrend, false},
		{"TBC with an effect", &proto.UIItem{Ilvl: 125}, &proto.CatalogItem{ProgressionTier: 8, HasEffect: true}, true},
		{"TBC set piece at tier 12", &proto.UIItem{Ilvl: 154, SetName: "Lightbringer Battlegear"}, &proto.CatalogItem{ProgressionTier: 12}, true},
		{"Ahune's cloak", &proto.UIItem{Ilvl: 232}, &proto.CatalogItem{ProgressionTier: 8}, false},
		{"Brewfest trinket", &proto.UIItem{Ilvl: 200}, &proto.CatalogItem{HasEffect: true}, false},
		{"no catalog row", &proto.UIItem{Ilvl: 150}, nil, true},
	}
	for _, c := range cases {
		if got := outclassed(c.item, c.row); got != c.want {
			t.Errorf("%s: outclassed = %v, want %v", c.name, got, c.want)
		}
	}
}

// TestOutclassedMirrorsUI fails when outclassed's constants drift from the pool builder's.
func TestOutclassedMirrorsUI(t *testing.T) {
	for _, c := range []struct {
		file, name string
		want       int
	}{
		{"../../ui/core/optimizer/pool_builder.ts", "OUTCLASSED_ITEM_LEVEL", outclassedItemLevel},
		{"../../ui/core/optimizer/catalog.ts", "PHASE_TIER_OFFSET", phaseTierOffset},
	} {
		data, err := os.ReadFile(c.file)
		if err != nil {
			t.Fatal(err)
		}
		m := regexp.MustCompile(`(?m)^export const ` + c.name + ` = (\d+);`).FindSubmatch(data)
		if m == nil {
			t.Errorf("%s has no %s", c.file, c.name)
			continue
		}
		if got, _ := strconv.Atoi(string(m[1])); got != c.want {
			t.Errorf("%s: %s = %d, the Go mirror has %d", c.file, c.name, got, c.want)
		}
	}
}

// Every replay fixture leaves outclassed gear out, except where it's worn.
func TestReplayLeavesOutclassedGearOut(t *testing.T) {
	uidb, _ := loadRealData(t)
	items := make(map[int32]*proto.UIItem, len(uidb.Items))
	for _, item := range uidb.Items {
		items[item.Id] = item
	}
	files, err := filepath.Glob("testdata/replay/*.json.gz")
	if err != nil {
		t.Fatal(err)
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
			rows := map[int32]*proto.CatalogItem{}
			for _, row := range req.Pool.CatalogItems {
				rows[row.Id] = row
			}
			worn := map[int32]int{}
			for _, spec := range req.Equipped.GetItems() {
				worn[spec.GetId()]++
			}
			pools := poolSlots(req)
			for id, slots := range pools {
				item := items[id]
				if item == nil {
					t.Errorf("item %d isn't in db.json", id)
					continue
				}
				// a worn one sits in its own slot's pool only, never in both rings' or trinkets'
				if outclassed(item, rows[id]) && len(slots) > worn[id] {
					t.Errorf("%s (%d) is outclassed but in the %v pools, worn %d times", item.Name, id, slots, worn[id])
				}
			}
			// the web server has no data for anything else
			target, err := raidPlayer(req.Base.Raid, int(req.TargetRaidIndex))
			if err != nil {
				t.Fatal(err)
			}
			seed := map[int32]bool{}
			for _, spec := range target.GetEquipment().GetItems() {
				seed[spec.GetId()] = true
			}
			for i, ws := range req.Settings.GetWarmStarts() {
				for slot, spec := range ws.GetItems() {
					if id := spec.GetId(); id != 0 && len(pools[id]) == 0 && !seed[id] && worn[id] == 0 {
						t.Errorf("warm start %d, %s: item %d is in no pool, the seed or the equipped gear", i, proto.ItemSlot(slot), id)
					}
				}
			}
		})
	}
}

func TestReplayOutclassedCut(t *testing.T) {
	for _, name := range []string{"ret_p1", "ret_p5"} {
		pools := poolSlots(loadReplay(t, name))
		for _, id := range holidayLoot {
			if len(pools[id]) == 0 {
				t.Errorf("%s: holiday item %d is in no pool", name, id)
			}
		}
	}

	p1 := poolSlots(loadReplay(t, "ret_p1"))
	// Brann's Lost Mining Helmet (plain, 183) and Dragonspine Trophy (TBC)
	for _, id := range []int32{35670, 28830} {
		if len(p1[id]) > 0 {
			t.Errorf("ret_p1: outclassed item %d is in the %v pools", id, p1[id])
		}
	}
	// Futuresight Rune (an effect) and a Blessed Battlegear of Undead Slaying piece (a set), both
	// Northrend and under the cut
	for _, id := range []int32{38763, 43069} {
		if len(p1[id]) == 0 {
			t.Errorf("ret_p1: item %d is in no pool", id)
		}
	}

	wornReq := loadReplay(t, "ret_p1_worn_outclassed")
	if ws := wornReq.Settings.GetWarmStarts(); len(ws) != 1 {
		t.Errorf("ret_p1_worn_outclassed has %d warm starts, want 1", len(ws))
	} else if head, mh := ws[0].Items[proto.ItemSlot_ItemSlotHead].GetId(), ws[0].Items[proto.ItemSlot_ItemSlotMainHand].GetId(); head != 0 || mh == 0 {
		t.Errorf("ret_p1_worn_outclassed's warm start has head %d and main hand %d, want Brann's Lost Mining Helmet out and the rest kept", head, mh)
	}
	worn := poolSlots(wornReq)
	for _, c := range []struct {
		id   int32
		want []proto.ItemSlot
	}{
		{38614, []proto.ItemSlot{proto.ItemSlot_ItemSlotBack}},
		// moved up from trinket 2, with its place in the pool
		{28830, []proto.ItemSlot{proto.ItemSlot_ItemSlotTrinket1}},
		// locked in finger 2
		{34213, nil},
	} {
		if !slices.Equal(worn[c.id], c.want) {
			t.Errorf("ret_p1_worn_outclassed: worn item %d is in the %v pools, want %v", c.id, worn[c.id], c.want)
		}
	}
}

// Coren Direbrew's trinkets with an effect in the sim: Coren's Chromium Coaster, Mithril Pocketwatch,
// Ancient Pickled Egg, Brawler's Souvenir. Bitter Balebrew and Bubbling Brightbrew Charm have none,
// so they price on their stats alone.
var corenDirebrewTrinkets = []int32{49074, 49076, 49078, 49080}

// Coren's Chromium Coaster and Mithril Pocketwatch: crit rating, which ranks them with Ret's better
// Naxx trinkets, well inside Quick's screening cut
var corenDirebrewCritTrinkets = []int32{49074, 49076}

// Coren Direbrew's trinkets read tier 0, which the effects budget's prior used to rank last, so
// Quick never priced them in Ret's P1 pool. They rank by their stats like Naxx loot now, so Quick's
// cut takes them only where it takes Naxx trinkets with the same stats: the Ancient Pickled Egg's
// 98 spell power can land right at the cut, tied with The Egg of Mortal Essence.
func TestRetP1QuickPricesBrewfestTrinkets(t *testing.T) {
	r, err := PrepareRequest(loadReplay(t, "ret_p1"))
	if err != nil {
		t.Fatal(err)
	}
	if r.Settings.GetEffort() != proto.OptimizerEffort_OptimizerEffortQuick {
		t.Fatalf("ret_p1 runs at %s, want Quick", r.Settings.GetEffort())
	}

	t.Run("ranked by stats", func(t *testing.T) {
		pool, err := CompilePool(r)
		if err != nil {
			t.Fatal(err)
		}
		// set slopes, so the order doesn't move with the sim's own
		s := newSurrogate(pool, r.Seed, Response{
			stats.Strength:         {Stat: stats.Strength, SlopeBelow: 2.5, SlopeAbove: 2.5},
			stats.AttackPower:      {Stat: stats.AttackPower, SlopeBelow: 1, SlopeAbove: 1},
			stats.MeleeHit:         {Stat: stats.MeleeHit, SlopeBelow: 3.5, SlopeAbove: 0},
			stats.MeleeCrit:        {Stat: stats.MeleeCrit, SlopeBelow: 2, SlopeAbove: 2},
			stats.MeleeHaste:       {Stat: stats.MeleeHaste, SlopeBelow: 1.5, SlopeAbove: 1.5},
			stats.Expertise:        {Stat: stats.Expertise, SlopeBelow: 3, SlopeAbove: 3},
			stats.ArmorPenetration: {Stat: stats.ArmorPenetration, SlopeBelow: 2, SlopeAbove: 2},
			stats.SpellPower:       {Stat: stats.SpellPower, SlopeBelow: 0.3, SlopeAbove: 0.3},
		})
		rn := &run{asked: r, r: r, pool: pool}
		v := s.slopes(s.seedStats, 0)
		fx := &effects{raw: map[effectKey]Estimate{}, missing: map[effectKey]bool{}, pairBase: map[proto.ItemSlot]Loadout{}}
		families := rn.effectFamilies(s, v, s.newGemPlan(v), fx)
		i := slices.IndexFunc(families, func(f *effectFamily) bool { return f.name == proto.ItemSlot_ItemSlotTrinket1.String()+" pair" })
		if i < 0 {
			t.Fatal("no trinket family")
		}
		prior := map[int32]float64{}
		for j, k := range families[i].keys {
			prior[k.id] = families[i].prior[j]
		}

		lo, hi := s.curveBox()
		gb := s.newGemBounds(lo, hi)
		statBound, naxxOrOlder := map[int32]float64{}, map[int32]bool{}
		for _, slot := range []proto.ItemSlot{proto.ItemSlot_ItemSlotTrinket1, proto.ItemSlot_ItemSlotTrinket2} {
			for _, cand := range pool.Slots[slot] {
				_, statBound[cand.Item.ID] = s.candStatBounds(slot, cand, lo, hi, gb)
				naxxOrOlder[cand.Item.ID] = cand.Catalog.GetProgressionTier() <= 13
			}
		}

		for _, id := range corenDirebrewTrinkets {
			p, ok := prior[id]
			if !ok {
				t.Errorf("trinket %d isn't in the effects screening", id)
				continue
			}
			for other, q := range prior {
				if slices.Contains(corenDirebrewTrinkets, other) || !naxxOrOlder[other] || math.IsInf(q, 1) {
					continue
				}
				if statBound[other] <= statBound[id] && q > p {
					t.Errorf("trinket %d (stats up to %.1f) ranks below %d (stats up to %.1f)", id, statBound[id], other, statBound[other])
					break
				}
			}
		}
	})

	metrics, err := WeightedMetrics(r.Settings)
	if err != nil {
		t.Fatal(err)
	}
	rec := &effectsRecorder{inner: NewSimEvaluator(r, metrics...), slot: proto.ItemSlot_ItemSlotTrinket1, seen: map[int32]bool{}}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	searched := false
	optimize(ctx, r, r, rec, func(p *proto.OptimizerProgress) {
		rec.mu.Lock()
		defer rec.mu.Unlock()
		rec.stage = p.Stage
		// the pricing's all done by the time the search starts
		if p.Stage == "Search" {
			searched = true
			cancel()
		}
	}, time.Now())
	rec.mu.Lock()
	defer rec.mu.Unlock()
	if !searched {
		t.Fatalf("the run stopped at %s, before the search", rec.stage)
	}
	for _, id := range corenDirebrewCritTrinkets {
		if !rec.seen[id] {
			t.Errorf("the Effects stage never simmed trinket %d", id)
		}
	}
}
