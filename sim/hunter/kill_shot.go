package hunter

import (
	"time"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
)

func (hunter *Hunter) registerKillShotSpell() {
	hunter.KillShot = hunter.RegisterSpell(core.SpellConfig{
		ActionID:     core.ActionID{SpellID: 61006},
		SpellSchool:  core.SpellSchoolPhysical,
		ProcMask:     core.ProcMaskRangedSpecial,
		Flags:        core.SpellFlagMeleeMetrics | core.SpellFlagIncludeTargetBonusDamage | core.SpellFlagAPL,
		MissileSpeed: 40,

		ManaCost: core.ManaCostOptions{
			BaseCost:   0.07,
			Multiplier: 1 - 0.03*float64(hunter.Talents.Efficiency),
		},
		Cast: core.CastConfig{
			DefaultCast: core.Cast{
				GCD: core.GCDDefault,
			},
			IgnoreHaste: true,
			CD: core.Cooldown{
				Timer:    hunter.NewTimer(),
				Duration: time.Second*15 - core.TernaryDuration(hunter.HasMajorGlyph(proto.HunterMajorGlyph_GlyphOfKillShot), time.Second*6, 0),
			},
		},
		ExtraCastCondition: func(sim *core.Simulation, target *core.Unit) bool {
			return sim.IsExecutePhase20()
		},

		BonusCritRating: 0 +
			5*core.CritRatingPerCritChance*float64(hunter.Talents.SniperTraining),
		DamageMultiplier: 1 *
			hunter.markedForDeathMultiplier(),
		CritMultiplier:   hunter.critMultiplier(true, true),
		ThreatMultiplier: 1,

		// Spell::EffectWeaponDmg adds the AP after the weapon percent
		Direct: core.SpellEffect{Effect: 0, Min: 325, Max: 325, AP: 0.4, WeaponPct: 2},

		ApplyEffects: func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
			// effect 17 isn't normalized, so the weapon's AP part is RAP / 14 × the weapon's own speed
			baseDamage := (spell.Direct.Roll(sim) + hunter.RangedWeaponDamage(sim, hunter.rangedWeaponAP()) +
				hunter.AmmoDamageBonus +
				spell.BonusWeaponDamage()) * spell.Direct.WeaponPct
			// Spell::EffectWeaponDmg's own 0.4 reads GetTotalAttackPowerValue with no victim, so no Hunter's Mark
			baseDamage += spell.Direct.AP * hunter.rangedWeaponAP()
			result := spell.CalcDamage(sim, target, baseDamage, spell.OutcomeRangedHitAndCrit)
			spell.DealDamageAfterTravel(sim, result)
		},
	})
}
