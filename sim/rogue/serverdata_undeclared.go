package rogue

// UndeclaredSpells are the Rogue spells, pets' included, with a damage or heal effect on the server that no
// declaration covers (core.SpellEffect). sim/serverdata_test.go keeps the list exact.
var UndeclaredSpells = []int32{
	14278, // Ghostly Strike
	48638, // Sinister Strike
	48657, // Backstab
	48660, // Hemorrhage
	48664, // Mutilate
	48665, // Mutilate
	48666, // Mutilate
	48668, // Eviscerate
	48672, // Rupture
	48676, // Garrote
	48691, // Ambush
	51723, // Fan of Knives
	57965, // Instant Poison IX
	57970, // Deadly Poison IX
	57975, // Wound Poison VII
	57993, // Envenom
}
