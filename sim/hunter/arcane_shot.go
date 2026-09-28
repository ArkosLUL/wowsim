package hunter

import (
	"time"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
)

func (hunter *Hunter) registerArcaneShotSpell(timer *core.Timer) {
	hasGlyph := hunter.HasMajorGlyph(proto.HunterMajorGlyph_GlyphOfArcaneShot)
	var manaMetrics *core.ResourceMetrics
	if hasGlyph {
		manaMetrics = hunter.NewManaMetrics(core.ActionID{ItemID: 42898})
	}

	hunter.ArcaneShot = hunter.RegisterSpell(core.SpellConfig{
		ActionID:     core.ActionID{SpellID: 49045},
		SpellSchool:  core.SpellSchoolArcane,
		ProcMask:     core.ProcMaskRangedSpecial,
		Flags:        core.SpellFlagMeleeMetrics | core.SpellFlagAPL,
		MissileSpeed: 40,

		ManaCost: core.ManaCostOptions{
			BaseCost:   0.05,
			Multiplier: 1 - 0.03*float64(hunter.Talents.Efficiency),
		},
		Cast: core.CastConfig{
			DefaultCast: core.Cast{
				GCD: core.GCDDefault,
			},
			IgnoreHaste: true,
			CD: core.Cooldown{
				Timer:    timer,
				Duration: time.Second * 6,
			},
		},

		BonusCritRating: 0 +
			2*core.CritRatingPerCritChance*float64(hunter.Talents.SurvivalInstincts),
		// Improved Arcane Shot (19454-19456): damage only, no cooldown cut in 3.3.5
		DamageMultiplierAdditive: spellModDamage(
			.03*float64(hunter.Talents.FerociousInspiration),
			.05*float64(hunter.Talents.ImprovedArcaneShot),
		),
		DamageMultiplier: 1 *
			hunter.markedForDeathMultiplier(),
		CritMultiplier:   hunter.critMultiplier(true, true),
		ThreatMultiplier: 1,

		Direct: core.SpellEffect{Effect: 0, Min: 492, Max: 492, AP: 0.15},

		ApplyEffects: func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
			baseDamage := spell.Direct.Roll(sim) + spell.Direct.AP*spell.RangedAttackPowerSpellBonus(target)
			result := spell.CalcDamage(sim, target, baseDamage, spell.OutcomeRangedHitAndCrit)
			spell.WaitTravelTime(sim, func(sim *core.Simulation) {
				if hasGlyph && result.Landed() && (hunter.SerpentSting.Dot(target).IsActive() || hunter.ScorpidStingAuras.Get(target).IsActive()) {
					hunter.AddMana(sim, 0.2*hunter.ArcaneShot.DefaultCast.Cost, manaMetrics)
				}
				spell.DealDamage(sim, result)
			})
		},
	})
}
