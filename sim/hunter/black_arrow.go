package hunter

import (
	"time"

	"github.com/wowsims/wotlk/sim/core"
)

func (hunter *Hunter) registerBlackArrowSpell(timer *core.Timer) {
	if !hunter.Talents.BlackArrow {
		return
	}

	actionID := core.ActionID{SpellID: 63672}

	hunter.BlackArrow = hunter.RegisterSpell(core.SpellConfig{
		ActionID:    actionID,
		SpellSchool: core.SpellSchoolShadow,
		ProcMask:    core.ProcMaskRangedSpecial,
		Flags:       core.SpellFlagAPL,

		ManaCost: core.ManaCostOptions{
			BaseCost: 0.06,
			Multiplier: 1 -
				0.03*float64(hunter.Talents.Efficiency) -
				0.2*float64(hunter.Talents.Resourcefulness),
		},
		Cast: core.CastConfig{
			DefaultCast: core.Cast{
				GCD: core.GCDDefault,
			},
			IgnoreHaste: true, // Hunter GCD is locked at 1.5s
			CD: core.Cooldown{
				Timer:    timer,
				Duration: time.Second*30 - time.Second*2*time.Duration(hunter.Talents.Resourcefulness),
			},
		},

		DamageMultiplierAdditive: 1 +
			.10*float64(hunter.Talents.TrapMastery) +
			.02*float64(hunter.Talents.TNT),
		DamageMultiplier: 1 *
			(1.0 / 1.06), // Black Arrow is not affected by its own 1.06 aura.
		ThreatMultiplier: 1,

		Dot: core.DotConfig{
			Aura: core.Aura{
				Label: "BlackArrow",
				OnGain: func(aura *core.Aura, sim *core.Simulation) {
					hunter.AttackTables[aura.Unit.UnitIndex].DamageTakenMultiplier *= 1.06
				},
				OnExpire: func(aura *core.Aura, sim *core.Simulation) {
					hunter.AttackTables[aura.Unit.UnitIndex].DamageTakenMultiplier /= 1.06
				},
			},
			NumberOfTicks: 5,
			TickLength:    time.Second * 3,
			OnSnapshot: func(sim *core.Simulation, target *core.Unit, dot *core.Dot, isRollover bool) {
				// spell_bonus_data's ap_dot_bonus: 0.02 a tick, on ranged AP with the target's Hunter's Mark
				dot.SnapshotBaseDamage = 553 + 0.02*dot.Spell.RangedAttackPower(target)
				dot.SnapshotAttackerMultiplier = dot.Spell.AttackerDamageMultiplier(dot.Spell.Unit.AttackTables[target.UnitIndex])
			},
			OnTick: func(sim *core.Simulation, target *core.Unit, dot *core.Dot) {
				dot.CalcAndDealPeriodicSnapshotDamage(sim, target, dot.OutcomeTick)
			},
		},

		ApplyEffects: func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
			result := spell.CalcOutcome(sim, target, spell.OutcomeRangedHit)
			if result.Landed() {
				spell.Dot(target).Apply(sim)
			}
			spell.DealOutcome(sim, result)
		},
	})
}
