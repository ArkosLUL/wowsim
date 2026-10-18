package core

import (
	"testing"
	"time"
)

// A player's missile lands on his first world tick at or after the tick its flight starts on plus the flight
// (SpellEvent::Execute). It starts on the tick the session casts it in, or its hardcast lands in, and on the
// next one when it's cast later in his update or off a landing.
func TestPlayerMissileLandsOnHisWorldTick(t *testing.T) {
	const flight = 825 * time.Millisecond // 18 yd less the 1.5 yd reach, at 20 yd/s
	for _, tc := range []struct {
		name string
		live bool
	}{
		{"10 ms ticks", false},
		{"live", true},
	} {
		launched, landed := map[string]time.Duration{}, map[string]time.Duration{}
		var missile, hardcast *Spell
		fire := func(sim *Simulation, label string, spell *Spell, then func(*Simulation)) {
			launched[label] = sim.CurrentTime
			spell.WaitTravelTime(sim, func(sim *Simulation) {
				landed[label] = sim.CurrentTime
				if then != nil {
					then(sim)
				}
			})
		}
		setup := func(a *timingTestAgent) {
			a.DistanceFromTarget = 18
			missile = &Spell{Unit: &a.Unit, MissileSpeed: 20}
			config := castConfig(testSpellNoServerData, ms(1500))
			config.MissileSpeed = 20
			config.ApplyEffects = func(sim *Simulation, _ *Unit, spell *Spell) {
				fire(sim, "hardcast", spell, nil)
				fire(sim, "off the hardcast", missile, nil)
			}
			hardcast = a.RegisterSpell(config)

			a.EnableAutoAttacks(a, AutoAttackOptions{Ranged: testWeapon(3), AutoSwingRanged: true})
			a.AutoAttacks.RangedConfig().MissileSpeed = 20
			a.AutoAttacks.RangedConfig().ApplyEffects = func(sim *Simulation, _ *Unit, spell *Spell) {
				if _, ok := launched["Auto Shot"]; !ok {
					fire(sim, "Auto Shot", spell, nil)
				}
			}
		}
		var sim *Simulation
		var a *timingTestAgent
		if tc.live {
			sim, a = newSettingsTestSim(t, nil, setup)
		} else {
			sim, a = newTickTestSim(t, 100, 10, setup)
		}
		sim.PrePull()

		// 3 ms into a world tick
		at(sim, sim.NextWorldTick(ms(2000))+ms(3), func(sim *Simulation) {
			fire(sim, "session", missile, func(sim *Simulation) { fire(sim, "off a landing", missile, nil) })
		})
		at(sim, ms(4000), func(sim *Simulation) {
			if !hardcast.Cast(sim, a.CurrentTarget) {
				t.Fatalf("%s: the hardcast failed", tc.name)
			}
		})
		runUntil(sim, 9*time.Second)

		next := func(t time.Duration) time.Duration { return sim.NextWorldTick(t + 1) }
		for label, start := range map[string]time.Duration{
			"session":          sim.NextWorldTick(launched["session"]),
			"hardcast":         launched["hardcast"],
			"off the hardcast": next(launched["off the hardcast"]),
			"off a landing":    next(launched["off a landing"]),
			"Auto Shot":        next(launched["Auto Shot"]),
		} {
			got, ok := landed[label]
			if !ok {
				t.Errorf("%s: %s never landed", tc.name, label)
				continue
			}
			if want := sim.NextWorldTick(start + flight); got != want {
				t.Errorf("%s: %s launched at %v landed at %v, want %v: the first tick from %v plus the flight",
					tc.name, label, launched[label], got, want, start)
			}
		}
	}
}

// With exact timing, and for anyone but a player, a missile lands its flight after the launch.
func TestMissileLandsAfterItsFlightOffTheWorldTick(t *testing.T) {
	const flight = 825 * time.Millisecond
	for _, tc := range []struct {
		name               string
		mapMs, worldTickMs int32
		unitType           UnitType
	}{
		{"exact timing", 0, 0, PlayerUnit},
		{"a pet", 100, 10, PetUnit},
	} {
		sim, _ := newTickTestSim(t, tc.mapMs, tc.worldTickMs, nil)
		spell := &Spell{Unit: &Unit{Type: tc.unitType, DistanceFromTarget: 18}, MissileSpeed: 20}
		launchAt := ms(2003)
		var landed time.Duration
		at(sim, launchAt, func(sim *Simulation) {
			spell.WaitTravelTime(sim, func(sim *Simulation) { landed = sim.CurrentTime })
		})
		runUntil(sim, 5*time.Second)
		if want := launchAt + flight; landed != want {
			t.Errorf("%s: landed at %v, want %v", tc.name, landed, want)
		}
	}
}
