package core

import (
	"testing"

	"github.com/wowsims/wotlk/sim/core/proto"
	"github.com/wowsims/wotlk/sim/core/stats"
)

const (
	itemTestReforgeCrit  = 9100001
	gemTestReforge       = 9100002
	enchantTestReforgeID = 9100003
)

func init() {
	addToDatabase(&proto.SimDatabase{
		Items: []*proto.SimItem{{
			Id:          itemTestReforgeCrit,
			Type:        proto.ItemType_ItemTypeHead,
			Stats:       stats.Stats{stats.Stamina: 50, stats.MeleeCrit: 83, stats.SpellCrit: 83, stats.MeleeHit: 40, stats.SpellHit: 40}.ToFloatArray(),
			GemSockets:  []proto.GemColor{proto.GemColor_GemColorRed},
			SocketBonus: stats.Stats{stats.Stamina: 6}.ToFloatArray(),
		}},
		Gems:     []*proto.SimGem{{Id: gemTestReforge, Color: proto.GemColor_GemColorRed, Stats: stats.Stats{stats.MeleeCrit: 20, stats.SpellCrit: 20}.ToFloatArray()}},
		Enchants: []*proto.SimEnchant{{EffectId: enchantTestReforgeID, Stats: stats.Stats{stats.MeleeCrit: 10, stats.SpellCrit: 10}.ToFloatArray()}},
	})
}

func TestReforgeStats(t *testing.T) {
	base := stats.Stats{stats.Stamina: 50, stats.MeleeCrit: 83, stats.SpellCrit: 83, stats.MeleeHit: 40, stats.SpellHit: 40, stats.Spirit: 2}

	for _, tc := range []struct {
		comment string
		reforge *proto.ItemReforge
		want    stats.Stats
	}{
		{
			comment: "83 crit to 33 haste moves both melee and spell ratings",
			reforge: &proto.ItemReforge{FromStatType: 32, ToStatType: 36},
			want:    stats.Stats{stats.MeleeCrit: -33, stats.SpellCrit: -33, stats.MeleeHaste: 33, stats.SpellHaste: 33},
		},
		{
			comment: "crit to expertise",
			reforge: &proto.ItemReforge{FromStatType: 32, ToStatType: 37},
			want:    stats.Stats{stats.MeleeCrit: -33, stats.SpellCrit: -33, stats.Expertise: 33},
		},
		{
			comment: "nil reforge",
		},
		{
			comment: "same from and to",
			reforge: &proto.ItemReforge{FromStatType: 32, ToStatType: 32},
		},
		{
			comment: "from stat not reforgeable (stamina)",
			reforge: &proto.ItemReforge{FromStatType: 7, ToStatType: 36},
		},
		{
			comment: "to stat not reforgeable (spell power)",
			reforge: &proto.ItemReforge{FromStatType: 32, ToStatType: 45},
		},
		{
			comment: "item lacks the from stat",
			reforge: &proto.ItemReforge{FromStatType: 13, ToStatType: 36},
		},
		{
			comment: "item already has the to stat",
			reforge: &proto.ItemReforge{FromStatType: 32, ToStatType: 31},
		},
		{
			comment: "amount rounds down to 0",
			reforge: &proto.ItemReforge{FromStatType: 6, ToStatType: 36},
		},
	} {
		if got := ReforgeStats(base, tc.reforge); got != tc.want {
			t.Errorf("%s: ReforgeStats(%v) = %v, want %v", tc.comment, tc.reforge, got, tc.want)
		}
	}
}

func TestNewItemReforge(t *testing.T) {
	critToHaste := &proto.ItemReforge{FromStatType: 32, ToStatType: 36}
	item := NewItem(ItemSpec{
		ID:      itemTestReforgeCrit,
		Enchant: enchantTestReforgeID,
		Gems:    []int32{gemTestReforge},
		Reforge: critToHaste,
	})

	wantStats := ItemsByID[itemTestReforgeCrit].Stats.Add(stats.Stats{stats.MeleeCrit: -33, stats.SpellCrit: -33, stats.MeleeHaste: 33, stats.SpellHaste: 33})
	if item.Stats != wantStats {
		t.Errorf("Stats = %v, want %v", item.Stats, wantStats)
	}
	if item.Reforge != critToHaste {
		t.Errorf("Reforge = %v, want %v", item.Reforge, critToHaste)
	}
	if len(item.Gems) != 1 || item.Gems[0].Stats[stats.MeleeCrit] != 20 {
		t.Errorf("gems changed by reforge: %v", item.Gems)
	}
	if item.Enchant.Stats[stats.MeleeCrit] != 10 {
		t.Errorf("enchant changed by reforge: %v", item.Enchant)
	}

	// 83 item - 33 reforge + 20 gem + 10 enchant
	if got := item.TotalStats()[stats.MeleeCrit]; got != 80 {
		t.Errorf("total melee crit = %v, want 80", got)
	}
	if got := item.TotalStats()[stats.SpellHaste]; got != 33 {
		t.Errorf("total spell haste = %v, want 33", got)
	}
	if spec := item.ToItemSpecProto(); spec.Reforge != critToHaste {
		t.Errorf("ToItemSpecProto reforge = %v, want %v", spec.Reforge, critToHaste)
	}
}

func TestNewItemDropsInvalidReforge(t *testing.T) {
	item := NewItem(ItemSpec{
		ID:      itemTestReforgeCrit,
		Reforge: &proto.ItemReforge{FromStatType: 32, ToStatType: 31},
	})
	if item.Reforge != nil || item.Stats != ItemsByID[itemTestReforgeCrit].Stats {
		t.Errorf("invalid reforge kept: %v, stats %v", item.Reforge, item.Stats)
	}
}

func TestItemTotalStatsSocketBonus(t *testing.T) {
	gemmed := NewItem(ItemSpec{ID: itemTestReforgeCrit, Gems: []int32{gemTestReforge}})
	if got := gemmed.TotalStats()[stats.Stamina]; got != 56 {
		t.Errorf("stamina with matching gem = %v, want 56", got)
	}
	empty := NewItem(ItemSpec{ID: itemTestReforgeCrit})
	if got := empty.TotalStats()[stats.Stamina]; got != 50 {
		t.Errorf("stamina with empty socket = %v, want 50", got)
	}

	equipment := Equipment{}
	equipment[proto.ItemSlot_ItemSlotHead] = gemmed
	if got, want := equipment.Stats(), gemmed.TotalStats(); got != want {
		t.Errorf("equipment stats = %v, want %v", got, want)
	}
}
