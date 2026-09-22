package core

import (
	"strconv"
	"time"
)

// EventProcessor::CalculateQueueTime(400), from Unit::CastDelayedSpellWithPeriodicAmount
const delayedPeriodicWindow = time.Millisecond * 400

// DelayedPeriodicApplier lands a dot refresh on the caster's next 400 ms event boundary, like the
// server's MunchingBlizzlike.Enabled. Two procs in one window land together, the later one wins.
type DelayedPeriodicApplier struct {
	caster *Unit

	cachedPhase time.Duration
	phaseSeed   int64
	hasPhase    bool
}

// Keep one per caster: it caches the phase per iteration.
func NewDelayedPeriodicApplier(caster *Unit) *DelayedPeriodicApplier {
	return &DelayedPeriodicApplier{caster: caster}
}

// The boundary follows the caster's own event clock, which the sim can't know, so each caster draws a
// phase per iteration. Hashed off the iteration's seed so it doesn't use up a roll.
func (dpa *DelayedPeriodicApplier) phase(sim *Simulation) time.Duration {
	seed := sim.rand.GetSeed()
	if !dpa.hasPhase || dpa.phaseSeed != seed {
		h := hash(dpa.caster.Label + "|" + strconv.FormatInt(seed, 16))
		dpa.cachedPhase = time.Duration(NewSplitMix(uint64(h)).NextFloat64() * float64(delayedPeriodicWindow))
		dpa.phaseSeed, dpa.hasPhase = seed, true
	}
	return dpa.cachedPhase
}

// Delay is how long a proc right now waits: to the boundary, then to the next server tick.
func (dpa *DelayedPeriodicApplier) Delay(sim *Simulation) time.Duration {
	untilBoundary := delayedPeriodicWindow - ((sim.CurrentTime + dpa.phase(sim)) % delayedPeriodicWindow)
	return sim.NextServerTick(sim.CurrentTime+untilBoundary) - sim.CurrentTime
}

// Apply queues onApply for Delay, except on a self-target, which skips the queue on the server too.
// Read the dot's outstanding damage before calling: the server reads it at the proc.
func (dpa *DelayedPeriodicApplier) Apply(sim *Simulation, target *Unit, onApply func(sim *Simulation)) {
	if target == dpa.caster {
		onApply(sim)
		return
	}
	sim.AddPendingAction(&PendingAction{
		// a nanosecond early, to beat a swing or the old dot's tick due on the same server tick: the
		// caster's events run first in Player::Update, and creatures' auras tick after players
		NextActionAt: sim.CurrentTime + dpa.Delay(sim) - time.Nanosecond,
		OnAction:     onApply,
	})
}
