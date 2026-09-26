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
	flametongueScript := "spell_sha_flametongue_weapon casts 10444 for the passive rank's effect value / 100 (58792's 6850, " +
		"58791's 6000) per second of weapon speed, plus 0.03811 spell power per second"
	flametongue := func(field core.ServerField, sim, server float64) core.ServerConflictAllowance {
		return core.ServerConflictAllowance{Spell: core.ActionID{SpellID: 10444}, Field: field, SimFloat: sim, ServerFloat: server,
			KeepSim: true, Why: flametongueScript}
	}
	flametongueDownranked := func(field core.ServerField, sim, server float64) core.ServerConflictAllowance {
		a := flametongue(field, sim, server)
		a.Spell.Tag = 1
		return a
	}
	healingStream := "spell_sha_healing_stream_totem heals for the value of the rank's trigger (58761: 25), " +
		"which 52042's own effect leaves at 0"
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

		core.ServerConflictAllowance{Spell: core.ActionID{SpellID: 13339}, Field: core.ServerSP, SimFloat: 0.2, ServerFloat: 0, KeepSim: true,
			Why: "npc_pet_shaman_fire_elemental casts Fire Blast 57984, whose spell_bonus_data row is 0.2. It has 13339's " +
				"roll, but serverdata doesn't have it"},

		flametongue(core.ServerSP, 0.03811, 0),
		flametongue(core.ServerMin, 68.5, 1),
		flametongue(core.ServerMax, 68.5, 1),
		flametongueDownranked(core.ServerMin, 60, 1),
		flametongueDownranked(core.ServerMax, 60, 1),

		core.ServerConflictAllowance{Spell: core.ActionID{SpellID: 52042}, Field: core.ServerMin, SimFloat: 25, ServerFloat: 0, KeepSim: true,
			Why: healingStream},
		core.ServerConflictAllowance{Spell: core.ActionID{SpellID: 52042}, Field: core.ServerMax, SimFloat: 25, ServerFloat: 0, KeepSim: true,
			Why: healingStream},
	)
}
