package paladin

import (
	"time"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
)

func (paladin *Paladin) registerAvengersShieldSpell() {
	glyphedSingleTargetAS := paladin.HasMajorGlyph(proto.PaladinMajorGlyph_GlyphOfAvengerSShield)
	// Glyph to single target, OR apply to up to 3 targets
	numHits := core.TernaryInt32(glyphedSingleTargetAS, 1, min(3, paladin.Env.GetNumTargets()))
	results := make([]*core.SpellResult, numHits)

	paladin.AvengersShield = paladin.RegisterSpell(core.SpellConfig{
		ActionID:     core.ActionID{SpellID: 48827},
		SpellSchool:  core.SpellSchoolHoly,
		ProcMask:     core.ProcMaskMeleeMHSpecial,
		Flags:        core.SpellFlagMeleeMetrics | core.SpellFlagAPL,
		MissileSpeed: 35,

		ManaCost: core.ManaCostOptions{
			BaseCost:   0.26,
			Multiplier: 1 - 0.02*float64(paladin.Talents.Benediction),
		},
		Cast: core.CastConfig{
			DefaultCast: core.Cast{
				GCD: core.GCDDefault,
			},
			IgnoreHaste: true,
			CD: core.Cooldown{
				Timer:    paladin.NewTimer(),
				Duration: time.Second * 30,
			},
		},

		DamageMultiplier: core.TernaryFloat64(glyphedSingleTargetAS, 2, 1),
		CritMultiplier:   paladin.MeleeCritMultiplier(),
		ThreatMultiplier: 1,

		Direct: core.SpellEffect{Effect: 0, Min: 1100, Max: 1344, SP: 0.07, AP: 0.07},

		ApplyEffects: func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
			constBaseDamage := spell.Direct.SP*spell.SpellPower() + spell.Direct.AP*spell.MeleeAttackPower()

			// every jump is worked out at the launch and flies from the caster
			curTarget := target
			for hitIndex := int32(0); hitIndex < numHits; hitIndex++ {
				baseDamage := constBaseDamage + spell.Direct.Roll(sim)

				results[hitIndex] = spell.CalcDamage(sim, curTarget, baseDamage, spell.OutcomeMeleeSpecialHitAndCrit)
				curTarget = sim.Environment.NextTargetUnit(curTarget)
			}

			spell.WaitTravelTime(sim, func(sim *core.Simulation) {
				for _, result := range results {
					spell.DealDamage(sim, result)
				}
			})
		},
	})
}
