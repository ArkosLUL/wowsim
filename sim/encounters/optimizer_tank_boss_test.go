package encounters

import (
	"math"
	"reflect"
	"testing"

	"github.com/wowsims/wotlk/sim/core/proto"
	"github.com/wowsims/wotlk/sim/core/stats"
)

func TestOptimizerTankBossAbilitiesRoundTrip(t *testing.T) {
	boss := OptimizerTankBoss{
		Name: "Test", DamageModifier: 70, AttackTimeMs: 2000, DualWield: true, NoParryHaste: true, SuppressDodge: true,
		HitChance: 5, EnemyDodgeMultiplier: 0.8,
		Abilities: []OptimizerTankAbility{
			{SpellID: 55802, School: proto.SpellSchool_SpellSchoolFrost, Damage: 30000, PeriodMin: 2, PeriodMax: 10, CastSeconds: 2, LandChance: 0.5, ResetsSwing: true},
			{SpellID: 64592, School: proto.SpellSchool_SpellSchoolPhysical, Damage: 35000, PeriodMin: 3, PeriodMax: 4.5, StartSeconds: 3.5},
			{SpellID: 73799, School: proto.SpellSchool_SpellSchoolShadow, Damage: 103081, PeriodMin: 30.5, PeriodMax: 30.5, ResetsSwing: true,
				FollowUpSeconds: 5, FollowUpDamage: 70000, FollowUpHaste: 2, FollowUpHasteSeconds: 5},
		},
	}
	target := OptimizerTankBossTarget(boss)
	if target.Id != OptimizerTankBossID {
		t.Errorf("id = %d, want %d", target.Id, OptimizerTankBossID)
	}
	hitChance, abilities := bossFromInputs(target.TargetInputs)
	if hitChance != boss.HitChance || !reflect.DeepEqual(abilities, boss.Abilities) {
		t.Errorf("read back hit chance %v and abilities %+v, want %v and %+v", hitChance, abilities, boss.HitChance, boss.Abilities)
	}
	if !target.DualWield || target.DualWieldPenalty {
		t.Errorf("dual wield %v with the miss penalty %v, want the off hand without the penalty", target.DualWield, target.DualWieldPenalty)
	}
	if target.ParryHaste || !target.SuppressDodge {
		t.Errorf("parry haste %v, dodge suppression %v, want off and on", target.ParryHaste, target.SuppressDodge)
	}
	if target.EnemyDodgeMultiplier != 0.8 {
		t.Errorf("enemy dodge multiplier = %v, want 0.8", target.EnemyDodgeMultiplier)
	}
}

// A request saved before the follow-up and the boss's hit chance existed has neither input. It still
// reads, with those fields at 0.
func TestOptimizerTankBossReadsOlderInputs(t *testing.T) {
	number := func(label string, value float64) *proto.TargetInput {
		return &proto.TargetInput{InputType: proto.InputType_Number, Label: label, NumberValue: value}
	}
	shadow := float64(proto.SpellSchool_SpellSchoolShadow)
	saved := []*proto.TargetInput{
		number("Spell id", 73799), number("School", shadow), number("Damage", 70000),
		number("Period min", 30.5), number("Period max", 30.5), number("Cast time", 0), number("Land chance", 1),
		number("Start", 5), number("Resets swing", 0),
		number("Spell id", 73781), number("School", shadow), number("Damage", 18750),
		number("Period min", 22.5), number("Period max", 22.5), number("Cast time", 2), number("Land chance", 1),
		number("Start", 0), number("Resets swing", 1),
	}
	want := []OptimizerTankAbility{
		{SpellID: 73799, School: proto.SpellSchool_SpellSchoolShadow, Damage: 70000, PeriodMin: 30.5, PeriodMax: 30.5, LandChance: 1, StartSeconds: 5},
		{SpellID: 73781, School: proto.SpellSchool_SpellSchoolShadow, Damage: 18750, PeriodMin: 22.5, PeriodMax: 22.5, CastSeconds: 2, LandChance: 1, ResetsSwing: true},
	}
	hitChance, abilities := bossFromInputs(saved)
	if hitChance != 0 || !reflect.DeepEqual(abilities, want) {
		t.Errorf("read hit chance %v and abilities %+v, want 0 and %+v", hitChance, abilities, want)
	}

	defer func() {
		if recover() == nil {
			t.Error("an input with a label nobody knows read without a panic")
		}
	}()
	bossFromInputs(append(saved, number("Splash", 1)))
}

// Kel'Thuzad's class 2 row swings (177.438 + 783/14) * 70 * 2.0 at its minimum. A boss without a row
// of its own gets the class 1 numbers.
func TestOptimizerTankBossSwingFollowsTheStatRow(t *testing.T) {
	for _, c := range []struct {
		boss              OptimizerTankBoss
		minDamage, ap, at float64
	}{
		{OptimizerTankBoss{Name: "KT", DamageModifier: 70, AttackTimeMs: 2000, DamageExp2: 177.438, AttackPower: 783}, 177.438 * 70 * 2.0, 783, 2.0},
		{OptimizerTankBoss{Name: "LK", DamageModifier: 492.9, AttackTimeMs: 1500}, 177.074 * 492.9 * 1.5, 805, 1.5},
		{OptimizerTankBoss{Name: "Anub", DamageModifier: 149.1}, 177.074 * 149.1 * 2.0, 805, 2.0},
	} {
		target := OptimizerTankBossTarget(c.boss)
		if math.Abs(target.MinBaseDamage-c.minDamage) > 0.001 {
			t.Errorf("%s: minimum weapon damage %.3f, want %.3f", c.boss.Name, target.MinBaseDamage, c.minDamage)
		}
		if got := target.Stats[stats.AttackPower]; got != c.ap {
			t.Errorf("%s: attack power %v, want %v", c.boss.Name, got, c.ap)
		}
		if target.SwingSpeed != c.at {
			t.Errorf("%s: swing speed %v, want %v", c.boss.Name, target.SwingSpeed, c.at)
		}
		if !target.ParryHaste {
			t.Errorf("%s: no parry haste, want it on unless the boss's row turns it off", c.boss.Name)
		}
	}
}
