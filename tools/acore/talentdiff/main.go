// Command talentdiff compares the talent and glyph client tables of two DBC sets,
// so the sim knows where the user's client disagrees with stock 3.3.5a.
//
// The sim keeps stock tree positions and the raid importer maps talents by them,
// so any talent the user's client moved, and any talent or glyph spell it changed,
// has to be a deliberate decision rather than a surprise.
//
//	tools/acore/dock.sh run ./tools/acore/talentdiff
//	DBC_DIR=<live DBCs> tools/acore/dock.sh run ./tools/acore/talentdiff -b /dbc -blabel live
package main

import (
	"encoding/csv"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/wowsims/wotlk/tools/database/azerothcore"
)

func main() {
	dirA := flag.String("a", "/dbc/Clean", "the DBC directory to compare against")
	dirB := flag.String("b", "/dbc/Changed", "the DBC directory to compare")
	labelA := flag.String("alabel", "clean", "column name for -a")
	labelB := flag.String("blabel", "changed", "column name for -b")
	out := flag.String("out", "docs/azerothcore-parity/audit/talents_diff.csv", "where the report is written")
	spells := flag.Bool("spells", true, "also compare the Spell.dbc rows of every talent and glyph spell")
	flag.Parse()

	if err := run(*dirA, *dirB, *labelA, *labelB, *out, *spells); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(dirA, dirB, labelA, labelB, out string, withSpells bool) error {
	var diffs []difference
	sides := map[string][2]*table{}

	for _, name := range talentFiles {
		a, err := readTable(dirA, name)
		if err != nil {
			return err
		}
		b, err := readTable(dirB, name)
		if err != nil {
			return err
		}
		diffs = append(diffs, diffTables(a, b)...)
		sides[name] = [2]*table{a, b}
	}

	if withSpells {
		spellDiffs, err := diffSpellTables(dirA, dirB, sides["Talent.dbc"], sides["GlyphProperties.dbc"])
		if err != nil {
			return err
		}
		diffs = append(diffs, spellDiffs...)
	}

	if err := writeReport(out, labelA, labelB, diffs); err != nil {
		return err
	}
	fmt.Printf("%s vs %s: %s\n", labelA, labelB, summarize(diffs))
	fmt.Printf("wrote %s\n", out)
	return nil
}

// diffSpellTables reads both Spell.dbc files once and compares only the spells the
// talent and glyph tables point at. Both sides contribute ids, so a spell only one
// of them names is still compared.
func diffSpellTables(dirA, dirB string, talents, glyphs [2]*table) ([]difference, error) {
	a, err := azerothcore.LoadDBC(dirA)
	if err != nil {
		return nil, err
	}
	b, err := azerothcore.LoadDBC(dirB)
	if err != nil {
		return nil, err
	}

	var talentSpells []int32
	for _, side := range talents {
		for _, id := range side.ids() {
			talentSpells = append(talentSpells, side.rankSpells(id)...)
		}
	}
	var glyphSpells []int32
	for _, side := range glyphs {
		for _, id := range side.ids() {
			if spell := side.glyphSpell(id); spell != 0 {
				glyphSpells = append(glyphSpells, spell)
			}
		}
	}

	diffs := diffSpells("talent_spell", talentSpells, a, b)
	return append(diffs, diffSpells("glyph_spell", glyphSpells, a, b)...), nil
}

func writeReport(path, labelA, labelB string, diffs []difference) error {
	if dir := filepath.Dir(path); dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}

	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	w := csv.NewWriter(f)
	defer w.Flush()

	if err := w.Write([]string{
		"table", "id", "change", "field", labelA, labelB,
		labelA + "_text", labelB + "_text", "context",
	}); err != nil {
		return err
	}
	for _, diff := range diffs {
		id := ""
		if diff.ID != 0 {
			id = itoa(diff.ID)
		}
		if err := w.Write([]string{
			diff.Table, id, diff.Change, diff.Field,
			diff.A, diff.B, diff.AText, diff.BText, diff.Context,
		}); err != nil {
			return err
		}
	}
	w.Flush()
	return w.Error()
}
