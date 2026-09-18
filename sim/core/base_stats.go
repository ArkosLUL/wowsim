package core

import (
	"github.com/wowsims/wotlk/sim/core/proto"
	"github.com/wowsims/wotlk/sim/core/stats"
)

// AttackPowerFormula is a class's attack power before auras, as Player::UpdateAttackPowerAndDamage
// computes it: PerLevel·level + PerStrength·Str + PerAgility·Agi + Base.
type AttackPowerFormula struct {
	PerLevel, PerStrength, PerAgility, Base float64
}

// StatScaling is what a class gets from its level and primary stats.
type StatScaling struct {
	// Crit chances, not percentages: gtChanceToMeleeCritBase, gtChanceToMeleeCrit and
	// gtChanceToSpellCritBase.
	MeleeCritBase, MeleeCritPerAgility, SpellCritBase float64

	MeleeAttackPower, RangedAttackPower AttackPowerFormula

	// Player::GetDodgeFromAgility: each point of agility gives CritToDodge times its crit as dodge,
	// and the dodge from base agility doesn't diminish.
	DodgeBase, CritToDodge float64

	// Diminishing returns constants, caps in percent. A class without a parry cap can't parry.
	DodgeCap, ParryCap, MissCap, DiminishingK float64
}

// BaseStats are a character's stats before gear, talents and auras. Race only moves the primary
// stats; the rest comes from the class.
func BaseStats(race proto.Race, class proto.Class) stats.Stats {
	scaling := ClassStatScaling[class]
	level := float64(CharacterLevel)

	base := ClassBaseStats[class].Add(RaceStatOffsets[race])
	base[stats.AttackPower] = scaling.MeleeAttackPower.PerLevel*level + scaling.MeleeAttackPower.Base
	base[stats.RangedAttackPower] = scaling.RangedAttackPower.PerLevel*level + scaling.RangedAttackPower.Base
	base[stats.MeleeCrit] = scaling.MeleeCritBase * 100 * CritRatingPerCritChance
	base[stats.SpellCrit] = scaling.SpellCritBase * 100 * CritRatingPerCritChance
	return base
}

// addClassStatDependencies adds what the class gets from strength and agility. Pets have no class
// and set up their own.
func (character *Character) addClassStatDependencies() {
	scaling := ClassStatScaling[character.Class]
	for _, dep := range []struct {
		src, dst stats.Stat
		amount   float64
	}{
		{stats.Strength, stats.AttackPower, scaling.MeleeAttackPower.PerStrength},
		{stats.Agility, stats.AttackPower, scaling.MeleeAttackPower.PerAgility},
		{stats.Strength, stats.RangedAttackPower, scaling.RangedAttackPower.PerStrength},
		{stats.Agility, stats.RangedAttackPower, scaling.RangedAttackPower.PerAgility},
		{stats.Agility, stats.MeleeCrit, scaling.MeleeCritPerAgility * 100 * CritRatingPerCritChance},
	} {
		if dep.amount != 0 {
			character.AddStatDependency(dep.src, dep.dst, dep.amount)
		}
	}
}

// RatingPerPercent is how much of a rating the class needs for 1%, or for one point of a skill:
// gtCombatRatings over the class's gtOCTClassCombatRatingScalar. Without a class it's the unscaled
// rating.
func RatingPerPercent(class proto.Class, cr CombatRating) float64 {
	scalars, ok := CombatRatingClassScalars[class]
	if !ok {
		return CombatRatingBase[cr]
	}
	return CombatRatingBase[cr] / scalars[cr]
}
