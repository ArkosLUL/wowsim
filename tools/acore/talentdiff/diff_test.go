package main

import (
	"encoding/binary"
	"testing"

	"github.com/wowsims/wotlk/tools/database/azerothcore"
)

// buildTable assembles a WDBC file in memory: the records, then the string block
// the string fields point into.
func buildTable(t *testing.T, name string, records [][]uint32, strings string) *table {
	t.Helper()
	if len(records) == 0 {
		t.Fatal("a DBC needs at least one record")
	}
	fields := len(records[0])

	data := make([]byte, 20)
	copy(data, "WDBC")
	binary.LittleEndian.PutUint32(data[4:], uint32(len(records)))
	binary.LittleEndian.PutUint32(data[8:], uint32(fields))
	binary.LittleEndian.PutUint32(data[12:], uint32(fields*4))
	binary.LittleEndian.PutUint32(data[16:], uint32(len(strings)))
	for _, record := range records {
		for _, value := range record {
			data = binary.LittleEndian.AppendUint32(data, value)
		}
	}
	data = append(data, strings...)

	file, err := azerothcore.ParseDBC(data)
	if err != nil {
		t.Fatal(err)
	}
	rows := map[int32]int{}
	for row := 0; row < file.RecordCount; row++ {
		rows[file.Int32(row, 0)] = row
	}
	return &table{Name: name, File: file, Rows: rows}
}

func TestDiffTablesFindsNothingInAMatchingPair(t *testing.T) {
	records := [][]uint32{{1, 363, 0, 2}, {2, 363, 1, 0}}
	a := buildTable(t, "Talent.dbc", records, "")
	b := buildTable(t, "Talent.dbc", records, "")

	if diffs := diffTables(a, b); len(diffs) != 0 {
		t.Errorf("%d differences in identical files: %+v", len(diffs), diffs)
	}
}

func TestDiffTablesNamesTheField(t *testing.T) {
	a := buildTable(t, "Talent.dbc", [][]uint32{{1341, 363, 0, 0}}, "")
	b := buildTable(t, "Talent.dbc", [][]uint32{{1341, 363, 2, 0}}, "")

	diffs := diffTables(a, b)
	if len(diffs) != 1 {
		t.Fatalf("%d differences, want 1: %+v", len(diffs), diffs)
	}
	got := diffs[0]
	if got.Field != "row" || got.A != "0" || got.B != "2" || got.Change != "changed" {
		t.Errorf("difference = %+v", got)
	}
	if got.Context != "tab 363 row 0 col 0" {
		t.Errorf("context = %q", got.Context)
	}
}

// A string field is an offset into its own file's string block, so the same text
// lands on different numbers.
func TestDiffTablesIgnoresStringOffsets(t *testing.T) {
	a := buildTable(t, "TalentTab.dbc", [][]uint32{{161, 1, 0, 0}}, "\x00Fire\x00")
	b := buildTable(t, "TalentTab.dbc", [][]uint32{{161, 5, 0, 0}}, "\x00pad\x00Fire\x00")

	if diffs := diffTables(a, b); len(diffs) != 0 {
		t.Errorf("a moved string counted as a difference: %+v", diffs)
	}
}

func TestDiffTablesReportsAddedAndRemoved(t *testing.T) {
	a := buildTable(t, "GlyphProperties.dbc", [][]uint32{{1, 100, 0, 0}, {2, 200, 0, 0}}, "")
	b := buildTable(t, "GlyphProperties.dbc", [][]uint32{{2, 200, 0, 0}, {3, 300, 0, 0}}, "")

	diffs := diffTables(a, b)
	if len(diffs) != 2 {
		t.Fatalf("%d differences, want 2: %+v", len(diffs), diffs)
	}
	if diffs[0].ID != 1 || diffs[0].Change != "removed" {
		t.Errorf("first = %+v, want glyph 1 removed", diffs[0])
	}
	if diffs[1].ID != 3 || diffs[1].Change != "added" {
		t.Errorf("second = %+v, want glyph 3 added", diffs[1])
	}
}

func TestDiffTablesReportsALayoutChange(t *testing.T) {
	a := buildTable(t, "Talent.dbc", [][]uint32{{1, 363, 0, 0}}, "")
	b := buildTable(t, "Talent.dbc", [][]uint32{{1, 363, 0}}, "")

	diffs := diffTables(a, b)
	if len(diffs) != 1 || diffs[len(diffs)-1].Field != "field_count" {
		t.Fatalf("differences = %+v, want only the field count", diffs)
	}
}

func TestRankSpellsSkipsEmptyRanks(t *testing.T) {
	talents := buildTable(t, "Talent.dbc", [][]uint32{{1341, 363, 0, 0, 111, 222, 0, 0, 0}}, "")

	got := talents.rankSpells(1341)
	if len(got) != 2 || got[0] != 111 || got[1] != 222 {
		t.Errorf("rankSpells = %v, want [111 222]", got)
	}
	if got := talents.rankSpells(9999); got != nil {
		t.Errorf("rankSpells of an unknown talent = %v", got)
	}
}

func TestSummarizeCountsPerTable(t *testing.T) {
	diffs := []difference{
		{Table: "Talent.dbc"}, {Table: "Talent.dbc"}, {Table: "talent_spell"},
	}
	if got := summarize(diffs); got != "Talent.dbc 2, talent_spell 1" {
		t.Errorf("summarize = %q", got)
	}
	if got := summarize(nil); got != "no differences" {
		t.Errorf("summarize(nil) = %q", got)
	}
}
