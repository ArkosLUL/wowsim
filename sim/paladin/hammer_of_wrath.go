package paladin

import (
	"time"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
)

func (paladin *Paladin) registerHammerOfWrathSpell() {
	paladin.HammerOfWrath = paladin.RegisterSpell(core.SpellConfig{
		ActionID:     core.ActionID{SpellID: 48806},
		SpellSchool:  core.SpellSchoolHoly,
		ProcMask:     core.ProcMaskMeleeMHSpecial,
		Flags:        core.SpellFlagMeleeMetrics | core.SpellFlagAPL,
		MissileSpeed: 50,

		ManaCost: core.ManaCostOptions{
			BaseCost:   0.12 * core.TernaryFloat64(paladin.HasMajorGlyph(proto.PaladinMajorGlyph_GlyphOfHammerOfWrath), 0, 1),
			Multiplier: 1 - 0.02*float64(paladin.Talents.Benediction),
		},
		Cast: core.CastConfig{
			DefaultCast: core.Cast{
				GCD: core.GCDDefault,
			},
			IgnoreHaste: true,
			CD: core.Cooldown{
				Timer:    paladin.NewTimer(),
				Duration: time.Second * 6,
			},
		},
		ExtraCastCondition: func(sim *core.Simulation, target *core.Unit) bool {
			return sim.IsExecutePhase20()
		},

		BonusCritRating:  25 * float64(paladin.Talents.SanctifiedWrath) * core.CritRatingPerCritChance,
		DamageMultiplier: spellModDamage(paladin.getItemSetLightbringerBattlegearBonus4(), paladin.getItemSetAegisBattlegearBonus2()),
		CritMultiplier:   paladin.MeleeCritMultiplier(),
		ThreatMultiplier: 1,

		Direct: core.SpellEffect{Effect: 0, Min: 1139, Max: 1257, SP: 0.15, AP: 0.15},

		ApplyEffects: func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
			// the damage and crit are worked out at the launch, and land with the missile
			baseDamage := spell.Direct.Roll(sim) +
				spell.Direct.SP*spell.SpellPower() +
				spell.Direct.AP*spell.MeleeAttackPower()

			result := spell.CalcDamage(sim, target, baseDamage, spell.OutcomeMeleeSpecialNoBlockDodgeParry)
			spell.DealDamageAfterTravel(sim, result)
		},
	})
}
