package hunter

import (
	"time"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
)

func (hunter *Hunter) registerAimedShotSpell(timer *core.Timer) {
	if !hunter.Talents.AimedShot {
		return
	}

	hunter.AimedShot = hunter.RegisterSpell(core.SpellConfig{
		ActionID:     core.ActionID{SpellID: 49050},
		SpellSchool:  core.SpellSchoolPhysical,
		ProcMask:     core.ProcMaskRangedSpecial,
		Flags:        core.SpellFlagMeleeMetrics | core.SpellFlagIncludeTargetBonusDamage | core.SpellFlagAPL,
		MissileSpeed: 40,

		ManaCost: core.ManaCostOptions{
			BaseCost: 0.08,
			Multiplier: 1 -
				0.03*float64(hunter.Talents.Efficiency) -
				0.05*float64(hunter.Talents.MasterMarksman),
		},
		Cast: core.CastConfig{
			DefaultCast: core.Cast{
				GCD: core.GCDDefault,
			},
			IgnoreHaste: true,
			CD: core.Cooldown{
				Timer:    timer,
				Duration: time.Second*10 - core.TernaryDuration(hunter.HasMajorGlyph(proto.HunterMajorGlyph_GlyphOfAimedShot), time.Second*2, 0),
			},
		},

		BonusCritRating: 0 +
			4*core.CritRatingPerCritChance*float64(hunter.Talents.ImprovedBarrage) +
			core.TernaryFloat64(hunter.Talents.TrueshotAura && hunter.HasMajorGlyph(proto.HunterMajorGlyph_GlyphOfTrueshotAura), 10*core.CritRatingPerCritChance, 0),
		DamageMultiplierAdditive: 1 +
			.04*float64(hunter.Talents.Barrage),
		DamageMultiplier: 1 *
			hunter.markedForDeathMultiplier(),
		CritMultiplier:   hunter.critMultiplier(true, true),
		ThreatMultiplier: 1,

		Direct: core.SpellEffect{Effect: 0, Min: 408, Max: 408, WeaponPct: 1},

		ApplyEffects: func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
			// the weapon's AP part, normalized: RAP / 14 × 2.8 s
			baseDamage := (spell.Direct.Roll(sim) + 0.2*spell.RangedAttackPower(target) +
				hunter.AutoAttacks.Ranged().BaseDamage(sim) +
				hunter.NormalizedAmmoDamageBonus +
				spell.BonusWeaponDamage()) * spell.Direct.WeaponPct
			result := spell.CalcDamage(sim, target, baseDamage, spell.OutcomeRangedHitAndCrit)
			spell.WaitTravelTime(sim, func(sim *core.Simulation) {
				spell.DealDamage(sim, result)
			})
		},
	})
}
