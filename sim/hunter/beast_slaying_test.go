package hunter

import (
	"math"
	"testing"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
	googleProto "google.golang.org/protobuf/proto"
)

// Beast Slaying (20557) is DONE_VERSUS: it scales damage to beasts per target, Hunter's Mark's flat bonus included,
// so it can't sit in the troll's DamageDealtMultiplier, which a physical weapon hit's flat bonus leaves out.
func TestTrollBeastSlayingIsPerTarget(t *testing.T) {
	trollAgainst := func(first, second proto.MobType) *Hunter {
		encounter := core.MakeSingleTargetEncounter(0)
		other := googleProto.Clone(encounter.Targets[0]).(*proto.Target)
		encounter.Targets[0].MobType = first
		other.MobType = second
		encounter.Targets = append(encounter.Targets, other)
		sim := core.NewSim(&proto.RaidSimRequest{
			SimOptions: &proto.SimOptions{RandomSeed: 1},
			Raid: core.SinglePlayerRaidProto(&proto.Player{
				Race:          proto.Race_RaceTroll,
				Class:         proto.Class_ClassHunter,
				Equipment:     core.GetGearSet("../../ui/hunter/gear_sets", "p1_sv").GearSet,
				Spec:          PlayerOptionsBasic,
				Rotation:      core.GetAplRotation("../../ui/hunter/apls", "sv").Rotation,
				TalentsString: SVTalents,
			}, &proto.PartyBuffs{}, &proto.RaidBuffs{}, &proto.Debuffs{}),
			Encounter: encounter,
		})
		sim.Reset()
		return sim.Raid.Parties[0].Players[0].(*Hunter)
	}

	beastFirst := trollAgainst(proto.MobType_MobTypeBeast, proto.MobType_MobTypeDemon)
	demonFirst := trollAgainst(proto.MobType_MobTypeDemon, proto.MobType_MobTypeBeast)
	if beastFirst.PseudoStats.DamageDealtMultiplier != demonFirst.PseudoStats.DamageDealtMultiplier {
		t.Errorf("damage dealt x%v against a beast first, x%v against a demon first: want the same",
			beastFirst.PseudoStats.DamageDealtMultiplier, demonFirst.PseudoStats.DamageDealtMultiplier)
	}
	for _, h := range []*Hunter{beastFirst, demonFirst} {
		beast, demon := h.Env.Encounter.TargetUnits[0], h.Env.Encounter.TargetUnits[1]
		if beast.MobType != proto.MobType_MobTypeBeast {
			beast, demon = demon, beast
		}
		ratio := h.AttackTables[beast.UnitIndex].DamageDealtMultiplier / h.AttackTables[demon.UnitIndex].DamageDealtMultiplier
		if math.Abs(ratio-1.05) > 1e-9 {
			t.Errorf("beast over demon x%v, want x1.05", ratio)
		}
	}
}
