package sim

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/wowsims/wotlk/sim/core/proto"
	googleProto "google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
)

const implementedTalentsPath = "core/testdata/simval/implemented_talents.json"

var updateTalents = flag.Bool("update", false, "rewrite "+implementedTalentsPath)

// untakenTalents allowlists the implemented talents no golden build takes: class, then the tree's
// fieldName, then why. Targets only attack raid.Tanks, and only the tank suites set it.
var untakenTalents = map[proto.Class]map[string]string{
	proto.Class_ClassHunter: {
		"improvedAspectOfTheMonkey": "tank-only: dodge in Aspect of the Dragonhawk, and nothing attacks a hunter in the goldens",
		"thickHide":                 "tank-only: armor for the hunter and the pet, and nothing attacks either in the goldens",
		"deflection":                "tank-only: parry, and nothing attacks a hunter in the goldens",
	},
	proto.Class_ClassMage: {
		"magicAbsorption": "tank-only: resistances, and nothing attacks a mage in the goldens",
		"slow":            "a cast no golden rotation makes: every golden target is snare immune, so it gives Torment the Weak nothing",
	},
	proto.Class_ClassRogue: {
		"deflection": "tank-only: parry, and nothing attacks a rogue in the goldens",
	},
	proto.Class_ClassShaman: {
		"anticipation":   "tank-only: dodge, and nothing attacks a shaman in the goldens",
		"toughness":      "tank-only: stamina, and nothing attacks a shaman in the goldens",
		"guardianTotems": "tank-only: Stoneskin Totem's armor, and nothing attacks a shaman in the goldens",
	},
	proto.Class_ClassWarlock: {
		"moltenSkin":              "tank-only: damage taken, and nothing attacks a warlock in the goldens",
		"improvedCurseOfWeakness": "raid debuff: FullDebuffs already applies improved Curse of Weakness",
	},
	proto.Class_ClassWarrior: {
		"rampage":            "raid buff: FullBuffs' Leader of the Pack already gives its crit",
		"commandingPresence": "raid buff: FullBuffs already gives improved Battle Shout, and Commanding Shout only adds health",
	},
}

var classTalents = map[proto.Class]googleProto.Message{
	proto.Class_ClassDeathknight: &proto.DeathknightTalents{},
	proto.Class_ClassDruid:       &proto.DruidTalents{},
	proto.Class_ClassHunter:      &proto.HunterTalents{},
	proto.Class_ClassMage:        &proto.MageTalents{},
	proto.Class_ClassPaladin:     &proto.PaladinTalents{},
	proto.Class_ClassPriest:      &proto.PriestTalents{},
	proto.Class_ClassRogue:       &proto.RogueTalents{},
	proto.Class_ClassShaman:      &proto.ShamanTalents{},
	proto.Class_ClassWarlock:     &proto.WarlockTalents{},
	proto.Class_ClassWarrior:     &proto.WarriorTalents{},
}

type talentTree struct {
	Name    string       `json:"name"`
	Talents []treeTalent `json:"talents"`
}

type treeTalent struct {
	FieldName string `json:"fieldName"`
	Location  struct {
		RowIdx int `json:"rowIdx"`
	} `json:"location"`
	SpellIDs  []int32 `json:"spellIds"`
	MaxPoints int     `json:"maxPoints"`

	// the JSON name of the proto field at the talent's position, which is what the sim fills
	protoName string
}

// implementedTalent is one entry of implemented_talents.json.
type implementedTalent struct {
	Class      string  `json:"class"`
	Tree       string  `json:"tree"`
	FieldName  string  `json:"fieldName"`
	Row        int     `json:"row"`
	MaxPoints  int     `json:"maxPoints"`
	RankSpells []int32 `json:"rankSpells"` // one per rank
}

// rankSpells is the talent's spell for each rank. Tree files can stop listing them early, the rest
// following on from the last one listed.
func (talent treeTalent) rankSpells() []int32 {
	spells := slices.Clone(talent.SpellIDs)
	for len(spells) < talent.MaxPoints {
		spells = append(spells, spells[len(spells)-1]+1)
	}
	return spells
}

// suiteBuild is a talent string a golden suite runs.
type suiteBuild struct {
	class   proto.Class
	talents string
	where   string
}

// classDir is the class's directory under sim/ and its talent tree's file name.
func classDir(class proto.Class) string {
	return strings.ToLower(strings.TrimPrefix(class.String(), "Class"))
}

// Every talent sim/<class> reads has to sit in a golden suite's build, or in untakenTalents.
func TestTalentCoverage(t *testing.T) {
	builds := suiteBuilds(t)
	var exported []implementedTalent

	type classResult struct {
		class       proto.Class
		reads       map[string]string
		taken       map[string]bool
		implemented []implementedTalent
	}
	var results []classResult
	for _, class := range slices.Sorted(maps.Keys(classTalents)) {
		trees := loadTalentTrees(t, class)
		r := classResult{class: class, reads: talentReads(t, class), taken: map[string]bool{}}
		for _, b := range builds {
			if b.class == class {
				for _, name := range takenTalents(t, trees, b) {
					r.taken[name] = true
				}
			}
		}
		for _, tree := range trees {
			for _, talent := range tree.Talents {
				_, read := r.reads[talent.protoName]
				switch {
				case talent.FieldName != talent.protoName && read:
					t.Errorf("%s.json names %s %q, the proto field at its position is %q", classDir(class), tree.Name, talent.FieldName, talent.protoName)
				case talent.FieldName != talent.protoName:
					t.Logf("%s.json names %s %q, the proto field at its position is %q; the sim doesn't read it", classDir(class), tree.Name, talent.FieldName, talent.protoName)
				case read:
					r.implemented = append(r.implemented, implementedTalent{Class: classDir(class), Tree: tree.Name,
						FieldName: talent.FieldName, Row: talent.Location.RowIdx, MaxPoints: talent.MaxPoints, RankSpells: talent.rankSpells()})
				}
			}
		}
		exported = append(exported, r.implemented...)
		results = append(results, r)
	}

	t.Run("ImplementedTalentsJSON", func(t *testing.T) {
		want := marshalImplementedTalents(exported)
		if *updateTalents {
			if err := os.WriteFile(implementedTalentsPath, want, 0o644); err != nil {
				t.Fatal(err)
			}
		}
		got, err := os.ReadFile(implementedTalentsPath)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(bytes.ReplaceAll(got, []byte("\r\n"), []byte("\n")), want) {
			t.Errorf("sim/%s is stale, rewrite it with go test ./sim -run TestTalentCoverage -update", implementedTalentsPath)
		}
	})

	t.Run("ServerDataPresets", func(t *testing.T) {
		for _, b := range builds {
			if !slices.ContainsFunc(serverDataPresets, func(p serverDataPreset) bool { return p.class == b.class && p.talents == b.talents }) {
				t.Errorf("%s: no serverDataPresets entry runs %s talents %q (sim/serverdata_test.go)", b.where, classDir(b.class), b.talents)
			}
		}
	})

	for _, r := range results {
		t.Run(strings.TrimPrefix(r.class.String(), "Class"), func(t *testing.T) {
			t.Logf("%d implemented talents", len(r.implemented))
			allowed := untakenTalents[r.class]
			for _, talent := range r.implemented {
				reason, listed := allowed[talent.FieldName]
				switch {
				case r.taken[talent.FieldName] && listed:
					t.Errorf("%s is allowlisted, but a golden build takes it now: remove its entry", talent.FieldName)
				case r.taken[talent.FieldName]:
				case !listed:
					t.Errorf("%s (%s, read at %s) is in no golden build: take it in a Talents-<label>-Average build, or allowlist it with a reason",
						talent.FieldName, talent.Tree, r.reads[talent.FieldName])
				case strings.TrimSpace(reason) == "":
					t.Errorf("%s: its allowlist entry needs a reason", talent.FieldName)
				}
			}
			for _, name := range slices.Sorted(maps.Keys(allowed)) {
				if _, ok := r.reads[name]; !ok {
					t.Errorf("%s is allowlisted, but sim/%s doesn't read it: remove its entry", name, classDir(r.class))
				}
			}
		})
	}
}

// loadTalentTrees reads ui/core/talents/trees/<class>.json. Its order is the talent string's, which
// core.FillTalentsProto reads as proto field numbers.
func loadTalentTrees(t *testing.T, class proto.Class) []talentTree {
	t.Helper()
	path := filepath.Join("..", "ui", "core", "talents", "trees", classDir(class)+".json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var trees []talentTree
	if err := json.Unmarshal(data, &trees); err != nil {
		t.Fatalf("%s: %v", path, err)
	}
	fields := classTalents[class].ProtoReflect().Descriptor().Fields()
	sizes := talentTreeSizes[class]
	if len(trees) != len(sizes) {
		t.Fatalf("%s has %d trees, want %d", path, len(trees), len(sizes))
	}
	number := 0
	for i, tree := range trees {
		if len(tree.Talents) != sizes[i] {
			t.Fatalf("%s: %s has %d talents, but %s.TalentTreeSizes says %d", path, tree.Name, len(tree.Talents), classDir(class), sizes[i])
		}
		for j := range tree.Talents {
			number++
			fd := fields.ByNumber(protoreflect.FieldNumber(number))
			if fd == nil {
				t.Fatalf("%s: no proto field for %s at talent string position %d", path, tree.Talents[j].FieldName, number)
			}
			tree.Talents[j].protoName = fd.JSONName()
		}
	}
	return trees
}

// talentReads maps each talent the non-test code under sim/<class> reads, by its proto JSON name, to
// the first place it's read. A read is x.Talents.Field, or Field on a variable bound from x.Talents in
// the same function. The hunter pet's are x.Talents(), so they don't count.
func talentReads(t *testing.T, class proto.Class) map[string]string {
	t.Helper()
	jsonNames := talentJSONNames(classTalents[class])
	fieldOf := func(name string) (string, bool) {
		if json, ok := jsonNames[name]; ok {
			return json, true
		}
		json, ok := jsonNames[strings.TrimPrefix(name, "Get")]
		return json, ok
	}

	reads := map[string]string{}
	fset := token.NewFileSet()
	err := filepath.WalkDir(classDir(class), func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return err
		}
		file, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			return err
		}
		for _, decl := range file.Decls {
			bound := map[string]bool{}
			ast.Inspect(decl, func(n ast.Node) bool {
				switch n := n.(type) {
				case *ast.AssignStmt:
					if len(n.Lhs) == len(n.Rhs) {
						for i, rhs := range n.Rhs {
							if id, ok := n.Lhs[i].(*ast.Ident); ok && isTalentsField(rhs) {
								bound[id.Name] = true
							}
						}
					}
				case *ast.ValueSpec:
					for i, value := range n.Values {
						if i < len(n.Names) && isTalentsField(value) {
							bound[n.Names[i].Name] = true
						}
					}
				}
				return true
			})
			ast.Inspect(decl, func(n ast.Node) bool {
				sel, ok := n.(*ast.SelectorExpr)
				if !ok {
					return true
				}
				if id, ok := sel.X.(*ast.Ident); !isTalentsField(sel.X) && !(ok && bound[id.Name]) {
					return true
				}
				if json, ok := fieldOf(sel.Sel.Name); ok {
					if _, seen := reads[json]; !seen {
						pos := fset.Position(sel.Pos())
						reads[json] = fmt.Sprintf("sim/%s:%d", filepath.ToSlash(pos.Filename), pos.Line)
					}
				}
				return true
			})
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return reads
}

func isTalentsField(e ast.Expr) bool {
	sel, ok := e.(*ast.SelectorExpr)
	return ok && sel.Sel.Name == "Talents"
}

// talentJSONNames maps the message's Go field names to their JSON names, which the trees use.
func talentJSONNames(msg googleProto.Message) map[string]string {
	fields := msg.ProtoReflect().Descriptor().Fields()
	names := map[string]string{}
	typ := reflect.TypeOf(msg).Elem()
	for i := range typ.NumField() {
		field := typ.Field(i)
		for _, part := range strings.Split(field.Tag.Get("protobuf"), ",") {
			if name, ok := strings.CutPrefix(part, "name="); ok {
				names[field.Name] = fields.ByName(protoreflect.Name(name)).JSONName()
			}
		}
	}
	return names
}

// takenTalents lists the fieldNames the build puts a point in.
func takenTalents(t *testing.T, trees []talentTree, b suiteBuild) []string {
	t.Helper()
	var taken []string
	parts := strings.Split(b.talents, "-")
	if len(parts) > len(trees) {
		t.Fatalf("%s: talents %q have %d trees", b.where, b.talents, len(parts))
	}
	for i, part := range parts {
		for j, c := range part {
			if j >= len(trees[i].Talents) || c < '0' || int(c-'0') > trees[i].Talents[j].MaxPoints {
				t.Fatalf("%s: talents %q don't fit the %s tree at position %d", b.where, b.talents, trees[i].Name, j)
			}
			if c != '0' {
				taken = append(taken, trees[i].Talents[j].protoName)
			}
		}
	}
	return taken
}

// One talent per line, so a change shows as a line diff.
func marshalImplementedTalents(talents []implementedTalent) []byte {
	var b bytes.Buffer
	b.WriteString("[\n")
	for i, talent := range talents {
		line, err := json.Marshal(talent)
		if err != nil {
			panic(err)
		}
		b.WriteString("  ")
		b.Write(line)
		if i < len(talents)-1 {
			b.WriteString(",")
		}
		b.WriteString("\n")
	}
	b.WriteString("]\n")
	return b.Bytes()
}

// suiteBuilds reads the talent strings each golden suite runs out of its test package. It starts at
// every Test function that calls RunTestSuite and follows the package-level functions, vars and
// consts it names. A build is the Talents of a CharacterSuiteConfig or TalentsCombo literal, or a
// TalentsString, and has to resolve to a string constant.
func suiteBuilds(t *testing.T) []suiteBuild {
	t.Helper()
	dirs := map[string]bool{}
	err := filepath.WalkDir(".", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, "_test.go") {
			return err
		}
		data, err := os.ReadFile(path)
		if err == nil && bytes.Contains(data, []byte("RunTestSuite(")) {
			dirs[filepath.Dir(path)] = true
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}

	var builds []suiteBuild
	suites := 0
	for _, dir := range slices.Sorted(maps.Keys(dirs)) {
		pkg := parseSuitePackage(t, dir)
		for _, root := range pkg.roots {
			suites++
			builds = append(builds, pkg.buildsFrom(t, root)...)
		}
	}
	if suites == 0 {
		t.Fatal("found no golden suites")
	}
	t.Logf("%d golden suites, %d talent strings", suites, len(builds))
	return builds
}

type suitePackage struct {
	fset   *token.FileSet
	class  proto.Class // from the directory, for a build that doesn't name its class
	funcs  map[string]*ast.FuncDecl
	values map[string]ast.Expr
	roots  []*ast.FuncDecl
}

func parseSuitePackage(t *testing.T, dir string) *suitePackage {
	t.Helper()
	pkg := &suitePackage{fset: token.NewFileSet(), funcs: map[string]*ast.FuncDecl{}, values: map[string]ast.Expr{}}
	top := strings.Split(filepath.ToSlash(dir), "/")[0]
	for class := range classTalents {
		if classDir(class) == top {
			pkg.class = class
		}
	}
	paths, err := filepath.Glob(filepath.Join(dir, "*.go"))
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range paths {
		file, err := parser.ParseFile(pkg.fset, path, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		if strings.HasSuffix(file.Name.Name, "_test") {
			continue
		}
		for _, decl := range file.Decls {
			switch decl := decl.(type) {
			case *ast.FuncDecl:
				if decl.Recv != nil {
					continue
				}
				pkg.funcs[decl.Name.Name] = decl
				if strings.HasSuffix(path, "_test.go") && strings.HasPrefix(decl.Name.Name, "Test") && callsRunTestSuite(decl) {
					pkg.roots = append(pkg.roots, decl)
				}
			case *ast.GenDecl:
				for _, spec := range decl.Specs {
					if vs, ok := spec.(*ast.ValueSpec); ok {
						for i, name := range vs.Names {
							if i < len(vs.Values) {
								pkg.values[name.Name] = vs.Values[i]
							}
						}
					}
				}
			}
		}
	}
	return pkg
}

func callsRunTestSuite(fn *ast.FuncDecl) bool {
	found := false
	ast.Inspect(fn, func(n ast.Node) bool {
		if call, ok := n.(*ast.CallExpr); ok {
			switch f := call.Fun.(type) {
			case *ast.SelectorExpr:
				found = found || f.Sel.Name == "RunTestSuite"
			case *ast.Ident:
				found = found || f.Name == "RunTestSuite"
			}
		}
		return !found
	})
	return found
}

func (pkg *suitePackage) buildsFrom(t *testing.T, root *ast.FuncDecl) []suiteBuild {
	t.Helper()
	var builds []suiteBuild
	add := func(class proto.Class, value ast.Expr) {
		pos := pkg.fset.Position(value.Pos())
		where := fmt.Sprintf("%s (sim/%s:%d)", root.Name.Name, filepath.ToSlash(pos.Filename), pos.Line)
		talents, ok := pkg.constString(value)
		if !ok {
			t.Errorf("%s: can't read these talents as a string constant", where)
			return
		}
		if class == 0 {
			class = pkg.class
		}
		if class == 0 {
			t.Errorf("%s: can't tell the class of talents %q", where, talents)
			return
		}
		builds = append(builds, suiteBuild{class, talents, where})
	}

	seen := map[string]bool{root.Name.Name: true}
	queue := []ast.Node{root}
	var visit func(n ast.Node)
	visit = func(n ast.Node) {
		ast.Inspect(n, func(n ast.Node) bool {
			switch n := n.(type) {
			case *ast.SelectorExpr:
				visit(n.X)
				return false
			case *ast.KeyValueExpr:
				if _, ok := n.Key.(*ast.Ident); ok {
					visit(n.Value)
					return false
				}
			case *ast.CompositeLit:
				typeName := ""
				switch typ := n.Type.(type) {
				case *ast.Ident:
					typeName = typ.Name
				case *ast.SelectorExpr:
					typeName = typ.Sel.Name
				}
				var class proto.Class
				for _, elt := range n.Elts {
					if kv, ok := elt.(*ast.KeyValueExpr); ok && isIdent(kv.Key, "Class") {
						if sel, ok := kv.Value.(*ast.SelectorExpr); ok {
							class = proto.Class(proto.Class_value[strings.TrimPrefix(sel.Sel.Name, "Class_")])
						}
					}
				}
				for _, elt := range n.Elts {
					kv, ok := elt.(*ast.KeyValueExpr)
					if ok && (isIdent(kv.Key, "TalentsString") ||
						isIdent(kv.Key, "Talents") && (typeName == "CharacterSuiteConfig" || typeName == "TalentsCombo")) {
						add(class, kv.Value)
					}
				}
			case *ast.AssignStmt:
				for i, lhs := range n.Lhs {
					if sel, ok := lhs.(*ast.SelectorExpr); ok && sel.Sel.Name == "TalentsString" && i < len(n.Rhs) {
						add(0, n.Rhs[i])
					}
				}
			case *ast.Ident:
				if seen[n.Name] {
					return true
				}
				if fn, ok := pkg.funcs[n.Name]; ok {
					seen[n.Name] = true
					queue = append(queue, fn)
				} else if value, ok := pkg.values[n.Name]; ok {
					seen[n.Name] = true
					queue = append(queue, value)
				}
			}
			return true
		})
	}
	for len(queue) > 0 {
		n := queue[0]
		queue = queue[1:]
		visit(n)
	}
	return builds
}

func (pkg *suitePackage) constString(e ast.Expr) (string, bool) {
	switch e := e.(type) {
	case *ast.BasicLit:
		if e.Kind == token.STRING {
			s, err := strconv.Unquote(e.Value)
			return s, err == nil
		}
	case *ast.Ident:
		if value, ok := pkg.values[e.Name]; ok {
			return pkg.constString(value)
		}
	}
	return "", false
}

func isIdent(e ast.Expr, name string) bool {
	id, ok := e.(*ast.Ident)
	return ok && id.Name == name
}
