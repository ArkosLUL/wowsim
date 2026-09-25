package paladin

// UndeclaredSpells are the Paladin spells, pets' included, with a damage or heal effect on the server that no
// declaration covers (core.SpellEffect). sim/serverdata_test.go keeps the list exact.
var UndeclaredSpells = []int32{
	20187, // Judgement of Righteousness
	20424, // Seal of Command
	20467, // Judgement of Command
	31803, // Holy Vengeance
	31804, // Judgement of Vengeance
	35395, // Crusader Strike
	42463, // Seal of Vengeance
	48801, // Exorcism
	48806, // Hammer of Wrath
	48817, // Holy Wrath
	48819, // Consecration
	48827, // Avenger's Shield
	53385, // Divine Storm
	53595, // Hammer of the Righteous
	61411, // Shield of Righteousness
	61840, // Righteous Vengeance
}
