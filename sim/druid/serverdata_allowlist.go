package druid

import "github.com/wowsims/wotlk/sim/core"

// Server data conflicts in Druid spells (core.ServerConflictAllowance); sim/serverdata_test.go checks them.
func init() {
	core.AllowServerConflicts(
		core.ServerConflictAllowance{Spell: core.ActionID{SpellID: 48477}, Field: core.ServerCastTime, Sim: 3500, Server: 2000, KeepSim: true,
			Why: "a Moonkin's Rebirth folds in the 1.5 s GCD of shifting back to Moonkin Form"},
		core.ServerConflictAllowance{Spell: core.ActionID{SpellID: 61384}, Field: core.ServerMissileSpeed, SimFloat: 0, ServerFloat: 27,
			Why: "Typhoon's cast travels at 27 yd/s on the server, declared instant. The damage waits only on 53227's own 30 yd/s"},
	)
}
