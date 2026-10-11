package core

import (
	"math"

	"github.com/wowsims/wotlk/sim/core/serverdata"
)

// serverDerived is what the hit and proc paths read off the spell's server data on every hit and tick,
// worked out once. Class code writes Flags directly and tests build Spells by hand, so it keeps every
// input it came from and each read compares them. The row is keyed by pointer: never edit one in place.
type serverDerived struct {
	serverSpell   *serverdata.Spell
	flags         SpellFlag // serverDerivedFlags only
	whiteHit      bool
	unitType      UnitType
	summonedAsPet bool
	valid         bool

	weapon, normalized bool
	ticksAsCasts       bool
	bleed              bool
	// takenCoefficient's, direct then periodic
	taken         [2]float32
	takenByStacks [2]bool
}

const serverDerivedFlags = SpellFlagNoServerData | SpellFlagIncludeTargetBonusDamage

func (spell *Spell) derived() *serverDerived {
	d := &spell.serverDerived
	flags, whiteHit := spell.Flags&serverDerivedFlags, spell.ProcMask.Matches(ProcMaskWhiteHit)
	if d.valid && d.serverSpell == spell.serverSpell && d.flags == flags && d.whiteHit == whiteHit &&
		d.unitType == spell.Unit.Type && d.summonedAsPet == spell.Unit.SummonedAsPet {
		return d
	}
	*d = serverDerived{
		serverSpell:   spell.serverSpell,
		flags:         flags,
		whiteHit:      whiteHit,
		unitType:      spell.Unit.Type,
		summonedAsPet: spell.Unit.SummonedAsPet,
		valid:         true,
	}
	d.weapon, d.normalized = spell.resolveWeaponDamage()
	sd := spell.ServerSpell()
	d.ticksAsCasts = ticksAsCastsOn(sd)
	d.bleed = spell.hasBleedMechanic(sd, d.weapon)
	if sd != nil {
		for i, isPeriodic := range [2]bool{false, true} {
			d.taken[i], d.takenByStacks[i] = spell.takenCoefficient(sd, isPeriodic)
		}
	}
	return d
}

func (spell *Spell) dealsWeaponDamage() (weapon, normalized bool) {
	d := spell.derived()
	return d.weapon, d.normalized
}

// resolveWeaponDamage is whether the server deals the hit through Unit::MeleeDamageBonusDone and
// MeleeDamageBonusTaken: a white swing or a spell with a weapon damage effect, normalized when one is
// SPELL_EFFECT_NORMALIZED_WEAPON_DMG. A spell without server data goes by SpellFlagIncludeTargetBonusDamage.
func (spell *Spell) resolveWeaponDamage() (weapon, normalized bool) {
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

// hasBleedMechanic is whether SPELL_AURA_MOD_MECHANIC_DAMAGE_TAKEN_PERCENT's bleed auras (Mangle, Trauma,
// Stampede) reach the spell, hit and ticks alike: its mechanic mask has bleed, or it's Shred or Maul, which
// Unit::MeleeDamageBonusTaken counts in by family flag.
func (spell *Spell) hasBleedMechanic(sd *serverdata.Spell, weapon bool) bool {
	if sd == nil {
		return false
	}
	return sd.Flags&serverdata.FlagBleed != 0 ||
		(weapon && sd.Family == spellFamilyDruid && sd.FamilyFlags[0]&0x8800 != 0)
}

// SPELLFAMILY_DRUID
const spellFamilyDruid = 7

// takesBleedBonus goes by the server's mechanic. A spell without server data is a bleed when it's a
// physical dot.
func (spell *Spell) takesBleedBonus(isPeriodic bool) bool {
	if spell.ServerSpell() == nil {
		return isPeriodic && spell.SpellSchool.Matches(SpellSchoolPhysical)
	}
	return spell.derived().bleed
}

// flatDamageTaken is a SPELL_AURA_MOD_DAMAGE_TAKEN amount on a hit: all of it on a weapon hit
// (Unit::MeleeDamageBonusTaken), else int32(amount × the taken coefficient × the aura's stacks)
// (Unit::SpellDamageBonusTaken), which is how Gift of Arthas adds 3 to an Instant Poison hit.
func (spell *Spell) flatDamageTaken(amount float64, target *Unit, isPeriodic bool) float64 {
	d := spell.derived()
	if d.weapon && !isPeriodic {
		return amount
	}
	if spell.ServerSpell() == nil {
		return 0
	}
	stacks := float32(1)
	if isPeriodic && int(target.UnitIndex) < len(spell.dots) {
		if dot := spell.dots.Get(target); dot != nil && dot.GetStacks() > 1 {
			stacks = float32(dot.GetStacks())
		}
	}
	i := 0
	if isPeriodic {
		i = 1
	}
	coefficient := d.taken[i]
	if d.takenByStacks[i] {
		coefficient *= stacks
	}
	// float32, as the server multiplies it
	return math.Trunc(float64(float32(amount) * coefficient * stacks))
}

// takenCoefficient is the spell's spell_bonus_data direct or dot value, else
// Unit::CalculateDefaultCoefficient, which then takes the stacks (so a stacked dot counts them twice).
func (spell *Spell) takenCoefficient(sd *serverdata.Spell, isPeriodic bool) (c float32, byStacks bool) {
	if b := serverdata.BonusBySpellID(sd.ID); b != nil {
		c := b.Direct
		if isPeriodic {
			c = b.Dot
		}
		if c > 0 {
			return c, false
		}
	}
	return spell.defaultCoefficient(sd, isPeriodic), true
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
