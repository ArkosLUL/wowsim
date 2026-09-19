package wotlk

import (
	"math"
	"testing"
	"time"
)

func TestServerProcFor(t *testing.T) {
	cases := []struct {
		name    string
		spellID int32
		want    ServerProc
	}{
		// spell_proc: PPM 1, 2 s ICD, REDUCE_PROC_60
		{"Hand of Justice", 15600, ServerProc{Chance: 0.02 / 3, PPM: 1.0 / 3, ICD: 2 * time.Second}},
		{"Black Magic", blackMagicSpellID, ServerProc{Chance: 0.35, ICD: 35 * time.Second}},
		{"Thundering Skyfire Diamond", 39958, ServerProc{PPM: 0.7, ICD: 40 * time.Second}},
		// no spell_proc row: Spell.dbc's 3% and no ICD
		{"Ashen Band of Courage", 72415, ServerProc{Chance: 0.03}},
	}
	for _, c := range cases {
		got := ServerProcFor(c.spellID)
		if math.Abs(got.Chance-c.want.Chance) > 1e-9 || math.Abs(got.PPM-c.want.PPM) > 1e-9 || got.ICD != c.want.ICD {
			t.Errorf("%s: got %+v, want %+v", c.name, got, c.want)
		}
	}
}

func TestServerEnchantPPM(t *testing.T) {
	for enchantID, want := range map[int32]float64{2673: 1, 3239: 3, 3273: 3, 3789: 1} {
		if got := ServerEnchantPPM(enchantID); got != want {
			t.Errorf("enchant %d: got PPM %v, want %v", enchantID, got, want)
		}
	}
}

func TestServerDuration(t *testing.T) {
	if got := ServerDuration(protectionOfAncientKingsSpellID); got != 8*time.Second {
		t.Errorf("Protection of Ancient Kings: got %v, want 8s", got)
	}
}
