package tank

import (
	"testing"
	"time"
)

// 56815 needs 56817 up: 10 s with one charge, which spell_proc takes only on a hit or crit.
func TestRuneStrikeWindow(t *testing.T) {
	sim, dk, start := newTankFixesDK(t)
	if dk.RuneStrikeAura.Duration != 10*time.Second {
		t.Errorf("window lasts %v, want 10s", dk.RuneStrikeAura.Duration)
	}
	start()
	target := dk.CurrentTarget
	metrics := &dk.RuneStrike.SpellMetrics[target.UnitIndex]
	// the table's 60% miss cap, so both outcomes come up
	table := dk.AttackTables[target.UnitIndex]
	baseMiss := table.BaseMissPct
	table.BaseMissPct = 100
	defer func() { table.BaseMissPct = baseMiss }()

	for i := 0; i < 40 && (metrics.Misses == 0 || metrics.Hits+metrics.Crits == 0); i++ {
		dk.RuneStrikeAura.Activate(sim)
		misses := metrics.Misses
		dk.RuneStrike.SkipCastAndApplyEffects(sim, target)
		missed := metrics.Misses > misses
		if up := dk.RuneStrikeAura.IsActive(); up != missed {
			t.Fatalf("strike %d (missed: %v): window up %v, want it up only after a miss", i, missed, up)
		}
	}
	if metrics.Misses == 0 || metrics.Hits+metrics.Crits == 0 {
		t.Fatalf("%d misses, %d hits, %d crits: want both outcomes", metrics.Misses, metrics.Hits, metrics.Crits)
	}
}
