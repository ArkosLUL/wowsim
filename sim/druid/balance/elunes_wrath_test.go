package balance

import (
	"testing"
	"time"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
)

// spell_proc 64823 has PROC_ATTR_REQ_SPELLMOD (2026_10_01_02), so Elune's Wrath goes with the Starfire whose
// cast time it took, one cast the moment it comes up included, and a Starfire already casting leaves it be.
func TestElunesWrathGoesWithTheStarfireItMadeInstant(t *testing.T) {
	player := nakedDruidWith(0, `{"type": "TypeAPL", "priorityList": []}`)
	player.Equipment.Items[proto.ItemSlot_ItemSlotShoulder] = &proto.ItemSpec{Id: 45352}
	player.Equipment.Items[proto.ItemSlot_ItemSlotChest] = &proto.ItemSpec{Id: 45354}
	player.Equipment.Items[proto.ItemSlot_ItemSlotHands] = &proto.ItemSpec{Id: 45351}
	player.Equipment.Items[proto.ItemSlot_ItemSlotLegs] = &proto.ItemSpec{Id: 45353}
	sim := core.NewSim(&proto.RaidSimRequest{
		Raid:       core.SinglePlayerRaidProto(player, nil, nil, nil),
		Encounter:  core.MakeSingleTargetEncounter(0),
		SimOptions: &proto.SimOptions{Iterations: 1, IsTest: true, RandomSeed: 101},
	})
	druid := sim.Raid.Parties[0].Players[0].(*BalanceDruid)
	elunesWrath := druid.MoonkinT84PCAura
	if elunesWrath == nil {
		t.Fatal("no Elune's Wrath, so no Nightsong 4 piece")
	}

	at := func(when time.Duration, do func(sim *core.Simulation)) {
		sim.AddPendingAction(&core.PendingAction{NextActionAt: when, Priority: core.ActionPriorityLow, OnAction: do})
	}
	cast := func(sim *core.Simulation) time.Duration {
		druid.GCD.Reset()
		if !druid.Starfire.Cast(sim, druid.CurrentTarget) {
			t.Fatalf("Starfire didn't cast at %s", sim.CurrentTime)
		}
		return druid.Starfire.CurCast.CastTime
	}

	sim.Reset()
	sim.PrePull()
	// just past a world tick, where the buff's countdown starts on that tick and its expiry rounds up to a
	// later one
	at(sim.NextWorldTick(time.Second)+time.Millisecond, func(sim *core.Simulation) {
		elunesWrath.Activate(sim)
		if castTime := cast(sim); castTime != 0 {
			t.Fatalf("Starfire under Elune's Wrath took %s", castTime)
		}
		if elunesWrath.IsActive() {
			t.Error("Elune's Wrath outlived the Starfire it made instant")
		}
	})
	// started between two world ticks, with the buff up before the next one, so the buff's countdown
	// starts on the tick before the cast began
	start := sim.NextWorldTick(5*time.Second) + time.Millisecond
	if sim.NextWorldTick(start) <= start+time.Millisecond {
		t.Fatalf("no room between %s and the next world tick", start)
	}
	checked := false
	at(start, func(sim *core.Simulation) {
		castTime := cast(sim)
		if castTime == 0 {
			t.Fatal("Starfire was instant without Elune's Wrath")
		}
		at(sim.CurrentTime+time.Millisecond, func(sim *core.Simulation) { elunesWrath.Activate(sim) })
		at(sim.CurrentTime+castTime+time.Millisecond, func(sim *core.Simulation) {
			checked = true
			if !elunesWrath.IsActive() {
				t.Error("a Starfire already casting when Elune's Wrath came up took it")
			}
		})
	})
	for !sim.Step() {
	}
	sim.Cleanup()

	if !checked {
		t.Fatal("the hardcast Starfire's check never ran")
	}
}
