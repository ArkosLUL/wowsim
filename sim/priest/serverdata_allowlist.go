package priest

import "github.com/wowsims/wotlk/sim/core"

// Server data conflicts in Priest spells (core.ServerConflictAllowance); sim/serverdata_test.go checks them.
func init() {
	core.AllowServerConflicts(
		core.ServerConflictAllowance{Spell: core.ActionID{SpellID: 63619}, Field: core.ServerGCD, Sim: 6000, Server: 1500, KeepSim: true,
			Why: "the Shadowfiend's AI casts Shadowcrawl every GCD, so a 6 s GCD gives the server's 6 s cooldown"},
		core.ServerConflictAllowance{Spell: core.ActionID{SpellID: 63619}, Field: core.ServerCD, Sim: 0, Server: 6000, KeepSim: true,
			Why: "the Shadowfiend's AI casts Shadowcrawl every GCD, so a 6 s GCD gives the server's 6 s cooldown"},
	)
}
