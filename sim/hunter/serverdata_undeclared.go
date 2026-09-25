package hunter

// UndeclaredSpells are the Hunter spells, pets' included, with a damage or heal effect on the server that no
// declaration covers (core.SpellEffect). sim/serverdata_test.go keeps the list exact.
var UndeclaredSpells = []int32{
	34490, // Silencing Shot
	48996, // Raptor Strike
	49001, // Serpent Sting
	49048, // Multi-Shot
	49050, // Aimed Shot
	49052, // Steady Shot
	52472, // Claw (pet)
	52474, // Bite (pet)
	52476, // Smack (pet)
	53209, // Chimera Shot
	53254, // Wild Quiver Auto Shot
	53508, // Wolverine Bite (pet)
	53548, // Pin (pet)
	53582, // Savage Rend (pet)
	53598, // Spore Cloud (pet)
	55485, // Fire Breath (pet)
	55509, // Venom Web Spray (pet)
	55557, // Poison Spit (pet)
	55728, // Scorpid Poison (pet)
	58434, // Volley
	59886, // Rake (pet)
	61006, // Kill Shot
	61198, // Spirit Strike (pet)
	63672, // Black Arrow
}
