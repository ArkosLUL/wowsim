package core

import (
	"math"
	"slices"
	"testing"
	"time"

	"github.com/wowsims/wotlk/sim/core/proto"
	"github.com/wowsims/wotlk/sim/core/stats"
)

// resettingTestPet is a guardian whose Reset dismisses it, as the Spirit Wolves' and the Shadowfiend's do.
type resettingTestPet struct{ timingTestPet }

func (p *resettingTestPet) Reset(sim *Simulation) { p.Disable(sim) }

// A guardian still out as a fight ends starts the next one clean: what its owner handed it went with the
// reset, so the Reset that dismisses it has nothing left to take off.
func TestPetResetForgetsASummonTheFightEndedOn(t *testing.T) {
	var pet *resettingTestPet
	timingTestConstruct = func(a *timingTestAgent) {
		pet = &resettingTestPet{timingTestPet{Pet: NewPet("Guardian", &a.Character, stats.Stats{}, func(stats.Stats) stats.Stats {
			return stats.Stats{stats.AttackPower: 500}
		}, false, true)}}
		pet.HasteCarrier = true
		pet.OwnerHasteSource = func() float64 { return 1.2 }
		a.AddPet(pet)
	}
	sim, _ := newTickTestSim(t, 100, 10, nil)
	timingTestConstruct = nil

	for fight := 1; fight <= 3; fight++ {
		runUntil(sim, ms(1000))
		if ap := pet.GetStat(stats.AttackPower); ap != 0 {
			t.Fatalf("fight %d: %v attack power before the summon, want 0", fight, ap)
		}
		pet.EnableWithTimeout(sim, pet, time.Minute)
		if ap, speed := pet.GetStat(stats.AttackPower), pet.PseudoStats.MeleeSpeedMultiplier; ap != 500 || math.Abs(speed-1.2) > 1e-9 {
			t.Fatalf("fight %d: summoned with %v attack power at swing speed %v, want 500 at 1.2", fight, ap, speed)
		}
		runUntil(sim, ms(5000))
		sim.Cleanup()
		sim.reset()
	}
}

// MP5 comes in the creature's own tick: two fifths of the whole MP5, whatever the five second rule says.
func TestCreatureManaRegenTakesTheMP5(t *testing.T) {
	pet := newTestPet(newTestOwner(proto.Class_ClassWarlock, ManaBar, stats.Stats{}))
	pet.stats[stats.Spirit] = 367.6
	// 92 * 2 / 5 is 36.8
	pet.stats[stats.MP5] = 92.9
	if got := pet.CreatureManaRegen(false); got != 90+36 {
		t.Errorf("out of the five second rule: %v a tick, want %v", got, 90+36)
	}
	if got := pet.CreatureManaRegen(true); got != 36 {
		t.Errorf("under the five second rule: %v a tick, want 36", got)
	}
}

// A creature's mana only moves on its own regen tick: core's tick for the players leaves it alone.
func TestCreatureManaOnlyMovesOnItsOwnTick(t *testing.T) {
	var pet *timingTestPet
	timingTestConstruct = func(a *timingTestAgent) {
		a.EnableManaBar()
		pet = &timingTestPet{Pet: NewPet("Pet", &a.Character, stats.Stats{stats.Spirit: 100, stats.MP5: 100}, func(stats.Stats) stats.Stats {
			return stats.Stats{}
		}, true, false)}
		pet.EnableCreatureManaBar(5000, 0, 15, false)
		a.AddPet(pet)
	}
	sim, _ := newTickTestSim(t, 100, 10, nil)
	timingTestConstruct = nil
	pet.currentMana = 0

	first := pet.creatureManaRefresh.NextActionAt
	var gains []float64
	for _, until := range []time.Duration{first, first + 1, first + 2*time.Second, first + 2*time.Second + 1} {
		runUntil(sim, until)
		gains = append(gains, pet.CurrentMana())
	}
	// spirit / 5 + 17 and two fifths of the MP5, every 2 s
	if want := []float64{0, 77, 77, 154}; !slices.Equal(gains, want) {
		t.Errorf("mana around the first two regen ticks %v, want %v", gains, want)
	}
}

// A pet out since before the pull has ticked all through the prepull, so it has what its owner gained
// there by the pull if a tick came since.
func TestPetTimersTickThroughThePrepull(t *testing.T) {
	var pet *timingTestPet
	var agent *timingTestAgent
	timingTestConstruct = func(a *timingTestAgent) {
		agent = a
		pet = &timingTestPet{Pet: NewPet("Pet", &a.Character, stats.Stats{}, func(owner stats.Stats) stats.Stats {
			return stats.Stats{stats.AttackPower: owner[stats.AttackPower]}
		}, true, false)}
		pet.SummonedAsPet = true
		pet.ScalingPeriod = 2 * time.Second
		a.AddPet(pet)
	}
	sim, _ := newTickTestSim(t, 100, 10, func(a *timingTestAgent) {
		a.RegisterPrepullAction(-5*time.Second, func(sim *Simulation) {
			a.AddStatsDynamic(sim, stats.Stats{stats.AttackPower: 100})
		})
	})
	timingTestConstruct = nil

	before := pet.GetStat(stats.AttackPower)
	tick := pet.scalingRefresh.NextActionAt
	if tick <= -5*time.Second || tick > -3*time.Second {
		t.Fatalf("first scaling tick at %v, want within 2 s of the prepull's start at -5 s", tick)
	}
	sim.PrePull()
	runUntil(sim, 0)
	if got, want := pet.GetStat(stats.AttackPower), before+100; got != want {
		t.Errorf("attack power %v at the pull, want %v with the owner's %v", got, want, agent.GetStat(stats.AttackPower))
	}
}

// A hunter pet gets 24 focus every 4 s and a creature on energy 20 every 2 s, each on its own update. The
// focus comes ahead of the pet's AI, the energy after it.
func TestPetFocusAndEnergyRegenOnItsOwnUpdates(t *testing.T) {
	for _, tc := range []struct {
		name     string
		enable   func(*Unit)
		perTick  float64
		interval time.Duration
		priority ActionPriority
	}{
		{"focus", func(u *Unit) { u.EnableFocusBar(1, nil) }, 24, 4 * time.Second, ActionPriorityRegen},
		{"focus with Bestial Discipline", func(u *Unit) { u.EnableFocusBar(1.5, nil) }, 36, 4 * time.Second, ActionPriorityRegen},
		{"energy", func(u *Unit) { u.EnableCreatureEnergyBar(nil) }, 20, 2 * time.Second, petRegenPriority},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var pet *timingTestPet
			timingTestConstruct = func(a *timingTestAgent) {
				pet = &timingTestPet{Pet: NewPet("Pet", &a.Character, stats.Stats{}, func(stats.Stats) stats.Stats {
					return stats.Stats{}
				}, false, false)}
				tc.enable(&pet.Unit)
				a.AddPet(pet)
			}
			sim, _ := newTickTestSim(t, 100, 10, nil)
			timingTestConstruct = nil

			if amount, interval := pet.FocusRegenTick(); amount != tc.perTick || interval != tc.interval {
				t.Fatalf("regenerates %v every %v, want %v every %v", amount, interval, tc.perTick, tc.interval)
			}
			runUntil(sim, ms(1234))
			pet.Enable(sim, pet)
			pet.currentFocus = 0

			// counted from the map update before its first one
			first := sim.NextServerTick(sim.CurrentTime) - sim.serverTickInterval + tc.interval
			if tick := pet.focusRefresh; tick.NextActionAt != first || tick.Priority != tc.priority {
				t.Fatalf("first tick at %v with priority %d, want %v with %d", tick.NextActionAt, tick.Priority, first, tc.priority)
			}
			half := 50 * time.Millisecond
			for i, until := range []time.Duration{first - half, first + half, first + tc.interval - half, first + tc.interval + half} {
				runUntil(sim, until)
				if got, want := pet.CurrentFocus(), tc.perTick*float64((i+1)/2); got != want {
					t.Errorf("by %v: %v, want %v", until, got, want)
				}
			}

			// a full bar gets nothing, and a pet that left gets nothing either
			pet.currentFocus = MaxFocus
			runUntil(sim, first+2*tc.interval+half)
			if events := pet.focusBar.regenMetrics.Events; events != 2 {
				t.Errorf("%d regen ticks counted with the bar full on the third, want 2", events)
			}
			pet.Disable(sim)
			pet.currentFocus = 0
			runUntil(sim, first+4*tc.interval)
			if pet.CurrentFocus() != 0 || pet.focusRefresh != nil {
				t.Errorf("after the pet left: %v on the bar, a tick queued %v", pet.CurrentFocus(), pet.focusRefresh != nil)
			}
		})
	}
}

// Guardian::UpdateMaxHealth: the create health plus the pet's own health a point of stamina over the
// create value. The Imp's 3867 and 8.4 make 4421 at 184 stamina.
func TestPetCreateHealth(t *testing.T) {
	for _, tc := range []struct {
		name                                    string
		createHealth, createStamina, perStamina float64
		stamina, want                           float64
	}{
		{"Imp", 3867, 118, 8.4, 184, 4421.4},
		{"Water Elemental", 7561, 118, 7.5, 135, 7688.5},
		{"Felguard", 5428, 328, 11, 394, 6154},
		{"Greater Fire Elemental", 8000, 125, 10, 125, 8000},
	} {
		var pet *timingTestPet
		timingTestConstruct = func(a *timingTestAgent) {
			pet = &timingTestPet{Pet: NewPet(tc.name, &a.Character, stats.Stats{stats.Stamina: tc.stamina}, func(stats.Stats) stats.Stats {
				return stats.Stats{}
			}, true, false)}
			pet.SetCreateHealth(tc.createHealth, tc.createStamina, tc.perStamina)
			a.AddPet(pet)
		}
		newTickTestSim(t, 100, 10, nil)
		timingTestConstruct = nil
		if got := pet.MaxHealth(); math.Abs(got-tc.want) > 1e-6 {
			t.Errorf("%s at %v stamina: %v health, want %v", tc.name, tc.stamina, got, tc.want)
		}
	}
}

// A scripted AI's next cast breaks the one it's in the middle of: nothing lands and nothing is paid.
func TestPetInterruptCast(t *testing.T) {
	var pet *timingTestPet
	var slow *Spell
	landed := 0
	timingTestConstruct = func(a *timingTestAgent) {
		pet = &timingTestPet{Pet: NewPet("Pet", &a.Character, stats.Stats{}, func(stats.Stats) stats.Stats {
			return stats.Stats{}
		}, true, true)}
		pet.EnableCreatureManaBar(1000, 0, 15, false)
		a.AddPet(pet)
		slow = pet.RegisterSpell(SpellConfig{
			ActionID: ActionID{SpellID: 1},
			Flags:    SpellFlagNoServerData,
			ManaCost: ManaCostOptions{FlatCost: 30},
			Cast:     CastConfig{DefaultCast: Cast{CastTime: 2 * time.Second}},
			ApplyEffects: func(*Simulation, *Unit, *Spell) {
				landed++
			},
		})
	}
	sim, _ := newTickTestSim(t, 100, 10, nil)
	timingTestConstruct = nil
	sim.PrePull()
	runUntil(sim, ms(1000))

	if !slow.Cast(sim, pet.CurrentTarget) || pet.Hardcast.Expires <= sim.CurrentTime {
		t.Fatal("the cast didn't start")
	}
	runUntil(sim, ms(2000))
	if !pet.InterruptCast(sim, slow) {
		t.Fatal("nothing to interrupt a second into the cast")
	}
	if pet.InterruptCast(sim, slow) {
		t.Error("interrupted the same cast twice")
	}
	runUntil(sim, ms(5000))
	if landed != 0 || pet.CurrentMana() != pet.MaxMana() {
		t.Errorf("the broken cast landed %d times and left %v of %v mana", landed, pet.CurrentMana(), pet.MaxMana())
	}
	if got := slow.SpellMetrics[pet.CurrentTarget.UnitIndex].TotalCastTime; got != time.Second {
		t.Errorf("%v of cast time booked, want the second it ran", got)
	}

	// the next one goes through
	if !slow.Cast(sim, pet.CurrentTarget) {
		t.Fatal("the cast after the broken one didn't start")
	}
	runUntil(sim, ms(8000))
	if landed != 1 || pet.CurrentMana() != pet.MaxMana()-30 {
		t.Errorf("the next cast landed %d times with %v of %v mana left", landed, pet.CurrentMana(), pet.MaxMana())
	}
}
