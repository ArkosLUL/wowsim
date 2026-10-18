package core

import (
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/wowsims/wotlk/sim/core/stats"
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

// m_Events keys a player's hardcast at the tick before + 1 ms and a landing at its go + flight, so a hardcast
// completing on a tick goes ahead of a landing due on it, even one launched before the cast began.
func TestHardcastCompletesAheadOfALandingOnItsTick(t *testing.T) {
	const flight = 825 * time.Millisecond
	var missile, hardcast *Spell
	var order []string
	sim, a := newTickTestSim(t, 100, 10, func(a *timingTestAgent) {
		a.DistanceFromTarget = 18
		missile = &Spell{Unit: &a.Unit, MissileSpeed: 20}
		config := castConfig(testSpellNoServerData, ms(500))
		config.ApplyEffects = func(sim *Simulation, _ *Unit, _ *Spell) {
			order = append(order, "hardcast at "+sim.CurrentTime.String())
		}
		hardcast = a.RegisterSpell(config)
	})
	sim.PrePull()

	launch := sim.NextWorldTick(ms(1000))
	lands := sim.NextWorldTick(launch + flight)
	at(sim, launch, func(sim *Simulation) {
		missile.WaitTravelTime(sim, func(sim *Simulation) {
			order = append(order, "landing at "+sim.CurrentTime.String())
		})
	})
	at(sim, lands-ms(500), func(sim *Simulation) {
		if !hardcast.Cast(sim, a.CurrentTarget) {
			t.Fatal("the hardcast failed")
		}
	})
	runUntil(sim, lands+ms(100))

	want := []string{"hardcast at " + lands.String(), "landing at " + lands.String()}
	if len(order) != len(want) || order[0] != want[0] || order[1] != want[1] {
		t.Errorf("went %v, want %v", order, want)
	}
}

// With exact timing a missile lands its flight after the launch.
func TestMissileLandsAfterItsFlightWithExactTiming(t *testing.T) {
	const flight = 825 * time.Millisecond
	for _, unitType := range []UnitType{PlayerUnit, PetUnit} {
		sim, _ := newTickTestSim(t, 0, 0, nil)
		spell := &Spell{Unit: &Unit{Type: unitType, DistanceFromTarget: 18}, MissileSpeed: 20}
		launchAt := ms(2003)
		var landed time.Duration
		at(sim, launchAt, func(sim *Simulation) {
			spell.WaitTravelTime(sim, func(sim *Simulation) { landed = sim.CurrentTime })
		})
		runUntil(sim, 5*time.Second)
		if want := launchAt + flight; landed != want {
			t.Errorf("unit type %v: landed at %v, want %v", unitType, landed, want)
		}
	}
}

// A creature's missile lands on its first map update at or after the update its flight starts on plus the
// flight: Map::Update only updates creatures, pets and guardians there. Its AI's instant, or one launched off
// its own landing or hardcast, starts on its next update, as the AI runs after Unit::Update's event pass. Its
// hardcast's own missile starts on the update the cast lands on, and one launched from a player's update or
// none starts on the creature's update at or after it, a cast copying the player's landing (the Rune Weapon's
// Death Coil) too.
func TestCreatureMissileLandsOnItsMapUpdate(t *testing.T) {
	for _, live := range []bool{false, true} {
		var pet *tickTestPet
		timingTestConstruct = func(a *timingTestAgent) {
			pet = &tickTestPet{Pet: NewPet("Pet", &a.Character, stats.Stats{}, func(stats.Stats) stats.Stats { return stats.Stats{} }, true, false)}
			a.AddPet(pet)
		}
		launched, landed := map[string]time.Duration{}, map[string]time.Duration{}
		var missile, instant, hardcast, copied, playerMissile *Spell
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
			pet.DistanceFromTarget = 18
			missile = &Spell{Unit: &pet.Unit, MissileSpeed: 20}
			config := castConfig(testSpellNoServerData, 0)
			config.MissileSpeed = 20
			config.ApplyEffects = func(sim *Simulation, _ *Unit, spell *Spell) {
				fire(sim, "AI instant", spell, func(sim *Simulation) { fire(sim, "off its landing", missile, nil) })
			}
			instant = pet.RegisterSpell(config)
			config = castConfig(testSpellNoServerData, ms(1500))
			config.ActionID.Tag = 1
			config.MissileSpeed = 20
			config.ApplyEffects = func(sim *Simulation, _ *Unit, spell *Spell) {
				fire(sim, "hardcast", spell, nil)
				fire(sim, "off the hardcast", missile, nil)
			}
			hardcast = pet.RegisterSpell(config)
			copied = pet.RegisterSpell(SpellConfig{
				ActionID:     ActionID{SpellID: testSpellNoServerData, Tag: 2},
				MissileSpeed: 20,
				ApplyEffects: func(sim *Simulation, _ *Unit, spell *Spell) {
					fire(sim, "cast off a player's landing", spell, nil)
				},
			})
			a.DistanceFromTarget = 18
			playerMissile = &Spell{Unit: &a.Unit, MissileSpeed: 20}
		}
		var sim *Simulation
		var a *timingTestAgent
		if live {
			sim, a = newSettingsTestSim(t, nil, setup)
		} else {
			sim, a = newTickTestSim(t, 100, 10, setup)
		}
		timingTestConstruct = nil
		sim.PrePull()

		cast := func(spell *Spell) func(*Simulation) {
			return func(sim *Simulation) {
				pet.GCD.Set(sim.CurrentTime)
				if !spell.Cast(sim, a.CurrentTarget) {
					t.Fatalf("live %v: %v failed", live, spell.ActionID)
				}
			}
		}
		at(sim, sim.NextServerTick(ms(2000)), cast(instant))
		at(sim, sim.NextServerTick(ms(4000)), cast(hardcast))
		// as the knight's Death Coil sets the Rune Weapon's copy off
		at(sim, ms(7003), func(sim *Simulation) {
			fire(sim, "player's", playerMissile, func(sim *Simulation) {
				fire(sim, "off a player's landing", missile, nil)
				if !copied.Cast(sim, a.CurrentTarget) {
					t.Fatalf("live %v: the copy failed", live)
				}
			})
		})
		at(sim, ms(10003), func(sim *Simulation) { fire(sim, "session", missile, nil) })
		runUntil(sim, 13*time.Second)

		flight := missile.TravelTime()
		next := func(t time.Duration) time.Duration { return sim.NextServerTick(t + 1) }
		for label, start := range map[string]time.Duration{
			"AI instant":                  next(launched["AI instant"]),
			"off its landing":             next(launched["off its landing"]),
			"hardcast":                    launched["hardcast"],
			"off the hardcast":            next(launched["off the hardcast"]),
			"off a player's landing":      sim.NextServerTick(launched["off a player's landing"]),
			"cast off a player's landing": sim.NextServerTick(launched["cast off a player's landing"]),
			"session":                     sim.NextServerTick(launched["session"]),
		} {
			got, ok := landed[label]
			if !ok {
				t.Errorf("live %v: %s never landed", live, label)
				continue
			}
			if want := sim.NextServerTick(start + flight); got != want {
				t.Errorf("live %v: %s launched at %v landed at %v, want %v: the first map update from %v plus the flight",
					live, label, launched[label], got, want, start)
			}
		}
	}
}

// A totem's missile, which the sim casts as the shaman, is the totem's hardcast's: it starts on the map update
// the cast lands on and lands on the map update, in the totem's update rather than the shaman's.
func TestTotemMissileLandsOnTheMapUpdate(t *testing.T) {
	sim, a := newTickTestSim(t, 100, 10, nil)
	a.DistanceFromTarget = 18
	spell := &Spell{Unit: &a.Unit, MissileSpeed: 20}
	spell.CastFromAnotherUnit()
	launchAt := ms(2003)
	var landed time.Duration
	at(sim, launchAt, func(sim *Simulation) {
		sim.updating = &a.Unit
		spell.WaitTravelTime(sim, func(sim *Simulation) { landed = sim.CurrentTime })
		sim.updating = nil
		for _, la := range sim.landings {
			if la.caster == &a.Unit && (la.playerEvent || la.Priority != 0) {
				t.Errorf("the landing is the player's event, priority %v", la.Priority)
			}
		}
	})
	runUntil(sim, 5*time.Second)
	if want := sim.NextServerTick(sim.NextServerTick(launchAt) + spell.TravelTime()); landed != want {
		t.Errorf("landed at %v, want %v", landed, want)
	}
}

// m_Events runs a caster's events due on one update by key: a cast waiting on its SpellEvent at the update before
// + 1 ms, then each landing at its go + flight, ties in the order they went in. A missile launched later with a
// shorter flight can land ahead of an earlier one on the same tick, and one launched from the SpellEvent starts on
// the next tick.
func TestLandingsOnOneTickGoByTheirKeys(t *testing.T) {
	var order []string
	var slow, fast, probe *Spell
	var lands time.Duration
	sim, a := newTickTestSim(t, 100, 10, func(a *timingTestAgent) {
		a.DistanceFromTarget = 18
		slow = &Spell{Unit: &a.Unit, MissileSpeed: 20}   // 825 ms
		fast = &Spell{Unit: &a.Unit, MissileSpeed: 29.8} // 553 ms
		probe = &Spell{Unit: &a.Unit, MissileSpeed: 20}
	})
	if slow.TravelTime() != ms(825) || fast.TravelTime() != ms(553) {
		t.Fatalf("flights %v and %v, want 825ms and 553ms", slow.TravelTime(), fast.TravelTime())
	}
	sim.PrePull()
	note := func(label string) func(*Simulation) {
		return func(sim *Simulation) { order = append(order, fmt.Sprintf("%s at %v", label, sim.CurrentTime-lands)) }
	}

	// the slow one's key is 825 ms past its tick and lands 5 ms later, the fast ones' 823, on the same tick
	start := sim.NextWorldTick(ms(2000))
	lands = start + ms(830)
	at(sim, start, func(sim *Simulation) { slow.WaitTravelTime(sim, note("slow")) })
	at(sim, start+ms(270), func(sim *Simulation) {
		fast.WaitTravelTime(sim, note("fast 1"))
		fast.WaitTravelTime(sim, note("fast 2"))
	})
	var probeLands time.Duration
	at(sim, start+ms(300), func(sim *Simulation) {
		sim.QueueSpellEvent(&a.Unit, lands, func(sim *Simulation) {
			note("spell event")(sim)
			probeLands, _ = probe.landsAt(sim, probe.TravelTime())
			probe.WaitTravelTime(sim, note("off the spell event"))
		})
	})
	runUntil(sim, lands+time.Second)

	want := []string{"spell event at 0s", "fast 1 at 0s", "fast 2 at 0s", "slow at 0s",
		fmt.Sprintf("off the spell event at %v", probeLands-lands)}
	if !slices.Equal(order, want) {
		t.Errorf("went %v, want %v", order, want)
	}
	if want := sim.NextWorldTick(lands+ms(10)+ms(825)) - lands; probeLands-lands != want {
		t.Errorf("the missile off the spell event lands %v after the tick, want %v", probeLands-lands, want)
	}
}

// A creature's hardcast completes ahead of its missile landing on the same map update, whichever was queued
// first: m_Events keys the cast's SpellEvent at the update before + 1 ms.
func TestCreatureHardcastCompletesAheadOfALandingOnItsUpdate(t *testing.T) {
	var pet *tickTestPet
	timingTestConstruct = func(a *timingTestAgent) {
		pet = &tickTestPet{Pet: NewPet("Pet", &a.Character, stats.Stats{}, func(stats.Stats) stats.Stats { return stats.Stats{} }, true, false)}
		a.AddPet(pet)
	}
	var missile, hardcast *Spell
	var order []string
	sim, a := newTickTestSim(t, 100, 10, func(a *timingTestAgent) {
		pet.DistanceFromTarget = 40
		missile = &Spell{Unit: &pet.Unit, MissileSpeed: 20}
		config := castConfig(testSpellNoServerData, ms(1500))
		config.ApplyEffects = func(sim *Simulation, _ *Unit, _ *Spell) {
			order = append(order, "hardcast at "+sim.CurrentTime.String())
		}
		hardcast = pet.RegisterSpell(config)
	})
	timingTestConstruct = nil
	sim.PrePull()

	launch := sim.NextServerTick(ms(1000)) + ms(3)
	lands := sim.NextServerTick(sim.NextServerTick(launch) + missile.TravelTime())
	if lands-ms(1500) <= launch {
		t.Fatalf("the cast would start at %v, before the launch at %v", lands-ms(1500), launch)
	}
	at(sim, launch, func(sim *Simulation) {
		missile.WaitTravelTime(sim, func(sim *Simulation) {
			order = append(order, "landing at "+sim.CurrentTime.String())
		})
	})
	at(sim, lands-ms(1500), func(sim *Simulation) {
		pet.GCD.Set(sim.CurrentTime)
		if !hardcast.Cast(sim, a.CurrentTarget) {
			t.Fatal("the hardcast failed")
		}
		if pet.Hardcast.Expires != lands {
			t.Fatalf("the hardcast lands at %v, want %v", pet.Hardcast.Expires, lands)
		}
	})
	runUntil(sim, lands+ms(100))

	if want := []string{"hardcast at " + lands.String(), "landing at " + lands.String()}; !slices.Equal(order, want) {
		t.Errorf("went %v, want %v", order, want)
	}
}
