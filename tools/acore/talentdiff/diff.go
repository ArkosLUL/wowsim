package main

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/wowsims/wotlk/tools/database/azerothcore"
)

// difference is one row of the report: a record only one side has, or a field the
// two sides disagree on.
type difference struct {
	Table   string
	ID      int32
	Change  string // added, removed or changed
	Field   string
	A, B    string
	AText   string
	BText   string
	Context string
}

// diffTables compares two copies of the same DBC field by field.
//
// A string field holds an offset into the file's own string block, so the same
// text lands on different numbers in two files. A field whose numbers differ but
// whose strings are equal and non-empty is one of those, and is left out.
func diffTables(a, b *table) []difference {
	var diffs []difference

	ids := a.ids()
	for _, id := range b.ids() {
		if _, ok := a.Rows[id]; !ok {
			ids = append(ids, id)
		}
	}
	slices.Sort(ids)

	fields := min(a.File.FieldCount, b.File.FieldCount)
	for _, id := range ids {
		rowA, inA := a.Rows[id]
		rowB, inB := b.Rows[id]
		switch {
		case !inB:
			diffs = append(diffs, difference{Table: a.Name, ID: id, Change: "removed", Context: a.talentOf(id)})
			continue
		case !inA:
			diffs = append(diffs, difference{Table: a.Name, ID: id, Change: "added", Context: b.talentOf(id)})
			continue
		}

		for field := 0; field < fields; field++ {
			valueA, valueB := a.File.Int32(rowA, field), b.File.Int32(rowB, field)
			if valueA == valueB {
				continue
			}
			textA, textB := a.File.String(rowA, field), b.File.String(rowB, field)
			if textA != "" && textA == textB {
				continue
			}
			diffs = append(diffs, difference{
				Table: a.Name, ID: id, Change: "changed", Field: a.fieldName(field),
				A: itoa(valueA), B: itoa(valueB), AText: textA, BText: textB,
				Context: a.talentOf(id),
			})
		}
	}

	if a.File.FieldCount != b.File.FieldCount {
		diffs = append(diffs, difference{
			Table: a.Name, Change: "changed", Field: "field_count",
			A: strconv.Itoa(a.File.FieldCount), B: strconv.Itoa(b.File.FieldCount),
			Context: "the two files have a different layout; only the shared fields were compared",
		})
	}
	return diffs
}

// diffSpells compares the Spell.dbc rows behind the talents and glyphs, so a
// client that moved no talent but changed what one does still shows up.
func diffSpells(kind string, ids []int32, a, b *azerothcore.DBC) []difference {
	var diffs []difference
	slices.Sort(ids)
	ids = slices.Compact(ids)

	for _, id := range ids {
		spellA, spellB := a.Spells[id], b.Spells[id]
		switch {
		case spellA == nil && spellB == nil:
			continue
		case spellB == nil:
			diffs = append(diffs, difference{Table: kind, ID: id, Change: "removed"})
			continue
		case spellA == nil:
			diffs = append(diffs, difference{Table: kind, ID: id, Change: "added", Context: spellB.Name})
			continue
		}

		for _, field := range spellFields(spellA, spellB) {
			if field.a == field.b {
				continue
			}
			diffs = append(diffs, difference{
				Table: kind, ID: id, Change: "changed", Field: field.name,
				A: field.a, B: field.b, Context: spellA.Name,
			})
		}
		if secondsA, secondsB := a.SpellDurationSeconds(spellA), b.SpellDurationSeconds(spellB); secondsA != secondsB {
			diffs = append(diffs, difference{
				Table: kind, ID: id, Change: "changed", Field: "duration_seconds",
				A: ftoa(secondsA), B: ftoa(secondsB), Context: spellA.Name,
			})
		}
	}
	return diffs
}

type spellField struct {
	name string
	a, b string
}

func spellFields(a, b *azerothcore.SpellEntry) []spellField {
	fields := []spellField{
		{"name", a.Name, b.Name},
		{"stances", utoa(a.Stances), utoa(b.Stances)},
		{"recovery_time", itoa(a.RecoveryTime), itoa(b.RecoveryTime)},
		{"category_recovery_time", itoa(a.CategoryRecoveryTime), itoa(b.CategoryRecoveryTime)},
		{"proc_chance", itoa(a.ProcChance), itoa(b.ProcChance)},
		{"duration_index", itoa(a.DurationIndex), itoa(b.DurationIndex)},
	}
	for i := 0; i < 3; i++ {
		suffix := strconv.Itoa(i + 1)
		fields = append(fields,
			spellField{"effect_" + suffix, itoa(a.Effect[i]), itoa(b.Effect[i])},
			spellField{"effect_aura_" + suffix, itoa(a.EffectApplyAuraName[i]), itoa(b.EffectApplyAuraName[i])},
			spellField{"effect_base_points_" + suffix, itoa(a.EffectBasePoints[i]), itoa(b.EffectBasePoints[i])},
			spellField{"effect_die_sides_" + suffix, itoa(a.EffectDieSides[i]), itoa(b.EffectDieSides[i])},
			spellField{"effect_misc_value_" + suffix, itoa(a.EffectMiscValue[i]), itoa(b.EffectMiscValue[i])},
			spellField{"effect_trigger_spell_" + suffix, itoa(a.EffectTriggerSpell[i]), itoa(b.EffectTriggerSpell[i])},
		)
	}
	return fields
}

func summarize(diffs []difference) string {
	counts := map[string]int{}
	for _, diff := range diffs {
		counts[diff.Table]++
	}

	tables := make([]string, 0, len(counts))
	for name := range counts {
		tables = append(tables, name)
	}
	slices.Sort(tables)

	parts := make([]string, 0, len(tables))
	for _, name := range tables {
		parts = append(parts, fmt.Sprintf("%s %d", name, counts[name]))
	}
	if len(parts) == 0 {
		return "no differences"
	}
	return strings.Join(parts, ", ")
}

func itoa(v int32) string   { return strconv.Itoa(int(v)) }
func utoa(v uint32) string  { return strconv.FormatUint(uint64(v), 10) }
func ftoa(v float64) string { return strconv.FormatFloat(v, 'g', -1, 64) }
