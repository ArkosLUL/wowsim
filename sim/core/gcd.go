package core

import (
	"time"
)

// A player's cast always lands through this, ahead of its auras on that tick. A creature's only when the
// hardcast and GCD actions happen at different times.
func (unit *Unit) newHardcastAction(sim *Simulation) {
	if unit.hardcastAction != nil && !unit.hardcastAction.consumed {
		unit.hardcastAction.Cancel(sim)
		unit.hardcastAction = nil
	}

	if unit.hardcastAction == nil {
		pa := &PendingAction{
			NextActionAt: unit.Hardcast.Expires,
			playerEvent:  unit.Type == PlayerUnit,
			OnAction: func(sim *Simulation) {
				if hc := &unit.Hardcast; hc.Expires != startingCDTime && hc.Expires <= sim.CurrentTime {
					hc.Expires = startingCDTime
					if hc.OnComplete != nil {
						hc.OnComplete(sim, hc.Target)
					}
				}
			},
		}
		if pa.playerEvent {
			pa.Priority = ActionPriorityEvent
		}
		unit.hardcastAction = pa
	} else {
		unit.hardcastAction.cancelled = false
		unit.hardcastAction.NextActionAt = unit.Hardcast.Expires
	}

	sim.AddPendingAction(unit.hardcastAction)
}

// landDueCast lands the unit's hardcast if it's due right now, ahead of the action queued for it, which
// then finds nothing to land. Reports whether it did.
func (unit *Unit) landDueCast(sim *Simulation) bool {
	hc := &unit.Hardcast
	if hc.Expires != sim.CurrentTime || hc.OnComplete == nil {
		return false
	}
	hc.Expires = startingCDTime
	hc.OnComplete(sim, hc.Target)
	return true
}

func (unit *Unit) NextGCDAt() time.Duration {
	return unit.gcdAction.NextActionAt
}

func (unit *Unit) SetGCDTimer(sim *Simulation, gcdReadyAt time.Duration) {
	if unit.gcdAction == nil {
		return
	}

	unit.GCD.Set(gcdReadyAt)

	if unit.gcdAction.consumed {
		unit.gcdAction.cancelled = false
		unit.gcdAction.NextActionAt = gcdReadyAt
	} else if sim.detachPendingAction(unit.gcdAction) {
		// what the branch below does, minus the allocation: the queued entry no longer points here
		unit.gcdAction.cancelled = false
		unit.gcdAction.NextActionAt = gcdReadyAt
		unit.gcdAction.Priority = ActionPriorityGCD
	} else {
		unit.gcdAction.Cancel(sim)
		oldAction := unit.gcdAction.OnAction
		unit.gcdAction = &PendingAction{
			NextActionAt: gcdReadyAt,
			Priority:     ActionPriorityGCD,
			OnAction:     oldAction,
		}
	}
	sim.AddPendingAction(unit.gcdAction)
}

// Call this to stop the GCD loop for a unit.
// This is mostly used for pets that get summoned / expire.
func (unit *Unit) CancelGCDTimer(sim *Simulation) {
	unit.gcdAction.Cancel(sim)
}

func (unit *Unit) WaitUntil(sim *Simulation, readyTime time.Duration) {
	if readyTime < sim.CurrentTime {
		panic(unit.Label + ": cannot wait negative time")
	}
	unit.SetGCDTimer(sim, readyTime)
	if sim.Log != nil && readyTime > sim.CurrentTime {
		unit.Log(sim, "Pausing GCD for %s due to rotation / CDs.", readyTime-sim.CurrentTime)
	}
}
