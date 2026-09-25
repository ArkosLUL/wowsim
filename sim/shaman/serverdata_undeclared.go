package shaman

// UndeclaredSpells are the Shaman spells, pets' included, with a damage or heal effect on the server that no
// declaration covers (core.SpellEffect). sim/serverdata_test.go keeps the list exact.
var UndeclaredSpells = []int32{
	10444, // Flametongue Attack
	11350, // Fire Shield (Greater Fire Elemental)
	12470, // Fire Nova (Greater Fire Elemental)
	13339, // Fire Blast (Greater Fire Elemental)
	17364, // Stormstrike
	32175, // Stormstrike
	32176, // Stormstrike
	49231, // Earth Shock
	49233, // Flame Shock
	49236, // Frost Shock
	49238, // Lightning Bolt
	49271, // Chain Lightning
	49273, // Healing Wave
	49276, // Lesser Healing Wave
	49279, // Lightning Shield
	55459, // Chain Heal
	58702, // Attack
	58735, // Magma Totem
	58799, // Frostbrand Attack
	59159, // Thunderstorm
	60043, // Lava Burst
	60103, // Lava Lash
	61301, // Riptide
	61654, // Fire Nova
}
