package core

import (
	"testing"
	"time"
)

// A cast lands once on each unit it reaches, however many of its parts reach it; a miss doesn't land, nor
// does a plain tick. The next cast lands again, and a new iteration forgets the last landing.
func TestCastLandsOnceATarget(t *testing.T) {
	var spell *Spell
	var landed []*Unit
	sim, a := newTimingTestSim(t, 10, func(a *timingTestAgent) {
		spell = a.RegisterSpell(SpellConfig{
			ActionID:         ActionID{SpellID: testSpellNoServerData},
			SpellSchool:      SpellSchoolHoly,
			ProcMask:         ProcMaskSpellHealing,
			DamageMultiplier: 1,
			ThreatMultiplier: 1,
		})
		a.RegisterOnCastLanded(func(_ *Simulation, s *Spell, target *Unit) {
			if s == spell {
				landed = append(landed, target)
			}
		})
	})
	self, enemy := &a.Unit, a.CurrentTarget
	expect := func(label string, want ...*Unit) {
		t.Helper()
		if len(landed) != len(want) {
			t.Fatalf("%s: landed on %d units, want %d", label, len(landed), len(want))
		}
		for i := range want {
			if landed[i] != want[i] {
				t.Fatalf("%s: landing %d on %s, want %s", label, i, landed[i].Label, want[i].Label)
			}
		}
	}

	spell.CalcAndDealHealing(sim, self, 100, spell.OutcomeHealing)
	spell.LandOn(sim, self)
	spell.CalcAndDealOutcome(sim, enemy, spell.OutcomeAlwaysMiss)
	expect("a heal with a hot, and a miss", self)

	result := spell.CalcHealing(sim, self, 100, spell.OutcomeHealing)
	spell.DealPeriodicHealing(sim, result)
	expect("a tick", self)

	runUntil(sim, time.Second)
	spell.CalcAndDealOutcome(sim, enemy, spell.OutcomeAlwaysHit)
	spell.LandOn(sim, self)
	expect("the next cast", self, enemy, self)

	// each iteration starts at the same time
	landed = nil
	sim.reset()
	spell.LandOn(sim, self)
	sim.reset()
	spell.LandOn(sim, self)
	expect("two iterations", self, self)
}
