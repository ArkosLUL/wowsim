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
		core.ServerConflictAllowance{Spell: core.ActionID{SpellID: 2894}, Field: core.ServerGCD, Sim: 1500, Server: 1000,
			Why: "totems take a 1 s GCD on the server"},

		core.ServerConflictAllowance{Spell: core.ActionID{SpellID: 12470}, Field: core.ServerGCD, Sim: 1500, Server: 0, KeepSim: true, Why: fireElementalAI},
		core.ServerConflictAllowance{Spell: core.ActionID{SpellID: 12470}, Field: core.ServerCD, Sim: 1000, Server: 9500, KeepSim: true, Why: fireElementalAI},
		core.ServerConflictAllowance{Spell: core.ActionID{SpellID: 13339}, Field: core.ServerGCD, Sim: 1500, Server: 0, KeepSim: true, Why: fireElementalAI},
		core.ServerConflictAllowance{Spell: core.ActionID{SpellID: 13339}, Field: core.ServerCD, Sim: 1000, Server: 5000, KeepSim: true, Why: fireElementalAI},

		core.ServerConflictAllowance{Spell: core.ActionID{SpellID: 2825}, Field: core.ServerCD, Sim: 600000, Server: 300000, KeepSim: true,
			Why: "Sated (57724) lasts 10 min, so the raid can't take Bloodlust sooner"},
		core.ServerConflictAllowance{Spell: core.ActionID{SpellID: 16188}, Field: core.ServerCD, Sim: 180000, Server: 120000,
			Why: "Nature's Swiftness has a 2 min cooldown on the server"},
		core.ServerConflictAllowance{Spell: core.ActionID{SpellID: 49273}, Field: core.ServerCastTime, Sim: 1500, Server: 3000,
			Why: "placeholder cast time; Healing Wave is 3 s before Improved Healing Wave"},
		core.ServerConflictAllowance{Spell: core.ActionID{SpellID: 55459}, Field: core.ServerCastTime, Sim: 1500, Server: 2500,
			Why: "placeholder cast time; Chain Heal is 2.5 s"},
		core.ServerConflictAllowance{Spell: core.ActionID{SpellID: 61301}, Field: core.ServerCastTime, Sim: 1500, Server: 0,
			Why: "placeholder cast time; Riptide is instant"},

		// the sim deals these hits under a binary cast's id, the server with a triggered spell that isn't binary
		core.ServerConflictAllowance{Spell: core.ActionID{SpellID: 58704}, Field: core.ServerBinary, Sim: 0, Server: 1, KeepSim: true,
			Why: "58704 is the Searing Totem summon. Its bolts are 58702, which resist partially"},
		core.ServerConflictAllowance{Spell: core.ActionID{SpellID: 58734}, Field: core.ServerBinary, Sim: 0, Server: 1, KeepSim: true,
			Why: "58734 is the Magma Totem summon. Its pulses are 58735, which resist partially"},
		core.ServerConflictAllowance{Spell: core.ActionID{SpellID: 58789}, Field: core.ServerBinary, Sim: 0, Server: 1, KeepSim: true,
			Why: "58789 is the Flametongue Weapon imbue. Its hits are Flametongue Attack (10444), which resists partially"},
		core.ServerConflictAllowance{Spell: core.ActionID{SpellID: 58790}, Field: core.ServerBinary, Sim: 0, Server: 1, KeepSim: true,
			Why: "58790 is the Flametongue Weapon imbue. Its hits are Flametongue Attack (10444), which resists partially"},
		core.ServerConflictAllowance{Spell: core.ActionID{SpellID: 61657}, Field: core.ServerBinary, Sim: 0, Server: 1, KeepSim: true,
			Why: "61657 is the Fire Nova cast, a dummy. The totem's nova is the triggered 61654, which resists partially"},
	)
}
