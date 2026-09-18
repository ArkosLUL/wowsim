package core

import (
	"fmt"
	"strings"

	"github.com/wowsims/wotlk/sim/core/stats"
)

func (result *SpellResult) applyResistances(sim *Simulation, spell *Spell, isPeriodic bool, attackTable *AttackTable) {
	// TODO check why result.Outcome isn't updated with resists anymore
	resistanceMultiplier := spell.ResistanceMultiplier(sim, isPeriodic, attackTable)
	result.Damage *= resistanceMultiplier

	result.ResistanceMultiplier = resistanceMultiplier
	result.PreOutcomeDamage = result.Damage
}

// Modifies damage based on Armor or Magic resistances, depending on the damage type.
func (spell *Spell) ResistanceMultiplier(sim *Simulation, isPeriodic bool, attackTable *AttackTable) float64 {
	if spell.Flags.Matches(SpellFlagIgnoreResists) {
		return 1
	}

	if spell.SpellSchool.Matches(SpellSchoolPhysical) {
		// All physical dots (Bleeds) ignore armor.
		if isPeriodic && !spell.Flags.Matches(SpellFlagApplyArmorReduction) {
			return 1
		}

		// Physical resistance (armor).
		return attackTable.GetArmorDamageModifier(spell)
	}

	if !spell.canBeResisted(attackTable.Defender) {
		return 1
	}

	averageResist := attackTable.AverageResist(spell)
	if averageResist <= 0 {
		return 1
	}

	thresholds := partialResistRollThresholds(averageResist)

	switch resistanceRoll := sim.RandomFloat("Partial Resist"); {
	case resistanceRoll < thresholds[0].cumulativeChance:
		return thresholds[0].damageMultiplier()
	case resistanceRoll < thresholds[1].cumulativeChance:
		return thresholds[1].damageMultiplier()
	case resistanceRoll < thresholds[2].cumulativeChance:
		return thresholds[2].damageMultiplier()
	default:
		return thresholds[3].damageMultiplier()
	}
}

// canBeResisted is CalcAbsorbResist's guard. A binary spell rolled its resist as
// part of the hit check and never partially resists, and holy is only resisted by
// creatures.
func (spell *Spell) canBeResisted(defender *Unit) bool {
	if spell.Flags.Matches(SpellFlagBinary) {
		return false
	}
	if spell.SpellSchool.Matches(SpellSchoolHoly) && defender.Type != EnemyUnit {
		return false
	}
	return true
}

// AverageResist is the share of damage the target resists on average. With no
// resistance to meet, all a spell gets is the flat 5 per level of difference,
// which works out to 3.6145% against a level 83 target.
func (at *AttackTable) AverageResist(spell *Spell) float64 {
	return EffectiveResistChance(
		at.Defender.schoolResistance(spell.SpellSchool),
		at.Attacker.stats[stats.SpellPenetration],
		at.Attacker.Level,
		at.Defender.Level,
		false)
}

var magicSchools = [...]SpellSchool{SpellSchoolArcane, SpellSchoolFire, SpellSchoolFrost, SpellSchoolHoly, SpellSchoolNature, SpellSchoolShadow}

// schoolResistance is Unit::GetResistance(mask): a spell of several schools meets
// the lowest of their resistances, so Frostfire Bolt goes through whichever of
// fire and frost is weaker. Holy has no resistance stat and counts as none.
// Physical never gets here, since it's armor instead.
func (unit *Unit) schoolResistance(school SpellSchool) float64 {
	var resistance float64
	found := false
	for _, single := range magicSchools {
		if !school.Matches(single) {
			continue
		}
		value := 0.0
		if stat, ok := single.ResistanceStat(); ok {
			value = unit.GetStat(stat)
		}
		if !found || value < resistance {
			resistance = value
			found = true
		}
	}
	return resistance
}

// GetArmorDamageModifier is CalcArmorReducedDamage. Note the two levels: how much
// a given amount of armor mitigates comes from the attacker's level, but how far
// armor penetration can reach through it comes from the target's.
func (at *AttackTable) GetArmorDamageModifier(spell *Spell) float64 {
	defenderArmor := at.Defender.Armor()
	armorPenRating := at.Attacker.stats[stats.ArmorPenetration] + spell.BonusArmorPenRating
	reducibleArmor := ArmorPenetrationCap(defenderArmor, at.Defender.Level)
	effectiveArmor := defenderArmor - reducibleArmor*at.Attacker.ArmorPenetrationPercentage(armorPenRating)
	return ArmorMultiplier(effectiveArmor, at.Attacker.Level)
}

type Threshold struct {
	cumulativeChance float64
	bracket          int
}

func (x Threshold) damageMultiplier() float64 {
	return 1 - 0.1*float64(x.bracket)
}

type Thresholds [4]Threshold

func (x Thresholds) String() string {
	var sb strings.Builder
	var chance float64
	for _, t := range x {
		sb.WriteString(fmt.Sprintf("%.1f%% for %d%% ", (t.cumulativeChance-chance)*100, t.bracket*10))
		if t.cumulativeChance >= 1 {
			break
		}
		chance = t.cumulativeChance
	}
	return sb.String()
}

// partialResistRollThresholds turns CalcAbsorbResist's discrete table into the
// cumulative form the roll walks. Only four brackets ever carry weight at once,
// and for a boss's 3.6% it is the first three.
func partialResistRollThresholds(averageResist float64) Thresholds {
	buckets := ResistBuckets(averageResist)

	const eps = 1e-9 // imprecision guard (25-50-25 might become almost0-25-50-25-almost0)

	var thresholds Thresholds
	var cumulativeChance float64
	var index int
	for bracket, chance := range buckets {
		if chance <= eps {
			continue
		}
		cumulativeChance += chance
		thresholds[index] = Threshold{cumulativeChance: cumulativeChance, bracket: bracket}
		index++
		if index == len(thresholds) {
			break
		}
	}

	if thresholds[index-1].cumulativeChance < 1 { // also guards against floating point imprecision
		thresholds[index-1].cumulativeChance = 1
	}

	return thresholds
}
