package core

import (
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"
)

// A hardcast runs on every caster's hot path, so casting one and landing it must not allocate.
func TestHardcastDoesNotAllocate(t *testing.T) {
	for _, tc := range []struct {
		name    string
		spellID int32
	}{
		{"resets the swing", testSpellLightningBolt},
		{"keeps the swing", testSpellSlam},
	} {
		var spell *Spell
		sim, a := newTimingTestSim(t, 10, func(a *timingTestAgent) {
			spell = a.RegisterSpell(castConfig(tc.spellID, ms(1500)))
		})
		landings := 0
		spell.ApplyEffects = func(*Simulation, *Unit, *Spell) { landings++ }
		sim.PrePull()

		allocs := testing.AllocsPerRun(10, func() {
			if !spell.Cast(sim, a.CurrentTarget) {
				t.Fatalf("%s: cast failed at %v", tc.name, sim.CurrentTime)
			}
			for a.Hardcast.Expires != startingCDTime {
				if sim.Step() {
					t.Fatalf("%s: the sim ended before the cast landed", tc.name)
				}
			}
		})
		if landings != 11 {
			t.Errorf("%s: %d landings, want 11", tc.name, landings)
		}
		if allocs != 0 {
			t.Errorf("%s: %v allocations per hardcast, want 0", tc.name, allocs)
		}
	}
}

// Pets retry spells on cooldown all fight. With logs off a failed cast must not allocate, and with logs
// on it must still say why it failed.
func TestFailedCastDoesNotAllocate(t *testing.T) {
	var spell *Spell
	sim, a := newTimingTestSim(t, 10, func(a *timingTestAgent) {
		a.EnableEnergyBar(100)
		spell = a.RegisterSpell(SpellConfig{
			ActionID:   ActionID{SpellID: testSpellNoServerData},
			EnergyCost: EnergyCostOptions{Cost: 40},
			Cast: CastConfig{
				DefaultCast: Cast{GCD: GCDDefault},
				CD:          Cooldown{Timer: a.NewTimer(), Duration: 10 * time.Second},
				SharedCD:    Cooldown{Timer: a.NewTimer(), Duration: 10 * time.Second},
			},
		})
	})
	unit := &a.Unit
	now := sim.CurrentTime
	if now < 0 {
		t.Fatalf("starts at %v: a failure before the pull warns the rotation instead", now)
	}

	for _, tc := range []struct {
		name, reason string
		set          func()
	}{
		{"short on energy", "not enough energy", func() { unit.currentEnergy = 30 }},
		{"on cooldown", "still on cooldown", func() { spell.CD.Set(now + time.Second) }},
		{"on shared cooldown", "still on shared cooldown", func() { spell.SharedCD.Set(now + time.Second) }},
		{"on the GCD", "GCD on cooldown", func() { unit.GCD.Set(now + time.Second) }},
		{"casting", "casting/channeling", func() { unit.Hardcast.Expires = now + time.Second }},
	} {
		// a cast failing a later check has already started the cooldowns checked before it
		castFailing := func() {
			spell.CD.Reset()
			spell.SharedCD.Reset()
			unit.GCD.Reset()
			unit.Hardcast.Expires = startingCDTime
			unit.currentEnergy = 100
			tc.set()
			if spell.Cast(sim, a.CurrentTarget) {
				t.Fatalf("%s: the cast went through", tc.name)
			}
		}

		var logs []string
		sim.Log = func(format string, args ...interface{}) { logs = append(logs, fmt.Sprintf(format, args...)) }
		castFailing()
		if !slices.ContainsFunc(logs, func(line string) bool { return strings.Contains(line, "failed to cast: "+tc.reason) }) {
			t.Errorf("%s: logged %q, want a %q failure", tc.name, logs, tc.reason)
		}

		sim.Log = nil
		if allocs := testing.AllocsPerRun(10, castFailing); allocs != 0 {
			t.Errorf("%s: %v allocations per failed cast, want 0", tc.name, allocs)
		}
	}

	// before the pull a failure warns the rotation, logs or not
	rot := &APLRotation{unit: unit}
	unit.Rotation = rot
	sim.CurrentTime = -time.Second
	spell.CD.Set(0)
	if spell.Cast(sim, a.CurrentTarget) || !slices.ContainsFunc(rot.curWarnings, func(w string) bool { return strings.Contains(w, "still on cooldown") }) {
		t.Errorf("before the pull: warned %q, want a cooldown failure", rot.curWarnings)
	}
}

// Off-GCD cooldowns with no cast of their own (makeCastFuncSimple) fail the same way.
func TestFailedSimpleCastDoesNotAllocate(t *testing.T) {
	var spell *Spell
	sim, a := newTimingTestSim(t, 10, func(a *timingTestAgent) {
		spell = a.RegisterSpell(SpellConfig{
			ActionID: ActionID{SpellID: testSpellNoServerData},
			Cast: CastConfig{
				CD:       Cooldown{Timer: a.NewTimer(), Duration: 10 * time.Second},
				SharedCD: Cooldown{Timer: a.NewTimer(), Duration: 10 * time.Second},
			},
		})
	})
	unit := &a.Unit
	now := sim.CurrentTime
	if now < 0 {
		t.Fatalf("starts at %v: a failure before the pull warns the rotation instead", now)
	}
	if spell.DefaultCast != (Cast{}) {
		t.Fatalf("has a default cast %+v, so it doesn't take the simple cast path", spell.DefaultCast)
	}

	for _, tc := range []struct {
		name, reason string
		set          func()
	}{
		{"on cooldown", "still on cooldown", func() { spell.CD.Set(now + time.Second) }},
		{"on shared cooldown", "still on shared cooldown", func() { spell.SharedCD.Set(now + time.Second) }},
	} {
		castFailing := func() {
			spell.CD.Reset()
			spell.SharedCD.Reset()
			tc.set()
			if spell.Cast(sim, a.CurrentTarget) {
				t.Fatalf("%s: the cast went through", tc.name)
			}
		}

		var logs []string
		sim.Log = func(format string, args ...interface{}) { logs = append(logs, fmt.Sprintf(format, args...)) }
		castFailing()
		if !slices.ContainsFunc(logs, func(line string) bool { return strings.Contains(line, "failed to cast: "+tc.reason) }) {
			t.Errorf("%s: logged %q, want a %q failure", tc.name, logs, tc.reason)
		}

		sim.Log = nil
		if allocs := testing.AllocsPerRun(10, castFailing); allocs != 0 {
			t.Errorf("%s: %v allocations per failed cast, want 0", tc.name, allocs)
		}
	}

	rot := &APLRotation{unit: unit}
	unit.Rotation = rot
	sim.CurrentTime = -time.Second
	spell.SharedCD.Reset()
	spell.CD.Set(0)
	if spell.Cast(sim, a.CurrentTarget) || !slices.ContainsFunc(rot.curWarnings, func(w string) bool { return strings.Contains(w, "still on cooldown") }) {
		t.Errorf("before the pull: warned %q, want a cooldown failure", rot.curWarnings)
	}
}
