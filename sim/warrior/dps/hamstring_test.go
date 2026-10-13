package dps

import (
	"math"
	"testing"
	"time"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
)

// Hamstring snares an add for 15 s. The snare-immune boss is immune to the whole spell, which still takes the
// GCD and the part of the rage a miss keeps.
func TestHamstringSnaresWhatTakesSnares(t *testing.T) {
	encounter := core.MakeSingleTargetEncounter(0)
	encounter.Targets = append(encounter.Targets, &proto.Target{Level: 80, MobType: proto.MobType_MobTypeHumanoid})
	sim := core.NewSim(&proto.RaidSimRequest{
		Raid: core.SinglePlayerRaidProto(&proto.Player{
			Race:          proto.Race_RaceOrc,
			Class:         proto.Class_ClassWarrior,
			TalentsString: ArmsTalents,
			Equipment:     core.GetGearSet("../../../ui/warrior/gear_sets", "p1_arms").GearSet,
			Consumes:      &proto.Consumes{},
			Spec:          PlayerOptionsArms,
			Rotation:      core.APLRotationFromJsonString(`{"type": "TypeAPL", "priorityList": []}`),
		}, &proto.PartyBuffs{}, &proto.RaidBuffs{}, &proto.Debuffs{}),
		Encounter:  encounter,
		SimOptions: &proto.SimOptions{Iterations: 1, IsTest: true, RandomSeed: 101},
	})
	sim.Reset()
	war := sim.Raid.Parties[0].Players[0].(*DpsWarrior)
	boss, add := sim.GetTargetUnit(0), sim.GetTargetUnit(1)
	if !boss.ImmuneToMechanic(core.MechanicSnare) || add.ImmuneToMechanic(core.MechanicSnare) {
		t.Fatalf("snare immune: boss %v, add %v", boss.ImmuneToMechanic(core.MechanicSnare), add.ImmuneToMechanic(core.MechanicSnare))
	}
	advance := func(by time.Duration) {
		for until := sim.CurrentTime + by; sim.CurrentTime < until && !sim.Step(); {
		}
	}
	cost := 10 - float64(war.Talents.FocusedRage)
	// the idle rotation parks the GCD, so each forced cast frees it first

	war.AddRage(sim, 100-war.CurrentRage(), war.RageRefundMetrics)
	war.GCD.Reset()
	if !war.Hamstring.Cast(sim, boss) {
		t.Fatal("Hamstring didn't cast at the boss")
	}
	if boss.HasActiveAuraWithTag(core.SnareAuraTag) {
		t.Error("the boss is snared")
	}
	// a miss takes irand(0, cost / 4) in tenths
	if got := 100 - war.CurrentRage(); got < 0 || got > cost/4+1e-9 {
		t.Errorf("Hamstring at the immune boss took %v rage, want 0 to %v", got, cost/4)
	}
	if war.GCD.IsReady(sim) {
		t.Error("Hamstring at the immune boss took no GCD")
	}

	// dodges and misses happen, so cast until it lands
	for try := 0; try < 10 && !add.HasActiveAuraWithTag(core.SnareAuraTag); try++ {
		advance(2 * time.Second)
		war.AddRage(sim, 100-war.CurrentRage(), war.RageRefundMetrics)
		before := war.CurrentRage()
		war.GCD.Reset()
		if !war.Hamstring.Cast(sim, add) {
			t.Fatal("Hamstring didn't cast at the add")
		}
		if add.HasActiveAuraWithTag(core.SnareAuraTag) {
			if got := before - war.CurrentRage(); math.Abs(got-cost) > 1e-9 {
				t.Errorf("a landed Hamstring took %v rage, want %v", got, cost)
			}
		}
	}
	if !add.HasActiveAuraWithTag(core.SnareAuraTag) {
		t.Fatal("ten Hamstrings left the add unsnared")
	}
	advance(14 * time.Second)
	if !add.HasActiveAuraWithTag(core.SnareAuraTag) {
		t.Error("the snare ran out before its 15 s")
	}
	advance(2 * time.Second)
	if add.HasActiveAuraWithTag(core.SnareAuraTag) {
		t.Error("the snare outlasted its 15 s")
	}

	war.DefensiveStance.Cast(sim, nil)
	advance(2 * time.Second)
	war.AddRage(sim, 100-war.CurrentRage(), war.RageRefundMetrics)
	war.GCD.Reset()
	if war.Hamstring.Cast(sim, add) {
		t.Error("Hamstring cast in Defensive Stance")
	}
}
