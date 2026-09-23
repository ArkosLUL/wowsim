package warlock

import (
	"testing"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
	"github.com/wowsims/wotlk/sim/core/stats"
)

func TestDemonicPactFortyPlayerRaid(t *testing.T) {
	demo := func(name string) *proto.Player {
		return &proto.Player{
			Name:          name,
			Race:          proto.Race_RaceOrc,
			Class:         proto.Class_ClassWarlock,
			Equipment:     core.GetGearSet("../../ui/warlock/gear_sets", "p4_demo").GearSet,
			Consumes:      FullConsumes,
			TalentsString: DemonologyTalents,
			Glyphs:        DemonologyGlyphs,
			Spec:          DefaultDemonologyWarlock,
			Rotation:      core.GetAplRotation("../../ui/warlock/apls", "demo").Rotation,
		}
	}
	parties := make([]*proto.Party, 8)
	for i := range parties {
		parties[i] = &proto.Party{}
	}
	// group 8, past the 25 slots a 25-player raid has
	parties[0].Players = []*proto.Player{demo("Front")}
	parties[7].Players = []*proto.Player{demo("Back")}
	raid := &proto.Raid{Parties: parties, NumActiveParties: 8}

	computed := core.ComputeStats(&proto.ComputeStatsRequest{Raid: raid, Encounter: core.MakeSingleTargetEncounter(0)})
	if computed.ErrorResult != "" {
		t.Fatal(computed.ErrorResult)
	}
	if sp := computed.RaidStats.Parties[7].Players[0].FinalStats.Stats[stats.SpellPower]; sp <= 0 {
		t.Errorf("the warlock in group 8 has %v spell power, want its gear's", sp)
	}

	result := core.RunRaidSim(&proto.RaidSimRequest{
		Raid:       raid,
		Encounter:  core.MakeSingleTargetEncounter(0),
		SimOptions: &proto.SimOptions{Iterations: 1, IsTest: true, RandomSeed: 101},
	})
	if result.ErrorResult != "" {
		t.Fatal(result.ErrorResult)
	}
	if dps := result.RaidMetrics.Parties[7].Players[0].Dps.Avg; dps <= 0 {
		t.Errorf("the warlock in group 8 did %v dps, want a sim that ran", dps)
	}
}
