package main

import (
	"bytes"
	"encoding/csv"
	"testing"

	"github.com/wowsims/wotlk/sim/core/serverdata"
	"github.com/wowsims/wotlk/tools/acore/spellids/spellset"
)

func TestAreaOf(t *testing.T) {
	for _, tc := range []struct{ file, want string }{
		{"sim/hunter/steady_shot.go", "hunter"},
		{"sim/core/spell.go", "core"},
		{"sim/common/wotlk/melee_items.go", "common/wotlk"},
		{"sim/common/tbc/items.go", "common/tbc"},
		{"tools/database/acdiff/main.go", ""},
		{"sim", ""},
	} {
		if got := areaOf(tc.file); got != tc.want {
			t.Errorf("areaOf(%q) = %q, want %q", tc.file, got, tc.want)
		}
	}
}

func TestProcAttributesNamesTheBits(t *testing.T) {
	mask := serverdata.ProcAttrReduceProc60 | serverdata.ProcAttrReqSpellmod | 0x4000
	want := "req_spellmod reduce_proc_60 0x4000"
	if got := procAttributes(mask); got != want {
		t.Errorf("procAttributes(%#x) = %q, want %q", mask, got, want)
	}
	if got := procAttributes(0); got != "" {
		t.Errorf("procAttributes(0) = %q, want empty", got)
	}
}

func TestRefsOfCaps(t *testing.T) {
	refs := []spellset.Ref{
		{File: "sim/a.go", Line: 1}, {File: "sim/b.go", Line: 2},
		{File: "sim/c.go", Line: 3}, {File: "sim/d.go", Line: 4}, {File: "sim/e.go", Line: 5},
	}
	want := "sim/a.go:1 sim/b.go:2 sim/c.go:3 +2 more"
	if got := refsOf(refs); got != want {
		t.Errorf("refsOf = %q, want %q", got, want)
	}
}

func TestTickMsTakesTheFirstPeriodicEffect(t *testing.T) {
	spell := &spellset.DumpSpell{Effects: []spellset.DumpEffect{
		{Index: 0},
		{Index: 1, AmplitudeMs: 3000},
		{Index: 2, AmplitudeMs: 1000},
	}}
	if got := tickMs(spell); got != 3000 {
		t.Errorf("tickMs = %d, want 3000", got)
	}
	if got := tickMs(&spellset.DumpSpell{}); got != 0 {
		t.Errorf("tickMs of a spell without periodic effects = %d", got)
	}
}

func TestBuildAuditSplitsCapturedFromMissing(t *testing.T) {
	literals := &spellset.Literals{
		Spells: map[int32][]spellset.Ref{
			100: {{File: "sim/hunter/shot.go", Line: 7}},
			200: {{File: "sim/mage/bolt.go", Line: 9}},
		},
		Unresolved: []spellset.Ref{{File: "sim/core/x.go", Line: 3}},
	}
	capture := spellset.Dump{100: &spellset.DumpSpell{ID: 100, Name: "Shot"}}

	audit := buildAudit(literals, capture, "")
	if len(audit.Rows) != 1 || audit.Rows[0].SpellID != 100 {
		t.Fatalf("rows = %+v", audit.Rows)
	}
	if len(audit.Missing) != 1 || audit.Missing[0].SpellID != 200 {
		t.Fatalf("missing = %+v", audit.Missing)
	}

	// -area keeps only the spells that class names.
	if got := buildAudit(literals, capture, "mage"); len(got.Rows) != 0 || len(got.Missing) != 1 {
		t.Errorf("area mage: %d rows, %d missing", len(got.Rows), len(got.Missing))
	}
	if got := buildAudit(literals, capture, "hunter"); len(got.Rows) != 1 || len(got.Missing) != 0 {
		t.Errorf("area hunter: %d rows, %d missing", len(got.Rows), len(got.Missing))
	}
}

func TestWriteMissingListsEveryKind(t *testing.T) {
	audit := &audit{
		Missing:    []row{{SpellID: 200, Refs: []spellset.Ref{{File: "sim/mage/bolt.go", Line: 9}}}},
		Unresolved: []spellset.Ref{{File: "sim/core/x.go", Line: 3}},
		TypeErrors: []string{"sim/core/y.go:1: undefined: z"},
	}

	var buf bytes.Buffer
	w := csv.NewWriter(&buf)
	if err := audit.writeMissing(w); err != nil {
		t.Fatal(err)
	}
	w.Flush()

	rows, err := csv.NewReader(&buf).ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 4 {
		t.Fatalf("%d rows, want a header and three findings: %v", len(rows), rows)
	}
	for i, want := range []string{"kind", "not captured", "not a constant", "type error"} {
		if rows[i][0] != want {
			t.Errorf("row %d kind = %q, want %q", i, rows[i][0], want)
		}
	}
}
