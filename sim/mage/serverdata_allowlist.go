package mage

import "github.com/wowsims/wotlk/sim/core"

// Server data conflicts in Mage spells (core.ServerConflictAllowance); sim/serverdata_test.go checks them.
func init() {
	core.AllowServerConflicts(
		core.ServerConflictAllowance{Spell: core.ActionID{SpellID: 59637}, Field: core.ServerCD, Sim: 0, Server: 6000, KeepSim: true,
			Why: "a creature only starts a cooldown from a pet bar or PetAI cast (Creature::AddSpellCooldown), and " +
				"npc_pet_mage_mirror_image casts off its own event timers"},
	)
}
