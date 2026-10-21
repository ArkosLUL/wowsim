package enhancement

import (
	"math"
	"strings"
	"testing"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
	"github.com/wowsims/wotlk/sim/core/stats"
)

// Weapon Mastery (29082, 29084, 29086) is aura 79 on physical damage, 12/24/36% with mod-spell-tweaks. The
// server puts it in each hand's TOTAL_PCT (Unit::UpdateDamagePctDoneMods), so spells and Lava Lash's fire
// damage never see it.
func TestWeaponMasteryRanks(t *testing.T) {
	for rank, want := range []float64{1, 1.12, 1.24, 1.36} {
		sim := core.NewSim(&proto.RaidSimRequest{
			Raid: core.SinglePlayerRaidProto(&proto.Player{
				Race:      proto.Race_RaceOrc,
				Class:     proto.Class_ClassShaman,
				Equipment: &proto.EquipmentSpec{},
				Spec: &proto.Player_EnhancementShaman{EnhancementShaman: &proto.EnhancementShaman{
					Options: &proto.EnhancementShaman_Options{Totems: &proto.ShamanTotems{}},
				}},
				Glyphs:        &proto.Glyphs{},
				TalentsString: "-" + strings.Repeat("0", 16) + string(rune('0'+rank)),
				Rotation:      core.APLRotationFromJsonString(`{"type":"TypeAPL","priorityList":[]}`),
			}, &proto.PartyBuffs{}, &proto.RaidBuffs{}, &proto.Debuffs{}),
			Encounter:  core.MakeSingleTargetEncounter(0),
			SimOptions: &proto.SimOptions{Iterations: 1, IsTest: true, RandomSeed: 101},
		})
		shaman := sim.Raid.Parties[0].Players[0].(*EnhancementShaman)
		if int(shaman.Talents.WeaponMastery) != rank {
			t.Fatalf("Weapon Mastery %d, want %d", shaman.Talents.WeaponMastery, rank)
		}
		pseudo := shaman.PseudoStats
		if got := pseudo.SchoolDamageDealtMultiplier[stats.SchoolIndexPhysical]; math.Abs(got-want) > 1e-9 {
			t.Errorf("Weapon Mastery %d: physical damage ×%v, want ×%v", rank, got, want)
		}
		if fire, nature := pseudo.SchoolDamageDealtMultiplier[stats.SchoolIndexFire], pseudo.SchoolDamageDealtMultiplier[stats.SchoolIndexNature]; math.Abs(fire-1) > 1e-9 || math.Abs(nature-1) > 1e-9 {
			t.Errorf("Weapon Mastery %d: fire ×%v, nature ×%v, want neither changed", rank, fire, nature)
		}
	}
}
