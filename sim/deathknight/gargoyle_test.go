package deathknight

import (
	"math"
	"testing"
	"time"

	"github.com/wowsims/wotlk/sim/core"
)

// 49206 summons the gargoyle 3 yd off the DK's front left, and AttackStartCaster's 40 yd chase leaves it there:
// from a DK 30 yd out it stands 27.96 yd from the target, so Gargoyle Strike flies 26.46 yd past its model's 1.5,
// 1322 ms at 20 yd a second.
func TestGargoyleCastsFromWhereItsSummoned(t *testing.T) {
	if got, want := gargoyleSpotDistance(30), math.Sqrt(math.Pow(30-3/math.Sqrt2, 2)+4.5); math.Abs(got-want) > 1e-9 {
		t.Errorf("the gargoyle stands %v yd from the target, want %v", got, want)
	}
	strike := &core.Spell{Unit: &core.Unit{DistanceFromTarget: gargoyleSpotDistance(30)}, MissileSpeed: 20}
	if got := strike.TravelTime(); got != 1322*time.Millisecond {
		t.Errorf("Gargoyle Strike flies %v, want 1322 ms", got)
	}
}
