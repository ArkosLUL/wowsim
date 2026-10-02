package hunter

import (
	"fmt"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/serverdata"
)

const (
	hunterSpellFamily = 9
	// Serpent Sting and Black Arrow put it on their target: SPELL_AURA_MOD_DAMAGE_FROM_CASTER
	modDamageFromCaster = 271
)

// applyStingDebuffs adds the aura 271 bonus of Serpent Sting (Noxious Stings) and Black Arrow to the hunter's
// own spells whose class flags the effect's mask covers (AuraEffect::IsAffectedOnSpell). Black Arrow's mask
// leaves out Black Arrow itself.
func (hunter *Hunter) applyStingDebuffs() {
	if hunter.Talents.NoxiousStings == 0 && !hunter.Talents.BlackArrow {
		return
	}
	stingMask := debuffMask(49001)
	arrowMask := debuffMask(63672)
	noxious := 1 + 0.01*float64(hunter.Talents.NoxiousStings)

	for _, target := range hunter.Env.Encounter.TargetUnits {
		target.AddDynamicDamageTakenModifier(func(_ *core.Simulation, spell *core.Spell, result *core.SpellResult) {
			if spell.Unit != &hunter.Unit {
				return
			}
			flags, ok := hunter.damageFlags(spell)
			if !ok {
				return
			}
			if noxious > 1 && hunter.SerpentSting != nil && covers(stingMask, flags) && hunter.SerpentSting.Dot(result.Target).IsActive() {
				result.Damage *= noxious
			}
			if hunter.BlackArrow != nil && covers(arrowMask, flags) && hunter.BlackArrow.Dot(result.Target).IsActive() {
				result.Damage *= 1.06
			}
		})
	}
}

func debuffMask(spellID int32) [3]uint32 {
	for _, e := range serverdata.SpellByID(spellID).Effects {
		if e.Aura == modDamageFromCaster {
			return e.ClassMask
		}
	}
	panic(fmt.Sprintf("spell %d has no aura %d", spellID, modDamageFromCaster))
}

func covers(mask, flags [3]uint32) bool {
	return mask[0]&flags[0] != 0 || mask[1]&flags[1] != 0 || mask[2]&flags[2] != 0
}

// damageFlags are the class flags of the spell the server deals the damage with. Auto Shot has no server
// data, and the Explosive Trap cast (49067) deals its damage as the trap effect (49065).
func (hunter *Hunter) damageFlags(spell *core.Spell) ([3]uint32, bool) {
	if spell == hunter.AutoAttacks.RangedAuto() {
		return [3]uint32{0x1}, true
	}
	sd := spell.ServerSpell()
	if spell.ActionID.SpellID == 49067 {
		sd = serverdata.SpellByID(49065)
	}
	if sd == nil || sd.Family != hunterSpellFamily {
		return [3]uint32{}, false
	}
	return sd.FamilyFlags, true
}
