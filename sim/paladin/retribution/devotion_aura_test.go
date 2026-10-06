package retribution

import (
	"math"
	"testing"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
	"github.com/wowsims/wotlk/sim/core/stats"
)

// Improved Devotion Aura's 17% a point reaches every raider at every rank, and only the best paladin's
// aura counts.
func TestImprovedDevotionAuraEveryRank(t *testing.T) {
	paladin := func(aura proto.PaladinAura, points byte) *proto.Player {
		return &proto.Player{
			Race:          proto.Race_RaceHuman,
			Class:         proto.Class_ClassPaladin,
			Equipment:     core.GetGearSet("../../../ui/retribution_paladin/gear_sets", "p1").GearSet,
			Consumes:      &proto.Consumes{},
			Spec:          &proto.Player_RetributionPaladin{RetributionPaladin: &proto.RetributionPaladin{Options: &proto.RetributionPaladin_Options{Aura: aura}}},
			Rotation:      &proto.APLRotation{Type: proto.APLRotation_TypeAPL},
			TalentsString: "-0500000000" + string(points),
		}
	}
	armor := func(players ...*proto.Player) []float64 {
		env, _, _ := core.NewEnvironment(&proto.Raid{
			Parties: []*proto.Party{{Players: players, Buffs: &proto.PartyBuffs{}}},
			Buffs:   &proto.RaidBuffs{},
			Debuffs: &proto.Debuffs{},
		}, core.MakeSingleTargetEncounter(0), false)
		var out []float64
		for _, agent := range env.Raid.Parties[0].Players {
			out = append(out, agent.GetCharacter().GetStat(stats.Armor))
		}
		return out
	}

	base := armor(paladin(proto.PaladinAura_DevotionAura, '0'), paladin(proto.PaladinAura_RetributionAura, '0'))
	for points, want := range []float64{0, 204, 409, 602} {
		got := armor(paladin(proto.PaladinAura_DevotionAura, byte('0'+points)), paladin(proto.PaladinAura_RetributionAura, '0'))
		for i := range got {
			if math.Abs(got[i]-base[i]-want) > 1e-6 {
				t.Errorf("Improved Devotion Aura %d, raider %d: +%.0f armor, want +%.0f", points, i, got[i]-base[i], want)
			}
		}
	}

	got := armor(paladin(proto.PaladinAura_DevotionAura, '1'), paladin(proto.PaladinAura_DevotionAura, '2'))
	for i := range got {
		if math.Abs(got[i]-base[i]-409) > 1e-6 {
			t.Errorf("1 and 2 points, raider %d: +%.0f armor, want the 2 points' +409", i, got[i]-base[i])
		}
	}
}
