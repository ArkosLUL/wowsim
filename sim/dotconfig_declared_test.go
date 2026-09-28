package sim

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// TestDotConfigDeclaresTicksCanCrit is the static check PAR-P8 substitutes for making the field
// required: Go has no way to force a struct literal to set one of its bool fields, so this walks
// every DotConfig{} literal under sim/ and fails on one that leaves TicksCanCrit unset. A literal
// with no fields at all (a Ternary's unused branch, e.g. priest/penance.go) never ticks, so it's
// exempt.
func TestDotConfigDeclaresTicksCanCrit(t *testing.T) {
	var undeclared []string
	fset := token.NewFileSet()

	err := filepath.WalkDir(".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".go") {
			return nil
		}

		file, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			return err
		}

		ast.Inspect(file, func(n ast.Node) bool {
			lit, ok := n.(*ast.CompositeLit)
			if !ok || len(lit.Elts) == 0 || !isDotConfigType(lit.Type) {
				return true
			}
			for _, elt := range lit.Elts {
				kv, ok := elt.(*ast.KeyValueExpr)
				if !ok {
					continue
				}
				if key, ok := kv.Key.(*ast.Ident); ok && key.Name == "TicksCanCrit" {
					return true
				}
			}
			pos := fset.Position(lit.Pos())
			undeclared = append(undeclared, pos.String())
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	sort.Strings(undeclared)
	for _, pos := range undeclared {
		t.Errorf("%s: DotConfig doesn't declare TicksCanCrit", pos)
	}
}

func isDotConfigType(expr ast.Expr) bool {
	switch t := expr.(type) {
	case *ast.Ident:
		return t.Name == "DotConfig"
	case *ast.SelectorExpr:
		return t.Sel.Name == "DotConfig"
	default:
		return false
	}
}
