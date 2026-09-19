package sim

import (
	"testing"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
	"github.com/wowsims/wotlk/sim/core/stats"
)

func init() {
	RegisterAll()
}

var SimOptions = &proto.SimOptions{
	Iterations: 1,
	IsTest:     true,
}

var StandardTarget = &proto.Target{
	Stats:   stats.Stats{stats.Armor: 7684}.ToFloatArray(),
	MobType: proto.MobType_MobTypeDemon,
}

var STEncounter = &proto.Encounter{
	Duration: 300,
	Targets: []*proto.Target{
		StandardTarget,
	},
}

var BasicRaid = &proto.Raid{
	Parties: []*proto.Party{
		{
			Players: []*proto.Player{},
		},
		{
			Players: []*proto.Player{},
		},
	},
}

// Tests that we don't crash with various combinations of empty parties / blank players.
func TestSparseRaid(t *testing.T) {
	sparseRaid := &proto.Raid{
		Parties: []*proto.Party{
			{},
			{
				Players: []*proto.Player{
					{},
					{},
				},
			},
			{
				Players: []*proto.Player{
					{},
					{},
				},
			},
		},
	}

	rsr := &proto.RaidSimRequest{
		Raid:       sparseRaid,
		Encounter:  STEncounter,
		SimOptions: SimOptions,
	}

	core.RunRaidSim(rsr)
	// Don't need to check results, as long as it doesn't crash we're fine.
}

// A player after an empty slot still gets its own slot's buffs and APL.
func TestSparseRaidKeepsPlayerSettings(t *testing.T) {
	druid := &proto.Player{
		Name:      "Druid",
		Race:      proto.Race_RaceNightElf,
		Class:     proto.Class_ClassDruid,
		Equipment: &proto.EquipmentSpec{},
		Spec:      &proto.Player_BalanceDruid{BalanceDruid: &proto.BalanceDruid{Options: &proto.BalanceDruid_Options{}}},
		Buffs:     &proto.IndividualBuffs{BlessingOfKings: true},
		Rotation: &proto.APLRotation{Type: proto.APLRotation_TypeAPL, PriorityList: []*proto.APLListItem{{
			Action: &proto.APLAction{Action: &proto.APLAction_CastSpell{CastSpell: &proto.APLActionCastSpell{
				SpellId: &proto.ActionID{RawId: &proto.ActionID_SpellId{SpellId: 48461}}, // Wrath
			}}},
		}}},
	}
	raid := func(players ...*proto.Player) *proto.Raid {
		return &proto.Raid{Parties: []*proto.Party{{Players: players}}}
	}
	intellect := func(raid *proto.Raid, slot int) float64 {
		result := core.ComputeStats(&proto.ComputeStatsRequest{Raid: raid, Encounter: STEncounter})
		if result.ErrorResult != "" {
			t.Fatal(result.ErrorResult)
		}
		return result.RaidStats.Parties[0].Players[slot].FinalStats.Stats[stats.Intellect]
	}

	if alone, afterGap := intellect(raid(druid), 0), intellect(raid(&proto.Player{}, druid), 1); afterGap != alone {
		t.Errorf("intellect after an empty slot = %v, want %v with Blessing of Kings", afterGap, alone)
	}

	result := core.RunRaidSim(&proto.RaidSimRequest{Raid: raid(&proto.Player{}, druid), Encounter: STEncounter, SimOptions: SimOptions})
	if result.ErrorResult != "" {
		t.Fatal(result.ErrorResult)
	}
	if dps := result.RaidMetrics.Parties[0].Players[1].Dps.Avg; dps <= 0 {
		t.Errorf("the druid after an empty slot did %v dps, want its Wrath APL to run", dps)
	}
}

func TestBasicRaid(t *testing.T) {
	t.Skip()
	rsr := &proto.RaidSimRequest{
		Raid:       BasicRaid,
		Encounter:  STEncounter,
		SimOptions: SimOptions,
	}

	core.RaidSimTest("P1 ST", t, rsr, 6323.79)
}

// To quickly debug raid sim issues, uncomment this test and copy in a request string.
/*
func testRaidString(t *testing.T, raidString string) {
	rsr := &proto.RaidSimRequest{}

	data := []byte(raidString)
	if err := protojson.Unmarshal(data, rsr); err != nil {
		panic(err)
	}

	core.RunRaidSim(rsr)
	//core.RaidSimTest("Fixed Raid", t, rsr, 10000.00)
}

func TestFixedRaid(t *testing.T) {
 	testRaidString(t, `
 	`)
}
*/
