package core

// Server data conflicts in spells no class owns: raid buffs, external cooldowns and items
// (ServerConflictAllowance); sim/serverdata_test.go checks them.
func init() {
	// A tag -1 spell is a raid member's cast on this character: it costs the character no GCD, and its
	// cooldown is how often that raid member hands it out.
	external := func(spellID int32, field ServerField, sim, server int64) ServerConflictAllowance {
		return ServerConflictAllowance{Spell: ActionID{SpellID: spellID, Tag: -1}, Field: field, Sim: sim, Server: server, KeepSim: true,
			Why: "external cooldown, cast by a raid member on its own schedule"}
	}
	itemCooldown := "the item's cooldown is item_template's, which Spell.dbc doesn't carry"
	AllowServerConflicts(
		external(2825, ServerGCD, 0, 1500),       // Bloodlust
		external(2825, ServerCD, 600000, 300000), // Bloodlust, spaced by Sated
		external(6940, ServerGCD, 0, 1500),       // Hand of Sacrifice
		external(6940, ServerCD, 10500, 120000),  // Hand of Sacrifice
		external(10060, ServerCD, 15000, 120000), // Power Infusion
		external(16190, ServerGCD, 0, 1000),      // Mana Tide Totem
		external(16190, ServerCD, 12000, 300000), // Mana Tide Totem
		external(29166, ServerGCD, 0, 1500),      // Innervate
		external(29166, ServerCD, 10000, 180000), // Innervate
		external(33206, ServerGCD, 0, 1500),      // Pain Suppression
		external(33206, ServerCD, 8000, 180000),  // Pain Suppression
		external(47788, ServerCD, 10000, 180000), // Guardian Spirit
		external(49016, ServerCD, 30000, 180000), // Hysteria
		external(53530, ServerCD, 6000, 0),       // Divine Guardian
		external(57933, ServerCD, 10000, 0),      // Tricks of the Trade
		external(64382, ServerCastTime, 0, 1500), // Shattering Throw
		external(64382, ServerGCD, 0, 1500),      // Shattering Throw
		external(64382, ServerCD, 10000, 300000), // Shattering Throw

		ServerConflictAllowance{Spell: ActionID{SpellID: 53307}, Field: ServerBinary, Sim: 1, Server: 0,
			Why: "Thorns isn't binary on the server (no SPELL_ATTR0_CU_BINARY_SPELL), so it resists partially"},

		ServerConflictAllowance{Spell: ActionID{SpellID: 56186}, Field: ServerCD, Sim: 300000, Server: 0, KeepSim: true, Why: itemCooldown},    // Sapphire Owl
		ServerConflictAllowance{Spell: ActionID{SpellID: 71586}, Field: ServerCD, Sim: 120000, Server: 1000, KeepSim: true, Why: itemCooldown}, // Hardened Skin
	)
}
