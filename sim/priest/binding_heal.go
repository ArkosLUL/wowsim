package priest

import (
	"time"

	"github.com/wowsims/wotlk/sim/core"
)

func (priest *Priest) registerBindingHealSpell() {
	priest.BindingHeal = priest.RegisterSpell(core.SpellConfig{
		ActionID:    core.ActionID{SpellID: 48120},
		SpellSchool: core.SpellSchoolHoly,
		ProcMask:    core.ProcMaskSpellHealing,
		Flags:       core.SpellFlagHelpful | core.SpellFlagAPL,

		ManaCost: core.ManaCostOptions{
			BaseCost: 0.27,
		},
		Cast: core.CastConfig{
			DefaultCast: core.Cast{
				GCD:      core.GCDDefault,
				CastTime: time.Millisecond * 1500,
			},
		},

		BonusCritRating: float64(priest.Talents.HolySpecialization) * 1 * core.CritRatingPerCritChance,
		DamageMultiplier: 1 *
			(1 + .02*float64(priest.Talents.SpiritualHealing)) *
			(1 + .01*float64(priest.Talents.BlessedResilience)) *
			(1 + .02*float64(priest.Talents.FocusedPower)) *
			(1 + .02*float64(priest.Talents.DivineProvidence)),
		CritMultiplier:   priest.DefaultHealingCritMultiplier(),
		ThreatMultiplier: 0.5 * (1 - []float64{0, .07, .14, .20}[priest.Talents.SilentResolve]),

		Direct: core.SpellEffect{Effect: 0, Min: 1959, Max: 2515, SP: 0.8057},
		Mods: []core.SpellMod{
			{Op: core.SpellModBonusMultiplier, Flat: 4 * int32(priest.Talents.EmpoweredHealing)},
		},

		// Server has two identical heal effects (self, target); SpellConfig has one Direct slot.
		// Never applied, just here to declare effect 1 too.
		Hot: core.DotConfig{
			Aura:          core.Aura{Label: "BindingHealSecondEffect"},
			NumberOfTicks: 1,
			TickLength:    time.Second,
			Tick:          core.SpellEffect{Effect: 1, Min: 1959, Max: 2515, SP: 0.8057},
		},

		ApplyEffects: func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
			healFromSP := spell.Direct.SP * spell.HealingPower(target)

			selfHealing := spell.Direct.Roll(sim) + healFromSP
			spell.CalcAndDealHealing(sim, &priest.Unit, selfHealing, spell.OutcomeHealingCrit)

			targetHealing := spell.Direct.Roll(sim) + healFromSP
			spell.CalcAndDealHealing(sim, target, targetHealing, spell.OutcomeHealingCrit)
		},
	})
}
