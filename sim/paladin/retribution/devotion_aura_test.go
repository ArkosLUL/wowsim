package retribution

import (
	"math"
	"testing"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
	"github.com/wowsims/wotlk/sim/core/stats"
)

func devotionPaladin(aura proto.PaladinAura, points byte) *proto.Player {
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

// devotionRaid builds the players as one party with no other buffs.
func devotionRaid(players ...*proto.Player) []core.Agent {
	env, _, _ := core.NewEnvironment(&proto.Raid{
		Parties: []*proto.Party{{Players: players, Buffs: &proto.PartyBuffs{}}},
		Buffs:   &proto.RaidBuffs{},
		Debuffs: &proto.Debuffs{},
	}, core.MakeSingleTargetEncounter(0), false)
	return env.Raid.Parties[0].Players
}

// Improved Devotion Aura's 17% a point reaches every raider at every rank, and only the best paladin's
// aura counts.
func TestImprovedDevotionAuraEveryRank(t *testing.T) {
	armor := func(players ...*proto.Player) []float64 {
		var out []float64
		for _, agent := range devotionRaid(players...) {
			out = append(out, agent.GetCharacter().GetStat(stats.Armor))
		}
		return out
	}

	base := armor(devotionPaladin(proto.PaladinAura_DevotionAura, '0'), devotionPaladin(proto.PaladinAura_RetributionAura, '0'))
	for points, want := range []float64{0, 204, 409, 602} {
		got := armor(devotionPaladin(proto.PaladinAura_DevotionAura, byte('0'+points)), devotionPaladin(proto.PaladinAura_RetributionAura, '0'))
		for i := range got {
			if math.Abs(got[i]-base[i]-want) > 1e-6 {
				t.Errorf("Improved Devotion Aura %d, raider %d: +%.0f armor, want +%.0f", points, i, got[i]-base[i], want)
			}
		}
	}

	got := armor(devotionPaladin(proto.PaladinAura_DevotionAura, '1'), devotionPaladin(proto.PaladinAura_DevotionAura, '2'))
	for i := range got {
		if math.Abs(got[i]-base[i]-409) > 1e-6 {
			t.Errorf("1 and 2 points, raider %d: +%.0f armor, want the 2 points' +409", i, got[i]-base[i])
		}
	}
}

// The talent's 63514 gives everyone under the paladin's Devotion Aura 2% healing taken a point, and nothing
// under another aura.
func TestImprovedDevotionAuraHealingTaken(t *testing.T) {
	healingTaken := func(players ...*proto.Player) []float64 {
		var out []float64
		for _, agent := range devotionRaid(players...) {
			out = append(out, agent.GetCharacter().PseudoStats.HealingTakenMultiplier)
		}
		return out
	}
	other := devotionPaladin(proto.PaladinAura_RetributionAura, '0')

	for points, want := range []float64{1, 1.02, 1.04, 1.06} {
		for i, got := range healingTaken(devotionPaladin(proto.PaladinAura_DevotionAura, byte('0'+points)), other) {
			if math.Abs(got-want) > 1e-9 {
				t.Errorf("Improved Devotion Aura %d, raider %d: healing taken x%.3f, want x%.2f", points, i, got, want)
			}
		}
	}
	for i, got := range healingTaken(devotionPaladin(proto.PaladinAura_RetributionAura, '3'), other) {
		if got != 1 {
			t.Errorf("3 points under Retribution Aura, raider %d: healing taken x%.3f, want x1", i, got)
		}
	}
	for i, got := range healingTaken(devotionPaladin(proto.PaladinAura_DevotionAura, '1'), devotionPaladin(proto.PaladinAura_DevotionAura, '2')) {
		if math.Abs(got-1.04) > 1e-9 {
			t.Errorf("1 and 2 points, raider %d: healing taken x%.3f, want the 2 points' x1.04", i, got)
		}
	}
}
