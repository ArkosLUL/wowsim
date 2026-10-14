package core

import (
	"math"
	"testing"

	"github.com/wowsims/wotlk/sim/core/stats"
)

// The APL's swap to the main set is a swap like any other, stat change and GCD included. Only a new
// iteration puts the main set back without either.
func TestItemSwapBackToTheMainSet(t *testing.T) {
	sim, a := metaSwapTestSim(t, true, nil)
	built := a.GetStats()
	expect := func(when string, gcd bool) {
		t.Helper()
		got := a.GetStats()
		for i := range got {
			if math.Abs(got[i]-built[i]) > 1e-6 {
				t.Errorf("%s: %s %v, want %v", when, stats.Stat(i).StatName(), got[i], built[i])
			}
		}
		if a.ItemSwap.IsSwapped() {
			t.Errorf("%s: still swapped", when)
		}
		if ready := a.GCD.IsReady(sim); ready == gcd {
			t.Errorf("%s: GCD ready %v, want %v", when, ready, !gcd)
		}
	}

	a.ItemSwap.SwapItems(sim, a.ItemSwap.slots)
	if a.GetStats() == built {
		t.Fatal("the swap moves no stat, nothing to tell")
	}
	a.GCD.Reset()
	a.ItemSwap.reset(sim)
	expect("swapped back mid-fight", true)

	a.ItemSwap.SwapItems(sim, a.ItemSwap.slots)
	sim.Cleanup()
	sim.reset()
	expect("at the next iteration", false)
}
