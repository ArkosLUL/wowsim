package optimizer

import (
	"context"
	"math"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
	"github.com/wowsims/wotlk/sim/core/stats"
	goproto "google.golang.org/protobuf/proto"
)

// mmHunter wears the P1 Marksmanship preset, with sim/hunter's MM talents and glyphs.
func mmHunter() *proto.Player {
	return &proto.Player{
		Name:          "Marksmanship",
		Race:          proto.Race_RaceOrc,
		Class:         proto.Class_ClassHunter,
		Equipment:     core.GetGearSet("../../ui/hunter/gear_sets", "p1_mm").GearSet,
		Rotation:      core.GetAplRotation("../../ui/hunter/apls", "mm").Rotation,
		TalentsString: "502-035335131030013233035031051-5000002",
		Glyphs: &proto.Glyphs{
			Major1: int32(proto.HunterMajorGlyph_GlyphOfSerpentSting),
			Major2: int32(proto.HunterMajorGlyph_GlyphOfSteadyShot),
			Major3: int32(proto.HunterMajorGlyph_GlyphOfChimeraShot),
		},
		Spec: &proto.Player_Hunter{Hunter: &proto.Hunter{Options: &proto.Hunter_Options{
			Ammo:           proto.Hunter_Options_SaroniteRazorheads,
			PetType:        proto.Hunter_Options_Wolf,
			PetUptime:      0.9,
			UseHuntersMark: true,
		}}},
		Consumes: &proto.Consumes{
			Flask:         proto.Flask_FlaskOfEndlessRage,
			DefaultPotion: proto.Potions_PotionOfSpeed,
			Food:          proto.Food_FoodFishFeast,
		},
		Buffs:              core.FullIndividualBuffs,
		Professions:        []proto.Profession{proto.Profession_Jewelcrafting, proto.Profession_Engineering},
		DistanceFromTarget: 30,
	}
}

// hunterRequest is a Quick P1 run for player over its realistic pool.
func hunterRequest(tb testing.TB, player *proto.Player) *proto.OptimizeGearRequest {
	req := presetOptimizeRequest(tb, "fury_p1")
	req.Base.Raid = core.SinglePlayerRaidProto(player, core.FullPartyBuffs, core.FullRaidBuffs, core.FullDebuffs)
	req.Settings = &proto.OptimizerSettings{
		ContentPhase: 1,
		Effort:       proto.OptimizerEffort_OptimizerEffortQuick,
		RacialMode:   proto.OptimizerRacialMode_OptimizerRacialKeepCurrent,
	}
	req.Pool = realisticPool(tb, player, 1, nil)
	return req
}

func isGun(id int32) bool {
	item, _ := core.LookupItem(id)
	return item.RangedWeaponType == proto.RangedWeaponType_RangedWeaponTypeGun
}

func isBow(id int32) bool {
	item, _ := core.LookupItem(id)
	return item.RangedWeaponType == proto.RangedWeaponType_RangedWeaponTypeBow
}

// A hunter's ranged weapons count from one it has, the seed's or the pool's best by stats, since a
// hunter without one has no Auto Shot. Other classes count them from the empty slot.
func TestRangedFamilyBase(t *testing.T) {
	ranged := proto.ItemSlot_ItemSlotRanged
	for _, tc := range []struct {
		name      string
		class     proto.Class
		emptySeed bool
	}{
		{"hunter", proto.Class_ClassHunter, false},
		{"hunter without a ranged weapon", proto.Class_ClassHunter, true},
		{"rogue", proto.Class_ClassRogue, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			player := mmHunter()
			if tc.emptySeed {
				player.Equipment = goproto.Clone(player.Equipment).(*proto.EquipmentSpec)
				player.Equipment.Items[ranged] = &proto.ItemSpec{}
			}
			r, err := PrepareRequest(hunterRequest(t, player))
			if err != nil {
				t.Fatal(err)
			}
			pool, err := CompilePool(r)
			if err != nil {
				t.Fatal(err)
			}
			// the families only read the class, so the hunter's pool and seed stand in for a rogue's
			r.Target().Class = tc.class
			rn := &run{asked: r, r: r, pool: pool}
			s := newSurrogate(pool, r.Seed, Response{
				stats.Agility:           {Stat: stats.Agility, SlopeBelow: 1, SlopeAbove: 1},
				stats.AttackPower:       {Stat: stats.AttackPower, SlopeBelow: 0.5, SlopeAbove: 0.5},
				stats.RangedAttackPower: {Stat: stats.RangedAttackPower, SlopeBelow: 0.5, SlopeAbove: 0.5},
			})
			v := s.slopes(s.seedStats)
			fx := &effects{raw: map[effectKey]Estimate{}, missing: map[effectKey]bool{}, pairBase: map[proto.ItemSlot]Loadout{}}
			families := rn.effectFamilies(s, v, s.newGemPlan(v), fx)

			i := slices.IndexFunc(families, func(f *effectFamily) bool { return f.name == ranged.String()+" items" })
			if i < 0 {
				t.Fatal("no ranged family")
			}
			f := families[i]
			baseID := f.base.Items[ranged].ItemID
			if tc.class != proto.Class_ClassHunter {
				if baseID != 0 {
					t.Errorf("the ranged family's base holds item %d, want the slot empty", baseID)
				}
				return
			}

			if tc.emptySeed {
				lo, hi := s.slopeBox()
				gb := s.newGemBounds(lo, hi)
				best := math.Inf(-1)
				var baseLB float64
				for _, cand := range pool.Slots[ranged] {
					lb, _ := s.candStatBounds(ranged, cand, lo, hi, gb)
					best = max(best, lb)
					if cand.Item.ID == baseID {
						baseLB = lb
					}
				}
				if baseID == 0 || baseLB < best {
					t.Errorf("the ranged family's base holds item %d at a stat lower bound of %.1f, want the pool's best, %.1f",
						baseID, baseLB, best)
				}
			} else if want := r.Seed.Items[ranged].ItemID; baseID != want {
				t.Errorf("the ranged family's base holds item %d, want the seed's %d", baseID, want)
			}
			if est, ok := fx.raw[effectKey{effectItem, ranged, baseID}]; !ok || est != (Estimate{}) {
				t.Errorf("the base's own weapon is priced at %+v (set: %v), want 0 without a sim", est, ok)
			}
			var bows, guns int
			for j, k := range f.keys {
				if k.id == baseID {
					t.Errorf("the base's own weapon is simmed against itself")
				}
				if isBow(k.id) {
					bows++
				}
				if isGun(k.id) {
					guns++
				}
				if f.variants[j].Items[ranged].ItemID != k.id {
					t.Errorf("variant %d holds item %d, want %d", j, f.variants[j].Items[ranged].ItemID, k.id)
				}
			}
			if bows == 0 || guns == 0 {
				t.Errorf("the ranged family sims %d bows and %d guns, want both", bows, guns)
			}

			if tc.emptySeed {
				return
			}
			for _, f := range families {
				for j, l := range append([]Loadout{f.base}, f.variants...) {
					if l.Items[ranged].ItemID == 0 {
						t.Errorf("%s: loadout %d has no ranged weapon", f.name, j)
					}
				}
			}
			for slot, l := range fx.pairBase {
				if l.Items[ranged].ItemID == 0 {
					t.Errorf("the %s pair's base has no ranged weapon", slot)
				}
			}
		})
	}
}

// rangedGuard sims through inner, but fails any point without a ranged weapon rather than sim it,
// counting them by stage. It notes the ranged weapons the Effects stage sims other than seed, since
// every other family's loadouts carry seed too.
type rangedGuard struct {
	inner Evaluator
	seed  int32

	mu     sync.Mutex
	stage  string
	empty  map[string]int
	simmed map[int32]bool
}

func (g *rangedGuard) Evaluate(ctx context.Context, points []Point, iterations int) ([]*Evaluation, error) {
	g.mu.Lock()
	for _, p := range points {
		id := p.Loadout.Items[proto.ItemSlot_ItemSlotRanged].ItemID
		if id == 0 {
			g.empty[g.stage]++
			g.mu.Unlock()
			return nil, &SimError{Point: p, Message: "no ranged weapon"}
		}
		if g.stage == "Effects" && id != g.seed {
			g.simmed[id] = true
		}
	}
	g.mu.Unlock()
	return g.inner.Evaluate(ctx, points, iterations)
}

// A Quick run for a Marksmanship hunter finishes, never sims one without a ranged weapon, and its
// Effects stage prices bows and guns.
func TestOptimizeMarksmanshipHunter(t *testing.T) {
	r, err := PrepareRequest(hunterRequest(t, mmHunter()))
	if err != nil {
		t.Fatal(err)
	}
	keep, err := WeightedMetrics(r.Settings)
	if err != nil {
		t.Fatal(err)
	}
	g := &rangedGuard{inner: NewSimEvaluator(r, keep...), seed: r.Seed.Items[proto.ItemSlot_ItemSlotRanged].ItemID,
		empty: map[string]int{}, simmed: map[int32]bool{}}
	start := time.Now()
	result := optimize(context.Background(), r, r, g, func(p *proto.OptimizerProgress) {
		g.mu.Lock()
		g.stage = p.Stage
		g.mu.Unlock()
	}, start)
	if result.ErrorResult != "" {
		t.Fatal(result.ErrorResult)
	}
	for _, w := range result.Warnings {
		t.Log(w)
	}
	for stage, n := range g.empty {
		t.Errorf("%s asked for %d sims without a ranged weapon", stage, n)
	}
	var bows, guns []int32
	for id := range g.simmed {
		if isBow(id) {
			bows = append(bows, id)
		}
		if isGun(id) {
			guns = append(guns, id)
		}
	}
	if len(bows) == 0 || len(guns) == 0 {
		t.Errorf("Effects simmed bows %v and guns %v, want some of each", bows, guns)
	}
	best, err := LoadoutFromProto(result.Best.GetEquipment(), result.Best.GetRacialTraits())
	if err != nil {
		t.Fatal(err)
	}
	if best.Items[proto.ItemSlot_ItemSlotRanged].ItemID == 0 {
		t.Error("the best loadout has no ranged weapon")
	}
	t.Logf("%.0f s, %d sims, best %+.1f ± %.1f over the seed, ranged weapon %d, Effects simmed bows %v and guns %v",
		time.Since(start).Seconds(), result.TotalSims, result.Best.GetScoreDelta(), result.Best.GetScoreDeltaSe(),
		best.Items[proto.ItemSlot_ItemSlotRanged].ItemID, bows, guns)
}
