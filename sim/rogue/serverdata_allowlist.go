package rogue

import "github.com/wowsims/wotlk/sim/core"

// Server data conflicts in Rogue spells (core.ServerConflictAllowance); sim/serverdata_test.go checks them.
func init() {
	core.AllowServerConflicts(
		core.ServerConflictAllowance{Spell: core.ActionID{SpellID: 51723}, Field: core.ServerMissileSpeed, SimFloat: 0, ServerFloat: 18,
			Why: "Fan of Knives travels at 18 yd/s on the server, declared instant"},
	)
}
