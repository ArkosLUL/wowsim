package azerothcore

import (
	"cmp"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
	"github.com/wowsims/wotlk/tools/database"
)

// RosterVersion is the format version of the exported file. Importers reject anything else.
const RosterVersion = 1

// Roster is one export of a group of characters, the input the sim's AzerothCore importers read.
type Roster struct {
	Version    int                `json:"version"`
	ExportedAt time.Time          `json:"exportedAt"`
	Group      RosterGroup        `json:"group"`
	Warnings   []string           `json:"warnings"`
	Characters []*RosterCharacter `json:"characters"`
}

type RosterGroup struct {
	Selector string `json:"selector"` // "leader" or "names"
	// Leader and LeaderIsGroupLeader are only set for the "leader" selector. Any group member works,
	// so the flag says whether the named character is the one leading it.
	Leader              string   `json:"leader,omitempty"`
	LeaderIsGroupLeader *bool    `json:"leaderIsGroupLeader,omitempty"`
	Names               []string `json:"names,omitempty"`
}

type RosterCharacter struct {
	Name    string `json:"name"`
	ClassID int32  `json:"classId"`
	RaceID  int32  `json:"raceId"`
	// SwapRaceID is the mod-racial-trait-swap race, 0 when the character has none. Only racials
	// follow it; base stats stay on RaceID.
	SwapRaceID  int32        `json:"swapRaceId"`
	Level       int32        `json:"level"`
	Subgroup    int32        `json:"subgroup"`
	MemberFlags int32        `json:"memberFlags"` // 1 assistant, 2 main tank, 4 main assist
	Talents     string       `json:"talents"`
	Glyphs      RosterGlyphs `json:"glyphs"`
	Professions []string     `json:"professions"`
	Gear        []RosterItem `json:"gear"`
	// A quiver or ammo pouch sits in one of the character's bag slots. Only hunters use it: the
	// AzerothCore importer sets Hunter.Options.quiver from it.
	Quiver   bool     `json:"quiver"`
	Warnings []string `json:"warnings"`
}

// RosterGlyphs holds glyph spell ids, which the sim maps to glyph items.
type RosterGlyphs struct {
	Major []int32 `json:"major"`
	Minor []int32 `json:"minor"`
}

// RosterItem is one equipped item. Gems has an entry per socket the item template has, 0 for an
// empty one, and ExtraGem is the gem in a belt buckle or Blacksmithing socket.
type RosterItem struct {
	ACSlot   int32          `json:"acSlot"`
	ID       int32          `json:"id"`
	Enchant  int32          `json:"enchant"`
	Gems     []int32        `json:"gems"`
	ExtraGem int32          `json:"extraGem"`
	Reforge  *RosterReforge `json:"reforge,omitempty"`
}

// RosterReforge holds raw server ItemModType ids, under the proto ItemReforge field names so the UI
// can read it straight into the proto.
type RosterReforge struct {
	FromStatType int32 `json:"fromStatType"`
	ToStatType   int32 `json:"toStatType"`
}

// AzerothCore equipment slots. 3 (shirt) and 18 (tabard) hold no stats.
const (
	ACSlotShirt    = 3
	ACSlotWrist    = 8
	ACSlotHands    = 9
	ACSlotMainHand = 15
	ACSlotOffHand  = 16
	ACSlotTabard   = 18
	ACSlotCount    = 19
)

// ACSlotBagStart and ACSlotBagEnd are the 4 bag slots, where a quiver or ammo pouch can sit equipped
// like any other bag.
const (
	ACSlotBagStart = 19
	ACSlotBagEnd   = 22
)

// ItemClassQuiver is item_template.class for quivers and ammo pouches.
const ItemClassQuiver = 11

// MaxLevel is the only level the sim supports.
const MaxLevel = 80

// EquippedItem is one equipped item as the characters database holds it.
type EquippedItem struct {
	ACSlot int32
	// GUID is the item_instance row, which character_reforging points at.
	GUID             uint32
	ItemID           int32
	Enchantments     string
	RandomPropertyID int32
	NativeSockets    int32
	// KnownTemplate is false when item_template has no row for the item, so its socket count is a guess.
	KnownTemplate bool
	Reforge       *ReforgeRow
}

type ReforgeRow struct {
	StatDecrease int32
	StatIncrease int32
}

// CharacterRows is everything the databases hold about one character to export.
type CharacterRows struct {
	Name              string
	ClassID           int32
	RaceID            int32
	SwapRaceID        int32
	Level             int32
	ActiveTalentGroup int32
	Subgroup          int32
	MemberFlags       int32
	TalentSpells      []int32 // active talent group only
	Glyphs            [6]int32
	Skills            map[int32]int32
	Items             []EquippedItem
	HasQuiver         bool
}

// BuildCharacter turns one character's rows into its roster entry. Anything it can't express lands
// in Warnings instead of failing the export. minSkill is where a profession counts as known.
func BuildCharacter(rows *CharacterRows, dbc *RosterDBC, trees TalentTrees, minSkill int32,
	itemStats ItemStats) *RosterCharacter {
	character := &RosterCharacter{
		Name:        rows.Name,
		ClassID:     rows.ClassID,
		RaceID:      rows.RaceID,
		SwapRaceID:  rows.SwapRaceID,
		Level:       rows.Level,
		Subgroup:    rows.Subgroup,
		MemberFlags: rows.MemberFlags,
		Gear:        []RosterItem{},
		Quiver:      rows.HasQuiver,
		Warnings:    []string{},
	}
	warn := func(format string, args ...any) {
		character.Warnings = append(character.Warnings, fmt.Sprintf(format, args...))
	}

	if rows.Level != MaxLevel {
		warn("level %d, the sim only sims level %d", rows.Level, MaxLevel)
	}

	talents, points, unmatched := BuildTalentString(rows.TalentSpells, rows.ClassID, dbc, trees)
	character.Talents = talents
	if want := expectedTalentPoints(rows.Level); points != want {
		warn("%d talent points spent, expected %d", points, want)
	}
	for _, spell := range unmatched {
		warn("talent spell %d isn't in the sim's talent trees", spell)
	}

	major, minor, unknown := SplitGlyphs(rows.Glyphs, dbc)
	character.Glyphs = RosterGlyphs{Major: major, Minor: minor}
	for _, glyph := range unknown {
		warn("glyph %d isn't in GlyphProperties.dbc", glyph)
	}

	// a copy, since sorting the caller's slice would reorder rows it still holds
	items := slices.Clone(rows.Items)
	slices.SortFunc(items, func(a, b EquippedItem) int { return cmp.Compare(a.ACSlot, b.ACSlot) })
	hasMainHand := false
	for _, item := range items {
		hasMainHand = hasMainHand || item.ACSlot == ACSlotMainHand
		built, warnings := BuildRosterItem(item, dbc, itemStats)
		character.Gear = append(character.Gear, built)
		character.Warnings = append(character.Warnings, warnings...)
	}
	if !hasMainHand && slices.ContainsFunc(character.Gear, func(item RosterItem) bool { return item.ACSlot == ACSlotOffHand }) {
		warn("off hand equipped without a main hand")
	}

	character.Professions = LearnedProfessions(rows.Skills, minSkill)
	if unmaxed := unmaxedProfessions(rows.Skills, character.Professions); len(unmaxed) > 0 {
		warn("%s aren't at skill %d, where the sim hands out their full bonus anyway", strings.Join(unmaxed, ", "), FullSkill)
	}
	if blacksmithSockets := blacksmithSocketSlots(character.Gear); len(blacksmithSockets) > 0 &&
		!slices.Contains(character.Professions, "Blacksmithing") {
		warn("gems sit in Blacksmithing sockets (slots %v) without the skill, so the sim drops them", blacksmithSockets)
	}
	return character
}

// unmaxedProfessions are the exported professions the character hasn't taken to FullSkill, in the
// order they were exported.
func unmaxedProfessions(skills map[int32]int32, professions []string) []string {
	var unmaxed []string
	for _, profession := range professions {
		if skills[professionSkills[profession]] < FullSkill {
			unmaxed = append(unmaxed, profession)
		}
	}
	return unmaxed
}

// blacksmithSocketSlots are the equipped slots whose extra socket Blacksmithing adds. The sim only
// counts the gem in one for a blacksmith, unlike a belt buckle, which anyone can use.
func blacksmithSocketSlots(gear []RosterItem) []int32 {
	var slots []int32
	for _, item := range gear {
		if item.ExtraGem != 0 && (item.ACSlot == ACSlotWrist || item.ACSlot == ACSlotHands) {
			slots = append(slots, item.ACSlot)
		}
	}
	return slots
}

// expectedTalentPoints is what a character of that level has to spend, 1 per level from 10 on.
func expectedTalentPoints(level int32) int32 {
	return max(level-9, 0)
}

// BuildRosterItem turns one equipped item into its roster entry, and returns what it had to leave out.
func BuildRosterItem(item EquippedItem, dbc *RosterDBC, itemStats ItemStats) (RosterItem, []string) {
	var warnings []string
	warn := func(format string, args ...any) {
		warnings = append(warnings, fmt.Sprintf("slot %d, item %d: ", item.ACSlot, item.ItemID)+fmt.Sprintf(format, args...))
	}

	built := RosterItem{ACSlot: item.ACSlot, ID: item.ItemID, Gems: []int32{}}
	if !item.KnownTemplate {
		warn("the server has no item_template row for it, so its sockets are read off the gems in them")
	}
	if item.RandomPropertyID != 0 {
		warn("random property %d isn't exported", item.RandomPropertyID)
	}
	if item.Reforge != nil {
		var reason string
		built.Reforge, reason = NewRosterReforge(item.Reforge.StatDecrease, item.Reforge.StatIncrease, itemStats.lookup(item.ItemID))
		if reason != "" {
			warn("%s", reason)
		}
	}

	enchant, sockets, prismatic, err := ParseEnchantments(item.Enchantments)
	if err != nil {
		warn("%v", err)
		return built, warnings
	}
	built.Enchant = enchant

	var unmapped, dropped []int32
	built.Gems, built.ExtraGem, unmapped, dropped = BuildGems(sockets, item.NativeSockets, item.KnownTemplate, prismatic, dbc.GemItems)
	for _, gemEnchant := range unmapped {
		warn("gem enchant %d has no gem item", gemEnchant)
	}
	for _, gemEnchant := range dropped {
		warn("gem enchant %d sits past the %d sockets item_template gives the item, dropped", gemEnchant, item.NativeSockets)
	}
	return built, warnings
}

// item_instance.enchantments holds 12 slots of (id, duration, charges), the EnchantmentSlot enum in
// AzerothCore's Item.h.
const (
	enchantmentTokens     = 36
	permEnchantToken      = 0
	socketEnchantToken    = 6  // sockets 1-3, 3 tokens apart
	prismaticEnchantToken = 18 // the socket-adding enchant itself: belt buckle or Blacksmithing
	maxGemSockets         = 3
)

// ParseEnchantments picks the permanent enchant, the gem enchants in the item's own sockets and the
// socket-adding enchant out of item_instance.enchantments.
func ParseEnchantments(enchantments string) (enchant int32, sockets [maxGemSockets]int32, prismatic int32, err error) {
	tokens := strings.Fields(enchantments)
	if len(tokens) != enchantmentTokens {
		return 0, sockets, 0, fmt.Errorf("got %d enchantment tokens, want %d", len(tokens), enchantmentTokens)
	}
	values := make([]int32, len(tokens))
	for i, token := range tokens {
		value, err := strconv.ParseUint(token, 10, 32)
		if err != nil {
			return 0, sockets, 0, fmt.Errorf("enchantment token %d: %w", i, err)
		}
		values[i] = int32(value)
	}
	for i := range sockets {
		sockets[i] = values[socketEnchantToken+3*i]
	}
	return values[permEnchantToken], sockets, values[prismaticEnchantToken], nil
}

// BuildGems maps the socketed gem enchants to gem items. The gem from a socket-adding enchant sits in
// the socket right after the item's own ones, which only exists while the item has fewer than
// MAX_GEM_SOCKETS of them. Without an item_template row the socket count is unknown, so it's read off
// the filled sockets instead, the last of which holds the added gem when the item carries that
// enchant. Gems past what the template accounts for come back in dropped rather than silently going
// missing.
func BuildGems(sockets [maxGemSockets]int32, nativeSockets int32, knownTemplate bool, prismatic int32,
	gemItems map[int32]int32) (gems []int32, extraGem int32, unmapped, dropped []int32) {
	filled := 0
	for i, socket := range sockets {
		if socket != 0 {
			filled = i + 1
		}
	}
	native := int(min(nativeSockets, maxGemSockets))
	if !knownTemplate {
		native = filled
		if prismatic != 0 && filled > 0 {
			native = filled - 1
		}
	}

	gemItem := func(gemEnchant int32) int32 {
		if gemEnchant == 0 {
			return 0
		}
		item := gemItems[gemEnchant]
		if item == 0 {
			unmapped = append(unmapped, gemEnchant)
		}
		return item
	}

	gems = make([]int32, 0, native)
	for i := 0; i < native; i++ {
		gems = append(gems, gemItem(sockets[i]))
	}
	exported := native
	if prismatic != 0 && native < maxGemSockets {
		extraGem = gemItem(sockets[native])
		exported++
	}
	for i := exported; i < maxGemSockets; i++ {
		if sockets[i] != 0 {
			dropped = append(dropped, sockets[i])
		}
	}
	return gems, extraGem, unmapped, dropped
}

// TalentTrees maps a class id to its talent trees in tab page order, each holding its talents in the
// order the sim's talent string uses.
type TalentTrees map[int32][][]TalentLocation

type TalentLocation struct {
	Row int32 `json:"rowIdx"`
	Col int32 `json:"colIdx"`
}

// ClassNames are the sim's names for the AzerothCore class ids. 10 is unused in 3.3.5a.
var ClassNames = map[int32]string{
	1: "warrior", 2: "paladin", 3: "hunter", 4: "rogue", 5: "priest",
	6: "deathknight", 7: "shaman", 8: "mage", 9: "warlock", 11: "druid",
}

// RaceNames are the AzerothCore race ids, for printing. 9 (goblin) has no playable race in 3.3.5a.
var RaceNames = map[int32]string{
	1: "human", 2: "orc", 3: "dwarf", 4: "nightelf", 5: "undead",
	6: "tauren", 7: "gnome", 8: "troll", 10: "bloodelf", 11: "draenei",
}

// LoadTalentTrees reads the sim's talent tree layouts, e.g. ui/core/talents/trees. Only the grid
// positions matter here; the spell ids in those files are incomplete.
func LoadTalentTrees(dir string) (TalentTrees, error) {
	trees := make(TalentTrees, len(ClassNames))
	for classID, name := range ClassNames {
		data, err := os.ReadFile(filepath.Join(dir, name+".json"))
		if err != nil {
			return nil, err
		}
		var file []struct {
			Talents []struct {
				Location TalentLocation `json:"location"`
			} `json:"talents"`
		}
		if err := json.Unmarshal(data, &file); err != nil {
			return nil, fmt.Errorf("%s.json: %w", name, err)
		}

		classTrees := make([][]TalentLocation, len(file))
		for i, tree := range file {
			classTrees[i] = make([]TalentLocation, len(tree.Talents))
			for j, talent := range tree.Talents {
				classTrees[i][j] = talent.Location
			}
		}
		trees[classID] = classTrees
	}
	return trees, nil
}

// BuildTalentString turns learned talent spells into the sim's talent string: one digit per talent in
// tree order, trees separated by '-'.
func BuildTalentString(spells []int32, classID int32, dbc *RosterDBC, trees TalentTrees) (talents string, points int32, unmatched []int32) {
	classTrees := trees[classID]
	ranks := make([][]int32, len(classTrees))
	for i, tree := range classTrees {
		ranks[i] = make([]int32, len(tree))
	}

	for _, spell := range spells {
		talent, ok := dbc.TalentRanks[spell]
		if !ok {
			unmatched = append(unmatched, spell)
			continue
		}
		tab, ok := dbc.TalentTabs[talent.TabID]
		if !ok || tab.ClassMask&(1<<(classID-1)) == 0 || int(tab.TabPage) >= len(classTrees) {
			unmatched = append(unmatched, spell)
			continue
		}
		tree := classTrees[tab.TabPage]
		index := slices.IndexFunc(tree, func(location TalentLocation) bool {
			return location.Row == talent.Row && location.Col == talent.Col
		})
		if index == -1 {
			unmatched = append(unmatched, spell)
			continue
		}
		// a character can hold lower ranks of a talent too
		ranks[tab.TabPage][index] = max(ranks[tab.TabPage][index], talent.Rank)
	}

	treeStrings := make([]string, len(ranks))
	for i, tree := range ranks {
		digits := make([]byte, len(tree))
		for j, rank := range tree {
			digits[j] = byte('0' + rank)
			points += rank
		}
		treeStrings[i] = strings.TrimRight(string(digits), "0")
	}
	return strings.TrimRight(strings.Join(treeStrings, "-"), "-"), points, unmatched
}

// SplitGlyphs turns GlyphProperties ids into glyph spell ids, split by slot kind. The slot a glyph
// sits in doesn't say which kind it is.
func SplitGlyphs(glyphs [6]int32, dbc *RosterDBC) (major, minor, unknown []int32) {
	major, minor = []int32{}, []int32{}
	for _, id := range glyphs {
		if id == 0 {
			continue
		}
		glyph, ok := dbc.Glyphs[id]
		if !ok {
			unknown = append(unknown, id)
			continue
		}
		if glyph.Minor {
			minor = append(minor, glyph.SpellID)
		} else {
			major = append(major, glyph.SpellID)
		}
	}
	return major, minor, unknown
}

// primaryProfessions maps character_skills ids to the sim's profession names.
var primaryProfessions = map[int32]string{
	164: "Blacksmithing", 165: "Leatherworking", 171: "Alchemy", 182: "Herbalism", 186: "Mining",
	197: "Tailoring", 202: "Engineering", 333: "Enchanting", 393: "Skinning", 755: "Jewelcrafting",
	773: "Inscription",
}

// professionSkills maps the sim's profession names back to their character_skills ids.
var professionSkills = func() map[string]int32 {
	skills := make(map[string]int32, len(primaryProfessions))
	for skill, name := range primaryProfessions {
		skills[name] = skill
	}
	return skills
}()

// FullSkill is the skill the sim's profession bonuses are worth, e.g. Mining's +60 stamina. It gives
// them whatever the character's real skill is.
const FullSkill = 450

// DefaultMinSkill counts a profession as known once the character has any skill in it at all, because
// the skill value doesn't say whether the character has the profession's bonus: a GM can hand out the
// bonus without the skill to match. Raise it with -minSkill to keep only professions levelled that far.
const DefaultMinSkill = 1

// LearnedProfessions returns every primary profession at minSkill or above, highest first. An
// AzerothCore character can hold more than the two the game allows, and so can a sim player.
func LearnedProfessions(skills map[int32]int32, minSkill int32) []string {
	type profession struct {
		name  string
		value int32
	}
	var professions []profession
	for skill, value := range skills {
		if name, ok := primaryProfessions[skill]; ok && value >= minSkill {
			professions = append(professions, profession{name, value})
		}
	}
	slices.SortFunc(professions, func(a, b profession) int {
		return cmp.Or(cmp.Compare(b.value, a.value), cmp.Compare(a.name, b.name))
	})

	names := []string{}
	for _, profession := range professions {
		names = append(names, profession.name)
	}
	return names
}

// ItemStats gives an item as the sim's item database holds it, and false for an item the sim has no
// row for. Its copy of the item_template stats, not the server's own, decides whether the sim would
// keep a reforge, and the two can disagree.
type ItemStats func(itemID int32) (core.Item, bool)

// lookup is nil safe: no item database and an item the sim doesn't know both come back nil, which
// leaves a reforge judged on its stat types alone.
func (itemStats ItemStats) lookup(itemID int32) *core.Item {
	if itemStats == nil {
		return nil
	}
	item, ok := itemStats(itemID)
	if !ok {
		return nil
	}
	return &item
}

// SimItemStats reads the sim's item database, e.g. assets/database/db.json.
func SimItemStats(path string) (ItemStats, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	db := database.ReadDatabaseFromJson(string(data))
	return func(itemID int32) (core.Item, bool) {
		item, ok := db.Items[itemID]
		if !ok {
			return core.Item{}, false
		}
		return core.ItemFromProto(&proto.SimItem{Stats: item.Stats, ServerStats: item.ServerStats}), true
	}, nil
}

// NewRosterReforge returns the reforge to export, or nil and the reason the sim would ignore it.
// base is the item in the sim's item database, nil when it doesn't have the item, which leaves only
// the stat types to go on.
func NewRosterReforge(statDecrease, statIncrease int32, base *core.Item) (*RosterReforge, string) {
	reforging := core.LiveReforging()
	if !reforging.ReforgeableStatPair(statDecrease, statIncrease) {
		return nil, fmt.Sprintf("reforge of stat type %d to %d isn't reforgeable, dropped", statDecrease, statIncrease)
	}
	if base != nil && !core.CanReforge(base, &proto.ItemReforge{FromStatType: statDecrease, ToStatType: statIncrease}, reforging) {
		return nil, fmt.Sprintf("reforge of stat type %d to %d does nothing on the sim's copy of the item,"+
			" which either already has the stat or has too little to reforge away, dropped", statDecrease, statIncrease)
	}
	return &RosterReforge{FromStatType: statDecrease, ToStatType: statIncrease}, ""
}
