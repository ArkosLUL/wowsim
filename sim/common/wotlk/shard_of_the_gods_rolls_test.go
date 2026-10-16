package wotlk

import (
	"testing"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/serverdata"
)

// Spell::DoAllEffectOnTarget raises the DMG_CLASS_MAGIC flags the sets proc on for a magic-class landing only: a
// spell with no damage class raises DMG_CLASS_NONE. A sim spell standing in for the magic one that really lands
// goes by that one, and one with no server data under its action id by its proc mask.
func TestShardOfTheGodsRollsOnMagicLandingsOnly(t *testing.T) {
	_, owner, _ := newValkyrTestSim(t, nil)
	for _, c := range []struct {
		name     string
		id       core.ActionID
		procMask core.ProcMask
		want     bool
	}{
		{"Renew, magic", core.ActionID{SpellID: 48068}, core.ProcMaskSpellHealing, true},
		{"Heroic Strike, melee", core.ActionID{SpellID: 47450}, core.ProcMaskMeleeMHSpecial, false},
		{"Invigorating Earthsiege Health Regen, no class", core.ActionID{SpellID: 55341}, core.ProcMaskSpellHealing, false},
		{"Ardent Defender's heal, no class", core.ActionID{SpellID: 66235}, core.ProcMaskSpellHealing, false},
		{"Persistent Shield, no class", core.ActionID{SpellID: 26470}, core.ProcMaskSpellHealing, false},
		{"a glyph's heal under the glyph's item", core.ActionID{ItemID: 42408}, core.ProcMaskSpellHealing, true},
		{"an item's action with no spell mask", core.ActionID{ItemID: 42408, Tag: 1}, core.ProcMaskEmpty, false},
	} {
		spell := owner.RegisterSpell(core.SpellConfig{ActionID: c.id, ProcMask: c.procMask})
		if got := rollsShardOfTheGods(spell); got != c.want {
			t.Errorf("%s: rolls %v, want %v", c.name, got, c.want)
		}
	}

	// no proc mask, so only the landing spell's server data can make these roll
	for id, landsAs := range shardLandingSpellIDs {
		if serverdata.SpellByID(landsAs) == nil {
			t.Errorf("%d lands as %d, which has no server data: rerun tools/acore/gen_serverdata", id, landsAs)
			continue
		}
		spell := owner.RegisterSpell(core.SpellConfig{ActionID: core.ActionID{SpellID: id}, ProcMask: core.ProcMaskEmpty})
		if !rollsShardOfTheGods(spell) {
			t.Errorf("%d, landing as %d: doesn't roll, want it to", id, landsAs)
		}
	}
}
