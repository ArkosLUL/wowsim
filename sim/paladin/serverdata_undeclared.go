package paladin

// UndeclaredSpells are the Paladin spells, pets' included, with a damage or heal effect on the server that no
// declaration covers (core.SpellEffect). sim/serverdata_test.go keeps the list exact.
// 64891: Holy Mending's amount is handed in as base points.
var UndeclaredSpells = []int32{64891}
