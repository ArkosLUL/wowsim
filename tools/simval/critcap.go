package main

import "github.com/wowsims/wotlk/sim/core/serverdata"

// SPELL_ATTR2_CANT_CRIT, in Attributes[2].
const attr2CantCrit = 0x20000000

// The effects that make a spell crit capable (SpellInfo::ComputeIsCritCapable): school damage, health
// leech, heal, the weapon damage effects, power burn, mechanical heal and percent heal.
var critEffects = map[int32]bool{2: true, 9: true, 10: true, 17: true, 31: true, 58: true, 62: true, 75: true, 121: true, 136: true}

// critCapable reports whether the server rolls a crit for the spell at all: it has one of critEffects
// and isn't CANT_CRIT. known is false for an id serverdata lacks, which is then assumed capable.
func critCapable(spellID int32) (capable, known bool) {
	spell := serverdata.SpellByID(spellID)
	if spell == nil {
		return true, false
	}
	if spell.Attributes[2]&attr2CantCrit != 0 {
		return false, true
	}
	for _, effect := range spell.Effects {
		if critEffects[effect.Effect] {
			return true, true
		}
	}
	return false, true
}
