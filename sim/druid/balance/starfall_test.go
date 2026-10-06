package balance

import (
	"testing"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
)

// Starfall's 53201 is an aura on the druid, so it never misses. Omen of Clarity rolls on the cast only, 8.75%
// (3.5 PPM over the 1.5 s floor); the live fight logs show no Clearcasting from the stars or the splash.
func TestStarfallNeverMissesAndOnlyItsCastRollsOmen(t *testing.T) {
	encounter := core.MakeSingleTargetEncounter(0)
	encounter.Duration = 12
	res := core.RunRaidSim(&proto.RaidSimRequest{
		Raid: core.SinglePlayerRaidProto(&proto.Player{
			Race:          proto.Race_RaceTauren,
			Class:         proto.Class_ClassDruid,
			Spec:          PlayerOptionsAdaptive,
			Consumes:      &proto.Consumes{},
			Rotation:      core.APLRotationFromJsonString(`{"type": "TypeAPL", "priorityList": [{"action": {"castSpell": {"spellId": {"spellId": 53201}}}}]}`),
			TalentsString: StandardTalents,
			// no gear, so no hit rating: a spell that rolls hit misses about one cast in eight
			Equipment: &proto.EquipmentSpec{},
		}, nil, nil, nil),
		Encounter:  encounter,
		SimOptions: &proto.SimOptions{Iterations: 400, IsTest: true, RandomSeed: 101},
	})
	if res.ErrorResult != "" {
		t.Fatal(res.ErrorResult)
	}
	player := res.RaidMetrics.Parties[0].Players[0]
	var casts, misses int32
	for _, action := range player.Actions {
		if action.Id.GetSpellId() == 53201 {
			for _, target := range action.Targets {
				casts += target.Casts
				misses += target.Misses
			}
		}
	}
	if casts == 0 || misses != 0 {
		t.Errorf("%d Starfalls, %d missed", casts, misses)
	}
	var procs float64
	for _, aura := range player.Auras {
		if aura.Id.GetSpellId() == 16870 {
			procs = aura.ProcsAvg
		}
	}
	if procs < 0.05 || procs > 0.14 {
		t.Errorf("%.3f Clearcasting procs a Starfall, want about 0.09", procs)
	}
	t.Logf("%d Starfalls, %.3f Clearcasting procs each", casts, procs)
}
