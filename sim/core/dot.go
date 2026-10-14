package core

import (
	"slices"
	"strconv"
	"time"

	"github.com/wowsims/wotlk/sim/core/serverdata"
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

// SPELL_EFFECT_PERSISTENT_AREA_AURA, SPELL_EFFECT_SUMMON (a totem) and SPELL_EFFECT_TRANS_DOOR (a
// trap): an area dot from one of these ticks from a dynamic object, totem or trap, all of which
// update on the map update.
const effectTransDoor = 105

var worldObjectEffects = []int32{27, 28, effectTransDoor}

func ticksFromWorldObject(sd *serverdata.Spell) bool {
	return serverSpellHasEffect(sd, worldObjectEffects...)
}

func serverSpellHasEffect(sd *serverdata.Spell, effects ...int32) bool {
	if sd == nil {
		return false
	}
	for _, e := range sd.Effects {
		if slices.Contains(effects, e.Effect) {
			return true
		}
	}
	return false
}

func serverSpellHasAura(sd *serverdata.Spell, aura int32) bool {
	if sd == nil {
		return false
	}
	for _, e := range sd.Effects {
		if e.Effect == effectApplyAura && e.Aura == aura {
			return true
		}
	}
	return false
}

// areaDotMadeInMapUpdate: the area dot's dynamic object is made in a map object's own update, a
// trap going off in GameObject::Update or a pet casting from its AI.
func areaDotMadeInMapUpdate(caster *Unit, sd *serverdata.Spell) bool {
	return caster.Type != PlayerUnit || serverSpellHasEffect(sd, effectTransDoor)
}

type DotConfig struct {
	IsAOE    bool // Set to true for AOE dots (Blizzard, Hurricane, Consecrate, etc)
	SelfOnly bool // Set to true to only create the self-hot.

	// Optional, will default to the corresponding spell.
	Spell *Spell

	Aura Aura

	NumberOfTicks int32         // number of ticks over the whole duration
	TickLength    time.Duration // time between each tick

	// The aura's duration where it isn't ticks times length: a dot whose ticks are charges (Earth Shield).
	AuraDuration time.Duration

	// If true, tick length will be shortened based on casting speed.
	AffectedByCastSpeed bool

	// How that haste lands, once AffectedByCastSpeed says the dot is hasted.
	TickHaste TickHaste

	// Whether these ticks may crit (AuraEffect::CanPeriodicTickCrit, which the server only allows with
	// an SPELL_AURA_ABILITY_PERIODIC_CRIT aura covering the spell, or for Rupture).
	TicksCanCrit bool

	// each tick's numbers, checked like SpellConfig.Direct
	Tick SpellEffect

	OnSnapshot OnSnapshot
	OnTick     OnTick
}

type Dot struct {
	Spell *Spell

	// Embed Aura, so we can use IsActive/Refresh/etc directly.
	*Aura

	NumberOfTicks int32         // number of ticks over the whole duration
	TickLength    time.Duration // time between each tick
	AuraDuration  time.Duration // see DotConfig.AuraDuration

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
	// A stacking dot whose SnapshotBaseDamage is one stack's sets its stacks here: AuraEffect::CalculateAmount
	// truncates one stack's done damage before it multiplies. 0 when the base already counts the stacks.
	SnapshotStacks int32

	// Tick.SP before SpellConfig.Mods
	unmoddedTickSP float64

	tickAction *PendingAction
	tickPeriod time.Duration

	// the last tick action made, armed again once nothing queued can still run it
	spareTick   *PendingAction
	tickCleanUp func(*Simulation)
	// bumped on every arming, so a running tick can tell its action got armed again under it
	tickArmings uint32

	// Number of ticks since the dot went on or was last refreshed.
	TickCount int32

	// the aura's max duration before haste: ticks times length as the dot went on or was refreshed,
	// plus what UpdateExpires added since
	maxDuration time.Duration

	// whose update ticks the dot, in spell id order with its other dots. Nil for an area dot's object.
	tickUnit  *Unit
	tickOrder int32

	lastTickTime time.Duration
	// when the next tick is due on its own lattice, and the server tick it lands on
	nextTickNominal time.Duration
	nextTickAt      time.Duration
	isChanneled     bool

	// DotConfig.Tick, with the server's values where RegisterSpell found a conflict, then SpellConfig.Mods
	Tick SpellEffect
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

// TotalTicks is AuraEffect::GetTotalTicks: how many whole tick intervals fit the max duration, in ms.
// Haste that only shortens the interval therefore buys extra ticks, and so does time added to a
// running dot (UpdateExpires).
func (dot *Dot) TotalTicks() int32 {
	period := dot.tickPeriod
	if dot.TickHaste == SpellHasteScalesBoth {
		// the duration shrinks with the interval, so count in unhasted ones
		period = dot.TickLength
	}
	if period <= 0 {
		return dot.NumberOfTicks
	}
	return int32(max(dot.TickLength*time.Duration(dot.NumberOfTicks), dot.maxDuration) / period)
}

// outOfTicks is AuraEffect::Update's stop: a dot that has ticked its total doesn't tick again until a
// refresh starts the count over, though its timer runs on.
func (dot *Dot) outOfTicks() bool {
	return dot.TickCount >= dot.TotalTicks() && dot.Aura.Duration != NeverExpires
}

// restartTickCount is Aura::RefreshDuration's ResetTicks, with the max duration back at the dot's own.
func (dot *Dot) restartTickCount() {
	dot.TickCount = 0
	dot.maxDuration = dot.TickLength * time.Duration(dot.NumberOfTicks)
}

// Refresh starts the duration and the tick count over and leaves the tick timer running, as
// Aura::RefreshTimers does without a periodic reset.
func (dot *Dot) Refresh(sim *Simulation) {
	dot.Aura.Refresh(sim)
	dot.restartTickCount()
}

// Activate puts the aura on, or refreshes a running one as Refresh does.
func (dot *Dot) Activate(sim *Simulation) {
	dot.restartTickCount()
	dot.Aura.Activate(sim)
}

// AddRemaining is a script's SetDuration(GetDuration() + extra) that raises the max duration only when the
// new time left passes it (spell_sha_t10_elemental_4p_bonus), so the added time ticks just past the old cap.
func (dot *Dot) AddRemaining(sim *Simulation, extra time.Duration) {
	dot.Aura.AddDuration(sim, extra)
	// GetDuration is what the unit's last update left of it
	left := dot.Aura.expiresNominal - dot.Aura.countdownStart(sim)
	if dot.TickHaste == SpellHasteScalesBoth && dot.tickPeriod > 0 && dot.tickPeriod != dot.TickLength {
		// the max duration counts in unhasted intervals for these (TotalTicks), the time left in hasted ones
		left = time.Duration(left.Milliseconds()*dot.TickLength.Milliseconds()/dot.tickPeriod.Milliseconds()) * time.Millisecond
	}
	dot.maxDuration = max(dot.maxDuration, left)
}

// UpdateExpires moves the expiry and the max duration with it, so the added time ticks: what the
// glyphs that lengthen a running dot do (SetDuration with SetMaxDuration). Aura.UpdateExpires moves
// the expiry alone.
func (dot *Dot) UpdateExpires(newExpires time.Duration) {
	dot.maxDuration += newExpires - dot.Aura.expires
	dot.Aura.UpdateExpires(newExpires)
}

// AddDuration lengthens the running dot and its max duration by extra, so the added time ticks.
func (dot *Dot) AddDuration(sim *Simulation, extra time.Duration) {
	dot.maxDuration += extra
	dot.Aura.AddDuration(sim, extra)
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
	dot.restartTickCount()
	dot.TakeSnapshot(sim, true)

	dot.RecomputeAuraDuration() // recalculate haste
	dot.Aura.Refresh(sim)       // update aura's duration
}

// Apply starts the dot, or reapplies a running one under the server's refresh rule.
func (dot *Dot) Apply(sim *Simulation) {
	if dot.IsActive() && !dot.refreshResetsTicks() {
		dot.refreshKeepingTicks(sim, false)
		return
	}

	dot.TakeSnapshot(sim, false)

	dot.Cancel(sim)
	dot.restartTickCount()
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

	dot.restartTickCount()

	oldTickAction := dot.tickAction
	dot.tickAction = nil      // prevent tickAction.CleanUp() from adding an extra tick
	oldTickAction.Cancel(sim) // remove old PA ticker

	dot.startTickAction(sim, dot.Aura.countdownStart(sim)+dot.tickPeriod)
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

	dot.restartTickCount()
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
		// the server truncates the duration and the period each on its own, so the aura can run a
		// few ms longer than period times ticks
		dot.Aura.Duration = dot.TickLength * time.Duration(dot.NumberOfTicks)
		if dot.AffectedByCastSpeed {
			dot.Aura.Duration = dot.Spell.Unit.ApplyCastSpeedForSpell(dot.Aura.Duration, dot.Spell).Truncate(time.Millisecond)
		}
	} else {
		// haste buys ticks instead of shortening the dot
		dot.Aura.Duration = dot.TickLength * time.Duration(dot.NumberOfTicks)
	}
	if dot.AuraDuration != 0 {
		dot.Aura.Duration = dot.AuraDuration
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

	// A tick only ends the channel by interrupting it. After the last one the aura still runs to its own
	// expiry, which can be the caster's next update.
	if dot.isChanneled && dot.MaxTicksRemaining() != 0 && dot.Spell.Unit.Rotation.shouldInterruptChannel(sim) {
		dot.Cancel(sim)
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
// period further along that same lattice. Each one goes off on the first update at or after its
// nominal time of whatever counts the dot down, so the rounding never piles up: that is
// AuraEffect::Update carrying what is left of m_periodicTimer into the next tick.
func (dot *Dot) startTickAction(sim *Simulation, nominal time.Duration) {
	dot.nextTickNominal = nominal
	dot.nextTickAt = dot.Aura.nextUpdate(sim, nominal)

	pa := dot.reusableTickAction(sim)
	if pa == nil {
		pa = dot.newTickAction()
	}
	pa.NextActionAt = dot.nextTickAt
	// a player's channel or self hot ticks on its update, ahead of the creatures' auras that update
	pa.playerEvent = dot.tickUnit != nil && dot.tickUnit.Type == PlayerUnit
	pa.Priority = 0
	if pa.playerEvent {
		pa.Priority = ActionPriorityEvent
	}
	pa.CleanUp = dot.tickCleanUp
	pa.cancelled = false
	dot.tickArmings++
	dot.tickAction = pa
	sim.AddPendingAction(pa)
}

// reusableTickAction is the dot's last tick action, or nil while it can still run: queued live, or the
// action Step is advancing the clock to.
func (dot *Dot) reusableTickAction(sim *Simulation) *PendingAction {
	pa := dot.spareTick
	if pa == nil {
		return nil
	}
	if pa.consumed || pa.slot >= int32(len(sim.pendingSlots)) || sim.pendingSlots[pa.slot] != pa || sim.detachPendingAction(pa) {
		return pa
	}
	return nil
}

func (dot *Dot) newTickAction() *PendingAction {
	if dot.tickCleanUp == nil {
		dot.tickCleanUp = func(sim *Simulation) {
			// In certain cases, the last tick and the dot aura expiration can happen in
			// different orders, so we might need to apply the last tick.
			if dot.tickIsDue(sim) {
				dot.runTick(sim)
			}
		}
	}
	pa := &PendingAction{}
	pa.OnAction = func(sim *Simulation) {
		dot.onTickAction(sim, pa)
	}
	dot.spareTick = pa
	return pa
}

func (dot *Dot) onTickAction(sim *Simulation, pa *PendingAction) {
	// a tick can refresh or end the dot, and a refresh can arm pa again for its new timer
	armings := dot.tickArmings

	// not tickIsDue: a tick queued for a time already past is late, and still runs
	if dot.tickOwed(sim) {
		ranOthers := false
		// only another action queued for now can be a tick due with this one
		if dot.tickUnit != nil && sim.pendingActions[len(sim.pendingActions)-1].at == sim.CurrentTime {
			ranOthers = dot.tickUnit.runDueTicks(sim, dot)
		}
		// a lower spell's tick can have refreshed or ended this dot
		if dot.tickAction == pa && dot.tickArmings == armings && (!ranOthers || dot.tickOwed(sim)) {
			dot.runTick(sim)
		}
	}

	// a refresh or the end has scheduled its own ticks
	if dot.tickAction != pa || dot.tickArmings != armings || pa.cancelled {
		return
	}
	if dot.lastTickTime != sim.CurrentTime && dot.outOfTicks() {
		if dot.Aura.expires < sim.CurrentTime {
			// nothing updates this unit (a pet that's away), so nothing else ends its aura
			dot.Cancel(sim)
			return
		}
		// the tick stays owed and the server looks again every update: a refresh lets it through
		dot.nextTickAt = dot.Aura.nextUpdate(sim, sim.CurrentTime+time.Millisecond)
	} else {
		dot.nextTickNominal += dot.tickPeriod
		dot.nextTickAt = dot.Aura.nextUpdate(sim, dot.nextTickNominal)
	}
	pa.NextActionAt = dot.nextTickAt
	sim.AddPendingAction(pa)
}

// tickIsDue is whether the tick queued for this update is still to run. nextTickAt always matches the
// armed action's NextActionAt.
func (dot *Dot) tickIsDue(sim *Simulation) bool {
	return dot.tickAction != nil && dot.nextTickAt == sim.CurrentTime && dot.tickOwed(sim)
}

// tickOwed is whether the dot may tick now: once an update, and not past its total.
func (dot *Dot) tickOwed(sim *Simulation) bool {
	return dot.lastTickTime != sim.CurrentTime && !dot.outOfTicks()
}

func (dot *Dot) runTick(sim *Simulation) {
	outer := sim.updating
	sim.updating = dot.Aura.countdownUnit()
	dot.TickCount++
	dot.TickOnce(sim)
	sim.updating = outer
}

// runDueTicks runs the ticks of the unit's dots that are due on this update, lowest spell id first, as
// Unit::_UpdateSpells walks the unit's auras. It stops short of upTo. Reports whether any ran.
func (unit *Unit) runDueTicks(sim *Simulation, upTo *Dot) bool {
	ran := false
	for {
		var due *Dot
		for _, dot := range unit.tickingDots {
			if dot == upTo {
				break
			}
			if dot.tickIsDue(sim) {
				due = dot
				break
			}
		}
		if due == nil {
			return ran
		}
		// a tick can put dots on and take them off, so the list is walked again after each
		due.runTick(sim)
		ran = true
	}
}

// startTicking puts the dot among its unit's ticking ones, after those with its spell id or a lower one.
func (dot *Dot) startTicking() {
	unit := dot.tickUnit
	if unit == nil {
		return
	}
	i := len(unit.tickingDots)
	for i > 0 && unit.tickingDots[i-1].tickOrder > dot.tickOrder {
		i--
	}
	unit.tickingDots = slices.Insert(unit.tickingDots, i, dot)
}

func (dot *Dot) stopTicking() {
	if unit := dot.tickUnit; unit != nil {
		if i := slices.Index(unit.tickingDots, dot); i >= 0 {
			unit.tickingDots = slices.Delete(unit.tickingDots, i, i+1)
		}
	}
}

func newDot(config Dot) *Dot {
	dot := &Dot{}
	*dot = config

	dot.tickPeriod = dot.TickLength
	dot.Aura.Duration = dot.TickLength * time.Duration(dot.NumberOfTicks)
	if dot.AuraDuration != 0 {
		dot.Aura.Duration = dot.AuraDuration
	}
	// Spell::update runs a channel on the caster's update, and most tick from an aura on the caster
	if dot.isChanneled {
		dot.Aura.countdownBy = dot.Spell.Unit
	}
	if !dot.Aura.countsOnMapGrid {
		dot.tickUnit = dot.Aura.countdownUnit()
	}
	dot.tickOrder = dot.Aura.ActionID.SpellID
	if dot.tickOrder == 0 {
		dot.tickOrder = dot.Spell.ActionID.SpellID
	}

	dot.Aura.ApplyOnGain(func(aura *Aura, sim *Simulation) {
		dot.lastTickTime = sim.CurrentTime
		dot.startTickAction(sim, aura.countdownStart(sim)+dot.tickPeriod)
		dot.startTicking()
		if dot.isChanneled {
			dot.Spell.Unit.ChanneledDot = dot
		}
	})
	dot.Aura.ApplyOnExpire(func(aura *Aura, sim *Simulation) {
		if dot.tickAction != nil {
			dot.tickAction.Cancel(sim)
			dot.tickAction = nil
		}
		dot.stopTicking()
		dot.maxDuration = 0
		if dot.isChanneled {
			caster := dot.Spell.Unit
			caster.ChanneledDot = nil
			caster.Rotation.interruptChannelIf = nil
			caster.Rotation.allowChannelRecastOnInterrupt = false
			// A GCD that ran out under the channel woke the rotation for nothing, so every way a channel
			// ends has to queue it again here. Not a dismissed pet's: its GCD loop is off.
			if caster.enabled && caster.GCD.IsReady(sim) {
				caster.WaitUntil(sim, sim.CurrentTime+caster.ChannelClipDelay)
			}
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
		AuraDuration:        config.AuraDuration,
		AffectedByCastSpeed: config.AffectedByCastSpeed,
		TickHaste:           config.TickHaste,
		TicksCanCrit:        config.TicksCanCrit,
		Tick:                config.Tick,
		unmoddedTickSP:      spell.unmoddedSP[TernaryInt32(isHot, 2, 1)],

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
		if sd := dot.Spell.ServerSpell(); config.IsAOE && !dot.isChanneled && ticksFromWorldObject(sd) {
			dot.Aura.countsOnMapGrid = true
			dot.Aura.madeInMapUpdate = areaDotMadeInMapUpdate(caster, sd)
		}
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
