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

		DamageMultiplier: spellModDamage(paladin.getTalentSanctityOfBattleBonus(), paladin.getMajorGlyphOfExorcismBonus(),
			paladin.getItemSetAegisBattlegearBonus2()),
		ThreatMultiplier: 1,
		CritMultiplier:   paladin.SpellCritMultiplier(),

		Direct: core.SpellEffect{Effect: 0, Min: 1033, Max: 1151, SP: 0.15, AP: 0.15},

		ApplyEffects: func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
			baseDamage := spell.Direct.Roll(sim) +
				spell.Direct.SP*spell.SpellPower() +
				spell.Direct.AP*spell.MeleeAttackPower()

			// hits any creature type (no TargetCreatureType in Spell.dbc), but undead and demons always
			// take a crit (Unit::SpellTakenCritChance)
			bonusCrit := core.TernaryFloat64(
				target.MobType == proto.MobType_MobTypeDemon || target.MobType == proto.MobType_MobTypeUndead,
				100*core.CritRatingPerCritChance,
				0)
			spell.BonusCritRating += bonusCrit
			spell.CalcAndDealDamage(sim, target, baseDamage, spell.OutcomeMagicHitAndCrit)
			spell.BonusCritRating -= bonusCrit
		},
	})
}
