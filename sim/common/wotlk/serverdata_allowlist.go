package wotlk

import "github.com/wowsims/wotlk/sim/core"

// Server data conflicts of this package's item spells (core.ServerConflictAllowance); sim/serverdata_test.go
// checks them.
func init() {
	itemCooldown := "the item's cooldown is item_template's, which Spell.dbc doesn't carry"
	glg := core.ActionID{SpellID: GnomishLightningGeneratorSpellID}
	core.AllowServerConflicts(
		core.ServerConflictAllowance{Spell: glg, Field: core.ServerCD, Sim: 60000, Server: 0, KeepSim: true, Why: itemCooldown},
		core.ServerConflictAllowance{Spell: glg, Field: core.ServerSharedCD, Sim: 10000, Server: 0, KeepSim: true, Why: itemCooldown},
	)
}
