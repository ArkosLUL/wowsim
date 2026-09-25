package deathknight

// UndeclaredSpells are the Death Knight spells, pets' included, with a damage or heal effect on the server that no
// declaration covers (core.SpellEffect). sim/serverdata_test.go keeps the list exact.
var UndeclaredSpells = []int32{
	47468, // Claw (Ghoul)
	47632, // Death Coil
	48743, // Death Pact
	49909, // Icy Touch
	49921, // Plague Strike
	49924, // Death Strike
	49930, // Blood Strike
	49941, // Blood Boil
	50401, // Razor Frost
	50463, // Blood-Caked Strike
	50536, // Unholy Blight
	51411, // Howling Blast
	51963, // Gargoyle Strike (Gargoyle)
	55078, // Blood Plague
	55095, // Frost Fever
	55262, // Heart Strike
	55268, // Frost Strike
	55271, // Scourge Strike
	56815, // Rune Strike
}
