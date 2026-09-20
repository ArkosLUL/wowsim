package core

import (
	"math"
	"testing"
)

// Unit::GetRageWeaponSpeedHitFactor truncates, so 0.1 of a second either way can cost a whole point.
func TestRageHitFactor(t *testing.T) {
	cases := []struct {
		swingSpeed float64
		offHand    bool
		want       float64
	}{
		{2.6, false, 9},  // 9.1
		{3.3, false, 11}, // 11.55
		{2.0, false, 7},
		{3.8, false, 13}, // 13.3
		{2.6, true, 4},   // 4.55
		{1.5, true, 2},   // 2.625
	}

	for _, c := range cases {
		hand := "main hand"
		if c.offHand {
			hand = "off hand"
		}
		if got := rageHitFactor(c.swingSpeed, c.offHand); got != c.want {
			t.Errorf("%.1f s %s: got %v, want %v", c.swingSpeed, hand, got, c.want)
		}
	}
}

// A crit doubles the factor the server has already truncated, so a 2.6 s weapon gives 18, not 2·9.1.
func TestRageHitFactorCrit(t *testing.T) {
	if got := rageHitFactor(2.6, false) * 2; got != 18 {
		t.Errorf("2.6 s main hand crit: got %v, want 18", got)
	}
}

func TestRageConversion(t *testing.T) {
	// Unit::RewardRage at level 80; Classic hardcodes 453.3.
	if math.Abs(RageFactor-453.32217) > 1e-4 {
		t.Errorf("level 80 rage conversion: got %v, want 453.32217", RageFactor)
	}
	// below 71 the guessed slope drops out
	if math.Abs(rageConversion(70)-274.69998) > 1e-4 {
		t.Errorf("level 70 rage conversion: got %v, want 274.69998", rageConversion(70))
	}
}
