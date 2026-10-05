package warlock

import (
	"math"
	"testing"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
	"github.com/wowsims/wotlk/sim/core/stats"
	googleProto "google.golang.org/protobuf/proto"
)

// Improved Imp's 10% a point reaches the Imp's Blood Pact at every rank, on the warlock and on its pet.
func TestImprovedImpBloodPactEveryRank(t *testing.T) {
	health := func(talents string) (owner, pet float64) {
		options := googleProto.Clone(DefaultAfflictionWarlock.Warlock.Options).(*proto.Warlock_Options)
		options.Summon = proto.Warlock_Options_Imp
		raid := core.SinglePlayerRaidProto(&proto.Player{
			Race:          proto.Race_RaceOrc,
			Class:         proto.Class_ClassWarlock,
			Equipment:     core.GetGearSet("../../ui/warlock/gear_sets", "p4_affliction").GearSet,
			Consumes:      &proto.Consumes{},
			Spec:          &proto.Player_Warlock{Warlock: &proto.Warlock{Options: options}},
			Rotation:      core.GetAplRotation("../../ui/warlock/apls", "affliction").Rotation,
			TalentsString: talents,
		}, &proto.PartyBuffs{}, &proto.RaidBuffs{}, &proto.Debuffs{})
		sim := core.NewSim(&proto.RaidSimRequest{
			Raid:       raid,
			Encounter:  core.MakeSingleTargetEncounter(0),
			SimOptions: &proto.SimOptions{Iterations: 1, IsTest: true, RandomSeed: 101},
		})
		sim.Reset()
		warlock := sim.Raid.Parties[0].Players[0].(*Warlock)
		return warlock.GetStat(stats.Health), warlock.Pet.GetStat(stats.Health)
	}

	baseOwner, basePet := health("-00")
	for points, want := range []float64{0, 133, 266, 399} {
		owner, pet := health("-0" + string(rune('0'+points)))
		if math.Abs(owner-baseOwner-want) > 1e-6 || math.Abs(pet-basePet-want) > 1e-6 {
			t.Errorf("Improved Imp %d: warlock +%.0f, Imp +%.0f, want +%.0f", points, owner-baseOwner, pet-basePet, want)
		}
	}
}
