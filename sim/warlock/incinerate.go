package warlock

import (
	"time"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
)

func (warlock *Warlock) registerIncinerateSpell() {
	warlock.Incinerate = warlock.RegisterSpell(core.SpellConfig{
		ActionID:     core.ActionID{SpellID: 47838},
		SpellSchool:  core.SpellSchoolFire,
		ProcMask:     core.ProcMaskSpellDamage,
		Flags:        core.SpellFlagAPL,
		MissileSpeed: 24,

		ManaCost: core.ManaCostOptions{
			BaseCost:   0.14,
			Multiplier: 1 - []float64{0, .04, .07, .10}[warlock.Talents.Cataclysm],
		},
		Cast: core.CastConfig{
			DefaultCast: core.Cast{
				GCD:      core.GCDDefault,
				CastTime: time.Millisecond * time.Duration(2500-50*warlock.Talents.Emberstorm),
			},
		},

		BonusCritRating: 0 +
			core.TernaryFloat64(warlock.Talents.Devastation, 5*core.CritRatingPerCritChance, 0) +
			core.TernaryFloat64(warlock.HasSetBonus(ItemSetDeathbringerGarb, 4), 5*core.CritRatingPerCritChance, 0) +
			core.TernaryFloat64(warlock.HasSetBonus(ItemSetDarkCovensRegalia, 2), 5*core.CritRatingPerCritChance, 0),
		DamageMultiplier: spellModDamage(
			warlock.GrandFirestoneBonus(),
			0.03*float64(warlock.Talents.Emberstorm),
			core.TernaryFloat64(warlock.HasMajorGlyph(proto.WarlockMajorGlyph_GlyphOfIncinerate), 0.05, 0),
			core.TernaryFloat64(warlock.HasSetBonus(ItemSetMaleficRaiment, 4), 0.06, 0),
		),
		CritMultiplier:   warlock.SpellCritMultiplier(1, float64(warlock.Talents.Ruin)/5),
		ThreatMultiplier: 1 - 0.1*float64(warlock.Talents.DestructiveReach),

		Direct: core.SpellEffect{Effect: 0, Min: 582, Max: 676, SP: 0.714},
		Mods: []core.SpellMod{
			{Op: core.SpellModBonusMultiplier, Pct: 4 * warlock.Talents.ShadowAndFlame},
		},

		ApplyEffects: func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
			baseDamage := spell.Direct.Roll(sim)
			// Spell::EffectSchoolDMG adds a quarter of the roll, before spell power, while Immolate is up
			if warlock.Immolate.Dot(target).IsActive() {
				baseDamage += baseDamage / 4
			}
			baseDamage += spell.Direct.SP * spell.SpellPower()

			result := spell.CalcDamage(sim, target, baseDamage, spell.OutcomeMagicHitAndCrit)
			spell.WaitTravelTime(sim, func(sim *core.Simulation) {
				spell.DealDamage(sim, result)
			})
		},
	})
}
