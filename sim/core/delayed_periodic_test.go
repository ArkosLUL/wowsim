package core

import (
	"testing"
	"time"
)

func newDelayedPeriodicTestSim(seed int64) *Simulation {
	sim := newOutcomeSim()
	sim.rand.Seed(seed)
	sim.serverTickInterval = 0
	sim.pendingActions = []*PendingAction{sentinelPendingAction}
	return sim
}

// Sweeping CurrentTime across a couple of 400 ms cycles should hit every point on the lattice, since
// a proc can land anywhere the server's own boundary falls relative to it.
func TestDelayedPeriodicApplierDelayRange(t *testing.T) {
	sim := newDelayedPeriodicTestSim(1)
	dpa := NewDelayedPeriodicApplier(&Unit{Label: "Caster"})

	min, max := delayedPeriodicWindow, time.Duration(0)
	for ms := 0; ms < int(2*delayedPeriodicWindow/time.Millisecond); ms++ {
		sim.CurrentTime = time.Duration(ms) * time.Millisecond
		d := dpa.Delay(sim)
		if d <= 0 || d > delayedPeriodicWindow {
			t.Fatalf("CurrentTime %s: delay = %s, want (0, %s]", sim.CurrentTime, d, delayedPeriodicWindow)
		}
		if d < min {
			min = d
		}
		if d > max {
			max = d
		}
	}
	// The phase itself falls at a sub-millisecond point, so a 1 ms sweep only gets close to the
	// two ends of the (0, 400 ms] range, not onto them exactly.
	if min > time.Millisecond {
		t.Errorf("min delay over a full sweep = %s, want close to %s", min, time.Millisecond)
	}
	if max < delayedPeriodicWindow-time.Millisecond {
		t.Errorf("max delay over a full sweep = %s, want close to %s", max, delayedPeriodicWindow)
	}
}

// The phase has to be stable for the whole iteration (so consecutive procs land on the same
// lattice), differ between casters (each unit's own clock), and move on to a new draw once the sim
// reseeds for the next iteration.
func TestDelayedPeriodicApplierPhasePerIterationPerCaster(t *testing.T) {
	sim := newDelayedPeriodicTestSim(7)
	warrior := NewDelayedPeriodicApplier(&Unit{Label: "Warrior"})
	paladin := NewDelayedPeriodicApplier(&Unit{Label: "Paladin"})

	sim.CurrentTime = 12345 * time.Millisecond
	warriorDelay := warrior.Delay(sim)
	if again := warrior.Delay(sim); again != warriorDelay {
		t.Errorf("same caster, same iteration: delay changed from %s to %s", warriorDelay, again)
	}
	if paladinDelay := paladin.Delay(sim); paladinDelay == warriorDelay {
		t.Errorf("two different casters drew the same phase (delay %s both): want independent clocks", warriorDelay)
	}

	sim.rand.Seed(8) // a new iteration reseeds sim.rand before any proc runs
	if next := warrior.Delay(sim); next == warriorDelay {
		t.Errorf("delay after reseeding for a new iteration = %s, same as the old iteration's %s", next, warriorDelay)
	}
}

// The server tick lattice (Map::Update) still rounds the landing up, on top of the caster's own
// 400 ms boundary.
func TestDelayedPeriodicApplierRoundsToServerTick(t *testing.T) {
	sim := newDelayedPeriodicTestSim(3)
	sim.serverTickInterval = 100 * time.Millisecond
	sim.serverTickPhase = 0
	dpa := NewDelayedPeriodicApplier(&Unit{Label: "Caster"})

	for ms := 0; ms < 500; ms += 17 {
		sim.CurrentTime = time.Duration(ms) * time.Millisecond
		landing := sim.CurrentTime + dpa.Delay(sim)
		if landing%sim.serverTickInterval != 0 {
			t.Fatalf("CurrentTime %s: landing %s isn't on the %s server tick lattice", sim.CurrentTime, landing, sim.serverTickInterval)
		}
	}
}

// A caster applying to itself skips the queue outright (Unit::CastDelayedSpellWithPeriodicAmount:
// `this == caster`).
func TestDelayedPeriodicApplierSkipsQueueForSelfTarget(t *testing.T) {
	sim := newDelayedPeriodicTestSim(1)
	caster := &Unit{Label: "Caster"}
	dpa := NewDelayedPeriodicApplier(caster)

	ran := false
	dpa.Apply(sim, caster, func(sim *Simulation) { ran = true })

	if !ran {
		t.Fatal("onApply didn't run immediately for a self-targeted apply")
	}
	if len(sim.pendingActions) != 1 {
		t.Errorf("pendingActions = %d, want just the sentinel (nothing queued)", len(sim.pendingActions))
	}
}

// Two procs from the same caster landing in the same 400 ms window queue for the exact same time,
// so whichever runs last (the later proc, added to the queue second) overwrites the first's
// snapshot: the munching the server's own AuraMunchingQueue produces.
func TestDelayedPeriodicApplierMunchesWithinOneWindow(t *testing.T) {
	sim := newDelayedPeriodicTestSim(9)
	caster := &Unit{Label: "Caster"}
	target := &Unit{Label: "Target"}
	dpa := NewDelayedPeriodicApplier(caster)

	// Land both procs just after the same boundary, well inside the 400 ms window that follows it.
	boundary := (delayedPeriodicWindow - dpa.phase(sim)) % delayedPeriodicWindow

	var outstanding int
	sim.CurrentTime = boundary + 10*time.Millisecond
	dpa.Apply(sim, target, func(sim *Simulation) { outstanding = 1 })
	firstProc := sim.pendingActions[len(sim.pendingActions)-1]

	sim.CurrentTime = boundary + 60*time.Millisecond
	dpa.Apply(sim, target, func(sim *Simulation) { outstanding = 2 })
	var secondProc *PendingAction
	for _, pa := range sim.pendingActions[1:] {
		if pa != firstProc {
			secondProc = pa
		}
	}

	if firstProc.NextActionAt != secondProc.NextActionAt {
		t.Fatalf("procs 50ms apart inside one window landed at %s and %s, want the same time", firstProc.NextActionAt, secondProc.NextActionAt)
	}

	// AddPendingAction keeps first-in-first-out order for ties, so the first proc's write lands
	// before the second's, the way the server's own event queue would run them.
	firstProc.OnAction(sim)
	secondProc.OnAction(sim)
	if outstanding != 2 {
		t.Errorf("outstanding = %d after both applications ran, want 2 (the later proc munching the earlier one)", outstanding)
	}
}
