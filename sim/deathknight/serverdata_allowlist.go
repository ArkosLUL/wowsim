package deathknight

import "github.com/wowsims/wotlk/sim/core"

// Server data conflicts in Death Knight spells (core.ServerConflictAllowance); sim/serverdata_test.go checks them.
func init() {
	core.AllowServerConflicts(
		core.ServerConflictAllowance{Spell: core.ActionID{SpellID: 59131}, Field: core.ServerSpellID, Sim: 59131, Server: 49909, KeepSim: true,
			Why: "59131 is an NPC's Icy Touch: binary, with a 6 s category cooldown. The player's rank 5 is 49909"},
		core.ServerConflictAllowance{Spell: core.ActionID{SpellID: 50689}, Field: core.ServerSpellID, Sim: 50689, Server: 48266, KeepSim: true,
			Why: "50689 is an NPC's Blood Presence, on the GCD. The player's is 48266, off the GCD with the presences' 1 s category cooldown"},
		core.ServerConflictAllowance{Spell: core.ActionID{SpellID: 49895}, Field: core.ServerBinary, Sim: 0, Server: 1, KeepSim: true,
			Why: "49895 is the Death Coil cast, a binary dummy. Its damage is the triggered 47632, which isn't binary, so it resists partially"},
	)
}
