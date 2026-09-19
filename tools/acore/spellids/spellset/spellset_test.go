package spellset

import (
	"go/ast"
	"go/parser"
	"go/token"
	"maps"
	"slices"
	"testing"
)

func TestScanSim(t *testing.T) {
	lits, err := ScanSim("testdata/fixture")
	if err != nil {
		t.Fatal(err)
	}
	if len(lits.TypeErrors) > 0 {
		t.Fatalf("type errors: %v", lits.TypeErrors)
	}

	want := []int32{75, 12867, 14179, 31221, 31222, 42842, 47465, 60229, 62478, 63512, 71484, 71556, 71561, 71600, 71846}
	if got := slices.Sorted(maps.Keys(lits.Spells)); !slices.Equal(got, want) {
		t.Errorf("spells\n got %v\nwant %v", got, want)
	}
	if got := slices.Sorted(maps.Keys(lits.Items)); !slices.Equal(got, []int32{40211, 40798, 40802}) {
		t.Errorf("items: %v", got)
	}
	if len(lits.Unresolved) != 1 || lits.Unresolved[0].File != "sim/class/class.go" {
		t.Errorf("unresolved should be just `58420 + rank`: %v", lits.Unresolved)
	}
	if refs := lits.Spells[42842]; len(refs) != 2 || refs[0] != (Ref{"sim/class/class.go", 5}) {
		t.Errorf("42842 refs: %v", refs)
	}
}

func TestDumpClosure(t *testing.T) {
	dump, err := ReadDump("testdata/dump.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	if got := dump.IDs(); !slices.Equal(got, []int32{100, 200, 400}) {
		t.Fatalf("ids %v", got)
	}
	if dump[400].PowerType != 4294967294 {
		t.Errorf("uint32 fields must read whole: %d", dump[400].PowerType)
	}

	triggered, missing := dump.Closure(map[int32]bool{100: true, 400: true, 500: true})
	if got := slices.Sorted(maps.Keys(triggered)); !slices.Equal(got, []int32{200, 300}) {
		t.Errorf("triggered %v", got)
	}
	if !slices.Equal(missing, []int32{300, 500}) {
		t.Errorf("missing %v", missing)
	}

	empty, err := ReadDump("testdata/none.jsonl")
	if err != nil || len(empty) != 0 {
		t.Errorf("a missing capture reads as empty: %v %v", empty, err)
	}

	lits := &Literals{Spells: map[int32][]Ref{100: nil, 500: nil}}
	ids, triggered, missing := dump.GeneratedSet(lits)
	if got := slices.Sorted(maps.Keys(ids)); !slices.Equal(got, []int32{100, 200, 300, 500}) {
		t.Errorf("generated set %v", got)
	}
	if got := slices.Sorted(maps.Keys(triggered)); !slices.Equal(got, []int32{200, 300}) || !slices.Equal(missing, []int32{300, 500}) {
		t.Errorf("triggered %v, missing %v", got, missing)
	}
}

// A constant passed to a serverdata lookup must count as a sim literal, or regenerating drops its spell.
func TestServerdataLookupsTakeSpellIDs(t *testing.T) {
	file, err := parser.ParseFile(token.NewFileSet(), "../../../../sim/core/serverdata/types.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	lookups := map[string]bool{"SpellByID": false, "ProcBySpellID": false, "BonusBySpellID": false}
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok {
			continue
		}
		if _, isLookup := lookups[fn.Name.Name]; !isLookup {
			continue
		}
		lookups[fn.Name.Name] = true
		if params := fn.Type.Params.List; len(params) != 1 || len(params[0].Names) != 1 || kindOf(params[0].Names[0].Name) != kindSpell {
			t.Errorf("%s's parameter must be named like a spell id", fn.Name.Name)
		}
	}
	for name, found := range lookups {
		if !found {
			t.Errorf("%s not found", name)
		}
	}
}
