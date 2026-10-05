package main

import (
	"bytes"
	"cmp"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"

	"google.golang.org/protobuf/reflect/protoreflect"

	"github.com/wowsims/wotlk/sim/core/proto"
	"github.com/wowsims/wotlk/tools/acore/spellids/spellset"
	"github.com/wowsims/wotlk/tools/database/azerothcore"
)

// The talent sweep learns every talent the sim implements, at max rank, on a naked level 80 character,
// one talent at a time with a `.simval info` after each. mod-sim-validation's TestSimvalTalentSweep walks
// the paths in talentPathsFile; TestTalentPathsFile holds that file to what talentPathsFromData makes.
const (
	talentPathsFile        = "talent_paths.json"
	implementedTalentsFile = "implemented_talents.json"
	maxTalentPoints        = 71
	// points spent in a tree before each row of it opens
	pointsPerTalentRow = 5
)

type talentPathsDoc struct {
	Paths []talentPath `json:"paths"`
}

// talentPath is one character of the sweep: its setup, then its talents in learn order.
type talentPath struct {
	Name  string `json:"name"`
	Class uint8  `json:"class"` // the server's ids
	Race  uint8  `json:"race"`
	// the proto.Player spec field the sim builds it as, which picks the sim's stance or form
	Spec  string         `json:"spec"`
	Items []goldcharItem `json:"items"`
	// cast before the first talent and again after every step, so form and stance bonuses pick up the
	// new talent: stance, form, presence, aspect, and the raid buffs the sim gives the class
	Auras []int32      `json:"auras"`
	Steps []talentStep `json:"steps"`
}

type talentStep struct {
	Talent string `json:"talent"` // the tree file's fieldName
	Rank   int32  `json:"rank"`
	Spell  int32  `json:"spell"` // that rank's spell
	// a talent the sim doesn't implement, there only to open a row; the sim's sheet doesn't move for it
	Filler bool `json:"filler,omitempty"`
	// what the sim has up once the talent is in and the server only gets from a cast, cast right after it
	Auras []int32 `json:"auras,omitempty"`
}

// sweepSetup is a character the sweep learns one tree deep on, with what that tree's talents need.
type sweepSetup struct {
	name  string
	tree  int
	spec  protoreflect.Name
	items []goldcharItem
	auras []int32
}

type sweepClass struct {
	class uint8
	// the base stats test's race, so a sweep character before its first talent is one that replay checks
	race   uint8
	setups []sweepSetup
	// talents whose sheet change only shows in some setups, past the item requirement of their spell: a
	// stance, a form, a shield, a weapon in each hand, a melee weapon (the server's parry chance is 0
	// without one)
	needs map[string][]string
	// a talent whose sheet change needs another talent's form, which then goes earlier in its path
	after map[string]string
	// auras the sim puts up by itself once a talent is in, which the server only gets from a cast
	stepAuras map[string][]int32
	// talents learned at the very end of their path, where the capture has them
	last []string
}

const (
	equipSlotChest    = 4
	equipSlotMainHand = 15
	equipSlotOffHand  = 16
	equipSlotRanged   = 17
)

func armed(mainHand, offHand int32) []goldcharItem {
	items := []goldcharItem{{Slot: equipSlotMainHand, ID: mainHand}}
	if offHand != 0 {
		items = append(items, goldcharItem{Slot: equipSlotOffHand, ID: offHand})
	}
	return items
}

func withChest(items []goldcharItem, chest int32) []goldcharItem {
	return append(items, goldcharItem{Slot: equipSlotChest, ID: chest})
}

// Weapons from the golden characters' gear sets, known to equip on the live server. Two-handed: 40384 axe,
// 40406 mace, 40388 staff. One-handed: 40402 axe, 40345 sword, 39714 and 40386 daggers, 39763 and 39468
// fists. 40400 shield, 40385 gun. Chests for the armor-from-items talents, stamina and frost resistance
// only so no crit or hit moves under the other talents: 43586 plate, 43593 mail, 43590 leather.
//
// Auras also hold the buffs the path's talents improve, cast by the character itself so its own spell mods
// apply: Battle Shout, Devotion and Retribution Aura, Inner Fire, Blood Pact, the Strength of Earth and
// Flametongue Totem auras. The sim gets them through sweepBuffOptions.
var sweepClasses = []sweepClass{
	{class: 1, race: 6, setups: []sweepSetup{
		{"WarriorArms", 0, "warrior", armed(40384, 0), []int32{2457}},
		{"WarriorFury", 1, "warrior", armed(40402, 40402), []int32{2458, 47436}},
		{"WarriorProtection", 2, "protection_warrior", armed(40402, 40400), []int32{71}},
	}, needs: map[string][]string{
		"improvedBerserkerStance": {"WarriorFury"},
		"shieldSpecialization":    {"WarriorProtection"},
		"shieldMastery":           {"WarriorProtection"},
	}},
	{class: 2, race: 11, setups: []sweepSetup{
		{"PaladinProtection", 1, "protection_paladin", armed(40345, 40400), []int32{31801, 25780, 48942}},
		{"PaladinRetribution", 2, "retribution_paladin", armed(40384, 0), []int32{31801, 54043}},
	}},
	{class: 3, race: 3, setups: []sweepSetup{
		{"HunterBeastMastery", 0, "hunter", withChest([]goldcharItem{{Slot: equipSlotRanged, ID: 40385}}, 43593), []int32{61847}},
		{"HunterMarksmanship", 1, "hunter", []goldcharItem{{Slot: equipSlotRanged, ID: 40385}}, []int32{61847}},
		{"HunterSurvival", 2, "hunter", append(armed(40384, 0), goldcharItem{Slot: equipSlotRanged, ID: 40385}), []int32{61847}},
	}, needs: map[string][]string{"deflection": {"HunterSurvival"}},
		stepAuras: map[string][]int32{"trueshotAura": {19506}}},
	{class: 4, race: 1, setups: []sweepSetup{
		{"RogueAssassination", 0, "rogue", armed(39714, 40386), nil},
		{"RogueCombat", 1, "rogue", armed(39714, 40386), nil},
		{"RogueSubtlety", 2, "rogue", armed(39714, 40386), nil},
	}, needs: map[string][]string{"endurance": {"RogueCombat"}}, last: []string{"endurance"}},
	{class: 5, race: 5, setups: []sweepSetup{
		{"PriestDiscipline", 0, "healing_priest", nil, []int32{48161, 48073, 48169, 48168}},
		{"PriestHoly", 1, "shadow_priest", nil, []int32{48161, 48073, 48169}},
		{"PriestShadow", 2, "shadow_priest", nil, []int32{48161, 48073, 48169}},
	}, stepAuras: map[string][]int32{"shadowform": {15473}}},
	{class: 6, race: 2, setups: []sweepSetup{
		{"DeathknightBlood", 0, "deathknight", armed(40406, 0), []int32{48266, 57623}},
		{"DeathknightFrost", 1, "deathknight", withChest(armed(40402, 40402), 43586), []int32{48266, 57623}},
		{"DeathknightUnholy", 2, "deathknight", armed(40406, 0), []int32{48266, 57623}},
	}, stepAuras: map[string][]int32{"boneShield": {49222}}},
	{class: 7, race: 8, setups: []sweepSetup{
		{"ShamanElemental", 0, "elemental_shaman", nil, nil},
		{"ShamanEnhancement", 1, "enhancement_shaman", armed(39763, 39468), []int32{58646, 58655}},
		{"ShamanRestoration", 2, "elemental_shaman", nil, nil},
	}, needs: map[string][]string{"dualWieldSpecialization": {"ShamanEnhancement"}}},
	{class: 8, race: 7, setups: []sweepSetup{
		{"MageArcane", 0, "mage", nil, []int32{42995}},
		{"MageFire", 1, "mage", nil, []int32{42995}},
		{"MageFrost", 2, "mage", nil, []int32{42995}},
	}},
	{class: 9, race: 10, setups: []sweepSetup{
		{"WarlockAffliction", 0, "warlock", nil, []int32{47982}},
		{"WarlockDemonology", 1, "warlock", nil, nil},
		{"WarlockDestruction", 2, "warlock", nil, nil},
	}, last: []string{"darkPact"}},
	{class: 11, race: 4, setups: []sweepSetup{
		{"DruidBalance", 0, "balance_druid", nil, []int32{48469}},
		{"DruidFeralCat", 1, "feral_druid", armed(40388, 0), []int32{768, 48469}},
		{"DruidFeralBear", 1, "feral_tank_druid", withChest(armed(40388, 0), 43590), []int32{9634, 48469}},
		{"DruidRestoration", 2, "balance_druid", nil, []int32{48469}},
	}, needs: map[string][]string{
		"sharpenedClaws":          {"DruidFeralCat", "DruidFeralBear"},
		"feralSwiftness":          {"DruidFeralCat", "DruidFeralBear"},
		"heartOfTheWild":          {"DruidFeralCat", "DruidFeralBear"},
		"predatoryStrikes":        {"DruidFeralCat", "DruidFeralBear"},
		"leaderOfThePack":         {"DruidFeralCat", "DruidFeralBear"},
		"improvedLeaderOfThePack": {"DruidFeralCat", "DruidFeralBear"},
		"masterShapeshifter":      {"DruidFeralCat"}, // Bear Form's is damage, which the sheet doesn't show
		"naturalReaction":         {"DruidFeralBear"},
		"protectorOfThePack":      {"DruidFeralBear"},
		"survivalOfTheFittest":    {"DruidFeralBear"},
		"improvedMoonkinForm":     {"DruidBalance"},
		"furor":                   {"DruidBalance"},
	}, after: map[string]string{"furor": "moonkinForm"},
		stepAuras: map[string][]int32{"moonkinForm": {24858}}},
}

// sheetAuraTypes are the SPELL_AURA_MOD_* types whose effect the info record's sheet shows: stats,
// ratings, armor, health and power, attack and spell power, crit, hit, expertise, haste, avoidance and
// block. A talent spell with one of them and an equipped item requirement only moves the sheet with
// that item on.
var sheetAuraTypes = map[int64]bool{
	13: true, 22: true, 29: true, 34: true, 35: true, 47: true, 49: true, 51: true, 52: true, 54: true,
	55: true, 57: true, 65: true, 71: true, 99: true, 101: true, 124: true, 132: true, 133: true, 137: true,
	138: true, 140: true, 141: true, 142: true, 150: true, 158: true, 166: true, 167: true, 174: true,
	175: true, 182: true, 189: true, 192: true, 199: true, 212: true, 216: true, 230: true, 237: true,
	238: true, 240: true, 250: true, 268: true, 285: true, 290: true,
}

const spellEffectApplyAura = 6

// itemNeed is a talent spell's equipped item requirement: the item class and a mask of its subclasses.
type itemNeed struct {
	class, subclassMask int64
}

// sheetItemNeed is the item a talent rank's sheet change needs on, if any.
func sheetItemNeed(s *spellset.DumpSpell) *itemNeed {
	if s == nil || !s.Passive || s.EquippedItemClass < 0 {
		return nil
	}
	for _, e := range s.Effects {
		if e.Effect == spellEffectApplyAura && sheetAuraTypes[e.Aura] {
			return &itemNeed{s.EquippedItemClass, s.EquippedItemSubClassMask}
		}
	}
	return nil
}

type serverItemClass struct {
	class, subclass int64
}

func (n itemNeed) fits(item serverItemClass) bool {
	return item.class == n.class && n.subclassMask&(1<<item.subclass) != 0
}

const (
	itemClassWeapon     = 2
	itemClassArmor      = 4
	itemSubclassShield  = 6
	itemSubclassUnknown = -1
)

// loadItemClasses reads the item_template class and subclass of each db.json weapon, shield and armor
// piece off the sim's own weapon, hand, ranged and armor types.
var loadItemClasses = sync.OnceValues(func() (map[int32]serverItemClass, error) {
	root, err := checkoutRoot()
	if err != nil {
		return nil, err
	}
	raw, err := os.ReadFile(filepath.Join(root, simDBJSON))
	if err != nil {
		return nil, err
	}
	var db struct {
		Items []struct {
			ID               int32                  `json:"id"`
			WeaponType       proto.WeaponType       `json:"weaponType"`
			HandType         proto.HandType         `json:"handType"`
			RangedWeaponType proto.RangedWeaponType `json:"rangedWeaponType"`
			ArmorType        proto.ArmorType        `json:"armorType"`
		} `json:"items"`
	}
	if err := json.Unmarshal(raw, &db); err != nil {
		return nil, fmt.Errorf("%s: %w", simDBJSON, err)
	}
	classes := map[int32]serverItemClass{}
	for _, item := range db.Items {
		if c := serverItemClassOf(item.WeaponType, item.HandType, item.RangedWeaponType, item.ArmorType); c.subclass != itemSubclassUnknown {
			classes[item.ID] = c
		}
	}
	return classes, nil
})

// item_template subclasses of ITEM_CLASS_WEAPON, one-handed and two-handed
var weaponSubclasses = map[proto.WeaponType][2]int64{
	proto.WeaponType_WeaponTypeAxe: {0, 1}, proto.WeaponType_WeaponTypeMace: {4, 5},
	proto.WeaponType_WeaponTypeSword: {7, 8}, proto.WeaponType_WeaponTypePolearm: {6, 6},
	proto.WeaponType_WeaponTypeStaff: {10, 10}, proto.WeaponType_WeaponTypeFist: {13, 13},
	proto.WeaponType_WeaponTypeDagger: {15, 15},
}

var rangedSubclasses = map[proto.RangedWeaponType]int64{
	proto.RangedWeaponType_RangedWeaponTypeBow: 2, proto.RangedWeaponType_RangedWeaponTypeGun: 3,
	proto.RangedWeaponType_RangedWeaponTypeThrown: 16, proto.RangedWeaponType_RangedWeaponTypeCrossbow: 18,
	proto.RangedWeaponType_RangedWeaponTypeWand: 19,
}

// serverItemClassOf is a sim weapon's, shield's or armor piece's item_template class and subclass.
func serverItemClassOf(weaponType proto.WeaponType, handType proto.HandType, rangedType proto.RangedWeaponType,
	armorType proto.ArmorType) serverItemClass {
	if weaponType == proto.WeaponType_WeaponTypeShield {
		return serverItemClass{itemClassArmor, itemSubclassShield}
	}
	if subclasses, ok := weaponSubclasses[weaponType]; ok {
		if handType == proto.HandType_HandTypeTwoHand {
			return serverItemClass{itemClassWeapon, subclasses[1]}
		}
		return serverItemClass{itemClassWeapon, subclasses[0]}
	}
	if subclass, ok := rangedSubclasses[rangedType]; ok {
		return serverItemClass{itemClassWeapon, subclass}
	}
	// item_template's cloth to plate subclasses are 1 to 4, as proto.ArmorType's
	if armorType != proto.ArmorType_ArmorTypeUnknown {
		return serverItemClass{itemClassArmor, int64(armorType)}
	}
	return serverItemClass{itemClassWeapon, itemSubclassUnknown}
}

// implementedTalent is one line of TE-TALENTS' implemented_talents.json.
type implementedTalent struct {
	Class      string  `json:"class"`
	Tree       string  `json:"tree"`
	FieldName  string  `json:"fieldName"`
	Row        int     `json:"row"`
	MaxPoints  int32   `json:"maxPoints"`
	RankSpells []int32 `json:"rankSpells"`
}

// sweepTalent is one talent of a class's tree file.
type sweepTalent struct {
	tree, index int
	field       string
	row, col    int
	maxPoints   int32
	spells      []int32 // per rank
	prereq      int     // its index in the tree, -1 for none
	implemented bool
	// the setups that show its sheet change; only implemented talents have any
	hosts []string
}

type talentRef struct{ tree, index int }

type treeFileTalent struct {
	FieldName      string          `json:"fieldName"`
	Location       talentLocation  `json:"location"`
	PrereqLocation *talentLocation `json:"prereqLocation"`
	SpellIds       []int32         `json:"spellIds"`
	MaxPoints      int32           `json:"maxPoints"`
}

// talents reads the class's tree file and marks what the sim implements and where its sheet change
// shows.
func (c sweepClass) talents(root string, implemented []implementedTalent, dump spellset.Dump,
	itemClasses map[int32]serverItemClass) ([][]*sweepTalent, error) {

	className := azerothcore.ClassNames[int32(c.class)]
	path := filepath.Join(root, talentTreesDir, className+".json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var file []struct {
		Name    string           `json:"name"`
		Talents []treeFileTalent `json:"talents"`
	}
	if err := json.Unmarshal(data, &file); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}

	var trees [][]*sweepTalent
	byName := map[[2]string]*sweepTalent{}
	for i, tree := range file {
		var talents []*sweepTalent
		for j, t := range tree.Talents {
			prereq := -1
			if t.PrereqLocation != nil {
				prereq = slices.IndexFunc(tree.Talents, func(o treeFileTalent) bool { return o.Location == *t.PrereqLocation })
				if prereq < 0 {
					return nil, fmt.Errorf("%s %s: its prerequisite isn't in the tree", className, t.FieldName)
				}
			}
			st := &sweepTalent{tree: i, index: j, field: t.FieldName, row: t.Location.Row, col: t.Location.Col,
				maxPoints: t.MaxPoints, spells: rankSpells(t.SpellIds, t.MaxPoints), prereq: prereq}
			talents = append(talents, st)
			byName[[2]string{tree.Name, t.FieldName}] = st
		}
		trees = append(trees, talents)
	}

	for _, it := range implemented {
		if it.Class != className {
			continue
		}
		t := byName[[2]string{it.Tree, it.FieldName}]
		if t == nil {
			return nil, fmt.Errorf("%s: %s %s isn't in %s", implementedTalentsFile, it.Tree, it.FieldName, path)
		}
		if t.maxPoints != it.MaxPoints || !slices.Equal(t.spells, it.RankSpells) {
			return nil, fmt.Errorf("%s: %s %s differs from %s; rewrite it", implementedTalentsFile, it.Tree, it.FieldName, path)
		}
		t.implemented = true

		need := sheetItemNeed(dump[t.spells[len(t.spells)-1]])
		for _, s := range c.setups {
			if need != nil && !slices.ContainsFunc(s.items, func(item goldcharItem) bool {
				return need.fits(itemClasses[item.ID])
			}) {
				continue
			}
			if only, ok := c.needs[t.field]; ok && !slices.Contains(only, s.name) {
				continue
			}
			t.hosts = append(t.hosts, s.name)
		}
		if len(t.hosts) == 0 {
			return nil, fmt.Errorf("%s %s: no setup shows its sheet change (item %+v, needs %v)", className, t.field,
				need, c.needs[t.field])
		}
	}
	for field, setups := range c.needs {
		for _, name := range setups {
			if !slices.ContainsFunc(c.setups, func(s sweepSetup) bool { return s.name == name }) {
				return nil, fmt.Errorf("%s %s needs setup %s, which isn't one", className, field, name)
			}
		}
	}
	return trees, nil
}

// sweepPlanner packs one class's implemented talents into paths.
type sweepPlanner struct {
	c         sweepClass
	trees     [][]*sweepTalent
	byField   map[string]talentRef
	uncovered map[talentRef]bool
}

func (p *sweepPlanner) talent(ref talentRef) *sweepTalent { return p.trees[ref.tree][ref.index] }

func (p *sweepPlanner) hosts(ref talentRef, s sweepSetup) bool {
	return slices.Contains(p.talent(ref).hosts, s.name)
}

// deps are the talents that have to be learned before this one, at max rank.
func (p *sweepPlanner) deps(ref talentRef) []talentRef {
	t := p.talent(ref)
	var deps []talentRef
	if t.prereq >= 0 {
		deps = append(deps, talentRef{ref.tree, t.prereq})
	}
	if field, ok := p.c.after[t.field]; ok {
		deps = append(deps, p.byField[field])
	}
	return deps
}

// closure is want with every talent it depends on.
func (p *sweepPlanner) closure(want map[talentRef]bool) map[talentRef]bool {
	all := map[talentRef]bool{}
	var add func(ref talentRef)
	add = func(ref talentRef) {
		if all[ref] {
			return
		}
		all[ref] = true
		for _, dep := range p.deps(ref) {
			add(dep)
		}
	}
	for ref := range want {
		add(ref)
	}
	return all
}

// fillTree learns want's talents of one tree at max rank, plus the fewest filler points that open each row
// they sit in. Fillers come from rows below, talents the sim doesn't implement first.
func (p *sweepPlanner) fillTree(tree int, want map[int]bool) (map[int]int32, bool) {
	talents := p.trees[tree]
	ranks := map[int]int32{}
	for i := range want {
		ranks[i] = talents[i].maxPoints
	}
	below := func(row int) int32 {
		var sum int32
		for i, rank := range ranks {
			if talents[i].row < row {
				sum += rank
			}
		}
		return sum
	}
	for row := 0; ; row++ {
		inRow, past := false, true
		for i := range want {
			inRow = inRow || talents[i].row == row
			past = past && talents[i].row < row
		}
		if past {
			return ranks, true
		}
		if !inRow {
			continue
		}
		// one talent at a time, so an implemented one only goes in when no filler's row is open yet
		for need := int32(pointsPerTalentRow*row) - below(row); need > 0; {
			var pick *sweepTalent
			for _, implemented := range []bool{false, true} {
				for _, t := range talents {
					if t.row < row && !want[t.index] && t.implemented == implemented && ranks[t.index] < t.maxPoints &&
						p.c.after[t.field] == "" &&
						(t.prereq < 0 || ranks[t.prereq] == talents[t.prereq].maxPoints) &&
						below(t.row) >= int32(pointsPerTalentRow*t.row) {
						pick = t
						break
					}
				}
				if pick != nil {
					break
				}
			}
			if pick == nil {
				return nil, false
			}
			add := min(pick.maxPoints-ranks[pick.index], need)
			ranks[pick.index] += add
			need -= add
		}
	}
}

const unaffordable = 1 << 20

// learn is what a set of wanted talents takes in each tree, and the points it all costs.
func (p *sweepPlanner) learn(want map[talentRef]bool) (map[talentRef]int32, int32) {
	perTree := make([]map[int]bool, len(p.trees))
	for ref := range p.closure(want) {
		if perTree[ref.tree] == nil {
			perTree[ref.tree] = map[int]bool{}
		}
		perTree[ref.tree][ref.index] = true
	}
	ranks := map[talentRef]int32{}
	var cost int32
	for tree, w := range perTree {
		if w == nil {
			continue
		}
		treeRanks, ok := p.fillTree(tree, w)
		if !ok {
			return nil, unaffordable
		}
		for i, rank := range treeRanks {
			ranks[talentRef{tree, i}] = rank
			cost += rank
		}
	}
	return ranks, cost
}

func (p *sweepPlanner) cost(want map[talentRef]bool) int32 {
	_, cost := p.learn(want)
	return cost
}

// exclusive is an uncovered talent only this setup shows.
func (p *sweepPlanner) exclusive(ref talentRef, s sweepSetup) bool {
	hosts := p.talent(ref).hosts
	return len(hosts) == 1 && hosts[0] == s.name
}

func sortedRefs(refs map[talentRef]bool, trees [][]*sweepTalent) []talentRef {
	out := make([]talentRef, 0, len(refs))
	for ref := range refs {
		out = append(out, ref)
	}
	slices.SortFunc(out, func(a, b talentRef) int {
		ta, tb := trees[a.tree][a.index], trees[b.tree][b.index]
		return cmp.Or(cmp.Compare(ta.row, tb.row), cmp.Compare(a.tree, b.tree), cmp.Compare(ta.col, tb.col))
	})
	return out
}

// trim drops wanted talents until the rest fits in a character's points, from the lowest row, which
// another path picks up cheapest: in that row the one that makes it fit with the fewest points, else the
// one saving the most. Talents another setup can show go before the ones only this one can.
func (p *sweepPlanner) trim(want map[talentRef]bool, s sweepSetup) error {
	for cost := p.cost(want); cost > maxTalentPoints; cost = p.cost(want) {
		var needed []talentRef
		for ref := range want {
			needed = append(needed, p.deps(ref)...)
		}
		var drop *talentRef
		for _, exclusiveToo := range []bool{false, true} {
			var best talentRef
			bestRow, bestFits, bestPoints, bestSaved, found := 0, false, int32(0), int32(0), false
			for _, ref := range sortedRefs(want, p.trees) {
				t := p.talent(ref)
				if slices.Contains(needed, ref) || p.exclusive(ref, s) && !exclusiveToo || found && t.row > bestRow {
					continue
				}
				delete(want, ref)
				after := p.cost(want)
				want[ref] = true
				fits, saved := after <= maxTalentPoints, cost-after
				if saved <= 0 {
					continue
				}
				if !found || fits && (!bestFits || t.maxPoints < bestPoints) || !fits && !bestFits && saved > bestSaved {
					best, bestRow, bestFits, bestPoints, bestSaved, found = ref, t.row, fits, t.maxPoints, saved, true
				}
			}
			if found {
				drop = &best
				break
			}
		}
		if drop == nil {
			return fmt.Errorf("%s: nothing left to drop at %d points", s.name, cost)
		}
		delete(want, *drop)
	}
	return nil
}

type plannedPath struct {
	setup   int
	ranks   map[talentRef]int32
	covered []talentRef
	points  int32
}

// build makes the setup's path over the uncovered talents: its own tree's and the ones only it shows, as
// many as fit, then the other trees' from the lowest row up while points are left.
func (p *sweepPlanner) build(setup int) (plannedPath, error) {
	s := p.c.setups[setup]
	want := map[talentRef]bool{}
	for ref := range p.uncovered {
		if p.hosts(ref, s) && (ref.tree == s.tree || p.exclusive(ref, s)) {
			want[ref] = true
		}
	}
	if err := p.trim(want, s); err != nil {
		return plannedPath{}, err
	}
	for _, ref := range sortedRefs(p.uncovered, p.trees) {
		if want[ref] || ref.tree == s.tree || !p.hosts(ref, s) {
			continue
		}
		want[ref] = true
		if p.cost(want) > maxTalentPoints {
			delete(want, ref)
		}
	}

	ranks, points := p.learn(want)
	plan := plannedPath{setup: setup, ranks: ranks}
	for ref, rank := range ranks {
		if p.uncovered[ref] && rank == p.talent(ref).maxPoints && p.hosts(ref, s) {
			plan.covered = append(plan.covered, ref)
			plan.points += rank
		}
	}
	if points > maxTalentPoints {
		plan.covered, plan.points = nil, 0
	}
	return plan, nil
}

// steps orders a path's talents the way the server can learn them: its own tree first, then the others in
// tree order, each row by row, a prerequisite before the talent in its row that needs it, and the class's
// last talents after all of them.
func (p *sweepPlanner) steps(plan plannedPath) []talentStep {
	s := p.c.setups[plan.setup]
	refs := make([]talentRef, 0, len(plan.ranks))
	for ref := range plan.ranks {
		refs = append(refs, ref)
	}
	treeOrder := func(ref talentRef) int {
		switch {
		case slices.Contains(p.c.last, p.talent(ref).field):
			return len(p.trees)
		case ref.tree == s.tree:
			return -1
		}
		return ref.tree
	}
	// how many prerequisites deep the talent sits inside its own row
	depth := func(ref talentRef) int {
		d := 0
		for t := p.talent(ref); t.prereq >= 0 && p.trees[ref.tree][t.prereq].row == t.row; t = p.trees[ref.tree][t.prereq] {
			d++
		}
		return d
	}
	slices.SortFunc(refs, func(a, b talentRef) int {
		ta, tb := p.talent(a), p.talent(b)
		return cmp.Or(cmp.Compare(treeOrder(a), treeOrder(b)), cmp.Compare(ta.row, tb.row),
			cmp.Compare(depth(a), depth(b)), cmp.Compare(ta.col, tb.col))
	})
	var steps []talentStep
	for _, ref := range refs {
		t, rank := p.talent(ref), plan.ranks[ref]
		step := talentStep{Talent: t.field, Rank: rank, Spell: t.spells[rank-1], Filler: !t.implemented}
		if rank == t.maxPoints {
			step.Auras = p.c.stepAuras[t.field]
		}
		steps = append(steps, step)
	}
	return steps
}

// plan covers every implemented talent of the class: a path per setup, in order, each its own tree first,
// then while any are left, the setup's path that covers the most points still uncovered.
func (c sweepClass) plan(trees [][]*sweepTalent) ([]talentPath, error) {
	p := &sweepPlanner{c: c, trees: trees, byField: map[string]talentRef{}, uncovered: map[talentRef]bool{}}
	for _, tree := range trees {
		for _, t := range tree {
			p.byField[t.field] = talentRef{t.tree, t.index}
			if t.implemented {
				p.uncovered[talentRef{t.tree, t.index}] = true
			}
		}
	}
	for field, dep := range c.after {
		if _, ok := p.byField[dep]; !ok {
			return nil, fmt.Errorf("%s comes after %s, which isn't a talent", field, dep)
		}
	}

	var plans []plannedPath
	for i := range c.setups {
		plan, err := p.build(i)
		if err != nil {
			return nil, err
		}
		if plan.points == 0 {
			continue
		}
		for _, ref := range plan.covered {
			delete(p.uncovered, ref)
		}
		plans = append(plans, plan)
	}
	for len(p.uncovered) > 0 {
		var best plannedPath
		for i := range c.setups {
			plan, err := p.build(i)
			if err != nil {
				return nil, err
			}
			if plan.points > best.points {
				best = plan
			}
		}
		if best.points == 0 {
			var left []string
			for _, ref := range sortedRefs(p.uncovered, trees) {
				left = append(left, p.talent(ref).field)
			}
			return nil, fmt.Errorf("class %d: no setup covers %v", c.class, left)
		}
		for _, ref := range best.covered {
			delete(p.uncovered, ref)
		}
		plans = append(plans, best)
	}

	slices.SortStableFunc(plans, func(a, b plannedPath) int { return cmp.Compare(a.setup, b.setup) })
	var paths []talentPath
	used := map[int]int{}
	for _, plan := range plans {
		s := c.setups[plan.setup]
		used[plan.setup]++
		name := s.name
		if n := used[plan.setup]; n > 1 {
			name += strconv.Itoa(n)
		}
		path := talentPath{Name: name, Class: c.class, Race: c.race, Spec: string(s.spec),
			Items: slices.Clone(s.items), Auras: slices.Clone(s.auras), Steps: p.steps(plan)}
		if path.Items == nil {
			path.Items = []goldcharItem{}
		}
		if path.Auras == nil {
			path.Auras = []int32{}
		}
		for _, step := range path.Steps {
			if err := p.checkAfter(path, step); err != nil {
				return nil, err
			}
		}
		paths = append(paths, path)
	}
	return paths, nil
}

// checkAfter holds a talent to coming after the one whose form it needs.
func (p *sweepPlanner) checkAfter(path talentPath, step talentStep) error {
	dep, ok := p.c.after[step.Talent]
	if !ok {
		return nil
	}
	at := slices.IndexFunc(path.Steps, func(s talentStep) bool { return s.Talent == step.Talent })
	before := slices.IndexFunc(path.Steps, func(s talentStep) bool { return s.Talent == dep })
	if before < 0 || before > at {
		return fmt.Errorf("%s learns %s before %s", path.Name, step.Talent, dep)
	}
	return nil
}

func readImplementedTalents(path string) ([]implementedTalent, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var talents []implementedTalent
	if err := json.Unmarshal(data, &talents); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return talents, nil
}

// talentPathsFromData generates every class's paths from the implemented talents, the tree files, the
// spelldump's item requirements and the sim's item database.
func talentPathsFromData(root string) ([]talentPath, error) {
	implemented, err := readImplementedTalents(filepath.Join(root, fixtureDir, implementedTalentsFile))
	if err != nil {
		return nil, err
	}
	dump, err := spellset.ReadDump(filepath.Join(root, spellset.DefaultDumpPath))
	if err != nil {
		return nil, err
	}
	if len(dump) == 0 {
		return nil, fmt.Errorf("no spelldump at %s", spellset.DefaultDumpPath)
	}
	itemClasses, err := loadItemClasses()
	if err != nil {
		return nil, err
	}
	var paths []talentPath
	for _, c := range sweepClasses {
		for _, s := range c.setups {
			for _, item := range s.items {
				if _, ok := itemClasses[item.ID]; !ok {
					return nil, fmt.Errorf("%s: item %d isn't a weapon, shield or armor in %s", s.name, item.ID, simDBJSON)
				}
			}
		}
		trees, err := c.talents(root, implemented, dump, itemClasses)
		if err != nil {
			return nil, err
		}
		classPaths, err := c.plan(trees)
		if err != nil {
			return nil, err
		}
		paths = append(paths, classPaths...)
	}
	return paths, nil
}

func readTalentPaths(path string) ([]talentPath, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var doc talentPathsDoc
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return doc.Paths, nil
}

// marshalTalentPaths writes one line per path head and per step, so a regenerated file diffs by talent.
func marshalTalentPaths(paths []talentPath) ([]byte, error) {
	var b bytes.Buffer
	b.WriteString("{\"paths\": [\n")
	for i, path := range paths {
		steps := path.Steps
		path.Steps = nil
		head, err := json.Marshal(path)
		if err != nil {
			return nil, err
		}
		head = bytes.TrimSuffix(head, []byte(`"steps":null}`))
		b.Write(head)
		b.WriteString("\"steps\": [\n")
		for j, step := range steps {
			line, err := json.Marshal(step)
			if err != nil {
				return nil, err
			}
			b.WriteString("  ")
			b.Write(line)
			if j < len(steps)-1 {
				b.WriteByte(',')
			}
			b.WriteByte('\n')
		}
		b.WriteString("]}")
		if i < len(paths)-1 {
			b.WriteByte(',')
		}
		b.WriteByte('\n')
	}
	b.WriteString("]}\n")
	return b.Bytes(), nil
}

// talentString is the sim's talent string for the path's first n steps.
func (path talentPath) talentString(n int) (string, error) {
	data, err := loadDamageData()
	if err != nil {
		return "", err
	}
	class := data.talents[path.Class]
	if class == nil {
		return "", fmt.Errorf("no talent trees for class %d", path.Class)
	}
	ranks := make([][]byte, len(class.sizes))
	for i, size := range class.sizes {
		ranks[i] = bytes.Repeat([]byte{'0'}, size)
	}
	for _, step := range path.Steps[:n] {
		slot, ok := class.fields[step.Talent]
		if !ok {
			return "", fmt.Errorf("%s: no talent %s", path.Name, step.Talent)
		}
		ranks[slot.tree][slot.index] = byte('0' + step.Rank)
	}
	trees := make([]string, len(ranks))
	for i, tree := range ranks {
		trees[i] = strings.TrimRight(string(tree), "0")
	}
	return strings.TrimRight(strings.Join(trees, "-"), "-"), nil
}

// goldcharSetup is the path's character after its first n steps, as goldchar.go builds characters: the
// setup's auras and those of the steps so far.
func (path talentPath) goldcharSetup(n int) (goldcharSetup, error) {
	talents, err := path.talentString(n)
	if err != nil {
		return goldcharSetup{}, err
	}
	s := goldcharSetup{Name: path.Name, Spec: path.Spec, Class: path.Class, Race: path.Race, Items: path.Items,
		Talents: talents, Auras: slices.Clone(path.Auras)}
	for _, step := range path.Steps[:n] {
		s.Auras = append(s.Auras, step.Auras...)
		s.TalentSpells = append(s.TalentSpells, step.Spell)
	}
	return s, nil
}

// sweepBuffOptions are the spec options that make the sim character put up a buff a path casts, so the
// talents improving it work through the class's own code. Battle Shout needs none: the warrior registers
// its aura, which the path's auras put on.
var sweepBuffOptions = map[int32]func(*proto.Player){
	48942: sweepPaladinAura(proto.PaladinAura_DevotionAura),
	54043: sweepPaladinAura(proto.PaladinAura_RetributionAura),
	48168: sweepInnerFire,
	47982: warlockSummon(proto.Warlock_Options_Imp),
	58646: sweepTotems(func(t *proto.ShamanTotems) { t.Earth = proto.EarthTotem_StrengthOfEarthTotem }),
	58655: sweepTotems(func(t *proto.ShamanTotems) { t.Fire = proto.FireTotem_FlametongueTotem }),
}

func sweepPaladinAura(aura proto.PaladinAura) func(*proto.Player) {
	return func(p *proto.Player) {
		switch spec := p.Spec.(type) {
		case *proto.Player_RetributionPaladin:
			spec.RetributionPaladin.Options.Aura = aura
		case *proto.Player_ProtectionPaladin:
			spec.ProtectionPaladin.Options.Aura = aura
		case *proto.Player_HolyPaladin:
			spec.HolyPaladin.Options.Aura = aura
		}
	}
}

func sweepInnerFire(p *proto.Player) {
	switch spec := p.Spec.(type) {
	case *proto.Player_HealingPriest:
		spec.HealingPriest.Options.UseInnerFire = true
	case *proto.Player_ShadowPriest:
		spec.ShadowPriest.Options.Armor = proto.ShadowPriest_Options_InnerFire
	}
}

func sweepTotems(set func(*proto.ShamanTotems)) func(*proto.Player) {
	return func(p *proto.Player) {
		var totems **proto.ShamanTotems
		switch spec := p.Spec.(type) {
		case *proto.Player_ElementalShaman:
			totems = &spec.ElementalShaman.Options.Totems
		case *proto.Player_EnhancementShaman:
			totems = &spec.EnhancementShaman.Options.Totems
		case *proto.Player_RestorationShaman:
			totems = &spec.RestorationShaman.Options.Totems
		default:
			return
		}
		if *totems == nil {
			*totems = &proto.ShamanTotems{}
		}
		set(*totems)
	}
}

// newPathSim builds a sweep path's character like newGoldcharSim, plus sweepBuffOptions for the setup's
// auras. It skips newGoldcharSim's Sniper Training check: a sweep record has no attacker auras.
func newPathSim(rec damageRecord, s goldcharSetup) (*damageSim, error) {
	player, err := goldcharPlayer(s)
	if err != nil {
		return nil, err
	}
	for _, id := range s.Auras {
		if give := sweepBuffOptions[id]; give != nil {
			give(player)
		}
	}
	return newDamageSim(rec, player, s.Auras, 1)
}
