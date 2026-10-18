package deathknight

import (
	"testing"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/stats"
)

// Obliterate's off-hand 66974 has half the main hand's 584 in the DBC. Spell::EffectWeaponDmg doubles it back
// before the off hand's TOTAL_PCT, here with Nerves of Cold Steel: int(584 x 0.5 x 1.25) = 365, not 182.
// Frost Strike's 66962 is frost, so it takes neither.
func TestThreatOfThassarianOffHandFixedBonus(t *testing.T) {
	unit := &core.Unit{PseudoStats: stats.NewPseudoStats()}
	obliterate := &core.Spell{Unit: unit, SpellSchool: core.SpellSchoolPhysical, TotalPctMultiplier: 1.25}
	if got := offHandStrikeFixed(obliterate, 292); got != 365 {
		t.Errorf("off-hand Obliterate fixed bonus %v, want 365", got)
	}
	frostStrike := &core.Spell{Unit: unit, SpellSchool: core.SpellSchoolFrost, TotalPctMultiplier: 1.25}
	if got := offHandStrikeFixed(frostStrike, 125); got != 125 {
		t.Errorf("off-hand Frost Strike fixed bonus %v, want 125", got)
	}
}
