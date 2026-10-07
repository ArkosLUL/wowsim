package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/wowsims/wotlk/sim/core/proto"
	"github.com/wowsims/wotlk/sim/optimizer"
)

const (
	ohpCLIGreat     = 9381001
	ohpCLISword     = 9381002
	ohpCLIWeakGreat = 9381003
	ohpCLIAxe       = 9381004
)

// A warrior holding mh and oh, with mhPool and ohPool to pick from: a great sword that far
// outdamages two swords, a weak one that two swords outdamage, a sword and an axe a bit weaker.
func ohPairRequest(mh, oh int32, mhPool, ohPool []int32) *proto.OptimizeGearRequest {
	equipment := &proto.EquipmentSpec{Items: make([]*proto.ItemSpec, optimizer.NumSlots)}
	for slot := range equipment.Items {
		equipment.Items[slot] = &proto.ItemSpec{}
	}
	equipment.Items[proto.ItemSlot_ItemSlotMainHand] = &proto.ItemSpec{Id: mh}
	equipment.Items[proto.ItemSlot_ItemSlotOffHand] = &proto.ItemSpec{Id: oh}

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
	weapon := func(id int32, hand proto.HandType, wt proto.WeaponType, lo, hi, speed float64) *proto.SimItem {
		return &proto.SimItem{Id: id, Type: proto.ItemType_ItemTypeWeapon, HandType: hand, WeaponType: wt,
			WeaponDamageMin: lo, WeaponDamageMax: hi, WeaponSpeed: speed}
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
					weapon(ohpCLIGreat, proto.HandType_HandTypeTwoHand, proto.WeaponType_WeaponTypeSword, 900, 1100, 3.6),
					weapon(ohpCLISword, proto.HandType_HandTypeOneHand, proto.WeaponType_WeaponTypeSword, 150, 250, 2.6),
					weapon(ohpCLIWeakGreat, proto.HandType_HandTypeTwoHand, proto.WeaponType_WeaponTypeSword, 150, 250, 3.6),
					weapon(ohpCLIAxe, proto.HandType_HandTypeOneHand, proto.WeaponType_WeaponTypeAxe, 120, 200, 2.6),
				}},
			}}}}},
			Encounter:  &proto.Encounter{Duration: 60, Targets: []*proto.Target{{}}},
			SimOptions: &proto.SimOptions{RandomSeed: 3},
		},
		Settings: settings,
		Pool: &proto.CandidatePool{
			Slots: []*proto.SlotPool{
				{Slot: proto.ItemSlot_ItemSlotMainHand, ItemIds: mhPool},
				{Slot: proto.ItemSlot_ItemSlotOffHand, ItemIds: ohPool},
			},
			CatalogDate: "2026-09-18",
		},
	}
}

// optimizeOHPair runs req through the command and reads the result file, both as an
// OptimizerResult and as plain JSON, each alternative's offHand left raw.
func optimizeOHPair(t *testing.T, req *proto.OptimizeGearRequest) (*proto.OptimizerResult, []json.RawMessage) {
	t.Helper()
	outfile := filepath.Join(t.TempDir(), "result.json")
	if _, err := runOptimize(t, "--infile", writeRequest(t, req), "--outfile", outfile); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(outfile)
	if err != nil {
		t.Fatal(err)
	}
	var raw struct {
		Alternatives []struct {
			OffHand json.RawMessage `json:"offHand"`
		} `json:"alternatives"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatal(err)
	}
	var offHands []json.RawMessage
	for _, a := range raw.Alternatives {
		offHands = append(offHands, a.OffHand)
	}
	return readResult(t, data), offHands
}

// The main-hand runner-up to the great sword is the sword with a second one in the off hand, and the
// result file carries both.
func TestOptimizeCommandOffHandAlternative(t *testing.T) {
	result, _ := optimizeOHPair(t, ohPairRequest(ohpCLIGreat, 0, []int32{ohpCLIGreat, ohpCLISword}, []int32{ohpCLISword}))

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

// Against two swords, the weak great sword drops the off hand and the axe keeps it. The file says so
// by the offHand field's presence alone: an object, empty, for the great sword, null for the axe.
func TestOptimizeCommandEmptyOffHandAlternative(t *testing.T) {
	req := ohPairRequest(ohpCLISword, ohpCLISword, []int32{ohpCLISword, ohpCLIWeakGreat, ohpCLIAxe}, []int32{ohpCLISword})
	result, offHands := optimizeOHPair(t, req)

	items := result.GetBest().GetEquipment().GetItems()
	if mh, oh := items[proto.ItemSlot_ItemSlotMainHand].GetId(), items[proto.ItemSlot_ItemSlotOffHand].GetId(); mh != ohpCLISword || oh != ohpCLISword {
		t.Fatalf("best holds %d and %d, want %d in both hands", mh, oh, ohpCLISword)
	}
	want := map[int32]bool{ohpCLIWeakGreat: true, ohpCLIAxe: false}
	for i, a := range result.Alternatives {
		drops, ok := want[a.GetItem().GetId()]
		if a.Slot != proto.ItemSlot_ItemSlotMainHand || !ok {
			t.Errorf("unexpected %s alternative %d", a.Slot, a.GetItem().GetId())
			continue
		}
		delete(want, a.GetItem().GetId())
		if present := a.OffHand != nil; present != drops || a.GetOffHand().GetId() != 0 {
			t.Errorf("main hand %d reads off hand %v, want an empty one %v", a.GetItem().GetId(), a.OffHand, drops)
		}
		if object := len(offHands[i]) > 0 && offHands[i][0] == '{'; object != drops {
			t.Errorf("main hand %d's offHand in the file is %s, want an object %v", a.GetItem().GetId(), offHands[i], drops)
		}
	}
	if len(want) > 0 {
		t.Errorf("no alternative for main hands %v: %v", want, result.Alternatives)
	}
}
