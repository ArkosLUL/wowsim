package core

import (
	"slices"
	"testing"
	"time"
)

func newRangedTimingSim(t *testing.T, mapUpdateMs int32, swingSpeed float64) (*Simulation, *AutoAttacks, *[]time.Duration) {
	t.Helper()
	sim, a := newTimingTestSim(t, mapUpdateMs, func(a *timingTestAgent) {
		a.EnableAutoAttacks(a, AutoAttackOptions{Ranged: testWeapon(swingSpeed), AutoSwingRanged: true})
	})
	sim.serverTickPhase = 0
	aa := &a.AutoAttacks
	shots := recordCasts(aa.RangedAuto())
	aa.ranged.setTimer(sim, 0)
	return sim, aa, shots
}

// Auto Shot fires in _UpdateSpells, before Unit::Update decrements the ranged timer, so one update
// after the timer runs out. It restarts the timer before that update's decrement too, which cancels
// out: shots land every NextServerTick of the attack time, like melee swings.
func TestAutoShotCadenceOnServerTicks(t *testing.T) {
	sim, _, shots := newRangedTimingSim(t, 100, 2.608)
	sim.PrePull()
	runUntil(sim, ms(8200))

	if want := []time.Duration{0, ms(2700), ms(5400), ms(8100)}; !slices.Equal(*shots, want) {
		t.Errorf("Auto Shots at %v, want %v", *shots, want)
	}
}

// Unit::ApplyAttackTimePercentMod keeps the fraction of the ranged timer left, as it does for melee.
func TestRangedHasteRescalesTheRunningTimer(t *testing.T) {
	sim, aa, shots := newRangedTimingSim(t, 0, 3)
	aa.ranged.setTimer(sim, ms(3000))
	at(sim, ms(1000), func(sim *Simulation) { aa.ranged.unit.MultiplyRangedSpeed(sim, 1.5) })
	sim.PrePull()
	runUntil(sim, ms(3500))

	// 2 s left of 3 s becomes 2 s left of 2 s: a third of the timer at 1.5x speed
	if want := []time.Duration{ms(2333)}; len(*shots) != 1 || (*shots)[0].Round(time.Millisecond) != want[0] {
		t.Errorf("Auto Shots at %v, want %v", *shots, want)
	}
}

// A SPELL_ATTR2_DO_NOT_RESET_COMBAT_TIMERS channel stops the ranged timer (Volley): what was left of
// it runs once the channel ends.
func TestSuspendedRangedTimerResumesWhereItStopped(t *testing.T) {
	sim, aa, shots := newRangedTimingSim(t, 0, 3)
	aa.ranged.setTimer(sim, ms(3000))
	at(sim, ms(1000), aa.SuspendRangedTimer)
	at(sim, ms(4000), aa.ResumeRangedTimer)
	sim.PrePull()
	runUntil(sim, ms(6500))

	if want := []time.Duration{ms(6000)}; !slices.Equal(*shots, want) {
		t.Errorf("Auto Shots at %v, want %v", *shots, want)
	}
}

// The channel doesn't hold Auto Shot: a shot due on the update that takes the channel still goes, and
// the timer it restarts stands still until the channel ends.
func TestSuspendedRangedTimerLetsADueShotGo(t *testing.T) {
	sim, aa, shots := newRangedTimingSim(t, 100, 3)
	aa.ranged.setTimer(sim, ms(1000))
	at(sim, ms(950), aa.SuspendRangedTimer)
	at(sim, ms(4000), aa.ResumeRangedTimer)
	sim.PrePull()
	runUntil(sim, ms(7500))

	if want := []time.Duration{ms(1000), ms(7000)}; !slices.Equal(*shots, want) {
		t.Errorf("Auto Shots at %v, want %v", *shots, want)
	}
}

// A haste change mid-channel scales what's left of the suspended timer.
func TestRangedHasteScalesASuspendedTimer(t *testing.T) {
	sim, aa, shots := newRangedTimingSim(t, 0, 3)
	aa.ranged.setTimer(sim, ms(3000))
	at(sim, ms(1000), aa.SuspendRangedTimer)
	at(sim, ms(2000), func(sim *Simulation) { aa.ranged.unit.MultiplyRangedSpeed(sim, 2) })
	at(sim, ms(4000), aa.ResumeRangedTimer)
	sim.PrePull()
	runUntil(sim, ms(5500))

	if want := []time.Duration{ms(5000)}; !slices.Equal(*shots, want) {
		t.Errorf("Auto Shots at %v, want %v", *shots, want)
	}
}
