package core

import (
	"slices"
	"testing"
	"time"

	"github.com/wowsims/wotlk/sim/core/proto"
)

func incomingDamageTestSim(dummies int32, damage float64) *Simulation {
	return NewSim(&proto.RaidSimRequest{
		SimOptions: &proto.SimOptions{RandomSeed: 1, IsTest: true, Iterations: 1},
		Raid: &proto.Raid{
			Parties:                 []*proto.Party{{}, {}},
			TargetDummies:           dummies,
			IncomingDamagePerSecond: damage,
		},
		Encounter: &proto.Encounter{
			Duration: 120,
			Targets:  []*proto.Target{NewDefaultTarget()},
		},
	})
}

func stepTo(sim *Simulation, at time.Duration) {
	StartDelayedAction(sim, DelayedActionOptions{DoAt: at, OnAction: func(*Simulation) {}})
	for sim.CurrentTime < at && !sim.Step() {
	}
}

func dummyUnits(sim *Simulation) []*Unit {
	var units []*Unit
	for _, party := range sim.Raid.Parties {
		for _, player := range party.Players {
			if dummy, ok := player.(*TargetDummy); ok {
				units = append(units, &dummy.Unit)
			}
		}
	}
	return units
}

func TestDummiesHaveHealthBarsOnlyWithIncomingDamage(t *testing.T) {
	with := incomingDamageTestSim(9, 1000)
	with.Reset()
	for i, unit := range dummyUnits(with) {
		want := float64(DefaultDummyRaidHealth)
		if i == 0 {
			want = DefaultDummyTankHealth
		}
		if !unit.HasHealthBar() || unit.MaxHealth() != want || unit.CurrentHealth() != want {
			t.Errorf("dummy %d: health bar %v, %v of %v, want full %v", i+1, unit.HasHealthBar(), unit.CurrentHealth(), unit.MaxHealth(), want)
		}
		if !unit.CountsOverheal() {
			t.Errorf("dummy %d doesn't count overhealing", i+1)
		}
	}

	without := incomingDamageTestSim(9, 0)
	without.Reset()
	for i, unit := range dummyUnits(without) {
		if unit.HasHealthBar() || unit.CountsOverheal() {
			t.Errorf("dummy %d has a health bar without incoming damage", i+1)
		}
	}
}

func TestDummyHealthByPhase(t *testing.T) {
	for phase, want := range map[int32][2]int32{1: {40000, 20000}, 2: {45000, 22500}, 5: {60000, 30000}} {
		if tank, raid := DummyHealthForPhase(phase); tank != want[0] || raid != want[1] {
			t.Errorf("phase %d: %d and %d, want %v", phase, tank, raid, want)
		}
	}
	if tank, raid := DummyHealthForPhase(0); tank != DefaultDummyTankHealth || raid != DefaultDummyRaidHealth {
		t.Errorf("phase 0: %d and %d, want phase 1's", tank, raid)
	}

	sim := NewSim(&proto.RaidSimRequest{
		SimOptions: &proto.SimOptions{RandomSeed: 1, Iterations: 1},
		Raid: &proto.Raid{
			Parties:                 []*proto.Party{{}, {}},
			TargetDummies:           3,
			IncomingDamagePerSecond: 1000,
			DummyTankHealth:         50000,
			DummyRaidHealth:         25000,
		},
		Encounter: &proto.Encounter{Duration: 60, Targets: []*proto.Target{NewDefaultTarget()}},
	})
	sim.Reset()
	for i, want := range []float64{50000, 25000, 25000} {
		if got := dummyUnits(sim)[i].MaxHealth(); got != want {
			t.Errorf("dummy %d has %v health, want %v", i+1, got, want)
		}
	}
}

// Half the damage hits dummy 1 every 2 s, the other half four of the eight others every 4 s.
func TestIncomingDamageSplitsBetweenTankAndRaid(t *testing.T) {
	sim := incomingDamageTestSim(9, 2000)
	sim.Reset()
	dummies := dummyUnits(sim)
	tank, raid := dummies[0], dummies[1:]

	stepTo(sim, 1900*time.Millisecond)
	if tank.CurrentHealth() != DefaultDummyTankHealth {
		t.Errorf("tank at %v before the first hit", tank.CurrentHealth())
	}
	stepTo(sim, 2100*time.Millisecond)
	if want := float64(DefaultDummyTankHealth - 2000); tank.CurrentHealth() != want {
		t.Errorf("tank at %v after 2 s, want %v (a hit of 1000 a second)", tank.CurrentHealth(), want)
	}
	for _, unit := range raid {
		if unit.CurrentHealth() != DefaultDummyRaidHealth {
			t.Errorf("a raid dummy took damage before 4 s")
		}
	}

	stepTo(sim, 4100*time.Millisecond)
	hurt := 0
	for _, unit := range raid {
		if lost := DefaultDummyRaidHealth - unit.CurrentHealth(); lost == 1000 {
			hurt++
		} else if lost != 0 {
			t.Errorf("a raid dummy lost %v, want 0 or 1000", lost)
		}
	}
	if hurt != 4 {
		t.Errorf("%d raid dummies hurt by the first raid hit, want 4", hurt)
	}
	if sim.Raid.IncomingDamageDealt() != 2000+2000+4*1000 {
		t.Errorf("dealt %v, want 8000", sim.Raid.IncomingDamageDealt())
	}
}

func TestIncomingDamageStopsAtOneHealth(t *testing.T) {
	sim := incomingDamageTestSim(9, 100000)
	sim.Reset()
	tank := dummyUnits(sim)[0]
	stepTo(sim, 20*time.Second)
	if tank.CurrentHealth() != 1 {
		t.Errorf("tank at %v, want it held at 1", tank.CurrentHealth())
	}
	if tank.Metrics.Died {
		t.Errorf("tank died")
	}
}

// A dummy that damage reached sees a hit taken, which is what effects waiting on one are armed by.
func TestIncomingDamageIsAHitTaken(t *testing.T) {
	sim := incomingDamageTestSim(2, 4000)
	var hits []float64
	sim.Reset()
	tank := dummyUnits(sim)[0]
	probe := tank.RegisterAura(Aura{
		Label:    "hit probe",
		Duration: NeverExpires,
		OnSpellHitTaken: func(_ *Aura, _ *Simulation, _ *Spell, result *SpellResult) {
			hits = append(hits, result.Damage)
		},
	})
	probe.Activate(sim)
	stepTo(sim, 5*time.Second)
	if want := []float64{4000, 4000}; !slices.Equal(hits, want) {
		t.Errorf("tank saw hits %v by 5 s, want %v", hits, want)
	}
}

func TestNoIncomingDamageWithoutDummies(t *testing.T) {
	sim := incomingDamageTestSim(0, 5000)
	sim.Reset()
	stepTo(sim, 10*time.Second)
	if sim.Raid.IncomingDamageDealt() != 0 {
		t.Errorf("dealt %v damage to a raid without dummies", sim.Raid.IncomingDamageDealt())
	}
}

// Every healer page ships a default for each phase, and a later phase gets more gear to heal with.
func TestPageIncomingDamageDefaultsCoverEveryPhase(t *testing.T) {
	for dir, variants := range map[string][]string{
		"restoration_shaman": {"default"},
		"restoration_druid":  {"default"},
		"holy_paladin":       {"default"},
		"healing_priest":     {"disc", "holy"},
	} {
		for _, variant := range variants {
			var first, last float64
			for phase := int32(1); phase <= 5; phase++ {
				damage := PageIncomingDamage("../../ui/"+dir, variant, phase)
				if damage <= 0 {
					t.Errorf("%s %s: no default for phase %d", dir, variant, phase)
				}
				if phase == 1 {
					first = damage
				}
				last = damage
			}
			if last <= first {
				t.Errorf("%s %s: phase 5 defaults to %v, no more than phase 1's %v", dir, variant, last, first)
			}
		}
	}
}
