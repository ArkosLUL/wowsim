package priest

import "github.com/wowsims/wotlk/sim/core"

// Server data conflicts in Priest spells (core.ServerConflictAllowance); sim/serverdata_test.go checks them.
func init() {
	core.AllowServerConflicts(
		core.ServerConflictAllowance{Spell: core.ActionID{SpellID: 63619}, Field: core.ServerGCD, Sim: 6000, Server: 1500, KeepSim: true,
			Why: "the Shadowfiend's AI casts Shadowcrawl every GCD, so a 6 s GCD gives the server's 6 s cooldown"},
		core.ServerConflictAllowance{Spell: core.ActionID{SpellID: 63619}, Field: core.ServerCD, Sim: 0, Server: 6000, KeepSim: true,
			Why: "the Shadowfiend's AI casts Shadowcrawl every GCD, so a 6 s GCD gives the server's 6 s cooldown"},
		core.ServerConflictAllowance{Spell: core.ActionID{SpellID: 63619}, Field: core.ServerSchool, Sim: 252, Server: 4,
			Why: "Shadowcrawl is Arcane on the server, declared Magic. It deals nothing"},

		core.ServerConflictAllowance{Spell: core.ActionID{SpellID: 47515}, Field: core.ServerSchool, Sim: 32, Server: 2, KeepSim: true,
			Why: "the sim casts Divine Aegis's shield under the talent's id, the server as its own spell 47753"},
		core.ServerConflictAllowance{Spell: core.ActionID{SpellID: 63543}, Field: core.ServerSchool, Sim: 32, Server: 2, KeepSim: true,
			Why: "the sim heals Empowered Renew under the talent's id, the server as its own spell 63544"},
		core.ServerConflictAllowance{Spell: core.ActionID{SpellID: 70770}, Field: core.ServerSchool, Sim: 32, Server: 2, KeepSim: true,
			Why: "the sim heals the T10 2pc under the set bonus's id, the server as its own spell 70772"},
	)
}
