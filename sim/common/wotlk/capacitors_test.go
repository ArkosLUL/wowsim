package wotlk_test

import (
	"testing"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
	"github.com/wowsims/wotlk/sim/rogue"
)

func init() {
	rogue.RegisterRogue()
}

// Mutilate's off-hand hit lands with no off-hand weapon and can start the motes, so every Manifest
// Anger has to come from the main hand
func TestTinyAbominationWithoutOffHandWeapon(t *testing.T) {
	for _, trinket := range []int32{50351, 50706} {
		gear := &proto.EquipmentSpec{}
		for range proto.ItemSlot_ItemSlotRanged + 1 {
			gear.Items = append(gear.Items, &proto.ItemSpec{})
		}
		gear.Items[proto.ItemSlot_ItemSlotTrinket1].Id = trinket
		gear.Items[proto.ItemSlot_ItemSlotMainHand].Id = 50621
		player := &proto.Player{
			Race:          proto.Race_RaceHuman,
			Class:         proto.Class_ClassRogue,
			TalentsString: "005303104352100520103331051-005005003-502",
			Equipment:     gear,
			Spec: &proto.Player_Rogue{Rogue: &proto.Rogue{Options: &proto.Rogue_Options{
				MhImbue: proto.Rogue_Options_DeadlyPoison,
				OhImbue: proto.Rogue_Options_InstantPoison,
			}}},
			Rotation: core.APLRotationFromJsonString(`{"type":"TypeAPL","priorityList":[
				{"action":{"castSpell":{"spellId":{"spellId":48666}}}}
			]}`),
		}
		result := core.RunRaidSim(&proto.RaidSimRequest{
			Raid:       core.SinglePlayerRaidProto(player, &proto.PartyBuffs{}, &proto.RaidBuffs{}, &proto.Debuffs{}),
			Encounter:  core.MakeSingleTargetEncounter(0),
			SimOptions: &proto.SimOptions{Iterations: 10, RandomSeed: 101},
		})
		if result.ErrorResult != "" {
			t.Fatalf("trinket %d: %s", trinket, result.ErrorResult)
		}

		casts := map[int32]int32{}
		for _, action := range result.RaidMetrics.Parties[0].Players[0].Actions {
			for _, target := range action.Targets {
				casts[action.Id.GetSpellId()] += target.Casts
			}
		}
		if casts[71433] == 0 || casts[71434] != 0 {
			t.Errorf("trinket %d: Manifest Anger cast %d times from the main hand and %d from the off hand, want some and none",
				trinket, casts[71433], casts[71434])
		}
	}
}
