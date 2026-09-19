package warrior

import "github.com/wowsims/wotlk/sim/core"

// Server data conflicts in Warrior spells (core.ServerConflictAllowance); sim/serverdata_test.go checks them.
func init() {
	core.AllowServerConflicts(
		core.ServerConflictAllowance{Spell: core.ActionID{SpellID: 1719}, Field: core.ServerGCD, Sim: 0, Server: 1500,
			Why: "Recklessness is on the GCD in 3.3.5; declared off it"},
		core.ServerConflictAllowance{Spell: core.ActionID{SpellID: 12292}, Field: core.ServerGCD, Sim: 0, Server: 1500,
			Why: "Death Wish is on the GCD in 3.3.5; declared off it"},
		core.ServerConflictAllowance{Spell: core.ActionID{SpellID: 64382}, Field: core.ServerCastTime, Sim: 0, Server: 1500,
			Why: "Shattering Throw has a 1.5 s cast; declared instant"},
		core.ServerConflictAllowance{Spell: core.ActionID{SpellID: 12723}, Field: core.ServerCD, Sim: 30000, Server: 0, KeepSim: true,
			Why: "12723 is the triggered Sweeping Strikes hit; the ability with the 30 s cooldown is 12328"},
		core.ServerConflictAllowance{Spell: core.ActionID{SpellID: 46924}, Field: core.ServerChanneled, Sim: 1, Server: 0, KeepSim: true,
			Why: "Bladestorm is a periodic aura on the server, modeled as a channel so nothing else is cast during it"},
		core.ServerConflictAllowance{Spell: core.ActionID{SpellID: 47475}, Field: core.ServerNoActiveDefense, Sim: 0, Server: 1, KeepSim: true,
			Why: "47475 is the Slam cast, a dummy that can only miss. Its hit is the triggered 50783, which can be dodged and parried"},
	)
}
