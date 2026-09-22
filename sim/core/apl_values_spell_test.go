package core

import (
	"testing"
	"time"
)

// A hardcast lands on the next server tick (cast.go's makeCastFunc), so the predicted duration
// needs the same rounding, not just the haste-scaled cast time.
func TestAPLValueSpellCastTimeMatchesTheActualLanding(t *testing.T) {
	for _, mapUpdateMs := range []int32{100, 0} {
		var spell *Spell
		sim, _ := newTimingTestSim(t, mapUpdateMs, func(a *timingTestAgent) {
			spell = a.RegisterSpell(SpellConfig{
				ActionID: ActionID{SpellID: testSpellNoServerData},
				Cast: CastConfig{
					DefaultCast: Cast{GCD: GCDDefault, CastTime: ms(1250)},
				},
			})
		})
		value := &APLValueSpellCastTime{spell: spell}
		casts := recordCasts(spell)
		sim.PrePull()

		start := ms(510)
		var predicted time.Duration
		at(sim, start, func(sim *Simulation) {
			predicted = value.GetDuration(sim)
			spell.Cast(sim, nil)
		})
		runUntil(sim, 3*time.Second)

		if len(*casts) != 1 {
			t.Fatalf("%d ms: %d casts landed, want 1", mapUpdateMs, len(*casts))
		}
		if want := (*casts)[0] - start; predicted != want {
			t.Errorf("%d ms: predicted %v, want %v (the actual landing)", mapUpdateMs, predicted, want)
		}
	}
}

func TestAPLValueSpellCastTimeLeavesInstantCastsAlone(t *testing.T) {
	var spell *Spell
	sim, _ := newTimingTestSim(t, 100, func(a *timingTestAgent) {
		spell = a.RegisterSpell(SpellConfig{
			ActionID: ActionID{SpellID: testSpellNoServerData},
			Cast:     CastConfig{DefaultCast: Cast{GCD: GCDDefault}},
		})
	})
	value := &APLValueSpellCastTime{spell: spell}
	sim.PrePull()

	if got := value.GetDuration(sim); got != 0 {
		t.Errorf("instant cast: predicted %v, want 0", got)
	}
}
