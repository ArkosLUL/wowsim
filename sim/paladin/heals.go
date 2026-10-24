package paladin

import (
	"math"
	"time"

	"github.com/wowsims/wotlk/sim/core"
)

func (paladin *Paladin) registerHealingSpells() {
	paladin.libramEffects()
	paladin.registerBeaconOfLightSpell()
	paladin.registerHolyLightSpell()
	paladin.registerFlashOfLightSpell()
	paladin.registerHolyShockSpell()
	paladin.registerSacredShieldSpell()
	paladin.registerDivineFavorSpell()
	paladin.registerDivineIlluminationSpell()
	paladin.registerHolyTalentProcs()
	paladin.registerHolyItems()
}

// healingDoneMultiplier is the healing done percents (aura 136) a direct heal carries in its own multiplier:
// Divinity's 1% a point.
func (paladin *Paladin) healingDoneMultiplier() float64 {
	return 1 + 0.01*float64(paladin.Talents.Divinity)
}

// healingDoneFactor is what SpellHealingBonusDone takes from auras that come and go: Avenging Wrath's 20% healing
// done, and Divine Plea's -50% SPELLMOD_DAMAGE, which only reaches Holy Light, Flash of Light and Holy Shock's heal.
func (paladin *Paladin) healingDoneFactor(underPlea bool) float64 {
	factor := 1.0
	if paladin.AvengingWrathAura != nil && paladin.AvengingWrathAura.IsActive() {
		factor *= 1.2
	}
	if paladin.DivineIlluminationAura != nil && paladin.DivineIlluminationAura.IsActive() && paladin.holyItems.t10DivineIllumination {
		factor *= 1.35
	}
	if underPlea && paladin.DivinePleaAura != nil && paladin.DivinePleaAura.IsActive() {
		factor *= 0.5
	}
	return factor
}

// directHeal deals one of Holy Light, Flash of Light and Holy Shock's heal, the paladin's procs off it, then Light's
// Beacon's copy of it: the heal before the target's taken terms, crit included (HealInfo::GetHealBeforeTakenMods).
func (paladin *Paladin) directHeal(sim *core.Simulation, target *core.Unit, spell *core.Spell, baseHealing float64,
	copySpell *core.Spell) {
	baseHealing *= paladin.healingDoneFactor(true)
	// SPELLMOD_DAMAGE flats come after every percent
	if flat := paladin.flatHealing(spell); flat != 0 {
		baseHealing += flat / spell.CasterHealingMultiplier()
	}
	infusionCrit := 0.0
	if spell == paladin.HolyLight {
		infusionCrit = paladin.infusionHolyLightCrit()
	}
	spell.BonusCritRating += infusionCrit
	result := spell.CalcHealing(sim, target, baseHealing, spell.OutcomeHealingCrit)
	spell.BonusCritRating -= infusionCrit
	paladin.useDivineFavor(sim)

	crit, heal := result.DidCrit(), result.Damage
	beforeTaken := math.Trunc(baseHealing * spell.CasterHealingMultiplier())
	if crit {
		beforeTaken = math.Trunc(beforeTaken * spell.CritMultiplier)
	}
	spell.DealHealing(sim, result)

	switch spell {
	case paladin.HolyLight:
		if infusionCrit != 0 {
			paladin.infusionOfLight.aura.Deactivate(sim)
		}
		paladin.lightsGraceOnHolyLight(sim)
		paladin.onHolyLight(sim, target, heal)
		if crit {
			paladin.illuminate(sim, 29)
		}
	case paladin.FlashOfLight:
		paladin.infusionOnFlashOfLight(sim, target, heal)
		if crit {
			paladin.illuminate(sim, 7)
		}
	case paladin.HolyShockHeal:
		paladin.onHolyShockHeal(sim, target, crit, heal)
		if crit {
			paladin.illuminate(sim, 18)
			paladin.infusionOnHolyShockCrit(sim)
		}
	}

	if heal > 0 {
		paladin.copyToBeacon(sim, target, copySpell, beforeTaken)
	}
}

func (paladin *Paladin) registerHolyLightSpell() {
	paladin.HolyLight = paladin.RegisterSpell(core.SpellConfig{
		ActionID:    core.ActionID{SpellID: 48782},
		SpellSchool: core.SpellSchoolHoly,
		ProcMask:    core.ProcMaskSpellHealing,
		Flags:       core.SpellFlagHelpful | core.SpellFlagAPL,

		ManaCost: core.ManaCostOptions{
			BaseCost: 0.29,
			FlatMod:  paladin.holyItems.holyLightCostFlat,
			// T7's 4 piece, 5%
			Multiplier: 1 - core.TernaryFloat64(paladin.HasSetBonus(ItemSetRedemptionRegalia, 4), 0.05, 0),
		},
		Cast: core.CastConfig{
			DefaultCast: core.Cast{
				GCD:      core.GCDDefault,
				CastTime: time.Millisecond * 2500,
			},
			ModifyCast: func(_ *core.Simulation, _ *core.Spell, cast *core.Cast) {
				paladin.lightsGraceCastTime(cast)
				paladin.holinessCastTime(cast)
			},
		},

		BonusCritRating:  paladin.sanctifiedLightCrit(),
		DamageMultiplier: paladin.healingDoneMultiplier() * paladin.healingLightMultiplier(),
		CritMultiplier:   paladin.DefaultHealingCritMultiplier(),
		ThreatMultiplier: 1,

		Direct: core.SpellEffect{Effect: 0, Min: 4888, Max: 5444, SP: 1.66},

		ApplyEffects: func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
			baseHealing := spell.Direct.Roll(sim) + core.PowerBonus(spell.Direct.SP, spell.HealingPower(target))
			paladin.directHeal(sim, target, spell, baseHealing, paladin.beacon.holyLight)
		},
	})
}

func (paladin *Paladin) registerFlashOfLightSpell() {
	paladin.FlashOfLight = paladin.RegisterSpell(core.SpellConfig{
		ActionID:    core.ActionID{SpellID: 48785},
		SpellSchool: core.SpellSchoolHoly,
		ProcMask:    core.ProcMaskSpellHealing,
		Flags:       core.SpellFlagHelpful | core.SpellFlagAPL,

		ManaCost: core.ManaCostOptions{
			BaseCost: 0.07,
		},
		Cast: core.CastConfig{
			DefaultCast: core.Cast{
				GCD:      core.GCDDefault,
				CastTime: time.Millisecond * 1500,
			},
			ModifyCast: func(_ *core.Simulation, _ *core.Spell, cast *core.Cast) {
				paladin.infusionFlashCastTime(cast)
			},
		},

		BonusCritRating:  paladin.glyphOfFlashOfLightCrit(),
		DamageMultiplier: paladin.healingDoneMultiplier() * paladin.healingLightMultiplier(),
		CritMultiplier:   paladin.DefaultHealingCritMultiplier(),
		ThreatMultiplier: 1,

		Direct: core.SpellEffect{Effect: 0, Min: 788, Max: 882, SP: 1},

		ApplyEffects: func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
			baseHealing := spell.Direct.Roll(sim) + core.PowerBonus(spell.Direct.SP, spell.HealingPower(target))
			bonusCrit := paladin.sacredShieldFlashCrit(target)
			spell.BonusCritRating += bonusCrit
			paladin.directHeal(sim, target, spell, baseHealing, paladin.beacon.flashOfLight)
			spell.BonusCritRating -= bonusCrit
		},
	})
}

// divineFavorSpells are the spells 20216's +100 crit chance reaches: Holy Light, Flash of Light and both of
// Holy Shock's.
func (paladin *Paladin) divineFavorSpells() []*core.Spell {
	var spells []*core.Spell
	for _, spell := range []*core.Spell{paladin.HolyLight, paladin.FlashOfLight, paladin.HolyShockHeal, paladin.HolyShockDamage} {
		if spell != nil {
			spells = append(spells, spell)
		}
	}
	return spells
}

func (paladin *Paladin) registerDivineFavorSpell() {
	if !paladin.Talents.DivineFavor {
		return
	}
	actionID := core.ActionID{SpellID: 20216}
	bonusCrit := 100 * core.CritRatingPerCritChance
	affected := paladin.divineFavorSpells()
	cooldown := core.Cooldown{Timer: paladin.NewTimer(), Duration: time.Minute * 2}

	// SPELL_ATTR0_COOLDOWN_ON_EVENT: held while the buff is up, then the 2 min start as it goes
	// (Aura::_ApplyForTarget / _UnapplyForTarget). The cast sets the timer before the aura goes on.
	paladin.DivineFavorAura = paladin.RegisterAura(core.Aura{
		Label:    "Divine Favor",
		ActionID: actionID,
		Duration: core.NeverExpires,
		OnGain: func(aura *core.Aura, sim *core.Simulation) {
			for _, spell := range affected {
				spell.BonusCritRating += bonusCrit
			}
			cooldown.Set(core.NeverExpires)
		},
		OnExpire: func(aura *core.Aura, sim *core.Simulation) {
			for _, spell := range affected {
				spell.BonusCritRating -= bonusCrit
			}
			cooldown.Set(sim.CurrentTime + cooldown.Duration)
		},
	})

	paladin.DivineFavor = paladin.RegisterSpell(core.SpellConfig{
		ActionID:    actionID,
		SpellSchool: core.SpellSchoolHoly,
		ProcMask:    core.ProcMaskEmpty,
		Flags:       core.SpellFlagAPL,

		ManaCost: core.ManaCostOptions{
			BaseCost:   0.03,
			Multiplier: 1 - 0.02*float64(paladin.Talents.Benediction),
		},
		Cast: core.CastConfig{
			CD: cooldown,
		},

		ApplyEffects: func(sim *core.Simulation, _ *core.Unit, spell *core.Spell) {
			paladin.DivineFavorAura.Activate(sim)
			spell.LandOn(sim, &paladin.Unit)
		},
	})
}

// useDivineFavor takes Divine Favor's one charge: spell_proc 20216 needs its spell mod on the spell
// (PROC_ATTR_REQ_SPELLMOD) and a landed hit, so a spell it covers calls this once its crit is rolled.
func (paladin *Paladin) useDivineFavor(sim *core.Simulation) {
	if paladin.DivineFavorAura != nil && paladin.DivineFavorAura.IsActive() {
		paladin.DivineFavorAura.Deactivate(sim)
	}
}

func (paladin *Paladin) registerDivineIlluminationSpell() {
	if !paladin.Talents.DivineIllumination {
		return
	}
	actionID := core.ActionID{SpellID: 31842}
	// T10's 2 piece casts 71166 beside it, 35% healing done
	t10 := paladin.HasSetBonus(ItemSetLightswornGarb, 2)

	// aura 72 on every school but physical: the cost multiplier, not a spell mod
	paladin.DivineIlluminationAura = paladin.RegisterAura(core.Aura{
		Label:    "Divine Illumination",
		ActionID: actionID,
		Duration: time.Second * 15,
		OnGain: func(aura *core.Aura, sim *core.Simulation) {
			aura.Unit.AddSchoolCostMultiplier(core.SpellSchoolMagic, -0.5)
			if t10 {
				aura.Unit.AddHealingDonePct(35)
			}
		},
		OnExpire: func(aura *core.Aura, sim *core.Simulation) {
			aura.Unit.AddSchoolCostMultiplier(core.SpellSchoolMagic, 0.5)
			if t10 {
				aura.Unit.RemoveHealingDonePct(35)
			}
		},
	})

	paladin.DivineIllumination = paladin.RegisterSpell(core.SpellConfig{
		ActionID:    actionID,
		SpellSchool: core.SpellSchoolHoly,
		ProcMask:    core.ProcMaskEmpty,
		Flags:       core.SpellFlagAPL,

		Cast: core.CastConfig{
			CD: core.Cooldown{
				Timer:    paladin.NewTimer(),
				Duration: time.Minute * 3,
			},
		},

		ApplyEffects: func(sim *core.Simulation, _ *core.Unit, spell *core.Spell) {
			paladin.DivineIlluminationAura.Activate(sim)
			spell.LandOn(sim, &paladin.Unit)
		},
	})
}
