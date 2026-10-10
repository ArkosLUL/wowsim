package core

import (
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/wowsims/wotlk/sim/core/proto"
)

type RuneChangeType int32

const (
	None             RuneChangeType = 0
	SpendRune        RuneChangeType = 1
	GainRune         RuneChangeType = 2
	ConvertToDeath   RuneChangeType = 4
	ConvertFromDeath RuneChangeType = 8
)

func (r RuneChangeType) Matches(other RuneChangeType) bool {
	return (r & other) != 0
}

type OnRuneChange func(sim *Simulation, changeType RuneChangeType)
type OnRunicPowerGain func(sim *Simulation)

type RuneMeta struct {
	regenAt       time.Duration // time at which the rune will no longer be spent.
	lastRegenTime time.Duration // last time the rune regenerated.

	revertAt time.Duration // time at which rune will no longer be kind death.
	talent   deathRuneTalent

	spentAt time.Duration // when a landed spell last put the rune on cooldown
}

// deathRuneTalent is the spell_dk_death_rune talent that turned a rune death. Each has its own 30 s periodic,
// which every proc restarts, and its runes revert when that runs out.
type deathRuneTalent int8

const (
	noDeathRuneTalent deathRuneTalent = iota
	bloodDeathRuneTalent
	frostUnholyDeathRuneTalent
)

const deathRuneTalentPeriod = 30 * time.Second

type runicPowerBar struct {
	unit *Unit

	maxRunicPower     float64
	currentRunicPower float64
	runeCD            time.Duration
	runeRegenPct      int64 // SPELL_AURA_MOD_POWER_REGEN_PERCENT on runes

	// These flags are used to simplify pending action checks
	// |DS|DS|DS|DS|DS|DS|
	runeStates int16
	runeMeta   [6]RuneMeta
	btSlot     int8

	bloodRuneGainMetrics  *ResourceMetrics
	frostRuneGainMetrics  *ResourceMetrics
	unholyRuneGainMetrics *ResourceMetrics
	deathRuneGainMetrics  *ResourceMetrics

	onRuneChange     OnRuneChange
	onRunicPowerGain OnRunicPowerGain

	pa *PendingAction
}

// Constants for finding runes
// |DS|DS|DS|DS|DS|DS|
const (
	baseRuneState = 0 // unspent, no death

	allDeath = 0b101010101010
	allSpent = 0b010101010101

	anyBloodSpent  = 0b0101 << 0
	anyFrostSpent  = 0b0101 << 4
	anyUnholySpent = 0b0101 << 8
)

var (
	isDeaths     = [6]int16{0b10 << 0, 0b10 << 2, 0b10 << 4, 0b10 << 6, 0b10 << 8, 0b10 << 10}
	isSpents     = [6]int16{0b01 << 0, 0b01 << 2, 0b01 << 4, 0b01 << 6, 0b01 << 8, 0b01 << 10}
	isSpentDeath = [6]int16{0b11 << 0, 0b11 << 2, 0b11 << 4, 0b11 << 6, 0b11 << 8, 0b11 << 10}
)

func (rp *runicPowerBar) DebugString() string {
	ss := make([]string, len(rp.runeMeta))
	for i := range rp.runeMeta {
		ss[i] += fmt.Sprintf("Rune %d - D: %v S: %v\n\tRegenAt: %0.1f, RevertAt: %0.1f", i, rp.runeStates&isDeaths[i] != 0, rp.runeStates&isSpents[i] != 0, rp.runeMeta[i].regenAt.Seconds(), rp.runeMeta[i].revertAt.Seconds())
	}
	return strings.Join(ss, "\n")
}

func (rp *runicPowerBar) Predictor() *Predictor {
	return &Predictor{
		rp:         rp,
		runeStates: rp.runeStates,
		runeMeta:   rp.runeMeta,
	}
}

func (rp *runicPowerBar) reset(sim *Simulation) {
	if rp.unit == nil {
		return
	}

	if rp.pa != nil {
		rp.pa.Cancel(sim)
	}

	for i := range rp.runeMeta {
		rp.runeMeta[i].lastRegenTime = -1
		rp.runeMeta[i].regenAt = NeverExpires

		rp.runeMeta[i].revertAt = NeverExpires
		rp.runeMeta[i].talent = noDeathRuneTalent
		rp.runeMeta[i].spentAt = NeverExpires
	}

	rp.runeStates = baseRuneState
}

func (unit *Unit) EnableRunicPowerBar(currentRunicPower float64, maxRunicPower float64, runeCD time.Duration,
	onRuneChange OnRuneChange, onRunicPowerGain OnRunicPowerGain) {
	unit.SetCurrentPowerBar(RunicPower)
	unit.runicPowerBar = runicPowerBar{
		unit: unit,

		maxRunicPower:     maxRunicPower,
		currentRunicPower: currentRunicPower,
		runeCD:            runeCD,

		runeStates: baseRuneState,
		btSlot:     -1,

		onRuneChange:     onRuneChange,
		onRunicPowerGain: onRunicPowerGain,
	}

	unit.bloodRuneGainMetrics = unit.NewBloodRuneMetrics(ActionID{OtherID: proto.OtherAction_OtherActionBloodRuneGain, Tag: 1})
	unit.frostRuneGainMetrics = unit.NewFrostRuneMetrics(ActionID{OtherID: proto.OtherAction_OtherActionFrostRuneGain, Tag: 1})
	unit.unholyRuneGainMetrics = unit.NewUnholyRuneMetrics(ActionID{OtherID: proto.OtherAction_OtherActionUnholyRuneGain, Tag: 1})
	unit.deathRuneGainMetrics = unit.NewDeathRuneMetrics(ActionID{OtherID: proto.OtherAction_OtherActionDeathRuneGain, Tag: 1})
}

func (unit *Unit) HasRunicPowerBar() bool {
	return unit.runicPowerBar.unit != nil
}

// SetRuneRegenPercent sets the rune regen percent auras' amount (Improved Unholy Presence's 63622), which
// scales the cooldown of a rune spent from then on.
func (rp *runicPowerBar) SetRuneRegenPercent(pct int64) {
	rp.runeRegenPct = pct
}

// runeCooldown is Player::GetRuneBaseCooldown for a rune spent with the given grace: the grace comes off
// first, then the regen percent scales what's left, in whole ms.
func (rp *runicPowerBar) runeCooldown(grace time.Duration) time.Duration {
	cd := rp.runeCD - grace
	if rp.runeRegenPct == 0 {
		return cd
	}
	return time.Duration(int64(cd/time.Millisecond)*(100-rp.runeRegenPct)/100) * time.Millisecond
}

func (rp *runicPowerBar) CurrentRunicPower() float64 {
	return rp.currentRunicPower
}

func (rp *runicPowerBar) maybeFireChange(sim *Simulation, changeType RuneChangeType) {
	if changeType != None && rp.onRuneChange != nil {
		rp.onRuneChange(sim, changeType)
	}
}

func (rp *runicPowerBar) addRunicPowerInterval(sim *Simulation, amount float64, metrics *ResourceMetrics) {
	if amount < 0 {
		panic("Trying to add negative runic power!")
	}

	newRunicPower := min(rp.currentRunicPower+amount, rp.maxRunicPower)

	metrics.AddEvent(amount, newRunicPower-rp.currentRunicPower)

	if sim.Log != nil {
		rp.unit.Log(sim, "Gained %0.3f runic power from %s (%0.3f --> %0.3f).", amount, metrics.ActionID, rp.currentRunicPower, newRunicPower)
	}
	if metrics.gainThreat != nil {
		rp.unit.addGainThreat(sim, metrics, amount, newRunicPower-rp.currentRunicPower)
	}

	rp.currentRunicPower = newRunicPower
}

func (rp *runicPowerBar) AddRunicPower(sim *Simulation, amount float64, metrics *ResourceMetrics) {
	rp.addRunicPowerInterval(sim, amount, metrics)
	if rp.onRunicPowerGain != nil {
		rp.onRunicPowerGain(sim)
	}
}

func (rp *runicPowerBar) spendRunicPower(sim *Simulation, amount float64, metrics *ResourceMetrics) {
	if amount < 0 {
		panic("Trying to spend negative runic power!")
	}

	newRunicPower := rp.currentRunicPower - amount

	metrics.AddEvent(-amount, -amount)

	if sim.Log != nil {
		rp.unit.Log(sim, "Spent %0.3f runic power from %s (%0.3f --> %0.3f).", amount, metrics.ActionID, rp.currentRunicPower, newRunicPower)
	}

	rp.currentRunicPower = newRunicPower
}

// DeathRuneRegenAt returns the time the given death rune will regen at.
// If the rune is not death or not spent it returns NeverExpires.
func (rp *runicPowerBar) DeathRuneRegenAt(slot int32) time.Duration {
	// If not death or not spent, no regen time
	if rp.runeStates&isSpentDeath[slot] != isSpentDeath[slot] {
		return NeverExpires
	}
	return rp.runeMeta[slot].regenAt
}

// DeathRuneRevertAt returns the next time that a death rune will revert.
// If there is no death rune that needs to revert it returns NeverExpires.
func (rp *runicPowerBar) DeathRuneRevertAt() time.Duration {
	minRevertAt := rp.runeMeta[0].revertAt
	for _, rm := range rp.runeMeta[1:] {
		if rm.revertAt < minRevertAt {
			minRevertAt = rm.revertAt
		}
	}
	return minRevertAt
}

func (rp *runicPowerBar) runeGraceRemaining(sim *Simulation, slot int8) time.Duration {
	if runeGrace := rp.CurrentRuneGrace(sim, slot); runeGrace > 0 {
		return time.Millisecond*2500 - runeGrace
	}
	return 0
}

func (rp *runicPowerBar) CurrentRuneGrace(sim *Simulation, slot int8) time.Duration {
	lastRegenTime := rp.runeMeta[slot].lastRegenTime

	// pre-pull casts should not get rune-grace
	if sim.CurrentTime <= 0 || lastRegenTime <= 0 {
		return 0
	}

	if lastRegenTime < sim.CurrentTime {
		return min(2500*time.Millisecond, sim.CurrentTime-lastRegenTime)
	}
	return 0
}

func (rp *runicPowerBar) CurrentBloodRuneGrace(sim *Simulation) time.Duration {
	return max(rp.CurrentRuneGrace(sim, 0), rp.CurrentRuneGrace(sim, 1))
}

func (rp *runicPowerBar) CurrentFrostRuneGrace(sim *Simulation) time.Duration {
	return max(rp.CurrentRuneGrace(sim, 2), rp.CurrentRuneGrace(sim, 3))
}

func (rp *runicPowerBar) CurrentUnholyRuneGrace(sim *Simulation) time.Duration {
	return max(rp.CurrentRuneGrace(sim, 4), rp.CurrentRuneGrace(sim, 5))
}

func (rp *runicPowerBar) FrostRuneGraceRemaining(sim *Simulation) time.Duration {
	return max(rp.runeGraceRemaining(sim, 2), rp.runeGraceRemaining(sim, 3))
}

func (rp *runicPowerBar) UnholyRuneGraceRemaining(sim *Simulation) time.Duration {
	return max(rp.runeGraceRemaining(sim, 4), rp.runeGraceRemaining(sim, 5))
}

func (rp *runicPowerBar) normalSpentRuneReadyAt(slot int8) time.Duration {
	readyAt := NeverExpires
	if t := rp.runeMeta[slot].regenAt; t < readyAt && rp.runeStates&isSpentDeath[slot] == isSpents[slot] {
		readyAt = t
	}
	if t := rp.runeMeta[slot+1].regenAt; t < readyAt && rp.runeStates&isSpentDeath[slot+1] == isSpents[slot+1] {
		readyAt = t
	}
	return readyAt
}

// NormalSpentBloodRuneReadyAt returns the earliest time a spent non-death blood rune is ready.
func (rp *runicPowerBar) NormalSpentBloodRuneReadyAt(_ *Simulation) time.Duration {
	return rp.normalSpentRuneReadyAt(0)
}

func (rp *runicPowerBar) normalRuneReadyAt(sim *Simulation, slot int8) time.Duration {
	if rp.runeStates&isSpentDeath[slot] == 0 || rp.runeStates&isSpentDeath[slot+1] == 0 {
		return sim.CurrentTime
	}
	return rp.normalSpentRuneReadyAt(slot)
}

// NormalFrostRuneReadyAt returns the earliest time a non-death frost rune is ready.
func (rp *runicPowerBar) NormalFrostRuneReadyAt(sim *Simulation) time.Duration {
	return rp.normalRuneReadyAt(sim, 2)
}

func (rp *runicPowerBar) NormalUnholyRuneReadyAt(sim *Simulation) time.Duration {
	return rp.normalRuneReadyAt(sim, 4)
}

func (rp *runicPowerBar) BloodDeathRuneBothReadyAt() time.Duration {
	if rp.runeStates&isDeaths[0] != 0 && rp.runeStates&isDeaths[1] != 0 {
		if max(rp.runeMeta[0].regenAt, rp.runeMeta[1].regenAt) > 150000000*time.Minute {
			return min(rp.runeMeta[0].regenAt, rp.runeMeta[1].regenAt)
		} else {
			return max(rp.runeMeta[0].regenAt, rp.runeMeta[1].regenAt)
		}
	} else {
		return -1
	}
}

func (rp *runicPowerBar) RuneReadyAt(sim *Simulation, slot int8) time.Duration {
	if rp.runeStates&isSpents[slot] != isSpents[slot] {
		return sim.CurrentTime
	}
	return rp.runeMeta[slot].regenAt
}

func (rp *runicPowerBar) SpendRuneReadyAt(slot int8, spendAt time.Duration) time.Duration {
	runeGraceDuration := rp.RuneGraceAt(slot, spendAt)
	return spendAt + rp.runeCooldown(runeGraceDuration)
}

// BloodRuneReadyAt returns the earliest time a (possibly death-converted) blood rune is ready.
func (rp *runicPowerBar) BloodRuneReadyAt(sim *Simulation) time.Duration {
	if rp.runeStates&anyBloodSpent != anyBloodSpent { // if any are not spent
		return sim.CurrentTime
	}
	return min(rp.runeMeta[0].regenAt, rp.runeMeta[1].regenAt)
}

func (rp *runicPowerBar) FrostRuneReadyAt(sim *Simulation) time.Duration {
	if rp.runeStates&anyFrostSpent != anyFrostSpent { // if any are not spent
		return sim.CurrentTime
	}
	return min(rp.runeMeta[2].regenAt, rp.runeMeta[3].regenAt)
}

func (rp *runicPowerBar) UnholyRuneReadyAt(sim *Simulation) time.Duration {
	if rp.runeStates&anyUnholySpent != anyUnholySpent { // if any are not spent
		return sim.CurrentTime
	}
	return min(rp.runeMeta[4].regenAt, rp.runeMeta[5].regenAt)
}

func (rp *runicPowerBar) bothRunesReadyAt(sim *Simulation, slot int8) time.Duration {
	switch (rp.runeStates >> (2 * slot)) & 0b0101 {
	case 0b0000:
		return sim.CurrentTime
	case 0b0001:
		return rp.runeMeta[slot].regenAt
	case 0b0100:
		return rp.runeMeta[slot+1].regenAt
	default:
		return max(rp.runeMeta[slot].regenAt, rp.runeMeta[slot+1].regenAt)
	}
}

func (rp *runicPowerBar) NextBloodRuneReadyAt(sim *Simulation) time.Duration {
	return rp.bothRunesReadyAt(sim, 0)
}

func (rp *runicPowerBar) NextFrostRuneReadyAt(sim *Simulation) time.Duration {
	return rp.bothRunesReadyAt(sim, 2)
}

func (rp *runicPowerBar) NextUnholyRuneReadyAt(sim *Simulation) time.Duration {
	return rp.bothRunesReadyAt(sim, 4)
}

// AnySpentRuneReadyAt returns the next time that a rune will regenerate.
// It will be NeverExpires if there is no rune pending regeneration.
func (rp *runicPowerBar) AnySpentRuneReadyAt() time.Duration {
	minRegenAt := rp.runeMeta[0].regenAt
	for _, rm := range rp.runeMeta[1:] {
		if rm.regenAt < minRegenAt {
			minRegenAt = rm.regenAt
		}
	}
	return minRegenAt
}

func (rp *runicPowerBar) AnyRuneReadyAt(sim *Simulation) time.Duration {
	if rp.runeStates&allSpent != allSpent {
		return sim.CurrentTime
	}
	return rp.AnySpentRuneReadyAt()
}

// ConvertFromDeath reverts the rune to its original type.
func (rp *runicPowerBar) ConvertFromDeath(sim *Simulation, slot int8) {
	rp.runeStates ^= isDeaths[slot]
	rp.runeMeta[slot].revertAt = NeverExpires
	rp.runeMeta[slot].talent = noDeathRuneTalent

	if rp.runeStates&isSpents[slot] == 0 {
		metrics := rp.bloodRuneGainMetrics
		if slot == 2 || slot == 3 {
			metrics = rp.frostRuneGainMetrics
		} else if slot == 4 || slot == 5 {
			metrics = rp.unholyRuneGainMetrics
		}
		rp.spendRuneMetrics(sim, rp.deathRuneGainMetrics, 1)
		rp.gainRuneMetrics(sim, metrics, 1)
	}
}

// ConvertToDeath converts the given slot to death until revertAt.
func (rp *runicPowerBar) ConvertToDeath(sim *Simulation, slot int8, revertAt time.Duration) {
	rp.runeStates |= isDeaths[slot]
	rp.runeMeta[slot].revertAt = revertAt

	// Note we gained
	metrics := rp.bloodRuneGainMetrics
	if slot == 2 || slot == 3 {
		metrics = rp.frostRuneGainMetrics
	} else if slot == 4 || slot == 5 {
		metrics = rp.unholyRuneGainMetrics
	}
	if rp.runeStates&isSpents[slot] == 0 {
		// Only lose/gain if it wasn't spent (which it should be at this point)
		rp.spendRuneMetrics(sim, metrics, 1)
		rp.gainRuneMetrics(sim, rp.deathRuneGainMetrics, 1)
	}
}

func (rp *runicPowerBar) CancelBloodTap(sim *Simulation) {
	if rp.btSlot == -1 {
		return
	}
	rp.ConvertFromDeath(sim, rp.btSlot)
	bloodTapAura := rp.unit.GetAura("Blood Tap")
	bloodTapAura.Deactivate(sim)
	rp.btSlot = -1

	rp.maybeFireChange(sim, ConvertFromDeath)
}

// BloodTapConversion is Blood Tap's two rune effects. Spell::EffectActivateRune goes first, at launch: it
// readies a blood slot's death rune that's on cooldown, or else, only with both blood runes plain and on
// cooldown, the one due back sooner (the second on a tie). Then the aura turns the first plain blood rune
// that's ready, so sometimes none.
func (rp *runicPowerBar) BloodTapConversion(sim *Simulation) {
	changeType := None

	ready := int8(-1)
	switch {
	case rp.runeStates&isSpentDeath[0] == isSpentDeath[0]:
		ready = 0
	case rp.runeStates&isSpentDeath[1] == isSpentDeath[1]:
		ready = 1
	case rp.runeStates&(isSpentDeath[0]|isSpentDeath[1]) == isSpents[0]|isSpents[1]:
		ready = 0
		if rp.runeMeta[0].regenAt >= rp.runeMeta[1].regenAt {
			ready = 1
		}
	}
	if ready != -1 {
		rp.regenRune(sim, sim.CurrentTime, ready)
		changeType |= GainRune
	}

	for slot := int8(0); slot < 2; slot++ {
		if rp.runeStates&isSpentDeath[slot] == 0 {
			rp.btSlot = slot
			rp.ConvertToDeath(sim, slot, sim.CurrentTime+time.Second*20)
			changeType |= ConvertToDeath
			// if PA isn't running, make it run 20s from now to disable BT
			rp.launchPA(sim, sim.CurrentTime+20.0*time.Second)
			break
		}
	}

	rp.maybeFireChange(sim, changeType)
}

func (rp *runicPowerBar) LeftBloodRuneReady() bool {
	return rp.runeStates&isSpents[0] == 0
}

func (rp *runicPowerBar) RuneIsActive(slot int8) bool {
	return rp.runeStates&isSpents[slot] == 0
}

func (rp *runicPowerBar) RuneIsDeath(slot int8) bool {
	return rp.runeStates&isDeaths[slot] != 0
}

// rune state to count of non-death, non-spent runes (0b00)
var rs2c = []int8{
	0b0000: 2, 0b0001: 1, 0b0010: 1, 0b0011: 1, 0b0100: 1, 0b0101: 0, 0b0110: 0, 0b0111: 0,
	0b1000: 1, 0b1001: 0, 0b1010: 0, 0b1011: 0, 0b1100: 1, 0b1101: 0, 0b1110: 0, 0b1111: 0,
}

func (rp *runicPowerBar) CurrentBloodRunes() int8 {
	return rs2c[(rp.runeStates>>0)&0b1111]
}

func (rp *runicPowerBar) CurrentFrostRunes() int8 {
	return rs2c[(rp.runeStates>>4)&0b1111]
}

func (rp *runicPowerBar) CurrentUnholyRunes() int8 {
	return rs2c[(rp.runeStates>>8)&0b1111]
}

// rune state to count of death, non-spent runes (0b10)
var rs2d = []int8{
	0b0000: 0, 0b0001: 0, 0b0010: 1, 0b0011: 0, 0b0100: 0, 0b0101: 0, 0b0110: 1, 0b0111: 0,
	0b1000: 1, 0b1001: 1, 0b1010: 2, 0b1011: 1, 0b1100: 0, 0b1101: 0, 0b1110: 1, 0b1111: 0,
}

func (rp *runicPowerBar) CurrentDeathRunes() int8 {
	return rs2d[(rp.runeStates>>0)&0b1111] + rs2d[(rp.runeStates>>4)&0b1111] + rs2d[(rp.runeStates>>8)&0b1111]
}

func (rp *runicPowerBar) DeathRunesInFU() int8 {
	return rs2d[(rp.runeStates>>4)&0b1111] + rs2d[(rp.runeStates>>8)&0b1111]
}

// rune state to count of non-spent runes (0bx0), masking death runes
var rs2cd = [16]int8{
	0b0000: 2, 0b0001: 1, 0b0100: 1,
}

func (rp *runicPowerBar) CurrentBloodOrDeathRunes() int8 {
	return rs2cd[(rp.runeStates>>0)&0b0101]
}

func (rp *runicPowerBar) CurrentFrostOrDeathRunes() int8 {
	return rs2cd[(rp.runeStates>>4)&0b0101]
}

func (rp *runicPowerBar) CurrentUnholyOrDeathRunes() int8 {
	return rs2cd[(rp.runeStates>>8)&0b0101]
}

func (rp *runicPowerBar) AllRunesSpent() bool {
	return rp.runeStates&allSpent == allSpent
}

func (rp *runicPowerBar) OptimalRuneCost(cost RuneCost) RuneCost {
	var b, f, u, d int8

	if b = cost.Blood(); b > 0 {
		if cb := rp.CurrentBloodRunes(); cb < b {
			d += b - cb
			b = cb
		}
	}

	if f = cost.Frost(); f > 0 {
		if cf := rp.CurrentFrostRunes(); cf < f {
			d += f - cf
			f = cf
		}
	}

	if u = cost.Unholy(); u > 0 {
		if cu := rp.CurrentUnholyRunes(); cu < u {
			d += u - cu
			u = cu
		}
	}

	if d == 0 {
		return cost
	}

	d += cost.Death()

	if cd := rp.CurrentDeathRunes(); cd >= d {
		return NewRuneCost(cost.RunicPower(), b, f, u, d)
	}

	return 0
}

// spendRuneCost takes the cost the way Spell::TakeRunePower does. A missed spell's runes come back after
// RUNE_MISS_COOLDOWN, and its death runes stay death.
func (rp *runicPowerBar) spendRuneCost(sim *Simulation, spell *Spell, cost RuneCost, missed bool) (RuneChangeType, []int8) {
	if !cost.HasRune() {
		if rpc := cost.RunicPower(); rpc > 0 {
			rp.spendRunicPower(sim, float64(rpc)/10, spell.RunicPowerMetrics())
		}
		return None, nil
	}

	b, f, u, d := cost.Blood(), cost.Frost(), cost.Unholy(), cost.Death()
	slots := make([]int8, 0, b+f+u+d)
	for i := int8(0); i < b; i++ {
		slots = append(slots, rp.spendRune(sim, 0, spell.BloodRuneMetrics(), missed))
	}
	for i := int8(0); i < f; i++ {
		slots = append(slots, rp.spendRune(sim, 2, spell.FrostRuneMetrics(), missed))
	}
	for i := int8(0); i < u; i++ {
		slots = append(slots, rp.spendRune(sim, 4, spell.UnholyRuneMetrics(), missed))
	}
	if d > 0 {
		modified := spell.Cost.(*RuneCostImpl).modified
		// what the plain runes left owed, by the slot pair's base type: blood, frost, unholy
		owed := [3]int8{modified.Blood() - b, modified.Frost() - f, modified.Unholy() - u}
		left := owed[0] + owed[1] + owed[2]
		// first the death runes whose base type is still owed, then any, both in the server's slot order:
		// blood, unholy, frost
		for _, anyBase := range [...]bool{false, true} {
			for _, slot := range [...]int8{0, 1, 4, 5, 2, 3} {
				base := slot / 2
				if left == 0 || rp.runeStates&isSpentDeath[slot] != isDeaths[slot] || !anyBase && owed[base] == 0 {
					continue
				}
				slots = append(slots, rp.spendDeathRune(sim, slot, spell.DeathRuneMetrics(), missed))
				left--
				if !anyBase {
					owed[base]--
				}
			}
		}
	}

	if rpc := cost.RunicPower(); rpc > 0 {
		rp.AddRunicPower(sim, float64(rpc)/10, spell.RunicPowerMetrics())
	}

	return SpendRune, slots
}

func (rp *runicPowerBar) typeAmount(metrics *ResourceMetrics) (string, int8) {
	switch metrics.Type {
	case proto.ResourceType_ResourceTypeDeathRune:
		return "death", rp.CurrentDeathRunes()
	case proto.ResourceType_ResourceTypeBloodRune:
		return "blood", rp.CurrentBloodRunes()
	case proto.ResourceType_ResourceTypeFrostRune:
		return "frost", rp.CurrentFrostRunes()
	case proto.ResourceType_ResourceTypeUnholyRune:
		return "unholy", rp.CurrentUnholyRunes()
	default:
		panic("invalid metrics for rune gaining")
	}
}

// gainRuneMetrics should be called after gaining the rune
func (rp *runicPowerBar) gainRuneMetrics(sim *Simulation, metrics *ResourceMetrics, gainAmount int8) {
	metrics.AddEvent(float64(gainAmount), float64(gainAmount))

	if sim.Log != nil {
		name, currRunes := rp.typeAmount(metrics)
		rp.unit.Log(sim, "Gained %0.3f %s rune from %s (%d --> %d).", float64(gainAmount), name, metrics.ActionID, currRunes-gainAmount, currRunes)
	}
}

// spendRuneMetrics should be called after spending the rune
func (rp *runicPowerBar) spendRuneMetrics(sim *Simulation, metrics *ResourceMetrics, spendAmount int8) {
	metrics.AddEvent(-float64(spendAmount), -float64(spendAmount))

	if sim.Log != nil {
		name, currRunes := rp.typeAmount(metrics)
		rp.unit.Log(sim, "Spent 1.000 %s rune from %s (%d --> %d).", name, metrics.ActionID, currRunes+spendAmount, currRunes)
	}
}

func (rp *runicPowerBar) regenRune(sim *Simulation, regenAt time.Duration, slot int8) {
	rp.runeStates ^= isSpents[slot] // unset spent flag for this rune.
	rp.runeMeta[slot].lastRegenTime = regenAt
	rp.runeMeta[slot].regenAt = NeverExpires

	metrics := rp.bloodRuneGainMetrics
	if rp.runeStates&isDeaths[slot] > 0 {
		metrics = rp.deathRuneGainMetrics
	} else if slot == 2 || slot == 3 {
		metrics = rp.frostRuneGainMetrics
	} else if slot == 4 || slot == 5 {
		metrics = rp.unholyRuneGainMetrics
	}

	rp.gainRuneMetrics(sim, metrics, 1)
}

func (rp *runicPowerBar) RegenAllRunes(sim *Simulation) {
	changeType := None
	for i := range rp.runeMeta {
		if rp.runeStates&isSpents[i] > 0 {
			rp.regenRune(sim, sim.CurrentTime, int8(i))
			changeType = GainRune
		}
	}

	rp.maybeFireChange(sim, changeType)
}

// RuneGraceAt is how much sooner a rune spent at `at` comes back.
func (rp *runicPowerBar) RuneGraceAt(slot int8, at time.Duration) time.Duration {
	return runeGraceCut(rp.runeMeta[slot].lastRegenTime, at)
}

// runeGraceCut is Player::GetRuneBaseCooldown's cut for the time a rune sat ready, capped at 2.5 s, of
// which the first 250 ms count as spent at once.
func runeGraceCut(lastRegenTime, at time.Duration) time.Duration {
	// pre-pull casts should not get rune-grace
	if at <= 0 || lastRegenTime <= 0 {
		return 0
	}
	return max(0, min(time.Millisecond*2500, at-lastRegenTime)-time.Millisecond*250)
}

// runeMissCooldown is RUNE_MISS_COOLDOWN, what a rune spent on a miss is down for, grace aside.
const runeMissCooldown = time.Millisecond * 1500

func (rp *runicPowerBar) launchRuneRegen(sim *Simulation, slot int8, missed bool) {
	if missed {
		rp.runeMeta[slot].regenAt = sim.CurrentTime + runeMissCooldown
	} else {
		rp.runeMeta[slot].regenAt = sim.CurrentTime + rp.runeCooldown(rp.RuneGraceAt(slot, sim.CurrentTime))
		rp.runeMeta[slot].spentAt = sim.CurrentTime
	}

	rp.launchPA(sim, rp.runeMeta[slot].regenAt)
}

func (rp *runicPowerBar) launchPA(sim *Simulation, at time.Duration) {
	if rp.pa != nil {
		if at >= rp.pa.NextActionAt {
			return
		}
		// If this new regen is before currently scheduled one, we must cancel old regen and start a new one.
		rp.pa.Cancel(sim)
	}

	pa := &PendingAction{
		NextActionAt: at,
		Priority:     ActionPriorityRegen,
	}
	pa.OnAction = func(sim *Simulation) {
		if !pa.cancelled {
			// regenerate and revert
			rp.Advance(sim, sim.CurrentTime)

			// Check when we need next check
			pa.NextActionAt = min(rp.AnySpentRuneReadyAt(), rp.DeathRuneRevertAt())
			if pa.NextActionAt < NeverExpires {
				sim.AddPendingAction(pa)
			}
		}
	}
	rp.pa = pa
	sim.AddPendingAction(pa)

}

func (rp *runicPowerBar) Advance(sim *Simulation, newTime time.Duration) {
	changeType := None
	if rp.runeStates&allDeath > 0 {
		for i, rm := range rp.runeMeta {
			if rm.revertAt <= newTime {
				if rp.btSlot == int8(i) {
					rp.btSlot = -1 // this was the BT slot
				}
				rp.ConvertFromDeath(sim, int8(i))
				changeType |= ConvertFromDeath
			}
		}
	}

	if rp.runeStates&allSpent > 0 {
		for i, rm := range rp.runeMeta {
			if rm.regenAt <= newTime && rp.runeStates&isSpents[i] > 0 {
				rp.regenRune(sim, newTime, int8(i))
				changeType |= GainRune
			}
		}
	}

	rp.maybeFireChange(sim, changeType)
}

func (rp *runicPowerBar) spendRune(sim *Simulation, firstSlot int8, metrics *ResourceMetrics, missed bool) int8 {
	slot := rp.findReadyRune(firstSlot)
	rp.runeStates |= isSpents[slot]

	rp.spendRuneMetrics(sim, metrics, 1)
	rp.launchRuneRegen(sim, slot, missed)
	return slot
}

func (rp *runicPowerBar) findReadyRune(slot int8) int8 {
	if rp.runeStates&isSpentDeath[slot] == 0 {
		return slot
	}
	if rp.runeStates&isSpentDeath[slot+1] == 0 {
		return slot + 1
	}
	panic(fmt.Sprintf("findReadyRune(%d) - no slot found (runeStates = %12b)", slot, rp.runeStates))
}

func (rp *runicPowerBar) spendDeathRune(sim *Simulation, slot int8, metrics *ResourceMetrics, missed bool) int8 {
	if rp.btSlot != slot && !missed {
		rp.runeMeta[slot].revertAt = NeverExpires // disable revert at
		rp.runeMeta[slot].talent = noDeathRuneTalent
		rp.runeStates ^= isDeaths[slot] // clear death bit to revert.
	}

	// mark spent bit to spend
	rp.runeStates |= isSpents[slot]

	rp.spendRuneMetrics(sim, metrics, 1)
	rp.launchRuneRegen(sim, slot, missed)
	return slot
}

func (rp *runicPowerBar) IsBloodTappedRune(slot int8) bool {
	return slot == rp.btSlot
}

type RuneCostOptions struct {
	BloodRuneCost  int8
	FrostRuneCost  int8
	UnholyRuneCost int8
	RunicPowerCost float64
	RunicPowerGain float64
	Refundable     bool
}

type RuneCostImpl struct {
	BloodRuneCost  int8
	FrostRuneCost  int8
	UnholyRuneCost int8
	RunicPowerCost float64
	RunicPowerGain float64
	Refundable     bool

	runicPowerMetrics *ResourceMetrics
	bloodRuneMetrics  *ResourceMetrics
	frostRuneMetrics  *ResourceMetrics
	unholyRuneMetrics *ResourceMetrics
	deathRuneMetrics  *ResourceMetrics

	modified RuneCost // this cast's cost after the cost mods, before death runes stand in
}

func newRuneCost(spell *Spell, options RuneCostOptions) *RuneCostImpl {
	baseCost := float64(NewRuneCost(int16(math.Round(options.RunicPowerCost*10)), options.BloodRuneCost, options.FrostRuneCost, options.UnholyRuneCost, 0))
	spell.DefaultCast.Cost = baseCost
	spell.CurCast.Cost = baseCost
	spell.setStaticCost(0, 0, 10)

	return &RuneCostImpl{
		BloodRuneCost:  options.BloodRuneCost,
		FrostRuneCost:  options.FrostRuneCost,
		UnholyRuneCost: options.UnholyRuneCost,
		RunicPowerCost: options.RunicPowerCost,
		RunicPowerGain: options.RunicPowerGain,
		Refundable:     options.Refundable,

		runicPowerMetrics: Ternary(options.RunicPowerCost > 0 || options.RunicPowerGain > 0, spell.Unit.NewRunicPowerMetrics(spell.ActionID), nil),
		bloodRuneMetrics:  Ternary(options.BloodRuneCost > 0, spell.Unit.NewBloodRuneMetrics(spell.ActionID), nil),
		frostRuneMetrics:  Ternary(options.FrostRuneCost > 0, spell.Unit.NewFrostRuneMetrics(spell.ActionID), nil),
		unholyRuneMetrics: Ternary(options.UnholyRuneCost > 0, spell.Unit.NewUnholyRuneMetrics(spell.ActionID), nil),
		deathRuneMetrics:  spell.Unit.NewDeathRuneMetrics(spell.ActionID),
	}
}

// applyCostMods is the base cost (runic power in tenths) after the cost mods: the runic power through
// SpellInfo::CalcPowerCost like any other power, each rune count through the spell mods alone.
func (rc *RuneCostImpl) applyCostMods(spell *Spell, base RuneCost) RuneCost {
	if base == 0 {
		return 0
	}
	var rp int16
	if base.RunicPower() > 0 {
		rp = int16(math.Round(spell.ApplyCostModifiers(float64(base.RunicPower())/10) * 10))
	}
	if !base.HasRune() {
		return NewRuneCost(rp, 0, 0, 0, 0)
	}
	ps := &spell.Unit.PseudoStats
	pct := spell.costStaticPct + (spell.CostMultiplier - 1) + (ps.CostMultiplier - 1)
	flat := (spell.costStaticFlat + spell.CostFlat + ps.CostFlat) * spell.costUnits()
	return NewRuneCost(rp, runeCountFor(base.Blood(), pct, flat), runeCountFor(base.Frost(), pct, flat),
		runeCountFor(base.Unholy(), pct, flat), base.Death())
}

func (rc *RuneCostImpl) MeetsRequirement(_ *Simulation, spell *Spell) bool {
	cost := rc.applyCostMods(spell, RuneCost(spell.CurCast.Cost))
	rc.modified = cost
	spell.CurCast.Cost = float64(cost)
	if cost == 0 {
		return true
	}

	if !cost.HasRune() {
		if float64(cost.RunicPower())/10 > spell.Unit.CurrentRunicPower() {
			return false
		}
	}

	optCost := spell.Unit.OptimalRuneCost(cost)
	if optCost == 0 { // no combo of runes to fulfill cost
		return false
	}
	spell.CurCast.Cost = float64(optCost) // assign chosen runes to the cost
	return true
}

func (rc *RuneCostImpl) CostFailureReason(_ *Simulation, _ *Spell) string {
	return "not enough RP or runes"
}

func (rc *RuneCostImpl) SpendCost(sim *Simulation, spell *Spell) {
	// a refundable spell pays on its main target's result (spendOnResult)
	if rc.Refundable {
		return
	}
	changeType, _ := spell.Unit.spendRuneCost(sim, spell, RuneCost(spell.CurCast.Cost), false)
	spell.Unit.maybeFireChange(sim, changeType)
	rc.gainRunicPower(sim, spell)
}

// gainRunicPower is Spell::TakeRunePower's gain, which needs no cost left to pay (Freezing Fog).
func (rc *RuneCostImpl) gainRunicPower(sim *Simulation, spell *Spell) {
	if rc.RunicPowerGain > 0 && spell.DefaultCast.Cost > 0 {
		spell.Unit.AddRunicPower(sim, rc.RunicPowerGain, spell.RunicPowerMetrics())
	}
}

// spendOnResult pays a refundable spell's cost as Spell::TakePower does on its main target's result: a miss,
// dodge or parry still takes the runes, for RUNE_MISS_COOLDOWN, but gives no runic power, and of a runic
// power cost it takes only irand(0, cost / 4).
func (rc *RuneCostImpl) spendOnResult(sim *Simulation, spell *Spell, result *SpellResult) (RuneChangeType, []int8, bool) {
	// a full block counts as a hit
	hit := result.Landed() || result.Outcome.Matches(OutcomeBlock)
	var changeType RuneChangeType
	var slots []int8
	cost := RuneCost(spell.CurCast.Cost) // cost was already optimized in RuneSpell.Cast
	switch {
	case cost == 0:
	case !hit && !cost.HasRune():
		// in tenths, like the server's power
		if spent := int(sim.RandomFloat("Runic Power on a Miss") * float64(cost.RunicPower()/4+1)); spent > 0 {
			spell.Unit.spendRunicPower(sim, float64(spent)/10, spell.RunicPowerMetrics())
		}
	default:
		changeType, slots = spell.Unit.spendRuneCost(sim, spell, cost, !hit)
	}
	if hit {
		rc.gainRunicPower(sim, spell)
	}
	return changeType, slots, hit
}

func (rc *RuneCostImpl) spendRefundableCost(sim *Simulation, spell *Spell, result *SpellResult) {
	changeType, _, _ := rc.spendOnResult(sim, spell, result)
	spell.Unit.maybeFireChange(sim, changeType)
}

func (spell *Spell) SpendRefundableCost(sim *Simulation, result *SpellResult) {
	spell.Cost.(*RuneCostImpl).spendRefundableCost(sim, spell, result)
}

func (rc *RuneCostImpl) spendRefundableCostAndConvertBloodRune(sim *Simulation, spell *Spell, result *SpellResult, convertChance float64) {
	changeType, _, hit := rc.spendOnResult(sim, spell, result)
	if hit && sim.Proc(convertChance, deathRuneTalentLabel) {
		changeType |= spell.Unit.procDeathRuneTalent(sim, bloodDeathRuneTalent)
	}
	spell.Unit.maybeFireChange(sim, changeType)
}

func (spell *Spell) SpendRefundableCostAndConvertBloodRune(sim *Simulation, result *SpellResult, convertChance float64) {
	spell.Cost.(*RuneCostImpl).spendRefundableCostAndConvertBloodRune(sim, spell, result, convertChance)
}

func (rc *RuneCostImpl) spendRefundableCostAndConvertFrostOrUnholyRune(sim *Simulation, spell *Spell, result *SpellResult, convertChance float64) {
	changeType, _, hit := rc.spendOnResult(sim, spell, result)
	if hit && sim.Proc(convertChance, deathRuneTalentLabel) {
		changeType |= spell.Unit.procDeathRuneTalent(sim, frostUnholyDeathRuneTalent)
	}
	spell.Unit.maybeFireChange(sim, changeType)
}

func (spell *Spell) SpendRefundableCostAndConvertFrostOrUnholyRune(sim *Simulation, result *SpellResult, convertChance float64) {
	spell.Cost.(*RuneCostImpl).spendRefundableCostAndConvertFrostOrUnholyRune(sim, spell, result, convertChance)
}

const deathRuneTalentLabel = "Blood of The North / Reaping / DRM"

// The slots each death rune talent looks through, in the server's order (blood, unholy, frost), and how many
// runes one proc turns.
var deathRuneTalentSlots = [...]struct {
	slots []int8
	runes int
}{
	bloodDeathRuneTalent:       {[]int8{0, 1}, 1},
	frostUnholyDeathRuneTalent: {[]int8{4, 5, 2, 3}, 2},
}

// procDeathRuneTalent is a spell_dk_death_rune proc: it restarts the talent's periodic, which pushes back the
// revert of every rune the talent has turned, then turns the runes of its base types that a landed spell just
// put on cooldown. A proc that turns none (a Blood Strike paid with a frost death rune) still restarts it. A
// rune Blood Tap turned becomes the talent's, so Blood Tap running out no longer reverts it.
func (rp *runicPowerBar) procDeathRuneTalent(sim *Simulation, talent deathRuneTalent) RuneChangeType {
	// the player's aura update counts the periodic down, so it reverts on a world tick
	revertAt := sim.NextWorldTick(sim.CurrentTime + deathRuneTalentPeriod)
	pending := false
	for i := range rp.runeMeta {
		if rp.runeMeta[i].talent == talent {
			rp.runeMeta[i].revertAt = revertAt
			pending = true
		}
	}
	changeType := None
	left := deathRuneTalentSlots[talent].runes
	for _, slot := range deathRuneTalentSlots[talent].slots {
		if left == 0 {
			break
		}
		if rp.runeMeta[slot].spentAt != sim.CurrentTime || rp.runeStates&isSpents[slot] == 0 {
			continue
		}
		left--
		if rp.btSlot == slot {
			rp.btSlot = -1
		}
		rp.ConvertToDeath(sim, slot, revertAt)
		rp.runeMeta[slot].talent = talent
		changeType |= ConvertToDeath
		pending = true
	}
	if pending {
		rp.launchPA(sim, revertAt)
	}
	return changeType
}

// RollBloodDeathRunes rolls Blood of the North and Reaping for a landed spell in their mask that pays no runes
// of its own: Threat of Thassarian's off-hand Blood Strike. Both talents can proc from procs, and their 100 ms
// cooldown only starts on a proc, so the off hand rolls again when the main hand's roll failed.
func (rp *runicPowerBar) RollBloodDeathRunes(sim *Simulation, chance float64) {
	if sim.Proc(chance, deathRuneTalentLabel) {
		rp.maybeFireChange(sim, rp.procDeathRuneTalent(sim, bloodDeathRuneTalent))
	}
}

// RollFrostUnholyDeathRunes is the same for Death Rune Mastery, which has no cooldown: the off-hand Death
// Strike and Obliterate.
func (rp *runicPowerBar) RollFrostUnholyDeathRunes(sim *Simulation, chance float64) {
	if sim.Proc(chance, deathRuneTalentLabel) {
		rp.maybeFireChange(sim, rp.procDeathRuneTalent(sim, frostUnholyDeathRuneTalent))
	}
}

func (rc *RuneCostImpl) IssueRefund(_ *Simulation, _ *Spell) {
	// nothing to give back: a refundable spell pays on its result (spendOnResult)
}

func (spell *Spell) RunicPowerMetrics() *ResourceMetrics {
	return spell.Cost.(*RuneCostImpl).runicPowerMetrics
}

func (spell *Spell) BloodRuneMetrics() *ResourceMetrics {
	return spell.Cost.(*RuneCostImpl).bloodRuneMetrics
}

func (spell *Spell) FrostRuneMetrics() *ResourceMetrics {
	return spell.Cost.(*RuneCostImpl).frostRuneMetrics
}

func (spell *Spell) UnholyRuneMetrics() *ResourceMetrics {
	return spell.Cost.(*RuneCostImpl).unholyRuneMetrics
}

func (spell *Spell) DeathRuneMetrics() *ResourceMetrics {
	return spell.Cost.(*RuneCostImpl).deathRuneMetrics
}
