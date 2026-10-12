package core

import (
	"math"

	"github.com/wowsims/wotlk/sim/core/stats"
)

// strikeTotalPct is the hand's TOTAL_PCT a physical strike or swing carries inside its weapon damage
// (Player::CalculateMinMaxDamage): half in the off hand, then the aura 79 percents. Off the physical
// school the weapon is read without it, off hand included.
func (spell *Spell) strikeTotalPct(hand Hand) float64 {
	if !spell.SpellSchool.Matches(SpellSchoolPhysical) {
		return 1
	}
	pct := spell.TotalPctMultiplier
	if !spell.Flags.Matches(SpellFlagIgnoreAttackerModifiers) {
		pct *= spell.Unit.PseudoStats.DamageDealtMultiplier *
			spell.Unit.PseudoStats.SchoolDamageDealtMultiplier[stats.SchoolIndexPhysical]
	}
	if hand == OffHand {
		pct *= 0.5
	}
	return pct
}

// strikeDoneMultiplier is what's left of a physical whole strike's attacker multiplier once the hand's
// TOTAL_PCT is in the base: Unit::MeleeDamageBonusDone's DoneTotalMod and the damage spell mods.
func (spell *Spell) strikeDoneMultiplier(target *Unit) float64 {
	multiplier := spell.DamageMultiplier * spell.DamageMultiplierAdditive
	if spell.TotalPctMultiplier != 1 {
		multiplier /= spell.TotalPctMultiplier
	}
	if !spell.Flags.Matches(SpellFlagIgnoreAttackerModifiers) {
		multiplier *= spell.Unit.AttackTables[target.UnitIndex].DamageDealtMultiplier
	}
	return multiplier
}

// StrikeDamage is Unit::CalculateDamage for a strike or a white swing: a whole roll between the truncated
// ends of the weapon's range, with the attack power at speed, the flat bonuses and the hand's TOTAL_PCT
// inside. It marks the spell, so doneDamage leaves that percent out of the multiplier.
func (spell *Spell) StrikeDamage(sim *Simulation, weapon *Weapon, hand Hand, speed float64, attackPower float64, flat float64) float64 {
	if weapon.AttackPowerPerDPS == 0 {
		// no weapon in that hand
		return math.NaN()
	}
	spell.wholeStrike = true
	return weapon.wholeRoll(sim, speed, attackPower, flat, float32(spell.strikeTotalPct(hand)))
}

// WholeDamage is a weapon roll with no percent inside, for a hit whose multipliers come after it (the rune
// weapon's copies, a copy of a normalized strike).
func (weapon *Weapon) WholeDamage(sim *Simulation, speed float64, attackPower float64, flat float64) float64 {
	return weapon.wholeRoll(sim, speed, attackPower, flat, 1)
}

// WholeBaseDamage is irand(int32(min), int32(max)) of the bare weapon, for a spell script that adds its own
// weapon roll (Steady Shot).
func (weapon *Weapon) WholeBaseDamage(sim *Simulation) float64 {
	low, high := math.Trunc(weapon.BaseDamageMin), math.Trunc(weapon.BaseDamageMax)
	return low + math.Trunc((high-low+1)*sim.UnitRandomFloat(weapon.unit, "Weapon Base Damage"))
}

func (weapon *Weapon) wholeRoll(sim *Simulation, speed float64, attackPower float64, flat float64, pct float32) float64 {
	// float32 steps, as the server takes them
	added := float32(float32(attackPower)/float32(weapon.AttackPowerPerDPS)*float32(speed)) + float32(flat)
	lo := max(float32((float32(weapon.BaseDamageMin)+added)*pct), 0)
	hi := max(float32((float32(weapon.BaseDamageMax)+added)*pct), 0)
	if lo > hi {
		lo, hi = hi, lo
	}
	low, high := math.Trunc(float64(lo)), math.Trunc(float64(hi))
	return low + math.Trunc((high-low+1)*sim.UnitRandomFloat(weapon.unit, "Weapon Base Damage"))
}

func (spell *Spell) MHStrikeDamage(sim *Simulation, attackPower float64) float64 {
	weapon := spell.Unit.AutoAttacks.MH()
	return spell.StrikeDamage(sim, weapon, MainHand, weapon.SwingSpeed, attackPower, spell.BonusWeaponDamage())
}

func (spell *Spell) MHNormalizedStrikeDamage(sim *Simulation, attackPower float64) float64 {
	weapon := spell.Unit.AutoAttacks.MH()
	return spell.StrikeDamage(sim, weapon, MainHand, weapon.NormalizedSwingSpeed, attackPower, spell.BonusWeaponDamage())
}

func (spell *Spell) OHStrikeDamage(sim *Simulation, attackPower float64) float64 {
	weapon := spell.Unit.AutoAttacks.OH()
	return spell.StrikeDamage(sim, weapon, OffHand, weapon.SwingSpeed, attackPower, spell.BonusWeaponDamage())
}

func (spell *Spell) OHNormalizedStrikeDamage(sim *Simulation, attackPower float64) float64 {
	weapon := spell.Unit.AutoAttacks.OH()
	return spell.StrikeDamage(sim, weapon, OffHand, weapon.NormalizedSwingSpeed, attackPower, spell.BonusWeaponDamage())
}

// RangedStrikeDamage takes the ammo's damage at the same speed, which the server adds to the weapon's range.
func (spell *Spell) RangedStrikeDamage(sim *Simulation, attackPower float64, ammo float64) float64 {
	weapon := spell.Unit.AutoAttacks.Ranged()
	return spell.StrikeDamage(sim, weapon, MainHand, weapon.SwingSpeed, attackPower, ammo+spell.BonusWeaponDamage())
}

func (spell *Spell) RangedNormalizedStrikeDamage(sim *Simulation, attackPower float64, ammo float64) float64 {
	weapon := spell.Unit.AutoAttacks.Ranged()
	return spell.StrikeDamage(sim, weapon, MainHand, weapon.NormalizedSwingSpeed, attackPower, ammo+spell.BonusWeaponDamage())
}

// StrikeAverageDamage is StrikeDamage's mean, for expected damage. It marks the spell too.
func (spell *Spell) StrikeAverageDamage(weapon *Weapon, hand Hand, speed float64, attackPower float64, flat float64) float64 {
	spell.wholeStrike = true
	return (weapon.AverageDamage() + attackPower/weapon.AttackPowerPerDPS*speed + flat) * spell.strikeTotalPct(hand)
}

// StrikeFixed is a strike's fixed or scripted bonus after the hand's TOTAL_PCT, truncated: Spell::EffectWeaponDmg
// scales both for a physical strike only.
func (spell *Spell) StrikeFixed(bonus float64, hand Hand) float64 {
	if bonus == 0 || !spell.SpellSchool.Matches(SpellSchoolPhysical) {
		return bonus
	}
	return float64(int32(float32(bonus) * float32(spell.strikeTotalPct(hand))))
}

// ApplyPct is the server's ApplyPct on a strike's whole damage, int32(damage × percent / 100) in float32:
// the weapon percent, then a script's total percent. pct is a share, 1.5 for 150%.
func ApplyPct(damage float64, pct float64) float64 {
	if math.IsNaN(damage) {
		return damage
	}
	return float64(int32(float32(float32(damage)*float32(pct*100)) / 100))
}
