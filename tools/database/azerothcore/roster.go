package azerothcore

import (
	"cmp"
	"encoding/json"
	"fmt"
	"math"
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

// RosterVersion is the format version of the exported file. Version 1 files have no bot flag, pet,
// ammo or consumables.
const RosterVersion = 2

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
	// Players is -players when given: exactly these count as played, everyone else as a bot.
	Players []string `json:"players,omitempty"`
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
	Quiver bool `json:"quiver"`
	// Bot is true for a character mod-playerbots runs, whose consumables follow the bot's rules
	// rather than its bags and saved buffs.
	Bot  bool        `json:"bot"`
	Pet  *RosterPet  `json:"pet,omitempty"`
	Ammo *RosterAmmo `json:"ammo,omitempty"`
	// Consumes is keyed by the sim's Consumes field names in proto JSON. A field that isn't there
	// stays as the importer finds it.
	Consumes map[string]RosterConsume `json:"consumes"`
	Warnings []string                 `json:"warnings"`
}

// RosterPet is the pet a hunter or warlock has out. PetType and Talents are a hunter's, Summon a
// warlock's, and either is left out when the sim has no value for the family.
type RosterPet struct {
	Name       string `json:"name"`
	Family     int32  `json:"family"`
	FamilyName string `json:"familyName"`
	PetType    string `json:"petType,omitempty"` // Hunter.Options.PetType
	// Talents is the pet talent string of the family's tree, as the sim's pet talent picker writes it.
	Talents *string `json:"talents,omitempty"`
	Summon  string  `json:"summon,omitempty"` // Warlock.Options.Summon
}

// RosterAmmo is a hunter's ammo. Value is the sim ammo with the same DPS, left out when there's none.
type RosterAmmo struct {
	ItemID int32   `json:"itemId"`
	DPS    float64 `json:"dps"`
	Value  string  `json:"value,omitempty"` // Hunter.Options.Ammo
}

// RosterConsume is what one Consumes field gets.
type RosterConsume struct {
	// Value is the field's proto JSON value: an enum value name, a bool or a number. It's left out
	// when the item or buff the character has isn't one the sim has, and the field stays as it is.
	Value  any    `json:"value,omitempty"`
	Source string `json:"source"`
	// ItemID and SpellID are what the value came from: a bag item, or a buff from the saved auras or
	// the world-buff matrix.
	ItemID  int32 `json:"itemId,omitempty"`
	SpellID int32 `json:"spellId,omitempty"`
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
	ACSlotRanged   = 17
	ACSlotTabard   = 18
	ACSlotCount    = 19
)

// ACSlotBackpackStart and ACSlotBackpackEnd are the backpack's own 16 slots. Bank slots come after.
const (
	ACSlotBackpackStart = 23
	ACSlotBackpackEnd   = 38
)

// ACSlotBagStart and ACSlotBagEnd are the 4 bag slots, where a quiver or ammo pouch can sit equipped
// like any other bag.
const (
	ACSlotBagStart = 19
	ACSlotBagEnd   = 22
)

// ItemClassQuiver is item_template.class for quivers and ammo pouches.
const ItemClassQuiver = 11

// item_template.class and subclass of consumables.
const (
	ItemClassConsumable = 0
	itemSubClassPotion  = 1
	itemSubClassElixir  = 2
	itemSubClassFlask   = 3
	itemSubClassFood    = 5
)

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
	AmmoID            int32
	AmmoDPS           float64  // from the ammo's item_template damage, 0 when it has no row
	Pet               *PetRows // character_pet slot 0, the pet that's out; nil when there's none
	Auras             []int32  // saved buffs
	Bags              []BagItem
	KnowsShadowfiend  bool
	Bot               bool
	Strategies        *BotStrategies // nil when mod-playerbots saved none for the character
}

type PetRows struct {
	Name   string
	Entry  int32
	Family int32   // creature_template.family
	Spells []int32 // pet_spell, which holds its talents among its abilities
}

// BagItem is a consumable in the backpack or a bag, never the bank.
type BagItem struct {
	ItemID    int32
	SubClass  int32
	Quality   int32
	ItemLevel int32
	Spells    [5]int32 // item_template.spellid_1..5
}

// BotStrategies are the strategy lists mod-playerbots saved for a bot in playerbots_db_store.
type BotStrategies struct {
	Combat, NonCombat, Dead []string
}

// ParseStrategies splits a saved strategy list, "+dps assist,+potions".
func ParseStrategies(value string) []string {
	strategies := []string{}
	for _, strategy := range strings.Split(value, ",") {
		if strategy = strings.TrimPrefix(strings.TrimSpace(strategy), "+"); strategy != "" {
			strategies = append(strategies, strategy)
		}
	}
	return strategies
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
		classTrees, err := readTalentTreeFile(dir, name)
		if err != nil {
			return nil, err
		}
		trees[classID] = classTrees
	}
	return trees, nil
}

// PetTalentTrees maps CreatureFamily.PetTalentType to the sim's hunter pet tree for it.
type PetTalentTrees map[int32][]TalentLocation

var petTalentTreeFiles = map[int32]string{0: "hunter_ferocity", 1: "hunter_tenacity", 2: "hunter_cunning"}

// LoadPetTalentTrees reads the sim's hunter pet trees from the same directory as LoadTalentTrees.
func LoadPetTalentTrees(dir string) (PetTalentTrees, error) {
	trees := make(PetTalentTrees, len(petTalentTreeFiles))
	for talentType, name := range petTalentTreeFiles {
		file, err := readTalentTreeFile(dir, name)
		if err != nil {
			return nil, err
		}
		if len(file) != 1 {
			return nil, fmt.Errorf("%s.json has %d trees, want 1", name, len(file))
		}
		trees[talentType] = file[0]
	}
	return trees, nil
}

func readTalentTreeFile(dir, name string) ([][]TalentLocation, error) {
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

	trees := make([][]TalentLocation, len(file))
	for i, tree := range file {
		trees[i] = make([]TalentLocation, len(tree.Talents))
		for j, talent := range tree.Talents {
			trees[i][j] = talent.Location
		}
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

// AzerothCore class ids.
const (
	classWarrior     = 1
	classPaladin     = 2
	classHunter      = 3
	classRogue       = 4
	classPriest      = 5
	classDeathKnight = 6
	classShaman      = 7
	classMage        = 8
	classWarlock     = 9
	classDruid       = 11
)

// Loadout is what BuildLoadout reads besides the character's own rows.
type Loadout struct {
	DBC      *RosterDBC
	PetTrees PetTalentTrees
	// WorldBuffs is the bots' AiPlayerbot.WorldBuffMatrix. Nil means none was given, which leaves the
	// bots' flask, elixirs and food as the importer finds them.
	WorldBuffs []WorldBuff
	// AuraItems maps a flask's or elixir's buff to its item, so a buff the sim has no value for still
	// reads as a flask or elixir.
	AuraItems map[int32]AuraItem
}

type AuraItem struct {
	ItemID   int32
	SubClass int32
}

// BuildLoadout adds the character's pet, ammo and consumables, with a warning for each thing the
// sim has no value for.
func BuildLoadout(character *RosterCharacter, rows *CharacterRows, loadout Loadout) {
	character.Bot = rows.Bot
	pet, petWarnings := BuildPet(rows.ClassID, rows.Pet, loadout.DBC, loadout.PetTrees)
	ammo, ammoWarnings := BuildAmmo(rows)
	consumes := PlayerConsumes
	if rows.Bot {
		consumes = BotConsumes
	}
	var consumeWarnings []string
	character.Consumes, consumeWarnings = consumes(rows, loadout)
	character.Pet, character.Ammo = pet, ammo
	for _, warnings := range [][]string{petWarnings, ammoWarnings, consumeWarnings} {
		character.Warnings = append(character.Warnings, warnings...)
	}
}

// BuildPet reads a hunter's or warlock's pet. Other classes' pets, like a DK's ghoul, aren't sim
// options.
func BuildPet(classID int32, rows *PetRows, dbc *RosterDBC, trees PetTalentTrees) (*RosterPet, []string) {
	if classID != classHunter && classID != classWarlock {
		return nil, nil
	}
	if rows == nil {
		return nil, []string{"no pet is out (character_pet slot 0), so the pet stays as it is"}
	}
	family := dbc.Families[rows.Family]
	pet := &RosterPet{Name: rows.Name, Family: rows.Family, FamilyName: family.Name}
	unknownFamily := []string{fmt.Sprintf("pet %s is a %q (family %d), which the sim has no value for, so the pet stays as it is",
		rows.Name, family.Name, rows.Family)}

	if classID == classWarlock {
		if pet.Summon = enumByName(proto.Warlock_Options_Summon_value, family.Name, proto.Warlock_Options_NoSummon.String()); pet.Summon == "" {
			return pet, unknownFamily
		}
		return pet, nil
	}

	if pet.PetType = enumByName(proto.Hunter_Options_PetType_value, family.Name, proto.Hunter_Options_PetNone.String()); pet.PetType == "" {
		return pet, unknownFamily
	}
	tree, ok := trees[family.PetTalentType]
	tab := petTalentTab(dbc, family.PetTalentType)
	if !ok || tab == 0 {
		return pet, []string{fmt.Sprintf("pet %s's family %d has no pet talent tree, so its talents stay as they are", rows.Name, rows.Family)}
	}
	talents, points, unmatched := BuildPetTalentString(rows.Spells, tab, tree, dbc)
	pet.Talents = &talents
	var warnings []string
	for _, spell := range unmatched {
		warnings = append(warnings, fmt.Sprintf("pet talent spell %d isn't in the %s's talent tree", spell, family.Name))
	}
	if points == 0 {
		warnings = append(warnings, fmt.Sprintf("pet %s has no talents saved", rows.Name))
	}
	return pet, warnings
}

// enumByName finds the enum value named like a creature family, spaces and case aside, so "Bird of
// Prey" is BirdOfPrey. none, the enum's empty value, never matches.
func enumByName(values map[string]int32, family, none string) string {
	key := strings.ToLower(strings.ReplaceAll(family, " ", ""))
	for name := range values {
		if name != none && strings.ToLower(name) == key {
			return name
		}
	}
	return ""
}

// petTalentTab is the Talent.dbc tab of a pet talent type's tree, 0 when there's none.
func petTalentTab(dbc *RosterDBC, talentType int32) int32 {
	if talentType < 0 || talentType > 30 {
		return 0
	}
	tab := int32(0)
	for id, entry := range dbc.TalentTabs {
		if entry.PetTalentMask == 1<<talentType && (tab == 0 || id < tab) {
			tab = id
		}
	}
	return tab
}

// BuildPetTalentString turns a pet's spells into the sim's talent string for its tree: one digit per
// talent in tree order. Spells that aren't pet talents, its abilities, are skipped, and talents from
// another tree come back unmatched.
func BuildPetTalentString(spells []int32, tabID int32, tree []TalentLocation, dbc *RosterDBC) (talents string, points int32, unmatched []int32) {
	ranks := make([]int32, len(tree))
	for _, spell := range spells {
		places := dbc.PetTalents[spell]
		if len(places) == 0 {
			continue
		}
		place := slices.IndexFunc(places, func(p TalentRank) bool { return p.TabID == tabID })
		if place == -1 {
			unmatched = append(unmatched, spell)
			continue
		}
		index := slices.IndexFunc(tree, func(location TalentLocation) bool {
			return location.Row == places[place].Row && location.Col == places[place].Col
		})
		if index == -1 {
			unmatched = append(unmatched, spell)
			continue
		}
		ranks[index] = max(ranks[index], places[place].Rank)
	}

	digits := make([]byte, len(ranks))
	for i, rank := range ranks {
		digits[i] = byte('0' + rank)
		points += rank
	}
	return strings.TrimRight(string(digits), "0"), points, unmatched
}

// thoridalItemID is the one bow that makes its own arrows, so its hunter has no ammo.
const thoridalItemID = 34334

// simAmmoDPS has to match the AmmoDPS NewHunter gives each value in sim/hunter/hunter.go. On a tie
// the first one wins, which the sim treats the same anyway.
var simAmmoDPS = []struct {
	value proto.Hunter_Options_Ammo
	dps   float64
}{
	{proto.Hunter_Options_IcebladeArrow, 91.5},
	{proto.Hunter_Options_SaroniteRazorheads, 67.5},
	{proto.Hunter_Options_TerrorshaftArrow, 46.5},
	{proto.Hunter_Options_TimelessArrow, 53},
	{proto.Hunter_Options_MysteriousArrow, 46.5},
	{proto.Hunter_Options_AdamantiteStinger, 43},
	{proto.Hunter_Options_BlackflightArrow, 32},
}

// BuildAmmo maps a hunter's ammo to the sim ammo with the same DPS, arrows and bullets alike.
func BuildAmmo(rows *CharacterRows) (*RosterAmmo, []string) {
	if rows.ClassID != classHunter {
		return nil, nil
	}
	if rows.AmmoID == 0 {
		// Thori'dal needs no arrows, but a bow swapped in for it does, so the player's ammo stays
		if slices.ContainsFunc(rows.Items, func(item EquippedItem) bool {
			return item.ACSlot == ACSlotRanged && item.ItemID == thoridalItemID
		}) {
			return nil, nil
		}
		return &RosterAmmo{Value: proto.Hunter_Options_AmmoNone.String()}, []string{"no ammo equipped"}
	}
	ammo := &RosterAmmo{ItemID: rows.AmmoID, DPS: rows.AmmoDPS}
	for _, known := range simAmmoDPS {
		if math.Abs(known.dps-rows.AmmoDPS) < 0.01 {
			ammo.Value = known.value.String()
			return ammo, nil
		}
	}
	return ammo, []string{fmt.Sprintf("ammo %d adds %g DPS, which none of the sim's ammo does, so the ammo stays as it is",
		rows.AmmoID, rows.AmmoDPS)}
}

// Consumes fields, by their proto JSON names.
const (
	ConsumeFlask               = "flask"
	ConsumeBattleElixir        = "battleElixir"
	ConsumeGuardianElixir      = "guardianElixir"
	ConsumeFood                = "food"
	ConsumePetFood             = "petFood"
	ConsumePetScrollOfAgility  = "petScrollOfAgility"
	ConsumePetScrollOfStrength = "petScrollOfStrength"
	ConsumeDefaultPotion       = "defaultPotion"
	ConsumePrepopPotion        = "prepopPotion"
	ConsumeDefaultConjured     = "defaultConjured"
	ConsumeThermalSapper       = "thermalSapper"
	ConsumeExplosiveDecoy      = "explosiveDecoy"
	ConsumeFillerExplosive     = "fillerExplosive"
)

// Where a consumable came from.
const (
	SourceMatrix = "matrix" // the bot's AiPlayerbot.WorldBuffMatrix rows
	SourceBuffs  = "buffs"  // the character's saved auras
	SourceBags   = "bags"
	SourceRules  = "rules" // mod-playerbots never has the bot use one
)

var sourceNames = map[string]string{SourceMatrix: "world-buff matrix row", SourceBuffs: "saved buffs", SourceBags: "bags"}

// consumeNone is what each field holds when the character has nothing for it.
var consumeNone = map[string]any{
	ConsumeFlask:               proto.Flask_FlaskUnknown.String(),
	ConsumeBattleElixir:        proto.BattleElixir_BattleElixirUnknown.String(),
	ConsumeGuardianElixir:      proto.GuardianElixir_GuardianElixirUnknown.String(),
	ConsumeFood:                proto.Food_FoodUnknown.String(),
	ConsumePetFood:             proto.PetFood_PetFoodUnknown.String(),
	ConsumePetScrollOfAgility:  int32(0),
	ConsumePetScrollOfStrength: int32(0),
	ConsumeDefaultPotion:       proto.Potions_UnknownPotion.String(),
	ConsumePrepopPotion:        proto.Potions_UnknownPotion.String(),
	ConsumeDefaultConjured:     proto.Conjured_ConjuredUnknown.String(),
	ConsumeThermalSapper:       false,
	ConsumeExplosiveDecoy:      false,
	ConsumeFillerExplosive:     proto.Explosive_ExplosiveUnknown.String(),
}

type potionKind int

const (
	potionOffensive potionKind = iota + 1
	potionDefensive
	potionMana
	potionHealing
)

// simConsumable is one value of a Consumes field and the items that give it. aura is the buff the
// item leaves, which is what character_aura and the world-buff matrix hold.
type simConsumable struct {
	field  string
	value  any
	items  []int32
	aura   int32
	potion potionKind
	prepop bool  // the sim offers it as a pre-pull potion too
	class  int32 // the only class that can use it, 0 for any
}

func flask(value proto.Flask, item, aura int32) simConsumable {
	return simConsumable{field: ConsumeFlask, value: value.String(), items: []int32{item}, aura: aura}
}

func battleElixir(value proto.BattleElixir, item, aura int32) simConsumable {
	return simConsumable{field: ConsumeBattleElixir, value: value.String(), items: []int32{item}, aura: aura}
}

func guardianElixir(value proto.GuardianElixir, item, aura int32) simConsumable {
	return simConsumable{field: ConsumeGuardianElixir, value: value.String(), items: []int32{item}, aura: aura}
}

func food(value proto.Food, item, aura int32) simConsumable {
	return simConsumable{field: ConsumeFood, value: value.String(), items: []int32{item}, aura: aura}
}

func potion(value proto.Potions, item int32, kind potionKind, prepop bool) simConsumable {
	return simConsumable{field: ConsumeDefaultPotion, value: value.String(), items: []int32{item}, potion: kind, prepop: prepop}
}

// simConsumables are the sim's consumables. The items are the ones the sim's consumable pickers show,
// plus other ranks of the same thing, and within a field the list runs best first.
var simConsumables = []simConsumable{
	flask(proto.Flask_FlaskOfTheFrostWyrm, 46376, 53755),
	flask(proto.Flask_FlaskOfEndlessRage, 46377, 53760),
	flask(proto.Flask_FlaskOfPureMojo, 46378, 54212),
	flask(proto.Flask_FlaskOfStoneblood, 46379, 53758),
	flask(proto.Flask_LesserFlaskOfToughness, 40079, 53752),
	flask(proto.Flask_LesserFlaskOfResistance, 44939, 62380),
	flask(proto.Flask_FlaskOfBlindingLight, 22861, 28521),
	flask(proto.Flask_FlaskOfMightyRestoration, 22853, 28519),
	flask(proto.Flask_FlaskOfPureDeath, 22866, 28540),
	flask(proto.Flask_FlaskOfRelentlessAssault, 22854, 28520),
	flask(proto.Flask_FlaskOfSupremePower, 13512, 17628),
	flask(proto.Flask_FlaskOfFortification, 22851, 28518),
	flask(proto.Flask_FlaskOfChromaticWonder, 33208, 42735),

	battleElixir(proto.BattleElixir_ElixirOfAccuracy, 44325, 60340),
	battleElixir(proto.BattleElixir_ElixirOfArmorPiercing, 44330, 60345),
	battleElixir(proto.BattleElixir_ElixirOfDeadlyStrikes, 44327, 60341),
	battleElixir(proto.BattleElixir_ElixirOfExpertise, 44329, 60344),
	battleElixir(proto.BattleElixir_ElixirOfLightningSpeed, 44331, 60346),
	battleElixir(proto.BattleElixir_ElixirOfMightyAgility, 39666, 28497),
	battleElixir(proto.BattleElixir_ElixirOfMightyStrength, 40073, 53748),
	battleElixir(proto.BattleElixir_GurusElixir, 40076, 53749),
	battleElixir(proto.BattleElixir_SpellpowerElixir, 40070, 33721),
	battleElixir(proto.BattleElixir_WrathElixir, 40068, 53746),
	battleElixir(proto.BattleElixir_AdeptsElixir, 28103, 54452),
	battleElixir(proto.BattleElixir_ElixirOfDemonslaying, 9224, 11406),
	battleElixir(proto.BattleElixir_ElixirOfMajorAgility, 22831, 54494),
	battleElixir(proto.BattleElixir_ElixirOfMajorFirePower, 22833, 28501),
	battleElixir(proto.BattleElixir_ElixirOfMajorFrostPower, 22827, 28493),
	battleElixir(proto.BattleElixir_ElixirOfMajorShadowPower, 22835, 28503),
	battleElixir(proto.BattleElixir_ElixirOfMajorStrength, 22824, 28490),
	battleElixir(proto.BattleElixir_ElixirOfMastery, 28104, 33726),
	battleElixir(proto.BattleElixir_ElixirOfTheMongoose, 13452, 17538),
	battleElixir(proto.BattleElixir_FelStrengthElixir, 31679, 38954),
	battleElixir(proto.BattleElixir_GreaterArcaneElixir, 13454, 17539),

	guardianElixir(proto.GuardianElixir_ElixirOfMightyDefense, 44328, 60343),
	guardianElixir(proto.GuardianElixir_ElixirOfMightyFortitude, 40078, 53751),
	guardianElixir(proto.GuardianElixir_ElixirOfMightyMageblood, 40109, 53764),
	guardianElixir(proto.GuardianElixir_ElixirOfMightyThoughts, 44332, 60347),
	guardianElixir(proto.GuardianElixir_ElixirOfProtection, 40097, 53763),
	guardianElixir(proto.GuardianElixir_ElixirOfSpirit, 40072, 53747),
	guardianElixir(proto.GuardianElixir_GiftOfArthas, 9088, 11371),
	guardianElixir(proto.GuardianElixir_ElixirOfDraenicWisdom, 32067, 39627),
	guardianElixir(proto.GuardianElixir_ElixirOfIronskin, 32068, 39628),
	guardianElixir(proto.GuardianElixir_ElixirOfMajorDefense, 22834, 28502),
	guardianElixir(proto.GuardianElixir_ElixirOfMajorFortitude, 32062, 39625),
	guardianElixir(proto.GuardianElixir_ElixirOfMajorMageblood, 22840, 28509),

	// the feasts' buffs come from the table they set down, not the item's own spell
	food(proto.Food_FoodFishFeast, 43015, 57399),
	food(proto.Food_FoodGreatFeast, 34753, 57294),
	food(proto.Food_FoodBlackenedDragonfin, 42999, 57367),
	food(proto.Food_FoodHeartyRhino, 42995, 57358),
	food(proto.Food_FoodMegaMammothMeal, 34754, 57325),
	food(proto.Food_FoodSpicedWormBurger, 34756, 57329),
	food(proto.Food_FoodRhinoliciousWormsteak, 42994, 57356),
	food(proto.Food_FoodImperialMantaSteak, 34769, 57332),
	food(proto.Food_FoodSnapperExtreme, 42996, 57360),
	food(proto.Food_FoodMightyRhinoDogs, 34758, 57334),
	food(proto.Food_FoodFirecrackerSalmon, 34767, 57327),
	food(proto.Food_FoodCuttlesteak, 42998, 57365),
	food(proto.Food_FoodDragonfinFilet, 43000, 57371),
	food(proto.Food_FoodBlackenedBasilisk, 27657, 33263),
	food(proto.Food_FoodGrilledMudfish, 27664, 33261),
	food(proto.Food_FoodRavagerDog, 27655, 33259),
	food(proto.Food_FoodRoastedClefthoof, 27658, 33256),
	food(proto.Food_FoodSpicyHotTalbuk, 33872, 43764),
	food(proto.Food_FoodSkullfishSoup, 33825, 43722),
	food(proto.Food_FoodFishermansFeast, 33052, 33257),

	potion(proto.Potions_PotionOfSpeed, 40211, potionOffensive, true),
	potion(proto.Potions_PotionOfWildMagic, 40212, potionOffensive, true),
	potion(proto.Potions_HastePotion, 22838, potionOffensive, false),
	potion(proto.Potions_DestructionPotion, 22839, potionOffensive, false),
	potion(proto.Potions_InsaneStrengthPotion, 22828, potionOffensive, true),
	potion(proto.Potions_HeroicPotion, 22837, potionOffensive, true),
	potion(proto.Potions_MightyRagePotion, 13442, potionOffensive, false),
	potion(proto.Potions_IndestructiblePotion, 40093, potionDefensive, true),
	potion(proto.Potions_IronshieldPotion, 22849, potionDefensive, false),
	potion(proto.Potions_RunicManaInjector, 42545, potionMana, false),
	potion(proto.Potions_RunicManaPotion, 33448, potionMana, false),
	potion(proto.Potions_FelManaPotion, 31677, potionMana, false),
	potion(proto.Potions_SuperManaPotion, 22832, potionMana, false),
	potion(proto.Potions_RunicHealingInjector, 41166, potionHealing, false),
	potion(proto.Potions_RunicHealingPotion, 33447, potionHealing, false),

	// Dark and Demonic Runes, the Fel and Master Healthstones; the highest item id goes first
	{field: ConsumeDefaultConjured, value: proto.Conjured_ConjuredDarkRune.String(), items: []int32{20520, 12662}},
	{field: ConsumeDefaultConjured, value: proto.Conjured_ConjuredFlameCap.String(), items: []int32{22788}},
	{field: ConsumeDefaultConjured, value: proto.Conjured_ConjuredRogueThistleTea.String(), items: []int32{7676}, class: classRogue},
	{field: ConsumeDefaultConjured, value: proto.Conjured_ConjuredHealthstone.String(),
		items: []int32{36894, 36893, 36892, 22105, 22104, 22103}},

	{field: ConsumeFillerExplosive, value: proto.Explosive_ExplosiveSaroniteBomb.String(), items: []int32{41119}},
	{field: ConsumeFillerExplosive, value: proto.Explosive_ExplosiveCobaltFragBomb.String(), items: []int32{40771}},
	{field: ConsumeThermalSapper, value: true, items: []int32{42641}},
	{field: ConsumeExplosiveDecoy, value: true, items: []int32{40536}},

	{field: ConsumePetFood, value: proto.PetFood_PetFoodSpicedMammothTreats.String(), items: []int32{43005}},
	{field: ConsumePetFood, value: proto.PetFood_PetFoodKiblersBits.String(), items: []int32{33874}},
}

var consumableByItem, consumableByAura = func() (map[int32]*simConsumable, map[int32]*simConsumable) {
	byItem, byAura := map[int32]*simConsumable{}, map[int32]*simConsumable{}
	for i := range simConsumables {
		consumable := &simConsumables[i]
		for _, item := range consumable.items {
			byItem[item] = consumable
		}
		if consumable.aura != 0 {
			byAura[consumable.aura] = consumable
		}
	}
	return byItem, byAura
}()

// candidate is a flask, elixir or food the character has. known is nil when the sim has no value
// for it.
type candidate struct {
	known   *simConsumable
	itemID  int32
	spellID int32
}

func (c candidate) String() string {
	switch {
	case c.itemID != 0 && c.spellID != 0:
		return fmt.Sprintf("spell %d (item %d)", c.spellID, c.itemID)
	case c.itemID != 0:
		return fmt.Sprintf("item %d", c.itemID)
	}
	return fmt.Sprintf("spell %d", c.spellID)
}

func (c candidate) consume(source string) RosterConsume {
	consume := RosterConsume{Source: source, ItemID: c.itemID, SpellID: c.spellID}
	if c.known != nil {
		consume.Value = c.known.value
	}
	return consume
}

// buffCandidates sorts what a character has into the flask, elixir and food fields. The other
// lists hold the ones the sim has no value for; an elixir's kind isn't known then.
type buffCandidates struct {
	flasks, battleElixirs, guardianElixirs, foods []candidate
	otherFlasks, otherElixirs, otherFoods         []candidate
}

func (c *buffCandidates) addKnown(found candidate) {
	switch found.known.field {
	case ConsumeFlask:
		c.flasks = append(c.flasks, found)
	case ConsumeBattleElixir:
		c.battleElixirs = append(c.battleElixirs, found)
	case ConsumeGuardianElixir:
		c.guardianElixirs = append(c.guardianElixirs, found)
	case ConsumeFood:
		c.foods = append(c.foods, found)
	}
}

func (c *buffCandidates) hasFlaskOrElixir() bool {
	return len(c.flasks)+len(c.battleElixirs)+len(c.guardianElixirs)+len(c.otherFlasks)+len(c.otherElixirs) > 0
}

func (c *buffCandidates) hasFood() bool {
	return len(c.foods)+len(c.otherFoods) > 0
}

// auraCandidates sorts buffs, saved ones or the world-buff matrix's. The rest are buffs that are
// none of the three.
func auraCandidates(auras []int32, loadout Loadout) (candidates buffCandidates, rest []int32) {
	for _, aura := range auras {
		if known := consumableByAura[aura]; known != nil {
			candidates.addKnown(candidate{known: known, itemID: known.items[0], spellID: aura})
			continue
		}
		item, isItem := loadout.AuraItems[aura]
		switch {
		case isItem && item.SubClass == itemSubClassFlask:
			candidates.otherFlasks = append(candidates.otherFlasks, candidate{itemID: item.ItemID, spellID: aura})
		case isItem && item.SubClass == itemSubClassElixir:
			candidates.otherElixirs = append(candidates.otherElixirs, candidate{itemID: item.ItemID, spellID: aura})
		case loadout.DBC.WellFedAuras[aura]:
			candidates.otherFoods = append(candidates.otherFoods, candidate{spellID: aura})
		default:
			rest = append(rest, aura)
		}
	}
	return candidates, rest
}

// bagCandidates sorts the flasks, elixirs and buff food in the bags, highest item level first. Food
// that leaves no Well Fed buff doesn't count.
func bagCandidates(bags []BagItem, dbc *RosterDBC) buffCandidates {
	var candidates buffCandidates
	for _, item := range slices.SortedFunc(slices.Values(bags), func(a, b BagItem) int {
		return cmp.Or(cmp.Compare(b.ItemLevel, a.ItemLevel), cmp.Compare(a.ItemID, b.ItemID))
	}) {
		if known := consumableByItem[item.ItemID]; known != nil {
			candidates.addKnown(candidate{known: known, itemID: item.ItemID})
			continue
		}
		switch item.SubClass {
		case itemSubClassFlask:
			candidates.otherFlasks = append(candidates.otherFlasks, candidate{itemID: item.ItemID})
		case itemSubClassElixir:
			candidates.otherElixirs = append(candidates.otherElixirs, candidate{itemID: item.ItemID})
		case itemSubClassFood:
			for _, spell := range item.Spells {
				if buff := dbc.FoodBuffs[spell]; buff != 0 {
					candidates.otherFoods = append(candidates.otherFoods, candidate{itemID: item.ItemID, spellID: buff})
					break
				}
			}
		}
	}
	return candidates
}

func noConsume(field, source string) RosterConsume {
	return RosterConsume{Value: consumeNone[field], Source: source}
}

// setFlaskAndElixirs fills the flask and both elixirs. A flask takes both elixir slots, so it wins
// and leaves them empty.
func setFlaskAndElixirs(consumes map[string]RosterConsume, candidates buffCandidates, source string) []string {
	switch {
	case len(candidates.flasks) > 0:
		consumes[ConsumeFlask] = candidates.flasks[0].consume(source)
	case len(candidates.otherFlasks) > 0:
		consumes[ConsumeFlask] = candidates.otherFlasks[0].consume(source)
	default:
		consumes[ConsumeFlask] = noConsume(ConsumeFlask, source)
		kept := false
		for _, elixir := range []struct {
			field string
			known []candidate
		}{{ConsumeBattleElixir, candidates.battleElixirs}, {ConsumeGuardianElixir, candidates.guardianElixirs}} {
			switch {
			case len(elixir.known) > 0:
				consumes[elixir.field] = elixir.known[0].consume(source)
			case len(candidates.otherElixirs) > 0:
				consumes[elixir.field] = candidates.otherElixirs[0].consume(source)
				kept = true
			default:
				consumes[elixir.field] = noConsume(elixir.field, source)
			}
		}
		if kept {
			return []string{fmt.Sprintf("the elixir from its %s, %s, isn't one the sim has, so the elixir it could be stays as it is",
				sourceNames[source], candidates.otherElixirs[0])}
		}
		return nil
	}

	consumes[ConsumeBattleElixir] = noConsume(ConsumeBattleElixir, source)
	consumes[ConsumeGuardianElixir] = noConsume(ConsumeGuardianElixir, source)
	if len(candidates.flasks) == 0 {
		return []string{fmt.Sprintf("the flask from its %s, %s, isn't one the sim has, so the flask stays as it is",
			sourceNames[source], candidates.otherFlasks[0])}
	}
	return nil
}

func setFood(consumes map[string]RosterConsume, candidates buffCandidates, source string) []string {
	switch {
	case len(candidates.foods) > 0:
		consumes[ConsumeFood] = candidates.foods[0].consume(source)
	case len(candidates.otherFoods) > 0:
		consumes[ConsumeFood] = candidates.otherFoods[0].consume(source)
		return []string{fmt.Sprintf("the food from its %s, %s, isn't one the sim has, so the food stays as it is",
			sourceNames[source], candidates.otherFoods[0])}
	default:
		consumes[ConsumeFood] = noConsume(ConsumeFood, source)
	}
	return nil
}

// PlayerConsumes reads a played character's consumables. Flask, elixirs and food come from its
// saved buffs, else its bags; everything else only from its bags. Pet scrolls aren't read.
func PlayerConsumes(rows *CharacterRows, loadout Loadout) (map[string]RosterConsume, []string) {
	consumes := map[string]RosterConsume{}
	var warnings []string
	buffs, _ := auraCandidates(rows.Auras, loadout)
	bags := bagCandidates(rows.Bags, loadout.DBC)
	if buffs.hasFlaskOrElixir() {
		warnings = append(warnings, setFlaskAndElixirs(consumes, buffs, SourceBuffs)...)
	} else {
		warnings = append(warnings, setFlaskAndElixirs(consumes, bags, SourceBags)...)
	}
	if buffs.hasFood() {
		warnings = append(warnings, setFood(consumes, buffs, SourceBuffs)...)
	} else {
		warnings = append(warnings, setFood(consumes, bags, SourceBags)...)
	}

	var potionWarnings []string
	consumes[ConsumeDefaultPotion], potionWarnings = playerPotion(rows)
	warnings = append(warnings, potionWarnings...)
	consumes[ConsumePrepopPotion] = playerPrepopPotion(rows)
	for _, field := range []string{ConsumeDefaultConjured, ConsumeFillerExplosive, ConsumeThermalSapper, ConsumeExplosiveDecoy, ConsumePetFood} {
		consumes[field] = firstInBags(rows, field)
	}
	return consumes, warnings
}

// botItemOrder is mod-playerbots' compare_items (InventoryAction.cpp), how a bot ranks what its bags
// hold: potions before flasks, lower quality first, then higher item level. It leaves ties
// unordered; the item id settles them here.
func botItemOrder(a, b BagItem) int {
	return cmp.Or(botItemRank(a, b), cmp.Compare(a.ItemID, b.ItemID))
}

func botItemRank(a, b BagItem) int {
	return cmp.Or(cmp.Compare(a.SubClass, b.SubClass), cmp.Compare(a.Quality, b.Quality), cmp.Compare(b.ItemLevel, a.ItemLevel))
}

func sortedBag(bags []BagItem, keep func(BagItem) bool) []BagItem {
	var kept []BagItem
	for _, item := range bags {
		if keep(item) {
			kept = append(kept, item)
		}
	}
	slices.SortFunc(kept, botItemOrder)
	return kept
}

func knownPotion(kind potionKind, prepop bool) func(BagItem) bool {
	return func(item BagItem) bool {
		known := consumableByItem[item.ItemID]
		return known != nil && known.field == ConsumeDefaultPotion && known.potion == kind && (known.prepop || !prepop)
	}
}

// playerPotion takes an offensive potion from the bags, else a defensive one, a mana potion for a
// class with mana, or a healing potion.
func playerPotion(rows *CharacterRows) (RosterConsume, []string) {
	kinds := []potionKind{potionOffensive, potionDefensive}
	if hasMana(rows.ClassID) {
		kinds = append(kinds, potionMana)
	}
	for _, kind := range append(kinds, potionHealing) {
		if potions := sortedBag(rows.Bags, knownPotion(kind, false)); len(potions) > 0 {
			return RosterConsume{Value: consumableByItem[potions[0].ItemID].value, Source: SourceBags, ItemID: potions[0].ItemID}, nil
		}
	}
	others := sortedBag(rows.Bags, func(item BagItem) bool {
		return item.SubClass == itemSubClassPotion && consumableByItem[item.ItemID] == nil
	})
	if len(others) > 0 {
		ids := make([]string, len(others))
		for i, item := range others {
			ids[i] = strconv.Itoa(int(item.ItemID))
		}
		return RosterConsume{Source: SourceBags, ItemID: others[0].ItemID},
			[]string{fmt.Sprintf("the bags hold only potions the sim doesn't have (%s), so the potion stays as it is", strings.Join(ids, ", "))}
	}
	return noConsume(ConsumeDefaultPotion, SourceBags), nil
}

func playerPrepopPotion(rows *CharacterRows) RosterConsume {
	for _, kind := range []potionKind{potionOffensive, potionDefensive} {
		if potions := sortedBag(rows.Bags, knownPotion(kind, true)); len(potions) > 0 {
			return RosterConsume{Value: consumableByItem[potions[0].ItemID].value, Source: SourceBags, ItemID: potions[0].ItemID}
		}
	}
	return noConsume(ConsumePrepopPotion, SourceBags)
}

// firstInBags is the field's first sim value, in simConsumables order, that the bags hold an item for.
func firstInBags(rows *CharacterRows, field string) RosterConsume {
	for _, consumable := range simConsumables {
		if consumable.field != field || (consumable.class != 0 && consumable.class != rows.ClassID) {
			continue
		}
		for _, item := range consumable.items {
			if slices.ContainsFunc(rows.Bags, func(bagged BagItem) bool { return bagged.ItemID == item }) {
				return RosterConsume{Value: consumable.value, Source: SourceBags, ItemID: item}
			}
		}
	}
	return noConsume(field, SourceBags)
}

// botNeverFields are what mod-playerbots never has a bot use: no pre-pot, conjured items,
// explosives, pet food or scrolls.
var botNeverFields = []string{ConsumePrepopPotion, ConsumeDefaultConjured, ConsumeThermalSapper, ConsumeExplosiveDecoy,
	ConsumeFillerExplosive, ConsumePetFood, ConsumePetScrollOfAgility, ConsumePetScrollOfStrength}

var worldBuffFields = []string{ConsumeFlask, ConsumeBattleElixir, ConsumeGuardianElixir, ConsumeFood}

// BotConsumes reads a bot's consumables the way mod-playerbots hands them out: flask, elixirs and
// food from its world-buff matrix rows, which the worldbuff strategy casts on it, and a potion from
// its bags.
func BotConsumes(rows *CharacterRows, loadout Loadout) (map[string]RosterConsume, []string) {
	consumes := map[string]RosterConsume{}
	var warnings []string
	if rows.Strategies == nil {
		warnings = append(warnings, "mod-playerbots saved no strategies for this bot, so it's taken to run worldbuff and potions")
	}
	tab := specTab(rows, loadout.DBC)

	switch {
	case !rows.Strategies.runsNonCombat("worldbuff"):
		for _, field := range worldBuffFields {
			consumes[field] = noConsume(field, SourceRules)
		}
	case loadout.WorldBuffs == nil:
		for _, field := range worldBuffFields {
			consumes[field] = RosterConsume{Source: SourceMatrix}
		}
	default:
		spec := worldBuffSpec(rows, tab)
		spells := MatrixSpells(loadout.WorldBuffs, faction(rows.RaceID), rows.ClassID, rows.Level, spec)
		if len(spells) == 0 {
			warnings = append(warnings, fmt.Sprintf("no world-buff matrix row is for its class, spec %d and level, so it has no flask, elixir or food", spec))
		}
		candidates, rest := auraCandidates(spells, loadout)
		for _, spell := range rest {
			warnings = append(warnings, fmt.Sprintf("world-buff matrix spell %d isn't a flask, elixir or food, so the sim leaves it out", spell))
		}
		warnings = append(warnings, setFlaskAndElixirs(consumes, candidates, SourceMatrix)...)
		warnings = append(warnings, setFood(consumes, candidates, SourceMatrix)...)
	}

	var potionWarnings []string
	consumes[ConsumeDefaultPotion], potionWarnings = botPotion(rows, loadout.DBC, tab)
	warnings = append(warnings, potionWarnings...)
	for _, field := range botNeverFields {
		consumes[field] = noConsume(field, SourceRules)
	}
	return consumes, warnings
}

// botOffensivePotions is mod-playerbots' OFFENSIVE_POTION_IDS (PlayerbotAI.h), the only potions a bot
// drinks for damage.
var botOffensivePotions = map[int32]bool{22838: true, 22828: true, 22839: true, 40211: true, 40212: true}

// botPotion is the one potion the bot drinks in a fight, since they share a cooldown: the offensive
// one when it's a DPS, else a mana potion when its class drinks them, as UsePotionsStrategy does.
func botPotion(rows *CharacterRows, dbc *RosterDBC, tab int32) (RosterConsume, []string) {
	offensive := botIsDps(rows, tab)
	mana := hasMana(rows.ClassID) && !skipsManaPotions(rows, tab)
	if !rows.Strategies.runsCombat("potions") || (!offensive && !mana) {
		return noConsume(ConsumeDefaultPotion, SourceRules), nil
	}

	var potions []BagItem
	if offensive {
		potions = sortedBag(rows.Bags, func(item BagItem) bool { return botOffensivePotions[item.ItemID] })
	}
	if len(potions) == 0 && mana {
		potions = sortedBag(rows.Bags, func(item BagItem) bool { return isManaPotion(item, dbc) })
	}
	if len(potions) == 0 {
		return noConsume(ConsumeDefaultPotion, SourceBags), nil
	}

	picked := potions[0]
	var warnings []string
	if len(potions) > 1 && botItemRank(picked, potions[1]) == 0 {
		warnings = append(warnings, fmt.Sprintf("the bags hold potions %d and %d, which mod-playerbots ranks the same, so it could drink either; took %d",
			picked.ItemID, potions[1].ItemID, picked.ItemID))
	}
	known := consumableByItem[picked.ItemID]
	if known == nil || known.field != ConsumeDefaultPotion {
		return RosterConsume{Source: SourceBags, ItemID: picked.ItemID},
			append(warnings, fmt.Sprintf("the potion it drinks, item %d, isn't one the sim has, so the potion stays as it is", picked.ItemID))
	}
	return RosterConsume{Value: known.value, Source: SourceBags, ItemID: picked.ItemID}, warnings
}

// isManaPotion is mod-playerbots' FindPotionVisitor for mana: a potion or flask with an energize
// effect, reading its spells in order up to the first empty one.
func isManaPotion(item BagItem, dbc *RosterDBC) bool {
	if item.SubClass != itemSubClassPotion && item.SubClass != itemSubClassFlask {
		return false
	}
	for _, spell := range item.Spells {
		if spell == 0 {
			return false
		}
		if dbc.ManaSpells[spell] {
			return true
		}
	}
	return false
}

// runsCombat and runsNonCombat say whether the bot runs a strategy in or out of combat. A bot with
// nothing saved is taken to run it.
func (s *BotStrategies) runsCombat(strategy string) bool {
	return s == nil || slices.Contains(s.Combat, strategy)
}

func (s *BotStrategies) runsNonCombat(strategy string) bool {
	return s == nil || slices.Contains(s.NonCombat, strategy)
}

// dpsStrategies are the strategies whose type has STRATEGY_TYPE_DPS, which is how PlayerbotAI::IsDps
// tells a bot's role: any class can run the first ones, the rest are per class. Healers running dps
// assist count, a cat druid doesn't count through its cat strategy, and a warlock's tank strategy
// does, since it's built on the warlock DPS one.
var dpsStrategies = map[int32][]string{
	0:                {"dps assist", "grind"},
	classWarrior:     {"arms", "fury"},
	classPaladin:     {"dps", "offheal"},
	classHunter:      {"bm", "mm", "surv"},
	classRogue:       {"combat", "assassin", "subtlety"},
	classPriest:      {"shadow", "dps", "holy dps"},
	classDeathKnight: {"frost", "unholy"},
	classShaman:      {"ele", "caster", "enh", "melee", "dps"},
	classMage:        {"arcane", "fire", "frost", "frostfire"},
	classWarlock:     {"affli", "demo", "destro", "tank"},
	classDruid:       {"balance"},
}

// botIsDps is PlayerbotAI::IsDps: from the bot's strategies, or its spec when none are saved.
func botIsDps(rows *CharacterRows, tab int32) bool {
	if rows.Strategies == nil {
		return isDpsSpec(rows, tab)
	}
	for _, list := range [][]string{rows.Strategies.Combat, rows.Strategies.NonCombat, rows.Strategies.Dead} {
		for _, strategy := range list {
			if slices.Contains(dpsStrategies[0], strategy) || slices.Contains(dpsStrategies[rows.ClassID], strategy) {
				return true
			}
		}
	}
	return false
}

// Talent spells mod-playerbots checks by id, each the rank it needs.
const (
	thickHideRank3    = 16931
	bladeBarrierRank5 = 55226
	shamanisticRage   = 30823
)

// defaultSpecTabs is the tab AiFactory gives a class below level 10 or with no talents: frost mage,
// retribution paladin, holy priest, demonology warlock, frost DK, and the first tree for the rest.
var defaultSpecTabs = map[int32]int32{classMage: 2, classPaladin: 2, classPriest: 1, classWarlock: 1, classDeathKnight: 1}

// specTab is AiFactory::GetPlayerSpecTab: the tree with the most points, the first one on a tie.
func specTab(rows *CharacterRows, dbc *RosterDBC) int32 {
	var points [3]int32
	for _, spell := range rows.TalentSpells {
		talent, ok := dbc.TalentRanks[spell]
		if !ok {
			continue
		}
		tab, ok := dbc.TalentTabs[talent.TabID]
		if ok && tab.ClassMask&(1<<(rows.ClassID-1)) != 0 && tab.TabPage >= 0 && tab.TabPage < 3 {
			points[tab.TabPage] += talent.Rank
		}
	}
	if rows.Level < 10 || points[0]+points[1]+points[2] == 0 {
		return defaultSpecTabs[rows.ClassID]
	}
	best := int32(0)
	for tab := int32(1); tab < 3; tab++ {
		if points[tab] > points[best] {
			best = tab
		}
	}
	return best
}

// worldBuffSpec is the spec the matrix goes by (WorldBuffAction): the spec tab, except that a feral
// druid without Thick Hide rank 3 is a cat and a blood DK without Blade Barrier rank 5 is DPS, both
// spec 3.
func worldBuffSpec(rows *CharacterRows, tab int32) int32 {
	switch {
	case rows.ClassID == classDruid && tab == 1 && !slices.Contains(rows.TalentSpells, thickHideRank3):
		return 3
	case rows.ClassID == classDeathKnight && tab == 0 && !slices.Contains(rows.TalentSpells, bladeBarrierRank5):
		return 3
	}
	return tab
}

// isHealSpec, isTankSpec and isDpsSpec are PlayerbotAI::IsHeal, IsTank and IsDps by spec.
func isHealSpec(classID, tab int32) bool {
	switch classID {
	case classPriest:
		return tab == 0 || tab == 1
	case classDruid, classShaman:
		return tab == 2
	case classPaladin:
		return tab == 0
	}
	return false
}

func isTankSpec(rows *CharacterRows, tab int32) bool {
	switch rows.ClassID {
	case classDeathKnight:
		return tab == 0
	case classPaladin:
		return tab == 1
	case classWarrior:
		return tab == 2
	case classDruid:
		return tab == 1 && slices.Contains(rows.TalentSpells, thickHideRank3)
	}
	return false
}

func isDpsSpec(rows *CharacterRows, tab int32) bool {
	switch rows.ClassID {
	case classMage, classWarlock, classHunter, classRogue:
		return true
	case classPriest:
		return tab == 2
	case classDruid:
		return tab == 0 || (tab == 1 && !isTankSpec(rows, tab))
	case classShaman:
		return tab != 2
	case classPaladin:
		return tab == 2
	case classDeathKnight:
		return tab != 0
	case classWarrior:
		return tab != 2
	}
	return false
}

// skipsManaPotions is SkipsManaPotions (PlayerbotAI.cpp): classes that carry their own mana spend the
// shared potion cooldown on damage instead.
func skipsManaPotions(rows *CharacterRows, tab int32) bool {
	switch rows.ClassID {
	case classHunter:
		return rows.Level >= 5
	case classWarlock:
		return rows.Level >= 6
	case classMage:
		return rows.Level >= 20
	case classPriest:
		return !isHealSpec(classPriest, tab) && rows.KnowsShadowfiend
	case classShaman:
		if isHealSpec(classShaman, tab) {
			return false
		}
		if tab == 1 {
			return slices.Contains(rows.TalentSpells, shamanisticRage)
		}
		return rows.Level >= 60
	}
	return false
}

// hasMana says whether the class has a mana bar, the only thing a bot's mana potion waits for.
func hasMana(classID int32) bool {
	return classID != classWarrior && classID != classRogue && classID != classDeathKnight
}

var allianceRaces = []int32{1, 3, 4, 7, 11}

// faction is the matrix's: 1 for the Alliance, 2 for the Horde.
func faction(raceID int32) int32 {
	if slices.Contains(allianceRaces, raceID) {
		return 1
	}
	return 2
}

// WorldBuff is one spell of an AiPlayerbot.WorldBuffMatrix entry. A faction, class or level of 0
// matches any.
type WorldBuff struct {
	Faction, Class, Spec, MinLevel, MaxLevel, Spell int32
}

// MatrixSpells is WorldBuffAction::NeedWorldBuffs: the spells of every entry for the bot.
func MatrixSpells(matrix []WorldBuff, faction, classID, level, spec int32) []int32 {
	var spells []int32
	for _, buff := range matrix {
		if (buff.Faction != 0 && buff.Faction != faction) || (buff.Class != 0 && buff.Class != classID) ||
			(buff.MinLevel != 0 && buff.MinLevel > level) || (buff.MaxLevel != 0 && buff.MaxLevel < level) ||
			buff.Spec != spec || slices.Contains(spells, buff.Spell) {
			continue
		}
		spells = append(spells, buff.Spell)
	}
	return spells
}

// ParseWorldBuffMatrix reads the matrix as mod-playerbots' loadWorldBuff does: entries split on ';',
// each "<label>:faction,class,spec,minLevel,maxLevel:spell,spell". The entries it would log and skip
// come back as problems.
func ParseWorldBuffMatrix(matrix string) (buffs []WorldBuff, problems []string) {
	for _, entry := range strings.Split(matrix, ";") {
		if entry = strings.TrimSpace(entry); entry == "" {
			continue
		}
		parts := strings.SplitN(entry, ":", 3)
		if len(parts) < 3 {
			problems = append(problems, fmt.Sprintf("malformed entry %q", entry))
			continue
		}
		var meta []int32
		for _, token := range strings.Split(parts[1], ",") {
			value, ok := leadingInt(token)
			if !ok {
				break
			}
			meta = append(meta, value)
		}
		if len(meta) != 5 {
			problems = append(problems, fmt.Sprintf("entry %q needs faction,class,spec,minLevel,maxLevel", entry))
			continue
		}
		for _, token := range strings.Split(parts[2], ",") {
			if strings.TrimSpace(token) == "" {
				continue
			}
			spell, ok := leadingInt(token)
			if !ok {
				problems = append(problems, fmt.Sprintf("entry %q has a spell that isn't a number, %q", entry, token))
				continue
			}
			buffs = append(buffs, WorldBuff{Faction: meta[0], Class: meta[1], Spec: meta[2], MinLevel: meta[3],
				MaxLevel: meta[4], Spell: spell})
		}
	}
	return buffs, problems
}

// leadingInt reads a number the way std::stoi does: leading whitespace skipped, anything after the
// digits ignored.
func leadingInt(token string) (int32, bool) {
	token = strings.TrimLeft(token, " \t\r\n\v\f")
	end := 0
	if end < len(token) && (token[end] == '+' || token[end] == '-') {
		end++
	}
	digits := end
	for end < len(token) && token[end] >= '0' && token[end] <= '9' {
		end++
	}
	if end == digits {
		return 0, false
	}
	value, err := strconv.ParseInt(token[:end], 10, 32)
	return int32(value), err == nil
}

// worldBuffMatrixKeys name the setting in a docker env file and in playerbots.conf.
var worldBuffMatrixKeys = []string{"AC_AI_PLAYERBOT_WORLD_BUFF_MATRIX", "AiPlayerbot.WorldBuffMatrix"}

// WorldBuffMatrixSetting pulls the matrix out of a config file, a quoted value running over several
// lines included. A file without the setting is taken to be the bare matrix.
func WorldBuffMatrixSetting(text string) string {
	for start := 0; start < len(text); {
		end := strings.IndexByte(text[start:], '\n')
		if end < 0 {
			end = len(text)
		} else {
			end += start
		}
		line := strings.TrimLeft(text[start:end], " \t")
		for _, key := range worldBuffMatrixKeys {
			rest := strings.TrimLeft(strings.TrimPrefix(line, key), " \t")
			if !strings.HasPrefix(line, key) || rest == "" || (rest[0] != '=' && rest[0] != ':') {
				continue
			}
			value := strings.TrimLeft(text[end-len(rest)+1:], " \t")
			if value != "" && (value[0] == '"' || value[0] == '\'') {
				if closing := strings.IndexByte(value[1:], value[0]); closing >= 0 {
					return value[1 : 1+closing]
				}
				return value[1:]
			}
			if newline := strings.IndexByte(value, '\n'); newline >= 0 {
				value = value[:newline]
			}
			return strings.TrimSpace(value)
		}
		start = end + 1
	}
	return text
}

// ReadWorldBuffMatrix reads the matrix from a file: Playerbot.env, playerbots.conf or the bare matrix.
func ReadWorldBuffMatrix(path string) ([]WorldBuff, []string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, err
	}
	buffs, problems := ParseWorldBuffMatrix(WorldBuffMatrixSetting(string(data)))
	if len(buffs) == 0 {
		return nil, problems, fmt.Errorf("%s holds no world-buff matrix entries", path)
	}
	return buffs, problems, nil
}
