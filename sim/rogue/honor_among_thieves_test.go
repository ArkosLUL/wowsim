package rogue

import (
	"testing"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
)

// spell_proc 51698 rolls at the finish phase, once a cast whatever its targets' crits: a Fan of Knives on ten
// targets rolls once for its main hand cast and once for its off hand's, so 1/3 Honor Among Thieves procs on
// about 1 - (2/3)^2 of them, not on nearly all.
func TestHonorAmongThievesRollsOnceACast(t *testing.T) {
	targets := make([]*proto.Target, 10)
	for i := range targets {
		targets[i] = core.NewDefaultTarget()
	}
	// SubtletyTalents with Honor Among Thieves, the subtlety tree's 24th talent, at 1 point
	talents := []byte(SubtletyTalents)
	talents[len(talents)-len("3113")] = '1'
	res := core.RunRaidSim(&proto.RaidSimRequest{
		Raid: core.SinglePlayerRaidProto(&proto.Player{
			Race:          proto.Race_RaceHuman,
			Class:         proto.Class_ClassRogue,
			Equipment:     core.GetGearSet("../../ui/rogue/gear_sets", "p2_hemosub").GearSet,
			Consumes:      &proto.Consumes{},
			Spec:          &proto.Player_Rogue{Rogue: &proto.Rogue{Options: &proto.Rogue_Options{}}},
			Rotation:      core.APLRotationFromJsonString(`{"type": "TypeAPL", "priorityList": [{"action": {"castSpell": {"spellId": {"spellId": 51723}}}}]}`),
			TalentsString: string(talents),
		}, nil, nil, nil),
		Encounter:  &proto.Encounter{Duration: core.LongDuration, Targets: targets},
		SimOptions: &proto.SimOptions{Iterations: 20, IsTest: true, RandomSeed: 101},
	})
	if res.ErrorResult != "" {
		t.Fatal(res.ErrorResult)
	}
	player := res.RaidMetrics.Parties[0].Players[0]
	var casts, procs float64
	for _, action := range player.Actions {
		if action.Id.GetSpellId() == FanOfKnivesSpellID && action.Id.Tag == 0 {
			for _, target := range action.Targets {
				casts += float64(target.Casts)
			}
		}
	}
	for _, resource := range player.Resources {
		if resource.Id.GetSpellId() == 51701 {
			procs += float64(resource.Events)
		}
	}
	if casts == 0 {
		t.Fatal("no Fan of Knives")
	}
	if ratio := procs / casts; ratio < 0.4 || ratio > 0.7 {
		t.Errorf("%.0f procs in %.0f Fans of Knives (%.2f a cast), want about 0.55", procs, casts, ratio)
	}
}
