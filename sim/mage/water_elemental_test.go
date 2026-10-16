package mage

import (
	"math"
	"testing"
	"time"

	"github.com/wowsims/wotlk/sim/core/proto"
)

// PetAI leaves the Water Elemental where it follows the mage while the target's in Waterbolt's range: its reach
// plus PET_FOLLOW_DIST's 2 yd off his left side. Glyph of Eternal Water's is a pet, with a pet's 1.5; the timed
// one is a guardian, with model 525's 0.85. Waterbolt flies the distance less that reach, at 16 yd a second.
func TestWaterElementalCastsFromWhereItFollows(t *testing.T) {
	for _, tc := range []struct {
		name      string
		permanent bool
		reach     float64
		flight    time.Duration
	}{
		{"Glyph of Eternal Water", true, 1.5, 1793 * time.Millisecond},
		{"timed", false, 0.85, 1830 * time.Millisecond},
	} {
		player := procMage(FrostTalents, "p3_frost_alliance", `{"type": "TypeAPL", "priorityList": []}`, PlayerOptionsFrost)
		player.Glyphs = &proto.Glyphs{}
		if tc.permanent {
			player.Glyphs.Major1 = int32(proto.MageMajorGlyph_GlyphOfEternalWater)
		}
		_, mage := newProcSim(t, player)
		elemental := mage.waterElemental
		if got, want := elemental.DistanceFromTarget, math.Hypot(30, tc.reach+2); math.Abs(got-want) > 1e-9 {
			t.Errorf("%s: it stands %v yd from the target, want %v", tc.name, got, want)
		}
		if got := elemental.Waterbolt.TravelTime(); got != tc.flight {
			t.Errorf("%s: Waterbolt flies %v, want %v", tc.name, got, tc.flight)
		}
	}
}
