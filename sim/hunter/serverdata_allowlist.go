package hunter

import "github.com/wowsims/wotlk/sim/core"

// Server data conflicts in Hunter spells (core.ServerConflictAllowance); sim/serverdata_test.go checks them.
func init() {
	explosiveShot := "the tick casts 53352 with the rolled amount (AuraEffect::HandlePeriodicDummyAuraTick), and 53352's " +
		"spell_bonus_data row adds 0.16 ranged AP. serverdata doesn't have 53352"
	core.AllowServerConflicts(
		core.ServerConflictAllowance{Spell: core.ActionID{SpellID: 61006}, Field: core.ServerAP, SimFloat: 0.4, ServerFloat: 0, KeepSim: true,
			Why: "Spell::EffectWeaponDmg's Kill Shot case adds 0.4 ranged AP after the weapon percent"},
		core.ServerConflictAllowance{Spell: core.ActionID{SpellID: 60052}, Field: core.ServerTickAP, SimFloat: 0.16, ServerFloat: 0, KeepSim: true,
			Why: explosiveShot},
		core.ServerConflictAllowance{Spell: core.ActionID{SpellID: 60053}, Field: core.ServerTickAP, SimFloat: 0.16, ServerFloat: 0, KeepSim: true,
			Why: explosiveShot},
	)
}
