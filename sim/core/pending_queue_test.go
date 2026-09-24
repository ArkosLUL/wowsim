package core

import (
	"math/rand"
	"slices"
	"testing"
	"time"
)

// queuedActions lists the queue as it's stored: sentinel first, next action last.
func (sim *Simulation) queuedActions() []*PendingAction {
	pas := make([]*PendingAction, len(sim.pendingActions))
	for i, e := range sim.pendingActions {
		pas[i] = sim.pendingSlots[e.slot]
	}
	return pas
}

func (sim *Simulation) nextPendingAction() *PendingAction {
	return sim.pendingSlots[sim.pendingActions[len(sim.pendingActions)-1].slot]
}

func newPendingQueueTestSim() *Simulation {
	sim := newOutcomeSim()
	sim.reset()
	sim.endOfCombatDuration = NeverExpires - 1
	return sim
}

// Checks every action the queue runs against a plain list: earliest NextActionAt first, then higher
// Priority, then the one added first, skipping cancelled ones. The actions add, cancel and re-add
// others while it runs.
func TestPendingActionsRunInOrder(t *testing.T) {
	type added struct {
		pa  *PendingAction
		seq int
	}

	for seed := int64(1); seed <= 50; seed++ {
		rng := rand.New(rand.NewSource(seed))
		sim := newPendingQueueTestSim()

		var queued []added
		seq, ran := 0, 0
		add := func(pa *PendingAction) {
			queued = append(queued, added{pa, seq})
			seq++
			sim.AddPendingAction(pa)
		}
		want := func() int {
			best := -1
			for i, q := range queued {
				if q.pa.cancelled {
					continue
				}
				if b := queued[max(best, 0)]; best < 0 || q.pa.NextActionAt < b.pa.NextActionAt ||
					q.pa.NextActionAt == b.pa.NextActionAt && (q.pa.Priority > b.pa.Priority || q.pa.Priority == b.pa.Priority && q.seq < b.seq) {
					best = i
				}
			}
			return best
		}
		schedule := func(pa *PendingAction) {
			pa.NextActionAt = sim.CurrentTime + time.Duration(rng.Intn(4))*time.Millisecond
			pa.Priority = ActionPriority(rng.Intn(4) - 1)
			add(pa)
		}

		var newAction func() *PendingAction
		newAction = func() *PendingAction {
			pa := &PendingAction{}
			pa.OnAction = func(sim *Simulation) {
				i := want()
				if i < 0 || queued[i].pa != pa {
					t.Fatalf("seed %d: ran an action at %s out of order", seed, sim.CurrentTime)
				}
				if sim.CurrentTime != pa.NextActionAt {
					t.Fatalf("seed %d: action due at %s ran at %s", seed, pa.NextActionAt, sim.CurrentTime)
				}
				queued = slices.Delete(queued, i, i+1)
				ran++

				if seq > 400 {
					return
				}
				for n := rng.Intn(3); n > 0; n-- {
					schedule(newAction())
				}
				if len(queued) > 0 && rng.Intn(3) == 0 {
					queued[rng.Intn(len(queued))].pa.Cancel(sim)
				}
				if rng.Intn(3) == 0 {
					schedule(pa)
				}
			}
			return pa
		}

		for range 20 {
			schedule(newAction())
		}
		for !sim.Step() {
		}

		if i := want(); i >= 0 {
			t.Fatalf("seed %d: an action due at %s never ran", seed, queued[i].pa.NextActionAt)
		}
		if ran < 100 {
			t.Fatalf("seed %d: only %d actions ran", seed, ran)
		}
	}
}

// Moving a queued GCD reuses its action, and the entry left behind at the old time does nothing.
func TestSetGCDTimerMovesTheQueuedAction(t *testing.T) {
	sim := newPendingQueueTestSim()
	var ran []time.Duration
	unit := &Unit{}
	unit.GCD = unit.NewTimer()
	unit.gcdAction = &PendingAction{Priority: ActionPriorityGCD, OnAction: func(sim *Simulation) {
		ran = append(ran, sim.CurrentTime)
	}}

	unit.SetGCDTimer(sim, 5*time.Millisecond)
	queued := unit.gcdAction
	unit.SetGCDTimer(sim, 9*time.Millisecond)
	unit.SetGCDTimer(sim, 7*time.Millisecond)
	if unit.gcdAction != queued {
		t.Errorf("SetGCDTimer made a new action for one still queued")
	}
	for !sim.Step() {
	}

	if want := []time.Duration{7 * time.Millisecond}; !slices.Equal(ran, want) {
		t.Errorf("the GCD ran at %v, want %v", ran, want)
	}
}

// Moving the GCD while Step advances time to its popped action cancels that action, as it always
// did: it mustn't run then and again at the new time.
func TestSetGCDTimerWhileItsActionWaitsToRun(t *testing.T) {
	sim := newPendingQueueTestSim()
	var ran []time.Duration
	unit := &Unit{}
	unit.GCD = unit.NewTimer()
	unit.gcdAction = &PendingAction{Priority: ActionPriorityGCD, OnAction: func(sim *Simulation) {
		ran = append(ran, sim.CurrentTime)
	}}

	unit.SetGCDTimer(sim, 5*time.Millisecond)
	unit.SetGCDTimer(sim, 5*time.Millisecond) // requeued in place, so it holds a slot
	moved := false
	// the zero-length test encounter enters its execute phases on the first advance
	sim.RegisterExecutePhaseCallback(func(sim *Simulation, _ int32) {
		if !moved {
			moved = true
			unit.SetGCDTimer(sim, 9*time.Millisecond)
		}
	})
	for !sim.Step() {
	}

	if want := []time.Duration{9 * time.Millisecond}; !moved || !slices.Equal(ran, want) {
		t.Errorf("the GCD ran at %v, want %v", ran, want)
	}
}

func TestDetachPendingActionOnlyTakesItsOwnSlot(t *testing.T) {
	sim := newPendingQueueTestSim()
	noop := func(*Simulation) {}

	withCleanUp := &PendingAction{NextActionAt: time.Millisecond, OnAction: noop, CleanUp: noop}
	sim.AddPendingAction(withCleanUp)
	if sim.detachPendingAction(withCleanUp) {
		t.Errorf("detached an action with a CleanUp to run")
	}

	neverAdded := &PendingAction{OnAction: noop}
	if sim.detachPendingAction(neverAdded) {
		t.Errorf("detached an action that was never queued")
	}

	// requeued after it ran, an action keeps its slot
	requeued := &PendingAction{OnAction: noop}
	sim.AddPendingAction(requeued)
	slot, slots := requeued.slot, len(sim.pendingSlots)
	sim.Step()
	sim.AddPendingAction(requeued)
	if requeued.slot != slot || len(sim.pendingSlots) != slots {
		t.Errorf("a requeued action took a new slot")
	}

	// the next iteration hands its slot to another action
	sim.resetPendingActions()
	for range 3 {
		sim.AddPendingAction(&PendingAction{NextActionAt: 2 * time.Millisecond, OnAction: noop})
	}
	if sim.pendingSlots[requeued.slot] == requeued {
		t.Fatalf("the slot still holds the old action")
	}
	if sim.detachPendingAction(requeued) {
		t.Errorf("detached another action's slot")
	}
	if slices.Contains(sim.queuedActions(), detachedPendingAction) {
		t.Errorf("a failed detach changed the queue")
	}
}

// Cleanup runs the CleanUp of each action still queued, the one due last first.
func TestPendingActionsCleanUpInQueueOrder(t *testing.T) {
	sim := newPendingQueueTestSim()

	var got []int
	for i, at := range []time.Duration{5, 1, 3, 3, 9} {
		sim.AddPendingAction(&PendingAction{
			NextActionAt: at * time.Millisecond,
			OnAction:     func(*Simulation) {},
			CleanUp:      func(*Simulation) { got = append(got, i) },
		})
	}
	sim.Step()
	sim.Cleanup()

	if want := []int{4, 0, 3, 2}; !slices.Equal(got, want) {
		t.Errorf("CleanUp order = %v, want %v", got, want)
	}
}
