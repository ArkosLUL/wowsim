package hunter

import (
	"testing"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
	googleProto "google.golang.org/protobuf/proto"
)

// The optimizer and the gear editor can both leave the ranged slot empty. Auto Shot with no weapon
// would swing every 0 s and the sim would never end.
func TestNoRangedWeapon(t *testing.T) {
	gear := googleProto.Clone(core.GetGearSet("../../ui/hunter/gear_sets", "p1_mm").GearSet).(*proto.EquipmentSpec)
	gear.Items[proto.ItemSlot_ItemSlotRanged] = &proto.ItemSpec{}
	rsr := &proto.RaidSimRequest{
		Raid: core.SinglePlayerRaidProto(
			&proto.Player{
				Race:               proto.Race_RaceOrc,
				Class:              proto.Class_ClassHunter,
				Equipment:          gear,
				Consumes:           FullConsumes,
				Spec:               PlayerOptionsBasic,
				Glyphs:             MMGlyphs,
				TalentsString:      MMTalents,
				Buffs:              core.FullIndividualBuffs,
				Rotation:           core.GetAplRotation("../../ui/hunter/apls", "mm").Rotation,
				DistanceFromTarget: 30,
			},
			core.FullPartyBuffs,
			core.FullRaidBuffs,
			core.FullDebuffs),
		Encounter:  core.MakeSingleTargetEncounter(0),
		SimOptions: &proto.SimOptions{Iterations: 10, IsTest: true, RandomSeed: 101},
	}

	result := core.RunRaidSim(rsr)
	if result.ErrorResult != "" {
		t.Fatal(result.ErrorResult)
	}
	player := result.RaidMetrics.Parties[0].Players[0]
	if player.Dps.Avg <= 0 {
		t.Errorf("DPS = %.1f, want the shots and the pet to still do damage", player.Dps.Avg)
	}
	for _, action := range player.Actions {
		if action.Id.GetOtherId() == proto.OtherAction_OtherActionShoot {
			t.Errorf("Auto Shot went off %d times without a ranged weapon", action.Targets[0].Casts)
		}
	}
}
