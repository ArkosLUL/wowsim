package warrior

import (
	"math"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
)

func (warrior *Warrior) registerExecuteSpell() {
	const maxRage = 30
	// spell_warr_execute: the effect's DamageMultiplier, 3.8, per tenth of a rage point
	const damagePerRage = 38

	var extraRageBonus float64
	if warrior.HasMajorGlyph(proto.WarriorMajorGlyph_GlyphOfExecution) {
		extraRageBonus = 10
	}

	var rageMetrics *core.ResourceMetrics
	warrior.Execute = warrior.RegisterSpell(core.SpellConfig{
		ActionID:    core.ActionID{SpellID: 47471},
		SpellSchool: core.SpellSchoolPhysical,
		ProcMask:    core.ProcMaskMeleeMHSpecial,
		Flags:       core.SpellFlagMeleeMetrics | core.SpellFlagIncludeTargetBonusDamage | core.SpellFlagAPL,

		RageCost: core.RageCostOptions{
			Cost: 15 -
				float64(warrior.Talents.FocusedRage) -
				[]float64{0, 2, 5}[warrior.Talents.ImprovedExecute] -
				core.TernaryFloat64(warrior.HasSetBonus(ItemSetOnslaughtBattlegear, 2), 3, 0),
			Refund: 0.8,
		},
		Cast: core.CastConfig{
			DefaultCast: core.Cast{
				GCD: core.GCDDefault,
			},
			IgnoreHaste: true,
		},
		ExtraCastCondition: func(sim *core.Simulation, target *core.Unit) bool {
			return sim.IsExecutePhase20() || warrior.IsSuddenDeathActive()
		},

		DamageMultiplier: 1,
		CritMultiplier:   warrior.critMultiplier(mh),
		ThreatMultiplier: 1.25,

		// spell_warr_execute hands 20647 the dummy effect's value plus 20% of attack power and the extra
		// rage's damage, in whole points
		Direct: core.SpellEffect{Effect: 0, Min: 1456, Max: 1456, SP: 1, AP: 0.2},

		ApplyEffects: func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
			extraRage := spell.Unit.CurrentRage()
			if extraRage > maxRage-spell.CurCast.Cost {
				extraRage = maxRage - spell.CurCast.Cost
			}
			warrior.SpendRage(sim, extraRage, rageMetrics)
			rageMetrics.Events--

			baseDamage := spell.Direct.Roll(sim) +
				math.Floor(damagePerRage*(extraRage+extraRageBonus)+spell.Direct.AP*spell.MeleeAttackPower())
			result := spell.CalcAndDealDamage(sim, target, baseDamage, spell.OutcomeMeleeSpecialHitAndCrit)

			if !result.Landed() {
				spell.IssueRefund(sim)
			}
		},
	})
	rageMetrics = warrior.Execute.Cost.(*core.RageCost).ResourceMetrics
}
