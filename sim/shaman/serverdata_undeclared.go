package shaman

// UndeclaredSpells are the Shaman spells, pets' included, with a damage or heal effect on the server that no
// declaration covers (core.SpellEffect). sim/serverdata_test.go keeps the list exact.
var UndeclaredSpells = []int32{
	13339, // Fire Blast (pet): the elemental casts 57984, which its declaration names
}
