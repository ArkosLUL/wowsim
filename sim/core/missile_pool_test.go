package core

import (
	"fmt"
	"slices"
	"testing"
	"time"
)

func newMissileTestSpell() *Spell {
	return &Spell{Unit: &Unit{DistanceFromTarget: 20}, MissileSpeed: 20} // lands after 1 s
}

// Landings keep the queue's order: after what's already due at that time and priority, before what's
// added later. One that ran gets reused, even from inside its own landing.
func TestWaitTravelTimeReusesLandings(t *testing.T) {
	sim := newPendingQueueTestSim()
	spell := newMissileTestSpell()
	var ran []string
	note := func(label string) func(*Simulation) {
		return func(sim *Simulation) { ran = append(ran, fmt.Sprintf("%s@%s", label, sim.CurrentTime)) }
	}

	StartDelayedAction(sim, DelayedActionOptions{DoAt: time.Second, OnAction: note("before")})
	spell.WaitTravelTime(sim, note("a"))
	spell.WaitTravelTime(sim, func(sim *Simulation) {
		note("b")(sim)
		spell.WaitTravelTime(sim, note("chained"))
	})
	StartDelayedAction(sim, DelayedActionOptions{DoAt: time.Second, OnAction: note("after")})
	for !sim.Step() {
	}

	want := []string{"before@1s", "a@1s", "b@1s", "after@1s", "chained@2s"}
	if !slices.Equal(ran, want) {
		t.Errorf("ran %v, want %v", ran, want)
	}
	if len(sim.landings) != 2 {
		t.Errorf("made %d landings for at most 2 in flight", len(sim.landings))
	}
}

// DealDamageAfterTravel deals the result when the missile lands, in the same queue order as a
// WaitTravelTime landing, and frees the landing and the result.
func TestDealDamageAfterTravel(t *testing.T) {
	sim := SetupFakeSim()
	spell := sim.Raid.Parties[0].Players[0].(*FakeAgent).Spell
	spell.MissileSpeed = 20
	target := sim.GetTargetUnit(0)
	dealt := func() float64 { return spell.SpellMetrics[target.UnitIndex].TotalDamage }

	var before, after float64 = -1, -1
	landsAt := sim.CurrentTime + spell.TravelTime()
	spell.WaitTravelTime(sim, func(*Simulation) { before = dealt() })
	spell.DealDamageAfterTravel(sim, spell.CalcDamage(sim, target, 100, spell.OutcomeAlwaysHit))
	spell.WaitTravelTime(sim, func(sim *Simulation) {
		after = dealt()
		if sim.CurrentTime != landsAt {
			t.Errorf("landed at %s, want %s", sim.CurrentTime, landsAt)
		}
	})
	if dealt() != 0 {
		t.Fatalf("dealt the damage before the missile landed")
	}
	for after < 0 && !sim.Step() {
	}

	if before != 0 || after <= 0 {
		t.Errorf("damage dealt before and after the landing: %v and %v, want 0 and more", before, after)
	}
	if spell.resultCache.inUse || len(sim.freeLandings) != len(sim.landings) {
		t.Errorf("the landing kept its result or its slot in the pool")
	}
}

// A missile still in flight when the iteration ends never lands, and its landing is free again.
func TestLandingsFreeWhenTheIterationEnds(t *testing.T) {
	sim := newPendingQueueTestSim()
	spell := newMissileTestSpell()
	landed := false
	spell.WaitTravelTime(sim, func(*Simulation) { landed = true })

	sim.resetPendingActions()
	if len(sim.freeLandings) != len(sim.landings) || sim.landings[0].onLand != nil {
		t.Fatalf("the landing in flight is still taken after the reset")
	}
	spell.WaitTravelTime(sim, func(*Simulation) {})
	for !sim.Step() {
	}
	if landed {
		t.Errorf("last iteration's missile landed in this one")
	}
	if len(sim.landings) != 1 {
		t.Errorf("made %d landings, want the old one reused", len(sim.landings))
	}
}

// A spare goes back to its own spell once, whichever spell deals it, and comes back zeroed. The next
// iteration starts with every result free.
func TestSpellResultSpares(t *testing.T) {
	spell, other := &Spell{}, &Spell{}
	target := &Unit{}

	cached := spell.NewResult(target)
	spare := spell.NewResult(target)
	if spare == cached {
		t.Fatalf("handed out the cached result twice")
	}
	spare.ResistanceMultiplier, spare.PreOutcomeDamage = 0.5, 7
	other.DisposeResult(spare)
	other.DisposeResult(spare)
	spell.DisposeResult(cached)
	if len(spell.freeResults) != 1 || len(other.freeResults) != 0 {
		t.Fatalf("free spares: %d on the spell and %d on the other, want 1 and 0", len(spell.freeResults), len(other.freeResults))
	}

	spell.NewResult(target) // the cache again
	reused := spell.NewResult(target)
	if reused != spare {
		t.Errorf("made a spare with one free")
	}
	if reused.ResistanceMultiplier != 0 || reused.PreOutcomeDamage != 0 {
		t.Errorf("a reused spare kept %v and %v from its last use", reused.ResistanceMultiplier, reused.PreOutcomeDamage)
	}
	extra := spell.NewResult(target)

	// the cache and both spares are held, e.g. by missiles in flight, when the iteration ends
	spell.reset(nil)
	if got := spell.NewResult(target); got != cached {
		t.Errorf("the cached result stayed taken after the reset")
	}
	for range 2 {
		if got := spell.NewResult(target); got != spare && got != extra {
			t.Errorf("made a spare after the reset freed both")
		}
	}
	if len(spell.spareResults) != 2 {
		t.Errorf("made %d spares, want 2", len(spell.spareResults))
	}
}
