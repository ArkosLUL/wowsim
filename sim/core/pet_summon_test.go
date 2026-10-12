package core

import (
	"testing"
	"time"

	"github.com/wowsims/wotlk/sim/core/proto"
	"github.com/wowsims/wotlk/sim/core/stats"
)

// Wrath of Air is the totem's raid area aura, which takes a pet or a guardian like anyone in the raid
// (AnyGroupedUnitInObjectRangeCheck). Only a haste carrier goes without: its script makes it immune to
// positive MOD_CASTING_SPEED_NOT_STACK.
func TestWrathOfAirReachesEveryPetButAHasteCarrier(t *testing.T) {
	for _, tc := range []struct {
		name                       string
		guardian, uncontrollable   bool
		hasteCarrier, wantWrathAir bool
	}{
		{name: "pet", wantWrathAir: true},
		{name: "guardian", guardian: true, wantWrathAir: true},
		{name: "guardian without a pet bar", guardian: true, uncontrollable: true, wantWrathAir: true},
		{name: "haste carrier", hasteCarrier: true},
		{name: "guardian haste carrier", guardian: true, uncontrollable: true, hasteCarrier: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var pet *timingTestPet
			timingTestConstruct = func(a *timingTestAgent) {
				pet = &timingTestPet{Pet: NewPet("Pet", &a.Character, stats.Stats{}, func(stats.Stats) stats.Stats { return stats.Stats{} }, !tc.guardian, tc.guardian)}
				pet.Uncontrollable = tc.uncontrollable
				pet.HasteCarrier = tc.hasteCarrier
				a.AddPet(pet)
			}
			defer func() { timingTestConstruct = nil }()
			sim := NewSim(&proto.RaidSimRequest{
				SimOptions: &proto.SimOptions{RandomSeed: 101, IsTest: true},
				Raid: &proto.Raid{
					Parties: []*proto.Party{{
						Players: []*proto.Player{{
							Name:      "Timing",
							Class:     proto.Class_ClassShaman,
							Consumes:  &proto.Consumes{},
							Buffs:     &proto.IndividualBuffs{},
							Spec:      &proto.Player_EnhancementShaman{},
							Equipment: &proto.EquipmentSpec{},
						}},
						Buffs: &proto.PartyBuffs{},
					}},
					Buffs: &proto.RaidBuffs{WrathOfAirTotem: true},
				},
				Encounter: &proto.Encounter{
					Targets:  []*proto.Target{{Name: "target", Level: 83, MobType: proto.MobType_MobTypeDemon}},
					Duration: 60,
				},
			})
			sim.reset()
			want := 1.0
			if tc.wantWrathAir {
				want = 1.05
			}
			if got := pet.PseudoStats.CastSpeedMultiplier; got != want {
				t.Errorf("cast speed multiplier %v under Wrath of Air, want %v", got, want)
			}
		})
	}
}

// newSummonTestSim has a guardian that isn't out yet, on the live 102.7 ms map update.
func newSummonTestSim(t *testing.T) (*Simulation, *timingTestPet) {
	t.Helper()
	return newSummonTestSimOn(t, 100)
}

// newSummonTestSimOn is newSummonTestSim with MapUpdateInterval as given, 0 for exact timing.
func newSummonTestSimOn(t *testing.T, mapUpdateMs int32) (*Simulation, *timingTestPet) {
	t.Helper()
	var pet *timingTestPet
	timingTestConstruct = func(a *timingTestAgent) {
		pet = &timingTestPet{Pet: NewPet("Guardian", &a.Character, stats.Stats{}, func(stats.Stats) stats.Stats { return stats.Stats{} }, false, true)}
		a.AddPet(pet)
	}
	sim, _ := newLoopTestSim(t, mapUpdateMs, 10, 10.27, nil)
	timingTestConstruct = nil
	return sim, pet
}

// TempSummon::Update counts the summon's timer down on the guardian's own updates and unsummons it once
// Creature::Update is through, so the guardian still acts on the update its time runs out on: the first
// one a whole duration past the update before its first.
func TestTimedSummonActsOnItsLastUpdate(t *testing.T) {
	const life = 12 * time.Second
	for _, summonAt := range []time.Duration{ms(1000), ms(1037), ms(1099), ms(2511)} {
		sim, pet := newSummonTestSim(t)
		runUntil(sim, summonAt)
		start := pet.AIClockStart(sim)
		last := sim.NextServerTick(start + life)
		if updates := (last - start) / sim.serverTickInterval; updates != 117 {
			t.Fatalf("summoned at %v: %d updates of %v in its %v, want 117", summonAt, updates, sim.serverTickInterval, life)
		}
		pet.EnableWithTimeout(sim, pet, life)

		var actedOnLast, outAfterLast bool
		sim.AddPendingAction(&PendingAction{NextActionAt: last, Priority: ActionPriorityLow, OnAction: func(*Simulation) {
			actedOnLast = pet.IsEnabled()
		}})
		sim.AddPendingAction(&PendingAction{NextActionAt: last, Priority: petUnsummonPriority - 1, OnAction: func(*Simulation) {
			outAfterLast = pet.IsEnabled()
		}})
		runUntil(sim, last-time.Nanosecond)
		if !pet.IsEnabled() {
			t.Fatalf("summoned at %v: gone before its last update at %v", summonAt, last)
		}
		runUntil(sim, last+ms(1))
		if !actedOnLast || outAfterLast {
			t.Errorf("summoned at %v: out for its AI on the last update = %v, still out after it = %v; want true, false",
				summonAt, actedOnLast, outAfterLast)
		}
	}
}

// With MapUpdateInterval 0 there's no last update to act on: the summon's time is just up, ahead of a cast
// the pet has due at that very moment.
func TestTimedSummonOnExactTimingEndsAheadOfACastDueThen(t *testing.T) {
	sim, pet := newSummonTestSimOn(t, 0)
	runUntil(sim, ms(1037))
	pet.EnableWithTimeout(sim, pet, 10*time.Second)
	ends := ms(11037)
	if got := pet.UnsummonAt(); got != ends {
		t.Fatalf("unsummoned at %v, want %v: the test wants exact timing", got, ends)
	}

	outForItsCast := false
	sim.AddPendingAction(&PendingAction{NextActionAt: ends, Priority: ActionPriorityGCD, OnAction: func(*Simulation) {
		outForItsCast = pet.IsEnabled()
	}})
	runUntil(sim, ends-time.Nanosecond)
	if !pet.IsEnabled() {
		t.Fatalf("gone before its time is up at %v", ends)
	}
	runUntil(sim, ends+ms(1))
	if outForItsCast || pet.IsEnabled() {
		t.Errorf("out for a cast due as its time is up = %v, still out after = %v; want false, false", outForItsCast, pet.IsEnabled())
	}
}

// A summon that comes again while the last one is still out gets its own time: the old timer doesn't cut
// it short.
func TestSummonedAgainWhileStillOutLastsItsNewTime(t *testing.T) {
	sim, pet := newSummonTestSim(t)
	runUntil(sim, ms(1000))
	pet.EnableWithTimeout(sim, pet, 10*time.Second)
	first := pet.UnsummonAt()

	runUntil(sim, first-ms(10))
	pet.EnableWithTimeout(sim, pet, 10*time.Second)
	second := pet.UnsummonAt()
	if second < first+9*time.Second {
		t.Fatalf("the second summon goes at %v, the first at %v: want it most of 10 s later", second, first)
	}
	runUntil(sim, first+ms(500))
	if !pet.IsEnabled() {
		t.Fatal("the first summon's timer took the second one out")
	}
	runUntil(sim, second+ms(1))
	if pet.IsEnabled() {
		t.Error("still out past the second summon's time")
	}
}

// Nothing can be cast on a creature that isn't in the world, so a summon is a new creature without what
// was cast on the pet while it was away. What stands for a buff that's always up (a permanent aura) is on.
func TestSummonStartsWithoutWhatWasCastOnItAway(t *testing.T) {
	sim, pet := newSummonTestSim(t)
	cast := pet.RegisterAura(Aura{Label: "Cast While Away", Duration: 15 * time.Second})
	always := MakePermanent(pet.RegisterAura(Aura{Label: "Always Up"}))
	sim.Cleanup()
	sim.reset()

	runUntil(sim, ms(1000))
	cast.Activate(sim)
	if cast.IsActive() {
		t.Error("an aura went onto a pet that isn't in the world")
	}
	runUntil(sim, ms(2000))
	pet.EnableWithTimeout(sim, pet, 10*time.Second)
	if cast.IsActive() {
		t.Error("the summon carries an aura cast on it while it was away")
	}
	if !always.IsActive() {
		t.Error("the summon lost its permanent aura")
	}

	// one it gets while it's out lasts its time
	cast.Activate(sim)
	runUntil(sim, ms(5000))
	if !cast.IsActive() {
		t.Error("an aura cast on the summon while it's out is gone early")
	}
}

// Unsummoning takes the creature out of the world and kills its events, a spell in flight among them.
func TestMissileInTheAirAtTheUnsummonIsLost(t *testing.T) {
	sim, pet := newSummonTestSim(t)
	// 20 yd at 20 yd a second
	pet.DistanceFromTarget = 20
	bolt := &Spell{Unit: &pet.Unit, MissileSpeed: 20}

	runUntil(sim, ms(1000))
	if pet.MissileLost(sim, bolt) {
		t.Fatal("a pet that isn't on a timer loses its missile")
	}
	pet.EnableWithTimeout(sim, pet, 10*time.Second)
	last := sim.NextServerTick(pet.AIClockStart(sim) + 10*time.Second)
	for _, tc := range []struct {
		launch time.Duration
		lost   bool
	}{
		{last - ms(1500), false},
		// it lands on the update the unsummon comes at the end of
		{last - ms(1000), false},
		{last - ms(999), true},
	} {
		runUntil(sim, tc.launch)
		if got := pet.MissileLost(sim, bolt); got != tc.lost {
			t.Errorf("launched %v before the unsummon: lost = %v, want %v", last-tc.launch, got, tc.lost)
		}
	}
}

// A pet's clocks step by the map updates there are, not by the shortest one: on the live settings the
// updates differ in length, so a fixed step lands between two of them.
func TestPetClocksFollowUpdatesOfVaryingLength(t *testing.T) {
	var pet *timingTestPet
	timingTestConstruct = func(a *timingTestAgent) {
		pet = &timingTestPet{Pet: NewPet("Guardian", &a.Character, stats.Stats{}, func(stats.Stats) stats.Stats { return stats.Stats{} }, false, true)}
		a.AddPet(pet)
	}
	sim, _ := newSettingsTestSim(t, nil, nil)
	timingTestConstruct = nil
	if sim.updates == nil {
		t.Fatal("the live settings give every map update the same length")
	}

	const life = 12 * time.Second
	varied := false
	for now := ms(1000); now < ms(1600); now += ms(7) {
		runUntil(sim, now)
		this := sim.NextServerTick(now)

		// the update before the one this summon's first is on
		start := pet.AIClockStart(sim)
		if sim.NextServerTick(start) != start || sim.NextServerTick(start+1) != this || start >= this {
			t.Fatalf("at %v the AI clock starts at %v, want the update before %v", now, start, this)
		}
		if got := pet.updateTimersStart(sim, now, false); got != start {
			t.Errorf("at %v the timers start at %v, the AI clock at %v", now, got, start)
		}

		after := pet.UpdateAfter(sim, now)
		if after <= this || sim.NextServerTick(this+1) != after {
			t.Fatalf("at %v the update after %v is %v", now, this, after)
		}
		varied = varied || this-start != sim.serverTickInterval || after-this != sim.serverTickInterval

		// a timed summon goes on the first update a duration past the one before its first
		if last := sim.NextServerTick(start + life); last < start+life || last-start >= life+ms(200) || sim.NextServerTick(last) != last {
			t.Fatalf("at %v the 12 s summon's last update is %v after %v", now, last, start)
		}
	}
	if !varied {
		t.Error("every update the pet's clocks met had the shortest length: the test never left the regular grid")
	}
}
