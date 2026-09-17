package main

import (
	"slices"
	"testing"

	"github.com/wowsims/wotlk/tools/database/azerothcore"
)

func TestGoHasValueDependsOnKind(t *testing.T) {
	for _, tc := range []struct {
		name    string
		numbers []float64
		value   spellValue
		want    bool
	}{
		{"stat amount doesn't match a duration in minutes", []float64{1250, 20}, spellValue{kind: effectAmount, value: 1200}, false},
		{"100% doesn't match any 1", []float64{1}, spellValue{kind: effectAmount, value: 100}, false},
		{"percent as multiplier", []float64{1.1}, spellValue{kind: effectAmount, value: 10}, true},
		{"cooldown in minutes", []float64{2}, spellValue{kind: cooldownSeconds, value: 120}, true},
		{"duration in milliseconds", []float64{1500}, spellValue{kind: durationSeconds, value: 1.5}, true},
		{"proc chance as fraction", []float64{0.15}, spellValue{kind: procChancePercent, value: 15}, true},
		{"ppm only as itself", []float64{3.5}, spellValue{kind: procsPerMinute, value: 3.5}, true},
		{"a value of 1 still matches itself", []float64{1}, spellValue{kind: procsPerMinute, value: 1}, true},
	} {
		if got := goHasValue(tc.numbers, tc.value); got != tc.want {
			t.Errorf("%s: goHasValue(%v, %+v) = %v, want %v", tc.name, tc.numbers, tc.value, got, tc.want)
		}
	}
}

func labels(values []spellValue, inTooltip bool) []string {
	var out []string
	for _, v := range values {
		if v.inTooltip == inTooltip {
			out = append(out, v.label)
		}
	}
	return out
}

func TestSpellChainValuesProcRatesAndCooldowns(t *testing.T) {
	c := &context{
		dbc: &azerothcore.DBC{
			Spells: map[int32]*azerothcore.SpellEntry{
				// On-use root with its own 1 min cooldown, triggering a 10 sec buff whose own cooldown
				// never starts, since it's triggered without an item.
				100: {ID: 100, RecoveryTime: 60000, Effect: [3]int32{azerothcore.SpellEffectTriggerSpell}, EffectTriggerSpell: [3]int32{101}},
				101: {ID: 101, RecoveryTime: 30000, DurationIndex: 1, Effect: [3]int32{azerothcore.SpellEffectApplyAura}, EffectBasePoints: [3]int32{339}, EffectDieSides: [3]int32{1}},
				// Proc aura with a spell_proc PPM and cooldown.
				200: {ID: 200, ProcChance: 5, Description: "Chance on hit: $h%"},
			},
			SpellDurations: map[int32]int32{1: 10000},
		},
		procs: map[int32]azerothcore.SpellProc{200: {ProcsPerMinute: 1.5, CooldownMs: 45000}},
	}

	values, chain := c.spellChainValues(100, castBy{label: "item 1", cast: true, cooldownMs: 120000})
	if !slices.Equal(chain, []int32{100, 101}) {
		t.Errorf("chain = %v", chain)
	}
	if got, want := labels(values, true), []string{"101 effect1", "101 duration"}; !slices.Equal(got, want) {
		t.Errorf("tooltip values = %v, want %v", got, want)
	}
	if got, want := labels(values, false), []string{"item 1 cooldown"}; !slices.Equal(got, want) {
		t.Errorf("server-only values = %v, want %v (item cooldown replaces the spell's)", got, want)
	}

	values, _ = c.spellChainValues(100, castBy{label: "item 1", cast: true, cooldownMs: -1})
	if got, want := labels(values, true), []string{"100 cooldown", "101 effect1", "101 duration"}; !slices.Equal(got, want) {
		t.Errorf("tooltip values = %v, want %v (spell's own cooldown when the item sets none)", got, want)
	}

	values, _ = c.spellChainValues(100, castBySpell)
	if got := labels(values, true); slices.Contains(got, "100 cooldown") {
		t.Errorf("passive aura got a cooldown: %v", got)
	}

	values, _ = c.spellChainValues(200, castBySpell)
	if got, want := labels(values, false), []string{"200 ppm", "200 proc cooldown"}; !slices.Equal(got, want) {
		t.Errorf("server-only values = %v, want %v", got, want)
	}
	if got, want := labels(values, true), []string{"200 proc chance"}; !slices.Equal(got, want) {
		t.Errorf("tooltip values = %v, want %v", got, want)
	}

	values, _ = c.spellChainValues(200, castBy{label: "enchant 3", cooldownMs: -1, procRateSet: true, ppm: 1})
	if got, want := labels(values, false), []string{"enchant 3 ppm"}; !slices.Equal(got, want) || len(labels(values, true)) > 0 {
		t.Errorf("values = %+v, want only the enchant's ppm", values)
	}
}

func TestModuleSQLMatchesWholeColumnNames(t *testing.T) {
	refs := moduleRefs{items: map[int32][]string{}, spells: map[int32][]string{}, enchants: map[int32][]string{}}
	refs.addSQL("UPDATE `item_template` SET `stat_value1` = 30, `DisenchantID` = 65, `displayid` = 1234 WHERE `entry` = 31034;", "tbc.sql")

	if got := sortedKeys(refs.items); !slices.Equal(got, []int32{31034}) {
		t.Errorf("items = %v, want only the entry", got)
	}
}
