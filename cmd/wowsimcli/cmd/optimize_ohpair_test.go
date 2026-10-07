package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/wowsims/wotlk/sim/core/proto"
	"github.com/wowsims/wotlk/sim/optimizer"
)

const (
	ohpCLIGreat = 9381001
	ohpCLISword = 9381002
)

// A warrior holding a great sword that far outdamages two swords, with a sword the only off hand.
func ohPairRequest() *proto.OptimizeGearRequest {
	equipment := &proto.EquipmentSpec{Items: make([]*proto.ItemSpec, optimizer.NumSlots)}
	for slot := range equipment.Items {
		equipment.Items[slot] = &proto.ItemSpec{}
	}
	equipment.Items[proto.ItemSlot_ItemSlotMainHand] = &proto.ItemSpec{Id: ohpCLIGreat}

	settings := &proto.OptimizerSettings{
		ContentPhase:    1,
		Effort:          proto.OptimizerEffort_OptimizerEffortQuick,
		RacialMode:      proto.OptimizerRacialMode_OptimizerRacialKeepCurrent,
		DisableStatCaps: true,
	}
	for slot := proto.ItemSlot(0); int(slot) < optimizer.NumSlots; slot++ {
		if slot != proto.ItemSlot_ItemSlotMainHand && slot != proto.ItemSlot_ItemSlotOffHand {
			settings.LockedSlots = append(settings.LockedSlots, slot)
		}
	}
	return &proto.OptimizeGearRequest{
		Base: &proto.RaidSimRequest{
			Raid: &proto.Raid{Parties: []*proto.Party{{Players: []*proto.Player{{
				Name:      "Target",
				Race:      proto.Race_RaceOrc,
				Class:     proto.Class_ClassWarrior,
				Equipment: equipment,
				Spec:      &proto.Player_Warrior{Warrior: &proto.Warrior{Options: &proto.Warrior_Options{}}},
				Rotation:  &proto.APLRotation{Type: proto.APLRotation_TypeAPL},
				Database: &proto.SimDatabase{Items: []*proto.SimItem{
					{Id: ohpCLIGreat, Type: proto.ItemType_ItemTypeWeapon, HandType: proto.HandType_HandTypeTwoHand, WeaponType: proto.WeaponType_WeaponTypeSword,
						WeaponDamageMin: 900, WeaponDamageMax: 1100, WeaponSpeed: 3.6},
					{Id: ohpCLISword, Type: proto.ItemType_ItemTypeWeapon, HandType: proto.HandType_HandTypeOneHand, WeaponType: proto.WeaponType_WeaponTypeSword,
						WeaponDamageMin: 150, WeaponDamageMax: 250, WeaponSpeed: 2.6},
				}},
			}}}}},
			Encounter:  &proto.Encounter{Duration: 60, Targets: []*proto.Target{{}}},
			SimOptions: &proto.SimOptions{RandomSeed: 3},
		},
		Settings: settings,
		Pool: &proto.CandidatePool{
			Slots: []*proto.SlotPool{
				{Slot: proto.ItemSlot_ItemSlotMainHand, ItemIds: []int32{ohpCLIGreat, ohpCLISword}},
				{Slot: proto.ItemSlot_ItemSlotOffHand, ItemIds: []int32{ohpCLISword}},
			},
			CatalogDate: "2026-09-18",
		},
	}
}

// The main-hand runner-up to the great sword is the sword with a second one in the off hand, and the
// result file carries both.
func TestOptimizeCommandOffHandAlternative(t *testing.T) {
	outfile := filepath.Join(t.TempDir(), "result.json")
	if _, err := runOptimize(t, "--infile", writeRequest(t, ohPairRequest()), "--outfile", outfile); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(outfile)
	if err != nil {
		t.Fatal(err)
	}
	result := readResult(t, data)

	items := result.GetBest().GetEquipment().GetItems()
	if mh, oh := items[proto.ItemSlot_ItemSlotMainHand].GetId(), items[proto.ItemSlot_ItemSlotOffHand].GetId(); mh != ohpCLIGreat || oh != 0 {
		t.Fatalf("best holds %d and %d, want %d alone", mh, oh, ohpCLIGreat)
	}
	for _, a := range result.Alternatives {
		if a.Slot == proto.ItemSlot_ItemSlotMainHand && a.GetItem().GetId() == ohpCLISword {
			if a.GetOffHand().GetId() != ohpCLISword {
				t.Errorf("the sword's off hand reads %d, want %d", a.GetOffHand().GetId(), ohpCLISword)
			}
			return
		}
	}
	t.Fatalf("no main-hand alternative holds the sword: %v", result.Alternatives)
}
