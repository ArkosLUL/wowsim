package mage

// UndeclaredSpells are the Mage spells, pets' included, with a damage or heal effect on the server that no
// declaration covers (core.SpellEffect). sim/serverdata_test.go keeps the list exact.
var UndeclaredSpells = []int32{
	31707, // Waterbolt (Water Elemental)
	42833, // Fireball
	42845, // Arcane Missiles
	42846, // Arcane Missiles
	42859, // Scorch
	42873, // Fire Blast
	42891, // Pyroblast
	42897, // Arcane Blast
	42914, // Ice Lance
	42921, // Arcane Explosion
	42925, // Flamestrike
	42926, // Flamestrike
	42938, // Blizzard
	42940, // Blizzard
	42945, // Blast Wave
	42950, // Dragon's Breath
	44781, // Arcane Barrage
	47610, // Frostfire Bolt
	55360, // Living Bomb
	55362, // Living Bomb
	59637, // Fire Blast (Mirror Image)
	59638, // Frostbolt (Mirror Image)
}
