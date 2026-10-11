package core

import (
	"slices"
	"testing"
	"time"

	"github.com/wowsims/wotlk/sim/core/proto"
	"github.com/wowsims/wotlk/sim/core/stats"
)

// newReplenishmentTestSim is one mana user that procs its own Replenishment, on exact timing. Its mana
// starts well under a max of maxMana, so no tick is cut short.
func newReplenishmentTestSim(t *testing.T, maxMana float64, raidBuff bool) (*Simulation, *timingTestAgent, ReplenishmentSource) {
	t.Helper()
	return newReplenishmentTickTestSim(t, 0, 0, maxMana, raidBuff, nil)
}

// newReplenishmentTickTestSim is newReplenishmentTestSim on the given map update and world tick, with whatever
// construct adds to the player.
func newReplenishmentTickTestSim(t *testing.T, mapUpdateMs, worldTickMs int32, maxMana float64, raidBuff bool,
	construct func(*timingTestAgent)) (*Simulation, *timingTestAgent, ReplenishmentSource) {
	t.Helper()
	timingTestConstruct = func(a *timingTestAgent) {
		a.EnableManaBar()
		if construct != nil {
			construct(a)
		}
	}
	defer func() { timingTestConstruct = nil }()
	var src ReplenishmentSource
	sim, a := newTickTestSim(t, mapUpdateMs, worldTickMs, func(a *timingTestAgent) {
		src = a.Env.Raid.NewReplenishmentSource(&a.Unit, ActionID{SpellID: 48160})
		if raidBuff {
			MakePermanent(replenishmentAura(&a.Unit))
		}
	})
	a.stats[stats.Mana] = maxMana
	a.currentMana = maxMana / 2
	return sim, a, src
}

// The checks below read the state between ticks: runUntil stops ahead of anything queued for its end later
// than the call.
func replenishmentProc(sim *Simulation, src ReplenishmentSource, when ...time.Duration) {
	for _, w := range when {
		at(sim, w, func(sim *Simulation) { sim.Raid.ProcReplenishment(sim, src) })
	}
}

func TestReplenishmentTicks(t *testing.T) {
	// 25437 * 0.002 is 50.87, which the server cuts to 50
	sim, a, src := newReplenishmentTestSim(t, 25437.9, false)
	r := a.replenishment
	replenishmentProc(sim, src, 0)

	for _, c := range []struct {
		until time.Duration
		ticks int32
		up    bool
	}{
		{ms(900), 0, true},
		{ms(1100), 1, true},
		{ms(14900), 14, true},
		// the last tick comes with the expiry
		{ms(15100), 15, false},
		{ms(20000), 15, false},
	} {
		runUntil(sim, c.until)
		if r.metrics.Events != c.ticks || r.metrics.Gain != float64(c.ticks)*50 {
			t.Errorf("by %v: %d ticks giving %v, want %d of 50", c.until, r.metrics.Events, r.metrics.Gain, c.ticks)
		}
		if a.ReplenishmentAura.IsActive() != c.up {
			t.Errorf("at %v: aura up %v, want %v", c.until, a.ReplenishmentAura.IsActive(), c.up)
		}
	}
}

func TestReplenishmentRefreshRestartsTheTimer(t *testing.T) {
	sim, a, src := newReplenishmentTestSim(t, 20000, false)
	r := a.replenishment
	// the second lands between two ticks, the third right as one is due
	replenishmentProc(sim, src, 0, ms(2500), ms(5500))

	for _, c := range []struct {
		until time.Duration
		ticks int32
	}{
		{ms(2100), 2},
		// no tick at 3 s: the refresh started the second over
		{ms(3400), 2},
		{ms(3600), 3},
		{ms(5400), 4},
		// the tick due at 5.5 s goes before the refresh
		{ms(5600), 5},
		{ms(6400), 5},
		{ms(6600), 6},
		{ms(20400), 19},
		{ms(20600), 20},
		{ms(30000), 20},
	} {
		runUntil(sim, c.until)
		if r.metrics.Events != c.ticks {
			t.Errorf("by %v: %d ticks, want %d", c.until, r.metrics.Events, c.ticks)
		}
	}
}

func TestReplenishmentAmountIsSetAsItGoesOn(t *testing.T) {
	sim, a, src := newReplenishmentTestSim(t, 20000, false)
	r := a.replenishment
	replenishmentProc(sim, src, 0, ms(3500))
	at(sim, ms(1500), func(*Simulation) { a.stats[stats.Mana] = 30000 })

	// 40 a tick until the refresh reads the new max
	runUntil(sim, ms(3400))
	if r.metrics.Gain != 3*40 {
		t.Errorf("before the refresh: %v, want 3 ticks of 40", r.metrics.Gain)
	}
	runUntil(sim, ms(4600))
	if r.metrics.Gain != 3*40+60 {
		t.Errorf("after the refresh: %v, want 60 more", r.metrics.Gain)
	}
}

func TestReplenishmentThreat(t *testing.T) {
	sim, a, src := newReplenishmentTestSim(t, 20000, false)
	r := a.replenishment
	// the cast's bonus is modified, a tick's threat isn't
	a.PseudoStats.ThreatMultiplier = 2
	// off the mana regen's even seconds
	replenishmentProc(sim, src, ms(500))

	runUntil(sim, ms(1000))
	if !threatNear(r.metrics.threat, 2) {
		t.Errorf("the cast: threat %v, want its level 1 doubled", r.metrics.threat)
	}
	// half of what a tick really gives
	at(sim, ms(2200), func(*Simulation) { a.currentMana = a.MaxMana() - 10 })
	runUntil(sim, ms(3000))
	if want := 2 + 0.5*40 + 0.5*10; !threatNear(r.metrics.threat, want) {
		t.Errorf("after a full tick and a capped one: threat %v, want %v", r.metrics.threat, want)
	}
	at(sim, ms(3200), func(*Simulation) { a.currentMana = a.MaxMana() })
	runUntil(sim, ms(4000))
	if want := 2 + 0.5*40 + 0.5*10; !threatNear(r.metrics.threat, want) {
		t.Errorf("after a tick at full mana: threat %v, want %v still", r.metrics.threat, want)
	}

	a.doneGainThreat(sim)
	action := a.Metrics.actions[replenishmentActionID]
	if action == nil || !threatNear(action.Targets[sim.Encounter.TargetUnits[0].UnitIndex].Threat, 2+20+5) {
		t.Errorf("the threat isn't under Replenishment's action: %+v", action)
	}
}

func TestReplenishmentBeforeThePullMakesNoThreat(t *testing.T) {
	sim, a, src := newReplenishmentTestSim(t, 20000, false)
	sim.CurrentTime = ms(-3000)
	replenishmentProc(sim, src, ms(-3000))
	runUntil(sim, ms(-500))
	if r := a.replenishment; r.metrics.Events != 2 || r.metrics.threat != 0 {
		t.Errorf("%d ticks making %v threat, want 2 making none", r.metrics.Events, r.metrics.threat)
	}
}

func TestReplenishmentNeedsManaAsThePower(t *testing.T) {
	sim, a, src := newReplenishmentTestSim(t, 20000, false)
	r := a.replenishment
	replenishmentProc(sim, src, 0, ms(3500))

	// a cat isn't a target of the cast, which then has nobody to split its bonus over
	a.currentPowerBar = EnergyBar
	runUntil(sim, ms(3000))
	if a.ReplenishmentAura.IsActive() || r.metrics.Events != 0 || r.metrics.threat != 0 {
		t.Errorf("in cat form: aura up %v, %d ticks, threat %v", a.ReplenishmentAura.IsActive(), r.metrics.Events,
			r.metrics.threat)
	}

	// back in caster form the next cast takes it, and its ticks stop while it's a cat again
	a.currentPowerBar = ManaBar
	at(sim, ms(5600), func(*Simulation) { a.currentPowerBar = EnergyBar })
	at(sim, ms(8600), func(*Simulation) { a.currentPowerBar = ManaBar })
	runUntil(sim, ms(10000))
	// 4.5 and 5.5 s, then 9.5 s
	if !a.ReplenishmentAura.IsActive() || r.metrics.Events != 3 {
		t.Errorf("shifting under the aura: aura up %v, %d ticks, want 3", a.ReplenishmentAura.IsActive(), r.metrics.Events)
	}
}

func TestReplenishmentPutOnAtNoMana(t *testing.T) {
	sim, a, src := newReplenishmentTestSim(t, 20000, false)
	r := a.replenishment
	a.currentMana = 0.5
	replenishmentProc(sim, src, 0, ms(1500), ms(20000))

	// the aura's script never loaded, and a refresh doesn't load it
	runUntil(sim, ms(3000))
	if r.metrics.Events != 2 || r.metrics.Gain != 2 {
		t.Errorf("%d ticks giving %v, want 2 giving 1 each", r.metrics.Events, r.metrics.Gain)
	}
	// a new aura does
	runUntil(sim, ms(21100))
	if got := r.metrics.Gain - float64(r.metrics.Events-1); got != 40 {
		t.Errorf("the next aura's first tick gave %v, want 40", got)
	}
}

func TestReplenishmentFromTheRaid(t *testing.T) {
	sim, a, _ := newReplenishmentTestSim(t, 20000, true)
	r := a.replenishment
	at(sim, ms(7500), func(*Simulation) { a.stats[stats.Mana] = 30000 })

	// a tick a second for good, each reading the max then
	runUntil(sim, ms(7400))
	if r.metrics.Events != 7 || r.metrics.Gain != 7*40 {
		t.Errorf("by 7.4 s: %d ticks giving %v, want 7 of 40", r.metrics.Events, r.metrics.Gain)
	}
	runUntil(sim, ms(40100))
	if r.metrics.Events != 40 || r.metrics.Gain != 7*40+33*60 {
		t.Errorf("by 40 s: %d ticks giving %v, want 33 more of 60", r.metrics.Events, r.metrics.Gain)
	}
	// nobody in the sim cast it
	if r.metrics.threat != 0 {
		t.Errorf("threat %v, want none", r.metrics.threat)
	}
}

func TestReplenishmentProcOverTheRaids(t *testing.T) {
	sim, a, src := newReplenishmentTestSim(t, 20000, true)
	r := a.replenishment
	replenishmentProc(sim, src, ms(4500))

	// the raid's until the proc
	runUntil(sim, ms(4400))
	if r.metrics.Events != 4 || r.metrics.threat != 0 {
		t.Errorf("before the proc: %d ticks, threat %v, want 4 and none", r.metrics.Events, r.metrics.threat)
	}
	// the proc's own 15 ticks, from 5.5 to 19.5 s
	runUntil(sim, ms(19600))
	if want := 1 + 15*0.5*40; r.metrics.Events != 19 || !threatNear(r.metrics.threat, want) {
		t.Errorf("after the proc's ticks: %d ticks, threat %v, want 19 and %v", r.metrics.Events, r.metrics.threat, want)
	}
	// then the raid's again, a second apart as before
	runUntil(sim, ms(30600))
	if want := 1 + 15*0.5*40; r.metrics.Events != 30 || !threatNear(r.metrics.threat, want) {
		t.Errorf("back on the raid's: %d ticks, threat %v, want 30 and %v", r.metrics.Events, r.metrics.threat, want)
	}
	if !a.ReplenishmentAura.IsActive() {
		t.Error("the raid's buff went off")
	}
}

func TestReplenishmentFromTheRaidSkipsACat(t *testing.T) {
	sim, a, _ := newReplenishmentTestSim(t, 20000, true)
	a.currentPowerBar = EnergyBar
	at(sim, ms(5500), func(*Simulation) { a.currentPowerBar = ManaBar })
	at(sim, ms(7500), func(*Simulation) { a.currentPowerBar = EnergyBar })

	// only the ticks at 6 and 7 s find it in caster form
	runUntil(sim, ms(20000))
	if r := a.replenishment; r.metrics.Events != 2 {
		t.Errorf("%d ticks, want 2", r.metrics.Events)
	}
}

// The player's update takes the world tick the aura went on in off the tick timer.
func TestReplenishmentTicksCountFromTheWorldTickBefore(t *testing.T) {
	sim, a, src := newReplenishmentTickTestSim(t, 100, 10, 20000, false, nil)
	sim.serverTickPhase, sim.worldTickPhase = 0, 0
	r := a.replenishment
	replenishmentProc(sim, src, ms(2000))

	for _, c := range []struct {
		until time.Duration
		ticks int32
	}{
		{ms(2985), 0},
		{ms(2995), 1},
		{ms(16985), 14},
		{ms(16995), 15},
		{ms(18000), 15},
	} {
		runUntil(sim, c.until)
		if r.metrics.Events != c.ticks {
			t.Errorf("by %v: %d ticks, want %d", c.until, r.metrics.Events, c.ticks)
		}
	}
}

// On world ticks a cast lands ahead of the player's update, so the refresh takes the place of a tick due on it.
func TestReplenishmentRefreshOnAWorldTickDropsTheTickDueThen(t *testing.T) {
	sim, a, src := newReplenishmentTickTestSim(t, 100, 10, 20000, false, nil)
	sim.serverTickPhase, sim.worldTickPhase = 0, 0
	r := a.replenishment
	replenishmentProc(sim, src, ms(2000))
	// the first tick is due at 2.99 s. Queued after it, so only the tick's place in the update puts it second
	at(sim, ms(2500), func(sim *Simulation) { replenishmentProc(sim, src, ms(2990)) })

	for _, c := range []struct {
		until time.Duration
		ticks int32
	}{
		{ms(2995), 0},
		{ms(3975), 0},
		{ms(3985), 1},
		{ms(17985), 15},
		{ms(19000), 15},
	} {
		runUntil(sim, c.until)
		if r.metrics.Events != c.ticks {
			t.Errorf("by %v: %d ticks, want %d", c.until, r.metrics.Events, c.ticks)
		}
	}
}

// A cast out of a creature's update (a dot's tick on the target) comes after every player's update of that
// world tick, so the tick due on it has landed by the time the refresh restarts the timer.
func TestReplenishmentRefreshFromACreaturesUpdateKeepsTheTickDueThen(t *testing.T) {
	sim, a, src := newReplenishmentTickTestSim(t, 100, 10, 20000, false, nil)
	sim.serverTickPhase, sim.worldTickPhase = 0, 0
	r := a.replenishment
	target := sim.Encounter.TargetUnits[0]
	replenishmentProc(sim, src, ms(2010))
	// the first tick is due at 3 s, a map update. Queued after it, as in the test above
	at(sim, ms(2500), func(sim *Simulation) {
		at(sim, ms(3000), func(sim *Simulation) {
			sim.updating = target
			sim.Raid.ProcReplenishment(sim, src)
			sim.updating = nil
		})
	})

	for _, c := range []struct {
		until time.Duration
		ticks int32
	}{
		{ms(2995), 0},
		{ms(3005), 1},
		{ms(3995), 1},
		{ms(4005), 2},
		{ms(18005), 16},
		{ms(19000), 16},
	} {
		runUntil(sim, c.until)
		if r.metrics.Events != c.ticks {
			t.Errorf("by %v: %d ticks, want %d", c.until, r.metrics.Events, c.ticks)
		}
	}
}

// newReplenishmentPetsTestSim gives the player two mana pets under the raid's buff, 10 mana a tick each: one out
// all fight and one that waits for its summon.
func newReplenishmentPetsTestSim(t *testing.T, mapUpdateMs, worldTickMs int32) (sim *Simulation, out, summoned *timingTestPet) {
	t.Helper()
	noScaling := func(stats.Stats) stats.Stats { return stats.Stats{} }
	sim, _, _ = newReplenishmentTickTestSim(t, mapUpdateMs, worldTickMs, 20000, false, func(a *timingTestAgent) {
		out = &timingTestPet{Pet: NewPet("Out", &a.Character, stats.Stats{}, noScaling, true, true)}
		summoned = &timingTestPet{Pet: NewPet("Summoned", &a.Character, stats.Stats{}, noScaling, false, true)}
		for _, pet := range []*timingTestPet{out, summoned} {
			pet.EnableManaBarWithModifier(1)
			a.AddPet(pet)
			MakePermanent(replenishmentAura(&pet.Unit))
		}
	})
	emptyReplenishmentTestPets(out, summoned)
	return sim, out, summoned
}

// emptyReplenishmentTestPets sets the pets' mana, which a reset takes back to the sheet's.
func emptyReplenishmentTestPets(pets ...*timingTestPet) {
	for _, pet := range pets {
		pet.stats[stats.Mana] = 5000
		pet.currentMana = 0
	}
}

// A pet that isn't out queues no tick, and once it's out its ticks land with those of a pet whose timer never
// stopped: every tick time below is half a second clear of a check.
func TestReplenishmentTimerKeepsItsPlaceWhileAPetIsAway(t *testing.T) {
	for _, tc := range []struct {
		name           string
		mapMs, worldMs int32
	}{
		{"exact timing", 0, 0},
		{"live", 100, 10},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sim, out, summoned := newReplenishmentPetsTestSim(t, tc.mapMs, tc.worldMs)
			always, r := out.replenishment.metrics, summoned.replenishment

			runUntil(sim, ms(3450))
			if always.Events != 3 {
				t.Fatalf("the pet that's out ticked %d times by 3.45 s, want 3", always.Events)
			}
			if r.tickAction != nil || r.metrics.Events != 0 || !summoned.ReplenishmentAura.IsActive() {
				t.Fatalf("before the summon: a tick queued %v, %d ticks, aura up %v", r.tickAction != nil, r.metrics.Events,
					summoned.ReplenishmentAura.IsActive())
			}

			// off the timer: one restarted here would tick at 4.45 s, the raid's at 4 s
			summoned.Enable(sim, summoned)
			summoned.currentMana = 0
			for _, until := range []time.Duration{ms(3500), ms(4200), ms(5500), ms(9500)} {
				runUntil(sim, until)
				if got, want := r.metrics.Events, always.Events-3; got != want {
					t.Errorf("by %v: %d ticks since the summon, the pet that's out had %d", until, got, want)
				}
				if got, want := r.metrics.ActualGain, always.ActualGain-30; got != want {
					t.Errorf("by %v: %v mana since the summon, the pet that's out got %v", until, got, want)
				}
			}
			if r.metrics.Events != 6 {
				t.Fatalf("%d ticks from 3.45 to 9.5 s, want 6", r.metrics.Events)
			}

			// the raid's buff stays on a pet that leaves, so it's there for the next summon
			summoned.Disable(sim)
			runUntil(sim, ms(14450))
			if r.tickAction != nil || r.metrics.Events != 6 || !summoned.ReplenishmentAura.IsActive() {
				t.Errorf("after the pet left: a tick queued %v, %d ticks, aura up %v, want none queued, 6 and up",
					r.tickAction != nil, r.metrics.Events, summoned.ReplenishmentAura.IsActive())
			}
			if always.Events != 14 {
				t.Fatalf("the pet that's out ticked %d times by 14.45 s, want 14", always.Events)
			}

			summoned.Enable(sim, summoned)
			summoned.currentMana = 0
			for _, until := range []time.Duration{ms(14500), ms(15200), ms(17500)} {
				runUntil(sim, until)
				if got, want := r.metrics.Events, 6+always.Events-14; got != want {
					t.Errorf("by %v: %d ticks over both summons, want %d", until, got, want)
				}
			}
			if r.metrics.Events != 9 {
				t.Errorf("%d ticks over both summons by 17.5 s, want 9", r.metrics.Events)
			}
		})
	}
}

// On a shared map update a pet's auras tick before it regenerates (Unit::Update, then Creature::Update's
// Regenerate), so a Replenishment tick fills the mana first and its caster gets the threat.
func TestReplenishmentTicksAheadOfThePetsRegen(t *testing.T) {
	var pet *timingTestPet
	sim, a, src := newReplenishmentTickTestSim(t, 100, 10, 20000, false, func(a *timingTestAgent) {
		pet = &timingTestPet{Pet: NewPet("Pet", &a.Character, stats.Stats{stats.Spirit: 100},
			func(stats.Stats) stats.Stats { return stats.Stats{} }, false, false)}
		pet.EnableCreatureManaBar(5000, 0, 15, true)
		a.AddPet(pet)
	})
	sim.serverTickPhase, sim.worldTickPhase = 0, 0

	// summoned between the updates at 0.9 and 1 s, so it regenerates at 2.9 s
	runUntil(sim, ms(950))
	pet.Enable(sim, pet)
	if regen := pet.creatureManaRefresh.NextActionAt; regen != ms(2900) {
		t.Fatalf("first regen tick at %v, want 2.9 s", regen)
	}
	// and the cast lands between those at 1.9 and 2 s, so its first tick on the pet is at 2.9 s too
	replenishmentProc(sim, src, ms(1950))
	runUntil(sim, ms(2000))
	if tick := pet.replenishment.tickAction; tick == nil || tick.NextActionAt != ms(2900) {
		t.Fatalf("the pet's first Replenishment tick isn't queued for 2.9 s: %+v", tick)
	}
	// room for half the tick's 10 mana, and none of the regen's 37
	pet.currentMana = pet.MaxMana() - 5

	// stops ahead of the caster's own first tick, a world tick before 2.95 s
	runUntil(sim, ms(2920))
	if got := pet.replenishment.metrics.ActualGain; got != 5 {
		t.Errorf("the tick gave the pet %v mana, want the 5 it had room for", got)
	}
	if got, want := a.replenishment.metrics.threat, 1+0.5*5; !threatNear(got, want) {
		t.Errorf("the caster: threat %v, want %v", got, want)
	}
	if pet.CurrentMana() != pet.MaxMana() {
		t.Errorf("the pet's mana %v of %v after both ticks", pet.CurrentMana(), pet.MaxMana())
	}
}

// The next fight starts with the summoned pet away again, whatever it was doing as the last one ended.
func TestReplenishmentWaitsForThePetAgainNextFight(t *testing.T) {
	sim, out, summoned := newReplenishmentPetsTestSim(t, 100, 10)
	runUntil(sim, ms(2500))
	summoned.Enable(sim, summoned)
	runUntil(sim, ms(5500))
	if summoned.replenishment.tickAction == nil {
		t.Fatal("no tick queued for the pet that's out")
	}

	sim.Cleanup()
	sim.reset()
	emptyReplenishmentTestPets(out, summoned)
	ticked, always := summoned.replenishment.metrics.Events, out.replenishment.metrics.Events
	runUntil(sim, ms(5500))
	if r := summoned.replenishment; r.tickAction != nil || r.metrics.Events != ticked {
		t.Errorf("the pet that isn't out: a tick queued %v, %d ticks this fight", r.tickAction != nil, r.metrics.Events-ticked)
	}
	if got := out.replenishment.metrics.Events - always; got != 5 {
		t.Errorf("the pet that's out from the start ticked %d times by 5.5 s, want 5", got)
	}
}

// A proc's cast doesn't land on a pet that's away.
func TestReplenishmentProcSkipsAPetThatIsAway(t *testing.T) {
	var pet *timingTestPet
	sim, a, src := newReplenishmentTickTestSim(t, 0, 0, 20000, false, func(a *timingTestAgent) {
		pet = &timingTestPet{Pet: NewPet("Pet", &a.Character, stats.Stats{}, func(stats.Stats) stats.Stats { return stats.Stats{} }, true, false)}
		pet.EnableManaBarWithModifier(1)
		a.AddPet(pet)
	})
	pet.stats[stats.Mana] = 5000
	pet.currentMana = 2500
	at(sim, ms(200), func(sim *Simulation) { pet.Disable(sim) })
	replenishmentProc(sim, src, ms(500))

	runUntil(sim, ms(4000))
	if r := pet.replenishment; r.aura.IsActive() || r.tickAction != nil || r.metrics.Events != 0 {
		t.Errorf("the pet: aura up %v, a tick queued %v, %d ticks, want none of it", r.aura.IsActive(), r.tickAction != nil,
			r.metrics.Events)
	}
	// the cast, then three of the caster's own ticks
	if got, want := a.replenishment.metrics.threat, 1+3*0.5*40; !threatNear(got, want) {
		t.Errorf("the caster: threat %v, want %v", got, want)
	}
}

// A tick on anyone in the raid makes its threat for the caster.
func TestReplenishmentOnAPetMakesTheCastersThreat(t *testing.T) {
	var pet *timingTestPet
	sim, a, src := newReplenishmentTickTestSim(t, 0, 0, 20000, false, func(a *timingTestAgent) {
		pet = &timingTestPet{Pet: NewPet("Pet", &a.Character, stats.Stats{}, func(stats.Stats) stats.Stats { return stats.Stats{} }, true, false)}
		pet.EnableManaBarWithModifier(1)
		a.AddPet(pet)
	})
	pet.stats[stats.Mana] = 5000
	pet.currentMana = 2500
	replenishmentProc(sim, src, ms(500))

	runUntil(sim, ms(2000))
	if got := pet.replenishment.metrics; got.Events != 1 || got.Gain != 10 || got.threat != 0 {
		t.Errorf("the pet: %d ticks giving %v, threat %v, want 1 of 10 and no threat of its own", got.Events, got.Gain,
			got.threat)
	}
	// one cast, split over both, then half of each one's tick
	if got, want := a.replenishment.metrics.threat, 1+0.5*40+0.5*10; !threatNear(got, want) {
		t.Errorf("the caster: threat %v, want %v", got, want)
	}
}

// newReplenishmentRaidTestSim gives the caster, at half of 20000 mana, pets that are out from the start: mana
// of them with a pool of 5000 each, the i-th at i+1 hundredths of it, and whatever more adds.
func newReplenishmentRaidTestSim(t *testing.T, mana int, more func(a *timingTestAgent, newPet func(name string, out bool) *timingTestPet)) (
	*Simulation, *timingTestAgent, ReplenishmentSource, []*timingTestPet) {
	t.Helper()
	var pets, extra []*timingTestPet
	sim, a, src := newReplenishmentTickTestSim(t, 0, 0, 20000, false, func(a *timingTestAgent) {
		newPet := func(name string, out bool) *timingTestPet {
			pet := &timingTestPet{Pet: NewPet(name, &a.Character, stats.Stats{}, func(stats.Stats) stats.Stats { return stats.Stats{} }, out, true)}
			a.AddPet(pet)
			return pet
		}
		for i := range mana {
			pet := newPet("Mana "+string(rune('A'+i)), true)
			pet.EnableManaBarWithModifier(1)
			pets = append(pets, pet)
		}
		if more != nil {
			more(a, func(name string, out bool) *timingTestPet {
				pet := newPet(name, out)
				extra = append(extra, pet)
				return pet
			})
		}
	})
	for i, pet := range pets {
		pet.stats[stats.Mana] = 5000
		pet.currentMana = float64(50 * (i + 1))
	}
	for _, pet := range extra {
		if pet.HasManaBar() {
			pet.stats[stats.Mana] = 5000
			pet.currentMana = 1
		}
	}
	return sim, a, src, pets
}

func replenishmentUp(units ...*Unit) (up []string) {
	for _, unit := range units {
		if unit.ReplenishmentAura.IsActive() {
			up = append(up, unit.Label)
		}
	}
	return up
}

// With more than ten on mana a cast lands on the ten lowest for their pool, and the others keep the aura
// they had (spell_gen_replenishment).
func TestReplenishmentLandsOnTheTenLowestOnMana(t *testing.T) {
	sim, a, src, pets := newReplenishmentRaidTestSim(t, 12, nil)
	units := []*Unit{&a.Unit}
	for _, pet := range pets {
		units = append(units, &pet.Unit)
	}
	label := func(from, to int) (labels []string) {
		for _, pet := range pets[from:to] {
			labels = append(labels, pet.Label)
		}
		return labels
	}

	replenishmentProc(sim, src, ms(500))
	runUntil(sim, ms(600))
	if got, want := replenishmentUp(units...), label(0, 10); !slices.Equal(got, want) {
		t.Fatalf("the first cast landed on %v, want the ten lowest %v", got, want)
	}

	// the lowest fills up, so the next cast takes the eleventh in its place
	pets[0].currentMana = pets[0].MaxMana()
	replenishmentProc(sim, src, ms(3000))
	runUntil(sim, ms(3100))
	if got, want := replenishmentUp(units...), label(0, 11); !slices.Equal(got, want) {
		t.Fatalf("after the second cast %v have the aura, want %v", got, want)
	}
	// the first cast's aura on the one left out runs its 15 s from 0.5 s, the second cast's from 3 s
	runUntil(sim, ms(15600))
	if got, want := replenishmentUp(units...), label(1, 11); !slices.Equal(got, want) {
		t.Errorf("at 15.6 s %v have the aura, want %v", got, want)
	}
}

// A cast counts neither a pet that's away nor a guardian without a pet bar, so they take no place from the
// ten.
func TestReplenishmentSkipsAPetThatIsAwayAndAGuardianWithoutAPetBar(t *testing.T) {
	var away, uncontrollable *timingTestPet
	sim, a, src, pets := newReplenishmentRaidTestSim(t, 9, func(a *timingTestAgent, newPet func(string, bool) *timingTestPet) {
		away = newPet("Away", false)
		away.EnableManaBarWithModifier(1)
		uncontrollable = newPet("Uncontrollable", true)
		uncontrollable.EnableManaBarWithModifier(1)
		uncontrollable.Uncontrollable = true
	})

	replenishmentProc(sim, src, ms(500))
	runUntil(sim, ms(600))
	if !a.ReplenishmentAura.IsActive() {
		t.Error("the caster, tenth lowest of the ten a cast can take, has no aura")
	}
	for _, pet := range pets {
		if !pet.ReplenishmentAura.IsActive() {
			t.Errorf("%s has no aura", pet.Label)
		}
	}
	if away.ReplenishmentAura.IsActive() {
		t.Error("the cast landed on the pet that's away")
	}
	if uncontrollable.ReplenishmentAura != nil {
		t.Error("a guardian without a pet bar has a Replenishment aura")
	}
}

// A summon whose power is mana with nothing in the pool is a target like any other, and the lowest of them.
func TestReplenishmentCountsAnEmptyManaPool(t *testing.T) {
	sim, a, src, pets := newReplenishmentRaidTestSim(t, 9, func(a *timingTestAgent, newPet func(string, bool) *timingTestPet) {
		newPet("Wolf 1", true).EmptyManaPool = true
		newPet("Wolf 2", true).EmptyManaPool = true
	})
	units := []*Unit{&a.Unit}
	var want []string
	for i, pet := range pets {
		units = append(units, &pet.Unit)
		if i < 8 {
			want = append(want, pet.Label)
		}
	}

	replenishmentProc(sim, src, ms(500))
	runUntil(sim, ms(600))
	if got := replenishmentUp(units...); !slices.Equal(got, want) {
		t.Errorf("with two empty pools in the raid the cast landed on %v, want the eight lowest %v", got, want)
	}
}

// The raid's buff only goes on a unit with mana.
func TestReplenishmentFromTheRaidNeedsMana(t *testing.T) {
	for _, mana := range []bool{true, false} {
		timingTestConstruct = func(a *timingTestAgent) {
			if mana {
				a.EnableManaBar()
			}
		}
		_, a := newTickTestSim(t, 0, 0, func(a *timingTestAgent) {
			applyBuffEffects(a, &proto.RaidBuffs{}, &proto.PartyBuffs{}, &proto.IndividualBuffs{VampiricTouch: true})
		})
		timingTestConstruct = nil
		if got := a.ReplenishmentAura.IsActive(); got != mana {
			t.Errorf("a unit with mana %v: the raid's aura is up %v", mana, got)
		}
	}
}

// What the mana potion and gem heuristics count on from Replenishment: the tick a second while it's up.
func TestReplenishmentManaPerSecond(t *testing.T) {
	sim, a, src := newReplenishmentTestSim(t, 20000, false)
	if got := a.ReplenishmentManaPerSecond(); got != 0 {
		t.Errorf("%v a second without the aura, want 0", got)
	}
	replenishmentProc(sim, src, ms(500))
	runUntil(sim, ms(600))
	if got := a.ReplenishmentManaPerSecond(); got != 40 {
		t.Errorf("%v a second under a proc's aura at 20000 mana, want 40", got)
	}
	runUntil(sim, ms(16000))
	if got := a.ReplenishmentManaPerSecond(); got != 0 {
		t.Errorf("%v a second once the aura ran out, want 0", got)
	}

	_, raid, _ := newReplenishmentTestSim(t, 20000, true)
	if got := raid.ReplenishmentManaPerSecond(); got != 40 {
		t.Errorf("%v a second under the raid's buff at 20000 mana, want 40", got)
	}
}
