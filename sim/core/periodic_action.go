package core

import (
	"time"
)

type DelayedActionOptions struct {
	// When the action should be performed.
	DoAt time.Duration

	Priority ActionPriority

	OnAction func(*Simulation)
	CleanUp  func(*Simulation)
}

func NewDelayedAction(sim *Simulation, options DelayedActionOptions) *PendingAction {
	if options.OnAction == nil {
		panic("NewDelayedAction: OnAction must not be nil")
	}

	return &PendingAction{
		NextActionAt: options.DoAt,
		Priority:     options.Priority,
		OnAction:     options.OnAction,
		CleanUp:      options.CleanUp,
	}
}

// Convenience for immediately creating and starting a delayed action.
func StartDelayedAction(sim *Simulation, options DelayedActionOptions) *PendingAction {
	pa := NewDelayedAction(sim, options)
	sim.AddPendingAction(pa)
	return pa
}

type PeriodicActionOptions struct {
	// How often the action should be performed.
	Period time.Duration

	// Number of times to perform the action before stopping.
	// 0 indicates a permanent periodic action.
	NumTicks int

	// Whether the first tick should happen immediately. If false, first tick will
	// wait for Period.
	TickImmediately bool

	// Owner makes the action one of that unit's periodic aura ticks (Aura::Update): it falls on the
	// first update of the unit's after each period, with the remainder carried, comes ahead of that
	// update's expiries if the unit is a player, and an aura it puts on counts from that update.
	Owner *Unit

	// FirstTickAt is when an Owner action ticks first, instead of a period from now.
	FirstTickAt time.Duration

	Priority ActionPriority

	OnAction func(*Simulation)
	CleanUp  func(*Simulation)
}

func NewPeriodicAction(sim *Simulation, options PeriodicActionOptions) *PendingAction {
	if options.OnAction == nil {
		panic("NewPeriodicAction: OnAction must not be nil")
	}

	pa := &PendingAction{
		NextActionAt: sim.CurrentTime + options.Period,
		Priority:     options.Priority,
		CleanUp:      options.CleanUp,
	}

	tickIndex := 0

	// an owner's ticks snap to its updates, the remainder carried like Aura::Update's periodic timer
	owner := options.Owner
	nominal := pa.NextActionAt
	if owner != nil {
		if options.FirstTickAt != 0 {
			nominal = options.FirstTickAt
		}
		pa.NextActionAt = sim.nextUpdate(owner, nominal)
		if owner.Type == PlayerUnit {
			pa.playerEvent = true
			pa.Priority = ActionPriorityEvent
		}
	}

	pa.OnAction = func(sim *Simulation) {
		if owner != nil {
			outer := sim.updating
			sim.updating = owner
			options.OnAction(sim)
			sim.updating = outer
		} else {
			options.OnAction(sim)
		}
		tickIndex++

		if options.NumTicks == 0 || tickIndex < options.NumTicks {
			// Refresh action.
			if owner != nil {
				nominal += options.Period
				pa.NextActionAt = sim.nextUpdate(owner, nominal)
			} else {
				pa.NextActionAt = sim.CurrentTime + options.Period
			}
			sim.AddPendingAction(pa)
		} else {
			pa.Cancel(sim)
		}
	}

	if options.TickImmediately {
		// t = 0 might be during reset, so put it in the actions queue instead of
		// invoking the callback directly.
		if sim.CurrentTime == 0 {
			pa.NextActionAt = 0
		} else {
			options.OnAction(sim)
			tickIndex++
			if options.NumTicks == 1 {
				pa.Cancel(sim)
			}
		}
	}

	return pa
}

// Convenience for immediately creating and starting a periodic action.
func StartPeriodicAction(sim *Simulation, options PeriodicActionOptions) *PendingAction {
	pa := NewPeriodicAction(sim, options)
	sim.AddPendingAction(pa)
	return pa
}
