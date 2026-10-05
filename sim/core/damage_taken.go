package core

import (
	"math"

	"github.com/wowsims/wotlk/sim/core/serverdata"
)

// dealsWeaponDamage is whether the server deals the hit through Unit::MeleeDamageBonusDone and
// MeleeDamageBonusTaken: a white swing or a spell with a weapon damage effect, normalized when one is
// SPELL_EFFECT_NORMALIZED_WEAPON_DMG. A spell without server data goes by SpellFlagIncludeTargetBonusDamage.
func (spell *Spell) dealsWeaponDamage() (weapon, normalized bool) {
	if spell.ProcMask.Matches(ProcMaskWhiteHit) {
		return true, false
	}
	sd := spell.ServerSpell()
	if sd == nil {
		return spell.Flags.Matches(SpellFlagIncludeTargetBonusDamage), false
	}
	for _, e := range sd.Effects {
		weapon = weapon || isWeaponEffect(e.Effect)
		normalized = normalized || e.Effect == effectNormalizedWeaponDmg
	}
	return weapon, normalized
}

// flatDamageTaken is a SPELL_AURA_MOD_DAMAGE_TAKEN amount on a hit: all of it on a weapon hit
// (Unit::MeleeDamageBonusTaken), else int32(amount × the taken coefficient × the aura's stacks)
// (Unit::SpellDamageBonusTaken), which is how Gift of Arthas adds 3 to an Instant Poison hit.
func (spell *Spell) flatDamageTaken(amount float64, target *Unit, isPeriodic bool) float64 {
	if weapon, _ := spell.dealsWeaponDamage(); weapon && !isPeriodic {
		return amount
	}
	sd := spell.ServerSpell()
	if sd == nil {
		return 0
	}
	stacks := float32(1)
	if isPeriodic && int(target.UnitIndex) < len(spell.dots) {
		if dot := spell.dots.Get(target); dot != nil && dot.GetStacks() > 1 {
			stacks = float32(dot.GetStacks())
		}
	}
	// float32, as the server multiplies it
	return math.Trunc(float64(float32(amount) * spell.takenCoefficient(sd, isPeriodic, stacks) * stacks))
}

// takenCoefficient is the spell's spell_bonus_data direct or dot value, else
// Unit::CalculateDefaultCoefficient times the stacks (so a stacked dot counts them twice).
func (spell *Spell) takenCoefficient(sd *serverdata.Spell, isPeriodic bool, stacks float32) float32 {
	if b := serverdata.BonusBySpellID(sd.ID); b != nil {
		c := b.Direct
		if isPeriodic {
			c = b.Dot
		}
		if c > 0 {
			return c
		}
	}
	return spell.defaultCoefficient(sd, isPeriodic) * stacks
}

// SpellEffects and AuraType values the default coefficient reads, besides spell_effect.go's.
const (
	effectApplyAura           = 6
	effectEnvironmentalDamage = 7
	effectPowerDrain          = 8
	effectPowerBurn           = 62

	auraTypePeriodicTriggerFromClient = 48
)

// defaultCoefficient is Unit::CalculateDefaultCoefficient: the cast time's share of 3.5 s, a dot's split
// over its ticks.
func (spell *Spell) defaultCoefficient(sd *serverdata.Spell, isPeriodic bool) float32 {
	channeled := sd.Flags&serverdata.FlagChanneled != 0
	dotFactor := float32(1)
	if isPeriodic {
		if !channeled && sd.DurationMs > 0 {
			dotFactor = float32(sd.DurationMs) / 15000
		}
		if ticks := maxTicks(sd); ticks > 0 {
			dotFactor /= float32(ticks)
		}
	}
	castingTime := sd.CastMs
	if channeled {
		castingTime = sd.DurationMs
	}
	return float32(spell.castingTimeForBonus(sd, isPeriodic, castingTime)) / 3500 * dotFactor
}

// maxTicks is SpellInfo::GetMaxTicks.
func maxTicks(sd *serverdata.Spell) int32 {
	duration := sd.DurationMs
	if duration == 0 {
		return 1
	}
	duration = min(duration, 30000)
	for _, e := range sd.Effects {
		if e.Effect != effectApplyAura || e.AmplitudeMs == 0 {
			continue
		}
		switch e.Aura {
		case auraTypePeriodicDamage, auraTypePeriodicHeal, auraTypePeriodicLeech, auraTypePeriodicTriggerFromClient:
			return max(0, duration/e.AmplitudeMs)
		}
	}
	return 6
}

// castingTimeForBonus is Unit::GetCastingTimeForBonus, in its uint32 milliseconds.
func (spell *Spell) castingTimeForBonus(sd *serverdata.Spell, isPeriodic bool, castingTime int32) int32 {
	if castingTime == 0 && spell.Unit.Type == PetUnit && !spell.Unit.SummonedAsPet {
		return 3500
	}
	channeled := sd.Flags&serverdata.FlagChanneled != 0
	castingTime = min(max(castingTime, 1500), 7000)
	if isPeriodic && !channeled {
		castingTime = 3500
	}

	var overTime int32
	effects := 0
	directDamage, leech := false, false
	for _, e := range sd.Effects {
		switch e.Effect {
		case effectSchoolDamage, effectPowerDrain, effectHealthLeech, effectEnvironmentalDamage, effectPowerBurn, effectHeal:
			directDamage = true
		case effectApplyAura:
			switch e.Aura {
			case auraTypePeriodicDamage, auraTypePeriodicHeal, auraTypePeriodicLeech:
				if sd.DurationMs != 0 {
					overTime = sd.DurationMs
				}
			default:
				effects++
			}
		}
		leech = leech || e.Effect == effectHealthLeech || (e.Effect == effectApplyAura && e.Aura == auraTypePeriodicLeech)
	}

	if overTime > 0 && directDamage {
		original := min(max(sd.CastMs, 1500), 7000)
		dot := float32(overTime) / 15000
		ptot := dot / (dot + float32(original)/3500)
		switch {
		case isPeriodic:
			castingTime = int32(float32(castingTime) * ptot)
		case ptot < 1:
			castingTime = int32(float32(castingTime) * (1 - ptot))
		default:
			castingTime = 0
		}
	}
	if sd.Flags&serverdata.FlagTargetsArea != 0 {
		castingTime /= 2
	}
	if leech {
		castingTime /= 2
	}
	for range effects {
		castingTime = int32(float32(castingTime) * 0.95)
	}
	return castingTime
}
