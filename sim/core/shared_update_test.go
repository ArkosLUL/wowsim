package core

import (
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/wowsims/wotlk/sim/core/stats"
)

// Unit::Update runs the attack timers down after its event pass, so the timer a player's landing cast
// restarts has that world tick off it at once. A cast the session makes loses its tick the same way.
func TestPlayerSwingAfterItsCastComesAWorldTickEarly(t *testing.T) {
	for _, tc := range []struct {
		name     string
		spellID  int32
		castTime time.Duration
	}{
		{"hardcast", testSpellExorcism, ms(1500)},
		{"instant", testSpellHeroicThrow, 0},
	} {
		var spell *Spell
		sim, a := newTickTestSim(t, 100, 10, func(a *timingTestAgent) {
			a.EnableAutoAttacks(a, AutoAttackOptions{MainHand: testWeapon(2), AutoSwingMelee: true})
			spell = a.RegisterSpell(castConfig(tc.spellID, tc.castTime))
		})
		swings, landings := recordCasts(a.AutoAttacks.MHAuto()), recordCasts(spell)
		sim.PrePull()
		// 3 ms into a world tick
		castAt := sim.NextWorldTick(ms(2500)) + ms(3)
		at(sim, castAt, func(sim *Simulation) {
			if !spell.Cast(sim, a.CurrentTarget) {
				t.Fatalf("%s: the cast failed", tc.name)
			}
		})
		runUntil(sim, 9*time.Second)

		if len(*landings) != 1 {
			t.Fatalf("%s: landings at %v, want one", tc.name, *landings)
		}
		landed := (*landings)[0]
		// the tick that takes the cast, or lands it
		taken := sim.NextWorldTick(landed)
		var after []time.Duration
		for _, swing := range *swings {
			if swing > landed {
				after = append(after, swing-taken)
			}
		}
		if want := []time.Duration{ms(1990), ms(3990)}; len(after) < 2 || !slices.Equal(after[:2], want) {
			t.Errorf("%s: swings %v after the tick at %v, want %v: the first a tick short of the 2 s", tc.name, after, taken, want)
		}
	}
}

// A cast from the swing's own APL call is the next tick's session's: that tick is the one the new timer
// loses, so the swing comes a whole attack time after this one.
func TestPlayerSwingAfterACastFromItsOwnUpdateKeepsItsTick(t *testing.T) {
	var spell *Spell
	sim, a := newTickTestSim(t, 100, 10, func(a *timingTestAgent) {
		a.EnableAutoAttacks(a, AutoAttackOptions{MainHand: testWeapon(2), AutoSwingMelee: true})
		spell = a.RegisterSpell(castConfig(testSpellHeroicThrow, 0))
	})
	swings := recordCasts(a.AutoAttacks.MHAuto())
	cast := false
	a.Rotation = &APLRotation{unit: &a.Unit, priorityList: []*APLAction{{impl: &testAPLAction{
		ready:   func(sim *Simulation) bool { return !cast && sim.CurrentTime > time.Second },
		execute: func(sim *Simulation) { cast = spell.Cast(sim, a.CurrentTarget) },
	}}}}
	sim.PrePull()
	runUntil(sim, 7*time.Second)

	if !cast || len(*swings) < 3 {
		t.Fatalf("cast %v, swings %v", cast, *swings)
	}
	if got := (*swings)[2] - (*swings)[1]; got != ms(2000) {
		t.Errorf("the swing after the cast came %v on, want the whole 2 s", got)
	}
}

// Creature::Update ticks the creature's auras in Unit::Update, ahead of the AI that swings.
func TestCreatureSwingsAfterTheTicksOnItOnOneUpdate(t *testing.T) {
	var pet *tickTestPet
	timingTestConstruct = func(a *timingTestAgent) {
		pet = &tickTestPet{Pet: NewPet("Pet", &a.Character, stats.Stats{}, func(stats.Stats) stats.Stats { return stats.Stats{} }, true, false)}
		pet.EnableAutoAttacks(pet, AutoAttackOptions{MainHand: testWeapon(2), AutoSwingMelee: true})
		a.AddPet(pet)
	}
	var hot *Spell
	var order []string
	sim, _ := newTickTestSim(t, 100, 10, func(a *timingTestAgent) {
		hot = a.RegisterSpell(SpellConfig{ActionID: ActionID{SpellID: testDotLowID}, Hot: DotConfig{
			Aura:          Aura{Label: "hot"},
			NumberOfTicks: 4,
			TickLength:    time.Second,
			TicksCanCrit:  false,
			OnTick:        func(sim *Simulation, _ *Unit, _ *Dot) { order = append(order, fmt.Sprintf("tick %v", sim.CurrentTime)) },
		}})
	})
	timingTestConstruct = nil
	pet.AutoAttacks.MHAuto().ApplyEffects = func(sim *Simulation, _ *Unit, _ *Spell) {
		order = append(order, fmt.Sprintf("swing %v", sim.CurrentTime))
	}
	sim.PrePull()
	// the pet swings on the first map update of the fight and every 2 s from there
	first := sim.NextServerTick(0)
	// counts from the map update before, so its second tick shares the pet's second swing's update
	at(sim, first+ms(50), func(sim *Simulation) { hot.Hot(&pet.Unit).Apply(sim) })
	runUntil(sim, first+ms(2050))

	shared := first + 2*time.Second
	want := []string{
		fmt.Sprintf("swing %v", first),
		fmt.Sprintf("tick %v", first+time.Second),
		fmt.Sprintf("tick %v", shared),
		fmt.Sprintf("swing %v", shared),
	}
	if !slices.Equal(order, want) {
		t.Errorf("order %v, want %v", order, want)
	}
}

// Map::Update runs every player's update, auras first and melee last, ahead of the creatures': a swing on
// the update a debuff on the target runs out on still finds it, and a buff of the player's is gone.
func TestPlayerSwingComesAheadOfTheCreaturesAurasOnOneUpdate(t *testing.T) {
	var buff, debuff *Aura
	var order []string
	sim, a := newTickTestSim(t, 100, 10, func(a *timingTestAgent) {
		a.EnableAutoAttacks(a, AutoAttackOptions{MainHand: testWeapon(2), AutoSwingMelee: true})
		note := func(label string) OnExpire {
			return func(_ *Aura, _ *Simulation) { order = append(order, label) }
		}
		buff = a.RegisterAura(Aura{Label: "buff", Duration: time.Second, OnExpire: note("buff ends")})
		debuff = a.Env.Encounter.TargetUnits[0].RegisterAura(Aura{Label: "debuff", Duration: time.Second, OnExpire: note("debuff ends")})
	})
	shared := sim.NextServerTick(2 * time.Second)
	a.AutoAttacks.mh.setTimer(sim, shared)
	a.AutoAttacks.MHAuto().ApplyEffects = func(sim *Simulation, _ *Unit, _ *Spell) {
		if sim.CurrentTime == shared {
			order = append(order, fmt.Sprintf("swing, buff %v, debuff %v", buff.IsActive(), debuff.IsActive()))
		}
	}
	sim.PrePull()
	// the target's counts from the map update before, the player's from the world tick before
	at(sim, shared-ms(950), func(sim *Simulation) { debuff.Activate(sim) })
	at(sim, shared-ms(995), func(sim *Simulation) { buff.Activate(sim) })
	runUntil(sim, shared+ms(50))

	if want := []string{"buff ends", "swing, buff false, debuff true", "debuff ends"}; !slices.Equal(order, want) {
		t.Errorf("order %v, want %v", order, want)
	}
}

// Unit::Update runs a creature's events ahead of its auras too: its cast landing on the update a buff on
// it runs out on still finds it.
func TestCreatureCastLandingOnItsAurasLastUpdateFindsIt(t *testing.T) {
	var pet *tickTestPet
	timingTestConstruct = func(a *timingTestAgent) {
		pet = &tickTestPet{Pet: NewPet("Pet", &a.Character, stats.Stats{}, func(stats.Stats) stats.Stats { return stats.Stats{} }, true, false)}
		a.AddPet(pet)
	}
	var spell *Spell
	var buff *Aura
	var order []string
	sim, a := newTickTestSim(t, 100, 10, func(a *timingTestAgent) {
		spell = pet.RegisterSpell(castConfig(testSpellSlam, ms(1500)))
		buff = pet.RegisterAura(Aura{
			Label:    "buff",
			Duration: time.Second,
			OnExpire: func(_ *Aura, _ *Simulation) { order = append(order, "expires") },
		})
	})
	timingTestConstruct = nil
	spell.ApplyEffects = func(sim *Simulation, _ *Unit, _ *Spell) {
		order = append(order, fmt.Sprintf("lands, buff up %v", buff.IsActive()))
	}
	sim.PrePull()
	start := sim.NextServerTick(ms(2500))
	var landsAt time.Duration
	at(sim, start, func(sim *Simulation) {
		// its idle rotation waits on a GCD of its own
		pet.GCD.Set(sim.CurrentTime)
		if !spell.Cast(sim, a.CurrentTarget) {
			t.Fatal("the pet's cast failed")
		}
		landsAt = pet.Hardcast.Expires
	})
	// put on by the player, so it counts from the map update before
	at(sim, start+ms(550), func(sim *Simulation) { buff.Activate(sim) })
	runUntil(sim, start+3*time.Second)

	if buff.ExpiresAt() != 0 || landsAt != start+ms(1500) {
		t.Fatalf("the buff expires at %v, the cast lands at %v", buff.ExpiresAt(), landsAt)
	}
	if want := []string{"lands, buff up true", "expires"}; !slices.Equal(order, want) {
		t.Errorf("order %v, want %v", order, want)
	}
}

// The players' attacks stay in front of the creatures' through adds and removals.
func TestWeaponAttacksKeepThePlayersInFront(t *testing.T) {
	var pet *tickTestPet
	timingTestConstruct = func(a *timingTestAgent) {
		pet = &tickTestPet{Pet: NewPet("Pet", &a.Character, stats.Stats{}, func(stats.Stats) stats.Stats { return stats.Stats{} }, true, false)}
		pet.EnableAutoAttacks(pet, AutoAttackOptions{MainHand: testWeapon(2), OffHand: testWeapon(2), AutoSwingMelee: true})
		a.AddPet(pet)
	}
	sim, a := newTickTestSim(t, 100, 10, func(a *timingTestAgent) {
		a.EnableAutoAttacks(a, AutoAttackOptions{MainHand: testWeapon(2), OffHand: testWeapon(2), Ranged: testWeapon(2), AutoSwingMelee: true, AutoSwingRanged: true})
	})
	timingTestConstruct = nil
	players, pets := &a.AutoAttacks, &pet.AutoAttacks
	check := func(step string, want ...*WeaponAttack) {
		t.Helper()
		if !slices.Equal(sim.weaponAttacks, want) {
			var got []string
			for _, wa := range sim.weaponAttacks {
				got = append(got, fmt.Sprintf("%s %s", wa.unit.Label, wa.config.ActionID))
			}
			t.Errorf("%s: the attacks are out of order: %v", step, got)
		}
		n := 0
		for _, wa := range want {
			if wa.unit.Type == PlayerUnit {
				n++
			}
		}
		if sim.playerAttacks != n {
			t.Errorf("%s: %d player attacks counted, want %d", step, sim.playerAttacks, n)
		}
	}

	// the pet is out from the start, so its attacks are on already
	sim.weaponAttacks, sim.playerAttacks = sim.weaponAttacks[:0], 0
	sim.addWeaponAttack(&pets.mh)
	sim.addWeaponAttack(&players.mh)
	sim.addWeaponAttack(&pets.oh)
	sim.addWeaponAttack(&players.oh)
	sim.addWeaponAttack(&players.ranged)
	check("added", &players.mh, &players.oh, &players.ranged, &pets.mh, &pets.oh)

	sim.removeWeaponAttack(&players.mh)
	check("a player's off", &players.ranged, &players.oh, &pets.oh, &pets.mh)
	sim.removeWeaponAttack(&pets.oh)
	check("a pet's off", &players.ranged, &players.oh, &pets.mh)
	sim.removeWeaponAttack(&players.oh)
	sim.removeWeaponAttack(&players.ranged)
	check("no player left", &pets.mh)
	sim.addWeaponAttack(&players.mh)
	check("a player's back", &players.mh, &pets.mh)
}

// The server sets a unit's under 35% and under 20% states in its own update, so those phases start on the
// target's first map update from the damage that crosses them. 25% starts with its damage: only Drain
// Soul reads it, off the health itself.
func TestDamageBasedExecutePhaseStartsOnTheTargetsUpdate(t *testing.T) {
	for _, gridMs := range []int32{100, 0} {
		var spell *Spell
		sim, a := newTickTestSim(t, gridMs, gridMs/10, func(a *timingTestAgent) {
			spell = a.RegisterSpell(SpellConfig{ActionID: ActionID{SpellID: testSpellNoServerData}})
		})
		var got []string
		sim.RegisterExecutePhaseCallback(func(sim *Simulation, phase int32) {
			got = append(got, fmt.Sprintf("%d at %v", phase, sim.CurrentTime))
		})
		sim.StartTargetAtHealth(1000, 400)
		sim.PrePull()
		target := a.CurrentTarget
		hit := func(doAt time.Duration, damage float64) {
			at(sim, doAt, func(sim *Simulation) {
				result := spell.NewResult(target)
				result.Damage = damage
				spell.DealDamage(sim, result)
			})
		}
		first := sim.NextServerTick(time.Second)
		hit(first+ms(30), 60)   // 34% left
		hit(first+ms(250), 100) // 24%
		hit(first+ms(420), 60)  // 18%
		runUntil(sim, first+time.Second)

		want := []string{
			fmt.Sprintf("35 at %v", sim.NextServerTick(first+ms(30))),
			fmt.Sprintf("25 at %v", first+ms(250)),
			fmt.Sprintf("20 at %v", sim.NextServerTick(first+ms(420))),
		}
		if !slices.Equal(got, want) {
			t.Errorf("%d ms: phases %v, want %v", gridMs, got, want)
		}
	}
}

// One hit through two thresholds starts both phases on that same update.
func TestOneHitThroughTwoExecuteThresholdsStartsBothOnOneUpdate(t *testing.T) {
	var spell *Spell
	sim, a := newTickTestSim(t, 100, 10, func(a *timingTestAgent) {
		spell = a.RegisterSpell(SpellConfig{ActionID: ActionID{SpellID: testSpellNoServerData}})
	})
	var got []string
	sim.RegisterExecutePhaseCallback(func(sim *Simulation, phase int32) {
		got = append(got, fmt.Sprintf("%d at %v", phase, sim.CurrentTime))
	})
	sim.StartTargetAtHealth(1000, 400)
	sim.PrePull()
	hitAt := sim.NextServerTick(time.Second) + ms(30)
	at(sim, hitAt, func(sim *Simulation) {
		result := spell.NewResult(a.CurrentTarget)
		result.Damage = 250
		spell.DealDamage(sim, result)
	})
	runUntil(sim, hitAt+time.Second)

	update := sim.NextServerTick(hitAt)
	want := []string{fmt.Sprintf("35 at %v", update), fmt.Sprintf("25 at %v", update), fmt.Sprintf("20 at %v", update)}
	if !slices.Equal(got, want) {
		t.Errorf("phases %v, want %v", got, want)
	}
}

// A script adds to what the duration has left (SetDuration(GetDuration() + extra)), so the aura ends on
// the first update that far on, not that far from the update it would have ended on.
func TestTimeAddedToAnAuraGoesOnItsDurationNotItsLastUpdate(t *testing.T) {
	var aura *Aura
	sim, _ := newLoopTestSim(t, 100, 10, 10.3, func(a *timingTestAgent) {
		aura = a.Env.Encounter.TargetUnits[0].RegisterAura(Aura{Label: "debuff", Duration: ms(1080)})
	})
	sim.PrePull()
	update := sim.NextServerTick(ms(400))
	at(sim, update+ms(88), func(sim *Simulation) { aura.Activate(sim) })
	at(sim, update+ms(388), func(sim *Simulation) { aura.AddDuration(sim, 2*time.Second) })
	runUntil(sim, update+ms(500))

	// it counts from the update before it went on: 3.08 s on is 30 updates of 103 ms. From the 11th, where
	// the 1.08 s would have ended, 2 s more would be the 31st.
	if got, want := aura.ExpiresAt()-update, ms(3090); got != want {
		t.Errorf("the aura ends %v after the update before it went on, want %v", got, want)
	}
}
