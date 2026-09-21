package wotlk_test

import (
	"testing"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
	"github.com/wowsims/wotlk/sim/shaman/elemental"
)

func init() {
	elemental.RegisterElementalShaman()
}

// goldens leave NibelungAverageCasts at 0, so they pass whether or not the pets get built
func TestValkyrPetsOnlyWithNibelung(t *testing.T) {
	for _, c := range []struct {
		name     string
		mainHand int32
		swapIn   int32
		want     int
	}{
		{"no Nibelung", 0, 0, 0},
		{"equipped", 49992, 0, 10},
		{"heroic equipped", 50648, 0, 10},
		{"only in item swap", 0, 50648, 10},
	} {
		t.Run(c.name, func(t *testing.T) {
			player := &proto.Player{
				Race:      proto.Race_RaceTroll,
				Class:     proto.Class_ClassShaman,
				Equipment: &proto.EquipmentSpec{},
				Spec: &proto.Player_ElementalShaman{
					ElementalShaman: &proto.ElementalShaman{Options: &proto.ElementalShaman_Options{}},
				},
				Rotation: &proto.APLRotation{},
			}
			if c.mainHand != 0 {
				player.Equipment.Items = []*proto.ItemSpec{{Id: c.mainHand}}
			}
			if c.swapIn != 0 {
				player.EnableItemSwap = true
				player.ItemSwap = &proto.ItemSwap{MhItem: &proto.ItemSpec{Id: c.swapIn}}
			}

			result := core.RunRaidSim(&proto.RaidSimRequest{
				Raid:       core.SinglePlayerRaidProto(player, &proto.PartyBuffs{}, &proto.RaidBuffs{}, &proto.Debuffs{}),
				Encounter:  core.MakeSingleTargetEncounter(0),
				SimOptions: &proto.SimOptions{Iterations: 1, RandomSeed: 101},
			})
			if result.ErrorResult != "" {
				t.Fatal(result.ErrorResult)
			}

			valkyrs := 0
			for _, pet := range result.RaidMetrics.Parties[0].Players[0].Pets {
				if pet.Name == "Valkyr" {
					valkyrs++
				}
			}
			if valkyrs != c.want {
				t.Errorf("got %d Val'kyr pets, want %d", valkyrs, c.want)
			}
		})
	}
}
