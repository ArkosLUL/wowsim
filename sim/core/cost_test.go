package core

import (
	"math"
	"testing"

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
	spell.serverSpell = &serverdata.Spell{SchoolMask: 1}
	if got := spell.ApplyCostModifiers(100); got != 50 {
		t.Errorf("server school physical = %v, want 50", got)
	}
}
