package paladin

import (
	"time"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
)

func (paladin *Paladin) registerExorcismSpell() {
	paladin.Exorcism = paladin.RegisterSpell(core.SpellConfig{
		ActionID:    core.ActionID{SpellID: 48801},
		SpellSchool: core.SpellSchoolHoly,
		ProcMask:    core.ProcMaskSpellDamage,
		Flags:       core.SpellFlagMeleeMetrics | core.SpellFlagAPL,

		ManaCost: core.ManaCostOptions{
			BaseCost:   0.08,
			Multiplier: 1 - 0.02*float64(paladin.Talents.Benediction),
		},
		Cast: core.CastConfig{
			DefaultCast: core.Cast{
				GCD:      core.GCDDefault,
				CastTime: time.Millisecond * 1500,
			},
			CD: core.Cooldown{
				Timer:    paladin.NewTimer(),
				Duration: time.Second * 15,
			},
		},
		ExtraCastCondition: func(sim *core.Simulation, target *core.Unit) bool {
			return target.MobType == proto.MobType_MobTypeDemon || target.MobType == proto.MobType_MobTypeUndead
		},

		DamageMultiplierAdditive: 1 +
			paladin.getTalentSanctityOfBattleBonus() +
			paladin.getMajorGlyphOfExorcismBonus() +
			paladin.getItemSetAegisBattlegearBonus2(),
		DamageMultiplier: 1,
		ThreatMultiplier: 1,
		CritMultiplier:   paladin.SpellCritMultiplier(),

		ApplyEffects: func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
			baseDamage := sim.Roll(1028, 1146) +
				.15*spell.SpellPower() +
				.15*spell.MeleeAttackPower()

			// always crits against undead and demons, the only targets ExtraCastCondition lets through
			bonusCrit := 100 * core.CritRatingPerCritChance
			spell.BonusCritRating += bonusCrit
			spell.CalcAndDealDamage(sim, target, baseDamage, spell.OutcomeMagicHitAndCrit)
			spell.BonusCritRating -= bonusCrit
		},
	})
}
