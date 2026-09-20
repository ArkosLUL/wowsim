package core

import (
	"strconv"
	"time"
)

type OnSnapshot func(sim *Simulation, target *Unit, dot *Dot, isRollover bool)
type OnTick func(sim *Simulation, target *Unit, dot *Dot)

// TickHaste is which haste shortens a dot's ticks, and whether the duration comes along, for a dot
// that AffectedByCastSpeed marks as hasted at all.
type TickHaste uint8

const (
	// SpellHasteScalesBoth is a channel: Spell::handle_immediate hastes the channel's duration and
	// AuraEffect::CalculatePeriodic its tick interval, so the tick count stays put.
	SpellHasteScalesBoth TickHaste = iota
	// SpellHasteAddsTicks is SPELL_ATTR5_SPELL_HASTE_AFFECTS_PERIODIC on a dot that isn't channeled:
	// only the tick interval shrinks, so more ticks fit the same duration.
	SpellHasteAddsTicks
	// MeleeHasteAddsTicks is SpellHasteAddsTicks off melee haste instead of spell haste.
	MeleeHasteAddsTicks
)

// SPELL_AURA_PERIODIC_DAMAGE and SPELL_AURA_PERIODIC_DAMAGE_PERCENT: the only aura types whose tick
// timer can survive a refresh. Anything else (a hot, a drain) restarts it however the spell stacks.
const (
	auraTypePeriodicDamage        = 3
	auraTypePeriodicDamagePercent = 89
)

type DotConfig struct {
	IsAOE    bool // Set to true for AOE dots (Blizzard, Hurricane, Consecrate, etc)
	SelfOnly bool // Set to true to only create the self-hot.

	// Optional, will default to the corresponding spell.
	Spell *Spell

	Aura Aura

	NumberOfTicks int32         // number of ticks over the whole duration
	TickLength    time.Duration // time between each tick

	// If true, tick length will be shortened based on casting speed.
	AffectedByCastSpeed bool

	// How that haste lands, once AffectedByCastSpeed says the dot is hasted.
	TickHaste TickHaste

	// Whether these ticks may crit (AuraEffect::CanPeriodicTickCrit, which the server only allows with
	// an SPELL_AURA_ABILITY_PERIODIC_CRIT aura covering the spell, or for Rupture).
	TicksCanCrit bool

	OnSnapshot OnSnapshot
	OnTick     OnTick
}

type Dot struct {
	Spell *Spell

	// Embed Aura, so we can use IsActive/Refresh/etc directly.
	*Aura

	NumberOfTicks int32         // number of ticks over the whole duration
	TickLength    time.Duration // time between each tick

	// If true, tick length will be shortened based on casting speed.
	AffectedByCastSpeed bool

	// How that haste lands, once AffectedByCastSpeed says the dot is hasted.
	TickHaste TickHaste

	// Whether these ticks may crit (AuraEffect::CanPeriodicTickCrit).
	TicksCanCrit bool

	OnSnapshot OnSnapshot
	OnTick     OnTick

	SnapshotBaseDamage         float64
	SnapshotCritChance         float64
	SnapshotAttackerMultiplier float64

	tickAction *PendingAction
	tickPeriod time.Duration

	// Number of ticks since last call to Apply().
	TickCount int32

	lastTickTime time.Duration
	// when the next tick is due on its own lattice, and the server tick it lands on
	nextTickNominal time.Duration
	nextTickAt      time.Duration
	isChanneled     bool
}

// TickPeriod is how fast the snapshot dot ticks.
func (dot *Dot) TickPeriod() time.Duration {
	return dot.tickPeriod
}

// NextTickAt is when the next tick goes off: the server tick at or after its nominal time.
func (dot *Dot) NextTickAt() time.Duration {
	return dot.nextTickAt
}

func (dot *Dot) TimeUntilNextTick(sim *Simulation) time.Duration {
	return dot.NextTickAt() - sim.CurrentTime
}

// TotalTicks is AuraEffect::GetTotalTicks: how many whole tick intervals fit the duration, in ms.
// Haste that only shortens the interval therefore buys extra ticks.
func (dot *Dot) TotalTicks() int32 {
	if dot.TickHaste == SpellHasteScalesBoth || dot.tickPeriod <= 0 {
		return dot.NumberOfTicks
	}
	return int32(dot.TickLength * time.Duration(dot.NumberOfTicks) / dot.tickPeriod)
}

func (dot *Dot) MaxTicksRemaining() int32 {
	return dot.TotalTicks() - dot.TickCount
}

// NumTicksRemaining is the ticks still to come. All of them land: the aura outlives its last tick.
// Don't count them off the clock, that's off by one between a tick's nominal time and its update.
func (dot *Dot) NumTicksRemaining(_ *Simulation) int {
	return int(max(0, dot.MaxTicksRemaining()))
}

// Roll over = gets carried over with everlasting refresh and doesn't get applied if triggered when the spell is already up.
// - Example: critical strike rating, internal % damage modifiers: buffs or debuffs on player
// Nevermelting Ice, Shadow Mastery (ISB), Trick of the Trades, Deaths Embrace, Thaddius Polarity, Hera Spores, Crit on weapons from swapping

// Snapshot = calculation happens at refresh and application (stays up even if buff falls of, until new refresh or application)
// - Example: Spell power, Haste rating
// Blood Fury, Lightweave Embroid, Eradication, Bloodlust

// Dynamic = realtime update
// - Example: external % damage modifier debuffs on target
// Haunt, Curse of Shadow, Shadow Embrace

// Rollover is used to reset the duration of a dot from an external spell (not casting the dot itself)
// This keeps the snapshot crit and %dmg modifiers.
// However, sp and haste are recalculated.
// Like Aura::RefreshTimersWithMods, it leaves the tick timer alone: the tick already scheduled keeps
// its time, and the new period applies from the one after it.
func (dot *Dot) Rollover(sim *Simulation) {
	dot.TakeSnapshot(sim, true)

	dot.RecomputeAuraDuration() // recalculate haste
	dot.Aura.Refresh(sim)       // update aura's duration
}

// RescheduleNextTick recomputes the period and moves the pending tick to one period past the last.
func (dot *Dot) RescheduleNextTick(sim *Simulation) {
	dot.RecomputeAuraDuration()

	dot.tickAction.Cancel(sim) // remove old PA ticker
	dot.startTickAction(sim, dot.lastTickTime+dot.tickPeriod)
}

// Apply starts the dot, or reapplies a running one under the server's refresh rule.
func (dot *Dot) Apply(sim *Simulation) {
	if dot.IsActive() && !dot.refreshResetsTicks() {
		dot.refreshKeepingTicks(sim, false)
		return
	}

	dot.TakeSnapshot(sim, false)

	dot.Cancel(sim)
	dot.TickCount = 0
	dot.RecomputeAuraDuration()
	dot.Aura.Activate(sim)
}

// ApplyOrReset is Apply() for a rolling dot that is usually already up: it refreshes in place instead
// of deactivating and reactivating the aura, which works around tickAction.CleanUp() wrongly adding
// an extra tick when (re-)application and tick happen at the same time.
func (dot *Dot) ApplyOrReset(sim *Simulation) {
	if !dot.IsActive() {
		dot.Apply(sim)
		return
	}
	if !dot.refreshResetsTicks() {
		dot.refreshKeepingTicks(sim, true)
		return
	}

	dot.TakeSnapshot(sim, true)

	dot.RecomputeAuraDuration() // recalculate haste
	dot.Aura.Refresh(sim)       // update aura's duration

	dot.TickCount = 0

	oldTickAction := dot.tickAction
	dot.tickAction = nil      // prevent tickAction.CleanUp() from adding an extra tick
	oldTickAction.Cancel(sim) // remove old PA ticker

	dot.startTickAction(sim, sim.CurrentTime+dot.tickPeriod)
}

// refreshResetsTicks is Spell::DoSpellHitOnUnit's refreshPeriodic: reapplying a dot restarts its
// tick timer unless the spell stacks to 2 or more. The rule's other half, TRIGGERED_NO_PERIODIC_RESET,
// sits outside TRIGGERED_FULL_MASK, so on this server only a GM's `.cast triggered` ever sets it.
func (dot *Dot) refreshResetsTicks() bool {
	sd := dot.Spell.ServerSpell()
	if sd == nil || sd.StackAmount < 2 {
		return true
	}
	for _, e := range sd.Effects {
		if e.Aura == auraTypePeriodicDamage || e.Aura == auraTypePeriodicDamagePercent {
			return false
		}
	}
	return true
}

// refreshKeepingTicks refreshes a stacking dot the way Aura::RefreshTimers does with periodicReset
// off: the duration and the tick count start over, the running tick timer doesn't.
func (dot *Dot) refreshKeepingTicks(sim *Simulation, doRollover bool) {
	dot.TakeSnapshot(sim, doRollover)

	dot.RecomputeAuraDuration()
	dot.Aura.Refresh(sim)

	dot.TickCount = 0
}

func (dot *Dot) Cancel(sim *Simulation) {
	if dot.Aura.IsActive() {
		dot.Aura.Deactivate(sim)
	}
}

// Call this after manually changing NumberOfTicks or TickLength.
func (dot *Dot) RecomputeAuraDuration() {
	dot.tickPeriod = dot.TickLength
	if dot.AffectedByCastSpeed {
		// every live "haste adds ticks" dot gets it from a mod-spell-tweaks script, and those clamp
		// the multiplier at 1 so a slow can't stretch the ticks. The core's own ATTR5 path would.
		switch dot.TickHaste {
		case SpellHasteAddsTicks:
			dot.tickPeriod = min(dot.Spell.Unit.ApplyCastSpeedForSpell(dot.TickLength, dot.Spell), dot.TickLength)
		case MeleeHasteAddsTicks:
			dot.tickPeriod = min(time.Duration(float64(dot.TickLength)/dot.Spell.Unit.SwingSpeed()), dot.TickLength)
		default:
			dot.tickPeriod = dot.Spell.Unit.ApplyCastSpeedForSpell(dot.TickLength, dot.Spell)
		}
		// the server keeps the amplitude in whole ms
		dot.tickPeriod = dot.tickPeriod.Truncate(time.Millisecond)
	}

	if dot.TickHaste == SpellHasteScalesBoth {
		dot.Aura.Duration = dot.tickPeriod * time.Duration(dot.NumberOfTicks)
	} else {
		// haste buys ticks instead of shortening the dot
		dot.Aura.Duration = dot.TickLength * time.Duration(dot.NumberOfTicks)
	}
}

// Takes a new snapshot of this Dot's effects.
//
// In most cases this will be called automatically, and should only be called
// to force a new snapshot to be taken.
//
//	doRollover will apply previously snapshotted crit/%dmg instead of recalculating.
func (dot *Dot) TakeSnapshot(sim *Simulation, doRollover bool) {
	if dot.OnSnapshot != nil {
		dot.OnSnapshot(sim, dot.Unit, dot, doRollover)
	}
}

// Forces an instant tick. Does not reset the tick timer or aura duration,
// the tick is simply an extra tick.
func (dot *Dot) TickOnce(sim *Simulation) {
	dot.lastTickTime = sim.CurrentTime
	dot.OnTick(sim, dot.Unit, dot)

	if dot.isChanneled {
		// Note: even if the clip delay is 0ms, need a WaitUntil so that APL is called after the channel aura fully fades.
		if dot.MaxTicksRemaining() == 0 {
			if dot.Spell.Unit.GCD.IsReady(sim) {
				// the aura is what holds the rotation, so wait for it, not for the tick. Its expiry
				// and the last tick share a server tick, and ExpiresAt is 0 once it already faded.
				channelEnd := max(sim.CurrentTime, dot.Aura.ExpiresAt())
				dot.Spell.Unit.WaitUntil(sim, channelEnd+dot.Spell.Unit.ChannelClipDelay)
			}
		} else if dot.Spell.Unit.Rotation.shouldInterruptChannel(sim) {
			dot.Cancel(sim)
			if dot.Spell.Unit.GCD.IsReady(sim) {
				dot.Spell.Unit.WaitUntil(sim, sim.CurrentTime+dot.Spell.Unit.ChannelClipDelay)
			}
		}
	}
}

// ManualTick forces the dot forward one tick
// Will cancel the dot if it is out of ticks.
func (dot *Dot) ManualTick(sim *Simulation) {
	if dot.lastTickTime == sim.CurrentTime {
		return
	}
	// count before the increment, or the last charge never lands (Earth Shield)
	if dot.MaxTicksRemaining() <= 0 {
		dot.Cancel(sim)
		return
	}
	dot.TickCount++
	dot.TickOnce(sim)
}

// startTickAction schedules the next tick for the nominal time given, and every tick after it one
// period further along that same lattice. Each one goes off on the first server tick at or after
// its nominal time, so the rounding never piles up: that is AuraEffect::Update carrying what is
// left of m_periodicTimer into the next tick.
func (dot *Dot) startTickAction(sim *Simulation, nominal time.Duration) {
	dot.nextTickNominal = nominal
	dot.nextTickAt = sim.NextServerTick(nominal)

	pa := &PendingAction{
		NextActionAt: dot.nextTickAt,
		//Priority: ActionPriorityDOT,
		CleanUp: func(sim *Simulation) {
			// In certain cases, the last tick and the dot aura expiration can happen in
			// different orders, so we might need to apply the last tick.
			if dot.tickAction != nil && dot.tickAction.NextActionAt == sim.CurrentTime {
				if dot.lastTickTime != sim.CurrentTime {
					dot.TickCount++
					dot.TickOnce(sim)
				}
			}
		},
	}
	pa.OnAction = func(sim *Simulation) {
		if dot.lastTickTime != sim.CurrentTime {
			dot.TickCount++
			dot.TickOnce(sim)
		}

		// a tick can refresh or end the dot, and that schedules its own ticks
		if dot.tickAction != pa || pa.cancelled {
			return
		}
		dot.nextTickNominal += dot.tickPeriod
		dot.nextTickAt = sim.NextServerTick(dot.nextTickNominal)
		pa.NextActionAt = dot.nextTickAt
		sim.AddPendingAction(pa)
	}

	dot.tickAction = pa
	sim.AddPendingAction(pa)
}

func newDot(config Dot) *Dot {
	dot := &Dot{}
	*dot = config

	dot.tickPeriod = dot.TickLength
	dot.Aura.Duration = dot.TickLength * time.Duration(dot.NumberOfTicks)

	dot.Aura.ApplyOnGain(func(aura *Aura, sim *Simulation) {
		dot.lastTickTime = sim.CurrentTime
		dot.startTickAction(sim, sim.CurrentTime+dot.tickPeriod)
		if dot.isChanneled {
			dot.Spell.Unit.ChanneledDot = dot
		}
	})
	dot.Aura.ApplyOnExpire(func(aura *Aura, sim *Simulation) {
		if dot.tickAction != nil {
			dot.tickAction.Cancel(sim)
			dot.tickAction = nil
		}
		if dot.isChanneled {
			dot.Spell.Unit.ChanneledDot = nil
			dot.Spell.Unit.Rotation.interruptChannelIf = nil
			dot.Spell.Unit.Rotation.allowChannelRecastOnInterrupt = false
		}
	})

	return dot
}

type DotArray []*Dot

func (dots DotArray) Get(target *Unit) *Dot {
	return dots[target.UnitIndex]
}

func (spell *Spell) createDots(config DotConfig, isHot bool) {
	if config.NumberOfTicks == 0 && config.TickLength == 0 {
		return
	}

	if config.Spell == nil {
		config.Spell = spell
	}
	dot := Dot{
		Spell: config.Spell,

		NumberOfTicks:       config.NumberOfTicks,
		TickLength:          config.TickLength,
		AffectedByCastSpeed: config.AffectedByCastSpeed,
		TickHaste:           config.TickHaste,
		TicksCanCrit:        config.TicksCanCrit,

		OnSnapshot: config.OnSnapshot,
		OnTick:     config.OnTick,

		isChanneled: config.Spell.Flags.Matches(SpellFlagChanneled),
	}

	auraConfig := config.Aura
	if auraConfig.ActionID.IsEmptyAction() {
		auraConfig.ActionID = dot.Spell.ActionID
	}

	caster := dot.Spell.Unit
	if config.IsAOE || config.SelfOnly {
		dot.Aura = caster.GetOrRegisterAura(auraConfig)
		spell.aoeDot = newDot(dot)
	} else {
		auraConfig.Label += "-" + strconv.Itoa(int(caster.UnitIndex))
		if spell.dots == nil {
			spell.dots = make([]*Dot, len(caster.Env.AllUnits))
		}
		for _, target := range caster.Env.AllUnits {
			if isHot != caster.IsOpponent(target) {
				dot.Aura = target.GetOrRegisterAura(auraConfig)
				spell.dots[target.UnitIndex] = newDot(dot)
			}
		}
	}
}
