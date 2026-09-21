package hunter

import (
	"math"
	"testing"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
	"github.com/wowsims/wotlk/sim/core/stats"
)

func TestQuiverHaste(t *testing.T) {
	for _, tc := range []struct {
		quiver proto.Hunter_Options_Quiver
		weapon proto.RangedWeaponType
		want   float64
	}{
		{proto.Hunter_Options_Quiver15Percent, proto.RangedWeaponType_RangedWeaponTypeBow, 1.15},
		{proto.Hunter_Options_Quiver15Percent, proto.RangedWeaponType_RangedWeaponTypeCrossbow, 1.15},
		{proto.Hunter_Options_Quiver15Percent, proto.RangedWeaponType_RangedWeaponTypeGun, 1.15},
		// no bag holds thrown weapons
		{proto.Hunter_Options_Quiver15Percent, proto.RangedWeaponType_RangedWeaponTypeThrown, 1},
		{proto.Hunter_Options_QuiverNone, proto.RangedWeaponType_RangedWeaponTypeBow, 1},
	} {
		weapon := &core.Item{RangedWeaponType: tc.weapon}
		if got := quiverHaste(tc.quiver, weapon); got != tc.want {
			t.Errorf("%v with a %v: x%v, want x%v", tc.quiver, tc.weapon, got, tc.want)
		}
	}
	if got := quiverHaste(proto.Hunter_Options_Quiver15Percent, nil); got != 1 {
		t.Errorf("no ranged weapon: x%v, want x1", got)
	}
}

// spell_hun_generic_scaling: 45% stamina, 22% ranged AP (plus Hunter vs. Wild's stamina share) and
// 12.87% ranged AP as spell damage, with Wild Hunt's int32 AddPct for the first two.
func TestPetStatInheritance(t *testing.T) {
	owner := stats.Stats{stats.RangedAttackPower: 6000, stats.Stamina: 2000, stats.Armor: 10000}
	for _, tc := range []struct {
		hvw, wildHunt           int32
		stamina, ap, spellPower float64
	}{
		{0, 0, 900, 1320, 772.2},
		// 2000 stamina x 30% goes on top of the 6000: 6600 x 22%
		{3, 0, 900, 1452, 772.2},
		// 45 + trunc(45 x 20%) = 54, 22 + trunc(3.3) = 25, 12.87 x 1.15
		{0, 1, 1080, 1500, 888.03},
		// 63, 28, 12.87 x 1.3
		{3, 2, 1260, 1848, 1003.86},
	} {
		hunter := &Hunter{
			Talents: &proto.HunterTalents{HunterVsWild: tc.hvw},
			Options: &proto.Hunter_Options{PetTalents: &proto.HunterPetTalents{WildHunt: tc.wildHunt}},
		}
		got := hunter.makeStatInheritance()(owner)
		for _, check := range []struct {
			stat stats.Stat
			want float64
		}{
			{stats.Stamina, tc.stamina}, {stats.AttackPower, tc.ap}, {stats.SpellPower, tc.spellPower}, {stats.Armor, 3500},
		} {
			if math.Abs(got[check.stat]-check.want) > 0.01 {
				t.Errorf("Hunter vs. Wild %d, Wild Hunt %d: pet %v %.2f, want %.2f",
					tc.hvw, tc.wildHunt, check.stat, got[check.stat], check.want)
			}
		}
		if got[stats.MeleeHit] != 0 || got[stats.SpellHit] != 0 || got[stats.Expertise] != 0 {
			t.Errorf("hit and expertise come from core's scaling aura, got %v", got)
		}
	}
}
