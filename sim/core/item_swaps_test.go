package core

import (
	"testing"

	"github.com/wowsims/wotlk/sim/core/proto"
	"github.com/wowsims/wotlk/sim/core/stats"
)

const (
	itemTestSwapSocketed = 9100021
	itemTestSwapPlain    = 9100022
	gemTestSwap          = 9100023
)

func init() {
	AddToDatabase(&proto.SimDatabase{
		Items: []*proto.SimItem{
			{
				Id:          itemTestSwapSocketed,
				Type:        proto.ItemType_ItemTypeWeapon,
				HandType:    proto.HandType_HandTypeOneHand,
				Stats:       stats.Stats{stats.Stamina: 30, stats.MeleeCrit: 50, stats.SpellCrit: 50}.ToFloatArray(),
				GemSockets:  []proto.GemColor{proto.GemColor_GemColorRed},
				SocketBonus: stats.Stats{stats.Stamina: 6}.ToFloatArray(),
			},
			{
				Id:       itemTestSwapPlain,
				Type:     proto.ItemType_ItemTypeWeapon,
				HandType: proto.HandType_HandTypeOneHand,
				Stats:    stats.Stats{stats.Stamina: 40}.ToFloatArray(),
			},
		},
		Gems: []*proto.SimGem{{Id: gemTestSwap, Color: proto.GemColor_GemColorRed, Stats: stats.Stats{stats.Strength: 20}.ToFloatArray()}},
	})
}

func TestItemSwapStatChangesIncludeSocketBonusAndReforge(t *testing.T) {
	character := &Character{}
	character.Equipment[proto.ItemSlot_ItemSlotMainHand] = NewItem(ItemSpec{
		ID:      itemTestSwapSocketed,
		Gems:    []int32{gemTestSwap},
		Reforge: &proto.ItemReforge{FromStatType: 32, ToStatType: 36},
	})
	swap := ItemSwap{character: character}
	*swap.GetItem(proto.ItemSlot_ItemSlotMainHand) = NewItem(ItemSpec{ID: itemTestSwapPlain})

	got := swap.CalcStatChanges([]proto.ItemSlot{proto.ItemSlot_ItemSlotMainHand})

	// 40 stamina on the new weapon vs 30 + 6 socket bonus on the old one
	want := stats.Stats{stats.Stamina: 4, stats.Strength: -20, stats.MeleeCrit: -30, stats.SpellCrit: -30, stats.MeleeHaste: -20, stats.SpellHaste: -20}
	if got != want {
		t.Errorf("stat changes = %v, want %v", got, want)
	}
}

func TestItemSwapSkipsSameItemWithDifferentReforge(t *testing.T) {
	character := &Character{}
	character.Equipment[proto.ItemSlot_ItemSlotMainHand] = NewItem(ItemSpec{
		ID:      itemTestSwapSocketed,
		Reforge: &proto.ItemReforge{FromStatType: 32, ToStatType: 36},
	})

	character.enableItemSwap(&proto.ItemSwap{MhItem: &proto.ItemSpec{Id: itemTestSwapSocketed}}, 1, 1, 1)

	if len(character.ItemSwap.slots) != 0 {
		t.Errorf("swap slots = %v, want none", character.ItemSwap.slots)
	}
}
