package deathknight

import (
	"testing"

	"github.com/wowsims/wotlk/sim/core/serverdata"
)

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
