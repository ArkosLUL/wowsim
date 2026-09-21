package deathknight

import (
	"testing"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/serverdata"
)

// The optimizer asks HasEnchantEffect/HasItemEffect before it builds any DK, and parallel sims
// building the first DKs would race the writes, so these register at init. Builds no DK on purpose.
func TestItemEffectsRegisteredWithoutBuildingADeathknight(t *testing.T) {
	for _, id := range []int32{3370, 3368, 3883, 3847, 3594, 3365, 3595, 3367, 3369} {
		if !core.HasEnchantEffect(id) {
			t.Errorf("enchant effect %d not registered", id)
		}
	}
	for _, id := range []int32{40714, 40715, 45144, 47672, 47673, 50459, 50462, 42618, 42619, 42620, 42621, 42622, 51417} {
		if !core.HasItemEffect(id) {
			t.Errorf("item effect %d not registered", id)
		}
	}
}

// spell_dk_pet_scaling's CalculateHasteAmount works in float32 and keeps whole percent, and a slowed
// owner hands over nothing.
func TestDKPetHaste(t *testing.T) {
	for _, tc := range []struct {
		owner, want float64
	}{
		{1, 1},
		{0.8, 1},
		{1.2, 1.2},
		{1.5, 1.5},
		{1.517, 1.51},
		{3.2, 3.2},
	} {
		if got := dkPetHaste(tc.owner); got != tc.want {
			t.Errorf("owner swing speed %v: got %v, want %v", tc.owner, got, tc.want)
		}
	}
}

// CalculatePct truncates the int32 stat and the result.
func TestCalculatePct(t *testing.T) {
	for _, tc := range []struct {
		stat float64
		pct  int64
		want float64
	}{
		{2003.9, 112, 2243}, // 2003 * 1.12 = 2243.36
		{1500, 152, 2280},
		{-50, 70, 0},
	} {
		if got := calculatePct(tc.stat, tc.pct); got != tc.want {
			t.Errorf("%v at %d%%: got %v, want %v", tc.stat, tc.pct, got, tc.want)
		}
	}
}

// Frost Vulnerability's class mask covers the DK's frost spells, Frost Fever included, and not Razor
// Frost or his shadow spells.
func TestFrostVulnerabilityCovers(t *testing.T) {
	for id, want := range map[int32]bool{
		49909: true,  // Icy Touch
		51411: true,  // Howling Blast
		55268: true,  // Frost Strike
		55095: true,  // Frost Fever
		50401: false, // Razor Frost
		47632: false, // Death Coil
		55078: false, // Blood Plague
	} {
		sd := serverdata.SpellByID(id)
		if sd == nil {
			t.Fatalf("no server data for %d", id)
		}
		if got := frostVulnerabilityCovers(sd); got != want {
			t.Errorf("%d: covered %v, want %v", id, got, want)
		}
	}
}
