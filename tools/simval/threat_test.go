package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// TestThreatRecords replays a live TestSimvalThreat capture against the sim. Skips without the fixture,
// like TestTimingRecords.
func TestThreatRecords(t *testing.T) {
	path := filepath.Join("..", "..", fixtureDir, threatFixture)
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Skipf("no capture at %s; record one with mod-sim-validation's TestSimvalThreat", path)
	}
	records, err := readThreatRecords(path)
	if err != nil {
		t.Fatal(err)
	}

	var checked int
	used := map[threatMismatch]bool{}
	for _, rec := range records {
		if rec.Command != "threat" {
			t.Errorf("%s holds a %q record", path, rec.Command)
			continue
		}
		for _, check := range checkThreat(rec) {
			checked++
			if name, ok := strings.CutSuffix(check.Name, knownMismatchSuffix); ok {
				used[threatMismatchOf(rec, name)] = true
			}
			if !check.Passed {
				t.Errorf("%s: %s: %s (%+v)", rec.label(), check.Name, check.Detail, rec.key())
			} else {
				t.Logf("%s: %s: %s", rec.label(), check.Name, check.Detail)
			}
		}
	}
	if len(records) == 0 {
		t.Fatalf("%s has no records", path)
	}
	for key := range knownThreatMismatches {
		if !used[key] {
			t.Errorf("%+v is a known mismatch, but no record of the capture fails it", key)
		}
	}
	t.Logf("%d checks over %d records", checked, len(records))
}

// threatKey names a capture's record by what it probed: the player's class, its stance, form, presence or
// Righteous Fury, the spell and the target count the probe was given.
type threatKey struct {
	class    uint8
	form     int32
	spell    int32
	override int32
}

func (r threatRecord) key() threatKey {
	return threatKey{r.Attacker.Class, r.form(), r.Spell.ID, r.override()}
}

// threatDrop is why a probe has no record in the capture, with a row in the fixture README: the replay
// can't build it, or it can now (recapture) but the capture lost the record, which the next
// TestSimvalThreat run brings back.
type threatDrop struct {
	reason    string
	recapture bool
}

var droppedThreatRecords = map[threatKey]threatDrop{
	{deathKnight, 48263, 45524, 0}: {reason: "the sim has no Chains of Ice"},
	{deathKnight, 48263, 49576, 0}: {reason: "the sim has no Death Grip"},
	{paladin, 25780, 25742, 0}:     {"dropped while the sim character had no weapon", true},
	{warlock, 0, 59092, 0}:         {"dropped while the sim character had no pet", true},
	{shaman, 0, 60103, 0}:          {"dropped while the sim character had no off hand weapon", true},
}

// threatBot is one TestSimvalThreat character: its class, race, the auras its casts leave on it (talents,
// stance or form and its passives) and its probes.
type threatBot struct {
	name        string
	class, race uint8
	auras       []int32
	probes      []threatProbe
}

// threatProbe is a probed spell and the target count the probe was given, 0 for the cast's own.
type threatProbe struct {
	spell, override int32
}

func threatProbesOf(spells ...int32) []threatProbe {
	out := make([]threatProbe, len(spells))
	for i, s := range spells {
		out[i] = threatProbe{spell: s}
	}
	return out
}

// threatBots are TestSimvalThreat's characters and probes.
var threatBots = []threatBot{
	{"ProtWarrior", warrior, 1, []int32{71, 7376, 57339}, slices.Concat(threatProbesOf(47450, 47520), []threatProbe{{47520, 2}},
		threatProbesOf(57823, 47488, 47475, 50783, 47467, 47498, 47437, 47502, 12809, 57755, 47471))},
	{"FuryWarrior", warrior, 1, []int32{29763, 2458, 7381},
		threatProbesOf(47450, 47520, 47475, 50783, 23881, 1680, 47471, 47486, 7384, 47465)},
	{"ProtPaladin", paladin, 1, []int32{25780, 57340}, threatProbesOf(48827, 53595, 61411, 20187, 48801, 48817, 48819, 25742, 35395)},
	{"RetPaladin", paladin, 1, []int32{31881}, threatProbesOf(35395, 53385, 48801, 20187, 48806, 48819, 20424)},
	{"BearDruid", druid, 4, []int32{9634, 9635, 21178, 57339}, slices.Concat(threatProbesOf(48480, 48568, 48564, 48562),
		[]threatProbe{{48562, 3}}, threatProbesOf(48560, 60089, 16857))},
	{"CatDruid", druid, 4, []int32{768, 3025}, threatProbesOf(48572, 48566, 48574, 49800, 48577, 62078, 16857)},
	{"FrostDeathKnight", deathKnight, 1, []int32{48263, 57340, 61261},
		threatProbesOf(56815, 52212, 49909, 55262, 49924, 49941, 55271, 45524, 49576, 49895, 47632)},
	{"UnholyDeathKnight", deathKnight, 1, []int32{49491, 48265}, threatProbesOf(55271, 49930, 51425, 49895, 47632, 49909, 55262, 52212)},
	{"Rogue", rogue, 1, nil, threatProbesOf(48638, 48668, 48657, 48672, 57993, 51723, 57965, 48660)},
	{"Mage", mage, 1, []int32{12592, 12351, 12519}, threatProbesOf(42897, 44781, 42833, 42926, 42842, 47610, 42914, 55360, 42859)},
	{"ShadowPriest", priest, 1, []int32{15320, 15473, 49868}, threatProbesOf(48127, 48156, 58381, 48125, 48160, 48300, 48158, 48123)},
	{"Warlock", warlock, 1, []int32{18372, 17918},
		threatProbesOf(47809, 47838, 47811, 47813, 59172, 47815, 47864, 47865, 47867, 50511, 59092)},
	{"ElementalShaman", shaman, 11, []int32{30674}, threatProbesOf(49238, 49271, 60043, 49231, 49233, 49236, 60103, 61657, 61654)},
}

func threatFormOf(auras []int32) int32 {
	for _, id := range auras {
		if _, ok := threatForms[id]; ok {
			return id
		}
	}
	return 0
}

func TestThreatFixtureHasEveryProbe(t *testing.T) {
	path := filepath.Join("..", "..", fixtureDir, threatFixture)
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Skipf("no capture at %s", path)
	}
	records, err := readThreatRecords(path)
	if err != nil {
		t.Fatal(err)
	}
	have := map[threatKey]bool{}
	for _, rec := range records {
		if have[rec.key()] {
			t.Errorf("%+v is in the capture twice; keep the probe's last record", rec.key())
		}
		have[rec.key()] = true
	}
	for _, bot := range threatBots {
		for _, p := range bot.probes {
			key := threatKey{bot.class, threatFormOf(bot.auras), p.spell, p.override}
			_, dropped := droppedThreatRecords[key]
			switch {
			case !have[key] && !dropped:
				t.Errorf("%s: no record for %+v", bot.name, key)
			case have[key] && dropped:
				t.Errorf("%s: %+v is dropped, but the capture holds it", bot.name, key)
			}
		}
	}
}

// handThreat is a threat record the way mod-sim-validation writes it: a level 80 player with those auras
// and main hand attack power against the boss dummy, and the record's numbers as given.
func handThreat(t *testing.T, class, race uint8, spellID int32, auras []int32, ap float64, body string) threatRecord {
	t.Helper()
	auraJSON := make([]string, len(auras))
	for i, id := range auras {
		auraJSON[i] = fmt.Sprintf(`{"id": %d, "durationMs": -1}`, id)
	}
	if body != "" {
		body += ","
	}
	text := fmt.Sprintf(`{
		"schema": 1, "command": "threat", "unixMs": 1, "gameTimeMs": 1, "map": 533,
		"spell": {"id": %d, "name": "spell %d", "schoolMask": 1, "dmgClass": 2, "family": 4, "spellLevel": 80,
			"noThreat": false, "suppressTargetProcs": false, "noInitialThreat": false},
		"attacker": {"name": "SvThrBot", "level": 80, "isPlayer": true, "class": %d, "race": %d, "auras": [%s],
			"attacks": [{"type": "mainhand", "attackPower": %v}]},
		"target": {"name": "Dummy", "entry": 999000, "level": 83, "worldBoss": true, "creatureType": 5,
			"resistances": [10643, 0, 0, 0, 0, 0, 0], "auras": []},
		"modifiers": {"schools": [1, 1, 1, 1, 1, 1, 1], "auras": [], "spellMods": [], "tempModifier": 0},
		%s "live": {"canHaveThreatList": true, "redirects": false, "gm": false, "landed": true, "engaged": false,
			"damage": 0, "bonus": 0}
	}`, spellID, spellID, class, race, strings.Join(auraJSON, ","), ap, body)
	var rec threatRecord
	if err := json.Unmarshal([]byte(text), &rec); err != nil {
		t.Fatalf("%v in %s", err, text)
	}
	return rec
}

// threatBody is a record's threat for 10000 damage at that multiplier, and its bonus after a cast at the
// dummy that hit `count` targets. bonus holds the bonus' fields past `selected`.
func threatBody(multiplier float64, count int, bonus string) string {
	return fmt.Sprintf(`"damage": {"amount": 10000, "pctMod": 1, "afterPctMod": 10000, "afterSpellMods": 10000,
			"schoolMultiplier": %v, "threat": %v},
		"bonus": {"selected": {"count": %d, "dummy": true, "caster": false}, %s}`, multiplier, 10000*multiplier, count, bonus)
}

// threatLandedBonus is a negative spell's bonus landing unmodified on the dummy.
func threatLandedBonus(source string, base float64, targets int, threat float64) string {
	return fmt.Sprintf(`"source": %q, "attackPower": 3000, "base": %v, "targets": %d, "perTarget": %v,
		"assist": false, "reachesDummy": true, "threat": %v`, source, base, targets, base/float64(targets), threat)
}

// threatAPBonus is a spell_threat row with nothing but an attack power part, read at that attack power.
func threatAPBonus(ap, threat float64) string {
	return fmt.Sprintf(`"source": "entry", "attackPower": %v, "base": %v, "targets": 1, "perTarget": %v,
		"assist": false, "reachesDummy": true, "threat": %v`, ap, threat, threat, threat)
}

// Every probe but the dropped ones builds its sim character and casts, whatever the server's numbers.
func TestThreatProbesReplay(t *testing.T) {
	for _, bot := range threatBots {
		for _, p := range bot.probes {
			key := threatKey{bot.class, threatFormOf(bot.auras), p.spell, p.override}
			targets := max(1, int(p.override))
			rec := handThreat(t, bot.class, bot.race, p.spell, bot.auras, 3000,
				threatBody(1, 1, threatLandedBonus("spellLevel", 80, targets, 80)))
			checks := checkThreat(rec)
			var broken []string
			for _, c := range checks {
				if c.Name == "attacker" || c.Name == "sim character" || c.Name == "sim cast" {
					broken = append(broken, c.Name+": "+c.Detail)
				}
			}
			drop, dropped := droppedThreatRecords[key]
			switch {
			case dropped && !drop.recapture && len(broken) == 0:
				t.Errorf("%s %+v replays, so it needn't be dropped", bot.name, key)
			case (!dropped || drop.recapture) && len(broken) > 0:
				t.Errorf("%s %+v: %v", bot.name, key, broken)
			case !dropped && len(checks) == 0:
				t.Errorf("%s %+v: no checks", bot.name, key)
			}
		}
	}
}

func TestThreatReplayOnHandCheckedRecords(t *testing.T) {
	var (
		protWarrior = []int32{71, 7376, 57339}
		furyWarrior = []int32{29763, 2458, 7381}
		bear        = []int32{9634, 9635, 21178, 57339}
		frostDK     = []int32{48263, 57340, 61261}
		warlockTal  = []int32{18372, 17918}
	)
	// Defensive Stance, Dire Bear Form and Frost Presence, each +45% with the tank passive's +43%: damage
	// threat only, never the bonus
	const def = 2.0735

	for _, c := range []struct {
		name   string
		rec    threatRecord
		checks []string // every check the record gets
		failed []string
	}{
		{
			name: "Heroic Strike, the bonus unmodified",
			rec: handThreat(t, warrior, 1, 47450, protWarrior, 3000,
				threatBody(def, 1, threatLandedBonus("entry", 259, 1, 259))),
			checks: []string{"damage threat", "bonus"},
		},
		{
			name: "Heroic Strike, the bonus times the stance",
			rec: handThreat(t, warrior, 1, 47450, protWarrior, 3000,
				threatBody(def, 1, threatLandedBonus("entry", 259, 1, 259*def))),
			checks: []string{"damage threat", "bonus"},
			failed: []string{"bonus"},
		},
		{
			// the sim's Improved Berserker Stance takes 0.1 off the stance's 0.8
			name: "Heroic Strike in Berserker Stance, the talent multiplying",
			rec: handThreat(t, warrior, 1, 47450, furyWarrior, 3000,
				threatBody(0.72, 1, threatLandedBonus("entry", 259, 1, 259))),
			checks: []string{"damage threat", "bonus"},
			failed: []string{"damage threat"},
		},
		{
			name: "Cleave on two targets, split between them",
			rec: handThreat(t, warrior, 1, 47520, protWarrior, 3000,
				threatBody(def, 1, threatLandedBonus("entry", 112, 2, 56))),
			checks: []string{"damage threat", "bonus"},
		},
		{
			name: "Cleave on two targets, the whole bonus on each",
			rec: handThreat(t, warrior, 1, 47520, protWarrior, 3000,
				threatBody(def, 1, threatLandedBonus("entry", 112, 2, 112))),
			checks: []string{"damage threat", "bonus"},
			failed: []string{"bonus"},
		},
		{
			// the bonus goes on the cast's hit only, never its ticks
			name: "Lacerate, the row's percent and bonus",
			rec: handThreat(t, druid, 4, 48568, bear, 3000,
				threatBody(0.5*def, 1, threatLandedBonus("entry", 515, 1, 515))),
			checks: []string{"damage threat", "bonus", "tick bonus"},
		},
		{
			name: "Lacerate without the row's percent",
			rec: handThreat(t, druid, 4, 48568, bear, 3000,
				threatBody(def, 1, threatLandedBonus("entry", 515, 1, 515))),
			checks: []string{"damage threat", "bonus", "tick bonus"},
			failed: []string{"damage threat"},
		},
		{
			// the attack power part, read at the record's attack power
			name: "Devastate at 3000 attack power",
			rec: handThreat(t, warrior, 1, 47498, protWarrior, 3000,
				threatBody(def, 1, threatAPBonus(3000, 0.05*3000))),
			checks: []string{"damage threat", "bonus"},
		},
		{
			name: "Devastate at 4000 attack power",
			rec: handThreat(t, warrior, 1, 47498, protWarrior, 4000,
				threatBody(def, 1, threatAPBonus(4000, 0.05*3000))),
			checks: []string{"damage threat", "bonus"},
			failed: []string{"bonus"},
		},
		{
			// no spell_threat row: the spell's level
			name: "Sunder Armor",
			rec: handThreat(t, warrior, 1, 47467, protWarrior, 3000,
				threatBody(def, 1, threatLandedBonus("spellLevel", 77, 1, 77))),
			checks: []string{"damage threat (not compared)", "bonus"},
		},
		{
			name: "Curse of Doom",
			rec: handThreat(t, warlock, 1, 47867, warlockTal, 0,
				threatBody(0.8, 1, threatLandedBonus("spellLevel", 80, 1, 80))),
			checks: []string{"damage threat", "bonus", "tick bonus"},
		},
		{
			name: "Death and Decay's damage, a tick of the sim's dot",
			rec: handThreat(t, deathKnight, 1, 52212, frostDK, 3000,
				threatBody(1.9*def, 1, threatLandedBonus("entry", 0, 1, 0))),
			checks: []string{"damage threat", "tick bonus"},
		},
		{
			name: "Death and Decay's damage with a bonus each tick",
			rec: handThreat(t, deathKnight, 1, 52212, frostDK, 3000,
				threatBody(1.9*def, 1, threatLandedBonus("spellLevel", 80, 1, 80))),
			checks: []string{"damage threat", "tick bonus"},
			failed: []string{"tick bonus"},
		},
		{
			// the cast's dummy effect deals nothing; 47632 is the damage
			name: "Death Coil's cast",
			rec: handThreat(t, deathKnight, 1, 49895, frostDK, 3000,
				`"damage": {"amount": 10000, "pctMod": 1, "afterPctMod": 10000, "afterSpellMods": 10000,
					"schoolMultiplier": 2.0735, "threat": 0},
				"bonus": {"selected": {"count": 1, "dummy": true, "caster": false},
					"skipped": "SPELL_ATTR1_NO_THREAT", "threat": 0}`),
			checks: []string{"damage threat (not compared)", "bonus"},
		},
		{
			// tag 0 only queues the strike, tag 1 hits
			name: "Rune Strike",
			rec: handThreat(t, deathKnight, 1, 56815, frostDK, 3000,
				threatBody(1.75*def, 1, threatLandedBonus("entry", 0, 1, 0))),
			checks: []string{"damage threat", "bonus"},
		},
		{
			name:   "Chains of Ice",
			rec:    handThreat(t, deathKnight, 1, 45524, frostDK, 3000, threatBody(def, 1, threatLandedBonus("entry", 240, 1, 240))),
			checks: []string{"sim character"},
			failed: []string{"sim character"},
		},
		{
			// the replay gives the paladin a weapon; Righteous Fury's holy +80% on the tank passive
			name: "Seal of Righteousness, the bonus of its damage spell",
			rec: handThreat(t, paladin, 1, 25742, []int32{25780, 57340}, 3000,
				threatBody(1.8*1.43, 1, threatLandedBonus("spellLevel", 1, 1, 1))),
			checks: []string{"damage threat", "bonus"},
		},
		{
			// the replay gives the shaman two weapons; Elemental Precision takes 30% off fire
			name: "Lava Lash",
			rec: handThreat(t, shaman, 11, 60103, []int32{30674}, 3000,
				threatBody(0.7, 1, threatLandedBonus("spellLevel", 41, 1, 41))),
			checks: []string{"damage threat", "bonus"},
		},
		{
			// the replay gives the warlock a pet to drain; no damage and no bonus
			name:   "Dark Pact",
			rec:    handThreat(t, warlock, 1, 59092, warlockTal, 0, threatBody(1, 1, threatLandedBonus("none", 0, 1, 0))),
			checks: []string{"damage threat (not compared)", "bonus"},
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			// the comparison alone, without knownThreatMismatches
			checks := replayThreatChecks(c.rec, buildThreatSim)
			var names, failedNames []string
			for _, check := range checks {
				names = append(names, check.Name)
				if !check.Passed {
					failedNames = append(failedNames, check.Name)
				}
			}
			if !slices.Equal(names, c.checks) {
				t.Errorf("checks %v, want %v", names, c.checks)
			}
			if !slices.Equal(failedNames, c.failed) {
				t.Errorf("failed %v, want %v: %s", failedNames, c.failed, strings.Join(failedChecks(checks), "; "))
			}
		})
	}
}

func TestThreatKnownMismatchPasses(t *testing.T) {
	rec := handThreat(t, warrior, 1, 47450, []int32{71}, 3000, threatBody(2.0735, 1, threatLandedBonus("entry", 259, 1, 259*2.0735)))
	key := threatMismatch{warrior, 71, 47450, 0, "bonus"}
	old, had := knownThreatMismatches[key]
	knownThreatMismatches[key] = "the server's bonus is modified"
	defer func() {
		if had {
			knownThreatMismatches[key] = old
		} else {
			delete(knownThreatMismatches, key)
		}
	}()

	checks := checkThreat(rec)
	if failed := failedChecks(checks); len(failed) != 0 {
		t.Fatalf("failed %v", failed)
	}
	if !slices.ContainsFunc(checks, func(c Check) bool { return c.Name == "bonus"+knownMismatchSuffix }) {
		t.Errorf("no known mismatch among %+v", checks)
	}
}

func TestThreatRecordTargets(t *testing.T) {
	for _, c := range []struct {
		name              string
		count, targets    int32
		caster            bool
		override, enemies int32
	}{
		{name: "the dummy alone", count: 1, targets: 1, enemies: 1},
		{name: "two given", count: 1, targets: 2, override: 2, enemies: 2},
		{name: "the dummy and the caster", count: 2, targets: 2, caster: true, enemies: 1},
		{name: "three given, the caster hit too", count: 2, targets: 3, caster: true, override: 3, enemies: 3},
		{name: "skipped", count: 1, enemies: 1},
	} {
		var rec threatRecord
		rec.Bonus.Selected.Count, rec.Bonus.Selected.Caster, rec.Bonus.Targets = c.count, c.caster, c.targets
		if got := rec.override(); got != c.override {
			t.Errorf("%s: override %d, want %d", c.name, got, c.override)
		}
		if got := rec.enemies(); got != int(c.enemies) {
			t.Errorf("%s: %d enemies, want %d", c.name, got, c.enemies)
		}
	}
}

// A simval.jsonl recapture that picks up TestSimvalThreat's records has to give them no checks.
func TestThreatRecordInTheReplayFixture(t *testing.T) {
	line, err := json.Marshal(map[string]any{
		"schema": 1, "command": "threat", "spell": map[string]any{"id": 47450, "name": "Heroic Strike"},
		"attacker": map[string]any{"name": "Bot", "isPlayer": true, "class": 1},
		"target":   map[string]any{"name": "Dummy", "entry": 999000},
		"damage":   map[string]any{"amount": 10000, "threat": 20735},
		"bonus":    map[string]any{"selected": map[string]any{"count": 1}, "threat": 259},
	})
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "simval.jsonl")
	if err := os.WriteFile(path, append(line, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
	records, err := readRecords(path)
	if err != nil {
		t.Fatal(err)
	}
	if checks := CheckRecord(records[0]); len(checks) != 0 {
		t.Errorf("the replay checked a threat record: %+v", checks)
	}
	if got, want := records[0].key(), (recordKey{"threat", 47450, warrior, 999000}); got != want {
		t.Errorf("key %v, want %v", got, want)
	}
}
