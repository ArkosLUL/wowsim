package encounters

import (
	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
	"github.com/wowsims/wotlk/sim/core/stats"
)

// GenericBossID is a synthetic id: no specific NPC carries it. The stats below are the plain level
// 83, class 1 curve creature_classlevelstats gives any creature with no boss script (Creature::
// SelectLevel, CreatureBaseStats), rather than one fight's scripted numbers.
const GenericBossID = 999999

// GenericBossDamageModifier is Patchwerk 25's (29324) creature_template.DamageModifier, the phase 1
// tank boss. Without one the curve's swing is a trash mob's, about 200 before armor.
const GenericBossDamageModifier = 70

// GenericBossTarget is the level 83, class 1 creature_classlevelstats row (SELECT level, basehp2,
// basearmor, attackpower, damage_exp2 FROM creature_classlevelstats WHERE level=83 AND class=1 on
// acore_world), the curve Creature::SelectLevel gives any creature with no boss script, hitting as
// hard as a boss with the given creature_template.DamageModifier. The tank suites' extra encounter
// and the tank specs' UI default both build their target from it.
func GenericBossTarget(damageModifier float64) *proto.Target {
	return &proto.Target{
		Id:        GenericBossID,
		Name:      "Generic Boss",
		Level:     83,
		TankIndex: 0,

		Stats: stats.Stats{
			stats.Health:      13945,
			stats.Armor:       10643,
			stats.AttackPower: 805,
		}.ToFloatArray(),

		SpellSchool: proto.SpellSchool_SpellSchoolPhysical,
		// BASE_ATTACK_TIME (Unit.h), which creature_template.BaseAttackTime falls back to at 0.
		SwingSpeed: 2.0,
		// damage_exp2, max = min * 1.5 (Creature::SelectLevel). Creature::CalculateMinMaxDamage
		// multiplies weapon damage plus AP/14 by DamageModifier, and EnemyWeaponDamage scales the AP
		// part by MinBaseDamage/177, so scaling MinBaseDamage covers both.
		MinBaseDamage: 177.074 * damageModifier,
		DamageSpread:  0.5,
		ParryHaste:    true,
	}
}

func RegisterGenericBoss() {
	core.AddPresetTarget(&core.PresetTarget{
		PathPrefix: "Generic",
		Config:     GenericBossTarget(GenericBossDamageModifier),
	})
	core.AddPresetEncounter("Generic Boss", []string{"Generic/Generic Boss"})
}
