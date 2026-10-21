package hunter

import (
	"math"
	"testing"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
)

// mod-spell-tweaks' Marked for Death ignores part of the target's armor on Auto Shot, Multi-Shot, Aimed Shot,
// Silencing Shot, Steady Shot and Kill Shot, the physical shots in its class mask.
func TestMarkedForDeathIgnoresArmor(t *testing.T) {
	mm := hunterWithTalents(t, MMTalents, proto.Hunter_Options_Wolf)
	if mm.Talents.MarkedForDeath != 5 {
		t.Fatalf("MMTalents has %d points in Marked for Death, want 5", mm.Talents.MarkedForDeath)
	}
	check := func(who string, spell *core.Spell, want float64) {
		t.Helper()
		if spell == nil {
			t.Fatalf("%s: a spell isn't registered", who)
		}
		if math.Abs(spell.IgnoreArmorPct-want) > 1e-9 {
			t.Errorf("%s: %v ignores %v of the armor, want %v", who, spell.ActionID, spell.IgnoreArmorPct, want)
		}
	}
	for _, spell := range []*core.Spell{mm.AutoAttacks.RangedAuto(), mm.MultiShot, mm.AimedShot, mm.SilencingShot,
		mm.SteadyShot, mm.KillShot} {
		check("MM", spell, -5*markedForDeathIgnoreArmorPerRank)
	}
	for _, spell := range []*core.Spell{mm.ChimeraShot, mm.ArcaneShot, mm.SerpentSting, mm.Volley} {
		check("MM", spell, 0)
	}
	for _, spell := range mm.pet.Spellbook {
		check("MM's pet", spell, 0)
	}

	sv := hunterWithTalents(t, SVTalents, proto.Hunter_Options_Wolf)
	check("SV", sv.SteadyShot, 0)
	check("SV", sv.AutoAttacks.RangedAuto(), 0)
}
