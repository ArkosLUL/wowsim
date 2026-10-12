package encounters

import (
	"reflect"
	"testing"

	"github.com/wowsims/wotlk/sim/core/proto"
)

func TestOptimizerTankBossAbilitiesRoundTrip(t *testing.T) {
	boss := OptimizerTankBoss{
		Name: "Test", DamageModifier: 70, AttackTimeMs: 2000, DamageExp2: 177.438, AttackPower: 783, DualWield: true,
		Abilities: []OptimizerTankAbility{
			{SpellID: 55802, School: proto.SpellSchool_SpellSchoolFrost, Damage: 30000, PeriodMin: 2, PeriodMax: 10, CastSeconds: 2, LandChance: 0.5},
			{SpellID: 73799, School: proto.SpellSchool_SpellSchoolShadow, Damage: 70000, PeriodMin: 30.5, PeriodMax: 30.5, StartSeconds: 5},
		},
	}
	target := OptimizerTankBossTarget(boss)
	if target.Id != OptimizerTankBossID {
		t.Errorf("id = %d, want %d", target.Id, OptimizerTankBossID)
	}
	if got := abilitiesFromInputs(target.TargetInputs); !reflect.DeepEqual(got, boss.Abilities) {
		t.Errorf("abilities read back as %+v, want %+v", got, boss.Abilities)
	}
	if !target.DualWield || !target.DualWieldPenalty {
		t.Error("an off hand takes the dual wield miss penalty with it")
	}
}

// Kel'Thuzad's swing is (177.438 + 783/14) * 70 * 2.0 at its minimum: 32,671.
func TestOptimizerTankBossSwingFollowsTheStatRow(t *testing.T) {
	target := OptimizerTankBossTarget(OptimizerTankBoss{Name: "KT", DamageModifier: 70, AttackTimeMs: 2000, DamageExp2: 177.438, AttackPower: 783})
	const attackPowerCoefficient = 1.0 / (14.0 * 177.0)
	got := target.MinBaseDamage * (1 + 783*attackPowerCoefficient)
	if got < 32500 || got > 32800 {
		t.Errorf("minimum swing %.0f, want about 32,671", got)
	}
}
