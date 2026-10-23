package core

import (
	"slices"
	"testing"
	"time"
)

// A hit taken stand-in fires every 60 / rate seconds from its start, stops when told to and starts over from
// the next start; a rate of 0 never fires.
func TestHitTakenStandIn(t *testing.T) {
	sim := newPendingQueueTestSim()
	var hits []time.Duration
	onHit := func(sim *Simulation) { hits = append(hits, sim.CurrentTime) }
	// a marker at the time keeps the clock moving when nothing else is queued
	runTo := func(at time.Duration) {
		StartDelayedAction(sim, DelayedActionOptions{DoAt: at, OnAction: func(*Simulation) {}})
		for sim.CurrentTime < at && !sim.Step() {
		}
	}

	standIn := NewHitTakenStandIn(12, onHit)
	standIn.Start(sim)
	runTo(11 * time.Second)
	if want := []time.Duration{5 * time.Second, 10 * time.Second}; !slices.Equal(hits, want) {
		t.Errorf("12 a minute hit at %v, want %v", hits, want)
	}

	standIn.Stop(sim)
	runTo(30 * time.Second)
	if len(hits) != 2 {
		t.Errorf("hit %d times by 30 s, want none after Stop", len(hits))
	}

	standIn.Start(sim)
	runTo(34 * time.Second)
	if len(hits) != 2 {
		t.Errorf("hit %d times 4 s after a restart, want the first a period in", len(hits))
	}
	runTo(36 * time.Second)
	if len(hits) != 3 || hits[2] != 35*time.Second {
		t.Errorf("hits %v, want a third at 35 s", hits)
	}

	// starting a running stand-in restarts its clock, with no second one beside it
	runTo(38 * time.Second)
	standIn.Start(sim)
	runTo(46 * time.Second)
	if want := []time.Duration{5 * time.Second, 10 * time.Second, 35 * time.Second, 43 * time.Second}; !slices.Equal(hits, want) {
		t.Errorf("hits %v after a restart at 38 s, want %v", hits, want)
	}
	standIn.Stop(sim)

	hits = hits[:0]
	idle := NewHitTakenStandIn(0, onHit)
	idle.Start(sim)
	runTo(120 * time.Second)
	if len(hits) != 0 {
		t.Errorf("a rate of 0 hit %d times", len(hits))
	}
}
