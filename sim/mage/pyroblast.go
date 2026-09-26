package mage

import (
	"time"

	"github.com/wowsims/wotlk/sim/core"
)

func (mage *Mage) registerPyroblastSpell() {
	if !mage.Talents.Pyroblast {
		return
	}

	var pyroblastDot *core.Spell

	pyroblastConfig := core.SpellConfig{
		ActionID:     core.ActionID{SpellID: 42891},
		SpellSchool:  core.SpellSchoolFire,
		ProcMask:     core.ProcMaskSpellDamage,
		Flags:        SpellFlagMage | core.SpellFlagAPL,
		MissileSpeed: 24,

		ManaCost: core.ManaCostOptions{
			BaseCost: 0.22,
		},
		Cast: core.CastConfig{
			DefaultCast: core.Cast{
				GCD:      core.GCDDefault,
				CastTime: time.Second * 5,
			},
			ModifyCast: func(sim *core.Simulation, spell *core.Spell, cast *core.Cast) {
				if mage.HotStreakAura.IsActive() {
					cast.CastTime = 0
					mage.HotStreakAura.Deactivate(sim)
				}
			},
		},

		BonusCritRating: 0 +
			2*float64(mage.Talents.CriticalMass)*core.CritRatingPerCritChance +
			2*float64(mage.Talents.WorldInFlames)*core.CritRatingPerCritChance,
		DamageMultiplier: 1 *
			(1 + .04*float64(mage.Talents.TormentTheWeak)),
		DamageMultiplierAdditive: 1 +
			.02*float64(mage.Talents.FirePower),
		CritMultiplier:   mage.SpellCritMultiplier(1, mage.bonusCritDamage),
		ThreatMultiplier: 1 - 0.1*float64(mage.Talents.BurningSoul),

		Direct: core.SpellEffect{Effect: 0, Min: 1210, Max: 1530, SP: 1.15},
		Mods: []core.SpellMod{
			{Op: core.SpellModBonusMultiplier, Flat: 5 * mage.Talents.EmpoweredFire},
		},

		Dot: core.DotConfig{
			Aura: core.Aura{
				Label: "Pyroblast",
			},
			NumberOfTicks: 4,
			TickLength:    time.Second * 3,
			// No SPELL_AURA_ABILITY_PERIODIC_CRIT aura covers this dot, so its ticks never crit.
			TicksCanCrit: false,

			Tick: core.SpellEffect{Effect: 1, Min: 113, Max: 113, SP: 0.05},

			OnSnapshot: func(sim *core.Simulation, target *core.Unit, dot *core.Dot, _ bool) {
				dot.SnapshotBaseDamage = dot.Tick.Roll(sim) + dot.Tick.SP*dot.Spell.SpellPower()
				dot.SnapshotAttackerMultiplier = dot.Spell.AttackerDamageMultiplier(dot.Spell.Unit.AttackTables[target.UnitIndex])
			},
			OnTick: func(sim *core.Simulation, target *core.Unit, dot *core.Dot) {
				dot.CalcAndDealPeriodicSnapshotDamage(sim, target, dot.OutcomeTick)
			},
		},

		ApplyEffects: func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
			baseDamage := spell.Direct.Roll(sim) + spell.Direct.SP*spell.SpellPower()
			result := spell.CalcDamage(sim, target, baseDamage, spell.OutcomeMagicHitAndCrit)
			spell.WaitTravelTime(sim, func(sim *core.Simulation) {
				if result.Landed() {
					pyroblastDot.SpellMetrics[target.UnitIndex].Casts++
					pyroblastDot.Dot(target).Apply(sim)
				}
				spell.DealDamage(sim, result)
			})
		},
	}

	mage.Pyroblast = mage.RegisterSpell(pyroblastConfig)

	dotConfig := pyroblastConfig
	dotConfig.ActionID = dotConfig.ActionID.WithTag(1)
	pyroblastDot = mage.RegisterSpell(dotConfig)
}
