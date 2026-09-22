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

// The boundary itself, off whatever the caster's own clock reads as "now".
func (dpa *DelayedPeriodicApplier) window(sim *Simulation, now time.Duration) time.Duration {
	return delayedPeriodicWindow - ((now + dpa.phase(sim)) % delayedPeriodicWindow)
}

// Delay is how long a proc processed with this same update waits: a swing, a hardcast landing in
// _UpdateSpells, a missile's hit, a dot tick on the target, or an NPC/bot's own direct cast. All of
// these run after WorldObject::Update has already advanced the caster's own event clock for this
// pass, so the boundary drawn off "now" is already the landing time: the event fires on this same
// caster's very next event pass, not gated to the map's slower creature/visibility tick, so no
// further rounding applies.
func (dpa *DelayedPeriodicApplier) Delay(sim *Simulation) time.Duration {
	return dpa.window(sim, sim.CurrentTime)
}

// DelayFromInstantCast is Delay for a proc fed by a human player's instant cast. CMSG_CAST_SPELL is
// PROCESS_THREADSAFE, handled in Map::Update's session pass, one pass before this same update's
// Player::Update advances the caster's clock (WorldObject::Update, called from Unit::Update). The
// boundary is drawn off that stale clock, so it can already be behind by the time this update's
// advance catches up: it lands sooner than Delay's, in the same update at the earliest.
func (dpa *DelayedPeriodicApplier) DelayFromInstantCast(sim *Simulation) time.Duration {
	stale := sim.CurrentTime - sim.serverTickInterval
	if stale < 0 {
		stale = 0
	}
	if d := dpa.window(sim, stale) - sim.serverTickInterval; d > time.Nanosecond {
		return d
	}
	return 2 * time.Nanosecond
}

// Apply queues onApply for Delay, except on a self-target, which skips the queue on the server too.
// Read the dot's outstanding damage before calling: the server reads it at the proc.
func (dpa *DelayedPeriodicApplier) Apply(sim *Simulation, target *Unit, onApply func(sim *Simulation)) {
	dpa.queue(sim, target, dpa.Delay(sim), onApply)
}

// ApplyFromInstantCast is Apply, delayed by DelayFromInstantCast instead of Delay: for a proc fed by
// a human player's own instant cast rather than by something this update itself processed.
func (dpa *DelayedPeriodicApplier) ApplyFromInstantCast(sim *Simulation, target *Unit, onApply func(sim *Simulation)) {
	dpa.queue(sim, target, dpa.DelayFromInstantCast(sim), onApply)
}

func (dpa *DelayedPeriodicApplier) queue(sim *Simulation, target *Unit, delay time.Duration, onApply func(sim *Simulation)) {
	if target == dpa.caster {
		onApply(sim)
		return
	}
	sim.AddPendingAction(&PendingAction{
		// a nanosecond early, to beat a swing or the old dot's tick due on the same server tick: the
		// caster's events run first in Player::Update, and creatures' auras tick after players
		NextActionAt: sim.CurrentTime + delay - time.Nanosecond,
		OnAction:     onApply,
	})
}
