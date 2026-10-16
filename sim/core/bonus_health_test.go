package core

import (
	"testing"

	"github.com/wowsims/wotlk/sim/core/stats"
)

// AuraEffect::HandleAuraModIncreaseHealth: the bonus comes off current health, to 1 when health isn't above it,
// then off max health. Not damage: no damage taken event and no TMI entry, even on a tank.
func TestLoseBonusHealth(t *testing.T) {
	sim, a := newTickTestSim(t, 0, 0, nil)
	unit := &a.Unit
	unit.Metrics.isTanking = true
	metrics := unit.NewHealthMetrics(ActionID{SpellID: 55233})

	// 50,000 max health and a 7,500 bonus; health is what the unit has with the bonus up
	for _, tc := range []struct {
		name         string
		health, want float64
	}{
		{"from full", 57500, 50000},
		{"at 30%", 22500, 15000},
		{"at the bonus", 7500, 1},
		{"dead", 0, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sim.Cleanup()
			sim.reset()
			unit.AddStatsDynamic(sim, stats.Stats{stats.Health: 50000 - unit.MaxHealth()})
			unit.GainHealth(sim, unit.MaxHealth(), metrics)

			unit.GainBonusHealth(sim, 7500, metrics)
			if unit.MaxHealth() != 57500 || unit.CurrentHealth() != 57500 {
				t.Fatalf("%v of %v health with the bonus up, want 57500 of 57500", unit.CurrentHealth(), unit.MaxHealth())
			}
			if tc.health < 57500 {
				unit.RemoveHealth(sim, 57500-tc.health)
			}

			events, tmi := unit.DamageTakenHealthMetrics.Events, len(unit.Metrics.tmiList)
			unit.LoseBonusHealth(sim, 7500)

			if unit.CurrentHealth() != tc.want || unit.MaxHealth() != 50000 {
				t.Errorf("%v of %v health after the bonus came off, want %v of 50000", unit.CurrentHealth(), unit.MaxHealth(), tc.want)
			}
			if unit.DamageTakenHealthMetrics.Events != events || len(unit.Metrics.tmiList) != tmi {
				t.Errorf("taking the bonus off counted as damage taken")
			}
		})
	}
}

// Player::UpdateMaxHealth puts a health percent aura over the bonus too, so with one up and health topped off,
// the bonus going leaves health over the new max, and Unit::SetMaxHealth caps it.
func TestLoseBonusHealthUnderAHealthPercent(t *testing.T) {
	sim, a := newTickTestSim(t, 0, 0, func(a *timingTestAgent) {
		a.MultiplyStat(stats.Health, 1.25)
	})
	unit := &a.Unit
	metrics := unit.NewHealthMetrics(ActionID{SpellID: 12976})
	unit.AddStatsDynamic(sim, stats.Stats{stats.Health: (50000 - unit.MaxHealth()) / 1.25})
	unit.GainHealth(sim, unit.MaxHealth(), metrics)

	unit.GainBonusHealth(sim, 10000, metrics)
	if unit.MaxHealth() != 62500 || unit.CurrentHealth() != 60000 {
		t.Fatalf("%v of %v health with the bonus up, want 60000 of 62500", unit.CurrentHealth(), unit.MaxHealth())
	}
	unit.GainHealth(sim, 2500, metrics)
	unit.LoseBonusHealth(sim, 10000)
	if unit.CurrentHealth() != 50000 || unit.MaxHealth() != 50000 {
		t.Errorf("%v of %v health after the bonus came off, want 50000 of 50000", unit.CurrentHealth(), unit.MaxHealth())
	}
}

// Unit::CountPctFromMaxHealth works on the whole max health and truncates.
func TestCountPctFromMaxHealth(t *testing.T) {
	sim, a := newTickTestSim(t, 0, 0, nil)
	unit := &a.Unit
	unit.AddStatsDynamic(sim, stats.Stats{stats.Health: 38617.88 - unit.MaxHealth()})
	// 38617 x 15 / 100 = 5792.55
	if got := unit.CountPctFromMaxHealth(15); got != 5792 {
		t.Errorf("15%% of 38617.88: %v, want 5792", got)
	}
}
