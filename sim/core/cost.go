package core

import (
	"math"

	"github.com/wowsims/wotlk/sim/core/stats"
)

// The server's cost arithmetic (SpellInfo::CalcPowerCost, Player::ApplySpellMod), in the resource's server
// units (rage in tenths), truncating where the server stores an int:
//
//	base   = trunc(base) + the first school's UNIT_FIELD_POWER_COST_MODIFIER
//	cost   = trunc((base + sum of SPELLMOD_COST flats) * (1 + sum of SPELLMOD_COST percents)), 0 if base + flats <= 0
//	cost   = trunc(cost * (1 + the first school's UNIT_FIELD_POWER_COST_MULTIPLIER)), at least 0
//
// Percent mods add up, they never multiply. A death knight's runic power cost takes all three steps, each
// rune count only the spell mods (runeCountFor). Where each kind lives:
//   - spell mod percent: Spell.CostMultiplier or PseudoStats.CostMultiplier (1 plus the sum, so `-= 0.1` is
//     -10%), or the options' Multiplier for a talent that never changes
//   - spell mod flat: Spell.CostFlat or PseudoStats.CostFlat, or the options' FlatMod
//   - school aura flat and percent: PseudoStats.SchoolCostFlat and SchoolCostMultiplier

// absorbs float error, so 450 computed as 449.99999999 still truncates to 450
const costEpsilon = 1e-6

// serverFirstSchool is GetFirstSchoolInMask: the lowest bit of the server's mask, which orders
// physical, holy, fire, nature, frost, shadow, arcane.
func serverFirstSchool(school SpellSchool) stats.SchoolIndex {
	switch {
	case school.Matches(SpellSchoolPhysical):
		return stats.SchoolIndexPhysical
	case school.Matches(SpellSchoolHoly):
		return stats.SchoolIndexHoly
	case school.Matches(SpellSchoolFire):
		return stats.SchoolIndexFire
	case school.Matches(SpellSchoolNature):
		return stats.SchoolIndexNature
	case school.Matches(SpellSchoolFrost):
		return stats.SchoolIndexFrost
	case school.Matches(SpellSchoolShadow):
		return stats.SchoolIndexShadow
	case school.Matches(SpellSchoolArcane):
		return stats.SchoolIndexArcane
	}
	return stats.SchoolIndexNone
}

// costCache holds a spell's last ApplyCostModifiers result and the inputs it came from. Class code
// changes those inputs by hand, so a read compares them all instead of waiting for an invalidation.
type costCache struct {
	base, spellMult, spellFlat, unitMult, unitFlat, schoolFlat, schoolMult float64
	cost                                                                   float64

	school   stats.SchoolIndex
	schoolOf SpellSchool // the SpellSchool school was resolved from
	valid    bool
}

// costSchool is the school the cost auras read: the spell's own, or the server's for a spell the sim
// declares none for (Rapid Fire, Water Elemental).
func (spell *Spell) costSchool() stats.SchoolIndex {
	if spell.SpellSchool == SpellSchoolNone {
		if sd := spell.ServerSpell(); sd != nil {
			return serverFirstSchool(SpellSchoolFromServerMask(sd.SchoolMask))
		}
	}
	return serverFirstSchool(spell.SpellSchool)
}

func truncCost(x float64) float64 {
	return math.Floor(x + costEpsilon)
}

func (spell *Spell) costUnits() float64 {
	if spell.costScale == 0 {
		return 1
	}
	return spell.costScale
}

// costFor runs the server's arithmetic on the base cost. pct and flat are the dynamic spell mods on top of the
// spell's static ones, schoolFlat and schoolMult the first school's auras.
// important: ApplyCostModifiers caches the result, so a new input goes into costCache's key, or into
// setStaticCost's reset if only that sets it.
func (spell *Spell) costFor(base, pct, flat, schoolFlat, schoolMult float64) float64 {
	scale := spell.costUnits()
	cost := spellModCost(truncCost(base*scale)+schoolFlat, spell.costStaticPct+pct, (spell.costStaticFlat+flat)*scale)
	if schoolMult != 0 {
		cost = truncCost(cost * (1 + schoolMult))
	}
	return max(0, cost) / scale
}

// spellModCost is Player::ApplySpellMod's SPELLMOD_COST on a value in server units: flats first, then
// percents, and 0 once the flats reach 0. A value already at 0 or below skips every mod.
func spellModCost(value, pct, flat float64) float64 {
	if value <= 0 {
		return 0
	}
	if value += flat; value <= 0 {
		return 0
	}
	return truncCost(value * (1 + pct))
}

// runeCountFor is one rune type's count after the spell's cost mods (Spell::CheckRuneCost): spell mods
// only, flats in the server's units (a runic power flat hits the runes too), no school auras.
func runeCountFor(count int8, pct, flat float64) int8 {
	return int8(max(0, spellModCost(float64(count), pct, flat)))
}

// ApplyCostModifiers returns the cost to pay for a spell whose base cost is the given (DefaultCast.Cost).
func (spell *Spell) ApplyCostModifiers(base float64) float64 {
	c := &spell.costCache
	ps := &spell.Unit.PseudoStats
	// -0 and +0 compare equal, fine since costFor gives both the same cost
	if !c.valid || c.schoolOf != spell.SpellSchool || base != c.base ||
		spell.CostMultiplier != c.spellMult || spell.CostFlat != c.spellFlat ||
		ps.CostMultiplier != c.unitMult || ps.CostFlat != c.unitFlat ||
		ps.SchoolCostFlat[c.school] != c.schoolFlat || ps.SchoolCostMultiplier[c.school] != c.schoolMult {
		spell.updateCost(base)
	}
	return c.cost
}

func (spell *Spell) updateCost(base float64) {
	c := &spell.costCache
	// RegisterSpell sets a schoolless spell's server data before any cost read, so only a SpellSchool
	// change needs the school resolved again
	if !c.valid || c.schoolOf != spell.SpellSchool {
		c.school, c.schoolOf = spell.costSchool(), spell.SpellSchool
	}
	ps := &spell.Unit.PseudoStats
	c.base, c.spellMult, c.spellFlat = base, spell.CostMultiplier, spell.CostFlat
	c.unitMult, c.unitFlat = ps.CostMultiplier, ps.CostFlat
	c.schoolFlat, c.schoolMult = ps.SchoolCostFlat[c.school], ps.SchoolCostMultiplier[c.school]
	c.cost = spell.costFor(base, (c.spellMult-1)+(c.unitMult-1), c.spellFlat+c.unitFlat, c.schoolFlat, c.schoolMult)
	c.valid = true
}

// StaticCost is the cost with only the talents that never change (the options' Multiplier and FlatMod): what
// rotations that plan ahead compare energy and rage against.
func (spell *Spell) StaticCost() float64 {
	return spell.costFor(spell.DefaultCast.Cost, 0, 0, 0, 0)
}

// StaticCostWithPercent is StaticCost with one more percent spell mod, such as Berserk's -0.5.
func (spell *Spell) StaticCostWithPercent(pct float64) float64 {
	return spell.costFor(spell.DefaultCast.Cost, pct, 0, 0, 0)
}

// AddSchoolCostMultiplier adds a UNIT_FIELD_POWER_COST_MULTIPLIER aura (-0.2 for -20%) to every school of the mask.
func (unit *Unit) AddSchoolCostMultiplier(schools SpellSchool, delta float64) {
	for _, school := range allSpellSchools {
		if schools.Matches(school) {
			unit.PseudoStats.SchoolCostMultiplier[schoolIndex(school)] += delta
		}
	}
}

// AddSchoolCostFlat adds a UNIT_FIELD_POWER_COST_MODIFIER aura, in the server's units.
func (unit *Unit) AddSchoolCostFlat(schools SpellSchool, delta float64) {
	for _, school := range allSpellSchools {
		if schools.Matches(school) {
			unit.PseudoStats.SchoolCostFlat[schoolIndex(school)] += delta
		}
	}
}

var allSpellSchools = [...]SpellSchool{SpellSchoolPhysical, SpellSchoolArcane, SpellSchoolFire, SpellSchoolFrost,
	SpellSchoolHoly, SpellSchoolNature, SpellSchoolShadow}

// applySparkOfHope is the trinket's aura 65010, which takes 42 off every non-physical cost. It's here, not an
// item effect, so it adds no AllItems golden row to every suite.
func (character *Character) applySparkOfHope() {
	if character.HasTrinketEquipped(45703) {
		character.AddSchoolCostFlat(SpellSchoolMagic, -42)
	}
}

// setStaticCost records the options' static talent mods; a multiplier of 0 means none.
func (spell *Spell) setStaticCost(multiplier, flatMod, scale float64) {
	if multiplier != 0 {
		spell.costStaticPct = multiplier - 1
	}
	spell.costStaticFlat = flatMod
	spell.costScale = scale
	spell.costCache = costCache{}
}
