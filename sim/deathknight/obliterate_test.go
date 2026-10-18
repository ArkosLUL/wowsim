package deathknight

import (
	"math"
	"testing"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/stats"
)

// Spell::EffectWeaponDmg's Obliterate branch runs at launch, so only on a hit that isn't missed, dodged or parried,
// and takes the diseases off before MeleeDamageBonusDone reads Rage of Rivendare or Tundra Stalker. A partial block
// comes off after that bonus, the whole block value or all of a smaller hit.
func TestObliterateOutcomeTakesTheDiseasesAtLaunch(t *testing.T) {
	target := &core.Unit{Type: core.EnemyUnit, PseudoStats: stats.NewPseudoStats()}
	target.AddStat(stats.BlockValue, 200)
	diseased := true
	dk := &Deathknight{RoRTSBonus: func(*core.Unit) float64 { return core.TernaryFloat64(diseased, 1.1, 1) }}

	for _, c := range []struct {
		name          string
		outcome       core.HitOutcome
		damage, want  float64
		launches      bool
		bonusAfterAll float64
	}{
		{"miss", core.OutcomeMiss, 0, 0, false, 1.1},
		{"dodge", core.OutcomeDodge, 0, 0, false, 1.1},
		{"parry", core.OutcomeParry, 0, 0, false, 1.1},
		{"hit", core.OutcomeHit, 1100, 1000, true, 1},
		{"crit", core.OutcomeCrit, 2200, 2000, true, 1},
		{"partial block", core.OutcomeHit | core.OutcomeBlock, 1100 - 200, 1000 - 200, true, 1},
		{"blocked whole", core.OutcomeHit | core.OutcomeBlock, 0, 0, true, 1},
	} {
		diseased = true
		launched := false
		roll := func(_ *core.Simulation, result *core.SpellResult, _ *core.AttackTable) {
			result.Outcome, result.Damage = c.outcome, c.damage
		}
		result := &core.SpellResult{Target: target}
		dk.obliterateOutcome(roll, 1.1, func(*core.Simulation, *core.Unit) {
			launched, diseased = true, false
		})(nil, result, nil)
		if launched != c.launches || math.Abs(result.Damage-c.want) > 1e-9 {
			t.Errorf("%s: took the diseases %v and dealt %v, want %v and %v", c.name, launched, result.Damage, c.launches, c.want)
		}
		if got := dk.RoRTSBonus(target); got != c.bonusAfterAll {
			t.Errorf("%s: bonus %v after, want %v", c.name, got, c.bonusAfterAll)
		}
	}
}
