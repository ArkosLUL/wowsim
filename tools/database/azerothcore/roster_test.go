package azerothcore

import (
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/wowsims/wotlk/sim/core/stats"
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
	crit := stats.Stats{stats.MeleeCrit: 100, stats.SpellCrit: 100}
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
	for comment, base := range map[string]stats.Stats{
		"item already has the target stat": {stats.MeleeCrit: 100, stats.MeleeHaste: 50},
		"too little to reforge away":       {stats.MeleeCrit: 2},
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
