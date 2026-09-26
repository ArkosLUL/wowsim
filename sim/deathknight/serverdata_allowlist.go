package deathknight

import "github.com/wowsims/wotlk/sim/core"

// Server data conflicts in Death Knight spells (core.ServerConflictAllowance); sim/serverdata_test.go checks them.
func init() {
	core.AllowServerConflicts(
		core.ServerConflictAllowance{Spell: core.ActionID{SpellID: 47568}, Field: core.ServerMissileSpeed, SimFloat: 0, ServerFloat: 1, KeepSim: true,
			Why: "Empower Rune Weapon is a self cast, and those never travel (Spell::AddUnitTarget)"},
		core.ServerConflictAllowance{Spell: core.ActionID{SpellID: 55271, Tag: 2}, Field: core.ServerSchool, Sim: 128, Server: 2, KeepSim: true,
			Why: "the sim deals Scourge Strike's Shadow part under the strike's id, the server as its own spell 70890"},
		core.ServerConflictAllowance{Spell: core.ActionID{SpellID: 47632}, Field: core.ServerAP, SimFloat: 0.15, ServerFloat: 0, KeepSim: true,
			Why: "spell_dk_death_coil hands 49895's value to 47632 as base points, and 47632's own spell_bonus_data row adds 0.15 AP"},
		core.ServerConflictAllowance{Spell: core.ActionID{SpellID: 49938}, Field: core.ServerTickAP, SimFloat: 0.04805, ServerFloat: 0, KeepSim: true,
			Why: "spell_dk_death_and_decay_aura casts each tick as 52212, whose spell_bonus_data row adds 0.04805 AP"},
		core.ServerConflictAllowance{Spell: core.ActionID{SpellID: 56815}, Field: core.ServerAP, SimFloat: 0.15, ServerFloat: 0, KeepSim: true,
			Why: "Spell::EffectWeaponDmg adds 15% of attack power to Rune Strike, after the weapon percent"},
	)
}
