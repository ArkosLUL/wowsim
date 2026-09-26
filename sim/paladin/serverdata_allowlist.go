package paladin

import "github.com/wowsims/wotlk/sim/core"

// Server data conflicts in Paladin spells (core.ServerConflictAllowance); sim/serverdata_test.go checks them.
func init() {
	core.AllowServerConflicts(
		core.ServerConflictAllowance{Spell: core.ActionID{SpellID: 498}, Field: core.ServerSharedCD, Sim: 30000, Server: 0, KeepSim: true,
			Why: "stands in for Forbearance (25771) and Avenging Wrath Marker (61987), the 30 s debuffs that lock each other out"},
		core.ServerConflictAllowance{Spell: core.ActionID{SpellID: 31884}, Field: core.ServerSharedCD, Sim: 30000, Server: 0, KeepSim: true,
			Why: "stands in for Forbearance (25771) and Avenging Wrath Marker (61987), the 30 s debuffs that lock each other out"},
		core.ServerConflictAllowance{Spell: core.ActionID{SpellID: 20467}, Field: core.ServerSP, SimFloat: 0.13, ServerFloat: 0, KeepSim: true,
			Why: "Spell::EffectWeaponDmg adds 13% of holy spell power to Judgement of Command, after the weapon percent"},
		core.ServerConflictAllowance{Spell: core.ActionID{SpellID: 20467}, Field: core.ServerAP, SimFloat: 0.08, ServerFloat: 0, KeepSim: true,
			Why: "Spell::EffectWeaponDmg adds 8% of attack power to Judgement of Command, after the weapon percent"},
	)
}
