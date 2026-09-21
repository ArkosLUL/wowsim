package azerothcore

import (
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/wowsims/wotlk/sim/core/proto"
	"google.golang.org/protobuf/encoding/protojson"
	googleProto "google.golang.org/protobuf/proto"
)

// bisResult builds a result whose best loadout fills the given slots.
func bisResult(phase int32, items map[proto.ItemSlot]*proto.ItemSpec, alts ...*proto.OptimizerSlotAlternative) *proto.OptimizerResult {
	equipment := make([]*proto.ItemSpec, proto.ItemSlot_ItemSlotRanged+1)
	for i := range equipment {
		equipment[i] = &proto.ItemSpec{}
	}
	for slot, item := range items {
		equipment[slot] = item
	}
	return &proto.OptimizerResult{
		Best:         &proto.OptimizerLoadoutResult{Equipment: &proto.EquipmentSpec{Items: equipment}},
		Alternatives: alts,
		Settings:     &proto.OptimizerSettings{ContentPhase: phase},
		SimCommit:    "0123456789abcdef0123456789abcdef01234567",
		CatalogDate:  "2026-09-19",
	}
}

func bisAlt(slot proto.ItemSlot, id int32, delta float64) *proto.OptimizerSlotAlternative {
	return &proto.OptimizerSlotAlternative{Slot: slot, Item: &proto.ItemSpec{Id: id}, ScoreDelta: delta}
}

func headResult(phase int32, head int32) *proto.OptimizerResult {
	return bisResult(phase, map[proto.ItemSlot]*proto.ItemSpec{proto.ItemSlot_ItemSlotHead: {Id: head}})
}

var testEnchants EnchantSpells = func(effectID int32, slot proto.ItemSlot) (int32, bool) {
	spell, ok := map[int32]int32{3817: 59954, 3789: 59621}[effectID]
	return spell, ok
}

func TestSimEnchantSpells(t *testing.T) {
	db := &proto.UIDatabase{Enchants: []*proto.UIEnchant{
		{EffectId: 2649, SpellId: 27950, Type: proto.ItemType_ItemTypeFeet},
		{EffectId: 2649, SpellId: 27914, Type: proto.ItemType_ItemTypeWrist},
		{EffectId: 3222, SpellId: 44529, Type: proto.ItemType_ItemTypeHands},
		{EffectId: 3222, SpellId: 42620, Type: proto.ItemType_ItemTypeWeapon},
		{EffectId: 3329, SpellId: 50906, Type: proto.ItemType_ItemTypeHead,
			ExtraTypes: []proto.ItemType{proto.ItemType_ItemTypeChest, proto.ItemType_ItemTypeLegs}},
	}}
	data, err := protojson.Marshal(db)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "db.json")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	enchants, err := SimEnchantSpells(path)
	if err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct {
		effect int32
		slot   proto.ItemSlot
		want   int32
	}{
		{2649, proto.ItemSlot_ItemSlotWrist, 27914},
		{2649, proto.ItemSlot_ItemSlotFeet, 27950},
		{3222, proto.ItemSlot_ItemSlotHands, 44529},
		{3222, proto.ItemSlot_ItemSlotOffHand, 42620},
		{3329, proto.ItemSlot_ItemSlotLegs, 50906},
		// no enchant fits, so the lowest spell
		{2649, proto.ItemSlot_ItemSlotHead, 27914},
	} {
		if got, ok := enchants(tc.effect, tc.slot); !ok || got != tc.want {
			t.Errorf("effect %d in slot %d: got %d, %t, want %d", tc.effect, tc.slot, got, ok, tc.want)
		}
	}
	if spell, ok := enchants(1, proto.ItemSlot_ItemSlotHead); ok {
		t.Errorf("unknown effect gave spell %d", spell)
	}
}

func TestBuildBisBlock(t *testing.T) {
	result := bisResult(4, map[proto.ItemSlot]*proto.ItemSpec{
		proto.ItemSlot_ItemSlotHead: {Id: 51227, Enchant: 3817, Gems: []int32{41398, 0, 40111},
			Reforge: &proto.ItemReforge{FromStatType: 31, ToStatType: 37}},
		proto.ItemSlot_ItemSlotNeck:     {Id: 50633, Enchant: 9999},
		proto.ItemSlot_ItemSlotMainHand: {Id: 50730, Enchant: 3789},
	},
		bisAlt(proto.ItemSlot_ItemSlotHead, 50713, -20.4),
		bisAlt(proto.ItemSlot_ItemSlotHead, 51000, -40),
		bisAlt(proto.ItemSlot_ItemSlotHead, 50712, -5.6),
		// the best item again, e.g. with other gems: it mustn't take a rank or set the delta
		bisAlt(proto.ItemSlot_ItemSlotHead, 51227, -1),
		bisAlt(proto.ItemSlot_ItemSlotHead, 51866, -30),
		// alternatives for an empty slot go nowhere
		bisAlt(proto.ItemSlot_ItemSlotOffHand, 50616, -2),
	)

	block, warnings := BuildBisBlock(result, testEnchants)
	want := BisBlock{Slots: []BisSlot{
		{Slot: proto.ItemSlot_ItemSlotHead, Items: []int32{51227, 50712, 50713, 51866}, Enchant: 59954,
			Gems: []int32{41398, 40111}, ReforgeFrom: 31, ReforgeTo: 37, Delta: 6, HasDelta: true},
		{Slot: proto.ItemSlot_ItemSlotNeck, Items: []int32{50633}},
		{Slot: proto.ItemSlot_ItemSlotMainHand, Items: []int32{50730}, Enchant: 59621},
	}}
	if !reflect.DeepEqual(block, want) {
		t.Errorf("got  %+v\nwant %+v", block, want)
	}
	if len(warnings) != 1 || !strings.Contains(warnings[0], "enchant effect 9999") {
		t.Errorf("warnings = %q", warnings)
	}
}

func testRoster() *Roster {
	return &Roster{Version: RosterVersion, Characters: []*RosterCharacter{
		{Name: "Deathsong", ClassID: 6, Subgroup: 0, Talents: "-55555"},
		{Name: "Tankbot", ClassID: 1, Subgroup: 0, MemberFlags: 2, Talents: "5-5-55555"},
		{Name: "Healbot", ClassID: 5, Subgroup: 1, Talents: "5-55555"},
		{Name: "Bear", ClassID: 11, Subgroup: 1, MemberFlags: 2, Talents: "-55555"},
	}}
}

var testGUIDs = map[string]uint32{"Deathsong": 1001, "Tankbot": 1002, "Healbot": 1003, "Bear": 1004}

func TestBuildBisDataset(t *testing.T) {
	failed := headResult(1, 1)
	failed.ErrorResult = "boom"
	raidDPS := headResult(3, 50000)
	raidDPS.Settings.Objective = proto.OptimizerObjective_OptimizerObjectiveRaidDps
	results := []BisResult{
		{Source: "ds2", Raider: "Deathsong", Result: headResult(2, 45000)},
		{Source: "fire", Class: "Mage", Spec: "Fire FFB", Result: headResult(1, 40001)},
		{Source: "ds1", Raider: "Deathsong", Result: headResult(1, 40000)},
		{Source: "bear", Raider: "Bear", Result: headResult(1, 40002)},
		{Source: "unholy", Class: "Death knight", Spec: "Unholy", Result: raidDPS},
		{Source: "unknown spec", Class: "Mage", Spec: "Spellblade", Result: headResult(1, 1)},
		{Source: "unknown class", Class: "Monk", Spec: "Brewmaster", Result: headResult(1, 1)},
		{Source: "unknown raider", Raider: "Nobody", Result: headResult(1, 1)},
		{Source: "failed", Raider: "Deathsong", Result: failed},
		{Source: "no phase", Raider: "Deathsong", Result: headResult(0, 1)},
		{Source: "noted", Class: "Mage", Spec: "Frost", Result: headResult(1, 40003), Warnings: []string{"a note"}},
	}
	exportedAt := time.Date(2026, 9, 21, 12, 0, 0, 0, time.UTC)
	dataset, err := BuildBisDataset(results, testRoster(), testGUIDs, testEnchants, exportedAt)
	if err != nil {
		t.Fatal(err)
	}

	wantSubjects := []BisSubject{
		{ID: 1, Kind: BisSubjectRoster, ClassID: 6, SpecName: "Frost", GUID: 1001, Name: "Deathsong", RaidIndex: 0},
		{ID: 2, Kind: BisSubjectRoster, ClassID: 11, SpecName: "Feral tank", GUID: 1004, Name: "Bear", RaidIndex: 6},
		{ID: 3, Kind: BisSubjectSpec, ClassID: 6, SpecName: "Unholy", RaidIndex: -1},
		{ID: 4, Kind: BisSubjectSpec, ClassID: 8, SpecName: "Fire FFB", RaidIndex: -1},
		{ID: 5, Kind: BisSubjectSpec, ClassID: 8, SpecName: "Frost", RaidIndex: -1},
	}
	if !reflect.DeepEqual(dataset.Subjects, wantSubjects) {
		t.Errorf("subjects:\ngot  %+v\nwant %+v", dataset.Subjects, wantSubjects)
	}

	var ids []int
	var payloads []string
	for _, block := range dataset.Blocks {
		ids = append(ids, block.ID())
		payloads = append(payloads, block.Payload)
		if block.Checksum != BisChecksum(block.Payload) {
			t.Errorf("block %d: checksum %s", block.ID(), block.Checksum)
		}
	}
	if want := []int{11, 12, 21, 33, 41, 51}; !reflect.DeepEqual(ids, want) {
		t.Errorf("block ids %v, want %v", ids, want)
	}
	if want := []string{"0:40000", "0:45000", "0:40002", "0:50000", "0:40001", "0:40003"}; !reflect.DeepEqual(payloads, want) {
		t.Errorf("payloads %v, want %v", payloads, want)
	}

	wantFingerprint := CompositionFingerprint([]BisCompositionMember{{6, 1}, {1, 2}, {5, 1}, {11, 1}})
	if dataset.Fingerprint != wantFingerprint {
		t.Errorf("fingerprint %s, want %s", dataset.Fingerprint, wantFingerprint)
	}
	if dataset.SimCommit != "0123456789ab" || dataset.CatalogDate != "2026-09-19" || dataset.Objective != "own+raid" {
		t.Errorf("meta %q %q %q", dataset.SimCommit, dataset.CatalogDate, dataset.Objective)
	}
	if !regexp.MustCompile(`^[0-9a-f]{8}$`).MatchString(dataset.Version) {
		t.Errorf("version %q", dataset.Version)
	}
	if len(dataset.Warnings) != 6 || !slices.Contains(dataset.Warnings, "noted: a note") {
		t.Errorf("warnings = %q", dataset.Warnings)
	}

	again, err := BuildBisDataset(results, testRoster(), testGUIDs, testEnchants, exportedAt.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if again.Version != dataset.Version {
		t.Error("exporting again at another time changed the version")
	}
	results[0].Result = headResult(2, 45001)
	changed, err := BuildBisDataset(results, testRoster(), testGUIDs, testEnchants, exportedAt)
	if err != nil {
		t.Fatal(err)
	}
	if changed.Version == dataset.Version {
		t.Error("a changed block kept the version")
	}
}

func TestBuildBisDatasetRejects(t *testing.T) {
	ds := func(phase int32) BisResult {
		return BisResult{Source: "ds", Raider: "Deathsong", Result: headResult(phase, 40000)}
	}
	for comment, tc := range map[string]struct {
		results []BisResult
		roster  *Roster
		guids   map[string]uint32
	}{
		"a raider and phase twice": {[]BisResult{ds(1), ds(1)}, testRoster(), testGUIDs},
		"a spec and phase twice": {[]BisResult{
			{Source: "a", Class: "Mage", Spec: "Fire", Result: headResult(1, 1)},
			{Source: "b", Class: "Mage", Spec: "Fire", Result: headResult(1, 2)},
		}, nil, nil},
		"a raider without a roster":   {[]BisResult{ds(1)}, nil, nil},
		"a raider without a guid":     {[]BisResult{ds(1)}, testRoster(), map[string]uint32{}},
		"a raider and a spec at once": {[]BisResult{{Source: "x", Raider: "Deathsong", Class: "Mage", Spec: "Fire", Result: headResult(1, 1)}}, testRoster(), testGUIDs},
		"neither a raider nor a spec": {[]BisResult{{Source: "x", Result: headResult(1, 1)}}, testRoster(), testGUIDs},
		"nothing usable":              {[]BisResult{ds(0)}, testRoster(), testGUIDs},
	} {
		if _, err := BuildBisDataset(tc.results, tc.roster, tc.guids, testEnchants, time.Now()); err == nil {
			t.Errorf("%s: accepted", comment)
		}
	}
}

func TestBuildBisDatasetUnnamedSpec(t *testing.T) {
	roster := testRoster()
	roster.Characters = append(roster.Characters, &RosterCharacter{Name: "Odd", ClassID: 10, Subgroup: 1, Talents: "5"})
	guids := map[string]uint32{"Deathsong": 1001, "Odd": 1005}
	results := []BisResult{
		{Source: "odd", Raider: "Odd", Result: headResult(1, 1)},
		{Source: "ds", Raider: "Deathsong", Result: headResult(1, 40000)},
	}
	dataset, err := BuildBisDataset(results, roster, guids, testEnchants, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if len(dataset.Subjects) != 1 || dataset.Subjects[0].Name != "Deathsong" {
		t.Errorf("subjects = %+v", dataset.Subjects)
	}
	if len(dataset.Warnings) != 1 || !strings.HasPrefix(dataset.Warnings[0], "odd: ") {
		t.Errorf("warnings = %q", dataset.Warnings)
	}
}

func TestBisResultsFromBatch(t *testing.T) {
	entry := func(raider string, phase, stage int32) *proto.OptimizerBatchEntry {
		return &proto.OptimizerBatchEntry{Raider: raider, ContentPhase: phase, Stage: stage, Result: headResult(phase, 40000+stage)}
	}
	failed := func(entry *proto.OptimizerBatchEntry) *proto.OptimizerBatchEntry {
		entry.Result.ErrorResult = "boom"
		return entry
	}
	batch := &proto.OptimizerBatchExport{Entries: []*proto.OptimizerBatchEntry{
		entry("Deathsong", 1, 1), entry("Deathsong", 1, 2), entry("Deathsong", 2, 1),
		entry("Bear", 1, 2), entry("Bear", 1, 1),
		entry("Bear", 2, 1), failed(entry("Bear", 2, 2)),
		failed(entry("Tankbot", 1, 1)), failed(entry("Tankbot", 1, 2)),
	}}
	var got []string
	for _, result := range BisResultsFromBatch(batch) {
		got = append(got, strings.Join(append([]string{result.Source}, result.Warnings...), " | "))
		if result.Raider == "" || result.Result == nil {
			t.Errorf("%s: raider %q, result %v", result.Source, result.Raider, result.Result)
		}
	}
	want := []string{
		"batch Deathsong phase 1 stage 2",
		"batch Deathsong phase 2 stage 1",
		"batch Bear phase 1 stage 2",
		"batch Bear phase 2 stage 1 | stage 2 skipped: the run failed (boom)",
		// nothing usable: the latest stays, for BuildBisDataset to report
		"batch Tankbot phase 1 stage 2",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got  %q\nwant %q", got, want)
	}
}

func TestLoadBisResults(t *testing.T) {
	dir := t.TempDir()
	write := func(name string, message googleProto.Message) {
		data, err := protojson.Marshal(message)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, name), data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("ds.json", headResult(1, 40000))
	write("fire.json", headResult(2, 40001))
	index := `{"results": [{"file": "ds.json", "raider": "Deathsong"}, {"file": "fire.json", "class": "Mage", "spec": "Fire"}]}`
	if err := os.WriteFile(filepath.Join(dir, "index.json"), []byte(index), 0o644); err != nil {
		t.Fatal(err)
	}

	results, err := LoadBisResults(filepath.Join(dir, "index.json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 2 || results[0].Raider != "Deathsong" || results[1].Class != "Mage" || results[1].Spec != "Fire" ||
		results[1].Result.GetSettings().GetContentPhase() != 2 {
		t.Errorf("results = %+v", results)
	}
}

func TestRaiderSpecName(t *testing.T) {
	for _, tc := range []struct {
		classID, flags int32
		talents, want  string
	}{
		{6, 0, "55555", "Blood dps"},
		{6, 2, "55555", "Blood tank"},
		{6, 2, "-55555", "Frost"},
		{11, 0, "-55555", "Feral dps"},
		{11, 2, "-55555", "Feral tank"},
		{1, 2, "5-5-55555", "Protection"},
		{8, 0, "5-55555-5", "Fire"},
		{8, 0, "55-55", "Arcane"},
		{9, 0, "--5", "Destruction"},
	} {
		if got := RaiderSpecName(&RosterCharacter{ClassID: tc.classID, MemberFlags: tc.flags, Talents: tc.talents}); got != tc.want {
			t.Errorf("class %d, flags %d, %q: got %q, want %q", tc.classID, tc.flags, tc.talents, got, tc.want)
		}
	}
	for classID, specs := range bisTreeSpecs {
		for _, spec := range specs {
			if !slices.Contains(BisSpecNames[classID], spec) {
				t.Errorf("class %d: %q isn't one of the addon's specs", classID, spec)
			}
		}
	}
	for key, spec := range bisTankTreeSpecs {
		if !slices.Contains(BisSpecNames[key[0]], spec) {
			t.Errorf("class %d: %q isn't one of the addon's specs", key[0], spec)
		}
	}
}

func TestBisSlotName(t *testing.T) {
	for _, tc := range []struct {
		slot    proto.ItemSlot
		classID int32
		want    string
	}{
		{proto.ItemSlot_ItemSlotFinger2, 1, "Finger"},
		{proto.ItemSlot_ItemSlotTrinket1, 1, "Trinket"},
		{proto.ItemSlot_ItemSlotMainHand, 1, "Weapon"},
		{proto.ItemSlot_ItemSlotOffHand, 1, "Off hand"},
		{proto.ItemSlot_ItemSlotRanged, 1, "Ranged"},
		{proto.ItemSlot_ItemSlotRanged, 8, "Ranged"},
		{proto.ItemSlot_ItemSlotRanged, 6, "Relic"},
		{proto.ItemSlot_ItemSlotRanged, 11, "Relic"},
	} {
		if got := BisSlotName(tc.slot, tc.classID); got != tc.want {
			t.Errorf("slot %d, class %d: got %q, want %q", tc.slot, tc.classID, got, tc.want)
		}
	}
	for slot := proto.ItemSlot_ItemSlotHead; slot <= proto.ItemSlot_ItemSlotRanged; slot++ {
		if BisSlotName(slot, 1) == "" {
			t.Errorf("slot %d has no name", slot)
		}
	}
}

func testDataset() *BisDataset {
	return &BisDataset{
		Version: "abcd1234", SimCommit: "0123456789ab", CatalogDate: "2026-09-19", Objective: "own",
		Fingerprint: "0badf00d", ExportedAt: time.Date(2026, 9, 21, 12, 30, 0, 0, time.UTC),
		Subjects: []BisSubject{
			{ID: 1, Kind: BisSubjectRoster, ClassID: 6, SpecName: "Frost", GUID: 1001, Name: "Deathsong", RaidIndex: 0},
			{ID: 2, Kind: BisSubjectSpec, ClassID: 8, SpecName: "Fire", RaidIndex: -1},
			{ID: 3, Kind: BisSubjectSpec, ClassID: 8, SpecName: "Frost", RaidIndex: -1},
		},
		Blocks: []BisDatasetBlock{
			{SubjectID: 1, ContentPhase: 1, Payload: "0:51227,50712(e59954,g41398,g40111,r31-37)+142;16:50462", Checksum: "11111111",
				Block: BisBlock{Slots: []BisSlot{
					{Slot: proto.ItemSlot_ItemSlotHead, Items: []int32{51227, 50712}, Enchant: 59954, Gems: []int32{41398, 40111},
						ReforgeFrom: 31, ReforgeTo: 37, Delta: 142, HasDelta: true},
					{Slot: proto.ItemSlot_ItemSlotRanged, Items: []int32{50462}},
				}}},
			{SubjectID: 2, ContentPhase: 4, Payload: "1:50633", Checksum: "22222222",
				Block: BisBlock{Slots: []BisSlot{{Slot: proto.ItemSlot_ItemSlotNeck, Items: []int32{50633}}}}},
			{SubjectID: 3, ContentPhase: 5, Payload: "10:50402(g40125)", Checksum: "33333333",
				Block: BisBlock{Slots: []BisSlot{{Slot: proto.ItemSlot_ItemSlotFinger1, Items: []int32{50402}, Gems: []int32{40125}}}}},
		},
	}
}

func TestWriteBisLua(t *testing.T) {
	var sb strings.Builder
	if err := WriteBisLua(&sb, testDataset()); err != nil {
		t.Fatal(err)
	}
	want := `Bistooltip_server_meta = {
    ["version"] = "abcd1234",
    ["sim_commit"] = "0123456789ab",
    ["catalog_date"] = "2026-09-19",
    ["objective"] = "own",
    ["fingerprint"] = "0badf00d",
}
Bistooltip_server_bislists = {
    ["Mage"] = {
        ["Fire"] = {
            ["T10"] = {
                [1] = { ["slot_name"] = "Neck", ["enhs"] = { }, [1] = 50633 },
            },
        },
        ["Frost"] = {
            ["RS"] = {
                [1] = { ["slot_name"] = "Finger", ["enhs"] = { [1] = { ["type"] = "none", ["id"] = 0 }, [2] = { ["type"] = "item", ["id"] = 40125 } }, [1] = 50402 },
            },
        },
    },
}
Bistooltip_server_roster = {
    [1001] = {
        ["name"] = "Deathsong",
        ["class"] = "Death knight",
        ["spec"] = "Frost",
        ["raid_index"] = 0,
        ["phases"] = {
            ["T7"] = {
                [1] = { ["slot_name"] = "Head", ["enhs"] = { [1] = { ["type"] = "spell", ["id"] = 59954 }, [2] = { ["type"] = "item", ["id"] = 41398 }, [3] = { ["type"] = "none", ["id"] = 0 }, [4] = { ["type"] = "item", ["id"] = 40111 } }, ["extra"] = { ["delta"] = 142, ["reforge"] = { ["from"] = 31, ["to"] = 37 } }, [1] = 51227, [2] = 50712 },
                [2] = { ["slot_name"] = "Relic", ["enhs"] = { }, [1] = 50462 },
            },
        },
    },
}
`
	if got := sb.String(); got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}

	// a class split up by another's specs still opens once
	interleaved := testDataset()
	interleaved.Subjects = append(interleaved.Subjects[:2:2],
		BisSubject{ID: 4, Kind: BisSubjectSpec, ClassID: 6, SpecName: "Unholy", RaidIndex: -1}, interleaved.Subjects[2])
	sb.Reset()
	if err := WriteBisLua(&sb, interleaved); err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(sb.String(), `["Mage"] = {`); n != 1 {
		t.Errorf("Mage opens %d times:\n%s", n, sb.String())
	}
}

func TestWriteBisSQL(t *testing.T) {
	var sb strings.Builder
	if err := WriteBisSQL(&sb, testDataset()); err != nil {
		t.Fatal(err)
	}
	got := sb.String()
	for _, want := range []string{
		BisTablesSQL,
		"START TRANSACTION;\nDELETE FROM `bistooltip_block`;\nDELETE FROM `bistooltip_subject`;\nDELETE FROM `bistooltip_dataset`;\n",
		"('abcd1234', '0123456789ab', '2026-09-19', 'own', '0badf00d', '2026-09-21 12:30:00');\n",
		"(1, 0, 1001, 'Deathsong', 6, 'Frost', 0),\n(2, 1, 0, '', 8, 'Fire', -1),\n(3, 1, 0, '', 8, 'Frost', -1);\n",
		"(1, 1, '0:51227,50712(e59954,g41398,g40111,r31-37)+142;16:50462', '11111111'),\n(2, 4, '1:50633', '22222222'),\n(3, 5, '10:50402(g40125)', '33333333');\n",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing:\n%s\nin:\n%s", want, got)
		}
	}
	if !strings.HasSuffix(got, "COMMIT;\n") {
		t.Error("doesn't end in COMMIT")
	}

	if err := WriteBisSQL(&sb, &BisDataset{Version: "abcd1234"}); err == nil {
		t.Error("wrote a dataset with no blocks")
	}
}

func TestSQLString(t *testing.T) {
	if got, want := sqlString("O'Brien \\ x\n\r\x00"), `'O''Brien \\ x\n\r\0'`; got != want {
		t.Errorf("got %s, want %s", got, want)
	}
}
