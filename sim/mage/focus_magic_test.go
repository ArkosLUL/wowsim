package mage

import (
	"strings"
	"testing"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
)

func focusMagicMage(playerbot bool, uptime int32, target *proto.UnitReference) *proto.Player {
	player := procMage(ArcaneTalents, "p3_fire_alliance", `{"type": "TypeAPL", "priorityList": []}`, &proto.Player_Mage{
		Mage: &proto.Mage{Options: &proto.Mage_Options{
			Armor:                   proto.Mage_Options_MoltenArmor,
			FocusMagicPercentUptime: uptime,
			FocusMagicTarget:        target,
		}},
	})
	player.Equipment = &proto.EquipmentSpec{}
	player.Playerbot = playerbot
	return player
}

func focusMagicAuras(unit *core.Unit) (returned bool, given bool) {
	for _, aura := range unit.GetAuras() {
		returned = returned || aura.Label == "Focus Magic"
		given = given || (strings.HasPrefix(aura.Label, "Focus Magic") && aura.Label != "Focus Magic")
	}
	return returned, given
}

// A bot mage gets none of Focus Magic's crit back; the raid member it targets keeps the buff.
func TestPlayerbotFocusMagic(t *testing.T) {
	encounter := core.MakeSingleTargetEncounter(0)
	for _, playerbot := range []bool{false, true} {
		env, _, _ := core.NewEnvironment(core.SinglePlayerRaidProto(focusMagicMage(playerbot, 100, nil), nil, nil, nil), encounter, false)
		if returned, _ := focusMagicAuras(&env.Raid.Parties[0].Players[0].GetCharacter().Unit); returned == playerbot {
			t.Errorf("playerbot %v, individual sim at 100%% uptime: returned crit %v", playerbot, returned)
		}

		target := &proto.UnitReference{Type: proto.UnitReference_Player, Index: 1}
		raid := core.SinglePlayerRaidProto(focusMagicMage(playerbot, 0, target), nil, nil, nil)
		raid.Parties[0].Players = append(raid.Parties[0].Players, focusMagicMage(false, 0, nil))
		env, _, _ = core.NewEnvironment(raid, encounter, false)
		players := env.Raid.Parties[0].Players
		returned, _ := focusMagicAuras(&players[0].GetCharacter().Unit)
		_, given := focusMagicAuras(&players[1].GetCharacter().Unit)
		if returned == playerbot || !given {
			t.Errorf("playerbot %v, raid: returned crit %v, target buffed %v", playerbot, returned, given)
		}
	}
}
