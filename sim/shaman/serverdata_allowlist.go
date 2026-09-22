package shaman

import "github.com/wowsims/wotlk/sim/core"

// Server data conflicts in Shaman spells (core.ServerConflictAllowance); sim/serverdata_test.go checks them.
func init() {
	totem := func(spellID int32) core.ServerConflictAllowance {
		return core.ServerConflictAllowance{Spell: core.ActionID{SpellID: spellID}, Field: core.ServerGCD, Sim: 0, Server: 1000, KeepSim: true,
			Why: "Call of the Elements (66842) drops it on its own GCD, as the server triggers it without one. " +
				"Cast alone it takes 1 s, so the two paths need splitting first"}
	}
	fireElementalAI := "the Greater Fire Elemental's AI picks its spells, which the server scripts"
	core.AllowServerConflicts(
		totem(3738),  // Wrath of Air
		totem(8143),  // Tremor
		totem(8512),  // Windfury
		totem(57722), // Totem of Wrath
		totem(58643), // Strength of Earth
		totem(58656), // Flametongue
		totem(58753), // Stoneskin
		totem(58757), // Healing Stream
		totem(58774), // Mana Spring

		core.ServerConflictAllowance{Spell: core.ActionID{SpellID: 12470}, Field: core.ServerGCD, Sim: 1500, Server: 0, KeepSim: true, Why: fireElementalAI},
		core.ServerConflictAllowance{Spell: core.ActionID{SpellID: 12470}, Field: core.ServerCD, Sim: 1000, Server: 9500, KeepSim: true, Why: fireElementalAI},
		core.ServerConflictAllowance{Spell: core.ActionID{SpellID: 13339}, Field: core.ServerGCD, Sim: 1500, Server: 0, KeepSim: true, Why: fireElementalAI},
		core.ServerConflictAllowance{Spell: core.ActionID{SpellID: 13339}, Field: core.ServerCD, Sim: 1000, Server: 5000, KeepSim: true, Why: fireElementalAI},

		core.ServerConflictAllowance{Spell: core.ActionID{SpellID: 2825}, Field: core.ServerCD, Sim: 600000, Server: 300000, KeepSim: true,
			Why: "Sated (57724) lasts 10 min, so the raid can't take Bloodlust sooner"},
	)
}
