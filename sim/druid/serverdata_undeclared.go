package druid

// UndeclaredSpells are the Druid spells, pets' included, with a damage or heal effect on the server that no
// declaration covers (core.SpellEffect). sim/serverdata_test.go keeps the list exact.
var UndeclaredSpells = []int32{
	48461, // Wrath
	48463, // Moonfire
	48465, // Starfire
	48466, // Hurricane
	48467, // Hurricane
	48468, // Insect Swarm
	48480, // Maul
	48562, // Swipe (Bear)
	48564, // Mangle (Bear)
	48566, // Mangle (Cat)
	48568, // Lacerate
	48572, // Shred
	48574, // Rake
	48577, // Ferocious Bite
	49800, // Rip
	53190, // Starfall
	53195, // Starfall
	53227, // Typhoon
	60089, // Faerie Fire (Feral)
	61384, // Typhoon
	62078, // Swipe (Cat)
}
