package main

import (
	"bytes"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/wowsims/wotlk/tools/acore/spellids/spellset"
)

// droppedCostAuras are spell steps of costAuraFixture, or checks of them, that disagree with the sim, each with
// a row in the fixture README.
var droppedCostAuras = slices.Concat(
	// Penance: the sim takes Mental Agility's cut though the server's mask misses it, and counts the cast's
	// channel, which the server's dummy 53007 doesn't have
	spellModDrops("PriestDiscipline", "powerInfusion", "cost", 53007),
	spellModDrops("PriestDiscipline", "powerInfusion", "channel", 53007),
)

func costAuraListFromData(t *testing.T, root string) costAuraList {
	t.Helper()
	dump, err := spellset.ReadDump(filepath.Join(root, spellset.DefaultDumpPath))
	if err != nil {
		t.Fatal(err)
	}
	talents, err := readImplementedTalents(filepath.Join(root, fixtureDir, implementedTalentsFile))
	if err != nil {
		t.Fatal(err)
	}
	lits, err := spellset.ScanSim(root)
	if err != nil {
		t.Fatal(err)
	}
	list, err := buildCostAuraList(dump, talents, lits.Spells)
	if err != nil {
		t.Fatal(err)
	}
	return list
}

// The list follows the implemented talents, the spelldump and the spell ids the sim names. Rewrite it with
// tools/acore/dock.sh test ./tools/simval -run TestCostAuraSpellsFile -update.
func TestCostAuraSpellsFile(t *testing.T) {
	root, err := checkoutRoot()
	if err != nil {
		t.Fatal(err)
	}
	list := costAuraListFromData(t, root)
	want, err := marshalCostAuraList(list)
	if err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(root, fixtureDir, costAuraSpellsFile)
	if *updateTalentPaths {
		if err := os.WriteFile(file, want, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	got, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(bytes.ReplaceAll(got, []byte("\r\n"), []byte("\n")), want) {
		t.Errorf("%s/%s is stale; rewrite it with tools/acore/dock.sh test ./tools/simval -run TestCostAuraSpellsFile -update",
			fixtureDir, costAuraSpellsFile)
	}

	// what the server's rule must give: Frost Channeling on fire, frost and arcane spells but not a
	// Frostfire Bolt's frost alone, Arcane Focus on arcane spells only
	spells := map[string][]int32{}
	for _, talent := range list.Talents {
		for _, g := range talent.Groups {
			for _, s := range g.Spells {
				spells[talent.Talent] = append(spells[talent.Talent], s.ID)
			}
		}
	}
	for talent, c := range map[string]struct{ in, out []int32 }{
		"frostChanneling": {in: []int32{42842, 42873, 42897, 47610}},
		"arcaneFocus":     {in: []int32{42897, 42846}, out: []int32{42842, 42873, 47610}},
		"precision":       {in: []int32{42842, 42873, 42897}},
		"powerInfusion":   {in: []int32{48125, 48300}},
	} {
		for _, id := range c.in {
			if !slices.Contains(spells[talent], id) {
				t.Errorf("%s doesn't list %d: %v", talent, id, spells[talent])
			}
		}
		for _, id := range c.out {
			if slices.Contains(spells[talent], id) {
				t.Errorf("%s lists %d", talent, id)
			}
		}
	}
}

func costAuraSteps(t *testing.T) map[int32]spellModStep {
	t.Helper()
	root, err := checkoutRoot()
	if err != nil {
		t.Fatal(err)
	}
	steps, err := readSpellModList(filepath.Join(root, fixtureDir, costAuraSpellsFile))
	if err != nil {
		t.Fatal(err)
	}
	return steps
}

// TestCostAuraRecords replays a live TestSimvalCostAuraSweep run, its SPELLMOD lines, the way
// TestSpellModRecords does. Skips without the capture.
func TestCostAuraRecords(t *testing.T) {
	needsItemDB(t)
	root, err := checkoutRoot()
	if err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(root, fixtureDir, costAuraFixture)
	if _, err := os.Stat(file); os.IsNotExist(err) {
		t.Skipf("no capture at %s; record one with mod-sim-validation's TestSimvalCostAuraSweep and keep its SPELLMOD lines", file)
	}
	raw, err := readJSONLines[spellModLine](file)
	if err != nil {
		t.Fatal(err)
	}
	lines, notes, err := latestSpellModLines(raw)
	if err != nil {
		t.Fatal(err)
	}
	for _, note := range notes {
		t.Log(note)
	}
	report := replaySpellMods(allTalentPaths(t), costAuraSteps(t), lines, droppedCostAuras)
	for _, line := range report.logs {
		t.Log(line)
	}
	for _, e := range report.errors {
		t.Error(e)
	}
}

// A server agreeing with the sim agrees on every listed spell of every cost aura step, and each talent's cost
// moves on some spell.
func TestCostAuraAgreesWithTheSim(t *testing.T) {
	needsItemDB(t)
	steps := costAuraSteps(t)
	costMoved := map[string]int{}
	compared := 0
	for _, path := range allTalentPaths(t) {
		sims := newSpellModSims(path)
		for n, step := range path.Steps {
			listed, ok := steps[step.Spell]
			if !ok {
				continue
			}
			var cast int32
			if !listed.passive {
				cast = step.Spell
			}
			for _, s := range listed.group.Spells {
				pair := sims.pair(n+1, cast, s.ID)
				if pair.err != nil {
					t.Fatalf("%s step %d: %v", path.Name, n+1, pair.err)
				}
				before, after := spellModLine{}, spellModLine{}
				if pair.spec != "" {
					before = spellModLineFrom(t, sims, n, 0, s.ID, pair.spec, pair.teach)
					after = spellModLineFrom(t, sims, n+1, cast, s.ID, pair.spec, pair.teach)
				}
				result := compareSpellMod(path, before, after, pair)
				if result.class == spellModDisagrees {
					t.Errorf("%s %s %d %s%s: %s", path.Name, step.Talent, s.ID, s.Name, pair.label(path),
						strings.Join(failedChecks(result.checks), "; "))
				}
				if result.class != spellModNotCompared {
					compared++
				}
				if slices.ContainsFunc(result.moved, func(m string) bool { return strings.HasPrefix(m, "cost ") }) {
					costMoved[path.Name+" "+step.Talent]++
				}
			}
		}
	}
	for _, step := range []string{"MageArcane arcaneFocus", "MageFire precision", "MageFrost frostChanneling",
		"PriestDiscipline powerInfusion"} {
		if costMoved[step] == 0 {
			t.Errorf("%s moves no spell's cost in the sim", step)
		}
	}
	t.Logf("%d spell steps compared, cost moved on %v", compared, costMoved)
}

// The replay catches a cost the talent cuts wrong, and one it cuts on a spell the sim leaves alone.
func TestCostAuraCatchesMismatches(t *testing.T) {
	needsItemDB(t)
	steps := costAuraSteps(t)
	for _, c := range []struct {
		name, path, talent string
		spell              int32
		change             int32
		class              string
	}{
		{"the cut as the sim has it", "MageFrost", "frostChanneling", 42842, 0, spellModAgrees},
		{"a mana short of the cut", "MageFrost", "frostChanneling", 42842, 1, spellModDisagrees},
		{"a cut the sim misses", "MageArcane", "arcaneFocus", 42897, -10, spellModDisagrees},
	} {
		t.Run(c.name, func(t *testing.T) {
			sc := newSpellModCase(t, steps, c.path, c.talent, c.spell)
			sc.after.Timing.Cost.Amount += c.change
			result := compareSpellMod(sc.path, sc.before, sc.after, sc.pair)
			if _, failed := checkNames(result.checks); result.class != c.class ||
				(c.class == spellModDisagrees) != slices.Equal(failed, []string{"cost"}) {
				t.Errorf("%s, failed %v; want %s: %s", result.class, failed, c.class, strings.Join(failedChecks(result.checks), "; "))
			}
		})
	}
}
