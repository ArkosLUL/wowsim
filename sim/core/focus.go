package core

import (
	"fmt"
	"math"
	"time"

	"github.com/wowsims/wotlk/sim/core/proto"
)

const MaxFocus = 100.0

// Creature::Regenerate: a hunter pet gets 24 focus every PET_FOCUS_REGEN_INTERVAL, a creature on
// energy 20 every CREATURE_REGEN_INTERVAL.
const (
	petFocusRegenInterval = 4 * time.Second
	petFocusPerTick       = 24.0
	creatureEnergyPerTick = 20.0
)

// OnFocusGain is called any time focus is increased.
type OnFocusGain func(sim *Simulation)

// focusBar is a pet's focus, or the energy of one that has it for a power. Either regenerates a
// fixed amount each time its timer runs out, on the pet's own update (Pet.enable starts the timer).
type focusBar struct {
	unit *Unit

	regenPerTick  float64
	regenInterval time.Duration
	// energy comes from Creature::Update's regen timer, which runs once the AI has
	regenAfterAI bool

	currentFocus float64

	onFocusGain OnFocusGain

	regenMetrics  *ResourceMetrics
	refundMetrics *ResourceMetrics
}

// EnableFocusBar gives a hunter pet its focus. regenMultiplier is 1 plus the focus regen percent
// auras on it (Bestial Discipline), and the tick is whole.
func (unit *Unit) EnableFocusBar(regenMultiplier float64, onFocusGain OnFocusGain) {
	unit.focusBar = focusBar{
		unit:          unit,
		regenPerTick:  math.Trunc(petFocusPerTick * regenMultiplier),
		regenInterval: petFocusRegenInterval,
		onFocusGain:   onFocusGain,
		regenMetrics:  unit.NewEnergyMetrics(ActionID{OtherID: proto.OtherAction_OtherActionFocusRegen}),
		refundMetrics: unit.NewEnergyMetrics(ActionID{OtherID: proto.OtherAction_OtherActionRefund}),
	}
}

// EnableCreatureEnergyBar gives a pet whose power is energy (the DK's ghouls) a bar of it, kept as
// its focus.
func (unit *Unit) EnableCreatureEnergyBar(onFocusGain OnFocusGain) {
	unit.EnableFocusBar(1, onFocusGain)
	unit.focusBar.regenPerTick = creatureEnergyPerTick
	unit.focusBar.regenInterval = creatureRegenInterval
	unit.focusBar.regenAfterAI = true
}

func (unit *Unit) HasFocusBar() bool {
	return unit.focusBar.unit != nil
}

// FocusRegenTick is what the bar regenerates a tick, and how far apart the ticks are.
func (unit *Unit) FocusRegenTick() (amount float64, interval time.Duration) {
	return unit.focusBar.regenPerTick, unit.focusBar.regenInterval
}

func (fb *focusBar) CurrentFocus() float64 {
	return fb.currentFocus
}

func (fb *focusBar) AddFocus(sim *Simulation, amount float64, metrics *ResourceMetrics) {
	if amount < 0 {
		panic("Trying to add negative focus!")
	}

	newFocus := min(fb.currentFocus+amount, MaxFocus)
	metrics.AddEvent(amount, newFocus-fb.currentFocus)

	if sim.Log != nil {
		fb.unit.Log(sim, "Gained %0.3f focus from %s (%0.3f --> %0.3f).", amount, metrics.ActionID, fb.currentFocus, newFocus)
	}

	fb.currentFocus = newFocus

	if fb.onFocusGain != nil {
		fb.onFocusGain(sim)
	}
}

func (fb *focusBar) SpendFocus(sim *Simulation, amount float64, metrics *ResourceMetrics) {
	if amount < 0 {
		panic("Trying to spend negative focus!")
	}

	newFocus := fb.currentFocus - amount
	metrics.AddEvent(-amount, -amount)

	if sim.Log != nil {
		fb.unit.Log(sim, "Spent %0.3f focus from %s (%0.3f --> %0.3f).", amount, metrics.ActionID, fb.currentFocus, newFocus)
	}

	fb.currentFocus = newFocus
}

func (fb *focusBar) reset(_ *Simulation) {
	if fb.unit == nil {
		return
	}

	fb.currentFocus = MaxFocus
}

// regenTick is the bar's Creature::Regenerate, which leaves a full bar alone.
func (fb *focusBar) regenTick(sim *Simulation) {
	if fb.currentFocus >= MaxFocus {
		return
	}
	fb.AddFocus(sim, fb.regenPerTick, fb.regenMetrics)
}

type FocusCostOptions struct {
	Cost float64

	Multiplier float64 // see ManaCostOptions
	FlatMod    float64

	Refund        float64
	RefundMetrics *ResourceMetrics // Optional, will default to unit.FocusRefundMetrics if not supplied
}
type FocusCost struct {
	Refund          float64
	RefundMetrics   *ResourceMetrics
	ResourceMetrics *ResourceMetrics
}

func newFocusCost(spell *Spell, options FocusCostOptions) *FocusCost {
	spell.DefaultCast.Cost = options.Cost
	spell.setStaticCost(options.Multiplier, options.FlatMod, 1)
	if options.Refund > 0 && options.RefundMetrics == nil {
		options.RefundMetrics = spell.Unit.refundMetrics
	}
	return &FocusCost{
		Refund:          options.Refund,
		RefundMetrics:   options.RefundMetrics,
		ResourceMetrics: spell.Unit.NewFocusMetrics(spell.ActionID),
	}
}

func (fc *FocusCost) MeetsRequirement(_ *Simulation, spell *Spell) bool {
	spell.CurCast.Cost = spell.ApplyCostModifiers(spell.CurCast.Cost)
	return spell.Unit.CurrentFocus() >= spell.CurCast.Cost
}
func (fc *FocusCost) CostFailureReason(_ *Simulation, spell *Spell) string {
	return fmt.Sprintf("not enough focus (Current Focus = %0.03f, Focus Cost = %0.03f)", spell.Unit.CurrentFocus(), spell.CurCast.Cost)
}
func (fc *FocusCost) SpendCost(sim *Simulation, spell *Spell) {
	spell.Unit.SpendFocus(sim, spell.CurCast.Cost, fc.ResourceMetrics)
}
func (fc *FocusCost) IssueRefund(sim *Simulation, spell *Spell) {
	if fc.Refund > 0 {
		spell.Unit.AddFocus(sim, fc.Refund*spell.CurCast.Cost, fc.RefundMetrics)
	}
}
