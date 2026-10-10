package mage

import (
	"time"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
)

func (mage *Mage) registerArcaneBarrageSpell() {
	if !mage.Talents.ArcaneBarrage {
		return
	}

	mage.ArcaneBarrage = mage.RegisterSpell(core.SpellConfig{
		ActionID:     core.ActionID{SpellID: 44781},
		SpellSchool:  core.SpellSchoolArcane,
		ProcMask:     core.ProcMaskSpellDamage,
		Flags:        SpellFlagMage | BarrageSpells | core.SpellFlagAPL,
		MissileSpeed: 24,

		ManaCost: core.ManaCostOptions{
			BaseCost:   0.18,
			Multiplier: 1 - core.TernaryFloat64(mage.HasMajorGlyph(proto.MageMajorGlyph_GlyphOfArcaneBarrage), .2, 0),
		},
		Cast: core.CastConfig{
			DefaultCast: core.Cast{
				GCD: core.GCDDefault,
			},
			CD: core.Cooldown{
				Timer:    mage.NewTimer(),
				Duration: time.Second * 3,
			},
		},

		BonusHitRating:   float64(mage.Talents.ArcaneFocus) * core.SpellHitRatingPerHitChance,
		DamageMultiplier: 1,
		CritMultiplier:   mage.SpellCritMultiplier(1, mage.bonusCritDamage),
		ThreatMultiplier: 1,

		Direct: core.SpellEffect{Effect: 0, Min: 936, Max: 1144, SP: 0.714},

		ApplyEffects: func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
			baseDamage := (spell.Direct.Roll(sim) + core.PowerBonus(spell.Direct.SP*spell.SpellPower())) * mage.tormentTheWeak(target)
			result := spell.CalcDamage(sim, target, baseDamage, spell.OutcomeMagicHitAndCrit)
			// spell_proc 36032 takes the Arcane Blast stacks off at the cast, once the launch has the damage
			mage.ArcaneBlastAura.Deactivate(sim)

			spell.DealDamageAfterTravel(sim, result)
		},
	})
}
