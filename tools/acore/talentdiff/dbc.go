package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/wowsims/wotlk/tools/database/azerothcore"
)

// The three client tables that decide where a talent sits and what a glyph does.
// Fields are compared by index rather than by a struct, so a layout this tool got
// wrong can't hide a difference.
var talentFiles = []string{"Talent.dbc", "TalentTab.dbc", "GlyphProperties.dbc"}

// fieldNames are the indices worth naming in the report, from AzerothCore's
// DBCStructure.h. Everything else reads as field_<n>.
var fieldNames = map[string]map[int]string{
	"Talent.dbc": {
		0: "talent_id", 1: "talent_tab", 2: "row", 3: "col",
		4: "rank_1", 5: "rank_2", 6: "rank_3", 7: "rank_4", 8: "rank_5",
		13: "depends_on", 16: "depends_on_rank",
	},
	"TalentTab.dbc": {
		0: "talent_tab_id", 17: "spell_icon", 18: "race_mask", 19: "class_mask",
		20: "pet_talent_mask", 21: "order_index",
	},
	"GlyphProperties.dbc": {
		0: "glyph_id", 1: "spell_id", 2: "slot_flags", 3: "spell_icon",
	},
}

// table is one DBC keyed by its first field, which is the id in all three.
type table struct {
	Name string
	File *azerothcore.DBCFile
	Rows map[int32]int // id -> row index
}

func readTable(dir, name string) (*table, error) {
	data, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		return nil, err
	}
	file, err := azerothcore.ParseDBC(data)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", filepath.Join(dir, name), err)
	}

	rows := make(map[int32]int, file.RecordCount)
	for row := 0; row < file.RecordCount; row++ {
		rows[file.Int32(row, 0)] = row
	}
	return &table{Name: name, File: file, Rows: rows}, nil
}

func (t *table) fieldName(index int) string {
	if name, ok := fieldNames[t.Name][index]; ok {
		return name
	}
	return fmt.Sprintf("field_%d", index)
}

func (t *table) ids() []int32 {
	ids := make([]int32, 0, len(t.Rows))
	for id := range t.Rows {
		ids = append(ids, id)
	}
	return ids
}

// talentOf describes where a talent sits, for the context column.
func (t *table) talentOf(id int32) string {
	row, ok := t.Rows[id]
	if !ok || t.Name != "Talent.dbc" {
		return ""
	}
	return fmt.Sprintf("tab %d row %d col %d", t.File.Int32(row, 1), t.File.Int32(row, 2), t.File.Int32(row, 3))
}

// rankSpells are a talent's five rank spell ids, zeros left out.
func (t *table) rankSpells(id int32) []int32 {
	row, ok := t.Rows[id]
	if !ok || t.Name != "Talent.dbc" {
		return nil
	}
	var spells []int32
	for rank := 0; rank < 5; rank++ {
		if spell := t.File.Int32(row, 4+rank); spell != 0 {
			spells = append(spells, spell)
		}
	}
	return spells
}

func (t *table) glyphSpell(id int32) int32 {
	row, ok := t.Rows[id]
	if !ok || t.Name != "GlyphProperties.dbc" {
		return 0
	}
	return t.File.Int32(row, 1)
}
