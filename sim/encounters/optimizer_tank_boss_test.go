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
		Abilities: []OptimizerTankAbility{
			{SpellID: 55802, School: proto.SpellSchool_SpellSchoolFrost, Damage: 30000, PeriodMin: 2, PeriodMax: 10, CastSeconds: 2, LandChance: 0.5, ResetsSwing: true},
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
	if !target.DualWield || target.DualWieldPenalty {
		t.Errorf("dual wield %v with the miss penalty %v, want the off hand without the penalty", target.DualWield, target.DualWieldPenalty)
	}
	if target.ParryHaste || !target.SuppressDodge {
		t.Errorf("parry haste %v, dodge suppression %v, want off and on", target.ParryHaste, target.SuppressDodge)
	}
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
