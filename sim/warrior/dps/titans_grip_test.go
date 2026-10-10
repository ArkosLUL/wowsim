package dps

import (
	"math"
	"testing"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
	"github.com/wowsims/wotlk/sim/core/stats"
	googleProto "google.golang.org/protobuf/proto"
)

const (
	testTwoHander = 40384 // Betrayer of Humanity
	testOneHander = 40402 // Last Laugh
	testShield    = 40400 // Wall of Terror
	testHeldItem  = 40350 // Urn of Lost Memories
)

// titansGripPhysical is the physical damage multiplier of a Titan's Grip warrior holding these two items.
func titansGripPhysical(t *testing.T, mainHand, offHand int32, tweaks *proto.SpellTweaksSettings) float64 {
	t.Helper()
	gear := &proto.EquipmentSpec{}
	for range proto.ItemSlot_ItemSlotRanged + 1 {
		gear.Items = append(gear.Items, &proto.ItemSpec{})
	}
	gear.Items[proto.ItemSlot_ItemSlotMainHand].Id = mainHand
	gear.Items[proto.ItemSlot_ItemSlotOffHand].Id = offHand
	encounter := core.MakeSingleTargetEncounter(0)
	encounter.ServerSettings = &proto.ServerSettings{SpellTweaks: tweaks}

	sim := core.NewSim(&proto.RaidSimRequest{
		Raid: core.SinglePlayerRaidProto(&proto.Player{
			Race:          proto.Race_RaceOrc,
			Class:         proto.Class_ClassWarrior,
			Equipment:     gear,
			Spec:          PlayerOptionsFury,
			Glyphs:        &proto.Glyphs{},
			TalentsString: FuryTalents,
			Rotation:      core.APLRotationFromJsonString(`{"type":"TypeAPL","priorityList":[]}`),
		}, &proto.PartyBuffs{}, &proto.RaidBuffs{}, &proto.Debuffs{}),
		Encounter:  encounter,
		SimOptions: &proto.SimOptions{Iterations: 1, IsTest: true, RandomSeed: 101},
	})
	warrior := sim.Raid.Parties[0].Players[0].(*DpsWarrior)
	if !warrior.Talents.TitansGrip {
		t.Fatal("FuryTalents has no Titan's Grip")
	}
	return warrior.PseudoStats.SchoolDamageDealtMultiplier[stats.SchoolIndexPhysical]
}

// Stock Titan's Grip (49152) takes 10% physical damage with a weapon in the main hand, a weapon, shield or
// held-in-off-hand item in the off hand, and a two-hander in either (AuraEffect::CalculateAmount). The
// TitansGripNoDamagePenalty tweak zeroes it.
func TestTitansGripPenalty(t *testing.T) {
	off := googleProto.Bool(false)
	noTweak := &proto.SpellTweaksSettings{TitansGripNoDamagePenalty: off}

	for _, tc := range []struct {
		name              string
		mainHand, offHand int32
		tweaks            *proto.SpellTweaksSettings
		penalty           float64
	}{
		{"two two-handers", testTwoHander, testTwoHander, noTweak, 0.9},
		{"a two-hander and a one-hander", testTwoHander, testOneHander, noTweak, 0.9},
		{"a one-hander and a two-hander", testOneHander, testTwoHander, noTweak, 0.9},
		{"a two-hander and a shield", testTwoHander, testShield, noTweak, 0.9},
		{"a two-hander and a held item", testTwoHander, testHeldItem, noTweak, 0.9},
		{"a two-hander and a shield, SpellTweaks.Enable off", testTwoHander, testShield, &proto.SpellTweaksSettings{Enable: off}, 0.9},
		{"a two-hander alone", testTwoHander, 0, noTweak, 1},
		{"a one-hander and a shield", testOneHander, testShield, noTweak, 1},
		{"two one-handers", testOneHander, testOneHander, noTweak, 1},
		{"a shield alone", 0, testShield, noTweak, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// the live server runs the tweak, so its settings give the same hands without the penalty
			free := titansGripPhysical(t, tc.mainHand, tc.offHand, nil)
			got := titansGripPhysical(t, tc.mainHand, tc.offHand, tc.tweaks) / free
			if math.Abs(got-tc.penalty) > 1e-9 {
				t.Errorf("physical damage x%v of what the tweak leaves, want x%v", got, tc.penalty)
			}
		})
	}
}
