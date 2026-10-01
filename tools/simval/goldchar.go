package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"math"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"

	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
	"github.com/wowsims/wotlk/sim/core/stats"
	"github.com/wowsims/wotlk/tools/database/azerothcore"
)

// The golden characters are the non-healer golden suites' default players. mod-sim-validation's
// TestSimvalGoldenCharacters builds each on the live server from goldcharSetupsFile, which this package
// reads too, and TestGoldcharSetupsFollowTheSuites keeps that file in step with the suites.
const goldcharSetupsFile = "goldchar_setups.json"

type goldcharSetups struct {
	Setups []goldcharSetup `json:"setups"`
}

type goldcharSetup struct {
	Name string `json:"name"`
	// "<package dir> <test>" of every suite whose default player this is
	Suites []string `json:"suites"`
	// the proto.Player spec field the suites build, e.g. "feral_druid"
	Spec    string `json:"spec"`
	Class   uint8  `json:"class"` // the server's ids
	Race    uint8  `json:"race"`
	GearSet string `json:"gearSet"`
	// the gear set's items by server equipment slot, gems and enchants left out
	Items   []goldcharItem `json:"items"`
	Talents string         `json:"talents"`
	// the talents' rank spells in the order the server can learn them
	TalentSpells []int32         `json:"talentSpells"`
	Glyphs       []goldcharGlyph `json:"glyphs"`
	// what the sim starts the spec with that the server character needs cast before recording: its
	// stance, form, presence, seal, aspect, tracking or Bone Shield, and the raid buffs the class gives itself
	Auras []int32 `json:"auras"`
	// the imbue spell the sim always puts on the main hand (Elemental's Flametongue Weapon), cast on the
	// server's weapon
	Imbue  int32           `json:"imbue,omitempty"`
	Probes []goldcharProbe `json:"probes"`
}

type goldcharItem struct {
	Slot uint8 `json:"slot"` // EQUIPMENT_SLOT_*
	ID   int32 `json:"id"`
}

type goldcharGlyph struct {
	Slot  uint8 `json:"slot"`  // the server's glyph slot
	Item  int32 `json:"item"`  // the sim's glyph id
	Spell int32 `json:"spell"` // the aura it puts on
	Glyph int32 `json:"glyph"` // the GlyphProperties id `.simval glyph` takes
}

// goldcharProbe is a spell recorded with `.simval damage`. Shaped is part of the reason the record has
// to give when the server shapes the spell, empty when the replay compares it.
type goldcharProbe struct {
	ID     int32  `json:"id"`
	Name   string `json:"name"`
	Shaped string `json:"shaped,omitempty"`
}

func readGoldcharSetups(path string) ([]goldcharSetup, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var file goldcharSetups
	if err := json.Unmarshal(data, &file); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return file.Setups, nil
}

// serverEquipSlots maps proto.ItemSlot to the server's EQUIPMENT_SLOT_*.
var serverEquipSlots = [...]uint8{0, 1, 2, 14, 4, 8, 9, 5, 6, 7, 10, 11, 12, 13, 15, 16, 17}

// The server's glyph slots per type, from GlyphSlot.dbc's Order (slot = Order - 1). All six are open at 80.
var (
	majorGlyphSlots = [3]uint8{0, 3, 5}
	minorGlyphSlots = [3]uint8{1, 2, 4}
)

var healerSpecs = map[string]bool{
	"restoration_druid": true, "holy_paladin": true, "healing_priest": true, "restoration_shaman": true,
}

// suiteSetup is a suite's default player as its CharacterSuiteConfig spells it out.
type suiteSetup struct {
	suite   string
	spec    string
	class   proto.Class
	race    proto.Race
	gearSet string // relative to the checkout
	talents string
	glyphs  *proto.Glyphs
}

// readSuiteSetups finds every CharacterSuiteConfig in a Test function under sim/, in file order.
func readSuiteSetups(root string) ([]suiteSetup, error) {
	var dirs []string
	err := filepath.WalkDir(filepath.Join(root, "sim"), func(p string, d fs.DirEntry, err error) error {
		if err == nil && d.IsDir() {
			dirs = append(dirs, p)
		}
		return err
	})
	if err != nil {
		return nil, err
	}

	var setups []suiteSetup
	for _, dir := range dirs {
		found, err := readPackageSuites(root, dir)
		if err != nil {
			return nil, err
		}
		setups = append(setups, found...)
	}
	return setups, nil
}

func readPackageSuites(root, dir string) ([]suiteSetup, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	fset := token.NewFileSet()
	var files []*ast.File
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") {
			continue
		}
		f, err := parser.ParseFile(fset, filepath.Join(dir, e.Name()), nil, 0)
		if err != nil {
			return nil, err
		}
		files = append(files, f)
	}

	pkg := &suitePackage{values: map[string]ast.Expr{}}
	for _, f := range files {
		for _, decl := range f.Decls {
			gen, ok := decl.(*ast.GenDecl)
			if !ok {
				continue
			}
			for _, spec := range gen.Specs {
				if vs, ok := spec.(*ast.ValueSpec); ok && len(vs.Names) == len(vs.Values) {
					for i, name := range vs.Names {
						pkg.values[name.Name] = vs.Values[i]
					}
				}
			}
		}
	}

	rel, err := filepath.Rel(root, dir)
	if err != nil {
		return nil, err
	}
	rel = filepath.ToSlash(rel)

	var setups []suiteSetup
	for _, f := range files {
		if !strings.HasSuffix(fset.File(f.Pos()).Name(), "_test.go") {
			continue
		}
		for _, decl := range f.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil || !strings.HasPrefix(fn.Name.Name, "Test") {
				continue
			}
			var problem error
			ast.Inspect(fn.Body, func(n ast.Node) bool {
				lit, ok := n.(*ast.CompositeLit)
				if !ok || !isSelector(lit.Type, "core", "CharacterSuiteConfig") || problem != nil {
					return problem == nil
				}
				setup, err := pkg.suiteSetup(lit, rel)
				if err != nil {
					problem = fmt.Errorf("%s %s: %w", rel, fn.Name.Name, err)
					return false
				}
				setup.suite = rel + " " + fn.Name.Name
				setups = append(setups, setup)
				return false
			})
			if problem != nil {
				return nil, problem
			}
		}
	}
	return setups, nil
}

// suitePackage holds a package's top-level vars and consts, which suite configs name.
type suitePackage struct {
	values map[string]ast.Expr
}

// resolve follows identifiers to the values they're declared with and drops a leading &.
func (p *suitePackage) resolve(expr ast.Expr) ast.Expr {
	for range 10 {
		switch e := expr.(type) {
		case *ast.Ident:
			value, ok := p.values[e.Name]
			if !ok {
				return expr
			}
			expr = value
		case *ast.UnaryExpr:
			if e.Op != token.AND {
				return expr
			}
			expr = e.X
		case *ast.ParenExpr:
			expr = e.X
		default:
			return expr
		}
	}
	return expr
}

func (p *suitePackage) suiteSetup(lit *ast.CompositeLit, dir string) (suiteSetup, error) {
	var setup suiteSetup
	for _, elt := range lit.Elts {
		kv, ok := elt.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		key, _ := kv.Key.(*ast.Ident)
		if key == nil {
			continue
		}
		value := p.resolve(kv.Value)
		var err error
		switch key.Name {
		case "Class":
			var n int32
			n, err = enumValue(value)
			setup.class = proto.Class(n)
		case "Race":
			var n int32
			n, err = enumValue(value)
			setup.race = proto.Race(n)
		case "GearSet":
			setup.gearSet, err = p.gearSetPath(value, dir)
		case "Talents":
			setup.talents, err = p.stringValue(value)
		case "Glyphs":
			setup.glyphs, err = p.glyphs(value)
		case "SpecOptions":
			setup.spec, err = p.specField(value)
		}
		if err != nil {
			return setup, fmt.Errorf("%s: %w", key.Name, err)
		}
	}
	if setup.glyphs == nil {
		setup.glyphs = &proto.Glyphs{}
	}
	return setup, nil
}

func isSelector(expr ast.Expr, pkg, name string) bool {
	sel, ok := expr.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != name {
		return false
	}
	x, ok := sel.X.(*ast.Ident)
	return ok && x.Name == pkg
}

// enumValue reads a generated proto enum constant such as proto.Race_RaceOrc, or a plain number.
func enumValue(expr ast.Expr) (int32, error) {
	if lit, ok := expr.(*ast.BasicLit); ok && lit.Kind == token.INT {
		n, err := strconv.ParseInt(lit.Value, 0, 32)
		return int32(n), err
	}
	sel, ok := expr.(*ast.SelectorExpr)
	if !ok || !isSelector(sel, "proto", sel.Sel.Name) {
		return 0, fmt.Errorf("%T isn't a proto enum constant", expr)
	}
	enumName, valueName, ok := strings.Cut(sel.Sel.Name, "_")
	if !ok {
		return 0, fmt.Errorf("proto.%s isn't an enum constant", sel.Sel.Name)
	}
	enum, err := protoregistry.GlobalTypes.FindEnumByName(protoreflect.FullName("proto." + enumName))
	if err != nil {
		return 0, err
	}
	value := enum.Descriptor().Values().ByName(protoreflect.Name(valueName))
	if value == nil {
		return 0, fmt.Errorf("proto.%s has no value %s", enumName, valueName)
	}
	return int32(value.Number()), nil
}

func (p *suitePackage) stringValue(expr ast.Expr) (string, error) {
	lit, ok := expr.(*ast.BasicLit)
	if !ok || lit.Kind != token.STRING {
		return "", fmt.Errorf("%T isn't a string literal", expr)
	}
	return strconv.Unquote(lit.Value)
}

// gearSetPath reads core.GetGearSet(dir, name), whose dir is relative to the suite's package.
func (p *suitePackage) gearSetPath(expr ast.Expr, pkgDir string) (string, error) {
	call, ok := expr.(*ast.CallExpr)
	if !ok || !isSelector(call.Fun, "core", "GetGearSet") || len(call.Args) != 2 {
		return "", fmt.Errorf("not a core.GetGearSet call")
	}
	dir, err := p.stringValue(p.resolve(call.Args[0]))
	if err != nil {
		return "", err
	}
	name, err := p.stringValue(p.resolve(call.Args[1]))
	if err != nil {
		return "", err
	}
	return path.Join(pkgDir, dir, name+".gear.json"), nil
}

func (p *suitePackage) glyphs(expr ast.Expr) (*proto.Glyphs, error) {
	lit, ok := expr.(*ast.CompositeLit)
	if !ok || !isSelector(lit.Type, "proto", "Glyphs") {
		return nil, fmt.Errorf("not a proto.Glyphs literal")
	}
	glyphs := &proto.Glyphs{}
	m := glyphs.ProtoReflect()
	for _, elt := range lit.Elts {
		kv, ok := elt.(*ast.KeyValueExpr)
		if !ok {
			return nil, fmt.Errorf("unkeyed proto.Glyphs field")
		}
		key, _ := kv.Key.(*ast.Ident)
		value := p.resolve(kv.Value)
		if call, ok := value.(*ast.CallExpr); ok && len(call.Args) == 1 {
			value = p.resolve(call.Args[0])
		}
		n, err := enumValue(value)
		if err != nil {
			return nil, err
		}
		fd := m.Descriptor().Fields().ByJSONName(lowerFirst(key.Name))
		if fd == nil {
			return nil, fmt.Errorf("proto.Glyphs has no %s", key.Name)
		}
		m.Set(fd, protoreflect.ValueOfInt32(n))
	}
	return glyphs, nil
}

func lowerFirst(s string) string {
	if s == "" {
		return s
	}
	return strings.ToLower(s[:1]) + s[1:]
}

// specField reads core.SpecOptionsCombo{SpecOptions: &proto.Player_FeralDruid{...}} as "feral_druid".
func (p *suitePackage) specField(expr ast.Expr) (string, error) {
	lit, ok := expr.(*ast.CompositeLit)
	if !ok || !isSelector(lit.Type, "core", "SpecOptionsCombo") {
		return "", fmt.Errorf("not a core.SpecOptionsCombo literal")
	}
	for _, elt := range lit.Elts {
		kv, ok := elt.(*ast.KeyValueExpr)
		if key, _ := kv.Key.(*ast.Ident); !ok || key == nil || key.Name != "SpecOptions" {
			continue
		}
		options, _ := p.resolve(kv.Value).(*ast.CompositeLit)
		var sel *ast.SelectorExpr
		if options != nil {
			sel, _ = options.Type.(*ast.SelectorExpr)
		}
		if sel == nil {
			return "", fmt.Errorf("spec options aren't a proto.Player_<Spec> literal")
		}
		wrapper := strings.TrimPrefix(sel.Sel.Name, "Player_")
		oneof := (&proto.Player{}).ProtoReflect().Descriptor().Oneofs().ByName("spec")
		for i := 0; i < oneof.Fields().Len(); i++ {
			field := oneof.Fields().Get(i)
			if camelCase(string(field.Name())) == wrapper {
				return string(field.Name()), nil
			}
		}
		return "", fmt.Errorf("no proto.Player spec field for %s", sel.Sel.Name)
	}
	return "", fmt.Errorf("no SpecOptions")
}

func camelCase(name string) string {
	var b strings.Builder
	for _, part := range strings.Split(name, "_") {
		b.WriteString(strings.ToUpper(part[:1]) + part[1:])
	}
	return b.String()
}

// goldcharSetupsFromSuites groups the non-healer suites by default player and works out everything
// about each the suites decide. Auras, probes and GlyphProperties ids are left for the list to give.
func goldcharSetupsFromSuites(root string) ([]goldcharSetup, error) {
	suites, err := readSuiteSetups(root)
	if err != nil {
		return nil, err
	}
	data, err := loadDamageData()
	if err != nil {
		return nil, err
	}
	glyphSpells := make(map[int32]int32, len(data.glyphItems))
	for spell, item := range data.glyphItems {
		glyphSpells[item] = spell
	}

	var setups []goldcharSetup
	for _, suite := range suites {
		if healerSpecs[suite.spec] {
			continue
		}
		setup, err := suite.goldchar(root, glyphSpells)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", suite.suite, err)
		}
		i := slices.IndexFunc(setups, setup.samePlayer)
		if i < 0 {
			setups = append(setups, setup)
		} else {
			setups[i].Suites = append(setups[i].Suites, setup.Suites...)
		}
	}
	return setups, nil
}

func (s suiteSetup) goldchar(root string, glyphSpells map[int32]int32) (goldcharSetup, error) {
	setup := goldcharSetup{Suites: []string{s.suite}, Spec: s.spec, GearSet: s.gearSet, Talents: s.talents}
	_, test, _ := strings.Cut(s.suite, " ")

	for id, c := range classes {
		if c.class == s.class {
			setup.Class = id
		}
	}
	for id, race := range races {
		if race == s.race {
			setup.Race = id
		}
	}
	className := azerothcore.ClassNames[int32(setup.Class)]
	if setup.Class == 0 || setup.Race == 0 || className == "" {
		return setup, fmt.Errorf("no server ids for %s %s", s.race, s.class)
	}
	setup.Name = camelCase(className) + strings.TrimPrefix(test, "Test")

	raw, err := os.ReadFile(filepath.Join(root, s.gearSet))
	if err != nil {
		return setup, err
	}
	var gear struct {
		Items []struct {
			ID int32 `json:"id"`
		} `json:"items"`
	}
	if err := json.Unmarshal(raw, &gear); err != nil {
		return setup, fmt.Errorf("%s: %w", s.gearSet, err)
	}
	if len(gear.Items) > len(serverEquipSlots) {
		return setup, fmt.Errorf("%s has %d items", s.gearSet, len(gear.Items))
	}
	for i, item := range gear.Items {
		if item.ID != 0 {
			setup.Items = append(setup.Items, goldcharItem{Slot: serverEquipSlots[i], ID: item.ID})
		}
	}

	setup.TalentSpells, err = talentStringSpells(filepath.Join(root, talentTreesDir, className+".json"), s.talents)
	if err != nil {
		return setup, err
	}

	for _, slot := range []struct {
		item   int32
		server uint8
	}{
		{s.glyphs.Major1, majorGlyphSlots[0]}, {s.glyphs.Major2, majorGlyphSlots[1]}, {s.glyphs.Major3, majorGlyphSlots[2]},
		{s.glyphs.Minor1, minorGlyphSlots[0]}, {s.glyphs.Minor2, minorGlyphSlots[1]}, {s.glyphs.Minor3, minorGlyphSlots[2]},
	} {
		if slot.item == 0 {
			continue
		}
		spell, ok := glyphSpells[slot.item]
		if !ok {
			return setup, fmt.Errorf("glyph item %d isn't in %s", slot.item, simDBJSON)
		}
		setup.Glyphs = append(setup.Glyphs, goldcharGlyph{Slot: slot.server, Item: slot.item, Spell: spell})
	}
	return setup, nil
}

type talentLocation struct {
	Row int `json:"rowIdx"`
	Col int `json:"colIdx"`
}

// talentTreeLayout is where a tree file puts each talent and the talent it depends on.
type talentTreeLayout []struct {
	Talents []talentPlace `json:"talents"`
}

type talentPlace struct {
	Location       talentLocation  `json:"location"`
	PrereqLocation *talentLocation `json:"prereqLocation"`
}

// talentStringSpells turns a talent string into the rank spell of each talent it takes, tree by tree in
// the tree file's order, except that a talent the server needs first goes first: Dual Wield sits after
// Dual Wield Specialization in its row.
func talentStringSpells(treeFile, talents string) ([]int32, error) {
	file, err := readTalentTreeFile(treeFile)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(treeFile)
	if err != nil {
		return nil, err
	}
	var layout talentTreeLayout
	if err := json.Unmarshal(data, &layout); err != nil {
		return nil, fmt.Errorf("%s: %w", treeFile, err)
	}
	trees := strings.Split(talents, "-")
	if len(trees) > len(file) {
		return nil, fmt.Errorf("%q has %d trees, %s %d", talents, len(trees), treeFile, len(file))
	}
	var spells []int32
	for i, digits := range trees {
		if len(digits) > len(file[i].Talents) {
			return nil, fmt.Errorf("%q: tree %d has %d talents", talents, i, len(file[i].Talents))
		}
		ranks := make([]int32, len(digits))
		for j, digit := range digits {
			ranks[j] = digit - '0'
			if talent := file[i].Talents[j]; ranks[j] < 0 || ranks[j] > talent.MaxPoints {
				return nil, fmt.Errorf("%q: %s at rank %c of %d", talents, talent.FieldName, digit, talent.MaxPoints)
			}
		}
		learned := make([]bool, len(digits))
		var learn func(j int)
		learn = func(j int) {
			if learned[j] {
				return
			}
			learned[j] = true
			if pre := layout[i].Talents[j].PrereqLocation; pre != nil {
				k := slices.IndexFunc(layout[i].Talents, func(t talentPlace) bool { return t.Location == *pre })
				if k >= 0 && k < len(ranks) && ranks[k] > 0 {
					learn(k)
				}
			}
			talent := file[i].Talents[j]
			spells = append(spells, rankSpells(talent.SpellIds, talent.MaxPoints)[ranks[j]-1])
		}
		for j := range ranks {
			if ranks[j] > 0 {
				learn(j)
			}
		}
	}
	return spells, nil
}

// samePlayer is whether two setups build the same character, whatever their names and suites.
func (s goldcharSetup) samePlayer(o goldcharSetup) bool {
	return s.Spec == o.Spec && s.Class == o.Class && s.Race == o.Race && s.GearSet == o.GearSet &&
		s.Talents == o.Talents && slices.Equal(s.glyphItems(), o.glyphItems())
}

func (s goldcharSetup) glyphItems() []int32 {
	var items []int32
	for _, g := range s.Glyphs {
		items = append(items, int32(g.Slot), g.Item)
	}
	return items
}

const goldcharFixture = "goldchar.jsonl"

// goldcharRecord is one line of a TestSimvalGoldenCharacters capture: a bot's info record, the only kind
// with a character, or the damage record of one of its probes.
type goldcharRecord struct {
	damageRecord
	Character *goldcharCharacter `json:"character"`
	// the attacker's sheet crit for the hands unitSnapshot leaves out
	critOffhand, critRanged float64
}

func (r *goldcharRecord) UnmarshalJSON(data []byte) error {
	type fields goldcharRecord
	if err := json.Unmarshal(data, (*fields)(r)); err != nil {
		return err
	}
	var hands struct {
		Attacker struct {
			Sheet struct {
				CritOffhand float64 `json:"critOffhand"`
				CritRanged  float64 `json:"critRanged"`
			} `json:"sheet"`
		} `json:"attacker"`
	}
	err := json.Unmarshal(data, &hands)
	r.critOffhand, r.critRanged = hands.Attacker.Sheet.CritOffhand, hands.Attacker.Sheet.CritRanged
	return err
}

// goldcharCharacter is what the server character has on.
type goldcharCharacter struct {
	Talents []int32 `json:"talents"` // rank spells, sorted
	// the GlyphProperties id in each slot, 0 when empty
	Glyphs    []int32 `json:"glyphs"`
	Equipment []struct {
		Slot   uint8 `json:"slot"`
		ItemID int32 `json:"itemId"`
	} `json:"equipment"`
}

func (r goldcharRecord) label() string {
	if r.Character == nil {
		return r.damageRecord.label()
	}
	return fmt.Sprintf("info: %s vs %s", r.Attacker.Name, r.Target.Name)
}

func readGoldcharRecords(path string) ([]goldcharRecord, error) {
	return readJSONLines[goldcharRecord](path)
}

// goldcharBot is one bot's records. Bot names are new every run, so only the info record's character
// tells which setup the bot plays.
type goldcharBot struct {
	name   string
	info   *goldcharRecord
	damage []damageRecord
}

func goldcharBots(records []goldcharRecord) ([]*goldcharBot, error) {
	var bots []*goldcharBot
	byName := map[string]*goldcharBot{}
	for i := range records {
		rec := &records[i]
		bot := byName[rec.Attacker.Name]
		if bot == nil {
			bot = &goldcharBot{name: rec.Attacker.Name}
			byName[bot.name] = bot
			bots = append(bots, bot)
		}
		switch {
		case rec.Command == "info" && rec.Character != nil:
			if bot.info != nil {
				return nil, fmt.Errorf("%s has two info records", bot.name)
			}
			bot.info = rec
		case rec.Command == "damage":
			bot.damage = append(bot.damage, rec.damageRecord)
		default:
			return nil, fmt.Errorf("%s has a %q record without a character", bot.name, rec.Command)
		}
	}
	for _, bot := range bots {
		if bot.info == nil {
			return nil, fmt.Errorf("%s has no info record", bot.name)
		}
	}
	return bots, nil
}

// builds is whether the server character is this setup's: its class and race, and exactly its talents,
// glyphs and items, as TestSimvalGoldenCharacters checks them.
func (s goldcharSetup) builds(class, race uint8, c *goldcharCharacter) bool {
	if class != s.Class || race != s.Race {
		return false
	}
	if !slices.Equal(c.Talents, slices.Sorted(slices.Values(s.TalentSpells))) {
		return false
	}
	glyphs := make([]int32, len(majorGlyphSlots)+len(minorGlyphSlots))
	for _, g := range s.Glyphs {
		if int(g.Slot) >= len(glyphs) {
			return false
		}
		glyphs[g.Slot] = g.Glyph
	}
	if !slices.Equal(c.Glyphs, glyphs) || len(c.Equipment) != len(s.Items) {
		return false
	}
	for _, e := range c.Equipment {
		if !slices.Contains(s.Items, goldcharItem{Slot: e.Slot, ID: e.ItemID}) {
			return false
		}
	}
	return true
}

func matchGoldcharSetup(setups []goldcharSetup, info *goldcharRecord) (goldcharSetup, error) {
	var names []string
	var found goldcharSetup
	for _, s := range setups {
		if s.builds(info.Attacker.Class, info.Attacker.Race, info.Character) {
			names = append(names, s.Name)
			found = s
		}
	}
	switch len(names) {
	case 1:
		return found, nil
	case 0:
		return found, fmt.Errorf("%s: no setup has class %d, race %d and %+v", info.Attacker.Name,
			info.Attacker.Class, info.Attacker.Race, *info.Character)
	default:
		return found, fmt.Errorf("%s: setups %v build the same character", info.Attacker.Name, names)
	}
}

// goldcharPlayer is the setup's character the way the golden suites build it, minus the gems, enchants,
// buffs, consumes and spec options the server character doesn't get. Glyphs the server lacks are left
// off too.
func goldcharPlayer(s goldcharSetup) (*proto.Player, error) {
	class, ok := classes[s.Class]
	if !ok {
		return nil, fmt.Errorf("no sim class for server class %d", s.Class)
	}
	race, ok := races[s.Race]
	if !ok {
		return nil, fmt.Errorf("no sim race for server race %d", s.Race)
	}

	items := make([]*proto.ItemSpec, len(serverEquipSlots))
	for i := range items {
		items[i] = &proto.ItemSpec{}
	}
	for _, item := range s.Items {
		i := slices.Index(serverEquipSlots[:], item.Slot)
		if i < 0 {
			return nil, fmt.Errorf("item %d in server slot %d, which the sim has no slot for", item.ID, item.Slot)
		}
		items[i].Id = item.ID
	}

	glyphs := &proto.Glyphs{}
	majors := []*int32{&glyphs.Major1, &glyphs.Major2, &glyphs.Major3}
	minors := []*int32{&glyphs.Minor1, &glyphs.Minor2, &glyphs.Minor3}
	for _, g := range s.Glyphs {
		if g.Glyph == 0 {
			continue
		}
		if i := slices.Index(majorGlyphSlots[:], g.Slot); i >= 0 {
			*majors[i] = g.Item
		} else if i := slices.Index(minorGlyphSlots[:], g.Slot); i >= 0 {
			*minors[i] = g.Item
		} else {
			return nil, fmt.Errorf("glyph %d in server slot %d", g.Item, g.Slot)
		}
	}

	player := &proto.Player{
		Name:          s.Name,
		Race:          race,
		Class:         class.class,
		Equipment:     &proto.EquipmentSpec{Items: items},
		Consumes:      &proto.Consumes{},
		Buffs:         &proto.IndividualBuffs{},
		TalentsString: s.Talents,
		Glyphs:        glyphs,
	}
	return player, setSpec(player, protoreflect.Name(s.Spec))
}

// newGoldcharSim is the setup's character against the record's dummy, with the setup's auras. The sim
// has most of them up after a reset anyway; one it has no aura for (Moonkin Form, Shadowform, a raid
// buff) comes with the talent or class there.
func newGoldcharSim(rec damageRecord, s goldcharSetup) (*damageSim, error) {
	player, err := goldcharPlayer(s)
	if err != nil {
		return nil, err
	}
	return newDamageSim(rec, player, s.Auras)
}

// checkGoldcharDamage holds a probe's damage record to the shaped reason the list expects, then compares
// an unshaped one like checkDamage does, on the setup's character.
func checkGoldcharDamage(rec damageRecord, s goldcharSetup) []Check {
	i := slices.IndexFunc(s.Probes, func(p goldcharProbe) bool { return p.ID == rec.Spell.ID })
	if i < 0 {
		return []Check{fail("probe", "not one of the setup's probes")}
	}
	want := s.Probes[i].Shaped
	switch {
	case want != "" && strings.Contains(rec.Shaped, want):
		return []Check{pass("shaped", rec.Shaped)}
	case want != "":
		return []Check{fail("shaped", fmt.Sprintf("%q, the list expects %q", rec.Shaped, want))}
	case rec.Shaped != "":
		return []Check{fail("shaped", fmt.Sprintf("%q, the list expects it compared", rec.Shaped))}
	}
	return replayDamage(rec, func(rec damageRecord) (*damageSim, error) { return newGoldcharSim(rec, s) })
}

// The server's magic schools in a snapshot's spellPower, holy (mask 2) through arcane (mask 64).
var magicSchools = []string{"holy", "fire", "nature", "frost", "shadow", "arcane"}

const spelldumpPath = "assets/db_inputs/acore/spelldump.jsonl"

// SPELL_AURA_MOD_INCREASES_SPELL_PCT_TO_HIT (Balance of Power, Suppression, Elemental Precision):
// spell hit for the schools in its misc value. MagicSpellHitResult adds it per spell, so it's missing
// from the sheet's spell hit.
const auraSchoolSpellHit = 199

type schoolHit struct {
	mask uint32
	pct  float64
}

// loadSchoolHitAuras reads the spells with a school spell hit effect out of the live server's spell
// capture.
var loadSchoolHitAuras = sync.OnceValues(func() (map[int32][]schoolHit, error) {
	root, err := checkoutRoot()
	if err != nil {
		return nil, err
	}
	f, err := os.Open(filepath.Join(root, spelldumpPath))
	if err != nil {
		return nil, err
	}
	defer f.Close()

	auras := map[int32][]schoolHit{}
	marker := []byte(fmt.Sprintf(`"aura":%d,`, auraSchoolSpellHit))
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64<<10), 8<<20)
	for scanner.Scan() {
		if !bytes.Contains(scanner.Bytes(), marker) {
			continue
		}
		var spell struct {
			ID      int32 `json:"id"`
			Effects []struct {
				Aura       int32  `json:"aura"`
				BasePoints int32  `json:"basePoints"`
				DieSides   int32  `json:"dieSides"`
				MiscValue  uint32 `json:"miscValue"`
			} `json:"effects"`
		}
		if err := json.Unmarshal(scanner.Bytes(), &spell); err != nil {
			return nil, fmt.Errorf("%s: %w", spelldumpPath, err)
		}
		for _, e := range spell.Effects {
			if e.Aura == auraSchoolSpellHit {
				auras[spell.ID] = append(auras[spell.ID], schoolHit{e.MiscValue, float64(e.BasePoints + min(e.DieSides, 1))})
			}
		}
	}
	return auras, scanner.Err()
})

// spellHitEverySchool is the school spell hit the unit's auras give every magic school.
func spellHitEverySchool(unit unitSnapshot) (float64, error) {
	hitAuras, err := loadSchoolHitAuras()
	if err != nil {
		return 0, err
	}
	lowest := math.Inf(1)
	for i := range magicSchools {
		school := uint32(2) << i
		var pct float64
		for _, a := range unit.Auras {
			for _, hit := range hitAuras[a.ID] {
				if hit.mask&school != 0 {
					pct += hit.pct
				}
			}
		}
		lowest = min(lowest, pct)
	}
	return lowest, nil
}

// checkGoldcharInfo compares the info record's sheet with the setup's character in the sim, with its
// starting auras: spell power, spell crit and hit, and for each hand the sim swings, its attack speed,
// weapon damage, crit, hit and expertise.
func checkGoldcharInfo(rec goldcharRecord, s goldcharSetup) (checks []Check) {
	unit := rec.Attacker
	if !unit.IsPlayer || unit.Level != core.CharacterLevel || unit.Sheet == nil {
		return []Check{fail("attacker", fmt.Sprintf("level %d player=%v; the replay builds a level %d player",
			unit.Level, unit.IsPlayer, core.CharacterLevel))}
	}
	d, err := newGoldcharSim(rec.damageRecord, s)
	if err != nil {
		return []Check{fail("sim character", err.Error())}
	}
	defer func() {
		if r := recover(); r != nil {
			checks = append(checks, fail("sim sheet", fmt.Sprintf("sim panic: %v", r)))
		}
	}()
	checks = d.sheetChecks(rec)

	var auras string
	for i, check := range checks {
		if check.Passed {
			continue
		}
		if auras == "" {
			auras = d.unmatchedAuras()
		}
		checks[i].Detail += "; " + auras
	}
	return checks
}

// sheetHand is one hand the sim swings, with what it has for the server's sheet.
type sheetHand struct {
	attack string // the snapshot's attack type
	slot   proto.ItemSlot
	weapon *core.Weapon
	spell  *core.Spell
	// the attack speed multiplier, haste included
	speed float64
}

func (d *damageSim) sheetHands() []sheetHand {
	aa := &d.char.AutoAttacks
	var hands []sheetHand
	if aa.AutoSwingMelee && aa.MHAuto() != nil {
		hands = append(hands, sheetHand{"mainhand", proto.ItemSlot_ItemSlotMainHand, aa.MH(), aa.MHAuto(), d.char.SwingSpeed()})
		if aa.IsDualWielding && aa.OHAuto() != nil {
			hands = append(hands, sheetHand{"offhand", proto.ItemSlot_ItemSlotOffHand, aa.OH(), aa.OHAuto(), d.char.SwingSpeed()})
		}
	}
	if aa.AutoSwingRanged && aa.RangedAuto() != nil {
		hands = append(hands, sheetHand{"ranged", proto.ItemSlot_ItemSlotRanged, aa.Ranged(), aa.RangedAuto(), d.char.RangedSwingSpeed()})
	}
	return hands
}

func (d *damageSim) sheetChecks(rec goldcharRecord) []Check {
	unit := rec.Attacker
	d.start()
	s := d.char.GetStats()
	class := classes[unit.Class].class

	// The server works out crit from whole primary stats, so the sim's can sit up to the dropped
	// fraction's worth above it.
	truncation := func(stat stats.Stat, pctPerPoint float64) float64 {
		return 1e-3 + (s[stat]-math.Floor(s[stat]+1e-6))*pctPerPoint
	}
	critPerAgility := 100 * core.ClassStatScaling[class].MeleeCritPerAgility

	if len(unit.SpellPower) == 0 || len(unit.Sheet.SpellCrit) <= len(magicSchools) {
		return []Check{fail("snapshot", "no spellPower or spellCrit per school")}
	}
	// school bonuses only add, so the lowest school holds what every spell gets
	var spellPower, spellCrit []float64
	for i, school := range magicSchools {
		spellPower = append(spellPower, unit.SpellPower[school])
		spellCrit = append(spellCrit, unit.Sheet.SpellCrit[i+1])
	}
	spellHit := checkNear("spell hit %", s[stats.SpellHit]/core.SpellHitRatingPerHitChance, unit.HitMods.Spell, 1e-3)
	if schoolHit, err := spellHitEverySchool(unit.unitSnapshot); err != nil {
		spellHit = fail(spellHit.Name, err.Error())
	} else if schoolHit > 0 {
		spellHit = checkNear("spell hit %", s[stats.SpellHit]/core.SpellHitRatingPerHitChance, unit.HitMods.Spell+schoolHit, 1e-3)
		spellHit.Detail += fmt.Sprintf(" (%.0f of it school hit)", schoolHit)
	}
	checks := []Check{
		checkSpellPower(s[stats.SpellPower], slices.Min(spellPower), slices.Max(spellPower)),
		checkNear("spell crit %", s[stats.SpellCrit]/core.CritRatingPerCritChance, slices.Min(spellCrit),
			truncation(stats.Intellect, 100*core.SpellCritPerIntellect)),
		spellHit,
	}

	for _, hand := range d.sheetHands() {
		attack := unit.attack(hand.attack)
		name := hand.attack + " "
		unhasted := hand.weapon.SwingSpeed * 1000
		checks = append(checks,
			checkTruncated(name+"speed ms", unhasted, float64(attack.UnhastedMs), 1),
			checkTruncated(name+"hasted speed ms", unhasted/hand.speed, float64(attack.HastedMs), 2))

		// the server reports the item's damage in a feral form too, where the sim swings the form's
		if item := d.char.Equipment[hand.slot]; item.ID != 0 && item.SwingSpeed > 0 {
			checks = append(checks,
				checkNear(name+"weapon damage min", item.WeaponDamageMin, attack.DamageMin, 0.5),
				checkNear(name+"weapon damage max", item.WeaponDamageMax, attack.DamageMax, 0.5))
		}

		sheetCrit := map[string]float64{
			"mainhand": unit.Sheet.CritMainhand, "offhand": rec.critOffhand, "ranged": rec.critRanged,
		}[hand.attack]
		checks = append(checks, checkNear(name+"crit %",
			(s[stats.MeleeCrit]+hand.spell.BonusCritRating)/core.CritRatingPerCritChance, sheetCrit,
			truncation(stats.Agility, critPerAgility)))

		// the server has one hit chance for both hands
		switch hand.attack {
		case "mainhand":
			checks = append(checks, checkNear(name+"hit %",
				(s[stats.MeleeHit]+hand.spell.BonusHitRating)/core.MeleeHitRatingPerHitChance, unit.HitMods.Melee, 1e-3))
		case "ranged":
			checks = append(checks, checkNear(name+"hit %",
				(s[stats.MeleeHit]+hand.spell.BonusHitRating)/core.MeleeHitRatingPerHitChance, unit.HitMods.Ranged, 1e-3))
		}
		if hand.attack != "ranged" {
			checks = append(checks, checkNear(name+"expertise %", hand.spell.ExpertisePercentage()*100,
				attack.ExpertiseReduction, 1e-3))
		}
	}
	return checks
}

// checkSpellPower allows for the server truncating each spell power bonus it works out from a stat or
// attack power, which only ever lowers its number.
func checkSpellPower(sim, server, serverMax float64) Check {
	detail := fmt.Sprintf("sim %.2f, server %.0f", sim, server)
	if serverMax > server {
		detail += fmt.Sprintf(" (%.0f in its best school)", serverMax)
	}
	below := max(server*0.001, 1)
	if diff := sim - server; diff >= -below && diff <= below+1 {
		return pass("spell power", detail)
	}
	return fail("spell power", detail)
}

// checkTruncated passes when the server's whole number is the sim's value cut down by less than slack,
// the server truncating once or twice on the way.
func checkTruncated(name string, sim, server, slack float64) Check {
	detail := fmt.Sprintf("sim %.2f, server %.0f", sim, server)
	if diff := sim - server; diff >= -0.01 && diff < slack+0.01 {
		return pass(name, detail)
	}
	return fail(name, detail)
}
