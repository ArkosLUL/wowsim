package hunter

import "github.com/wowsims/wotlk/sim/core"

// Server data conflicts in Hunter spells (core.ServerConflictAllowance); sim/serverdata_test.go checks them.
func init() {
	petAI := func(spellID int32, server int64) core.ServerConflictAllowance {
		return core.ServerConflictAllowance{Spell: core.ActionID{SpellID: spellID}, Field: core.ServerGCD, Sim: 1600, Server: server, KeepSim: true,
			Why: "PetGCD stands in for how long the pet AI waits before using an ability"}
	}
	core.AllowServerConflicts(
		core.ServerConflictAllowance{Spell: core.ActionID{SpellID: 49045}, Field: core.ServerCD, Sim: 5400, Server: 6000,
			Why: "Improved Arcane Shot is +15% damage in 3.3.5; the cooldown cut is TBC's"},

		petAI(25012, 1500), // Lightning Breath
		petAI(35295, 1500), // Gore
		petAI(52472, 1500), // Claw
		petAI(52474, 1500), // Bite
		petAI(52476, 1500), // Smack
		petAI(53533, 1500), // Stampede
		petAI(53548, 0),    // Pin
		petAI(53589, 0),    // Nether Shock
		petAI(53598, 1500), // Sting
		petAI(55485, 1500), // Fire Breath
		petAI(55487, 1500), // Demoralizing Screech
		petAI(55499, 1500), // Monstrous Bite
		petAI(55557, 1500), // Poison Spit
		petAI(55728, 1500), // Scorpid Poison
		petAI(55754, 1500), // Acid Spit
		petAI(56631, 1500), // Tendon Rip
		petAI(58611, 1500), // Lava Breath
		petAI(59886, 1500), // Rake
		core.ServerConflictAllowance{Spell: core.ActionID{SpellID: 53589}, Field: core.ServerCD, Sim: 7000, Server: 40000,
			Why: "Nether Shock has a 40 s cooldown on the server, less Longevity's 10% a point; declared with 10 s"},

		core.ServerConflictAllowance{Spell: core.ActionID{SpellID: 53217}, Field: core.ServerBinary, Sim: 0, Server: 1, KeepSim: true,
			Why: "53217 is the Wild Quiver talent aura, which is binary. Its shot is the triggered 53254, which resists partially"},
	)
}
