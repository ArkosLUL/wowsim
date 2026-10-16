package hunter

import (
	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
)

// Spell::CheckItems fails a cast whose EquippedItemClass the hunter doesn't wear. In Spell.dbc these shots and
// stings want a bow, gun or crossbow (sub class mask 0x4000c), and Raptor Strike a melee weapon (0x2a5f3).
var needsShotWeapon = map[int32]bool{
	3043:  true, // Scorpid Sting
	34490: true, // Silencing Shot
	49001: true, // Serpent Sting
	49045: true, // Arcane Shot
	49048: true, // Multi-Shot
	49050: true, // Aimed Shot
	49052: true, // Steady Shot
	53209: true, // Chimera Shot
	58434: true, // Volley
	60052: true, // Explosive Shot
	60053: true, // Explosive Shot
	61006: true, // Kill Shot
	63672: true, // Black Arrow
}

const raptorStrikeSpellID = 48996

func (hunter *Hunter) wearsShotWeapon() bool {
	switch hunter.Ranged().RangedWeaponType {
	case proto.RangedWeaponType_RangedWeaponTypeBow, proto.RangedWeaponType_RangedWeaponTypeCrossbow,
		proto.RangedWeaponType_RangedWeaponTypeGun:
		return hunter.Ranged().ID != 0
	}
	return false
}

// requireWeapons keeps the shots from being cast without a ranged weapon and Raptor Strike without a main hand,
// as the server does. Without the weapon the sim has no weapon damage to roll for them.
func (hunter *Hunter) requireWeapons() {
	hunter.OnSpellRegistered(func(spell *core.Spell) {
		var worn func() bool
		switch id := spell.ActionID.SpellID; {
		case needsShotWeapon[id]:
			worn = hunter.wearsShotWeapon
		case id == raptorStrikeSpellID:
			worn = hunter.HasMHWeapon
		default:
			return
		}
		if extra := spell.ExtraCastCondition; extra != nil {
			spell.ExtraCastCondition = func(sim *core.Simulation, target *core.Unit) bool {
				return worn() && extra(sim, target)
			}
		} else {
			spell.ExtraCastCondition = func(*core.Simulation, *core.Unit) bool {
				return worn()
			}
		}
	})
}
