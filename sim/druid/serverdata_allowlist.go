package druid

import "github.com/wowsims/wotlk/sim/core"

// Server data conflicts in Druid spells (core.ServerConflictAllowance); sim/serverdata_test.go checks them.
func init() {
	core.AllowServerConflicts(
		core.ServerConflictAllowance{Spell: core.ActionID{SpellID: 48477}, Field: core.ServerCastTime, Sim: 3500, Server: 2000, KeepSim: true,
			Why: "a Moonkin's Rebirth folds in the 1.5 s GCD of shifting back to Moonkin Form"},
	)
}
