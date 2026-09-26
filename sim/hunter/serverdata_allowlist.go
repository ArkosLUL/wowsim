package hunter

import "github.com/wowsims/wotlk/sim/core"

// Server data conflicts in Hunter spells (core.ServerConflictAllowance); sim/serverdata_test.go checks them.
func init() {
	core.AllowServerConflicts(
		core.ServerConflictAllowance{Spell: core.ActionID{SpellID: 61006}, Field: core.ServerAP, SimFloat: 0.4, ServerFloat: 0, KeepSim: true,
			Why: "Spell::EffectWeaponDmg's Kill Shot case adds 0.4 ranged AP after the weapon percent"},
	)
}
