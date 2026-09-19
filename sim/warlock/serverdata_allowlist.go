package warlock

import "github.com/wowsims/wotlk/sim/core"

// Server data conflicts in Warlock spells (core.ServerConflictAllowance); sim/serverdata_test.go checks them.
func init() {
	core.AllowServerConflicts(
		core.ServerConflictAllowance{Spell: core.ActionID{SpellID: 47964}, Field: core.ServerGCD, Sim: 1500, Server: 1000,
			Why: "the Imp's Firebolt has a 1 s GCD on the server"},
		core.ServerConflictAllowance{Spell: core.ActionID{SpellID: 23720}, Field: core.ServerCD, Sim: 300000, Server: 0, KeepSim: true,
			Why: "The Black Book's cooldown is item_template's, which Spell.dbc doesn't carry"},
	)
}
