package warrior

import (
	"time"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
)

func (warrior *Warrior) RegisterShatteringThrowCD() {
	hasGlyph := warrior.HasMinorGlyph(proto.WarriorMinorGlyph_GlyphOfShatteringThrow)
	shattDebuffs := warrior.NewEnemyAuraArray(core.ShatteringThrowAura)

	ShatteringThrowSpell := warrior.RegisterSpell(core.SpellConfig{
		ActionID:     core.ActionID{SpellID: 64382},
		SpellSchool:  core.SpellSchoolPhysical,
		ProcMask:     core.ProcMaskMeleeMHSpecial,
		Flags:        core.SpellFlagMeleeMetrics,
		MissileSpeed: 50,

		RageCost: core.RageCostOptions{
			Cost: 25 - float64(warrior.Talents.FocusedRage),
		},
		Cast: core.CastConfig{
			DefaultCast: core.Cast{
				GCD:      core.GCDDefault,
				CastTime: time.Millisecond * 1500,
			},
			CD: core.Cooldown{
				Timer:    warrior.NewTimer(),
				Duration: time.Minute * 5,
			},
			ModifyCast: func(sim *core.Simulation, spell *core.Spell, cast *core.Cast) {
				if !hasGlyph && !warrior.StanceMatches(BattleStance) && warrior.BattleStance.IsReady(sim) {
					warrior.BattleStance.Cast(sim, nil)
				}
			},
			IgnoreHaste: true,
		},
		ExtraCastCondition: func(sim *core.Simulation, target *core.Unit) bool {
			return warrior.StanceMatches(BattleStance) || warrior.BattleStance.IsReady(sim) || hasGlyph
		},

		DamageMultiplier: 1,
		CritMultiplier:   warrior.critMultiplier(mh),
		ThreatMultiplier: 1,

		Direct: core.SpellEffect{Effect: 0, Min: 12, Max: 12, AP: 0.5},

		ApplyEffects: func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
			// the damage and crit are worked out at the launch, the armor debuff lands with the missile
			baseDamage := spell.Direct.Roll(sim) + spell.Direct.AP*spell.MeleeAttackPower()
			result := spell.CalcDamage(sim, target, baseDamage, spell.OutcomeMeleeSpecialNoBlockDodgeParry)
			spell.WaitTravelTime(sim, func(sim *core.Simulation) {
				spell.DealDamage(sim, result)
				if result.Landed() {
					shattDebuffs.Get(target).Activate(sim)
				}
			})
		},

		RelatedAuras: []core.AuraArray{shattDebuffs},
	})

	warrior.AddMajorCooldown(core.MajorCooldown{
		Spell: ShatteringThrowSpell,
		Type:  core.CooldownTypeDPS,
	})
}
