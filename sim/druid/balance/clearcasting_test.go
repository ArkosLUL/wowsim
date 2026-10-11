package balance

import (
	"testing"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
)

// Starfire rolls Omen of Clarity as it lands, ahead of its own cast-complete step: the Clearcasting it
// procs is for the next cast, so it stays on for about a Starfire's cast time.
func TestStarfireKeepsTheClearcastingItProcs(t *testing.T) {
	const starfire = 48465
	encounter := core.MakeSingleTargetEncounter(0)
	encounter.Duration = 120
	res := core.RunRaidSim(&proto.RaidSimRequest{
		Raid: core.SinglePlayerRaidProto(&proto.Player{
			Race:     proto.Race_RaceTauren,
			Class:    proto.Class_ClassDruid,
			Spec:     PlayerOptionsAdaptive,
			Consumes: &proto.Consumes{},
			Rotation: core.APLRotationFromJsonString(
				`{"type": "TypeAPL", "priorityList": [{"action": {"castSpell": {"spellId": {"spellId": 48465}}}}]}`),
			TalentsString: StandardTalents,
			Equipment:     core.GetGearSet("../../../ui/balance_druid/gear_sets", "p1").GearSet,
		}, nil, nil, nil),
		Encounter:  encounter,
		SimOptions: &proto.SimOptions{Iterations: 100, IsTest: true, RandomSeed: 101},
	})
	if res.ErrorResult != "" {
		t.Fatal(res.ErrorResult)
	}
	player := res.RaidMetrics.Parties[0].Players[0]
	var casts int32
	for _, action := range player.Actions {
		if action.Id.GetSpellId() == starfire {
			for _, target := range action.Targets {
				casts += target.Casts
			}
		}
	}
	for _, aura := range player.Auras {
		if aura.Id.GetSpellId() != 16870 {
			continue
		}
		if casts == 0 || aura.ProcsAvg < 1 {
			t.Fatalf("%d Starfires, %.2f Clearcasting procs a fight", casts, aura.ProcsAvg)
		}
		if perProc := aura.UptimeSecondsAvg / aura.ProcsAvg; perProc < 1.5 {
			t.Errorf("Clearcasting stays on %.2f s a proc, want about a Starfire's cast time", perProc)
		}
		return
	}
	t.Fatal("no Clearcasting aura")
}
