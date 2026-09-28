package wotlk_test

import (
	"math"
	"testing"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
	"github.com/wowsims/wotlk/sim/rogue"
)

func init() {
	rogue.RegisterRogue()
}

// spell_item_tiny_abomination_in_a_jar picks the off hand half the time when there's an off-hand
// weapon, and always the main hand without one
func TestTinyAbominationHand(t *testing.T) {
	for _, trinket := range []int32{50351, 50706} {
		for _, offHand := range []int32{0, 50736} {
			gear := &proto.EquipmentSpec{}
			for range proto.ItemSlot_ItemSlotRanged + 1 {
				gear.Items = append(gear.Items, &proto.ItemSpec{})
			}
			gear.Items[proto.ItemSlot_ItemSlotTrinket1].Id = trinket
			gear.Items[proto.ItemSlot_ItemSlotMainHand].Id = 50621
			gear.Items[proto.ItemSlot_ItemSlotOffHand].Id = offHand
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
					{"action":{"castSpell":{"spellId":{"spellId":48638}}}}
				]}`),
			}
			result := core.RunRaidSim(&proto.RaidSimRequest{
				Raid:       core.SinglePlayerRaidProto(player, &proto.PartyBuffs{}, &proto.RaidBuffs{}, &proto.Debuffs{}),
				Encounter:  core.MakeSingleTargetEncounter(0),
				SimOptions: &proto.SimOptions{Iterations: 20, RandomSeed: 101},
			})
			if result.ErrorResult != "" {
				t.Fatalf("trinket %d, off hand %d: %s", trinket, offHand, result.ErrorResult)
			}

			casts := map[int32]int32{}
			for _, action := range result.RaidMetrics.Parties[0].Players[0].Actions {
				for _, target := range action.Targets {
					casts[action.Id.GetSpellId()] += target.Casts
				}
			}
			mh, oh := casts[71433], casts[71434]
			if offHand == 0 {
				if mh == 0 || oh != 0 {
					t.Errorf("trinket %d, no off hand: Manifest Anger cast %d times from the main hand and %d from the off hand, want some and none",
						trinket, mh, oh)
				}
				continue
			}
			n := float64(mh + oh)
			if share := float64(oh) / n; n < 100 || math.Abs(share-0.5) > 5*math.Sqrt(0.25/n) {
				t.Errorf("trinket %d, off hand %d: Manifest Anger cast %d times from the main hand and %d from the off hand, want about half each",
					trinket, offHand, mh, oh)
			}
		}
	}
}
