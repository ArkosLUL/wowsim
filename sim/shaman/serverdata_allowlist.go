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

		core.ServerConflictAllowance{Spell: core.ActionID{SpellID: 58804}, Field: core.ServerSchool, Sim: 2, Server: 64, KeepSim: true,
			Why: "the sim deals the Windfury attacks under the enchant's id, the server as its own spell 25504, Physical"},
		core.ServerConflictAllowance{Spell: core.ActionID{SpellID: 58704}, Field: core.ServerSchool, Sim: 8, Server: 2,
			Why: "the Searing Totem summon is Physical on the server, declared Fire. Only its bolt (58702) deals damage"},
		core.ServerConflictAllowance{Spell: core.ActionID{SpellID: 58734}, Field: core.ServerSchool, Sim: 8, Server: 2,
			Why: "the Magma Totem summon is Physical on the server, declared Fire. Only its pulse (58735) deals damage"},
		core.ServerConflictAllowance{Spell: core.ActionID{SpellID: 49238}, Field: core.ServerMissileSpeed, SimFloat: 0, ServerFloat: 20,
			Why: "Lightning Bolt travels at 20 yd/s on the server, declared instant"},
		core.ServerConflictAllowance{Spell: core.ActionID{SpellID: 60043}, Field: core.ServerMissileSpeed, SimFloat: 0, ServerFloat: 24,
			Why: "Lava Burst travels at 24 yd/s on the server, declared instant"},
	)
}
