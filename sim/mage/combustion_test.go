package mage

import (
	"testing"
	"time"

	"github.com/wowsims/wotlk/sim/core"
)

// spell_mage_combustion adds a hit's stack 1 ms later, so on the mage's next tick: a Blast Wave's hits all roll
// on the stacks from before it, and each landed hit's stack shows up a tick on.
func TestCombustionStacksComeOnTheNextTick(t *testing.T) {
	sim, mage := newProcSim(t, procMage(FireTalents, "p3_fire_alliance", `{"type": "TypeAPL", "priorityList": []}`, PlayerOptionsFire),
		core.NewDefaultTarget(), core.NewDefaultTarget(), core.NewDefaultTarget())
	combustion := mage.GetSpell(core.ActionID{SpellID: 11129})
	if combustion == nil || mage.BlastWave == nil {
		t.Fatalf("FireTalents lacks Combustion or Blast Wave")
	}
	target := sim.GetTargetUnit(0)

	combustion.Cast(sim, target)
	crit := mage.BlastWave.BonusCritRating
	mage.BlastWave.Cast(sim, target)
	if got := mage.CombustionAura.GetStacks(); got != 1 || mage.BlastWave.BonusCritRating != crit {
		t.Fatalf("right after Blast Wave: %d stacks, crit rating %v (was %v), want the cast's 1 stack alone",
			got, mage.BlastWave.BonusCritRating, crit)
	}

	landed, crits := int32(0), int32(0)
	for i := range 3 {
		metrics := mage.BlastWave.SpellMetrics[sim.GetTargetUnit(int32(i)).UnitIndex]
		landed += metrics.Hits + metrics.Crits
		crits += metrics.Crits
	}
	if crits >= 3 {
		t.Fatalf("Blast Wave crit all three targets, which ends Combustion: pick another seed")
	}

	castAt := sim.CurrentTime
	for sim.CurrentTime < castAt+time.Second && !sim.Step() {
	}
	if got, want := mage.CombustionAura.GetStacks(), 1+landed; got != want {
		t.Errorf("a tick on: %d stacks, want %d (the cast's and %d landed hits')", got, want, landed)
	}
}
