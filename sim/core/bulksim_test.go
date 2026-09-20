package core

import (
	"context"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/wowsims/wotlk/sim/core/proto"
	"github.com/wowsims/wotlk/sim/core/stats"
	"google.golang.org/protobuf/encoding/protojson"
	goproto "google.golang.org/protobuf/proto"
)

const (
	itemStarshardEdge     = 45620
	itemPillarOfFortitude = 46350
	itemIronmender        = 45271
)

var (
	starshardEdge1 = &itemWithSlot{
		Item:  &proto.ItemSpec{Id: itemStarshardEdge},
		Slot:  proto.ItemSlot_ItemSlotMainHand,
		Index: 0,
	}
	ironmender = &itemWithSlot{
		Item:  &proto.ItemSpec{Id: itemIronmender},
		Slot:  proto.ItemSlot_ItemSlotOffHand,
		Index: 1,
	}
	starshardEdge2 = &itemWithSlot{
		Item:  &proto.ItemSpec{Id: itemStarshardEdge},
		Slot:  proto.ItemSlot_ItemSlotMainHand,
		Index: 2,
	}
	pillarOfFortitude = &itemWithSlot{
		Item:  &proto.ItemSpec{Id: itemPillarOfFortitude},
		Slot:  proto.ItemSlot_ItemSlotMainHand,
		Index: 3,
	}

	tinyItemDatabase = &proto.SimDatabase{
		Items: []*proto.SimItem{
			{Id: itemStarshardEdge, Type: proto.ItemType_ItemTypeWeapon, HandType: proto.HandType_HandTypeMainHand},
			{Id: itemPillarOfFortitude, Type: proto.ItemType_ItemTypeWeapon, HandType: proto.HandType_HandTypeTwoHand},
			{Id: itemIronmender, Type: proto.ItemType_ItemTypeWeapon, HandType: proto.HandType_HandTypeOffHand},
		},
	}
)

func TestIsValidEquipment(t *testing.T) {
	// This is a bit awkward because code everywhere accesses the global database maps. Hopefully
	// this won't mess with any other unit tests that need existing item/gem/enchant databases?
	AddToDatabase(tinyItemDatabase)

	for _, tc := range []struct {
		comment string
		spec    *proto.EquipmentSpec
		want    bool
	}{
		{
			comment: "simple equipment set with just one mainhand weapon is valid",
			spec:    createEquipmentFromItems(starshardEdge1),
			want:    true,
		},
		{
			comment: "cannot equip offhand and two-hander",
			spec:    createEquipmentFromItems(pillarOfFortitude, ironmender),
			want:    false,
		},
	} {
		if got := isValidEquipment(tc.spec); got != tc.want {
			t.Fatalf("%s: isValidEquipment(%v) = %v, want %v", tc.comment, tc.spec, got, tc.want)
		}
	}
}

func createEquipmentFromItems(items ...*itemWithSlot) *proto.EquipmentSpec {
	spec := &proto.EquipmentSpec{
		Items: make([]*proto.ItemSpec, len(proto.ItemSlot_name)),
	}
	for _, is := range items {
		spec.Items[is.Slot] = is.Item
	}
	for i := range spec.Items {
		if spec.Items[i] == nil {
			spec.Items[i] = &proto.ItemSpec{}
		}
	}
	return spec
}

func TestBulkSim(t *testing.T) {
	t.Skip("TODO: Implement")

	fakeRunSim := func(rsr *proto.RaidSimRequest, progress chan *proto.ProgressMetrics, skipPresim bool) *proto.RaidSimResult {
		return &proto.RaidSimResult{}
	}

	bulk := &bulkSimRunner{
		SingleRaidSimRunner: fakeRunSim,
		Request:             &proto.BulkSimRequest{},
	}

	got, err := bulk.Run(context.Background(), nil)
	if err != nil {
		t.Fatalf("BulkSim() returned error: %v", err)
	}

	want := &proto.BulkSimResult{}
	if diff := cmp.Diff(want, got, cmp.Comparer(func(a, b *proto.BulkSimResult) bool {
		return protojson.Format(a) == protojson.Format(b)
	})); diff != "" {
		t.Fatalf("BulkSim() returned diff (-want +got):\n%s", diff)
	}
}

func TestGenerateAllEquipmentSubstitutions(t *testing.T) {
	baseItems := make([]*proto.ItemSpec, len(proto.ItemSlot_name))
	for i := range baseItems {
		baseItems[i] = &proto.ItemSpec{Id: int32(i) + 1000}
	}
	item1 := &proto.ItemSpec{Id: 1}
	item2 := &proto.ItemSpec{Id: 2}
	item3 := &proto.ItemSpec{Id: 1010}
	item4 := &proto.ItemSpec{Id: 4}
	type args struct {
		combinations           bool
		distinctItemSlotCombos []*itemWithSlot
	}
	tests := []struct {
		name string
		args args
		want []*equipmentSubstitution
	}{
		{
			name: "no combos",
			args: args{
				combinations:           true,
				distinctItemSlotCombos: []*itemWithSlot{},
			},
			want: []*equipmentSubstitution{
				{},
			},
		},
		{
			name: "one item",
			args: args{
				combinations: true,
				distinctItemSlotCombos: []*itemWithSlot{
					{Item: item1, Slot: proto.ItemSlot_ItemSlotHead},
				},
			},
			want: []*equipmentSubstitution{
				{},
				{Items: []*itemWithSlot{
					{Item: item1, Slot: proto.ItemSlot_ItemSlotHead},
				}},
			},
		},
		{
			name: "two items",
			args: args{
				combinations: true,
				distinctItemSlotCombos: []*itemWithSlot{
					{Item: item1, Slot: proto.ItemSlot_ItemSlotHead},
					{Item: item2, Slot: proto.ItemSlot_ItemSlotShoulder},
				},
			},
			want: []*equipmentSubstitution{
				{},
				{Items: []*itemWithSlot{
					{Item: item1, Slot: proto.ItemSlot_ItemSlotHead},
				}},
				{Items: []*itemWithSlot{
					{Item: item1, Slot: proto.ItemSlot_ItemSlotHead},
					{Item: item2, Slot: proto.ItemSlot_ItemSlotShoulder},
				}},
				{Items: []*itemWithSlot{
					{Item: item2, Slot: proto.ItemSlot_ItemSlotShoulder},
				}},
			},
		},
		{
			name: "ring and trinket",
			args: args{
				combinations: true,
				distinctItemSlotCombos: []*itemWithSlot{
					{Item: item1, Slot: proto.ItemSlot_ItemSlotFinger1},
					{Item: item1, Slot: proto.ItemSlot_ItemSlotFinger2},
					{Item: item2, Slot: proto.ItemSlot_ItemSlotTrinket1},
					{Item: item2, Slot: proto.ItemSlot_ItemSlotTrinket2},
				},
			},
			want: []*equipmentSubstitution{
				{},
				{Items: []*itemWithSlot{
					{Item: item1, Slot: proto.ItemSlot_ItemSlotFinger1},
				}},
				{Items: []*itemWithSlot{
					{Item: item1, Slot: proto.ItemSlot_ItemSlotFinger1},
					{Item: item2, Slot: proto.ItemSlot_ItemSlotTrinket1},
				}},
				{Items: []*itemWithSlot{
					{Item: item1, Slot: proto.ItemSlot_ItemSlotFinger1},
					{Item: item2, Slot: proto.ItemSlot_ItemSlotTrinket2},
				}},
				{Items: []*itemWithSlot{
					{Item: item1, Slot: proto.ItemSlot_ItemSlotFinger2},
				}},
				{Items: []*itemWithSlot{
					{Item: item1, Slot: proto.ItemSlot_ItemSlotFinger2},
					{Item: item2, Slot: proto.ItemSlot_ItemSlotTrinket1},
				}},
				{Items: []*itemWithSlot{
					{Item: item1, Slot: proto.ItemSlot_ItemSlotFinger2},
					{Item: item2, Slot: proto.ItemSlot_ItemSlotTrinket2},
				}},
				{Items: []*itemWithSlot{
					{Item: item2, Slot: proto.ItemSlot_ItemSlotTrinket1},
				}},
				{Items: []*itemWithSlot{
					{Item: item2, Slot: proto.ItemSlot_ItemSlotTrinket2},
				}},
			},
		},
		{
			name: "two rings and one trinket",
			args: args{
				combinations: true,
				distinctItemSlotCombos: []*itemWithSlot{
					{Item: item1, Slot: proto.ItemSlot_ItemSlotFinger1},
					{Item: item1, Slot: proto.ItemSlot_ItemSlotFinger2},
					{Item: item4, Slot: proto.ItemSlot_ItemSlotFinger1},
					{Item: item4, Slot: proto.ItemSlot_ItemSlotFinger2},
					{Item: item2, Slot: proto.ItemSlot_ItemSlotTrinket1},
					{Item: item2, Slot: proto.ItemSlot_ItemSlotTrinket2},
				},
			},
			want: []*equipmentSubstitution{
				{},
				{Items: []*itemWithSlot{
					{Item: item1, Slot: proto.ItemSlot_ItemSlotFinger1},
				}},
				{Items: []*itemWithSlot{
					{Item: item1, Slot: proto.ItemSlot_ItemSlotFinger1},
					{Item: item4, Slot: proto.ItemSlot_ItemSlotFinger2},
				}},
				{Items: []*itemWithSlot{
					{Item: item1, Slot: proto.ItemSlot_ItemSlotFinger1},
					{Item: item4, Slot: proto.ItemSlot_ItemSlotFinger2},
					{Item: item2, Slot: proto.ItemSlot_ItemSlotTrinket1},
				}},
				{Items: []*itemWithSlot{
					{Item: item1, Slot: proto.ItemSlot_ItemSlotFinger1},
					{Item: item4, Slot: proto.ItemSlot_ItemSlotFinger2},
					{Item: item2, Slot: proto.ItemSlot_ItemSlotTrinket2},
				}},
				{Items: []*itemWithSlot{
					{Item: item1, Slot: proto.ItemSlot_ItemSlotFinger1},
					{Item: item2, Slot: proto.ItemSlot_ItemSlotTrinket1},
				}},
				{Items: []*itemWithSlot{
					{Item: item1, Slot: proto.ItemSlot_ItemSlotFinger1},
					{Item: item2, Slot: proto.ItemSlot_ItemSlotTrinket2},
				}},
				{Items: []*itemWithSlot{
					{Item: item4, Slot: proto.ItemSlot_ItemSlotFinger2},
				}},
				{Items: []*itemWithSlot{
					{Item: item4, Slot: proto.ItemSlot_ItemSlotFinger2},
					{Item: item2, Slot: proto.ItemSlot_ItemSlotTrinket1},
				}},
				{Items: []*itemWithSlot{
					{Item: item4, Slot: proto.ItemSlot_ItemSlotFinger2},
					{Item: item2, Slot: proto.ItemSlot_ItemSlotTrinket2},
				}},
				{Items: []*itemWithSlot{
					{Item: item1, Slot: proto.ItemSlot_ItemSlotFinger1},
				}},
				{Items: []*itemWithSlot{
					{Item: item1, Slot: proto.ItemSlot_ItemSlotFinger1},
					{Item: item2, Slot: proto.ItemSlot_ItemSlotTrinket1},
				}},
				{Items: []*itemWithSlot{
					{Item: item1, Slot: proto.ItemSlot_ItemSlotFinger1},
					{Item: item2, Slot: proto.ItemSlot_ItemSlotTrinket2},
				}},
				{Items: []*itemWithSlot{
					{Item: item4, Slot: proto.ItemSlot_ItemSlotFinger2},
				}},
				{Items: []*itemWithSlot{
					{Item: item4, Slot: proto.ItemSlot_ItemSlotFinger2},
					{Item: item2, Slot: proto.ItemSlot_ItemSlotTrinket1},
				}},
				{Items: []*itemWithSlot{
					{Item: item4, Slot: proto.ItemSlot_ItemSlotFinger2},
					{Item: item2, Slot: proto.ItemSlot_ItemSlotTrinket2},
				}},
				{Items: []*itemWithSlot{
					{Item: item2, Slot: proto.ItemSlot_ItemSlotTrinket1},
				}},
				{Items: []*itemWithSlot{
					{Item: item2, Slot: proto.ItemSlot_ItemSlotTrinket2},
				}},
			},
		},
		{
			name: "special case same itemID",
			args: args{
				combinations: false,
				distinctItemSlotCombos: []*itemWithSlot{
					{Item: item1, Slot: proto.ItemSlot_ItemSlotFinger1},
					{Item: item1, Slot: proto.ItemSlot_ItemSlotFinger2},
					{Item: item3, Slot: proto.ItemSlot_ItemSlotFinger1},
					{Item: item3, Slot: proto.ItemSlot_ItemSlotFinger2},
				},
			},
			want: []*equipmentSubstitution{
				{},
				{Items: []*itemWithSlot{
					{Item: item1, Slot: proto.ItemSlot_ItemSlotFinger1},
				}},
				{Items: []*itemWithSlot{
					{Item: item1, Slot: proto.ItemSlot_ItemSlotFinger1},
					{Item: item3, Slot: proto.ItemSlot_ItemSlotFinger2},
				}},
			},
		},
		{
			name: "special case finger combo",
			args: args{
				combinations: false,
				distinctItemSlotCombos: []*itemWithSlot{
					{Item: item1, Slot: proto.ItemSlot_ItemSlotFinger1},
					{Item: item1, Slot: proto.ItemSlot_ItemSlotFinger2},
					{Item: item2, Slot: proto.ItemSlot_ItemSlotFinger1},
					{Item: item2, Slot: proto.ItemSlot_ItemSlotFinger2},
				},
			},
			want: []*equipmentSubstitution{
				{},
				{Items: []*itemWithSlot{
					{Item: item1, Slot: proto.ItemSlot_ItemSlotFinger1},
				}},
				{Items: []*itemWithSlot{
					{Item: item1, Slot: proto.ItemSlot_ItemSlotFinger1},
					{Item: item2, Slot: proto.ItemSlot_ItemSlotFinger2},
				}},
				{Items: []*itemWithSlot{
					{Item: item2, Slot: proto.ItemSlot_ItemSlotFinger1},
				}},
				{Items: []*itemWithSlot{
					{Item: item1, Slot: proto.ItemSlot_ItemSlotFinger2},
				}},
				{Items: []*itemWithSlot{
					{Item: item2, Slot: proto.ItemSlot_ItemSlotFinger2},
				}},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			results := generateAllEquipmentSubstitutions(context.Background(), baseItems, tt.args.combinations, tt.args.distinctItemSlotCombos)

			idx := 0
			for got := range results {
				wanted := tt.want[idx]
				if len(got.Items) != len(wanted.Items) {
					t.Errorf("generateAllEquipmentSubstitutions(%d) has incorrect number of items, expected: %d, got: %d", idx, len(wanted.Items), len(got.Items))
				}
				for itemIdx, item := range got.Items {
					if wanted.Items[itemIdx].Item.Id != item.Item.Id {
						t.Errorf("generateAllEquipmentSubstitutions(%d) has incorrect item in list, expected: %d, got: %d", idx, wanted.Items[itemIdx].Item.Id, item.Item.Id)
					}
				}
				idx++
			}
			if idx != len(tt.want) {
				t.Errorf("generateAllEquipmentSubstitutions has incorrect number of items, expected: %d, got: %d", len(tt.want), idx)
			}
		})
	}
}

func TestCreateNewRequestWithSubstitutionCarriesReforge(t *testing.T) {
	const (
		oldHead        = 9100011
		critHead       = 9100012
		critHasteHead  = 9100013
		enchantForHead = 9100014
	)
	AddToDatabase(&proto.SimDatabase{
		Items: []*proto.SimItem{
			{Id: oldHead, Type: proto.ItemType_ItemTypeHead, Stats: stats.Stats{stats.MeleeCrit: 83, stats.SpellCrit: 83}.ToFloatArray(),
				ServerStats: []*proto.ItemStat{{StatType: 32, Value: 83}}},
			{Id: critHead, Type: proto.ItemType_ItemTypeHead, Stats: stats.Stats{stats.MeleeCrit: 60, stats.SpellCrit: 60}.ToFloatArray(),
				ServerStats: []*proto.ItemStat{{StatType: 32, Value: 60}}},
			{Id: critHasteHead, Type: proto.ItemType_ItemTypeHead, Stats: stats.Stats{stats.MeleeCrit: 60, stats.SpellCrit: 60, stats.MeleeHaste: 30, stats.SpellHaste: 30}.ToFloatArray(),
				ServerStats: []*proto.ItemStat{{StatType: 32, Value: 60}, {StatType: 36, Value: 30}}},
		},
	})
	critToHaste := &proto.ItemReforge{FromStatType: 32, ToStatType: 36}
	critToHit := &proto.ItemReforge{FromStatType: 32, ToStatType: 31}

	for _, tc := range []struct {
		comment     string
		candidate   *proto.ItemSpec
		autoEnchant bool
		want        *proto.ItemSpec
	}{
		{
			comment:   "valid on the candidate",
			candidate: &proto.ItemSpec{Id: critHead},
			want:      &proto.ItemSpec{Id: critHead, Reforge: critToHaste},
		},
		{
			comment:   "candidate already has the to stat",
			candidate: &proto.ItemSpec{Id: critHasteHead},
			want:      &proto.ItemSpec{Id: critHasteHead},
		},
		{
			comment:   "candidate keeps its own reforge",
			candidate: &proto.ItemSpec{Id: critHead, Reforge: critToHit},
			want:      &proto.ItemSpec{Id: critHead, Reforge: critToHit},
		},
		{
			comment:     "carried together with the enchant",
			candidate:   &proto.ItemSpec{Id: critHead},
			autoEnchant: true,
			want:        &proto.ItemSpec{Id: critHead, Enchant: enchantForHead, Reforge: critToHaste},
		},
	} {
		baseEquipment := createEquipmentFromItems(&itemWithSlot{
			Item: &proto.ItemSpec{Id: oldHead, Enchant: enchantForHead, Reforge: critToHaste},
			Slot: proto.ItemSlot_ItemSlotHead,
		})
		request := &proto.RaidSimRequest{
			Raid: &proto.Raid{Parties: []*proto.Party{{Players: []*proto.Player{{Equipment: baseEquipment}}}}},
		}
		candidate := goproto.Clone(tc.candidate).(*proto.ItemSpec)
		substitution := &equipmentSubstitution{Items: []*itemWithSlot{{Item: candidate, Slot: proto.ItemSlot_ItemSlotHead}}}

		newRequest, changeLog := createNewRequestWithSubstitution(request, substitution, tc.autoEnchant)

		if got := newRequest.Raid.Parties[0].Players[0].Equipment.Items[proto.ItemSlot_ItemSlotHead]; !goproto.Equal(got, tc.want) {
			t.Errorf("%s: equipped %v, want %v", tc.comment, got, tc.want)
		}
		if got := changeLog.AddedItems[0].Item; !goproto.Equal(got, tc.want) {
			t.Errorf("%s: items added %v, want %v", tc.comment, got, tc.want)
		}
		if !goproto.Equal(candidate, tc.candidate) {
			t.Errorf("%s: bulk item changed to %v", tc.comment, candidate)
		}
	}
}
