package warrior

// UndeclaredSpells are the Warrior spells, pets' included, with a damage or heal effect on the server that no
// declaration covers (core.SpellEffect). sim/serverdata_test.go keeps the list exact.
var UndeclaredSpells = []int32{
	1680,  // Whirlwind
	7384,  // Overpower
	23881, // Bloodthirst
	46924, // Bladestorm
	47450, // Heroic Strike
	47465, // Rend
	47488, // Shield Slam
	47498, // Devastate
	47502, // Thunder Clap
	47520, // Cleave
	50783, // Slam
	57755, // Heroic Throw
	57823, // Revenge
	64382, // Shattering Throw
}
