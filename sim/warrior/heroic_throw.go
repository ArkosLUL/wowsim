package warrior

import (
	"time"

	"github.com/wowsims/wotlk/sim/core"
)

func (warrior *Warrior) RegisterHeroicThrow() {
	warrior.RegisterSpell(core.SpellConfig{
		ActionID:     core.ActionID{SpellID: 57755},
		SpellSchool:  core.SpellSchoolPhysical,
		ProcMask:     core.ProcMaskMeleeMHSpecial,
		Flags:        core.SpellFlagMeleeMetrics | core.SpellFlagAPL,
		MissileSpeed: 50,

		Cast: core.CastConfig{
			DefaultCast: core.Cast{
				GCD: core.GCDDefault,
			},
			CD: core.Cooldown{
				Timer:    warrior.NewTimer(),
				Duration: time.Minute * 1,
			},
			IgnoreHaste: true,
		},
		DamageMultiplier: 1,
		CritMultiplier:   warrior.critMultiplier(mh),
		ThreatMultiplier: 1.5,

		Direct: core.SpellEffect{Effect: 0, Min: 12, Max: 12, AP: 0.5},

		ApplyEffects: func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
			// the damage and crit are worked out at the launch, and land with the missile
			baseDamage := spell.Direct.Roll(sim) + spell.Direct.AP*spell.MeleeAttackPower()
			result := spell.CalcDamage(sim, target, baseDamage, spell.OutcomeMeleeSpecialNoBlockDodgeParry)
			spell.DealDamageAfterTravel(sim, result)
		},
	})
}
