package mage

import "github.com/wowsims/wotlk/sim/core"

// Server data conflicts in Mage spells (core.ServerConflictAllowance); sim/serverdata_test.go checks them.
func init() {
	core.AllowServerConflicts(
		core.ServerConflictAllowance{Spell: core.ActionID{SpellID: 59637}, Field: core.ServerGCD, Sim: 1000, Server: 0, KeepSim: true,
			Why: "the GCD paces the Mirror Images' AI, which the server scripts"},
		core.ServerConflictAllowance{Spell: core.ActionID{SpellID: 59638}, Field: core.ServerGCD, Sim: 1500, Server: 0, KeepSim: true,
			Why: "the GCD paces the Mirror Images' AI, which the server scripts"},
	)
}
