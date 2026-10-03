package core

import "github.com/wowsims/wotlk/sim/core/serverdata"

// AffectedByAuraEffect is SpellInfo::IsAffected for the aura's first effect: the spell is in the aura's
// family and its family flags meet the effect's class mask.
func (spell *Spell) AffectedByAuraEffect(auraSpellID int32) bool {
	sd, aura := spell.ServerSpell(), serverdata.SpellByID(auraSpellID)
	if sd == nil || aura == nil || aura.Family != sd.Family {
		return false
	}
	mask := aura.Effects[0].ClassMask
	return mask[0]&sd.FamilyFlags[0] != 0 || mask[1]&sd.FamilyFlags[1] != 0 || mask[2]&sd.FamilyFlags[2] != 0
}

// NaturesSwiftnessSpells are the spells with a cast time that Nature's Swiftness (its aura spell id) makes
// instant. Call it once the spellbook is complete.
func (unit *Unit) NaturesSwiftnessSpells(auraSpellID int32) []*Spell {
	var spells []*Spell
	for _, spell := range unit.Spellbook {
		if spell.DefaultCast.CastTime > 0 && spell.AffectedByAuraEffect(auraSpellID) {
			spells = append(spells, spell)
		}
	}
	return spells
}
