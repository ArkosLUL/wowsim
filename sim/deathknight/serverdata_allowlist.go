package deathknight

import "github.com/wowsims/wotlk/sim/core"

// Server data conflicts in Death Knight spells (core.ServerConflictAllowance); sim/serverdata_test.go checks them.
func init() {
	core.AllowServerConflicts(
		core.ServerConflictAllowance{Spell: core.ActionID{SpellID: 47528}, Field: core.ServerSchool, Sim: 252, Server: 16,
			Why: "Mind Freeze is Frost on the server, declared Magic"},
		core.ServerConflictAllowance{Spell: core.ActionID{SpellID: 47568}, Field: core.ServerMissileSpeed, SimFloat: 0, ServerFloat: 1, KeepSim: true,
			Why: "Empower Rune Weapon is a self cast, and those never travel (Spell::AddUnitTarget)"},
		core.ServerConflictAllowance{Spell: core.ActionID{SpellID: 50536}, Field: core.ServerMissileSpeed, SimFloat: 0, ServerFloat: 15,
			Why: "Unholy Blight travels at 15 yd/s on the server, declared instant"},
		core.ServerConflictAllowance{Spell: core.ActionID{SpellID: 51963}, Field: core.ServerMissileSpeed, SimFloat: 0, ServerFloat: 20,
			Why: "Gargoyle Strike travels at 20 yd/s on the server, declared instant"},
		core.ServerConflictAllowance{Spell: core.ActionID{SpellID: 55271, Tag: 2}, Field: core.ServerSchool, Sim: 128, Server: 2, KeepSim: true,
			Why: "the sim deals Scourge Strike's Shadow part under the strike's id, the server as its own spell 70890"},
	)
}
