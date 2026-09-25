package azerothcore

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/wowsims/wotlk/sim/core"
)

// enchantments builds an item_instance.enchantments string with the given tokens set.
func enchantments(tokens map[int]int32) string {
	values := make([]string, enchantmentTokens)
	for i := range values {
		values[i] = "0"
	}
	for index, value := range tokens {
		values[index] = strconv.Itoa(int(value))
	}
	return strings.Join(values, " ") + " "
}

func TestParseEnchantments(t *testing.T) {
	enchant, sockets, prismatic, err := ParseEnchantments(enchantments(map[int]int32{
		permEnchantToken: 3817, socketEnchantToken: 3563, socketEnchantToken + 6: 3520, prismaticEnchantToken: 3729,
	}))
	if err != nil {
		t.Fatal(err)
	}
	if enchant != 3817 || prismatic != 3729 {
		t.Errorf("enchant %d, prismatic %d", enchant, prismatic)
	}
	if sockets != [maxGemSockets]int32{3563, 0, 3520} {
		t.Errorf("sockets = %v", sockets)
	}
}

func TestParseEnchantmentsRejectsBadInput(t *testing.T) {
	for comment, value := range map[string]string{
		"empty":        "",
		"too short":    strings.TrimSuffix(enchantments(nil), "0 "),
		"not a number": strings.Replace(enchantments(nil), "0", "x", 1),
	} {
		if _, _, _, err := ParseEnchantments(value); err == nil {
			t.Errorf("%s: accepted", comment)
		}
	}
}

func TestBuildGems(t *testing.T) {
	gemItems := map[int32]int32{3563: 40111, 3520: 40155, 3600: 40119}

	for _, tc := range []struct {
		comment       string
		sockets       [maxGemSockets]int32
		nativeSockets int32
		unknown       bool // no item_template row, so nativeSockets says nothing
		prismatic     int32
		wantGems      []int32
		wantExtra     int32
		wantUnmapped  []int32
		wantDropped   []int32
	}{
		{
			comment: "two sockets and a belt buckle",
			sockets: [maxGemSockets]int32{3563, 3520, 3600}, nativeSockets: 2, prismatic: 3729,
			wantGems: []int32{40111, 40155}, wantExtra: 40119,
		},
		{
			comment: "extra socket left empty",
			sockets: [maxGemSockets]int32{3563, 0, 0}, nativeSockets: 1, prismatic: 3729,
			wantGems: []int32{40111},
		},
		{
			comment: "three sockets have no room for a prismatic gem",
			sockets: [maxGemSockets]int32{3563, 3520, 3600}, nativeSockets: 3, prismatic: 3729,
			wantGems: []int32{40111, 40155, 40119},
		},
		{
			comment: "gem in a socket the item doesn't have is reported as dropped",
			sockets: [maxGemSockets]int32{3563, 3520, 0}, nativeSockets: 1,
			wantGems: []int32{40111}, wantDropped: []int32{3520},
		},
		{
			comment: "unmapped gem enchant",
			sockets: [maxGemSockets]int32{9999, 3520, 0}, nativeSockets: 2,
			wantGems: []int32{0, 40155}, wantUnmapped: []int32{9999},
		},
		{
			comment: "unknown template, so every filled socket is the item's own",
			sockets: [maxGemSockets]int32{3563, 3520, 0}, unknown: true,
			wantGems: []int32{40111, 40155},
		},
		{
			comment: "unknown template with a belt buckle, whose gem is the last filled socket",
			sockets: [maxGemSockets]int32{3563, 3520, 0}, unknown: true, prismatic: 3729,
			wantGems: []int32{40111}, wantExtra: 40155,
		},
		{
			comment:  "no sockets",
			wantGems: []int32{},
		},
	} {
		gems, extra, unmapped, dropped := BuildGems(tc.sockets, tc.nativeSockets, !tc.unknown, tc.prismatic, gemItems)
		if !reflect.DeepEqual(gems, tc.wantGems) || extra != tc.wantExtra ||
			!reflect.DeepEqual(unmapped, tc.wantUnmapped) || !reflect.DeepEqual(dropped, tc.wantDropped) {
			t.Errorf("%s: gems %v, extra %d, unmapped %v, dropped %v; want %v, %d, %v, %v",
				tc.comment, gems, extra, unmapped, dropped, tc.wantGems, tc.wantExtra, tc.wantUnmapped, tc.wantDropped)
		}
	}
}

// testTrees is a class with two trees of 3 talents, laid out like the sim's talent tree files.
var testTrees = TalentTrees{
	2: {
		{{Row: 0, Col: 0}, {Row: 0, Col: 1}, {Row: 1, Col: 0}},
		{{Row: 0, Col: 0}, {Row: 0, Col: 2}, {Row: 2, Col: 1}},
	},
}

// testDBC matches testTrees: tab 10 is the first tree, tab 11 the second, tab 12 another class'.
var testDBC = &RosterDBC{
	TalentRanks: map[int32]TalentRank{
		101: {TabID: 10, Row: 0, Col: 0, Rank: 1},
		102: {TabID: 10, Row: 0, Col: 0, Rank: 2},
		103: {TabID: 10, Row: 1, Col: 0, Rank: 1},
		104: {TabID: 11, Row: 2, Col: 1, Rank: 3},
		105: {TabID: 10, Row: 4, Col: 3, Rank: 1},
		106: {TabID: 12, Row: 0, Col: 0, Rank: 1},
	},
	TalentTabs: map[int32]TalentTabEntry{
		10: {ClassMask: 1 << 1, TabPage: 0},
		11: {ClassMask: 1 << 1, TabPage: 1},
		12: {ClassMask: 1 << 3, TabPage: 0},
	},
	Glyphs: map[int32]GlyphPropsEntry{
		170: {SpellID: 54733},
		399: {SpellID: 58386, Minor: true},
	},
}

func TestBuildTalentString(t *testing.T) {
	for _, tc := range []struct {
		comment       string
		spells        []int32
		want          string
		wantPoints    int32
		wantUnmatched []int32
	}{
		{
			comment: "digits sit at the talent's place in its tree",
			spells:  []int32{102, 103, 104},
			want:    "201-003", wantPoints: 6,
		},
		{
			comment: "lower ranks of the same talent don't add up",
			spells:  []int32{101, 102},
			want:    "2", wantPoints: 2,
		},
		{
			comment: "trailing zeros and empty trees are trimmed",
			spells:  []int32{101},
			want:    "1", wantPoints: 1,
		},
		{
			comment: "only the second tree",
			spells:  []int32{104},
			want:    "-003", wantPoints: 3,
		},
		{
			comment:       "talent the sim's trees don't have",
			spells:        []int32{105},
			wantUnmatched: []int32{105},
		},
		{
			comment:       "another class' tree",
			spells:        []int32{106},
			wantUnmatched: []int32{106},
		},
		{
			comment:       "spell that isn't a talent",
			spells:        []int32{48932},
			wantUnmatched: []int32{48932},
		},
		{
			comment: "no talents",
		},
	} {
		talents, points, unmatched := BuildTalentString(tc.spells, 2, testDBC, testTrees)
		if talents != tc.want || points != tc.wantPoints || !reflect.DeepEqual(unmatched, tc.wantUnmatched) {
			t.Errorf("%s: %q, %d points, unmatched %v; want %q, %d, %v",
				tc.comment, talents, points, unmatched, tc.want, tc.wantPoints, tc.wantUnmatched)
		}
	}
}

func TestSplitGlyphs(t *testing.T) {
	major, minor, unknown := SplitGlyphs([6]int32{170, 0, 399, 912, 0, 0}, testDBC)
	if !reflect.DeepEqual(major, []int32{54733}) || !reflect.DeepEqual(minor, []int32{58386}) {
		t.Errorf("major %v, minor %v", major, minor)
	}
	if !reflect.DeepEqual(unknown, []int32{912}) {
		t.Errorf("unknown = %v", unknown)
	}
}

func TestLearnedProfessions(t *testing.T) {
	// 129 first aid and 185 cooking are secondary, so they never count.
	skills := map[int32]int32{129: 450, 185: 450, 171: 450, 202: 450, 333: 300, 393: 120}
	want := []string{"Alchemy", "Engineering", "Enchanting", "Skinning"}
	if got := LearnedProfessions(skills, DefaultMinSkill); !reflect.DeepEqual(got, want) {
		t.Errorf("LearnedProfessions = %v, want %v", got, want)
	}
	if got := LearnedProfessions(skills, FullSkill); !reflect.DeepEqual(got, []string{"Alchemy", "Engineering"}) {
		t.Errorf("maxed only = %v", got)
	}
	if got := LearnedProfessions(nil, DefaultMinSkill); !reflect.DeepEqual(got, []string{}) {
		t.Errorf("no professions = %v", got)
	}
}

func TestBuildCharacterWarnsAboutUnmaxedProfessions(t *testing.T) {
	dbc := &RosterDBC{TalentRanks: testDBC.TalentRanks, TalentTabs: testDBC.TalentTabs, Glyphs: testDBC.Glyphs}
	rows := &CharacterRows{Name: "Angry", ClassID: 2, Level: MaxLevel, Skills: map[int32]int32{171: FullSkill, 393: 1}}

	character := BuildCharacter(rows, dbc, testTrees, DefaultMinSkill, nil)
	if !reflect.DeepEqual(character.Professions, []string{"Alchemy", "Skinning"}) {
		t.Fatalf("professions = %v", character.Professions)
	}
	warning := findWarning(character.Warnings, "Skinning")
	if warning == "" || strings.Contains(warning, "Alchemy") {
		t.Errorf("warning about the skill 1 profession = %q, warnings %v", warning, character.Warnings)
	}

	maxedOnly := BuildCharacter(rows, dbc, testTrees, FullSkill, nil)
	if findWarning(maxedOnly.Warnings, "skill") != "" {
		t.Errorf("warned with -minSkill 450: %v", maxedOnly.Warnings)
	}
}

func TestNewRosterReforge(t *testing.T) {
	crit := core.Item{ServerStats: []core.ItemStat{{Type: 32, Value: 100}}}
	if got, reason := NewRosterReforge(32, 36, &crit); reason != "" ||
		got == nil || *got != (RosterReforge{FromStatType: 32, ToStatType: 36}) {
		t.Errorf("crit to haste = %v, %q", got, reason)
	}
	for comment, reforge := range map[string][2]int32{
		"stat type outside the reforgeable list": {7, 36},
		"target outside the list":                {32, 45},
		"same stat":                              {32, 32},
	} {
		if got, reason := NewRosterReforge(reforge[0], reforge[1], nil); got != nil || reason == "" {
			t.Errorf("%s: %v, %q", comment, got, reason)
		}
	}

	// the rest of the server's rule, which only the sim's copy of the item can answer
	for comment, base := range map[string]core.Item{
		"item already has the target stat": {ServerStats: []core.ItemStat{{Type: 32, Value: 100}, {Type: 36, Value: 50}}},
		"too little to reforge away":       {ServerStats: []core.ItemStat{{Type: 32, Value: 2}}},
		"no item_template stats at all":    {},
	} {
		if got, reason := NewRosterReforge(32, 36, &base); got != nil || reason == "" {
			t.Errorf("%s: %v, %q", comment, got, reason)
		}
	}
}

func TestBuildRosterItem(t *testing.T) {
	item := EquippedItem{
		ACSlot: 5, GUID: 42, ItemID: 47112, KnownTemplate: true, NativeSockets: 2,
		Enchantments: enchantments(map[int]int32{permEnchantToken: 3817, socketEnchantToken: 3563,
			socketEnchantToken + 3: 3520, prismaticEnchantToken: 3729, socketEnchantToken + 6: 3600}),
		Reforge: &ReforgeRow{StatDecrease: 13, StatIncrease: 37},
	}

	built, warnings := BuildRosterItem(item, testGemDBC(), nil)
	want := RosterItem{ACSlot: 5, ID: 47112, Enchant: 3817, Gems: []int32{40111, 40155}, ExtraGem: 40119,
		Reforge: &RosterReforge{FromStatType: 13, ToStatType: 37}}
	if !reflect.DeepEqual(built, want) {
		t.Errorf("item = %+v, want %+v", built, want)
	}
	if len(warnings) != 0 {
		t.Errorf("warnings = %v", warnings)
	}
}

func TestBuildRosterItemWarns(t *testing.T) {
	for comment, item := range map[string]EquippedItem{
		"unknown item template": {ItemID: 47112, Enchantments: enchantments(nil)},
		"random property":       {ItemID: 47112, KnownTemplate: true, RandomPropertyID: 446, Enchantments: enchantments(nil)},
		"broken enchantments":   {ItemID: 47112, KnownTemplate: true, Enchantments: "0 0 0"},
		"unmapped gem": {ItemID: 47112, KnownTemplate: true, NativeSockets: 1,
			Enchantments: enchantments(map[int]int32{socketEnchantToken: 9999})},
		"reforge the sim can't model": {ItemID: 47112, KnownTemplate: true, Enchantments: enchantments(nil),
			Reforge: &ReforgeRow{StatDecrease: 7, StatIncrease: 36}},
	} {
		built, warnings := BuildRosterItem(item, testGemDBC(), nil)
		if len(warnings) != 1 {
			t.Errorf("%s: warnings = %v", comment, warnings)
		}
		if built.Reforge != nil {
			t.Errorf("%s: kept reforge %v", comment, built.Reforge)
		}
	}
}

func testGemDBC() *RosterDBC {
	return &RosterDBC{GemItems: map[int32]int32{3563: 40111, 3520: 40155, 3600: 40119}}
}

func TestBuildCharacter(t *testing.T) {
	rows := &CharacterRows{
		Name: "Bulwark", ClassID: 2, RaceID: 1, SwapRaceID: 11, Level: MaxLevel, Subgroup: 4, MemberFlags: 3,
		TalentSpells: []int32{102, 103, 104},
		Glyphs:       [6]int32{170, 399, 0, 0, 0, 0},
		Skills:       map[int32]int32{164: 450, 755: 450},
		Items: []EquippedItem{
			{ACSlot: ACSlotOffHand, ItemID: 47064, KnownTemplate: true, Enchantments: enchantments(nil)},
			{ACSlot: 0, ItemID: 47112, KnownTemplate: true, Enchantments: enchantments(nil)},
		},
	}

	character := BuildCharacter(rows, &RosterDBC{TalentRanks: testDBC.TalentRanks, TalentTabs: testDBC.TalentTabs,
		Glyphs: testDBC.Glyphs, GemItems: testGemDBC().GemItems}, testTrees, DefaultMinSkill, nil)

	if character.Talents != "201-003" {
		t.Errorf("talents = %q", character.Talents)
	}
	if !reflect.DeepEqual(character.Professions, []string{"Blacksmithing", "Jewelcrafting"}) {
		t.Errorf("professions = %v", character.Professions)
	}
	if !reflect.DeepEqual(character.Glyphs, RosterGlyphs{Major: []int32{54733}, Minor: []int32{58386}}) {
		t.Errorf("glyphs = %+v", character.Glyphs)
	}
	// sorted by slot, and without reordering the rows the caller still holds
	if len(character.Gear) != 2 || character.Gear[0].ACSlot != 0 || character.Gear[1].ACSlot != ACSlotOffHand {
		t.Errorf("gear = %+v", character.Gear)
	}
	if rows.Items[0].ACSlot != ACSlotOffHand {
		t.Errorf("rows were sorted in place: %+v", rows.Items)
	}
	// 6 points at level 80, and an off hand with no main hand
	if len(character.Warnings) != 2 {
		t.Errorf("warnings = %v", character.Warnings)
	}
}

func TestBuildCharacterQuiver(t *testing.T) {
	rows := &CharacterRows{Name: "Angry", ClassID: 3, Level: MaxLevel, Skills: map[int32]int32{}, HasQuiver: true}
	character := BuildCharacter(rows, &RosterDBC{}, testTrees, DefaultMinSkill, nil)
	if !character.Quiver {
		t.Errorf("quiver = %v, want true", character.Quiver)
	}

	rows.HasQuiver = false
	character = BuildCharacter(rows, &RosterDBC{}, testTrees, DefaultMinSkill, nil)
	if character.Quiver {
		t.Errorf("quiver = %v, want false", character.Quiver)
	}
}

func TestBuildCharacterBlacksmithSocket(t *testing.T) {
	// gloves with a Blacksmithing socket, and the gem sitting in it
	gloves := EquippedItem{ACSlot: ACSlotHands, ItemID: 45141, KnownTemplate: true,
		Enchantments: enchantments(map[int]int32{prismaticEnchantToken: 3717, socketEnchantToken: 3563})}
	dbc := &RosterDBC{TalentRanks: testDBC.TalentRanks, TalentTabs: testDBC.TalentTabs, Glyphs: testDBC.Glyphs,
		GemItems: testGemDBC().GemItems}
	build := func(skills map[int32]int32) *RosterCharacter {
		return BuildCharacter(&CharacterRows{Name: "Angry", ClassID: 2, Level: MaxLevel, Skills: skills,
			Items: []EquippedItem{gloves}}, dbc, testTrees, DefaultMinSkill, nil)
	}

	character := build(map[int32]int32{164: 450, 171: 450, 202: 450})
	if character.Gear[0].ExtraGem != 40111 {
		t.Fatalf("extra gem = %d", character.Gear[0].ExtraGem)
	}
	if !reflect.DeepEqual(character.Professions, []string{"Alchemy", "Blacksmithing", "Engineering"}) {
		t.Errorf("professions = %v", character.Professions)
	}
	if warning := findWarning(character.Warnings, "Blacksmithing"); warning != "" {
		t.Errorf("warned about a blacksmith: %q", warning)
	}

	character = build(map[int32]int32{171: 450, 202: 450})
	if warning := findWarning(character.Warnings, "Blacksmithing"); warning == "" {
		t.Errorf("no warning for gems the sim drops: %v", character.Warnings)
	}
}

func findWarning(warnings []string, text string) string {
	for _, warning := range warnings {
		if strings.Contains(warning, text) {
			return warning
		}
	}
	return ""
}

// loadoutDBC holds the paladin, druid, DK and shaman trees (tab page = last digit of the tab id), two
// hunter pet trees, and the spells the consumable rules read.
var loadoutDBC = &RosterDBC{
	TalentRanks: map[int32]TalentRank{
		1001: {TabID: 20, Rank: 5}, 1002: {TabID: 22, Rank: 5}, 1003: {TabID: 21, Rank: 5},
		2001: {TabID: 31, Rank: 5}, thickHideRank3: {TabID: 31, Rank: 3},
		3001: {TabID: 40, Rank: 5}, bladeBarrierRank5: {TabID: 40, Rank: 5},
		4001: {TabID: 51, Rank: 5}, shamanisticRage: {TabID: 51, Rank: 1},
	},
	TalentTabs: map[int32]TalentTabEntry{
		20: {ClassMask: 1 << 1, TabPage: 0}, 21: {ClassMask: 1 << 1, TabPage: 1}, 22: {ClassMask: 1 << 1, TabPage: 2},
		30: {ClassMask: 1 << 10, TabPage: 0}, 31: {ClassMask: 1 << 10, TabPage: 1}, 32: {ClassMask: 1 << 10, TabPage: 2},
		40: {ClassMask: 1 << 5, TabPage: 0}, 41: {ClassMask: 1 << 5, TabPage: 1},
		50: {ClassMask: 1 << 6, TabPage: 0}, 51: {ClassMask: 1 << 6, TabPage: 1},
		409: {PetTalentMask: 2}, 410: {PetTalentMask: 1},
	},
	PetTalents: map[int32][]TalentRank{
		61683: {{TabID: 409, Rank: 2}, {TabID: 410, Rank: 2}},                         // Cobra Reflexes
		61685: {{TabID: 409, Col: 1, Rank: 1}, {TabID: 410, Row: 2, Col: 3, Rank: 1}}, // Charge
		53184: {{TabID: 410, Row: 1, Rank: 3}},                                        // Spiked Collar
	},
	Families: map[int32]CreatureFamilyEntry{
		42: {Name: "Worm", PetTalentType: 1}, 1: {Name: "Wolf", PetTalentType: 0}, 26: {Name: "Bird of Prey", PetTalentType: 2},
		15: {Name: "Felhunter", PetTalentType: -1}, 19: {Name: "Doomguard", PetTalentType: -1},
	},
	ManaSpells:   map[int32]bool{43186: true, 41618: true},
	WellFedAuras: map[int32]bool{57371: true, 24799: true},
	FoodBuffs:    map[int32]int32{57370: 57371, 24800: 24799},
}

var loadoutPetTrees = PetTalentTrees{
	1: {{Row: 0, Col: 0}, {Row: 0, Col: 1}, {Row: 1, Col: 0}},
	0: {{Row: 0, Col: 0}, {Row: 1, Col: 0}, {Row: 2, Col: 3}},
}

var loadoutMatrix, _ = ParseWorldBuffMatrix(`
	1:0,2,2,80,80:53760,57371; 2:0,2,1,80,80:53758,57356;
	3:0,11,3,80,80:53760,57358; 4:0,11,1,80,80:53749,53763,57367;
	5:0,6,3,80,80:53760,57371; 6:0,6,0,80,80:53758,57356;
	7:2,2,2,80,80:17626; 8:0,7,1,80,80:53760,57358`)

func testLoadout() Loadout {
	return Loadout{DBC: loadoutDBC, PetTrees: loadoutPetTrees, WorldBuffs: loadoutMatrix,
		AuraItems: map[int32]AuraItem{17627: {ItemID: 13513, SubClass: itemSubClassFlask}, 11390: {ItemID: 9155, SubClass: itemSubClassElixir}}}
}

// Bag items, as item_template has them.
var (
	potionOfSpeed      = BagItem{ItemID: 40211, SubClass: itemSubClassPotion, Quality: 1, ItemLevel: 80, Spells: [5]int32{53908}}
	potionOfWildMagic  = BagItem{ItemID: 40212, SubClass: itemSubClassPotion, Quality: 1, ItemLevel: 80, Spells: [5]int32{53909}}
	hastePotion        = BagItem{ItemID: 22838, SubClass: itemSubClassPotion, Quality: 1, ItemLevel: 70, Spells: [5]int32{28507}}
	runicManaPotion    = BagItem{ItemID: 33448, SubClass: itemSubClassPotion, Quality: 1, ItemLevel: 80, Spells: [5]int32{43186}}
	nethergonVapor     = BagItem{ItemID: 32905, SubClass: itemSubClassPotion, Quality: 1, ItemLevel: 65, Spells: [5]int32{41618}}
	runicHealing       = BagItem{ItemID: 33447, SubClass: itemSubClassPotion, Quality: 1, ItemLevel: 80, Spells: [5]int32{43185}}
	resurgentHealing   = BagItem{ItemID: 39671, SubClass: itemSubClassPotion, Quality: 1, ItemLevel: 75, Spells: [5]int32{53144}}
	dragonfinFilet     = BagItem{ItemID: 43000, SubClass: itemSubClassFood, Quality: 1, ItemLevel: 80, Spells: [5]int32{57370}}
	desertDumplings    = BagItem{ItemID: 20452, SubClass: itemSubClassFood, Quality: 1, ItemLevel: 55, Spells: [5]int32{24800}}
	plainBread         = BagItem{ItemID: 35950, SubClass: itemSubClassFood, Quality: 1, ItemLevel: 75, Spells: [5]int32{45548}}
	endlessRage        = BagItem{ItemID: 46377, SubClass: itemSubClassFlask, Quality: 1, ItemLevel: 75, Spells: [5]int32{53760}}
	gurusElixir        = BagItem{ItemID: 40076, SubClass: itemSubClassElixir, Quality: 1, ItemLevel: 70, Spells: [5]int32{53749}}
	arcaneElixir       = BagItem{ItemID: 9155, SubClass: itemSubClassElixir, Quality: 1, ItemLevel: 47, Spells: [5]int32{11390}}
	felHealthstone     = BagItem{ItemID: 36892, SubClass: 0, Quality: 1, ItemLevel: 80}
	thistleTea         = BagItem{ItemID: 7676, SubClass: 0, Quality: 1, ItemLevel: 35}
	saroniteBomb       = BagItem{ItemID: 41119, SubClass: 8, Quality: 1, ItemLevel: 72}
	thermalSapper      = BagItem{ItemID: 42641, SubClass: 8, Quality: 1, ItemLevel: 80}
	spicedMammothTreat = BagItem{ItemID: 43005, SubClass: itemSubClassFood, Quality: 1, ItemLevel: 75}
)

// consumeSummary turns consumes into "value/source", "?" standing for a value left out.
func consumeSummary(consumes map[string]RosterConsume) map[string]string {
	summary := map[string]string{}
	for field, consume := range consumes {
		value := "?"
		if consume.Value != nil {
			value = fmt.Sprint(consume.Value)
		}
		summary[field] = value + "/" + consume.Source
	}
	return summary
}

// checkConsumes compares the fields want names, and that a bot or player gets every field it should.
func checkConsumes(t *testing.T, comment string, consumes map[string]RosterConsume, bot bool, want map[string]string) {
	t.Helper()
	fields := 13
	if !bot {
		fields = 11 // pet scrolls aren't read off played characters
	}
	if len(consumes) != fields {
		t.Errorf("%s: %d fields, want %d: %v", comment, len(consumes), fields, consumeSummary(consumes))
	}
	summary := consumeSummary(consumes)
	for field, value := range want {
		if summary[field] != value {
			t.Errorf("%s: %s = %s, want %s", comment, field, summary[field], value)
		}
	}
}

func strategies(combat, nonCombat string) *BotStrategies {
	return &BotStrategies{Combat: ParseStrategies(combat), NonCombat: ParseStrategies(nonCombat)}
}

func TestBotConsumes(t *testing.T) {
	dps := strategies("+dps assist,+potions,+racials", "+worldbuff,+food")
	tank := strategies("+tank assist,+tank,+potions", "+worldbuff")
	for _, tc := range []struct {
		comment      string
		rows         CharacterRows
		loadout      func(*Loadout)
		want         map[string]string
		wantWarnings []string
	}{
		{
			comment: "ret paladin: the matrix row's flask and food, the offensive potion over the mana one, no explosives",
			rows: CharacterRows{ClassID: classPaladin, RaceID: 1, TalentSpells: []int32{1002}, Strategies: dps,
				Bags: []BagItem{runicManaPotion, potionOfSpeed, saroniteBomb, felHealthstone}},
			want: map[string]string{
				ConsumeFlask: "FlaskOfEndlessRage/matrix", ConsumeBattleElixir: "BattleElixirUnknown/matrix",
				ConsumeGuardianElixir: "GuardianElixirUnknown/matrix", ConsumeFood: "FoodDragonfinFilet/matrix",
				ConsumeDefaultPotion: "PotionOfSpeed/bags", ConsumePrepopPotion: "UnknownPotion/rules",
				ConsumeFillerExplosive: "ExplosiveUnknown/rules", ConsumeThermalSapper: "false/rules",
				ConsumeDefaultConjured: "ConjuredUnknown/rules", ConsumePetFood: "PetFoodUnknown/rules",
				ConsumePetScrollOfAgility: "0/rules",
			},
		},
		{
			comment: "prot paladin without a DPS strategy drinks the mana potion",
			rows: CharacterRows{ClassID: classPaladin, RaceID: 1, TalentSpells: []int32{1003}, Strategies: tank,
				Bags: []BagItem{potionOfSpeed, runicManaPotion}},
			want: map[string]string{ConsumeFlask: "FlaskOfStoneblood/matrix", ConsumeFood: "FoodRhinoliciousWormsteak/matrix",
				ConsumeDefaultPotion: "RunicManaPotion/bags"},
		},
		{
			comment: "feral druid without Thick Hide 3 reads the cat row",
			rows:    CharacterRows{ClassID: classDruid, RaceID: 4, TalentSpells: []int32{2001}, Strategies: dps},
			want: map[string]string{ConsumeFlask: "FlaskOfEndlessRage/matrix", ConsumeFood: "FoodHeartyRhino/matrix",
				ConsumeDefaultPotion: "UnknownPotion/bags"},
		},
		{
			comment: "bear druid: two elixirs, no flask",
			rows:    CharacterRows{ClassID: classDruid, RaceID: 4, TalentSpells: []int32{2001, thickHideRank3}, Strategies: tank},
			want: map[string]string{ConsumeFlask: "FlaskUnknown/matrix", ConsumeBattleElixir: "GurusElixir/matrix",
				ConsumeGuardianElixir: "ElixirOfProtection/matrix", ConsumeFood: "FoodBlackenedDragonfin/matrix"},
		},
		{
			comment: "blood DK without Blade Barrier 5 reads the DPS row",
			rows: CharacterRows{ClassID: classDeathKnight, RaceID: 11, TalentSpells: []int32{3001}, Strategies: dps,
				Bags: []BagItem{potionOfSpeed}},
			want: map[string]string{ConsumeFlask: "FlaskOfEndlessRage/matrix", ConsumeFood: "FoodDragonfinFilet/matrix",
				ConsumeDefaultPotion: "PotionOfSpeed/bags"},
		},
		{
			comment: "blood tank DK: no mana and not a DPS, so no potion at all",
			rows: CharacterRows{ClassID: classDeathKnight, RaceID: 11, TalentSpells: []int32{3001, bladeBarrierRank5}, Strategies: tank,
				Bags: []BagItem{potionOfSpeed}},
			want: map[string]string{ConsumeFlask: "FlaskOfStoneblood/matrix", ConsumeDefaultPotion: "UnknownPotion/rules"},
		},
		{
			comment: "no potions strategy",
			rows: CharacterRows{ClassID: classPaladin, RaceID: 1, TalentSpells: []int32{1002},
				Strategies: strategies("+dps assist", "+worldbuff"), Bags: []BagItem{potionOfSpeed}},
			want: map[string]string{ConsumeDefaultPotion: "UnknownPotion/rules"},
		},
		{
			comment: "no worldbuff strategy",
			rows: CharacterRows{ClassID: classPaladin, RaceID: 1, TalentSpells: []int32{1002},
				Strategies: strategies("+dps assist,+potions", "+food")},
			want: map[string]string{ConsumeFlask: "FlaskUnknown/rules", ConsumeFood: "FoodUnknown/rules"},
		},
		{
			comment: "no matrix given keeps flask, elixirs and food",
			rows:    CharacterRows{ClassID: classPaladin, RaceID: 1, TalentSpells: []int32{1002}, Strategies: dps},
			loadout: func(loadout *Loadout) { loadout.WorldBuffs = nil },
			want: map[string]string{ConsumeFlask: "?/matrix", ConsumeBattleElixir: "?/matrix", ConsumeGuardianElixir: "?/matrix",
				ConsumeFood: "?/matrix"},
		},
		{
			comment:      "no saved strategies: taken to run worldbuff and potions, and a DPS by spec",
			rows:         CharacterRows{ClassID: classPaladin, RaceID: 1, TalentSpells: []int32{1002}, Bags: []BagItem{potionOfSpeed}},
			want:         map[string]string{ConsumeFlask: "FlaskOfEndlessRage/matrix", ConsumeDefaultPotion: "PotionOfSpeed/bags"},
			wantWarnings: []string{"saved no strategies"},
		},
		{
			comment: "no matrix row for its spec",
			rows:    CharacterRows{ClassID: classPaladin, RaceID: 1, TalentSpells: []int32{1001}, Strategies: dps},
			want: map[string]string{ConsumeFlask: "FlaskUnknown/matrix", ConsumeBattleElixir: "BattleElixirUnknown/matrix",
				ConsumeFood: "FoodUnknown/matrix"},
			wantWarnings: []string{"no world-buff matrix row"},
		},
		{
			comment: "enhancement shaman with Shamanistic Rage drinks no mana potion",
			rows: CharacterRows{ClassID: classShaman, RaceID: 11, TalentSpells: []int32{4001, shamanisticRage},
				Strategies: strategies("+enh,+potions", "+worldbuff"), Bags: []BagItem{runicManaPotion}},
			want: map[string]string{ConsumeFlask: "FlaskOfEndlessRage/matrix", ConsumeDefaultPotion: "UnknownPotion/bags"},
		},
		{
			comment: "enhancement shaman without it does",
			rows: CharacterRows{ClassID: classShaman, RaceID: 11, TalentSpells: []int32{4001},
				Strategies: strategies("+enh,+potions", "+worldbuff"), Bags: []BagItem{runicManaPotion}},
			want: map[string]string{ConsumeDefaultPotion: "RunicManaPotion/bags"},
		},
		{
			comment: "a mana potion the sim doesn't have stays as it is",
			rows: CharacterRows{ClassID: classPaladin, RaceID: 1, TalentSpells: []int32{1003}, Strategies: tank,
				Bags: []BagItem{nethergonVapor, hastePotion}},
			want:         map[string]string{ConsumeDefaultPotion: "?/bags"},
			wantWarnings: []string{"item 32905"},
		},
		{
			comment: "the higher item level wins, and a tie is reported",
			rows: CharacterRows{ClassID: classPaladin, RaceID: 1, TalentSpells: []int32{1002}, Strategies: dps,
				Bags: []BagItem{hastePotion, potionOfWildMagic, potionOfSpeed}},
			want:         map[string]string{ConsumeDefaultPotion: "PotionOfSpeed/bags"},
			wantWarnings: []string{"40211 and 40212"},
		},
		{
			comment: "a flask the sim doesn't have, and a matrix buff that isn't a consumable",
			rows:    CharacterRows{ClassID: classShaman, RaceID: 11, TalentSpells: []int32{4001}, Strategies: dps},
			loadout: func(loadout *Loadout) {
				loadout.WorldBuffs, _ = ParseWorldBuffMatrix("1:0,7,1,80,80:17627,25898,57371")
			},
			want: map[string]string{ConsumeFlask: "?/matrix", ConsumeBattleElixir: "BattleElixirUnknown/matrix",
				ConsumeFood: "FoodDragonfinFilet/matrix"},
			wantWarnings: []string{"spell 17627 (item 13513)", "spell 25898 isn't"},
		},
	} {
		tc.rows.Level, tc.rows.Bot = MaxLevel, true
		loadout := testLoadout()
		if tc.loadout != nil {
			tc.loadout(&loadout)
		}
		consumes, warnings := BotConsumes(&tc.rows, loadout)
		checkConsumes(t, tc.comment, consumes, true, tc.want)
		if len(warnings) != len(tc.wantWarnings) {
			t.Errorf("%s: warnings %v, want %d", tc.comment, warnings, len(tc.wantWarnings))
		}
		for _, want := range tc.wantWarnings {
			if findWarning(warnings, want) == "" {
				t.Errorf("%s: no warning about %q in %v", tc.comment, want, warnings)
			}
		}
	}
}

func TestPlayerConsumes(t *testing.T) {
	for _, tc := range []struct {
		comment      string
		rows         CharacterRows
		want         map[string]string
		wantWarnings []string
	}{
		{
			comment: "the saved flask beats the bags; food, potions, conjured and explosives from the bags",
			rows: CharacterRows{ClassID: classDeathKnight, Auras: []int32{53760, 25898},
				Bags: []BagItem{desertDumplings, dragonfinFilet, gurusElixir, runicManaPotion, runicHealing, potionOfSpeed,
					felHealthstone, thistleTea, thermalSapper, saroniteBomb}},
			want: map[string]string{
				ConsumeFlask: "FlaskOfEndlessRage/buffs", ConsumeBattleElixir: "BattleElixirUnknown/buffs",
				ConsumeGuardianElixir: "GuardianElixirUnknown/buffs", ConsumeFood: "FoodDragonfinFilet/bags",
				ConsumeDefaultPotion: "PotionOfSpeed/bags", ConsumePrepopPotion: "PotionOfSpeed/bags",
				ConsumeDefaultConjured: "ConjuredHealthstone/bags", ConsumeThermalSapper: "true/bags",
				ConsumeExplosiveDecoy: "false/bags", ConsumeFillerExplosive: "ExplosiveSaroniteBomb/bags",
				ConsumePetFood: "PetFoodUnknown/bags",
			},
		},
		{
			comment: "saved Well Fed buff, and a rogue's thistle tea",
			rows:    CharacterRows{ClassID: classRogue, Auras: []int32{57371}, Bags: []BagItem{thistleTea, felHealthstone, spicedMammothTreat}},
			want: map[string]string{ConsumeFood: "FoodDragonfinFilet/buffs", ConsumeFlask: "FlaskUnknown/bags",
				ConsumeDefaultConjured: "ConjuredRogueThistleTea/bags", ConsumePetFood: "PetFoodSpicedMammothTreats/bags",
				ConsumeDefaultPotion: "UnknownPotion/bags", ConsumePrepopPotion: "UnknownPotion/bags"},
		},
		{
			comment:      "a buff food the sim doesn't have stays as it is, plain food doesn't count",
			rows:         CharacterRows{ClassID: classWarrior, Bags: []BagItem{desertDumplings, plainBread}},
			want:         map[string]string{ConsumeFood: "?/bags"},
			wantWarnings: []string{"spell 24799 (item 20452)"},
		},
		{
			comment: "elixirs from the bags, one of which the sim doesn't have",
			rows:    CharacterRows{ClassID: classWarrior, Bags: []BagItem{gurusElixir, arcaneElixir}},
			want: map[string]string{ConsumeFlask: "FlaskUnknown/bags", ConsumeBattleElixir: "GurusElixir/bags",
				ConsumeGuardianElixir: "?/bags"},
			wantWarnings: []string{"item 9155"},
		},
		{
			comment: "a saved flask the sim doesn't have keeps the bags' flask out too",
			rows:    CharacterRows{ClassID: classWarrior, Auras: []int32{17627}, Bags: []BagItem{endlessRage}},
			want: map[string]string{ConsumeFlask: "?/buffs", ConsumeBattleElixir: "BattleElixirUnknown/buffs",
				ConsumeFood: "FoodUnknown/bags"},
			wantWarnings: []string{"spell 17627 (item 13513)"},
		},
		{
			comment: "a class with mana takes the mana potion over the healing one",
			rows:    CharacterRows{ClassID: classPriest, Bags: []BagItem{runicHealing, runicManaPotion}},
			want:    map[string]string{ConsumeDefaultPotion: "RunicManaPotion/bags", ConsumePrepopPotion: "UnknownPotion/bags"},
		},
		{
			comment: "one without mana has no use for a mana potion",
			rows:    CharacterRows{ClassID: classWarrior, Bags: []BagItem{runicManaPotion}},
			want:    map[string]string{ConsumeDefaultPotion: "UnknownPotion/bags"},
		},
		{
			comment:      "potions the sim doesn't have stay as they are",
			rows:         CharacterRows{ClassID: classWarrior, Bags: []BagItem{resurgentHealing}},
			want:         map[string]string{ConsumeDefaultPotion: "?/bags"},
			wantWarnings: []string{"39671"},
		},
	} {
		tc.rows.Level = MaxLevel
		consumes, warnings := PlayerConsumes(&tc.rows, testLoadout())
		checkConsumes(t, tc.comment, consumes, false, tc.want)
		if len(warnings) != len(tc.wantWarnings) {
			t.Errorf("%s: warnings %v, want %d", tc.comment, warnings, len(tc.wantWarnings))
		}
		for _, want := range tc.wantWarnings {
			if findWarning(warnings, want) == "" {
				t.Errorf("%s: no warning about %q in %v", tc.comment, want, warnings)
			}
		}
	}
}

func TestBuildPet(t *testing.T) {
	worm := &PetRows{Name: "Grub", Family: 42, Spells: []int32{52474, 61683, 61685, 53184}}
	pet, warnings := BuildPet(classHunter, worm, loadoutDBC, loadoutPetTrees)
	if pet == nil || pet.PetType != "Worm" || pet.Talents == nil || *pet.Talents != "21" || pet.Summon != "" {
		t.Errorf("worm = %+v", pet)
	}
	// Spiked Collar is only in the ferocity tree, and Bite isn't a talent at all
	if len(warnings) != 1 || !strings.Contains(warnings[0], "53184") {
		t.Errorf("worm warnings = %v", warnings)
	}

	wolf, warnings := BuildPet(classHunter, &PetRows{Name: "Terror", Family: 1, Spells: []int32{53184, 61685}}, loadoutDBC, loadoutPetTrees)
	if wolf.PetType != "Wolf" || *wolf.Talents != "031" || len(warnings) != 0 {
		t.Errorf("wolf = %+v, %v", wolf, warnings)
	}

	bird, warnings := BuildPet(classHunter, &PetRows{Name: "Hoot", Family: 26}, loadoutDBC, loadoutPetTrees)
	if bird.PetType != "BirdOfPrey" || bird.Talents != nil || len(warnings) != 1 {
		t.Errorf("bird of prey without a sim tree = %+v, %v", bird, warnings)
	}

	noTalents, warnings := BuildPet(classHunter, &PetRows{Name: "Worm", Family: 42}, loadoutDBC, loadoutPetTrees)
	if *noTalents.Talents != "" || findWarning(warnings, "no talents") == "" {
		t.Errorf("pet without talents = %+v, %v", noTalents, warnings)
	}

	felhunter, warnings := BuildPet(classWarlock, &PetRows{Name: "Khiigrom", Family: 15}, loadoutDBC, loadoutPetTrees)
	if felhunter.Summon != "Felhunter" || felhunter.PetType != "" || felhunter.Talents != nil || len(warnings) != 0 {
		t.Errorf("felhunter = %+v, %v", felhunter, warnings)
	}

	doomguard, warnings := BuildPet(classWarlock, &PetRows{Name: "Kazzak", Family: 19}, loadoutDBC, loadoutPetTrees)
	if doomguard.Summon != "" || findWarning(warnings, "stays as it is") == "" {
		t.Errorf("doomguard = %+v, %v", doomguard, warnings)
	}

	if pet, warnings := BuildPet(classHunter, nil, loadoutDBC, loadoutPetTrees); pet != nil || findWarning(warnings, "no pet") == "" {
		t.Errorf("hunter without a pet = %+v, %v", pet, warnings)
	}
	if pet, warnings := BuildPet(classDeathKnight, &PetRows{Name: "Ghoul", Family: 40}, loadoutDBC, loadoutPetTrees); pet != nil || warnings != nil {
		t.Errorf("DK ghoul = %+v, %v", pet, warnings)
	}
}

func TestBuildAmmo(t *testing.T) {
	for _, tc := range []struct {
		comment     string
		rows        CharacterRows
		want        *RosterAmmo
		wantWarning bool
	}{
		{"arrows", CharacterRows{ClassID: classHunter, AmmoID: 52021, AmmoDPS: 91.5},
			&RosterAmmo{ItemID: 52021, DPS: 91.5, Value: "IcebladeArrow"}, false},
		{"bullets of the same DPS", CharacterRows{ClassID: classHunter, AmmoID: 41164, AmmoDPS: 67.5},
			&RosterAmmo{ItemID: 41164, DPS: 67.5, Value: "SaroniteRazorheads"}, false},
		{"a DPS two sim ammos share", CharacterRows{ClassID: classHunter, AmmoID: 41584, AmmoDPS: 46.5},
			&RosterAmmo{ItemID: 41584, DPS: 46.5, Value: "TerrorshaftArrow"}, false},
		{"a DPS no sim ammo has", CharacterRows{ClassID: classHunter, AmmoID: 30319, AmmoDPS: 63.5},
			&RosterAmmo{ItemID: 30319, DPS: 63.5}, true},
		{"no ammo", CharacterRows{ClassID: classHunter}, &RosterAmmo{Value: "AmmoNone"}, true},
		{"no ammo with Thori'dal", CharacterRows{ClassID: classHunter, Items: []EquippedItem{{ACSlot: ACSlotRanged, ItemID: thoridalItemID}}},
			nil, false},
		{"a warrior's ammo isn't a sim option", CharacterRows{ClassID: classWarrior, AmmoID: 52020, AmmoDPS: 91.5}, nil, false},
	} {
		ammo, warnings := BuildAmmo(&tc.rows)
		if !reflect.DeepEqual(ammo, tc.want) || (len(warnings) > 0) != tc.wantWarning {
			t.Errorf("%s: %+v, %v; want %+v, warning %v", tc.comment, ammo, warnings, tc.want, tc.wantWarning)
		}
	}
}

func TestBuildLoadout(t *testing.T) {
	rows := &CharacterRows{Name: "Trueshot", ClassID: classHunter, RaceID: 11, Level: MaxLevel, Bot: true,
		Strategies: strategies("+surv,+potions", "+worldbuff"), Bags: []BagItem{potionOfSpeed}}
	character := &RosterCharacter{Warnings: []string{"from BuildCharacter"}}
	BuildLoadout(character, rows, testLoadout())

	if !character.Bot || character.Ammo == nil || character.Ammo.Value != "AmmoNone" {
		t.Errorf("bot %v, ammo %+v", character.Bot, character.Ammo)
	}
	if character.Consumes[ConsumeDefaultPotion].Value != "PotionOfSpeed" || len(character.Consumes) != 13 {
		t.Errorf("consumes = %v", consumeSummary(character.Consumes))
	}
	// no pet, no ammo and no matrix row for a hunter, after what was already there
	if len(character.Warnings) != 4 || character.Warnings[0] != "from BuildCharacter" {
		t.Errorf("warnings = %v", character.Warnings)
	}
}

func TestSpecTab(t *testing.T) {
	for _, tc := range []struct {
		comment string
		rows    CharacterRows
		want    int32
	}{
		{"the tree with the most points", CharacterRows{ClassID: classPaladin, Level: 80, TalentSpells: []int32{1003, 1003, 1002}}, 1},
		{"the first tree on a tie", CharacterRows{ClassID: classPaladin, Level: 80, TalentSpells: []int32{1002, 1003}}, 1},
		{"no talents: the class default", CharacterRows{ClassID: classPaladin, Level: 80}, 2},
		{"below level 10: the class default", CharacterRows{ClassID: classDeathKnight, Level: 9, TalentSpells: []int32{3001}}, 1},
		{"another class' talent doesn't count", CharacterRows{ClassID: classPaladin, Level: 80, TalentSpells: []int32{2001, 1001}}, 0},
	} {
		if got := specTab(&tc.rows, loadoutDBC); got != tc.want {
			t.Errorf("%s: tab %d, want %d", tc.comment, got, tc.want)
		}
	}
}

func TestBotIsDps(t *testing.T) {
	for _, tc := range []struct {
		comment string
		rows    CharacterRows
		want    bool
	}{
		{"healer with dps assist", CharacterRows{ClassID: classPriest, Strategies: strategies("+holy heal,+dps assist", "")}, true},
		{"healer without it", CharacterRows{ClassID: classPriest, Strategies: strategies("+holy heal", "")}, false},
		{"cat druid", CharacterRows{ClassID: classDruid, Strategies: strategies("+cat,+offheal", "")}, false},
		{"warlock tank", CharacterRows{ClassID: classWarlock, Strategies: strategies("+tank,+potions", "")}, true},
		{"another class' DPS strategy", CharacterRows{ClassID: classWarrior, Strategies: strategies("+tank,+balance", "")}, false},
		{"no strategies: ret paladin by spec", CharacterRows{ClassID: classPaladin, Level: 80, TalentSpells: []int32{1002}}, true},
	} {
		if got := botIsDps(&tc.rows, specTab(&tc.rows, loadoutDBC)); got != tc.want {
			t.Errorf("%s: dps %v, want %v", tc.comment, got, tc.want)
		}
	}
}

func TestParseWorldBuffMatrix(t *testing.T) {
	buffs, problems := ParseWorldBuffMatrix(`# WARRIOR ARMS 1:0,1,0,80,80:53760,57358;
		2:2,11,3,70,79: 28520 ,33261,;
		3:0,1,0,80:53760;
		4 no colons;
		5:0,1,x,80,80:53760;
		6:0,1,1,80,80:53760,abc,57358`)
	want := []WorldBuff{
		{Class: 1, MinLevel: 80, MaxLevel: 80, Spell: 53760}, {Class: 1, MinLevel: 80, MaxLevel: 80, Spell: 57358},
		{Faction: 2, Class: 11, Spec: 3, MinLevel: 70, MaxLevel: 79, Spell: 28520},
		{Faction: 2, Class: 11, Spec: 3, MinLevel: 70, MaxLevel: 79, Spell: 33261},
		{Class: 1, Spec: 1, MinLevel: 80, MaxLevel: 80, Spell: 53760}, {Class: 1, Spec: 1, MinLevel: 80, MaxLevel: 80, Spell: 57358},
	}
	if !reflect.DeepEqual(buffs, want) {
		t.Errorf("buffs = %+v", buffs)
	}
	// a short meta block, no colons, a meta token that isn't a number, a spell that isn't one
	if len(problems) != 4 {
		t.Errorf("problems = %v", problems)
	}
}

func TestWorldBuffMatrixSetting(t *testing.T) {
	for comment, text := range map[string]string{
		"env file, quoted over several lines": "      AC_AI_PLAYERBOT_ROLL: \"5\"\n       AC_AI_PLAYERBOT_WORLD_BUFF_MATRIX=\"1:0,1,0,80,80:53760; \n         2:0,1,1,80,80:53760\"\n      AC_NEXT: \"1\"\n",
		"compose environment":                 "    AC_AI_PLAYERBOT_WORLD_BUFF_MATRIX: '1:0,1,0,80,80:53760; 2:0,1,1,80,80:53760'\r\n",
		"playerbots.conf, after a comment":    "# AiPlayerbot.WorldBuffMatrix = 9:0,9,9,1,1:1\nAiPlayerbot.WorldBuffMatrix = # ARMS 1:0,1,0,80,80:53760; # FURY 2:0,1,1,80,80:53760\nAiPlayerbot.Next = 1\n",
		"bare matrix":                         "1:0,1,0,80,80:53760;\n2:0,1,1,80,80:53760\n",
	} {
		buffs, problems := ParseWorldBuffMatrix(WorldBuffMatrixSetting(text))
		if len(buffs) != 2 || buffs[0].Spec != 0 || buffs[1].Spec != 1 || len(problems) != 0 {
			t.Errorf("%s: %+v, %v", comment, buffs, problems)
		}
	}
}

func TestMatrixSpells(t *testing.T) {
	matrix, _ := ParseWorldBuffMatrix("1:0,1,1,80,80:53760,57358; 2:1,0,1,0,0:53760,1; 3:2,1,1,0,0:2; 4:0,1,1,81,90:3; 5:0,1,2,80,80:4")
	if got := MatrixSpells(matrix, 1, 1, 80, 1); !reflect.DeepEqual(got, []int32{53760, 57358, 1}) {
		t.Errorf("alliance fury warrior = %v", got)
	}
	if got := MatrixSpells(matrix, 2, 1, 80, 1); !reflect.DeepEqual(got, []int32{53760, 57358, 2}) {
		t.Errorf("horde fury warrior = %v", got)
	}
}

func TestParseStrategies(t *testing.T) {
	if got := ParseStrategies("+dps assist, +potions,,+worldbuff"); !reflect.DeepEqual(got, []string{"dps assist", "potions", "worldbuff"}) {
		t.Errorf("strategies = %v", got)
	}
	if got := ParseStrategies(""); len(got) != 0 {
		t.Errorf("empty list = %v", got)
	}
}

func TestLoadPetTalentTrees(t *testing.T) {
	dir := t.TempDir()
	for name, rows := range map[string]int{"hunter_ferocity": 1, "hunter_tenacity": 2, "hunter_cunning": 3} {
		tree := `[{"talents": [` + strings.Repeat(`{"location": {"rowIdx": 0, "colIdx": 0}},`, rows-1) + `{"location": {"rowIdx": 4, "colIdx": 1}}]}]`
		if err := os.WriteFile(filepath.Join(dir, name+".json"), []byte(tree), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	trees, err := LoadPetTalentTrees(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(trees[0]) != 1 || len(trees[1]) != 2 || len(trees[2]) != 3 || trees[1][1] != (TalentLocation{Row: 4, Col: 1}) {
		t.Errorf("trees = %+v", trees)
	}
}
