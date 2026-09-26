package druid

import (
	"time"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
)

func (druid *Druid) registerStarfireSpell() {
	// the idols add spell power (override class script 5148), so the coefficient scales them,
	// Wrath of Cenarius included
	idolSpellPower := 0 +
		core.TernaryFloat64(druid.Ranged().ID == 27518, 55, 0) + // Ivory Moongoddess
		core.TernaryFloat64(druid.Ranged().ID == 40321, 165, 0) // Shooting Star

	hasGlyph := druid.HasMajorGlyph(proto.DruidMajorGlyph_GlyphOfStarfire)

	starfireGlyphSpell := druid.RegisterSpell(Humanoid|Moonkin, core.SpellConfig{
		ActionID: core.ActionID{SpellID: 54845},
		ProcMask: core.ProcMaskSuppressedProc,
		Flags:    core.SpellFlagNoLogs,
		ApplyEffects: func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
			moonfireDot := druid.Moonfire.Dot(target)

			if moonfireDot.IsActive() && druid.ExtendingMoonfireStacks > 0 {
				druid.ExtendingMoonfireStacks -= 1
				moonfireDot.UpdateExpires(moonfireDot.ExpiresAt() + time.Second*3)
			}
		},
	})

	druid.Starfire = druid.RegisterSpell(Humanoid|Moonkin, core.SpellConfig{
		ActionID:    core.ActionID{SpellID: 48465},
		SpellSchool: core.SpellSchoolArcane,
		ProcMask:    core.ProcMaskSpellDamage,
		Flags:       SpellFlagNaturesGrace | SpellFlagOmenTrigger | core.SpellFlagAPL,

		ManaCost: core.ManaCostOptions{
			BaseCost:   0.16,
			Multiplier: 1 - 0.03*float64(druid.Talents.Moonglow),
		},
		Cast: core.CastConfig{
			DefaultCast: core.Cast{
				GCD:      core.GCDDefault,
				CastTime: druid.starfireCastTime(),
			},
		},

		BonusCritRating: 0 +
			2*float64(druid.Talents.NaturesMajesty)*core.CritRatingPerCritChance +
			core.TernaryFloat64(druid.HasSetBonus(ItemSetThunderheartRegalia, 4), 5*core.CritRatingPerCritChance, 0) +
			core.TernaryFloat64(druid.HasSetBonus(ItemSetDreamwalkerGarb, 4), 5*core.CritRatingPerCritChance, 0),
		DamageMultiplier: spellModDamage(
			[]float64{0.0, 0.03, 0.06, 0.1}[druid.Talents.Moonfury],
			core.TernaryFloat64(druid.HasSetBonus(ItemSetMalfurionsRegalia, 4), 0.04, 0),
		),
		CritMultiplier:   druid.BalanceCritMultiplier(),
		ThreatMultiplier: 1,

		Direct: core.SpellEffect{Effect: 0, Min: 1038, Max: 1222, SP: 1},
		Mods: []core.SpellMod{
			{Op: core.SpellModBonusMultiplier, Flat: 4 * druid.Talents.WrathOfCenarius},
		},

		ApplyEffects: func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
			baseDamage := spell.Direct.Roll(sim) + spell.Direct.SP*(spell.SpellPower()+idolSpellPower)
			result := spell.CalcDamage(sim, target, baseDamage, spell.OutcomeMagicHitAndCrit)
			if result.Landed() && hasGlyph {
				starfireGlyphSpell.Cast(sim, target)
			}
			spell.DealDamage(sim, result)
		},
	})
}

func (druid *Druid) starfireCastTime() time.Duration {
	return time.Millisecond*3500 - time.Millisecond*100*time.Duration(druid.Talents.StarlightWrath)
}
