package core

import (
	"testing"
	"time"

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
