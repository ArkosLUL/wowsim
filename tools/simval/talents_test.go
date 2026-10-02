package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// sweepDrop is a step of talentsFixture, or one field of it, that disagrees with the sim, each with a
// row in the fixture README. An empty field drops the whole step. The records stay in the fixture, since
// the next step's change starts from them: a dropped step or field is only logged, and one that agrees
// fails.
type sweepDrop struct {
	path, talent, field string
}

var droppedSweep = []sweepDrop{}

// TestTalentSweepRecords replays a live TestSimvalTalentSweep run, trimmed by `simval talents`: each
// step's sheet change against the sim's change from the same talent. Skips without the capture, like
// TestGoldenCharacters. Every step's class is logged on a SWEEPSIM line.
func TestTalentSweepRecords(t *testing.T) {
	needsItemDB(t)
	root, err := checkoutRoot()
	if err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(root, fixtureDir, talentsFixture)
	if _, err := os.Stat(file); os.IsNotExist(err) {
		t.Skipf("no capture at %s; record one with mod-sim-validation's TestSimvalTalentSweep and trim it with `simval talents`", file)
	}
	records, err := readJSONLines[talentRecord](file)
	if err != nil {
		t.Fatal(err)
	}
	replayTalentSweep(t, allTalentPaths(t), records, droppedSweep)
}

// replayTalentSweep compares every step of every path, holding the records to drops.
func replayTalentSweep(t *testing.T, paths []talentPath, records []talentRecord, drops []sweepDrop) {
	t.Helper()
	byPath := map[string]map[int]talentRecord{}
	for _, rec := range records {
		i := slices.IndexFunc(paths, func(p talentPath) bool { return p.Name == rec.Path })
		switch {
		case i < 0:
			t.Errorf("%s: no path %s", rec.Bot, rec.Path)
			continue
		case !paths[i].fits(rec) || paths[i].stepOf(rec.Talents) != rec.Step:
			t.Errorf("%s: class, race, items or talents aren't %s's after step %d", rec.Bot, rec.Path, rec.Step)
			continue
		}
		if byPath[rec.Path] == nil {
			byPath[rec.Path] = map[int]talentRecord{}
		}
		if _, ok := byPath[rec.Path][rec.Step]; ok {
			t.Errorf("%s: two records after step %d", rec.Path, rec.Step)
		}
		byPath[rec.Path][rec.Step] = rec
	}

	used := map[sweepDrop]bool{}
	dropped := func(d sweepDrop) bool {
		if slices.Contains(drops, d) {
			used[d] = true
			return true
		}
		return false
	}
	counts := map[string]int{}
	var noField []string
	for _, path := range paths {
		t.Run(path.Name, func(t *testing.T) {
			recs := byPath[path.Name]
			sims := map[int]sweepSim{}
			sim := func(n int) sweepSim {
				if _, ok := sims[n]; !ok {
					sims[n] = path.sweepSim(n)
				}
				return sims[n]
			}
			for n, step := range path.Steps {
				k := n + 1
				label := fmt.Sprintf("%s step %d %s %d/%d", path.Name, k, step.Talent, step.Rank, step.Spell)
				if step.Filler {
					label += " (filler)"
				}
				whole := sweepDrop{path.Name, step.Talent, ""}
				before, ok := recs[k-1]
				after, ok2 := recs[k]
				if !ok || !ok2 {
					if dropped(whole) {
						t.Logf("%s (dropped): no record before or after it", label)
					} else {
						t.Errorf("%s: no record before or after it", label)
					}
					continue
				}

				result := compareSweepStep(before, after, sim(k-1), sim(k))
				counts[result.class]++
				if result.class == sweepNoField {
					noField = append(noField, fmt.Sprintf("%s %s", path.Name, step.Talent))
				}
				line := fmt.Sprintf("SWEEPSIM %s: %s", label, result.class)
				if len(result.moved) > 0 {
					line += "; " + strings.Join(result.moved, ", ")
				}
				t.Log(line)

				if dropped(whole) {
					if result.class == sweepDisagrees {
						t.Logf("%s (dropped): %s", label, strings.Join(failedChecks(result.checks), "; "))
					} else {
						t.Errorf("%s %s, so it shouldn't be dropped", label, result.class)
					}
					continue
				}
				for _, c := range result.checks {
					switch isDropped := dropped(sweepDrop{path.Name, step.Talent, c.Name}); {
					case isDropped && c.Passed:
						t.Errorf("%s: %s passes, so it shouldn't be dropped: %s", label, c.Name, c.Detail)
					case isDropped:
						t.Logf("%s: %s (dropped): %s", label, c.Name, c.Detail)
					case !c.Passed:
						t.Errorf("%s: %s: %s", label, c.Name, c.Detail)
					}
				}
			}
		})
	}

	for _, d := range drops {
		if !used[d] {
			t.Errorf("%+v is dropped, but no step of the capture matches it", d)
		}
	}
	t.Logf("%d steps agree, %d move no field, %d disagree", counts[sweepAgrees], counts[sweepNoField], counts[sweepDisagrees])
	t.Logf("moving no field on either side: %s", strings.Join(noField, ", "))
}

// A whole run as a server agreeing with the sim would record it, raw lines through the trim, replays
// clean.
func TestTalentSweepReplaysAFullRun(t *testing.T) {
	needsItemDB(t)
	paths := allTalentPaths(t)
	var raw []string
	for i, path := range paths {
		bot := fmt.Sprintf("Svtlead%c%c", 'a'+i/26, 'a'+i%26)
		for n := 0; n <= len(path.Steps); n++ {
			rec := talentRecordFrom(t, path, n, path.sweepSim(n))
			rec.Bot = bot
			raw = append(raw, rawTalentLine(t, rec))
		}
	}
	records, notes, err := trimTalentRecords(strings.NewReader(strings.Join(raw, "\n")), paths)
	if err != nil || len(notes) > 0 {
		t.Fatalf("trim: %v %v", err, notes)
	}
	if want := len(raw); len(records) != want {
		t.Fatalf("%d records, want %d", len(records), want)
	}
	replayTalentSweep(t, paths, records, nil)
}

func allTalentPaths(t *testing.T) []talentPath {
	t.Helper()
	root, err := checkoutRoot()
	if err != nil {
		t.Fatal(err)
	}
	paths, err := readTalentPaths(filepath.Join(root, fixtureDir, talentPathsFile))
	if err != nil {
		t.Fatal(err)
	}
	return paths
}

func talentPathNamed(t *testing.T, name string) talentPath {
	t.Helper()
	paths := allTalentPaths(t)
	i := slices.IndexFunc(paths, func(p talentPath) bool { return p.Name == name })
	if i < 0 {
		t.Fatalf("no path %s", name)
	}
	return paths[i]
}

// stepNamed is how many steps of the path it takes to learn the talent.
func stepNamed(t *testing.T, path talentPath, talent string) int {
	t.Helper()
	i := slices.IndexFunc(path.Steps, func(s talentStep) bool { return s.Talent == talent })
	if i < 0 {
		t.Fatalf("%s doesn't learn %s", path.Name, talent)
	}
	return i + 1
}

// talentRecordFrom is the record a server agreeing with the sim would write after the path's first n
// steps: the sim's sheet with the numbers the server stores whole truncated, in the server's units.
func talentRecordFrom(t *testing.T, path talentPath, n int, sim sweepSim) talentRecord {
	t.Helper()
	if sim.err != nil {
		t.Fatalf("%s after %d steps: %v", path.Name, n, sim.err)
	}
	v := sim.values
	whole := func(name string) float64 { return math.Floor(v[name] + 1e-6) }
	talents := make([]int32, n)
	for i, step := range path.Steps[:n] {
		talents[i] = step.Spell
	}
	slices.Sort(talents)
	rec := talentRecord{
		Path: path.Name, Step: n, Bot: "Svtleadabcde", Class: path.Class, Race: path.Race, Talents: talents,
		Equipment: slices.Clone(path.Items),
		Strength:  whole("strength"), Agility: whole("agility"), Stamina: whole("stamina"),
		Intellect: whole("intellect"), Spirit: whole("spirit"),
		Resistances: []float64{whole("armor"), 0, whole("fire resistance"), whole("nature resistance"),
			whole("frost resistance"), whole("shadow resistance"), whole("arcane resistance")},
		MaxHealth: whole("max health"), AttackPower: whole("attack power"), RangedAttackPower: whole("ranged attack power"),
		HitMelee: v["melee hit %"], HitRanged: v["ranged hit %"], HitSpell: v["spell hit %"],
		CritMainhand: v["mainhand crit %"], CritOffhand: v["offhand crit %"], CritRanged: v["ranged crit %"],
		ExpertiseMainhand: v["mainhand expertise %"], ExpertiseOffhand: v["offhand expertise %"],
		SpeedMainhand: 1 / v["melee haste"], SpeedRanged: 1 / v["ranged haste"], CastSpeed: 1 / v["spell haste"],
		Dodge: v["dodge %"], Parry: v["parry %"], Block: v["block %"], BlockValue: whole("block value"),
		Auras: []int32{},
	}
	for range magicSchools {
		rec.SpellPower = append(rec.SpellPower, whole("spell power"))
		rec.SpellCrit = append(rec.SpellCrit, v["spell crit %"])
	}
	// a druid in Cat or Bear Form has mana too, but the server shows the form's power
	for _, powerType := range []uint32{6, 3, 1, 0} {
		power := sweepPowerFields[powerType]
		if value, ok := v[power.name]; ok {
			rec.PowerType, rec.MaxPower = powerType, math.Floor(value*power.scale+1e-6)
			break
		}
	}
	return rec
}

// A server that agrees with the sim, whole numbers truncated, agrees on every step of every path.
// Checks the fields' units and the truncation bounds.
func TestTalentSweepAgreesWithTheSim(t *testing.T) {
	needsItemDB(t)
	counts := map[string]int{}
	for _, path := range allTalentPaths(t) {
		prev := path.sweepSim(0)
		for k := 1; k <= len(path.Steps); k++ {
			step := path.Steps[k-1]
			cur := path.sweepSim(k)
			result := compareSweepStep(talentRecordFrom(t, path, k-1, prev), talentRecordFrom(t, path, k, cur), prev, cur)
			counts[result.class]++
			switch {
			case result.class == sweepDisagrees:
				t.Errorf("%s step %d %s: %s", path.Name, k, step.Talent, strings.Join(failedChecks(result.checks), "; "))
			case step.Filler && result.class != sweepNoField:
				t.Errorf("%s step %d: filler %s moves %v in the sim", path.Name, k, step.Talent, result.moved)
			}
			prev = cur
		}
	}
	if counts[sweepAgrees] == 0 || counts[sweepNoField] == 0 {
		t.Errorf("classes %v; want steps that agree and steps that move nothing", counts)
	}
	t.Logf("classes %v", counts)
}

func TestTalentSweepCatchesMismatches(t *testing.T) {
	needsItemDB(t)
	for _, c := range []struct {
		name, path, talent string
		change             func(after *talentRecord)
		class              string
		failed             []string
		moved              string
	}{
		{"as the sim has it", "WarriorProtection", "shieldSpecialization", nil, sweepAgrees, nil, "block %"},
		{"a point more block", "WarriorProtection", "shieldSpecialization", func(r *talentRecord) { r.Block++ },
			sweepDisagrees, []string{"block %"}, "block %"},
		{"a spell mod talent", "WarriorArms", "improvedHeroicStrike", nil, sweepNoField, nil, ""},
		{"stamina the sim doesn't give", "WarriorArms", "improvedHeroicStrike", func(r *talentRecord) {
			r.Stamina += 10
			r.MaxHealth += 100
		}, sweepDisagrees, []string{"stamina", "max health"}, "stamina"},
		// Vitality's stamina truncated on the server costs up to 10 health
		{"health a truncated stamina lower", "WarriorProtection", "vitality", func(r *talentRecord) { r.MaxHealth -= 8 },
			sweepAgrees, nil, "max health"},
		{"health a talent lower", "WarriorProtection", "vitality", func(r *talentRecord) { r.MaxHealth -= 30 },
			sweepDisagrees, []string{"max health"}, "max health"},
		{"slower swings", "RogueCombat", "lightningReflexes", func(r *talentRecord) { r.SpeedMainhand *= 1.01 },
			sweepDisagrees, []string{"melee haste"}, "melee haste"},
		{"no more runic power", "DeathknightFrost", "runicPowerMastery", func(r *talentRecord) { r.MaxPower -= 300 },
			sweepDisagrees, []string{"max runic power"}, "max runic power"},
		{"a filler", "MageFrost", "frostbite", nil, sweepNoField, nil, ""},
		{"a filler that moves the sheet", "MageFrost", "frostbite", func(r *talentRecord) { r.Stamina += 12 },
			sweepDisagrees, []string{"stamina"}, "stamina"},
		// Balance of Power is school hit, which the sheet leaves out
		{"Balance of Power off the sheet", "DruidBalance", "balanceOfPower", func(r *talentRecord) {
			r.HitSpell -= 4
			r.Auras = append(r.Auras, 33596)
		}, sweepAgrees, nil, "spell hit %"},
		{"Balance of Power off the sheet and the auras", "DruidBalance", "balanceOfPower", func(r *talentRecord) { r.HitSpell -= 4 },
			sweepDisagrees, []string{"spell hit %"}, "spell hit %"},
		{"Mental Quickness truncated", "ShamanEnhancement", "mentalQuickness", func(r *talentRecord) {
			for i := range r.SpellPower {
				r.SpellPower[i] -= 2
			}
		}, sweepAgrees, nil, "spell power"},
		{"Arcane Mind in the wrong stat", "MageArcane", "arcaneMind", func(r *talentRecord) {
			r.Intellect -= 38
			r.Spirit += 38
		}, sweepDisagrees, []string{"intellect", "spirit"}, "intellect"},
		{"Lethal Shots on the ranged sheet", "HunterMarksmanship", "lethalShots", func(r *talentRecord) { r.CritRanged -= 5 },
			sweepDisagrees, []string{"ranged crit %"}, "ranged crit %"},
	} {
		t.Run(c.name, func(t *testing.T) {
			path := talentPathNamed(t, c.path)
			k := stepNamed(t, path, c.talent)
			simBefore, simAfter := path.sweepSim(k-1), path.sweepSim(k)
			before, after := talentRecordFrom(t, path, k-1, simBefore), talentRecordFrom(t, path, k, simAfter)
			if c.change != nil {
				c.change(&after)
			}
			result := compareSweepStep(before, after, simBefore, simAfter)
			if _, failed := checkNames(result.checks); result.class != c.class || !slices.Equal(failed, c.failed) {
				t.Errorf("%s, failed %v; want %s, %v: %s", result.class, failed, c.class, c.failed,
					strings.Join(failedChecks(result.checks), "; "))
			}
			if c.moved != "" && !slices.ContainsFunc(result.moved, func(m string) bool { return strings.HasPrefix(m, c.moved+" ") }) {
				t.Errorf("moved %v, want %s among them", result.moved, c.moved)
			}
		})
	}
}

// rawTalentLine is the module's info record for a talent record, as `.simval info` writes it with the
// fields the trim doesn't read filled in too.
func rawTalentLine(t *testing.T, rec talentRecord) string {
	t.Helper()
	spellPower := map[string]float64{"physical": 0}
	for i, school := range magicSchools {
		spellPower[school] = rec.SpellPower[i]
	}
	auras := []map[string]any{}
	for _, id := range rec.Auras {
		auras = append(auras, map[string]any{"id": id, "name": "Aura", "stacks": 1, "charges": 0, "durationMs": -1, "fromSelf": true})
	}
	equipment := []map[string]any{}
	for _, item := range rec.Equipment {
		equipment = append(equipment, map[string]any{"slot": item.Slot, "itemId": item.ID})
	}
	attack := func(kind string, ap, speed, expertise float64) map[string]any {
		return map[string]any{"type": kind, "canUse": true, "hasWeapon": false, "unhastedMs": 2000,
			"speedMultiplier": speed, "hastedMs": int(2000 * speed), "damageMin": 1, "damageMax": 2, "attackPower": ap,
			"missChanceTaken": 5, "expertiseReduction": expertise, "weaponSkillVsOther": 400, "critVsOther": 5}
	}
	body := map[string]any{
		"schema": 1, "command": "info", "unixMs": 1790891101445,
		"attacker": map[string]any{
			"name": rec.Bot, "level": 80, "class": rec.Class, "race": rec.Race, "isPlayer": true,
			"health": rec.MaxHealth, "maxHealth": rec.MaxHealth, "powerType": rec.PowerType, "power": 0, "maxPower": rec.MaxPower,
			"stats": map[string]float64{"strength": rec.Strength, "agility": rec.Agility, "stamina": rec.Stamina,
				"intellect": rec.Intellect, "spirit": rec.Spirit},
			"resistances":   rec.Resistances,
			"hitMods":       map[string]float64{"melee": rec.HitMelee, "ranged": rec.HitRanged, "spell": rec.HitSpell},
			"auraExpertise": 0, "castSpeedMultiplier": rec.CastSpeed, "spellPower": spellPower,
			"defense": map[string]any{"dodge": rec.Dodge, "parry": rec.Parry, "block": rec.Block, "blockValue": rec.BlockValue},
			"attacks": []map[string]any{
				attack("mainhand", rec.AttackPower, rec.SpeedMainhand, rec.ExpertiseMainhand),
				attack("offhand", rec.AttackPower, rec.SpeedMainhand, rec.ExpertiseOffhand),
				attack("ranged", rec.RangedAttackPower, rec.SpeedRanged, 0),
			},
			"ratings": []any{},
			"sheet": map[string]any{"critMainhand": rec.CritMainhand, "critOffhand": rec.CritOffhand, "critRanged": rec.CritRanged,
				"spellCrit": append([]float64{0}, rec.SpellCrit...), "expertiseMainhand": 0, "expertiseOffhand": 0,
				"dodge": rec.Dodge, "parry": rec.Parry, "block": rec.Block, "shieldBlock": rec.BlockValue, "realDodge": rec.Dodge},
			"auras": auras,
		},
		"target":    map[string]any{"name": "SimVal Boss Dummy", "entry": 999000, "level": 83, "worldBoss": true, "creatureType": 5},
		"character": map[string]any{"talents": rec.Talents, "glyphs": make([]int32, 6), "equipment": equipment},
	}
	text, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	return string(text)
}

// sweepBotRecords are a bot's records for the path's first steps, as a server agreeing with the sim
// writes them.
func sweepBotRecords(t *testing.T, path talentPath, bot string, steps int) []talentRecord {
	t.Helper()
	var recs []talentRecord
	for n := 0; n <= steps; n++ {
		rec := talentRecordFrom(t, path, n, path.sweepSim(n))
		rec.Bot, rec.Auras = bot, append(rec.Auras, path.Auras...)
		recs = append(recs, rec)
	}
	return recs
}

// The trim keeps the sweep bots' info records from a raw stream or an e2e log and puts each on its path
// and step, whatever else the stream holds.
func TestTalentTrim(t *testing.T) {
	needsItemDB(t)
	paths := allTalentPaths(t)
	combat, arcane := talentPathNamed(t, "RogueCombat"), talentPathNamed(t, "MageArcane")
	firstTry := sweepBotRecords(t, combat, "Svtleadaaaaa", 3)
	unsure := sweepBotRecords(t, combat, "Svtleadbbbbb", 0)
	mage := sweepBotRecords(t, arcane, "Svtleadccccc", 2)
	rerun := sweepBotRecords(t, combat, "Svtleaddddda", 1)
	goldchar := sweepBotRecords(t, arcane, "Svgleadeeeee", 1)

	var raw []string
	for _, rec := range firstTry {
		raw = append(raw, rawTalentLine(t, rec))
	}
	raw = append(raw, `{"schema":1,"command":"melee","attacker":{"name":"Svtleadaaaaa"}}`, "", rawTalentLine(t, unsure[0]))
	for _, rec := range mage {
		raw = append(raw, "    dk_test.go:150: RECORD "+rawTalentLine(t, rec))
	}
	for _, rec := range append(goldchar, rerun...) {
		raw = append(raw, rawTalentLine(t, rec))
	}

	got, notes, err := trimTalentRecords(strings.NewReader(strings.Join(raw, "\r\n")), paths)
	if err != nil {
		t.Fatal(err)
	}
	want := append(slices.Clone(rerun), mage...)
	if len(got) != len(want) {
		t.Fatalf("%d records, want %d: %+v", len(got), len(want), got)
	}
	gotText, err := marshalTalentRecords(got)
	if err != nil {
		t.Fatal(err)
	}
	wantText, err := marshalTalentRecords(want)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(gotText, wantText) {
		t.Errorf("trimmed\n%s\nwant\n%s", gotText, wantText)
	}
	joined := strings.Join(notes, "\n")
	if !strings.Contains(joined, "Svtleadbbbbb left out") || !strings.Contains(joined, "Svtleaddddda's records replace Svtleadaaaaa's") {
		t.Errorf("notes %q", notes)
	}

	// the fixture reads back as written
	file := filepath.Join(t.TempDir(), talentsFixture)
	if err := os.WriteFile(file, gotText, 0o644); err != nil {
		t.Fatal(err)
	}
	reread, err := readJSONLines[talentRecord](file)
	if err != nil {
		t.Fatal(err)
	}
	if text, _ := marshalTalentRecords(reread); !bytes.Equal(text, gotText) {
		t.Errorf("read back\n%s", text)
	}

	wrong := firstTry[2]
	wrong.Talents = []int32{1}
	recs, notes, err := trimTalentRecords(strings.NewReader(rawTalentLine(t, firstTry[1])+"\n"+rawTalentLine(t, wrong)), paths)
	if err != nil || len(recs) > 0 || len(notes) != 1 || !strings.Contains(notes[0], "fit no path") {
		t.Errorf("a talent off the path: %d records, notes %q, %v", len(recs), notes, err)
	}

	// a rerun of some paths keeps the old fixture's other paths, and drops a path the file no longer has
	stale := sweepBotRecords(t, arcane, "Svtleadfffff", 1)
	removed := stale[0]
	removed.Path = "MageArcane9"
	old := append(append(slices.Clone(firstTry), stale...), removed)
	merged, notes := keepOtherPaths(got[:len(rerun)], old, paths)
	if want := append(slices.Clone(got[:len(rerun)]), stale...); len(merged) != len(want) ||
		!slices.EqualFunc(merged, want, func(a, b talentRecord) bool { return a.Bot == b.Bot && a.Step == b.Step }) {
		t.Errorf("merged %+v", merged)
	}
	if joined := strings.Join(notes, "\n"); !strings.Contains(joined, "records of MageArcane (2)\n") ||
		!strings.Contains(joined, "MageArcane9 left out") || strings.Contains(joined, "RogueCombat") {
		t.Errorf("merge notes %q", notes)
	}

	for name, text := range map[string]string{
		"no maxPower":            strings.Replace(rawTalentLine(t, firstTry[1]), `"maxPower":`, `"maxPowerOld":`, 1),
		"a step recorded twice":  rawTalentLine(t, firstTry[1]) + "\n" + rawTalentLine(t, firstTry[1]),
		"a line that isn't JSON": rawTalentLine(t, firstTry[1])[:100],
	} {
		if _, _, err := trimTalentRecords(strings.NewReader(text), paths); err == nil {
			t.Errorf("%s: trimmed", name)
		}
	}
}
