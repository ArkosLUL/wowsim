package mage

import (
	"time"

	"github.com/wowsims/wotlk/sim/core"
)

func (mage *Mage) registerFlamestrikeSpell(rank8 bool) *core.Spell {
	actionID := core.ActionID{SpellID: 42926}.WithTag(9)
	direct := core.SpellEffect{Effect: 0, Min: 876, Max: 1070, SP: 0.2357}
	tick := core.SpellEffect{Effect: 1, Min: 195, Max: 195, SP: 0.122}
	spCoeffMultiplier := 1.0
	label := "Flamestrike (Rank 9)"
	if rank8 {
		actionID = core.ActionID{SpellID: 42925}.WithTag(8)
		direct.Min, direct.Max = 699, 853
		tick.Min, tick.Max = 155, 155
		label = "Flamestrike (Rank 8)"
		// downranking penalty on the spell power part (Unit::CalculateLevelPenalty): rank 8 is a level 72
		// spell, so (72 + 6) / 80
		spCoeffMultiplier = 0.975
	}

	return mage.RegisterSpell(core.SpellConfig{
		ActionID:    actionID,
		SpellSchool: core.SpellSchoolFire,
		ProcMask:    core.ProcMaskSpellDamage,
		Flags:       SpellFlagMage | core.SpellFlagAPL,

		ManaCost: core.ManaCostOptions{
			BaseCost: 0.30,
		},
		Cast: core.CastConfig{
			DefaultCast: core.Cast{
				GCD:      core.GCDDefault,
				CastTime: time.Second * 2,
			},
		},

		BonusCritRating: float64(mage.Talents.CriticalMass+mage.Talents.WorldInFlames) * 2 * core.CritRatingPerCritChance,
		DamageMultiplierAdditive: spellModDamage(
			.02*float64(mage.Talents.SpellImpact),
			.02*float64(mage.Talents.FirePower),
		),
		CritMultiplier:   mage.SpellCritMultiplier(1, mage.bonusCritDamage),
		ThreatMultiplier: 1 - 0.05*float64(mage.Talents.BurningSoul),

		Direct: direct,

		Dot: core.DotConfig{
			IsAOE: true,
			Aura: core.Aura{
				Label: label,
			},
			NumberOfTicks: 4,
			TickLength:    time.Second * 2,
			// No SPELL_AURA_ABILITY_PERIODIC_CRIT aura covers this dot, so its ticks never crit.
			TicksCanCrit: false,

			Tick: tick,

			OnSnapshot: func(sim *core.Simulation, _ *core.Unit, dot *core.Dot, _ bool) {
				target := mage.CurrentTarget
				dot.SnapshotBaseDamage = dot.Tick.Roll(sim) + dot.Tick.SP*dot.Spell.SpellPower()*spCoeffMultiplier
				dot.SnapshotAttackerMultiplier = dot.Spell.AttackerDamageMultiplier(dot.Spell.Unit.AttackTables[target.UnitIndex])
			},
			OnTick: func(sim *core.Simulation, target *core.Unit, dot *core.Dot) {
				for _, aoeTarget := range sim.Encounter.TargetUnits {
					dot.CalcAndDealPeriodicSnapshotDamage(sim, aoeTarget, dot.OutcomeTick)
				}
			},
		},

		ApplyEffects: func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
			dmgFromSP := spell.Direct.SP * spell.SpellPower() * spCoeffMultiplier
			for _, aoeTarget := range sim.Encounter.TargetUnits {
				baseDamage := spell.Direct.Roll(sim) + dmgFromSP
				baseDamage *= sim.Encounter.AOECapMultiplier()
				spell.CalcAndDealDamage(sim, aoeTarget, baseDamage, spell.OutcomeMagicHitAndCrit)
			}
			spell.AOEDot().Apply(sim)
		},
	})
}

func (mage *Mage) registerFlamestrikeSpells() {
	mage.Flamestrike = mage.registerFlamestrikeSpell(false)
	mage.FlamestrikeRank8 = mage.registerFlamestrikeSpell(true)
}
