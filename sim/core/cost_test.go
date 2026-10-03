package core

import (
	"math"
	"math/rand"
	"testing"

	"github.com/wowsims/wotlk/sim/core/proto"
	"github.com/wowsims/wotlk/sim/core/serverdata"
	"github.com/wowsims/wotlk/sim/core/stats"
)

func costTestSpell(school SpellSchool, base, scale float64) *Spell {
	unit := &Unit{}
	unit.PseudoStats = stats.NewPseudoStats()
	spell := &Spell{Unit: unit, SpellSchool: school, CostMultiplier: 1}
	spell.DefaultCast.Cost = base
	spell.setStaticCost(0, 0, scale)
	return spell
}

func TestCostPercentModsAddUp(t *testing.T) {
	spell := costTestSpell(SpellSchoolFire, 1000, 1)
	spell.CostMultiplier -= 0.3
	spell.Unit.PseudoStats.CostMultiplier += 0.2
	spell.setStaticCost(0.9, 0, 1) // -10% from a talent
	// 1 - 0.3 + 0.2 - 0.1 = 0.8, never 0.7 * 1.2 * 0.9
	if got := spell.ApplyCostModifiers(spell.DefaultCast.Cost); got != 800 {
		t.Errorf("cost = %v, want 800", got)
	}
}

func TestCostFlatModsComeAfterPercent(t *testing.T) {
	spell := costTestSpell(SpellSchoolPhysical, 60, 1)
	spell.CostMultiplier -= 0.5
	spell.CostFlat -= 9
	spell.Unit.PseudoStats.CostFlat -= 1
	// 60 * 0.5 - 10, not (60 - 10) * 0.5
	if got := spell.ApplyCostModifiers(60); got != 20 {
		t.Errorf("cost = %v, want 20", got)
	}
	spell.setStaticCost(0, -4, 1)
	if got := spell.ApplyCostModifiers(60); got != 16 {
		t.Errorf("with a static flat mod: cost = %v, want 16", got)
	}
}

func TestCostTruncates(t *testing.T) {
	spell := costTestSpell(SpellSchoolFrost, 359.9, 1)
	if got := spell.ApplyCostModifiers(359.9); got != 359 {
		t.Errorf("base = %v, want 359", got)
	}
	spell.CostMultiplier -= 0.03 // 359 * 0.97 = 348.23
	if got := spell.ApplyCostModifiers(359.9); got != 348 {
		t.Errorf("after the percent mod = %v, want 348", got)
	}
	spell.Unit.PseudoStats.SchoolCostMultiplier[stats.SchoolIndexFrost] = -0.15 // 348 * 0.85 = 295.8
	if got := spell.ApplyCostModifiers(359.9); got != 295 {
		t.Errorf("after the school multiplier = %v, want 295", got)
	}
}

func TestCostSchoolAuras(t *testing.T) {
	spell := costTestSpell(SpellSchoolFire, 100, 1)
	// the flat school aura goes onto the base, the multiplier is applied last
	spell.Unit.AddSchoolCostFlat(SpellSchoolMagic, -42)
	spell.CostMultiplier -= 0.5
	spell.Unit.AddSchoolCostMultiplier(SpellSchoolMagic, -0.2)
	// (100 - 42) * 0.5 = 29, * 0.8 = 23.2
	if got := spell.ApplyCostModifiers(100); got != 23 {
		t.Errorf("cost = %v, want 23", got)
	}

	physical := costTestSpell(SpellSchoolPhysical, 100, 1)
	physical.Unit.AddSchoolCostFlat(SpellSchoolMagic, -42)
	physical.Unit.AddSchoolCostMultiplier(SpellSchoolMagic, -0.2)
	if got := physical.ApplyCostModifiers(100); got != 100 {
		t.Errorf("physical cost = %v, want 100", got)
	}

	cheap := costTestSpell(SpellSchoolArcane, 30, 1)
	cheap.Unit.AddSchoolCostFlat(SpellSchoolMagic, -42)
	if got := cheap.ApplyCostModifiers(30); got != 0 {
		t.Errorf("a cost below the flat cut = %v, want 0", got)
	}
}

// the server reads the lowest school bit: physical, holy, fire, nature, frost, shadow, arcane
func TestCostUsesTheFirstSchool(t *testing.T) {
	spell := costTestSpell(SpellSchoolFire|SpellSchoolArcane, 1000, 1)
	spell.Unit.AddSchoolCostMultiplier(SpellSchoolArcane, -0.5)
	if got := spell.ApplyCostModifiers(1000); got != 1000 {
		t.Errorf("spellfire with an arcane aura = %v, want 1000", got)
	}
	spell.Unit.AddSchoolCostMultiplier(SpellSchoolFire, -0.1)
	if got := spell.ApplyCostModifiers(1000); got != 900 {
		t.Errorf("spellfire with a fire aura = %v, want 900", got)
	}
}

func TestCostFreeSpellStaysFree(t *testing.T) {
	spell := costTestSpell(SpellSchoolFire, 500, 1)
	spell.CostMultiplier -= 1
	spell.CostFlat += 20
	if got := spell.ApplyCostModifiers(500); got != 20 {
		// the flat mod still applies: only the percent zeroes the base
		t.Errorf("cost = %v, want 20", got)
	}
	spell.CostMultiplier -= 100
	spell.CostFlat = 0
	if got := spell.ApplyCostModifiers(500); got != 0 {
		t.Errorf("a cost cut below zero = %v, want 0", got)
	}
}

// rage is stored in tenths on the server, so it truncates to 0.1
func TestCostRageTruncatesToTenths(t *testing.T) {
	spell := costTestSpell(SpellSchoolPhysical, 15, 10)
	spell.CostMultiplier -= 0.15 // 150 * 0.85 = 127.5
	if got := spell.ApplyCostModifiers(15); math.Abs(got-12.7) > 1e-9 {
		t.Errorf("cost = %v, want 12.7", got)
	}
	spell.CostFlat -= 5 // -50 tenths
	if got := spell.ApplyCostModifiers(15); math.Abs(got-7.7) > 1e-9 {
		t.Errorf("with a flat mod = %v, want 7.7", got)
	}
}

func TestStaticCostIgnoresDynamicMods(t *testing.T) {
	spell := costTestSpell(SpellSchoolPhysical, 60, 1)
	spell.setStaticCost(1, -9, 1)
	spell.CostMultiplier -= 0.5
	if got := spell.StaticCost(); got != 51 {
		t.Errorf("static cost = %v, want 51", got)
	}
	if got := spell.StaticCostWithPercent(-0.5); got != 21 {
		t.Errorf("static cost under Berserk = %v, want 21", got)
	}
}

func TestCostSchoollessSpellUsesServerSchool(t *testing.T) {
	spell := costTestSpell(SpellSchoolNone, 100, 1)
	spell.Unit.AddSchoolCostMultiplier(SpellSchoolPhysical, -0.5)
	if got := spell.ApplyCostModifiers(100); got != 100 {
		t.Errorf("no server data = %v, want 100", got)
	}
	// RegisterSpell sets the server data before anything reads the cost
	withServer := costTestSpell(SpellSchoolNone, 100, 1)
	withServer.serverSpell = &serverdata.Spell{SchoolMask: 1}
	withServer.Unit.AddSchoolCostMultiplier(SpellSchoolPhysical, -0.5)
	if got := withServer.ApplyCostModifiers(100); got != 50 {
		t.Errorf("server school physical = %v, want 50", got)
	}
}

// ApplyCostModifiers without its cache
func uncachedCost(spell *Spell, base float64) float64 {
	ps := &spell.Unit.PseudoStats
	school := spell.costSchool()
	return spell.costFor(base, (spell.CostMultiplier-1)+(ps.CostMultiplier-1), spell.CostFlat+ps.CostFlat,
		ps.SchoolCostFlat[school], ps.SchoolCostMultiplier[school])
}

// each input changed after a read cached the cost, which the next read has to see
func TestCostCacheSeesEveryInput(t *testing.T) {
	cases := []struct {
		name   string
		setup  func(c *Character, spell *Spell)
		change func(c *Character, spell *Spell)
		want   float64
	}{
		{name: "base", change: func(_ *Character, s *Spell) { s.DefaultCast.Cost = 500 }, want: 500},
		{name: "spell percent", change: func(_ *Character, s *Spell) { s.CostMultiplier -= 0.2 }, want: 800},
		{name: "spell flat", change: func(_ *Character, s *Spell) { s.CostFlat -= 30 }, want: 970},
		{name: "unit percent", change: func(c *Character, _ *Spell) { c.PseudoStats.CostMultiplier -= 0.1 }, want: 900},
		{name: "unit flat", change: func(c *Character, _ *Spell) { c.PseudoStats.CostFlat -= 40 }, want: 960},
		{name: "school flat", change: func(c *Character, _ *Spell) { c.AddSchoolCostFlat(SpellSchoolFire, -100) }, want: 900},
		{name: "school multiplier", change: func(c *Character, _ *Spell) { c.AddSchoolCostMultiplier(SpellSchoolFire, -0.25) }, want: 750},
		{name: "spark of hope", change: func(c *Character, _ *Spell) {
			c.Equipment[proto.ItemSlot_ItemSlotTrinket1] = Item{ID: 45703}
			c.applySparkOfHope()
		}, want: 958},
		{name: "static mods", change: func(_ *Character, s *Spell) { s.setStaticCost(0.9, -10, 1) }, want: 890},
		// the unit has a frost aura from the start, see below
		{name: "school", change: func(_ *Character, s *Spell) { s.SpellSchool = SpellSchoolFrost }, want: 500},
		{name: "iteration reset",
			setup:  func(c *Character, _ *Spell) { c.initialPseudoStats = c.PseudoStats; c.PseudoStats.CostFlat -= 40 },
			change: func(c *Character, _ *Spell) { c.PseudoStats = c.initialPseudoStats },
			want:   1000},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			character := &Character{}
			character.PseudoStats = stats.NewPseudoStats()
			character.AddSchoolCostMultiplier(SpellSchoolFrost, -0.5)
			spell := &Spell{Unit: &character.Unit, SpellSchool: SpellSchoolFire, CostMultiplier: 1}
			spell.DefaultCast.Cost = 1000
			spell.setStaticCost(0, 0, 1)
			if tc.setup != nil {
				tc.setup(character, spell)
			}
			before := spell.ApplyCostModifiers(spell.DefaultCast.Cost)
			tc.change(character, spell)
			got := spell.ApplyCostModifiers(spell.DefaultCast.Cost)
			if got != tc.want || got == before {
				t.Errorf("cost %v before the change, %v after, want %v", before, got, tc.want)
			}
			if want := uncachedCost(spell, spell.DefaultCast.Cost); got != want {
				t.Errorf("cached %v, uncached %v", got, want)
			}
		})
	}
}

// a random walk over every input, signed zeros included, against the uncached cost at each step
func TestCostCacheMatchesUncached(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	spell := costTestSpell(SpellSchoolArcane, 400, 1)
	ps := &spell.Unit.PseudoStats
	zero, negZero := 0.0, math.Copysign(0, -1)
	values := []float64{zero, negZero, -0.5, -0.1, 0.25, 1, -1, 3, -42, 17.5}
	for i := 0; i < 20000; i++ {
		v := values[rng.Intn(len(values))]
		base := spell.DefaultCast.Cost
		switch rng.Intn(10) {
		case 0:
			base = math.Abs(v) * 400
		case 1:
			spell.CostMultiplier = 1 + v
		case 2:
			spell.CostFlat = v
		case 3:
			ps.CostMultiplier = 1 + v
		case 4:
			ps.CostFlat = v
		case 5:
			ps.SchoolCostFlat[stats.SchoolIndexArcane] = v
		case 6:
			ps.SchoolCostMultiplier[stats.SchoolIndexArcane] = v
		case 7:
			ps.SchoolCostMultiplier[stats.SchoolIndexFire] = v
		case 8:
			spell.SpellSchool = []SpellSchool{SpellSchoolArcane, SpellSchoolFire, SpellSchoolArcane | SpellSchoolFire}[rng.Intn(3)]
		case 9:
			spell.setStaticCost(1+v, v, []float64{1, 10}[rng.Intn(2)])
		}
		got, want := spell.ApplyCostModifiers(base), uncachedCost(spell, base)
		if math.Float64bits(got) != math.Float64bits(want) {
			t.Fatalf("step %d: cached %v, uncached %v", i, got, want)
		}
	}
}
