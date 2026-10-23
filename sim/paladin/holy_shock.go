package paladin

import (
	"time"

	"github.com/wowsims/wotlk/sim/core"
)

// registerHolyShockSpell is 48825 and spell_pal_holy_shock: its dummy casts 48821's heal on a friendly target and
// 48823's damage on any other.
func (paladin *Paladin) registerHolyShockSpell() {
	if !paladin.Talents.HolyShock {
		return
	}

	paladin.HolyShockHeal = paladin.RegisterSpell(core.SpellConfig{
		ActionID:    core.ActionID{SpellID: 48821},
		SpellSchool: core.SpellSchoolHoly,
		ProcMask:    core.ProcMaskSpellHealing,
		Flags:       core.SpellFlagHelpful | core.SpellFlagNoOnCastComplete,

		BonusCritRating:  paladin.sanctifiedLightCrit(),
		DamageMultiplier: paladin.healingDoneMultiplier() * paladin.healingLightMultiplier(),
		CritMultiplier:   paladin.DefaultHealingCritMultiplier(),
		ThreatMultiplier: 1,

		Direct: core.SpellEffect{Effect: 0, Min: 2401, Max: 2599, SP: 0.8057},

		ApplyEffects: func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
			baseHealing := spell.Direct.Roll(sim) + core.PowerBonus(spell.Direct.SP, spell.HealingPower(target))
			paladin.directHeal(sim, target, spell, baseHealing, paladin.beacon.holyShock)
		},
	})

	paladin.HolyShockDamage = paladin.RegisterSpell(core.SpellConfig{
		ActionID:    core.ActionID{SpellID: 48823},
		SpellSchool: core.SpellSchoolHoly,
		ProcMask:    core.ProcMaskSpellDamage,
		Flags:       core.SpellFlagNoOnCastComplete,

		BonusCritRating:  paladin.sanctifiedLightCrit(),
		DamageMultiplier: paladin.healingLightMultiplier(),
		CritMultiplier:   paladin.SpellCritMultiplier(),
		ThreatMultiplier: 1,

		Direct: core.SpellEffect{Effect: 0, Min: 1296, Max: 1402, SP: 0.4286},

		ApplyEffects: func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
			baseDamage := spell.Direct.Roll(sim) + core.PowerBonus(spell.Direct.SP, spell.SpellPower())
			result := spell.CalcDamage(sim, target, baseDamage, spell.OutcomeMagicHitAndCrit)
			landed, crit := result.Landed(), result.DidCrit()
			spell.DealDamage(sim, result)
			if landed {
				paladin.useDivineFavor(sim)
			}
			if crit {
				paladin.infusionOnHolyShockCrit(sim)
			}
		},
	})

	paladin.HolyShock = paladin.RegisterSpell(core.SpellConfig{
		ActionID:    core.ActionID{SpellID: 48825},
		SpellSchool: core.SpellSchoolHoly,
		ProcMask:    core.ProcMaskEmpty,
		Flags:       core.SpellFlagAPL,

		ManaCost: core.ManaCostOptions{
			BaseCost:   0.18,
			Multiplier: 1 - 0.02*float64(paladin.Talents.Benediction),
		},
		Cast: core.CastConfig{
			DefaultCast: core.Cast{
				GCD: core.GCDDefault,
			},
			CD: core.Cooldown{
				Timer:    paladin.NewTimer(),
				Duration: time.Second * 6,
			},
		},

		ApplyEffects: func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
			if target.Type == core.EnemyUnit {
				paladin.HolyShockDamage.Cast(sim, target)
			} else {
				paladin.HolyShockHeal.Cast(sim, target)
			}
			spell.LandOn(sim, target)
		},
	})
}
