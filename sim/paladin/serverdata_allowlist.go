package paladin

import "github.com/wowsims/wotlk/sim/core"

// Server data conflicts in Paladin spells (core.ServerConflictAllowance); sim/serverdata_test.go checks them.
func init() {
	core.AllowServerConflicts(
		core.ServerConflictAllowance{Spell: core.ActionID{SpellID: 498}, Field: core.ServerSharedCD, Sim: 30000, Server: 0, KeepSim: true,
			Why: "stands in for Forbearance (25771) and Avenging Wrath Marker (61987), the 30 s debuffs that lock each other out"},
		core.ServerConflictAllowance{Spell: core.ActionID{SpellID: 31884}, Field: core.ServerSharedCD, Sim: 30000, Server: 0, KeepSim: true,
			Why: "stands in for Forbearance (25771) and Avenging Wrath Marker (61987), the 30 s debuffs that lock each other out"},
	)
}
