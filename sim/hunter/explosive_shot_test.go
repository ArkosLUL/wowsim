package hunter

import (
	"testing"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
	"github.com/wowsims/wotlk/sim/core/serverdata"
)

// mod-spell-tweaks doubles both ends of Explosive Shot's tick roll on every rank: 772-928 at rank 4, 650-782 at
// rank 3, before 53352's ranged attack power.
func TestExplosiveShotTickBase(t *testing.T) {
	hunter := hunterWithTalents(t, SVTalents, proto.Hunter_Options_Wolf)
	for _, tc := range []struct {
		spell    *core.Spell
		min, max float64
	}{
		{hunter.ExplosiveShotR4, 772, 928},
		{hunter.ExplosiveShotR3, 650, 782},
	} {
		id := tc.spell.ActionID.SpellID
		if conflicts := tc.spell.ServerConflicts(); len(conflicts) != 0 {
			t.Errorf("%d: server data conflicts %v", id, conflicts)
		}
		server := serverdata.SpellByID(id).Effects[0]
		if low, high := float64(server.BasePoints+1), float64(server.BasePoints+server.DieSides); low != tc.min || high != tc.max {
			t.Errorf("%d: server data rolls %v-%v, want %v-%v", id, low, high, tc.min, tc.max)
		}
		if tick := tc.spell.Dot(hunter.CurrentTarget).Tick; tick.Min != tc.min || tick.Max != tc.max {
			t.Errorf("%d: ticks roll %v-%v, want %v-%v", id, tick.Min, tick.Max, tc.min, tc.max)
		}
	}
}
