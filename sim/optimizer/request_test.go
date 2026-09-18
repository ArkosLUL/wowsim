package optimizer

import (
	"runtime"
	"strings"
	"testing"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
	"github.com/wowsims/wotlk/sim/core/stats"
	goproto "google.golang.org/protobuf/proto"
)

// Made-up ids, so these tests don't depend on the with_db item data.
const (
	testHeadID     = 9400001
	testRingID     = 9400002
	testGemID      = 9400003
	testEnchantID  = 9400004
	testPoolRingID = 9400005
	testPoolGemID  = 9400006
	testPoolEnchID = 9400007
)

// testRequest has a warrior in the second slot of the second party, index 6, carrying its own
// database, and a pool with a database of its own.
func testRequest() *proto.OptimizeGearRequest {
	warrior := &proto.Player{
		Name:  "Target",
		Race:  proto.Race_RaceOrc,
		Class: proto.Class_ClassWarrior,
		Equipment: testEquipment(map[proto.ItemSlot]*proto.ItemSpec{
			proto.ItemSlot_ItemSlotHead:    {Id: testHeadID, Enchant: testEnchantID, Gems: []int32{testGemID}},
			proto.ItemSlot_ItemSlotFinger1: {Id: testRingID},
		}),
		Database: &proto.SimDatabase{
			Items: []*proto.SimItem{
				{Id: testHeadID, Type: proto.ItemType_ItemTypeHead, Stats: stats.Stats{stats.Strength: 50}.ToFloatArray(), GemSockets: []proto.GemColor{proto.GemColor_GemColorRed}},
				{Id: testRingID, Type: proto.ItemType_ItemTypeFinger},
			},
			Gems:     []*proto.SimGem{{Id: testGemID, Color: proto.GemColor_GemColorRed}},
			Enchants: []*proto.SimEnchant{{EffectId: testEnchantID}},
		},
	}
	return &proto.OptimizeGearRequest{
		Base: &proto.RaidSimRequest{
			Raid: &proto.Raid{Parties: []*proto.Party{
				{Players: []*proto.Player{{Name: "Other", Class: proto.Class_ClassMage, Race: proto.Race_RaceGnome}}},
				{Players: []*proto.Player{{}, warrior}},
			}},
			Encounter:  &proto.Encounter{Duration: 180, Targets: []*proto.Target{{}}},
			SimOptions: &proto.SimOptions{Iterations: 100, RandomSeed: 1},
		},
		TargetRaidIndex: 6,
		Settings:        &proto.OptimizerSettings{ContentPhase: 1},
		Pool: &proto.CandidatePool{
			Slots: []*proto.SlotPool{{
				Slot:           proto.ItemSlot_ItemSlotFinger1,
				ItemIds:        []int32{testRingID, testPoolRingID},
				EnchantOptions: []*proto.ItemEnchantOptions{{ItemId: testPoolRingID, EnchantIds: []int32{testPoolEnchID}}},
			}},
			GemIds:         []int32{testGemID, testPoolGemID},
			MetaConditions: []*proto.MetaGemCondition{{GemId: testPoolGemID, Constraints: []*proto.MetaColorConstraint{{Blue: 1, MinTotal: 2}}}},
			LimitGroups:    []*proto.LimitGroup{{Id: 2, Name: "Jeweler's Gems", MaxEquipped: 3}},
			CatalogItems:   []*proto.CatalogItem{{Id: testPoolRingID, ProgressionTier: 13, LimitCategory: 2}},
			Database: &proto.SimDatabase{
				Items:    []*proto.SimItem{{Id: testPoolRingID, Type: proto.ItemType_ItemTypeFinger}},
				Gems:     []*proto.SimGem{{Id: testPoolGemID, Color: proto.GemColor_GemColorMeta}},
				Enchants: []*proto.SimEnchant{{EffectId: testPoolEnchID}},
			},
			CatalogDate: "2026-09-18",
		},
	}
}

// testEquipment fills the slots it isn't given with empty specs, as the UI does.
func testEquipment(items map[proto.ItemSlot]*proto.ItemSpec) *proto.EquipmentSpec {
	es := &proto.EquipmentSpec{Items: make([]*proto.ItemSpec, NumSlots)}
	for slot := range es.Items {
		es.Items[slot] = &proto.ItemSpec{}
	}
	for slot, spec := range items {
		es.Items[slot] = spec
	}
	return es
}

func TestPrepareRequest(t *testing.T) {
	req := testRequest()
	original := goproto.Clone(req)

	r, err := PrepareRequest(req)
	if err != nil {
		t.Fatal(err)
	}

	if !goproto.Equal(req, original) {
		t.Error("PrepareRequest changed the caller's request")
	}
	if r.TargetIndex != 6 || r.Target().Name != "Target" {
		t.Errorf("target = %d %q, want 6 Target", r.TargetIndex, r.Target().Name)
	}
	for _, party := range r.Base.Raid.Parties {
		for _, player := range party.Players {
			if player.Database != nil {
				t.Errorf("player %q still has a database", player.Name)
			}
		}
	}
	if r.Pool.Database != nil {
		t.Error("the pool still has a database")
	}
	for _, id := range []int32{testHeadID, testRingID, testPoolRingID} {
		if _, ok := core.LookupItem(id); !ok {
			t.Errorf("item %d wasn't added to the database", id)
		}
	}

	if r.Seed.RacialTraits != proto.Race_RaceOrc {
		t.Errorf("seed racial traits = %v, want the player's race", r.Seed.RacialTraits)
	}
	head := r.Seed.Items[proto.ItemSlot_ItemSlotHead]
	if head.ItemID != testHeadID || head.Enchant != testEnchantID || head.Gems[0] != testGemID {
		t.Errorf("seed head = %+v", head)
	}
	if r.Settings.Workers != int32(runtime.GOMAXPROCS(0)) {
		t.Errorf("workers = %d, want GOMAXPROCS", r.Settings.Workers)
	}
	if r.Catalog[testPoolRingID].GetLimitCategory() != 2 || r.LimitGroups[2].GetMaxEquipped() != 3 || r.MetaConditions[testPoolGemID] == nil {
		t.Error("pool lookups are missing entries")
	}
}

func TestPrepareRequestKeepsRacialTraitsAndWarmStarts(t *testing.T) {
	req := testRequest()
	req.Base.Raid.Parties[1].Players[1].RacialTraits = proto.Race_RaceHuman
	req.Settings.WarmStarts = []*proto.EquipmentSpec{testEquipment(map[proto.ItemSlot]*proto.ItemSpec{proto.ItemSlot_ItemSlotFinger1: {Id: testPoolRingID}})}
	req.Settings.Workers = 3

	r, err := PrepareRequest(req)
	if err != nil {
		t.Fatal(err)
	}
	if r.Seed.RacialTraits != proto.Race_RaceHuman {
		t.Errorf("seed racial traits = %v, want human", r.Seed.RacialTraits)
	}
	if len(r.WarmStarts) != 1 || r.WarmStarts[0].Items[proto.ItemSlot_ItemSlotFinger1].ItemID != testPoolRingID || r.WarmStarts[0].RacialTraits != proto.Race_RaceHuman {
		t.Errorf("warm starts = %+v", r.WarmStarts)
	}
	if r.Settings.Workers != 3 {
		t.Errorf("workers = %d, want the requested 3", r.Settings.Workers)
	}
}

func TestPrepareRequestRejects(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*proto.OptimizeGearRequest)
		want   string
	}{
		{"no base", func(r *proto.OptimizeGearRequest) { r.Base = nil }, "no base raid"},
		{"no raid", func(r *proto.OptimizeGearRequest) { r.Base.Raid = nil }, "no base raid"},
		{"negative index", func(r *proto.OptimizeGearRequest) { r.TargetRaidIndex = -1 }, "outside"},
		{"missing party", func(r *proto.OptimizeGearRequest) { r.TargetRaidIndex = 10 }, "outside"},
		{"inactive party", func(r *proto.OptimizeGearRequest) { r.Base.Raid.NumActiveParties = 1 }, "outside"},
		{"empty slot", func(r *proto.OptimizeGearRequest) { r.TargetRaidIndex = 5 }, "empty raid slot"},
		{"past the party", func(r *proto.OptimizeGearRequest) { r.TargetRaidIndex = 8 }, "empty raid slot"},
		{"unknown seed item", func(r *proto.OptimizeGearRequest) {
			r.Base.Raid.Parties[1].Players[1].Equipment.Items[proto.ItemSlot_ItemSlotNeck] = &proto.ItemSpec{Id: 9499999}
		}, "item 9499999 isn't in the database"},
		{"unknown seed gem", func(r *proto.OptimizeGearRequest) {
			r.Base.Raid.Parties[1].Players[1].Equipment.Items[proto.ItemSlot_ItemSlotHead].Gems = []int32{9499998}
		}, "gem 9499998 isn't in the database"},
		{"unknown warm start item", func(r *proto.OptimizeGearRequest) {
			r.Settings.WarmStarts = []*proto.EquipmentSpec{{Items: []*proto.ItemSpec{{Id: 9499997}}}}
		}, "warm start 0"},
		{"unknown pool item", func(r *proto.OptimizeGearRequest) { r.Pool.Slots[0].ItemIds = append(r.Pool.Slots[0].ItemIds, 9499996) }, "item 9499996"},
		{"unknown pool enchant", func(r *proto.OptimizeGearRequest) {
			r.Pool.Slots[0].EnchantOptions[0].EnchantIds = []int32{9499995}
		}, "enchant 9499995"},
		{"unknown pool gem", func(r *proto.OptimizeGearRequest) { r.Pool.GemIds = []int32{9499994} }, "gem 9499994"},
		{"too many gems", func(r *proto.OptimizeGearRequest) {
			r.Base.Raid.Parties[1].Players[1].Equipment.Items[proto.ItemSlot_ItemSlotHead].Gems = []int32{testGemID, testGemID, testGemID, testGemID, testGemID}
		}, "at most 4"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := testRequest()
			tc.change(req)
			_, err := PrepareRequest(req)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("err = %v, want one containing %q", err, tc.want)
			}
		})
	}
}
