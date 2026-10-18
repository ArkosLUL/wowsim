package restoration

import (
	"slices"
	"testing"
	"time"

	"github.com/wowsims/wotlk/sim/core"
)

// SpellInfo::CheckTarget keeps the dead out of the chain's search, though their deficit is the largest.
func TestChainHealSkipsTheDead(t *testing.T) {
	r := newChainHealRaid()
	sim := r.sim
	main, second, third := &r.shamans[0].Unit, &r.shamans[1].Unit, &r.shamans[2].Unit

	sim.Reset()
	sim.PrePull()
	for sim.CurrentTime < 0 && !sim.Step() {
	}
	r.healed = r.healed[:0]
	r.watch.Activate(sim)
	second.RemoveHealth(sim, second.MaxHealth())
	third.RemoveHealth(sim, 100)
	if !r.shamans[0].ChainHeal.Cast(sim, main) {
		t.Fatal("Chain Heal wasn't cast")
	}
	for sim.CurrentTime < 3*time.Second && !sim.Step() {
	}
	sim.Cleanup()

	if want := []*core.Unit{main, third}; !slices.Equal(r.healed, want) {
		t.Errorf("healed %v, want %v", unitLabels(r.healed), unitLabels(want))
	}
}
