package core

import (
	"slices"
	"testing"
	"time"
)

// A refresh that restarts the tick timer arms the dot's tick action again, the same one when nothing
// queued can still run it. However it's re-armed, from its own tick, with its next tick queued or
// after the dot ended, it ticks on the new timer only.
func TestReArmedDotTicksOnlyOnItsNewTimer(t *testing.T) {
	var spell *Spell
	var ticks *[]time.Duration
	resetOnTick := false
	sim, _ := newTickTestSim(t, 0, 0, func(a *timingTestAgent) {
		spell, ticks = dotSpell(a, SpellFlagNone, DotConfig{
			Aura:          Aura{Label: "dot"},
			NumberOfTicks: 3,
			TickLength:    time.Second,
			TicksCanCrit:  false,
			OnTick: func(sim *Simulation, _ *Unit, dot *Dot) {
				if resetOnTick {
					resetOnTick = false
					dot.ApplyOrReset(sim)
				}
			},
		})
	})
	sim.PrePull()
	dot := spell.Dot(sim.Encounter.TargetUnits[0])
	at(sim, 0, func(sim *Simulation) { dot.Apply(sim) })
	// the tick due at 2 s is queued
	at(sim, ms(1500), func(sim *Simulation) { dot.ApplyOrReset(sim) })
	// the tick at 3.5 s restarts the timer itself
	at(sim, ms(3200), func(sim *Simulation) { resetOnTick = true })
	at(sim, ms(8000), func(sim *Simulation) { dot.Apply(sim) })
	runUntil(sim, 15*time.Second)

	want := []time.Duration{
		ms(1000),
		ms(2500), ms(3500),
		ms(4500), ms(5500), ms(6500),
		ms(9000), ms(10000), ms(11000),
	}
	if !slices.Equal(*ticks, want) {
		t.Errorf("ticks at %v, want %v", *ticks, want)
	}
	if dot.IsActive() {
		t.Error("the dot should have ended at 11 s")
	}
}

func TestReArmingADotsTicksAllocatesNothing(t *testing.T) {
	var spell *Spell
	sim, _ := newTickTestSim(t, 0, 0, func(a *timingTestAgent) {
		spell, _ = dotSpell(a, SpellFlagNone, DotConfig{
			Aura:          Aura{Label: "dot"},
			NumberOfTicks: 3,
			TickLength:    time.Second,
			TicksCanCrit:  false,
		})
	})
	sim.PrePull()
	dot := spell.Dot(sim.Encounter.TargetUnits[0])
	at(sim, 0, func(sim *Simulation) { dot.Apply(sim) })
	runUntil(sim, ms(1500))

	// the queue and the slot table grow by one stale entry a refresh, which append amortizes away
	if allocs := testing.AllocsPerRun(100, func() { dot.ApplyOrReset(sim) }); allocs != 0 {
		t.Errorf("a refresh made %v allocations, want none", allocs)
	}
}

// A refresh that comes while the clock is advancing to the dot's own queued tick can't take that action
// back: it makes another, and the old one does nothing when it gets there.
func TestAReArmDuringTheAdvanceToItsTick(t *testing.T) {
	var spell *Spell
	var ticks *[]time.Duration
	var helper *Aura
	sim, a := newTickTestSim(t, 0, 0, func(a *timingTestAgent) {
		spell, ticks = dotSpell(a, SpellFlagNone, DotConfig{
			Aura:          Aura{Label: "dot"},
			NumberOfTicks: 3,
			TickLength:    time.Second,
			TicksCanCrit:  false,
		})
	})
	sim.PrePull()
	dot := spell.Dot(sim.Encounter.TargetUnits[0])
	// the aura runs out at 1.9 s, between the dot's first tick (1 s) and its second (2 s), and refreshes the dot
	// as the clock moves to the 2 s tick
	helper = a.RegisterAura(Aura{
		Label:    "refresher",
		Duration: ms(1900),
		OnExpire: func(_ *Aura, sim *Simulation) {
			dot.ApplyOrReset(sim)
		},
	})
	at(sim, 0, func(sim *Simulation) {
		dot.Apply(sim)
		helper.Activate(sim)
	})
	runUntil(sim, 10*time.Second)

	want := []time.Duration{ms(1000), ms(2900), ms(3900), ms(4900)}
	if !slices.Equal(*ticks, want) {
		t.Errorf("ticks at %v, want %v", *ticks, want)
	}
}
