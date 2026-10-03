package main

import (
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/wowsims/wotlk/tools/acore/spellids/spellset"
)

func flatMod(family int64, op int64, mask [3]int64) spellMod {
	return spellMod{Type: "flat", Op: opNames[op], ClassMask: mask, op: op, family: family}
}

func TestMatchesIsSpellInfoIsAffected(t *testing.T) {
	spell := &spellset.DumpSpell{Family: 3, FamilyFlags: [3]int64{0, 0x40, 0}}
	for _, tc := range []struct {
		name string
		mod  spellMod
		want bool
	}{
		{"mask hits the second word", flatMod(3, 0, [3]int64{1, 0x40, 0}), true},
		{"mask misses", flatMod(3, 0, [3]int64{0x40, 0, 0x40}), false},
		{"other family", flatMod(4, 0, [3]int64{0, 0x40, 0}), false},
		{"empty mask takes the whole family", flatMod(3, 0, [3]int64{}), true},
		{"family 0 takes every spell", flatMod(0, 0, [3]int64{1, 0, 0}), true},
	} {
		if got := tc.mod.matches(spell); got != tc.want {
			t.Errorf("%s: matches = %t, want %t", tc.name, got, tc.want)
		}
	}
}

func TestExcludedBy(t *testing.T) {
	var ignoresMods spellset.DumpSpell
	ignoresMods.Attributes[3] = attr3IgnoreCasterModifiers
	infinite := spellset.DumpSpell{DurationMs: -1}
	for _, tc := range []struct {
		name  string
		op    int64
		spell spellset.DumpSpell
		want  string
	}{
		{"damage on a spell ignoring caster mods", 0, ignoresMods, "SPELL_ATTR3_IGNORE_CASTER_MODIFIERS"},
		{"duration on a spell ignoring caster mods", spellModDuration, ignoresMods, ""},
		{"duration on an infinite spell", spellModDuration, infinite, "infinite duration"},
		{"cost on an infinite spell", 14, infinite, ""},
	} {
		if got := flatMod(3, tc.op, [3]int64{1}).excludedBy(&tc.spell); got != tc.want {
			t.Errorf("%s: excludedBy = %q, want %q", tc.name, got, tc.want)
		}
	}
}

func modEffect(index, aura, op int64, mask [3]int64) spellset.DumpEffect {
	return spellset.DumpEffect{Index: index, Aura: aura, MiscValue: op, ClassMask: mask}
}

func TestBuildListGroupsRanksAndFiltersSpells(t *testing.T) {
	dump := spellset.Dump{
		// ranks 1 and 2 share a mask; rank 3 widens it and adds a duration mod
		101: {ID: 101, Family: 3, Passive: true, Effects: []spellset.DumpEffect{modEffect(0, auraAddPctModifier, 0, [3]int64{1})}},
		102: {ID: 102, Family: 3, Passive: true, Effects: []spellset.DumpEffect{modEffect(0, auraAddPctModifier, 0, [3]int64{1})}},
		103: {ID: 103, Family: 3, Passive: true, Effects: []spellset.DumpEffect{
			modEffect(0, auraAddPctModifier, 0, [3]int64{3}), modEffect(1, auraAddFlatModifier, 1, [3]int64{1})}},
		// a talent without spell mods
		201: {ID: 201, Family: 3, Passive: true, Effects: []spellset.DumpEffect{{Aura: 4}}},

		1: {ID: 1, Name: "Bolt", Family: 3, FamilyFlags: [3]int64{1}, DurationMs: -1},
		2: {ID: 2, Name: "Blast", Family: 3, FamilyFlags: [3]int64{2}},
		3: {ID: 3, Name: "Pet Blast", Family: 3, FamilyFlags: [3]int64{2}},
		4: {ID: 4, Name: "Unnamed", Family: 3, FamilyFlags: [3]int64{1}},
		5: {ID: 5, Name: "Shared", Family: 3, FamilyFlags: [3]int64{3}},
	}
	talents := []implementedTalent{
		{Class: "mage", Tree: "Fire", FieldName: "a", RankSpells: []int32{101, 102, 103}},
		{Class: "mage", Tree: "Fire", FieldName: "b", RankSpells: []int32{201}},
		{Class: "warlock", Tree: "Destruction", FieldName: "c", RankSpells: []int32{}},
	}
	named := map[int32][]spellset.Ref{
		1: {{File: "sim/mage/bolt.go"}},
		2: {{File: "sim/mage/blast.go"}},
		3: {{File: "sim/warlock/pet.go"}},
		5: {{File: "sim/warlock/x.go"}, {File: "sim/core/debuffs.go"}},
	}
	list, stats, err := buildList(dump, talents, named)
	if err != nil {
		t.Fatal(err)
	}
	if len(list.Talents) != 1 || list.Talents[0].Talent != "a" {
		t.Fatalf("talents = %+v, want only a", list.Talents)
	}
	groups := list.Talents[0].Groups
	if len(groups) != 2 || !slices.Equal(groups[0].Ranks, []int32{101, 102}) || !slices.Equal(groups[1].Ranks, []int32{103}) {
		t.Fatalf("groups = %+v, want ranks [101 102] and [103]", groups)
	}
	want := []affectedSpell{{ID: 1, Name: "Bolt", Ops: []string{"DAMAGE"}}, {ID: 5, Name: "Shared", Ops: []string{"DAMAGE"}}}
	if !reflect.DeepEqual(groups[0].Spells, want) {
		t.Errorf("rank 1 spells = %+v, want %+v", groups[0].Spells, want)
	}
	want = []affectedSpell{
		{ID: 1, Name: "Bolt", Ops: []string{"DAMAGE"}},
		{ID: 2, Name: "Blast", Ops: []string{"DAMAGE"}},
		{ID: 5, Name: "Shared", Ops: []string{"DAMAGE", "DURATION"}},
	}
	if !reflect.DeepEqual(groups[1].Spells, want) {
		t.Errorf("rank 3 spells = %+v, want %+v", groups[1].Spells, want)
	}
	wantExcluded := []excludedSpell{{ID: 1, Name: "Bolt", Op: "DURATION", Reason: "infinite duration"}}
	if !reflect.DeepEqual(groups[1].Excluded, wantExcluded) {
		t.Errorf("rank 3 excluded = %+v, want %+v", groups[1].Excluded, wantExcluded)
	}
	if stats.unnamed != 1 || stats.otherClass != 1 {
		t.Errorf("unnamed %d, other class %d; want 1 (Unnamed) and 1 (Pet Blast)", stats.unnamed, stats.otherClass)
	}
}

func TestBuildListFailsOnAnUnmodelledScript(t *testing.T) {
	dump := spellset.Dump{1: {ID: 1, Name: "Talent", Family: 4, Passive: true, Scripts: []string{"spell_warr_glyph_of_sunder_armor"}}}
	talents := []implementedTalent{{Class: "warrior", FieldName: "x", RankSpells: []int32{1}}}
	if _, _, err := buildList(dump, talents, nil); err == nil {
		t.Error("no error for a rank spell whose script rewrites its spell mods")
	}
}

func TestMarshalListRoundTrips(t *testing.T) {
	list := &spellModList{Talents: []talentMods{
		{Class: "mage", Tree: "Fire", Talent: "a", Passive: true, Equipped: &equippedItem{Class: 2, SubClassMask: 3},
			Groups: []rankGroup{
				{Ranks: []int32{1, 2}, Mods: []spellMod{{Effect: 0, Type: "pct", Op: "DAMAGE", ClassMask: [3]int64{1}}},
					Spells:   []affectedSpell{{ID: 9, Name: "Bolt", Ops: []string{"DAMAGE"}}, {ID: 10, Name: "Blast", Ops: []string{"DAMAGE"}}},
					Excluded: []excludedSpell{{ID: 11, Name: "Nova", Op: "DAMAGE", Reason: "infinite duration"}}},
				{Ranks: []int32{3}, Mods: []spellMod{}, Spells: []affectedSpell{}},
			}},
		{Class: "mage", Tree: "Frost", Talent: "b", Groups: []rankGroup{{Ranks: []int32{4}, Mods: []spellMod{}, Spells: []affectedSpell{}}}},
	}}
	data, err := marshalList(list)
	if err != nil {
		t.Fatal(err)
	}
	var got spellModList
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("%v\n%s", err, data)
	}
	if !reflect.DeepEqual(&got, list) {
		t.Errorf("round trip lost something:\n%s", data)
	}
}

// TestSpellModScriptsMatchServer reads the server source dock.sh mounts at /ac, so the rule's script
// exceptions stay complete: every DoEffectCalcSpellMod script is in spellModScripts, and nothing beyond the
// core's own files names OnIsAffectedBySpellModCheck, which would force a mod onto spells the rule skips.
func TestSpellModScriptsMatchServer(t *testing.T) {
	const ac = "/ac"
	if _, err := os.Stat(filepath.Join(ac, "src", "server")); err != nil {
		t.Skipf("no server source at %s (tools/acore/dock.sh mounts it)", ac)
	}
	calcSpellMod := regexp.MustCompile(`DoEffectCalcSpellMod\s*\+=\s*AuraEffectCalcSpellModFn\(\s*(\w+)::`)
	hookFiles := []string{
		"src/server/game/Scripting/ScriptDefines/GlobalScript.cpp",
		"src/server/game/Scripting/ScriptDefines/GlobalScript.h",
		"src/server/game/Scripting/ScriptMgr.h",
		"src/server/game/Spells/SpellInfo.cpp",
	}
	var scripts []string
	for _, dir := range []string{"src", "modules"} {
		err := filepath.WalkDir(filepath.Join(ac, dir), func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				if d.Name() == ".git" || d.Name() == "build" || d.Name() == "node_modules" {
					return filepath.SkipDir
				}
				return nil
			}
			if ext := filepath.Ext(path); ext != ".cpp" && ext != ".h" {
				return nil
			}
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			rel := filepath.ToSlash(strings.TrimPrefix(path, ac+"/"))
			text := string(data)
			// src/server/game only declares the hook, with a usage example in SpellScript.h
			if !strings.HasPrefix(rel, "src/server/game/") {
				for _, m := range calcSpellMod.FindAllStringSubmatch(text, -1) {
					scripts = append(scripts, m[1])
				}
			}
			if (strings.Contains(text, "OnIsAffectedBySpellModCheck") ||
				strings.Contains(text, "GLOBALHOOK_ON_IS_AFFECTED_BY_SPELL_MOD_CHECK")) && !slices.Contains(hookFiles, rel) {
				t.Errorf("%s hooks OnIsAffectedBySpellModCheck: the rule doesn't model it, so its spells need an exception", rel)
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	slices.Sort(scripts)
	scripts = slices.Compact(scripts)
	var known []string
	for name := range spellModScripts {
		known = append(known, name)
	}
	slices.Sort(known)
	if !slices.Equal(scripts, known) {
		t.Errorf("DoEffectCalcSpellMod scripts in the server source:\n  %v\nspellModScripts:\n  %v", scripts, known)
	}
}
