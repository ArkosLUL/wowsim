package wotlk_test

import (
	"math"
	"testing"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
	"github.com/wowsims/wotlk/sim/core/stats"
)

const (
	swapTestSword    = 37179 // Infantry Assault Blade, one-hand
	swapTestOffSword = 37235 // Crypt Lord's Deft Blade, one-hand
	swapTestAxe      = 37260 // Cloudstrider's Waraxe, one-hand
)

// swapRaceDK is a death knight of race wearing mh and oh (0 for none) with bag to swap to in the main hand.
func swapRaceDK(t *testing.T, race proto.Race, mh, oh, bag int32) (*core.Simulation, *core.Character) {
	t.Helper()
	items := make([]*proto.ItemSpec, proto.ItemSlot_ItemSlotRanged+1)
	for i := range items {
		items[i] = &proto.ItemSpec{}
	}
	items[proto.ItemSlot_ItemSlotMainHand] = &proto.ItemSpec{Id: mh}
	items[proto.ItemSlot_ItemSlotOffHand] = &proto.ItemSpec{Id: oh}
	sim := core.NewSim(&proto.RaidSimRequest{
		Raid: core.SinglePlayerRaidProto(&proto.Player{
			Race:           race,
			Class:          proto.Class_ClassDeathknight,
			Equipment:      &proto.EquipmentSpec{Items: items},
			Consumes:       &proto.Consumes{},
			Spec:           &proto.Player_Deathknight{Deathknight: &proto.Deathknight{Options: &proto.Deathknight_Options{}}},
			Rotation:       core.APLRotationFromJsonString(`{"type": "TypeAPL", "priorityList": []}`),
			EnableItemSwap: true,
			ItemSwap:       &proto.ItemSwap{MhItem: &proto.ItemSpec{Id: bag}},
		}, &proto.PartyBuffs{}, &proto.RaidBuffs{}, &proto.Debuffs{}),
		Encounter:  core.MakeSingleTargetEncounter(0),
		SimOptions: &proto.SimOptions{Iterations: 1, IsTest: true, RandomSeed: 101},
	})
	sim.Reset()
	return sim, sim.Raid.Parties[0].Players[0].GetCharacter()
}

// expertise is the expertise rating each hand's white swing reads, and the sheet's.
func expertise(c *core.Character) [3]float64 {
	sheet := c.GetStat(stats.Expertise)
	return [3]float64{sheet + c.AutoAttacks.MHAuto().BonusExpertiseRating, sheet + c.AutoAttacks.OHAuto().BonusExpertiseRating, sheet}
}

// A human's Sword Specialization counts on a hand while it holds a sword or a mace (Player::UpdateExpertise),
// so a swap moves it: in the main hand, out with an axe, and back with the sword. An iteration that ends
// swapped starts the next on the sword. A night elf in the same gear has none, so the difference is the racial's.
func TestItemSwapMovesTheRacialWeaponExpertise(t *testing.T) {
	spec, _ := core.RacialWeaponSpecialization(proto.Race_RaceHuman)
	e := spec.Expertise
	for _, c := range []struct {
		name       string
		oh         int32
		sword, axe [3]float64 // main hand, off hand, sheet
	}{
		{"main hand only", 0, [3]float64{e, e, e}, [3]float64{0, 0, 0}},
		{"two swords", swapTestOffSword, [3]float64{e, e, e}, [3]float64{0, e, 0}},
	} {
		humanSim, human := swapRaceDK(t, proto.Race_RaceHuman, swapTestSword, c.oh, swapTestAxe)
		elfSim, elf := swapRaceDK(t, proto.Race_RaceNightElf, swapTestSword, c.oh, swapTestAxe)
		expect := func(when string, want [3]float64) {
			t.Helper()
			h, n := expertise(human), expertise(elf)
			var got [3]float64
			off := false
			for i := range got {
				got[i] = h[i] - n[i]
				off = off || math.Abs(got[i]-want[i]) > 1e-6
			}
			if off {
				t.Errorf("%s, %s: the racial's main hand, off hand and sheet expertise %v, want %v", c.name, when, got, want)
			}
		}
		swap := func() {
			slots := []proto.ItemSlot{proto.ItemSlot_ItemSlotMainHand}
			human.ItemSwap.SwapItems(humanSim, slots)
			elf.ItemSwap.SwapItems(elfSim, slots)
		}

		expect("on the sword", c.sword)
		swap()
		expect("swapped to the axe", c.axe)
		swap()
		expect("swapped back", c.sword)
		swap()
		for _, sim := range []*core.Simulation{humanSim, elfSim} {
			sim.Cleanup()
			sim.Reset()
		}
		expect("the next iteration", c.sword)
	}
}
