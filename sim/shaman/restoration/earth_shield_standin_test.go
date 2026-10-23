package restoration

import (
	"testing"
	"time"

	"github.com/wowsims/wotlk/sim/core"
)

// charges is how many of Earth Shield's charges have healed the target.
func charges(shaman *RestorationShaman, target *core.Unit) int32 {
	metrics := shaman.EarthShield.SpellMetrics[target.UnitIndex]
	return metrics.Hits + metrics.Crits
}

// Nothing damages the sim's raid, so Earth Shield's charges come off a stand-in hit taken at the page's rate
// (earth_shield_p_p_m), the first a period after the cast, 379's 3.5 s cooldown spacing them.
func TestEarthShieldChargesFromStandInHits(t *testing.T) {
	for _, c := range []struct {
		name string
		ppm  int32
		at   time.Duration
		want int32
	}{
		{"no rate, no charge", 0, 60 * time.Second, 0},
		{"12 a minute", 12, 11 * time.Second, 2},
		{"12 a minute, before the first", 12, 4900 * time.Millisecond, 0},
		// hits at 1, 2, 3 ... s, of which one in four gets through: 1, 5, 9 and 13 s
		{"60 a minute, 3.5 s apart", 60, 14 * time.Second, 4},
	} {
		sim, shaman := healSetup{ppm: c.ppm, dummies: 1}.build(t)
		startFight(sim)
		target := targetDummies(sim)[0]
		shaman.EarthShield.SkipCastAndApplyEffects(sim, target)
		runUntil(sim, c.at)
		if got := charges(shaman, target); got != c.want {
			t.Errorf("%s: %d charges by %s, want %d", c.name, got, c.at, c.want)
		}
	}
}

// The shield ends with its last charge, 6 and 1 more a point of Improved Earth Shield, and stops taking hits.
func TestEarthShieldEndsWithItsLastCharge(t *testing.T) {
	sim, shaman := healSetup{ppm: 60, dummies: 1}.build(t)
	startFight(sim)
	target := targetDummies(sim)[0]
	shaman.EarthShield.SkipCastAndApplyEffects(sim, target)
	runUntil(sim, 120*time.Second)

	want := 6 + shaman.Talents.ImprovedEarthShield
	if got := charges(shaman, target); got != want {
		t.Errorf("%d charges, want %d", got, want)
	}
	if shaman.EarthShield.Hot(target).IsActive() {
		t.Errorf("Earth Shield is still up after its last charge")
	}
}
