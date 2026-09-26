package encounters

import (
	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
	"github.com/wowsims/wotlk/sim/core/stats"
)

// GenericBossID is a synthetic id: no specific NPC carries it. The stats below are the plain level
// 83, class 1 curve creature_classlevelstats gives any creature with no boss script (Creature::
// SelectLevel, CreatureBaseStats), rather than one fight's scripted numbers. BIS-tank-boss scales
// Damage by a phase boss's creature_template.DamageModifier to approximate that boss's real output.
const GenericBossID = 999999

// GenericBossTarget is the level 83, class 1 creature_classlevelstats row (SELECT level, basehp2,
// basearmor, attackpower, damage_exp2 FROM creature_classlevelstats WHERE level=83 AND class=1 on
// acore_world), the curve Creature::SelectLevel gives any creature with no boss script. The tank
// suites' extra encounter and the tank specs' UI default both build their target from it.
func GenericBossTarget() *proto.Target {
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
		// damage_exp2, max = min * 1.5 (Creature::SelectLevel).
		MinBaseDamage: 177.074,
		DamageSpread:  0.5,
		ParryHaste:    true,
	}
}

func RegisterGenericBoss() {
	core.AddPresetTarget(&core.PresetTarget{
		PathPrefix: "Generic",
		Config:     GenericBossTarget(),
	})
	core.AddPresetEncounter("Generic Boss", []string{"Generic/Generic Boss"})
}
