package core

import (
	"strconv"
	"time"
)

// delayedPeriodicWindow is the caster's own periodic event-clock boundary that
// Unit::CastDelayedSpellWithPeriodicAmount queues onto (EventProcessor::CalculateQueueTime(400)).
const delayedPeriodicWindow = time.Millisecond * 400

// DelayedPeriodicApplier reapplies a periodic snapshot on the delay the live server imposes when a
// caster refreshes a "munching" DoT on a different unit (MunchingBlizzlike.Enabled, live on;
// Unit::CastDelayedSpellWithPeriodicAmount): rather than landing immediately, the application queues
// on the caster's own event list for its next 400 ms boundary (EventProcessor::CalculateQueueTime),
// 1-400 ms out, then on the sim's own server tick. That boundary comes from a clock private to the
// caster (m_time, counting from the unit's creation) whose alignment the sim has no way to know, so
// each caster draws its own phase once per iteration; every proc from that caster this iteration then
// lands on the same lattice, so two procs inside one window both read the same stale dot and the
// later application overwrites the earlier one's snapshot, matching the server's munching.
type DelayedPeriodicApplier struct {
	caster *Unit
}

// NewDelayedPeriodicApplier scopes the delay to caster. It carries no state of its own (the phase is
// derived fresh each call from the caster and the current iteration), so it's fine to construct one
// per proc instead of holding onto it.
func NewDelayedPeriodicApplier(caster *Unit) *DelayedPeriodicApplier {
	return &DelayedPeriodicApplier{caster: caster}
}

// phase is this iteration's draw of the caster's clock alignment, stable for every call from this
// caster this iteration. It's derived from the caster and the iteration's own seed rather than
// through RandomFloat, so it never consumes from (or gets perturbed by) any other roll.
func (dpa *DelayedPeriodicApplier) phase(sim *Simulation) time.Duration {
	seed := hash(dpa.caster.Label + "|" + strconv.FormatInt(sim.rand.GetSeed(), 16))
	return time.Duration(NewSplitMix(uint64(seed)).NextFloat64() * float64(delayedPeriodicWindow))
}

// Delay returns how long from sim.CurrentTime a proc happening right now would queue for.
func (dpa *DelayedPeriodicApplier) Delay(sim *Simulation) time.Duration {
	untilBoundary := delayedPeriodicWindow - ((sim.CurrentTime + dpa.phase(sim)) % delayedPeriodicWindow)
	return sim.NextServerTick(sim.CurrentTime+untilBoundary) - sim.CurrentTime
}

// Apply runs onApply immediately when target is the caster itself
// (Unit::CastDelayedSpellWithPeriodicAmount: `this == caster` skips the queue), or queues it for
// Delay otherwise. Read whatever the application needs (the dot's outstanding damage included)
// before calling Apply: the server does that at the proc, not when the queued event fires.
func (dpa *DelayedPeriodicApplier) Apply(sim *Simulation, target *Unit, onApply func(sim *Simulation)) {
	if target == dpa.caster {
		onApply(sim)
		return
	}
	sim.AddPendingAction(&PendingAction{
		NextActionAt: sim.CurrentTime + dpa.Delay(sim),
		OnAction:     onApply,
	})
}
