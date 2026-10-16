package mage

import (
	"math"
	"testing"
	"time"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
	googleProto "google.golang.org/protobuf/proto"
)

// Fire Blast's range and the chase's both run between the two combat reaches, so a target whose reach is 5 yd
// past the boss dummy's 2.5 moves every one of them out by 5: from 28 yd the images cast and close in as they
// would from 23 against the dummy.
func TestMirrorImageRangesMoveWithTheTargetsReach(t *testing.T) {
	run := func(yards, reach float64) (bolts []float64, blasts int32) {
		player := procMage("", "p5_bis_arcane", `{"type": "TypeAPL", "priorityList": []}`, PlayerOptionsArcane)
		player.DistanceFromTarget = yards
		encounter := core.MakeSingleTargetEncounter(0)
		// the default target is one shared proto
		encounter.Targets[0] = googleProto.Clone(encounter.Targets[0]).(*proto.Target)
		encounter.Targets[0].CombatReach = reach
		sim := core.NewSim(&proto.RaidSimRequest{
			Raid:       core.SinglePlayerRaidProto(player, nil, nil, nil),
			Encounter:  core.OnRegularGrid(encounter, 10.27),
			SimOptions: &proto.SimOptions{Iterations: 1, IsTest: true, RandomSeed: 101},
		})
		sim.Reset()
		image := sim.Raid.Parties[0].Players[0].(*Mage).mirrorImage
		effects := image.Frostbolt.ApplyEffects
		image.Frostbolt.ApplyEffects = func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
			bolts = append(bolts, image.DistanceFromTarget)
			effects(sim, target, spell)
		}
		sim.PrePull()
		for sim.CurrentTime < 2*time.Second && !sim.Step() {
		}
		image.summon(sim)
		for image.IsEnabled() && !sim.Step() {
		}
		blasts = image.Fireblast.SpellMetrics[sim.GetTargetUnit(0).UnitIndex].Casts
		sim.Cleanup()
		return bolts, blasts
	}

	dummyBolts, dummyBlasts := run(23, 0)
	bolts, blasts := run(28, 7.5)
	if blasts != dummyBlasts || len(bolts) != len(dummyBolts) {
		t.Fatalf("against a 7.5 yd reach from 28 yd: %d Fire Blasts, Frostbolts from %.2f; against the dummy from 23: %d, %.2f",
			blasts, bolts, dummyBlasts, dummyBolts)
	}
	for i := range bolts {
		if math.Abs(bolts[i]-(dummyBolts[i]+5)) > 1e-9 {
			t.Errorf("Frostbolt %d went off from %.4f yd against the 7.5 yd reach, want 5 past the dummy's %.4f",
				i+1, bolts[i], dummyBolts[i])
		}
	}
	if dummyBlasts != 3 || dummyBolts[len(dummyBolts)-1] != mirrorImageChaseRange {
		t.Errorf("against the dummy from 23 yd: %d Fire Blasts, last Frostbolt from %.2f, want 3 and the chase's range",
			dummyBlasts, dummyBolts[len(dummyBolts)-1])
	}
}
