package main

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"google.golang.org/protobuf/reflect/protoreflect"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
	"github.com/wowsims/wotlk/sim/core/serverdata"
	"github.com/wowsims/wotlk/tools/acore/spellids/spellset"
)

var updateTalentPaths = flag.Bool("update", false, "rewrite "+fixtureDir+"/"+talentPathsFile)

// The sweep's paths follow the implemented talents, the tree files and the setups here. Rewrite the file
// with tools/acore/dock.sh test ./tools/simval -run TestTalentPathsFile -update.
func TestTalentPathsFile(t *testing.T) {
	root, err := checkoutRoot()
	if err != nil {
		t.Fatal(err)
	}
	paths, err := talentPathsFromData(root)
	if err != nil {
		t.Fatal(err)
	}
	want, err := marshalTalentPaths(paths)
	if err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(root, fixtureDir, talentPathsFile)
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
		t.Errorf("%s/%s is stale; rewrite it with tools/acore/dock.sh test ./tools/simval -run TestTalentPathsFile -update",
			fixtureDir, talentPathsFile)
	}

	byClass := map[uint8][]string{}
	for _, p := range paths {
		var points, fillers int32
		for _, s := range p.Steps {
			points += s.Rank
			if s.Filler {
				fillers += s.Rank
			}
		}
		byClass[p.Class] = append(byClass[p.Class], fmt.Sprintf("%s %d steps, %d points (%d filler)", p.Name,
			len(p.Steps), points, fillers))
	}
	for _, c := range sweepClasses {
		t.Logf("class %d: %s", c.class, strings.Join(byClass[c.class], "; "))
	}
}

// pathSetup is the setup a path was built on: its name without a trailing count.
func pathSetup(path talentPath) (sweepClass, sweepSetup, bool) {
	name := strings.TrimRight(path.Name, "0123456789")
	for _, c := range sweepClasses {
		for _, s := range c.setups {
			if c.class == path.Class && s.name == name {
				return c, s, true
			}
		}
	}
	return sweepClass{}, sweepSetup{}, false
}

// Each path is one the server would take from a character spending its points through the talent pane:
// at most 71 points, a row opened by 5 points a row in that tree before it, a prerequisite at max rank
// before what needs it. Between them the paths take every talent the sim implements at max rank, each
// in a setup that shows its sheet change. Checked on the committed file, apart from the generator.
func TestTalentPathsAreLegal(t *testing.T) {
	root, err := checkoutRoot()
	if err != nil {
		t.Fatal(err)
	}
	paths, err := readTalentPaths(filepath.Join(root, fixtureDir, talentPathsFile))
	if err != nil {
		t.Fatal(err)
	}
	implemented, err := readImplementedTalents(filepath.Join(root, fixtureDir, implementedTalentsFile))
	if err != nil {
		t.Fatal(err)
	}
	dump, err := spellset.ReadDump(filepath.Join(root, spellset.DefaultDumpPath))
	if err != nil || len(dump) == 0 {
		t.Fatalf("spelldump: %v", err)
	}
	itemClasses, err := loadItemClasses()
	if err != nil {
		t.Fatal(err)
	}

	for _, c := range sweepClasses {
		trees, err := c.talents(root, implemented, dump, itemClasses)
		if err != nil {
			t.Fatal(err)
		}
		byField := map[string]*sweepTalent{}
		for _, tree := range trees {
			for _, talent := range tree {
				byField[talent.field] = talent
			}
		}
		covered := map[string]bool{}

		for _, path := range paths {
			if path.Class != c.class {
				continue
			}
			_, setup, ok := pathSetup(path)
			if !ok {
				t.Errorf("%s: no setup of that name", path.Name)
				continue
			}
			if path.Race != c.race || path.Spec != string(setup.spec) || !slices.Equal(path.Items, setup.items) ||
				!slices.Equal(path.Auras, setup.auras) {
				t.Errorf("%s: race, spec, items or auras aren't its setup's", path.Name)
			}

			spent := make([]int32, len(trees))
			ranks := map[string]int32{}
			var total int32
			for i, step := range path.Steps {
				talent := byField[step.Talent]
				label := fmt.Sprintf("%s step %d %s", path.Name, i+1, step.Talent)
				switch {
				case talent == nil:
					t.Errorf("%s: no such talent", label)
					continue
				case ranks[step.Talent] > 0:
					t.Errorf("%s: learned twice", label)
				case step.Rank < 1 || step.Rank > talent.maxPoints || step.Spell != talent.spells[step.Rank-1]:
					t.Errorf("%s: rank %d spell %d, its ranks are %v", label, step.Rank, step.Spell, talent.spells)
				case step.Filler == talent.implemented:
					t.Errorf("%s: filler %v, implemented %v", label, step.Filler, talent.implemented)
				}
				if spent[talent.tree] < int32(pointsPerTalentRow*talent.row) {
					t.Errorf("%s: row %d with %d points in the tree", label, talent.row, spent[talent.tree])
				}
				if talent.prereq >= 0 {
					pre := trees[talent.tree][talent.prereq]
					if ranks[pre.field] < pre.maxPoints {
						t.Errorf("%s: before its prerequisite %s at max rank", label, pre.field)
					}
				}
				if dep, ok := c.after[step.Talent]; ok && ranks[dep] == 0 {
					t.Errorf("%s: before %s", label, dep)
				}
				ranks[step.Talent] = step.Rank
				spent[talent.tree] += step.Rank
				total += step.Rank
				if talent.implemented && step.Rank == talent.maxPoints && slices.Contains(talent.hosts, setup.name) {
					covered[step.Talent] = true
				}
			}
			if total > maxTalentPoints {
				t.Errorf("%s: %d points", path.Name, total)
			}
		}

		for _, tree := range trees {
			for _, talent := range tree {
				if talent.implemented && !covered[talent.field] {
					t.Errorf("class %d: %s isn't learned at max rank in any of %v", c.class, talent.field, talent.hosts)
				}
			}
		}
	}
}

// The sweep's server character has to stand the way the sim stands the path's character, so a path lists
// what the sim has up after a reset that isn't a passive talent or racial: its stance or form, a presence
// or aspect, the raid buffs the class gives itself, and what a talent puts up once it's in (Moonkin Form,
// Shadowform, Bone Shield, Trueshot Aura). Checked after every step the sim can build.
func TestTalentPathsCoverTheSimsStart(t *testing.T) {
	needsItemDB(t)
	root, err := checkoutRoot()
	if err != nil {
		t.Fatal(err)
	}
	paths, err := readTalentPaths(filepath.Join(root, fixtureDir, talentPathsFile))
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range paths {
		c, _, _ := pathSetup(path)
		reported := map[string]bool{}
		report := func(format string, args ...any) {
			if msg := fmt.Sprintf(format, args...); !reported[msg] {
				reported[msg] = true
				t.Error(msg)
			}
		}
		for n := 0; n <= len(path.Steps); n++ {
			if n > 0 && slices.Contains(c.last, path.Steps[n-1].Talent) {
				break
			}
			s, err := path.goldcharSetup(n)
			if err != nil {
				t.Fatal(err)
			}
			listed := func(id int32) bool {
				_, unlisted := unlistedStartingAuras[id]
				return unlisted || slices.Contains(s.Auras, id)
			}
			d, err := newGoldcharSim(goldcharInfo(t, s, serverSheet{}).damageRecord, s)
			if err != nil {
				report("%s after %d steps: %v", path.Name, n, err)
				break
			}
			// with the listed auras on, which put a DPS warrior with no talents out of the Battle Stance its
			// primary tree picks
			d.start()
			for _, aura := range d.char.GetAuras() {
				id := aura.ActionID.SpellID
				if !aura.IsActive() || id == 0 || listed(id) {
					continue
				}
				if spell := serverdata.SpellByID(id); spell != nil && spell.Flags&serverdata.FlagPassive != 0 {
					continue
				}
				report("%s starts with %d (%s) by step %d, which it doesn't cast", path.Name, id, aura.Label, n)
			}

			buffs := &proto.RaidBuffs{}
			d.sim.Raid.Parties[0].Players[0].AddRaidBuffs(buffs)
			buffs.ProtoReflect().Range(func(fd protoreflect.FieldDescriptor, _ protoreflect.Value) bool {
				switch id, ok := classRaidBuffs[fd.Name()]; {
				case !ok:
					report("%s gives itself %s, which classRaidBuffs doesn't know", path.Name, fd.Name())
				case id != 0 && !listed(id):
					report("%s gives itself %s by step %d, so it has to cast %d", path.Name, fd.Name(), n, id)
				}
				return true
			})
		}
	}
}

// A full simval.jsonl recapture would hold TestSimvalTalentSweep's records too, if SIMVAL_TALENT_PATHS
// were set for it. Most are naked, and a talent's aura keeps them from the base stats checks; the
// character before its first talent is a bare one, which gets them.
func TestTalentSweepInfoRecordInTheReplayFixture(t *testing.T) {
	info := func(auras string) record {
		return spellRecord(t, fmt.Sprintf(`{"schema": 1, "command": "info",
			"attacker": {"name": "Svtleadabcde", "isPlayer": true, "class": 8, "race": 7, "level": 80,
				"maxHealth": 1, "stats": {"intellect": 1000}, "resistances": [0, 0, 0, 0, 0, 0, 0],
				"attacks": [{"type": "mainhand"}, {"type": "offhand"}, {"type": "ranged"}],
				"sheet": {"critMainhand": 5, "spellCrit": [0, 5, 5, 5, 5, 5, 5]}, "auras": [%s]},
			"target": {"name": "Dummy", "entry": 999000, "level": 83},
			"character": {"talents": [12503], "glyphs": [0, 0, 0, 0, 0, 0], "equipment": []}}`, auras))
	}
	// Arcane Mind 5/5
	if checks := CheckRecord(info(`{"id": 12503, "durationMs": -1}`)); len(checks) > 0 {
		t.Errorf("a talented naked character got checks %+v", checks)
	}
	if checks := CheckRecord(info(`{"id": 42995, "durationMs": 3600000}`)); len(checks) == 0 {
		t.Error("a naked character without talents got no base stats checks")
	} else if _, failed := checkNames(checks); !slices.Contains(failed, "intellect") {
		t.Errorf("intellect 1000 passed the base stats checks: %+v", checks)
	}
}

// The setups' items are what the generator thinks they are, checked against the sim's item records.
func TestTalentPathsItemClasses(t *testing.T) {
	needsItemDB(t)
	classes, err := loadItemClasses()
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range sweepClasses {
		for _, s := range c.setups {
			for _, item := range s.items {
				simItem, ok := core.LookupItem(item.ID)
				if !ok {
					t.Errorf("%s: item %d isn't in the sim's database", s.name, item.ID)
					continue
				}
				want := serverItemClassOf(simItem.WeaponType, simItem.HandType, simItem.RangedWeaponType)
				if classes[item.ID] != want {
					t.Errorf("%s: item %d is %+v in db.json, %+v in the sim", s.name, item.ID, classes[item.ID], want)
				}
			}
		}
	}
}
