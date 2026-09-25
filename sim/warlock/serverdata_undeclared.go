package warlock

// UndeclaredSpells are the Warlock spells, pets' included, with a damage or heal effect on the server that no
// declaration covers (core.SpellEffect). sim/serverdata_test.go keeps the list exact.
var UndeclaredSpells = []int32{
	17962, // Conflagrate
	20153, // Immolation (Infernal)
	47809, // Shadow Bolt
	47811, // Immolate
	47815, // Searing Pain
	47825, // Soul Fire
	47827, // Shadowburn
	47834, // Seed of Corruption
	47836, // Seed of Corruption
	47838, // Incinerate
	47843, // Unstable Affliction
	47855, // Drain Soul
	47864, // Curse of Agony
	47867, // Curse of Doom
	47964, // Firebolt (Imp)
	47992, // Lash of Pain (Succubus)
	47994, // Cleave (Felguard)
	50589, // Immolation Aura (Demon)
	54053, // Shadow Bite (Felhunter)
	59164, // Haunt
	59172, // Chaos Bolt
}
