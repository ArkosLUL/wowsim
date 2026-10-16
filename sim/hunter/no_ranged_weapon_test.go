package hunter

import (
	"math"
	"testing"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
	googleProto "google.golang.org/protobuf/proto"
)

// The optimizer and the gear editor can both leave the ranged slot empty. Auto Shot with no weapon
// would swing every 0 s and the sim would never end, and a shot would roll an empty weapon's damage.
// Spell::CheckItems refuses the shots without a bow, gun or crossbow and Raptor Strike without a main hand,
// so only the pet and the spells that need no weapon are left.
func TestNoRangedWeapon(t *testing.T) {
	for _, emptyMainHand := range []bool{false, true} {
		gear := googleProto.Clone(core.GetGearSet("../../ui/hunter/gear_sets", "p1_mm").GearSet).(*proto.EquipmentSpec)
		gear.Items[proto.ItemSlot_ItemSlotRanged] = &proto.ItemSpec{}
		if emptyMainHand {
			gear.Items[proto.ItemSlot_ItemSlotMainHand] = &proto.ItemSpec{}
		}
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
		if math.IsNaN(player.Dps.Avg) || player.Dps.Avg <= 0 {
			t.Errorf("empty main hand %v: DPS = %.1f, want the pet and the spells that need no weapon to still do damage",
				emptyMainHand, player.Dps.Avg)
		}
		for _, action := range player.Actions {
			casts := int32(0)
			for _, target := range action.Targets {
				casts += target.Casts
			}
			id := action.Id.GetSpellId()
			switch {
			case action.Id.GetOtherId() == proto.OtherAction_OtherActionShoot:
				t.Errorf("empty main hand %v: Auto Shot went off %d times without a ranged weapon", emptyMainHand, casts)
			case needsShotWeapon[id] && casts > 0:
				t.Errorf("empty main hand %v: %d cast %d times without a ranged weapon", emptyMainHand, id, casts)
			case id == raptorStrikeSpellID && emptyMainHand && casts > 0:
				t.Errorf("Raptor Strike cast %d times without a main hand", casts)
			}
		}
	}
}
