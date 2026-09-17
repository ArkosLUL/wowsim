package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
)

type goFile struct {
	path string // slash-separated
	dir  string
	fset *token.FileSet
	ast  *ast.File
}

// goLocation is a literal or identifier in the sim's Go code.
type goLocation struct {
	file *goFile
	node ast.Node
}

func (loc goLocation) line() int {
	return loc.file.fset.Position(loc.node.Pos()).Line
}

// path returns the nodes from the file down to loc.node.
func (loc goLocation) path() []ast.Node {
	var stack, path []ast.Node
	target := loc.node
	ast.Inspect(loc.file.ast, func(n ast.Node) bool {
		if path != nil {
			return false
		}
		if n == nil {
			stack = stack[:len(stack)-1]
			return true
		}
		if n.Pos() > target.Pos() || n.End() < target.End() {
			return false
		}
		stack = append(stack, n)
		if n == target {
			path = slices.Clone(stack)
		}
		return true
	})
	return path
}

// goSourceIndex finds where the sim hardcodes item, spell, and set data. Locations keep file walk
// and source order, so output built from them is stable between runs.
type goSourceIndex struct {
	fset    *token.FileSet
	files   []*goFile
	ints    map[int64][]goLocation
	strings map[string][]goLocation
	idents  map[string][]goLocation
}

func newGoSourceIndex() *goSourceIndex {
	return &goSourceIndex{
		fset:    token.NewFileSet(),
		ints:    map[int64][]goLocation{},
		strings: map[string][]goLocation{},
		idents:  map[string][]goLocation{},
	}
}

func indexGoSources(root string) (*goSourceIndex, error) {
	idx := newGoSourceIndex()
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && d.Name() == "proto" {
			return filepath.SkipDir
		}
		if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		return idx.addFile(path, nil)
	})
	if err != nil {
		return nil, fmt.Errorf("indexing sim Go sources (-simSrc): %w", err)
	}
	if len(idx.files) == 0 {
		return nil, fmt.Errorf("no Go files under %s", root)
	}
	return idx, nil
}

// addFile parses src, or the file at path when src is nil.
func (idx *goSourceIndex) addFile(path string, src any) error {
	f, err := parser.ParseFile(idx.fset, path, src, parser.SkipObjectResolution)
	if err != nil {
		return err
	}
	path = filepath.ToSlash(path)
	file := &goFile{path: path, dir: filepath.ToSlash(filepath.Dir(path)), fset: idx.fset, ast: f}
	idx.files = append(idx.files, file)

	ast.Inspect(f, func(n ast.Node) bool {
		loc := goLocation{file, n}
		switch n := n.(type) {
		case *ast.BasicLit:
			switch n.Kind {
			case token.INT:
				if v, err := strconv.ParseInt(n.Value, 0, 64); err == nil {
					idx.ints[v] = append(idx.ints[v], loc)
				}
			case token.STRING:
				if s, err := strconv.Unquote(n.Value); err == nil {
					idx.strings[s] = append(idx.strings[s], loc)
				}
			}
		case *ast.Ident:
			idx.idents[n.Name] = append(idx.idents[n.Name], loc)
		}
		return true
	})
	return nil
}

func (idx *goSourceIndex) findLiteral(n int32) []goLocation {
	return idx.ints[int64(n)]
}

// findID returns where an item, gem or enchant ID is written. An ID in a package-level list such as
// `var AlchStoneItemIDs = []int32{...}` also brings in the places that use the list.
func (idx *goSourceIndex) findID(id int32) []goLocation {
	var locs []goLocation
	for _, loc := range idx.findLiteral(id) {
		locs = append(locs, loc)
		path := loc.path()
		if spec, ok := enclosingUnit(path).(*ast.ValueSpec); ok && isPackageLevel(path, spec) {
			locs = append(locs, idx.varUsages(loc.file, spec)...)
		}
	}
	return locs
}

// findSetUsages returns a set's definition and every place that uses its variable, since set bonus
// values usually live next to HasSetBonus checks in class spell code.
func (idx *goSourceIndex) findSetUsages(setName string) []goLocation {
	var locs []goLocation
	for _, def := range idx.strings[setName] {
		locs = append(locs, def)
		path := def.path()
		for _, n := range path {
			if spec, ok := n.(*ast.ValueSpec); ok && isPackageLevel(path, spec) {
				locs = append(locs, idx.varUsages(def.file, spec)...)
			}
		}
	}
	return locs
}

func isPackageLevel(path []ast.Node, spec *ast.ValueSpec) bool {
	return len(path) > 2 && path[2] == spec
}

// varUsages finds uses of package-level variables by name: bare in their own package directory,
// qualified (pkg.Name) elsewhere.
func (idx *goSourceIndex) varUsages(declFile *goFile, spec *ast.ValueSpec) []goLocation {
	var locs []goLocation
	for _, name := range spec.Names {
		if name.Name == "_" {
			continue
		}
		for _, use := range idx.idents[name.Name] {
			if use.node == name {
				continue
			}
			path := use.path()
			sel, _ := path[len(path)-2].(*ast.SelectorExpr)
			qualified := sel != nil && sel.Sel == use.node
			if use.file.dir == declFile.dir {
				if !qualified {
					locs = append(locs, use)
				}
			} else if qualified {
				if _, ok := sel.X.(*ast.Ident); ok {
					locs = append(locs, use)
				}
			}
		}
	}
	return locs
}

// enclosingUnit picks the code that defines the value found at the end of path:
//   - a map entry keyed by it
//   - else the innermost call or composite literal holding other numbers, which skips wrappers
//     like core.ActionID{SpellID: id} and plain ID lists
//   - else its statement or declaration, widened to the enclosing function when the statement
//     only stores the literal in a variable (itemID := int32(50352)), since the function uses it
//
// An identifier (a set variable) skips the call it's passed to, e.g. HasSetBonus(set, 2), since
// the bonus values sit around that call.
func enclosingUnit(path []ast.Node) ast.Node {
	hit := path[len(path)-1]
	_, skipCall := hit.(*ast.Ident)
	for i := len(path) - 2; i >= 0; i-- {
		switch n := path[i].(type) {
		case *ast.KeyValueExpr:
			if within(n.Key, hit) {
				return n
			}
		case *ast.CallExpr:
			if skipCall {
				skipCall = false
				continue
			}
			if hasOtherNumbers(n, hit) {
				return n
			}
		case *ast.CompositeLit:
			if !isLiteralList(n) && hasOtherNumbers(n, hit) {
				return n
			}
		case ast.Stmt, ast.Spec, ast.Decl:
			if storesLiteral(n, hit) {
				for j := i - 1; j >= 0; j-- {
					switch fn := path[j].(type) {
					case *ast.FuncLit, *ast.FuncDecl:
						return fn
					}
				}
			}
			return n
		}
	}
	return hit
}

func storesLiteral(stmt, hit ast.Node) bool {
	var values []ast.Expr
	switch s := stmt.(type) {
	case *ast.AssignStmt:
		values = s.Rhs
	case *ast.ValueSpec:
		values = s.Values
	}
	for _, value := range values {
		// Allow a conversion such as int32(50352).
		if call, ok := value.(*ast.CallExpr); ok && len(call.Args) == 1 {
			value = call.Args[0]
		}
		if ast.Node(value) == hit {
			return true
		}
	}
	return false
}

func within(outer, inner ast.Node) bool {
	return inner.Pos() >= outer.Pos() && inner.End() <= outer.End()
}

func hasOtherNumbers(n, hit ast.Node) bool {
	found := false
	ast.Inspect(n, func(child ast.Node) bool {
		if lit, ok := child.(*ast.BasicLit); ok && child != hit && isNumber(lit) {
			found = true
		}
		return !found
	})
	return found
}

func isLiteralList(lit *ast.CompositeLit) bool {
	for _, elt := range lit.Elts {
		if unary, ok := elt.(*ast.UnaryExpr); ok {
			elt = unary.X
		}
		if _, ok := elt.(*ast.BasicLit); !ok {
			return false
		}
	}
	return len(lit.Elts) > 0
}

func isNumber(lit *ast.BasicLit) bool {
	return lit.Kind == token.INT || lit.Kind == token.FLOAT
}

var timeUnitSeconds = map[string]float64{"Millisecond": 0.001, "Second": 1, "Minute": 60, "Hour": 3600}

// numbers returns the numeric literals in the code that defines the value at loc, plus durations
// in seconds (time.Minute*2 gives 2 and 120). Identifiers and comments aren't literals, so digits
// in names like float64 or T84PcProcChance never count.
func (loc goLocation) numbers() []float64 {
	var numbers []float64
	scaled := map[ast.Node]bool{}
	ast.Inspect(enclosingUnit(loc.path()), func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.BasicLit:
			if v, ok := literalValue(n); ok {
				numbers = append(numbers, v)
			}
		case *ast.BinaryExpr:
			if n.Op != token.MUL {
				break
			}
			for _, pair := range [][2]ast.Expr{{n.X, n.Y}, {n.Y, n.X}} {
				lit, isLit := pair[0].(*ast.BasicLit)
				unit, isUnit := timeUnit(pair[1])
				if v, ok := literalValue(lit); isLit && isUnit && ok {
					numbers = append(numbers, v*unit)
					scaled[pair[1]] = true
				}
			}
		case *ast.SelectorExpr:
			if unit, ok := timeUnit(n); ok && !scaled[n] {
				numbers = append(numbers, unit)
			}
		}
		return true
	})
	return numbers
}

func literalValue(lit *ast.BasicLit) (float64, bool) {
	switch {
	case lit == nil:
		return 0, false
	case lit.Kind == token.INT:
		v, err := strconv.ParseInt(lit.Value, 0, 64)
		return float64(v), err == nil
	case lit.Kind == token.FLOAT:
		v, err := strconv.ParseFloat(lit.Value, 64)
		return v, err == nil
	}
	return 0, false
}

// timeUnit returns the length in seconds of time.Second, time.Minute and the like.
func timeUnit(expr ast.Expr) (float64, bool) {
	sel, ok := expr.(*ast.SelectorExpr)
	if !ok {
		return 0, false
	}
	pkg, ok := sel.X.(*ast.Ident)
	if !ok || pkg.Name != "time" {
		return 0, false
	}
	seconds, ok := timeUnitSeconds[sel.Sel.Name]
	return seconds, ok
}

func formatLocations(locs []goLocation) string {
	var parts []string
	for _, loc := range locs {
		if part := loc.file.path + ":" + strconv.Itoa(loc.line()); !slices.Contains(parts, part) {
			parts = append(parts, part)
		}
	}
	return strings.Join(parts, " ")
}
