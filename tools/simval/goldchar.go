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
	// the gear set's items by server equipment slot
	Items []goldcharItem `json:"items"`
	// The gear set's enchants and gems, which only the golden characters wear: buffs.go's and the sweeps'
	// characters go without, as the server characters behind their captures did.
	Enchants []goldcharEnchant `json:"enchants,omitempty"`
	Gems     []goldcharGem     `json:"gems,omitempty"`
	Talents  string            `json:"talents"`
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
	// set by wornBy for a capture whose character wears Enchants and Gems, so the sim's does too
	geared bool
}

type goldcharItem struct {
	Slot uint8 `json:"slot"` // EQUIPMENT_SLOT_*
	ID   int32 `json:"id"`
}

// goldcharEnchant is a SpellItemEnchantment id for `.simval enchant`: one the sim knows from the gear set,
// or one that adds the socket a gem past the item's sockets needs.
type goldcharEnchant struct {
	Slot    uint8 `json:"slot"`
	Enchant int32 `json:"enchant"`
}

// goldcharGem is a gem for `.simval gem`, by the socket's index in the gear set's list.
type goldcharGem struct {
	Slot   uint8 `json:"slot"`
	Socket uint8 `json:"socket"`
	Gem    int32 `json:"gem"`
}

// socketEnchants add a socket to the item in a server slot: Eternal Belt Buckle, and Blacksmithing's
// Socket Bracer and Socket Gloves. The sim takes a gem past an item's sockets as if it had one.
var socketEnchants = map[uint8]int32{5: 3729, 8: 3717, 9: 3723}

// MAX_GEM_SOCKETS, an added socket included
const maxGemSockets = 3

// goldcharGearData is what db.json says about the gear sets' gems and enchants.
type goldcharGearData struct {
	sockets  map[int32]int // per item
	enchants map[int32]bool
}

var loadGoldcharGearData = sync.OnceValues(func() (*goldcharGearData, error) {
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
			ID         int32   `json:"id"`
			GemSockets []int32 `json:"gemSockets"`
		} `json:"items"`
		Enchants []struct {
			EffectID int32 `json:"effectId"`
		} `json:"enchants"`
	}
	if err := json.Unmarshal(raw, &db); err != nil {
		return nil, fmt.Errorf("%s: %w", simDBJSON, err)
	}
	data := &goldcharGearData{sockets: map[int32]int{}, enchants: map[int32]bool{}}
	for _, item := range db.Items {
		data.sockets[item.ID] = len(item.GemSockets)
	}
	for _, enchant := range db.Enchants {
		data.enchants[enchant.EffectID] = true
	}
	return data, nil
})

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

	pkg := &suitePackage{values: map[string]ast.Expr{}, configs: map[string]*ast.CompositeLit{}}
	for _, f := range files {
		for _, decl := range f.Decls {
			if fn, ok := decl.(*ast.FuncDecl); ok {
				if lit := returnedSuiteConfig(fn); lit != nil {
					pkg.configs[fn.Name.Name] = lit
				}
				continue
			}
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
				if call, isCall := n.(*ast.CallExpr); isCall {
					// a config a function of the package hands the test
					if name, isName := call.Fun.(*ast.Ident); isName && pkg.configs[name.Name] != nil {
						lit, ok = pkg.configs[name.Name], true
					}
				}
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

// returnedSuiteConfig is the core.CharacterSuiteConfig literal a function returns as its whole body, or nil.
func returnedSuiteConfig(fn *ast.FuncDecl) *ast.CompositeLit {
	if fn.Body == nil || len(fn.Body.List) != 1 {
		return nil
	}
	ret, ok := fn.Body.List[0].(*ast.ReturnStmt)
	if !ok || len(ret.Results) != 1 {
		return nil
	}
	lit, ok := ret.Results[0].(*ast.CompositeLit)
	if !ok || !isSelector(lit.Type, "core", "CharacterSuiteConfig") {
		return nil
	}
	return lit
}

// suitePackage holds a package's top-level vars and consts, which suite configs name, and the configs its
// functions return, by function.
type suitePackage struct {
	configs map[string]*ast.CompositeLit
	values  map[string]ast.Expr
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

// gearSetPath reads core.GetGearSet(dir, name), whose dir is relative to the suite's package, or
// filepath.Join(root, dir) with dir from the checkout's root.
func (p *suitePackage) gearSetPath(expr ast.Expr, pkgDir string) (string, error) {
	call, ok := expr.(*ast.CallExpr)
	if !ok || !isSelector(call.Fun, "core", "GetGearSet") || len(call.Args) != 2 {
		return "", fmt.Errorf("not a core.GetGearSet call")
	}
	dirExpr := p.resolve(call.Args[0])
	if join, ok := dirExpr.(*ast.CallExpr); ok && isSelector(join.Fun, "filepath", "Join") && len(join.Args) == 2 {
		pkgDir, dirExpr = "", p.resolve(join.Args[1])
	}
	dir, err := p.stringValue(dirExpr)
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
	glyphSpells, err := glyphItemSpells()
	if err != nil {
		return nil, err
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

// glyphItemSpells maps each glyph item to the glyph spell it teaches.
func glyphItemSpells() (map[int32]int32, error) {
	data, err := loadDamageData()
	if err != nil {
		return nil, err
	}
	spells := make(map[int32]int32, len(data.glyphItems))
	for spell, item := range data.glyphItems {
		spells[item] = spell
	}
	return spells, nil
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
		Items []gearSetItem `json:"items"`
	}
	if err := json.Unmarshal(raw, &gear); err != nil {
		return setup, fmt.Errorf("%s: %w", s.gearSet, err)
	}
	err = setup.fill(root, s.gearSet, gear.Items, s.glyphs, glyphSpells)
	return setup, err
}

// gearSetItem is an item as a gear set file or a page's equipment lists it, in the sim's ItemSlot order.
type gearSetItem struct {
	ID      int32   `json:"id"`
	Enchant int32   `json:"enchant"`
	Gems    []int32 `json:"gems"`
}

// fill works out the server's items, enchants, gems, talent spells and glyphs for the class and talents the
// setup already has. source names where the items came from, for an error.
func (setup *goldcharSetup) fill(root, source string, items []gearSetItem, glyphs *proto.Glyphs,
	glyphSpells map[int32]int32) error {
	className := azerothcore.ClassNames[int32(setup.Class)]
	if len(items) > len(serverEquipSlots) {
		return fmt.Errorf("%s has %d items", source, len(items))
	}
	gearData, err := loadGoldcharGearData()
	if err != nil {
		return err
	}
	for i, item := range items {
		if item.ID == 0 {
			continue
		}
		slot := serverEquipSlots[i]
		setup.Items = append(setup.Items, goldcharItem{Slot: slot, ID: item.ID})
		// the sim drops an enchant id it doesn't know, like a spell or item id in a gear set
		if gearData.enchants[item.Enchant] {
			setup.Enchants = append(setup.Enchants, goldcharEnchant{Slot: slot, Enchant: item.Enchant})
		}
		sockets := gearData.sockets[item.ID]
		for socket, gem := range item.Gems {
			switch {
			case gem == 0:
				continue
			case socket > sockets || socket == sockets && socketEnchants[slot] == 0 || socket >= maxGemSockets:
				return fmt.Errorf("%s: item %d has %d sockets and no way to add one, but a gem in socket %d",
					source, item.ID, sockets, socket)
			case socket == sockets:
				setup.Enchants = append(setup.Enchants, goldcharEnchant{Slot: slot, Enchant: socketEnchants[slot]})
			}
			setup.Gems = append(setup.Gems, goldcharGem{Slot: slot, Socket: uint8(socket), Gem: gem})
		}
	}

	setup.TalentSpells, err = talentStringSpells(filepath.Join(root, talentTreesDir, className+".json"), setup.Talents)
	if err != nil {
		return err
	}

	for _, slot := range []struct {
		item   int32
		server uint8
	}{
		{glyphs.GetMajor1(), majorGlyphSlots[0]}, {glyphs.GetMajor2(), majorGlyphSlots[1]}, {glyphs.GetMajor3(), majorGlyphSlots[2]},
		{glyphs.GetMinor1(), minorGlyphSlots[0]}, {glyphs.GetMinor2(), minorGlyphSlots[1]}, {glyphs.GetMinor3(), minorGlyphSlots[2]},
	} {
		if slot.item == 0 {
			continue
		}
		spell, ok := glyphSpells[slot.item]
		if !ok {
			return fmt.Errorf("glyph item %d isn't in %s", slot.item, simDBJSON)
		}
		setup.Glyphs = append(setup.Glyphs, goldcharGlyph{Slot: slot.server, Item: slot.item, Spell: spell})
	}
	return nil
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
	// the record as the talent sweep's trim reads an info record
	raw rawTalentInfo
	// nil in a record from before the module wrote a player's regen
	regen *timingRegen
}

func (r *goldcharRecord) UnmarshalJSON(data []byte) error {
	type fields goldcharRecord
	if err := json.Unmarshal(data, (*fields)(r)); err != nil {
		return err
	}
	if err := json.Unmarshal(data, &r.raw); err != nil {
		return err
	}
	if sheet := r.raw.Attacker.Sheet; sheet != nil {
		r.critOffhand, r.critRanged = sheet.CritOffhand, sheet.CritRanged
	}
	var regen struct {
		Attacker struct {
			Regen *timingRegen `json:"regen"`
		} `json:"attacker"`
	}
	if err := json.Unmarshal(data, &regen); err != nil {
		return err
	}
	r.regen = regen.Attacker.Regen
	return nil
}

// sheet is the info record's sheet the way the talent sweep reads one, without max power when the
// record predates the module's maxPower.
func (r goldcharRecord) sheet() (sweepSheet, error) {
	raw := r.raw
	noMaxPower := raw.Attacker.MaxPower == nil
	if noMaxPower {
		raw.Attacker.MaxPower = new(float64)
	}
	rec, err := raw.project()
	if err != nil {
		return nil, err
	}
	sheet, err := rec.sheet()
	if err == nil && noMaxPower {
		delete(sheet, sweepPowerFields[rec.PowerType].name)
	}
	return sheet, err
}

// goldcharCharacter is what the server character has on.
type goldcharCharacter struct {
	Talents []int32 `json:"talents"` // rank spells, sorted
	// the GlyphProperties id in each slot, 0 when empty
	Glyphs    []int32            `json:"glyphs"`
	Equipment []goldcharEquipped `json:"equipment"`
}

// goldcharEquipped is an item the server character has on, with its enchantments by SpellItemEnchantment
// id and its gems by item id.
type goldcharEquipped struct {
	Slot   uint8 `json:"slot"`
	ItemID int32 `json:"itemId"`
	// one per socket, 0 when empty; nil in a record from before the module wrote enchantments
	Gems        []int32 `json:"gems"`
	Enchant     int32   `json:"enchant"`
	Prismatic   int32   `json:"prismatic"`
	SocketBonus int32   `json:"socketBonus"`
	// what Player::ApplyEnchantment would skip now
	Inactive []goldcharInactive `json:"inactive"`
}

type goldcharInactive struct {
	Enchant int32 `json:"enchant"`
	// "condition" for a meta gem whose colors the gear doesn't meet
	Reason string `json:"reason"`
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

// wornBy is the setup geared when the character wears exactly its enchants and gems, as
// TestSimvalGoldenCharacters checks them, and as it is when the record predates the module writing them.
// Other enchants or gems, none included, are an error.
func (s goldcharSetup) wornBy(c *goldcharCharacter) (goldcharSetup, error) {
	if !slices.ContainsFunc(c.Equipment, func(e goldcharEquipped) bool { return e.Gems != nil }) {
		return s, nil
	}
	enchants := map[uint8][]int32{}
	for _, e := range s.Enchants {
		enchants[e.Slot] = append(enchants[e.Slot], e.Enchant)
	}
	gems := map[uint8][maxGemSockets]int32{}
	for _, g := range s.Gems {
		if int(g.Socket) >= maxGemSockets {
			return s, fmt.Errorf("%s: gem %d in socket %d", s.Name, g.Gem, g.Socket)
		}
		socketed := gems[g.Slot]
		socketed[g.Socket] = g.Gem
		gems[g.Slot] = socketed
	}

	var wrong []string
	for _, e := range c.Equipment {
		var have []int32
		for _, id := range []int32{e.Enchant, e.Prismatic} {
			if id != 0 {
				have = append(have, id)
			}
		}
		want := slices.Sorted(slices.Values(enchants[e.Slot]))
		if slices.Sort(have); !slices.Equal(have, want) {
			wrong = append(wrong, fmt.Sprintf("slot %d enchants %v, want %v", e.Slot, have, want))
		}
		var socketed [maxGemSockets]int32
		copy(socketed[:], e.Gems)
		if socketed != gems[e.Slot] {
			wrong = append(wrong, fmt.Sprintf("slot %d gems %v, want %v", e.Slot, socketed, gems[e.Slot]))
		}
	}
	if len(wrong) > 0 {
		return s, fmt.Errorf("%s's character doesn't wear its enchants and gems: %s", s.Name, strings.Join(wrong, "; "))
	}
	s.geared = true
	return s, nil
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

// goldcharPlayer is the setup's character the way the golden suites build it, minus the buffs, consumes
// and spec options the server character doesn't get, and minus the gems and enchants unless the setup is
// geared. Glyphs the server lacks are left off too.
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
	if s.geared {
		if err := s.gearItems(items); err != nil {
			return nil, err
		}
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

// gearItems puts the setup's enchants and gems on the sim's items, indexed by proto.ItemSlot. The sim
// has no socket-adding enchants: it takes a gem past the item's sockets as one more socket.
func (s goldcharSetup) gearItems(items []*proto.ItemSpec) error {
	item := func(slot uint8) (*proto.ItemSpec, error) {
		i := slices.Index(serverEquipSlots[:], slot)
		if i < 0 || items[i].Id == 0 {
			return nil, fmt.Errorf("an enchant or gem for server slot %d, which has no item", slot)
		}
		return items[i], nil
	}
	for _, e := range s.Enchants {
		if socketEnchants[e.Slot] == e.Enchant {
			continue
		}
		spec, err := item(e.Slot)
		if err != nil {
			return err
		}
		if spec.Enchant != 0 {
			return fmt.Errorf("enchants %d and %d for server slot %d", spec.Enchant, e.Enchant, e.Slot)
		}
		spec.Enchant = e.Enchant
	}
	for _, g := range s.Gems {
		spec, err := item(g.Slot)
		if err != nil {
			return err
		}
		for len(spec.Gems) <= int(g.Socket) {
			spec.Gems = append(spec.Gems, 0)
		}
		spec.Gems[g.Socket] = g.Gem
	}
	return nil
}

// newGoldcharSim is the setup's character against the record's dummy, with the setup's auras. The sim
// has most of them up after a reset anyway; one it has no aura for (Moonkin Form, Shadowform, a raid
// buff) comes with the talent or class there.
func newGoldcharSim(rec damageRecord, s goldcharSetup) (*damageSim, error) {
	player, err := goldcharPlayer(s)
	if err != nil {
		return nil, err
	}
	mirror := s.Auras
	// the server's bot stood still, so it had the buff Sniper Training gives (64420); the sim only
	// registers the aura (53304) for an uptime above 0
	if hunter := player.GetHunter(); hunter != nil && slices.ContainsFunc(rec.Attacker.Auras, func(a aura) bool { return a.ID == 64420 }) {
		hunter.Options.SniperTrainingUptime = 1
		mirror = append(slices.Clone(mirror), 53304)
	}
	// Prey on the Weak's buff comes and goes with the health of the rogue and the dummy
	if slices.ContainsFunc(rec.Attacker.Auras, func(a aura) bool { return a.ID == 58670 }) {
		mirror = append(slices.Clone(mirror), 58670)
	}
	return newDamageSim(rec, player, mirror, 1)
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
// starting auras: the talent sweep's sheet fields (checkGoldcharSheet), spell hit, for each hand the
// sim swings its attack speed, weapon damage, crit, hit and expertise, a geared character's socket
// bonuses and enchantments (gearChecks), and the regen when the record has it.
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
	checks = append(checkGoldcharSheet(rec, d.sweepSim()), d.sheetChecks(rec)...)
	if s.geared && rec.Character != nil {
		checks = append(checks, d.gearChecks(rec.Character)...)
	}
	if rec.regen != nil {
		checks = append(checks, d.goldcharRegen(rec)...)
	}

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

// gearChecks hold each item's socket bonus and enchantments on the server to the sim's: the sim gives the
// bonus when every socket's gem matches its color, a meta gem when its color condition is met
// (HasMetaGemEquipped), and always counts every enchant.
func (d *damageSim) gearChecks(c *goldcharCharacter) []Check {
	var checks []Check
	for _, e := range c.Equipment {
		name := fmt.Sprintf("slot %d ", e.Slot)
		i := slices.Index(serverEquipSlots[:], e.Slot)
		if i < 0 {
			checks = append(checks, fail(name+"item", "the sim has no slot for it"))
			continue
		}
		item := d.char.Equipment[i]
		if on, ok := simSocketBonus(&item); ok {
			checks = append(checks, checkOn(name+"socket bonus", on, e.SocketBonus != 0))
		}

		metaAt := slices.IndexFunc(item.Gems, func(g core.Gem) bool { return g.Color == proto.GemColor_GemColorMeta })
		metaOff := false
		for _, off := range e.Inactive {
			if off.Reason == "condition" {
				metaOff = true
				continue
			}
			checks = append(checks, fail(fmt.Sprintf("%senchant %d", name, off.Enchant),
				fmt.Sprintf("the server skips it (%s), the sim doesn't", off.Reason)))
		}
		if metaAt >= 0 || metaOff {
			// the sim's own read of the gem's color condition
			simOn := metaAt >= 0 && d.char.HasMetaGemEquipped(item.Gems[metaAt].ID)
			checks = append(checks, checkOn(name+"meta gem", simOn, !metaOff))
		}
	}
	return checks
}

// simSocketBonus is whether the sim gives the item its socket bonus, read off its total stats. False
// for an item without sockets or a bonus.
func simSocketBonus(item *core.Item) (on, ok bool) {
	if len(item.GemSockets) == 0 || item.SocketBonus == (stats.Stats{}) {
		return false, false
	}
	without := item.Stats
	without.AddInplace(&item.Enchant.Stats)
	for i := range item.Gems {
		without.AddInplace(&item.Gems[i].Stats)
	}
	return item.TotalStats() != without, true
}

func checkOn(name string, sim, server bool) Check {
	state := map[bool]string{true: "on", false: "off"}
	detail := fmt.Sprintf("sim %s, server %s", state[sim], state[server])
	if sim == server {
		return pass(name, detail)
	}
	return fail(name, detail)
}

// goldcharRegen holds the record's regen to the sim the way the timing replay does a bot's info record.
// That pins intellect and spirit to the record's, so it starts the sim over first.
func (d *damageSim) goldcharRegen(rec goldcharRecord) []Check {
	ts := &timingSim{damageSim: d}
	ts.start()
	var info timingRecord
	info.Command = "info"
	info.Attacker = timingUnit{damageUnit: rec.Attacker, Regen: rec.regen}
	return ts.checkRegen(info)
}

// goldcharSheetFields are the talent sweep's sheet fields an info record is held to, besides each
// school's spell power and crit.
var goldcharSheetFields = []string{
	"strength", "agility", "stamina", "intellect", "spirit",
	"max health", "max mana", "max rage", "max energy", "max runic power",
	"armor", "fire resistance", "nature resistance", "frost resistance", "shadow resistance", "arcane resistance",
	"attack power", "ranged attack power", "dodge %", "parry %", "block %", "block value", "armor penetration %",
}

// checkGoldcharSheet holds the info record's sheet to the sim's: each field can sit as far below the
// sim's as its sweepSim bound, and no further above than float noise. A field the sim leaves out is 0
// there (no parry, block or bar), except block value, which the server reports without a shield too.
// Every school gets the sim's one spell power, and the crit of the sim's spells of that school, or the
// stat's when it has none.
func checkGoldcharSheet(rec goldcharRecord, sim sweepSim) []Check {
	if sim.err != nil {
		return []Check{fail("sim sheet", sim.err.Error())}
	}
	server, err := rec.sheet()
	if err != nil {
		return []Check{fail("server sheet", err.Error())}
	}
	type field struct{ name, sim string }
	var fields []field
	for _, name := range goldcharSheetFields {
		fields = append(fields, field{name, name})
	}
	for _, school := range magicSchools {
		power, crit := school+" spell power", school+" spell crit %"
		server[power] = rec.Attacker.SpellPower[school]
		fields = append(fields, field{power, "spell power"})
		if _, ok := sim.values[crit]; ok {
			fields = append(fields, field{crit, crit})
		} else {
			fields = append(fields, field{crit, "spell crit %"})
		}
	}

	var checks []Check
	for _, f := range fields {
		got, ok := server[f.name]
		if !ok {
			continue // another power type's bar, or one the record predates
		}
		want, ok := sim.values[f.sim]
		if !ok && f.name == "block value" {
			continue
		}
		bound := sim.bounds[f.sim]
		detail := fmt.Sprintf("sim %.4f, server %.4f (up to %.3g below)", want, got, bound)
		if diff := want - got; diff >= -sweepEpsilon && diff <= bound+sweepEpsilon {
			checks = append(checks, pass(f.name, detail))
		} else {
			checks = append(checks, fail(f.name, detail))
		}
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

	spellHit := checkNear("spell hit %", s[stats.SpellHit]/core.SpellHitRatingPerHitChance, unit.HitMods.Spell, 1e-3)
	if schoolHit, err := spellHitEverySchool(unit.unitSnapshot); err != nil {
		spellHit = fail(spellHit.Name, err.Error())
	} else if schoolHit > 0 {
		spellHit = checkNear("spell hit %", s[stats.SpellHit]/core.SpellHitRatingPerHitChance, unit.HitMods.Spell+schoolHit, 1e-3)
		spellHit.Detail += fmt.Sprintf(" (%.0f of it school hit)", schoolHit)
	}
	checks := []Check{spellHit}

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

// checkTruncated passes when the server's whole number is the sim's value cut down by less than slack,
// the server truncating once or twice on the way.
func checkTruncated(name string, sim, server, slack float64) Check {
	detail := fmt.Sprintf("sim %.2f, server %.0f", sim, server)
	if diff := sim - server; diff >= -0.01 && diff < slack+0.01 {
		return pass(name, detail)
	}
	return fail(name, detail)
}
