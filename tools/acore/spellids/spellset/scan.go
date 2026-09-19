package spellset

import (
	"errors"
	"fmt"
	"go/ast"
	"go/build"
	"go/constant"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"
)

// Ref is a source position, with the file relative to the module root.
type Ref struct {
	File string
	Line int
}

func (r Ref) String() string { return fmt.Sprintf("%s:%d", r.File, r.Line) }

// Literals are the spell and item ids the sim spells out as constants.
type Literals struct {
	Spells map[int32][]Ref
	Items  map[int32][]Ref
	// Spell or item id positions whose value isn't a constant, e.g. `58420 + talentPoints`.
	Unresolved []Ref
	// Type errors, which only cost the constants that depend on the broken code.
	TypeErrors []string
}

var (
	spellIDName = regexp.MustCompile(`(?i)(spell|aura)_?ids?$`)
	itemIDName  = regexp.MustCompile(`(?i)item_?ids?$`)
)

// Packages that hold no hand-written spell ids. serverdata is generated from this scan.
var skipDirs = []string{"sim/core/proto", "sim/core/serverdata", "sim/web"}

type idKind int

const (
	kindNone idKind = iota
	kindSpell
	kindItem
)

func kindOf(name string) idKind {
	switch {
	case spellIDName.MatchString(name):
		return kindSpell
	case itemIDName.MatchString(name):
		return kindItem
	}
	return kindNone
}

// ScanSim type-checks every non-test package under <moduleDir>/sim and collects the integer constants
// that end up in something named like a spell id (SpellID, AuraID, spellId, auraIDs, ...) or an item id:
// struct fields, variables and constants, parameters of the called function, returns of a function so
// named, comparisons and switch cases. Constants resolve through named constants and TernaryInt32-style
// calls; anything else is listed as unresolved.
func ScanSim(moduleDir string) (*Literals, error) {
	moduleDir, err := filepath.Abs(moduleDir)
	if err != nil {
		return nil, err
	}
	modPath, err := modulePath(moduleDir)
	if err != nil {
		return nil, err
	}

	l := &loader{
		fset:    token.NewFileSet(),
		modPath: modPath,
		modDir:  moduleDir,
		pkgs:    map[string]*loadedPkg{},
	}
	l.ext = importer.ForCompiler(l.fset, "source", nil).(types.ImporterFrom)

	var dirs []string
	simDir := filepath.Join(moduleDir, "sim")
	err = filepath.WalkDir(simDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || !d.IsDir() {
			return err
		}
		rel := filepath.ToSlash(mustRel(moduleDir, path))
		if slices.Contains(skipDirs, rel) || d.Name() == "testdata" {
			return filepath.SkipDir
		}
		dirs = append(dirs, rel)
		return nil
	})
	if err != nil {
		return nil, err
	}

	lits := &Literals{Spells: map[int32][]Ref{}, Items: map[int32][]Ref{}}
	for _, rel := range dirs {
		pkg, err := l.load(modPath + "/" + rel)
		var noGo *build.NoGoError
		if errors.As(err, &noGo) {
			continue
		}
		if err != nil {
			return nil, err
		}
		s := &scanner{l: l, pkg: pkg, lits: lits}
		for _, file := range pkg.files {
			s.scanFile(file)
		}
	}
	for _, p := range l.pkgs {
		lits.TypeErrors = append(lits.TypeErrors, p.errors...)
	}
	sort.Strings(lits.TypeErrors)
	for _, refs := range []map[int32][]Ref{lits.Spells, lits.Items} {
		for id := range refs {
			refs[id] = sortRefs(refs[id])
		}
	}
	lits.Unresolved = sortRefs(lits.Unresolved)
	return lits, nil
}

func sortRefs(refs []Ref) []Ref {
	slices.SortFunc(refs, func(a, b Ref) int {
		if c := strings.Compare(a.File, b.File); c != 0 {
			return c
		}
		return a.Line - b.Line
	})
	return slices.Compact(refs)
}

func mustRel(base, path string) string {
	rel, err := filepath.Rel(base, path)
	if err != nil {
		panic(err)
	}
	return rel
}

func modulePath(dir string) (string, error) {
	data, err := os.ReadFile(filepath.Join(dir, "go.mod"))
	if err != nil {
		return "", err
	}
	for _, line := range strings.Split(string(data), "\n") {
		if rest, ok := strings.CutPrefix(strings.TrimSpace(line), "module "); ok {
			return strings.TrimSpace(rest), nil
		}
	}
	return "", fmt.Errorf("%s/go.mod has no module line", dir)
}

type loadedPkg struct {
	pkg    *types.Package
	files  []*ast.File
	info   *types.Info
	errors []string
}

// loader type-checks the module's own packages once each, keeping their syntax and type info, and
// leaves everything else to the source importer.
type loader struct {
	fset    *token.FileSet
	modPath string
	modDir  string
	ext     types.ImporterFrom
	pkgs    map[string]*loadedPkg
}

func (l *loader) Import(path string) (*types.Package, error) {
	return l.ImportFrom(path, l.modDir, 0)
}

func (l *loader) ImportFrom(path, dir string, mode types.ImportMode) (*types.Package, error) {
	if path == l.modPath || strings.HasPrefix(path, l.modPath+"/") {
		p, err := l.load(path)
		if err != nil {
			return nil, err
		}
		return p.pkg, nil
	}
	return l.ext.ImportFrom(path, dir, mode)
}

func (l *loader) load(path string) (*loadedPkg, error) {
	if p, ok := l.pkgs[path]; ok {
		if p == nil {
			return nil, fmt.Errorf("import cycle through %s", path)
		}
		return p, nil
	}
	l.pkgs[path] = nil

	dir := filepath.Join(l.modDir, filepath.FromSlash(strings.TrimPrefix(strings.TrimPrefix(path, l.modPath), "/")))
	bp, err := build.Default.ImportDir(dir, 0)
	if err != nil {
		delete(l.pkgs, path)
		return nil, err
	}

	p := &loadedPkg{
		info: &types.Info{
			Types: map[ast.Expr]types.TypeAndValue{},
			Defs:  map[*ast.Ident]types.Object{},
			Uses:  map[*ast.Ident]types.Object{},
		},
	}
	for _, name := range append(slices.Clone(bp.GoFiles), bp.CgoFiles...) {
		file, err := parser.ParseFile(l.fset, filepath.Join(dir, name), nil, parser.SkipObjectResolution)
		if err != nil {
			return nil, err
		}
		p.files = append(p.files, file)
	}

	conf := types.Config{
		Importer:    l,
		FakeImportC: true,
		Error: func(err error) {
			if len(p.errors) < 5 {
				p.errors = append(p.errors, err.Error())
			}
		},
	}
	p.pkg, _ = conf.Check(path, l.fset, p.files, p.info)
	l.pkgs[path] = p
	return p, nil
}

type scanner struct {
	l    *loader
	pkg  *loadedPkg
	lits *Literals
}

func (s *scanner) ref(pos token.Pos) Ref {
	p := s.l.fset.Position(pos)
	return Ref{File: filepath.ToSlash(mustRel(s.l.modDir, p.Filename)), Line: p.Line}
}

func (s *scanner) scanFile(file *ast.File) {
	for _, decl := range file.Decls {
		if fn, ok := decl.(*ast.FuncDecl); ok && fn.Body != nil {
			if kind := kindOf(fn.Name.Name); kind != kindNone {
				s.scanReturns(fn.Body, kind)
			}
		}
	}

	ast.Inspect(file, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.CompositeLit:
			if _, isStruct := s.underlying(n).(*types.Struct); !isStruct {
				break
			}
			for _, elt := range n.Elts {
				if kv, ok := elt.(*ast.KeyValueExpr); ok {
					if key, ok := kv.Key.(*ast.Ident); ok {
						s.collect(kv.Value, kindOf(key.Name))
					}
				}
			}
		case *ast.AssignStmt:
			if len(n.Lhs) == len(n.Rhs) {
				for i, lhs := range n.Lhs {
					s.collect(n.Rhs[i], kindOf(lastName(lhs)))
				}
			}
		case *ast.ValueSpec:
			if len(n.Names) == len(n.Values) {
				for i, name := range n.Names {
					s.collect(n.Values[i], kindOf(name.Name))
				}
			}
		case *ast.CallExpr:
			sig, ok := s.underlying(n.Fun).(*types.Signature)
			if !ok || s.pkg.info.Types[n.Fun].IsType() {
				break
			}
			params := sig.Params()
			for i, arg := range n.Args {
				if params.Len() == 0 {
					break
				}
				param := params.At(min(i, params.Len()-1))
				if i >= params.Len() && !sig.Variadic() {
					break
				}
				s.collect(arg, kindOf(param.Name()))
			}
		case *ast.BinaryExpr:
			if n.Op == token.EQL || n.Op == token.NEQ {
				if kind := kindOf(lastName(n.X)); kind != kindNone && s.isConstant(n.Y) {
					s.collect(n.Y, kind)
				}
				if kind := kindOf(lastName(n.Y)); kind != kindNone && s.isConstant(n.X) {
					s.collect(n.X, kind)
				}
			}
		case *ast.SwitchStmt:
			if n.Tag == nil {
				break
			}
			if kind := kindOf(lastName(n.Tag)); kind != kindNone {
				for _, stmt := range n.Body.List {
					for _, e := range stmt.(*ast.CaseClause).List {
						s.collect(e, kind)
					}
				}
			}
		}
		return true
	})
}

// scanReturns collects what a spell- or item-id-named function returns, skipping nested closures.
func (s *scanner) scanReturns(body *ast.BlockStmt, kind idKind) {
	ast.Inspect(body, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.FuncLit:
			return false
		case *ast.ReturnStmt:
			for _, r := range n.Results {
				s.collect(r, kind)
			}
		}
		return true
	})
}

func (s *scanner) underlying(e ast.Expr) types.Type {
	tv, ok := s.pkg.info.Types[e]
	if !ok || tv.Type == nil {
		return nil
	}
	return tv.Type.Underlying()
}

func (s *scanner) isConstant(e ast.Expr) bool {
	return s.pkg.info.Types[e].Value != nil
}

func (s *scanner) collect(e ast.Expr, kind idKind) {
	if kind == kindNone {
		return
	}
	tv := s.pkg.info.Types[e]
	if tv.Value != nil {
		if tv.Value.Kind() != constant.Int {
			return
		}
		if v, exact := constant.Int64Val(tv.Value); exact && v > 0 && v <= math.MaxInt32 {
			refs := s.lits.Spells
			if kind == kindItem {
				refs = s.lits.Items
			}
			refs[int32(v)] = append(refs[int32(v)], s.ref(e.Pos()))
		}
		return
	}
	if basic, ok := s.underlying(e).(*types.Basic); ok && basic.Info()&types.IsBoolean != 0 {
		return
	}

	switch e := e.(type) {
	case *ast.ParenExpr:
		s.collect(e.X, kind)
	case *ast.UnaryExpr:
		if e.Op == token.AND {
			s.collect(e.X, kind)
			return
		}
		s.lits.Unresolved = append(s.lits.Unresolved, s.ref(e.Pos()))
	case *ast.CompositeLit:
		// only slice, array and map values: a struct's fields are matched by their own names
		if _, isStruct := s.underlying(e).(*types.Struct); isStruct {
			return
		}
		for _, elt := range e.Elts {
			if kv, ok := elt.(*ast.KeyValueExpr); ok {
				elt = kv.Value
			}
			s.collect(elt, kind)
		}
	case *ast.CallExpr:
		if s.pkg.info.Types[e.Fun].IsType() && len(e.Args) == 1 {
			s.collect(e.Args[0], kind)
			return
		}
		// TernaryInt32(cond, a, b) and friends; the bool condition is skipped above
		if strings.HasPrefix(lastName(e.Fun), "Ternary") {
			for _, arg := range e.Args {
				s.collect(arg, kind)
			}
			return
		}
		if kindOf(lastName(e.Fun)) == kindNone {
			s.lits.Unresolved = append(s.lits.Unresolved, s.ref(e.Pos()))
		}
	case *ast.IndexExpr:
		// []int32{0, 31221, 31222}[rank] picks one of its constants
		if _, ok := e.X.(*ast.CompositeLit); ok {
			s.collect(e.X, kind)
		} else if kindOf(lastName(e.X)) == kindNone {
			s.lits.Unresolved = append(s.lits.Unresolved, s.ref(e.Pos()))
		}
	case *ast.Ident, *ast.SelectorExpr:
		// a variable: fine when it's set from a spell-id-named source, which is scanned there
		if kindOf(lastName(e)) == kindNone {
			s.lits.Unresolved = append(s.lits.Unresolved, s.ref(e.Pos()))
		}
	default:
		s.lits.Unresolved = append(s.lits.Unresolved, s.ref(e.Pos()))
	}
}

// lastName is the identifier an expression ends in: `x` for x, `SpellID` for a.ActionID.SpellID, and
// the function's name for a call.
func lastName(e ast.Expr) string {
	switch e := e.(type) {
	case *ast.Ident:
		return e.Name
	case *ast.SelectorExpr:
		return e.Sel.Name
	case *ast.IndexExpr:
		return lastName(e.X)
	case *ast.IndexListExpr:
		return lastName(e.X)
	case *ast.CallExpr:
		return lastName(e.Fun)
	case *ast.ParenExpr:
		return lastName(e.X)
	case *ast.StarExpr:
		return lastName(e.X)
	}
	return ""
}
