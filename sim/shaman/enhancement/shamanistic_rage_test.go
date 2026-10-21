package enhancement

import (
	"testing"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
)

// With the T10 2pc (Sanctified Frost Witch's Shoulderguards and Grips), Shamanistic Rage is a damage
// cooldown cast on cooldown; without it, only at low mana, which an idle shaman never reaches.
func TestShamanisticRageOnCooldownWithT10(t *testing.T) {
	for _, tc := range []struct {
		name     string
		t10      bool
		min, max int32
	}{
		{"T10 2pc", true, 4, 5},
		{"no T10", false, 0, 0},
	} {
		gear := &proto.EquipmentSpec{}
		for range 17 {
			gear.Items = append(gear.Items, &proto.ItemSpec{})
		}
		if tc.t10 {
			gear.Items[proto.ItemSlot_ItemSlotShoulder] = &proto.ItemSpec{Id: 51240}
			gear.Items[proto.ItemSlot_ItemSlotHands] = &proto.ItemSpec{Id: 51243}
		}
		encounter := core.MakeSingleTargetEncounter(0)
		encounter.Duration = 270
		res := core.RunRaidSim(&proto.RaidSimRequest{
			Raid: core.SinglePlayerRaidProto(&proto.Player{
				Race:      proto.Race_RaceOrc,
				Class:     proto.Class_ClassShaman,
				Equipment: gear,
				Spec: &proto.Player_EnhancementShaman{EnhancementShaman: &proto.EnhancementShaman{
					Options: &proto.EnhancementShaman_Options{Totems: &proto.ShamanTotems{}},
				}},
				Glyphs:        &proto.Glyphs{},
				TalentsString: StandardTalents,
				Rotation:      core.APLRotationFromJsonString(`{"type":"TypeAPL","priorityList":[{"action":{"autocastOtherCooldowns":{}}}]}`),
			}, &proto.PartyBuffs{}, &proto.RaidBuffs{}, &proto.Debuffs{}),
			Encounter:  encounter,
			SimOptions: &proto.SimOptions{Iterations: 1, IsTest: true, RandomSeed: 101},
		})
		if res.ErrorResult != "" {
			t.Fatalf("%s: %s", tc.name, res.ErrorResult)
		}
		var casts int32
		for _, action := range res.RaidMetrics.Parties[0].Players[0].Actions {
			if action.Id.GetSpellId() == 30823 {
				for _, target := range action.Targets {
					casts += target.Casts
				}
			}
		}
		if casts < tc.min || casts > tc.max {
			t.Errorf("%s: %d Shamanistic Rage casts over 270 s, want %d to %d", tc.name, casts, tc.min, tc.max)
		}
	}
}
