package warlock

import (
	"time"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
)

func (warlock *Warlock) registerChaosBoltSpell() {
	if !warlock.Talents.ChaosBolt {
		return
	}

	// ChaosBolt is affected by level-based partial resists.
	// TODO If there's bosses with elevated fire resistances, we'd need another spell flag,
	//  or add an unlimited amount of "bonusSpellPenetration".
	warlock.ChaosBolt = warlock.RegisterSpell(core.SpellConfig{
		ActionID:     core.ActionID{SpellID: 59172},
		SpellSchool:  core.SpellSchoolFire,
		ProcMask:     core.ProcMaskSpellDamage,
		Flags:        core.SpellFlagAPL,
		MissileSpeed: 20,

		ManaCost: core.ManaCostOptions{
			BaseCost:   0.07,
			Multiplier: 1 - []float64{0, .04, .07, .10}[warlock.Talents.Cataclysm],
		},
		Cast: core.CastConfig{
			DefaultCast: core.Cast{
				GCD:      core.GCDDefault,
				CastTime: time.Millisecond*2500 - (time.Millisecond * 100 * time.Duration(warlock.Talents.Bane)),
			},
			CD: core.Cooldown{
				Timer:    warlock.NewTimer(),
				Duration: time.Second * (12 - 2*core.TernaryDuration(warlock.HasMajorGlyph(proto.WarlockMajorGlyph_GlyphOfChaosBolt), 1, 0)),
			},
		},

		BonusCritRating: 0 +
			core.TernaryFloat64(warlock.Talents.Devastation, 1, 0)*5*core.CritRatingPerCritChance,
		DamageMultiplier: spellModDamage(
			warlock.GrandFirestoneBonus(),
			0.03*float64(warlock.Talents.Emberstorm),
		),
		CritMultiplier:   warlock.SpellCritMultiplier(1, float64(warlock.Talents.Ruin)/5),
		ThreatMultiplier: 1 - 0.1*float64(warlock.Talents.DestructiveReach),

		Direct: core.SpellEffect{Effect: 0, Min: 1429, Max: 1813, SP: 0.714},
		Mods: []core.SpellMod{
			{Op: core.SpellModBonusMultiplier, Pct: 4 * warlock.Talents.ShadowAndFlame},
		},

		ApplyEffects: func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
			baseDamage := spell.Direct.Roll(sim) + spell.Direct.SP*spell.SpellPower()
			result := spell.CalcDamage(sim, target, baseDamage, spell.OutcomeMagicCrit)
			spell.DealDamageAfterTravel(sim, result)
		},
	})
}
