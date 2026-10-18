package tank

import (
	"math"
	"testing"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/stats"
)

// spell_dk_icebound_fortitude has no cap: 467 skill over 400 gives -100%, 480 gives -102%, and the damage taken
// functions clamp the hit at 0. The expiry has to give back exactly what the other auras left.
func TestIceboundFortitudeAtFullReduction(t *testing.T) {
	sim := core.NewSim(genericBossRequest())
	dk := sim.Raid.Parties[0].Players[0].(*TankDeathknight)
	swing := sim.Encounter.TargetUnits[0].AutoAttacks.MHAuto()
	hit := func() float64 {
		return swing.CalcDamage(sim, &dk.Unit, 10000, swing.OutcomeAlwaysHit).Damage
	}

	for _, tc := range []struct {
		overCap int32
		blocked bool // every hit 0 under Icebound Fortitude
	}{
		{466, false},
		{467, true},
		{480, true},
	} {
		sim.Reset()
		dk.PseudoStats.BonusDefenseSkill += tc.overCap - dk.DefenseSkill()
		dk.AddStatDynamic(sim, stats.Armor, -dk.GetStat(stats.Armor))
		multiplier := dk.PseudoStats.DamageTakenMultiplier
		before := hit()
		// another damage taken aura, here alone and then taken on under Icebound Fortitude
		dk.PseudoStats.DamageTakenMultiplier = multiplier * 0.8
		reduced := hit()
		dk.PseudoStats.DamageTakenMultiplier = multiplier
		if before <= 0 || reduced >= before {
			t.Fatalf("%d over 400: a hit deals %v, %v under the other aura", tc.overCap, before, reduced)
		}

		dk.IceboundFortitudeAura.Activate(sim)
		switch got := hit(); {
		case tc.blocked && got != 0:
			t.Errorf("%d over 400: a hit deals %v under Icebound Fortitude, want 0", tc.overCap, got)
		case !tc.blocked && (got <= 0 || math.Abs(got-before/100) > 1):
			t.Errorf("%d over 400: a hit deals %v under Icebound Fortitude, want 1%% of %v", tc.overCap, got, before)
		}
		dk.PseudoStats.DamageTakenMultiplier *= 0.8
		dk.IceboundFortitudeAura.Deactivate(sim)
		if tc.blocked && dk.PseudoStats.DamageTakenMultiplier != multiplier*0.8 {
			t.Errorf("%d over 400: damage taken x%v after it, want x%v", tc.overCap, dk.PseudoStats.DamageTakenMultiplier, multiplier*0.8)
		}
		if got := hit(); got != reduced {
			t.Errorf("%d over 400: a hit deals %v after it, want %v", tc.overCap, got, reduced)
		}
		dk.PseudoStats.DamageTakenMultiplier = multiplier
		sim.Cleanup()
	}
}
