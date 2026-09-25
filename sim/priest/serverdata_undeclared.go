package priest

// UndeclaredSpells are the Priest spells, pets' included, with a damage or heal effect on the server that no
// declaration covers (core.SpellEffect). sim/serverdata_test.go keeps the list exact.
var UndeclaredSpells = []int32{
	48063, // Greater Heal
	48068, // Renew
	48071, // Flash Heal
	48072, // Prayer of Healing
	48089, // Circle of Healing
	48120, // Binding Heal
	48123, // Smite
	48125, // Shadow Word: Pain
	48127, // Mind Blast
	48135, // Holy Fire
	48158, // Shadow Word: Death
	48160, // Vampiric Touch
	48300, // Devouring Plague
	53022, // Mind Sear
	53023, // Mind Sear
	58381, // Mind Flay
	63675, // Improved Devouring Plague
}
