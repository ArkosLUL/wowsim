package core

import (
	"testing"

	"github.com/wowsims/wotlk/sim/core/proto"
)

// A target's combat reach is the boss dummy's 2.5 unless the encounter gives its model's.
func TestEnemyCombatReach(t *testing.T) {
	for _, tc := range []struct {
		reach, want float64
	}{
		{0, 2.5},
		{7.5, 7.5},
		{1.5, 1.5},
	} {
		if got := NewTarget(&proto.Target{CombatReach: tc.reach}, 0).EnemyCombatReach; got != tc.want {
			t.Errorf("combat_reach %v: reach %v, want %v", tc.reach, got, tc.want)
		}
	}
}
