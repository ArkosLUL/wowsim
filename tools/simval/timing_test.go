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

// TestTimingRecords replays a live TestSimvalTiming capture: every timing record against the sim, and
// every bot's info record's regen. Skips without the fixture, like TestDamageRecords.
func TestTimingRecords(t *testing.T) {
	path := filepath.Join("..", "..", fixtureDir, timingFixture)
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Skipf("no capture at %s; record one with mod-sim-validation's TestSimvalTiming", path)
	}
	records, err := readTimingRecords(path)
	if err != nil {
		t.Fatal(err)
	}

	var compared, checked int
	used := map[timingMismatch]bool{}
	for _, rec := range records {
		if rec.Command != "timing" && rec.Command != "info" {
			t.Errorf("%s holds a %q record", path, rec.Command)
			continue
		}
		checks := checkTiming(rec)
		if len(checks) > 0 {
			compared++
		}
		for _, check := range checks {
			checked++
			if name, ok := strings.CutSuffix(check.Name, knownMismatchSuffix); ok {
				used[timingMismatchOf(rec, name)] = true
			}
			if !check.Passed {
				t.Errorf("%s: %s: %s (class %d)", rec.label(), check.Name, check.Detail, rec.Attacker.Class)
			} else {
				t.Logf("%s: %s: %s", rec.label(), check.Name, check.Detail)
			}
		}
	}
	if compared == 0 {
		t.Fatalf("%s has %d records but none to compare", path, len(records))
	}
	for key := range knownTimingMismatches {
		if !used[key] {
			t.Errorf("%+v is a known mismatch, but no record of the capture fails it", key)
		}
	}
	t.Logf("%d checks over %d compared records of %d", checked, compared, len(records))
}

// timingKey names a capture's record by what it probed: the spell (0 for an info), the player's class and
// the pass.
type timingKey struct {
	spell int32
	class uint8
	pass  string
}

// droppedTimingRecords disagree with the sim and were removed from the capture, each with a row in the
// fixture README.
var droppedTimingRecords = []timingKey{}

// timingProbes are TestSimvalTiming's spells per class.
var timingProbes = map[uint8][]int32{
	warrior:     {47465, 47502, 47486, 57823, 47436, 47450},
	paladin:     {53385, 48801, 48819, 20271, 48806, 31884},
	hunter:      {49052, 49050, 49001, 63672, 3045, 49067},
	rogue:       {48676, 48672, 6774, 48638, 48668},
	priest:      {48156, 48127, 48125, 48071, 48300, 48066},
	deathKnight: {49909, 51425, 51411, 48982, 55095, 49895},
	shaman:      {49238, 49271, 49231, 49233, 60043, 49273},
	mage:        {42842, 42833, 42846, 42873, 42897, 42914},
	warlock:     {47809, 47811, 47813, 47838, 59172, 47855},
	druid:       {48465, 48463, 48467, 48461, 48574},
}

func TestTimingFixtureHasEveryProbe(t *testing.T) {
	path := filepath.Join("..", "..", fixtureDir, timingFixture)
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Skipf("no capture at %s", path)
	}
	records, err := readTimingRecords(path)
	if err != nil {
		t.Fatal(err)
	}
	have := map[timingKey]bool{}
	for _, rec := range records {
		pass, err := rec.pass()
		if err != nil {
			t.Fatalf("%s: %v", rec.label(), err)
		}
		have[timingKey{rec.Spell.ID, rec.Attacker.Class, pass}] = true
	}
	for class, spells := range timingProbes {
		for _, pass := range []string{"bare", "talents", "bloodlust"} {
			for _, spell := range append([]int32{0}, spells...) {
				// a bot's two info records are its bare one and its Bloodlust one
				if spell == 0 && pass == "talents" {
					continue
				}
				key := timingKey{spell, class, pass}
				if !have[key] && !slices.Contains(droppedTimingRecords, key) {
					t.Errorf("no record for spell %d class %d, %s", spell, class, pass)
				}
			}
		}
	}
}

// handTiming is a timing record the way mod-sim-validation writes it: a bare level 80 player against
// the boss dummy, its auras and the spell's numbers as given.
func handTiming(t *testing.T, class, race uint8, spellID int32, auras []int32, body string) timingRecord {
	t.Helper()
	auraJSON := make([]string, len(auras))
	for i, id := range auras {
		auraJSON[i] = fmt.Sprintf(`{"id": %d, "durationMs": -1}`, id)
	}
	command := "timing"
	if spellID == 0 {
		command = "info"
	}
	if body != "" {
		body += ","
	}
	text := fmt.Sprintf(`{
		"schema": 1, "command": %q, "unixMs": 1, "gameTimeMs": 1, "map": 0,
		"spell": {"id": %d, "name": "spell %d"},
		"attacker": {"name": "SvTimBot", "level": 80, "isPlayer": true, "class": %d, "race": %d, "auras": [%s]},
		"target": {"name": "Dummy", "entry": 999000, "level": 83, "worldBoss": true, "creatureType": 5,
			"resistances": [10643, 0, 0, 0, 0, 0, 0], "auras": []},
		%s "end": 0
	}`, command, spellID, spellID, class, race, strings.Join(auraJSON, ","), body)
	var rec timingRecord
	if err := json.Unmarshal([]byte(text), &rec); err != nil {
		t.Fatalf("%v in %s", err, text)
	}
	return rec
}

const (
	noHaste   = `"castSpeed": 1, "rangedSpeed": 1`
	lustSpeed = `"castSpeed": 0.7692307692307692, "rangedSpeed": 0.7692307692307692`
)

func TestTimingReplayOnHandCheckedRecords(t *testing.T) {
	rend := func(ticks int, ms int) string {
		return fmt.Sprintf(`
			"cast": {"afterSpellModsMs": 0, "ms": 0, "haste": "none", %s},
			"gcd": {"category": 133, "ms": 1500},
			"cooldown": {"startsOnEvent": false, "ms": 0, "categoryMs": 0},
			"cost": {"powerType": 1, "base": 100, "amount": 100},
			"aura": {"applies": true, "onCaster": false, "baseMs": 15000, "ms": %d,
				"effects": [{"index": 0, "periodic": true, "amplitudeMs": 3000, "ticks": %d}]}`, noHaste, ms, ticks)
	}
	mortalStrike := func(cooldown int) string {
		return fmt.Sprintf(`
			"cast": {"afterSpellModsMs": 0, "ms": 0, "haste": "none", %s},
			"gcd": {"category": 133, "ms": 1500},
			"cooldown": {"startsOnEvent": false, "ms": %d, "categoryMs": 6000},
			"cost": {"powerType": 1, "base": 300, "amount": 300},
			"aura": {"applies": true, "onCaster": false, "baseMs": 10000, "skipped": "the aura is already on the target"}`,
			noHaste, cooldown)
	}
	// Bloodlust: the hasted GCD int(1500 * 0.7692...) = 1153, the cast int(3000 * 0.7692...) = 2307
	frostbolt := func(speed string, cast, gcd int) string {
		return fmt.Sprintf(`
			"cast": {"afterSpellModsMs": 3000, "ms": %d, "haste": "cast", %s},
			"gcd": {"category": 133, "ms": %d, "hasted": true},
			"cooldown": {"startsOnEvent": false, "ms": 0, "categoryMs": 0},
			"cost": {"powerType": 0, "percent": 11, "amount": 0},
			"aura": {"applies": true, "onCaster": false, "baseMs": 9000, "skipped": "no"}`, cast, speed, gcd)
	}
	deathCoil := func(cost int) string {
		return fmt.Sprintf(`
			"cast": {"afterSpellModsMs": 0, "ms": 0, "haste": "none", %s},
			"gcd": {"category": 133, "ms": 1500},
			"cooldown": {"startsOnEvent": false, "ms": 0, "categoryMs": 0},
			"cost": {"powerType": 6, "base": 400, "amount": %d,
				"runes": {"cost": [0, 0, 0], "runicPowerGain": 0, "baseCooldownMs": [10000, 10000, 10000, 10000, 10000, 10000]}},
			"aura": {"applies": false}`, noHaste, cost)
	}

	// the mage's mana cost is a percent of the base mana the sim's character has
	frostboltBare := handTiming(t, mage, 1, 42842, nil, frostbolt(noHaste, 3000, 1500))
	ts, err := buildTimingSim(frostboltBare)
	if err != nil {
		t.Fatal(err)
	}
	frostCost := int(0.11 * ts.char.BaseMana)
	withCost := func(body string) string {
		return strings.Replace(body, `"percent": 11, "amount": 0`, fmt.Sprintf(`"percent": 11, "amount": %d`, frostCost), 1)
	}

	for _, c := range []struct {
		name   string
		rec    timingRecord
		failed []string // check names expected to fail
	}{
		{name: "Rend bare", rec: handTiming(t, warrior, 1, 47465, nil, rend(5, 15000))},
		{
			name:   "Rend with a sixth tick",
			rec:    handTiming(t, warrior, 1, 47465, nil, rend(6, 15000)),
			failed: []string{"e0 ticks"},
		},
		{
			name:   "Rend a second long",
			rec:    handTiming(t, warrior, 1, 47465, nil, rend(5, 16000)),
			failed: []string{"aura duration"},
		},
		// Bloodlust leaves a melee ability's GCD and a physical dot alone
		{name: "Rend under Bloodlust", rec: handTiming(t, warrior, 1, 47465, []int32{2825},
			strings.Replace(rend(5, 15000), noHaste, lustSpeed, 1))},
		{name: "Mortal Strike bare", rec: handTiming(t, warrior, 1, 47486, nil, mortalStrike(6000))},
		{
			name:   "Mortal Strike with a 7 s cooldown",
			rec:    handTiming(t, warrior, 1, 47486, nil, mortalStrike(7000)),
			failed: []string{"cooldown"},
		},
		{name: "Frostbolt bare", rec: handTiming(t, mage, 1, 42842, nil, withCost(frostbolt(noHaste, 3000, 1500)))},
		{
			name:   "Frostbolt a millisecond long",
			rec:    handTiming(t, mage, 1, 42842, nil, withCost(frostbolt(noHaste, 3001, 1500))),
			failed: []string{"cast time"},
		},
		{name: "Frostbolt under Bloodlust", rec: handTiming(t, mage, 1, 42842, []int32{2825},
			withCost(frostbolt(lustSpeed, 2307, 1153)))},
		{
			name: "Frostbolt under Bloodlust, the haste missing",
			rec: handTiming(t, mage, 1, 42842, []int32{2825},
				withCost(frostbolt(lustSpeed, 3000, 1500))),
			failed: []string{"cast time", "gcd"},
		},
		{name: "Death Coil bare", rec: handTiming(t, deathKnight, 1, 49895, nil, deathCoil(400))},
		{
			name:   "Death Coil at 30 runic power",
			rec:    handTiming(t, deathKnight, 1, 49895, nil, deathCoil(300)),
			failed: []string{"runic power cost"},
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			checks := checkTiming(c.rec)
			if len(checks) == 0 {
				t.Fatal("no checks")
			}
			var failedNames []string
			for _, check := range checks {
				if !check.Passed {
					failedNames = append(failedNames, check.Name)
				}
			}
			if !slices.Equal(failedNames, c.failed) {
				t.Errorf("failed %v, want %v: %s", failedNames, c.failed, strings.Join(failedChecks(checks), "; "))
			}
		})
	}
}

func TestTimingRegenOnHandCheckedRecords(t *testing.T) {
	regen := func(flat, interrupted, rune string) string {
		return fmt.Sprintf(`"regen": {"flat": [%s], "interrupted": [%s], "rune": [%s], "manaRate": 1, "energyRate": 1}`,
			flat, interrupted, rune)
	}
	info := func(t *testing.T, class, race uint8, powerType int, auras []int32, regen string) timingRecord {
		rec := handTiming(t, class, race, 0, auras, "")
		rec.Attacker.PowerType = uint32(powerType)
		if err := json.Unmarshal([]byte("{"+regen+"}"), &rec.Attacker); err != nil {
			t.Fatal(err)
		}
		return rec
	}

	t.Run("shaman mana", func(t *testing.T) {
		probe := info(t, shaman, 2, 0, nil, regen("0,0,0,0,0,0,0", "0,0,0,0,0,0,0", "0,0,0,0"))
		ts, err := buildTimingSim(probe)
		if err != nil {
			t.Fatal(err)
		}
		ts.start()
		flat, interrupted := ts.char.ManaRegenPerSecondWhileNotCasting(), ts.char.ManaRegenPerSecondWhileCasting()
		zeros := "0,0,0,0,0,0"
		good := info(t, shaman, 2, 0, nil, regen(fmt.Sprintf("%v,%s", flat, zeros), fmt.Sprintf("%v,%s", interrupted, zeros), "0,0,0,0"))
		if failed := failedChecks(checkTiming(good)); len(failed) != 0 {
			t.Errorf("failed %v", failed)
		}
		high := info(t, shaman, 2, 0, nil, regen(fmt.Sprintf("%v,%s", flat*1.01, zeros), fmt.Sprintf("%v,%s", interrupted, zeros), "0,0,0,0"))
		if failed := failedChecks(checkTiming(high)); len(failed) != 1 || !strings.HasPrefix(failed[0], "mana regen:") {
			t.Errorf("1%% more mana regen: failed %v", failed)
		}
	})

	t.Run("rogue energy", func(t *testing.T) {
		good := info(t, rogue, 1, 3, nil, regen("0,0,0,0,0,0,0", "0,0,0,0,0,0,0", "0,0,0,0"))
		if failed := failedChecks(checkTiming(good)); len(failed) != 0 {
			t.Errorf("failed %v", failed)
		}
		fast := info(t, rogue, 1, 3, nil, regen("0,0,0,2,0,0,0", "0,0,0,0,0,0,0", "0,0,0,0"))
		if failed := failedChecks(checkTiming(fast)); len(failed) != 1 || !strings.HasPrefix(failed[0], "energy regen:") {
			t.Errorf("energy flat 2: failed %v", failed)
		}
	})

	t.Run("death knight runes", func(t *testing.T) {
		good := info(t, deathKnight, 1, 6, nil, regen("0,0,0,0,0,0,0", "0,0,0,0,0,0,0", "0.1,0.1,0.1,0.1"))
		if failed := failedChecks(checkTiming(good)); len(failed) != 0 {
			t.Errorf("failed %v", failed)
		}
		slow := info(t, deathKnight, 1, 6, nil, regen("0,0,0,0,0,0,0", "0,0,0,0,0,0,0", "0.1,0.09,0.1,0.1"))
		if failed := failedChecks(checkTiming(slow)); len(failed) != 1 || !strings.HasPrefix(failed[0], "rune 1 cooldown:") {
			t.Errorf("a slower frost rune: failed %v", failed)
		}
	})

	t.Run("a record without regen gives no checks", func(t *testing.T) {
		rec := handTiming(t, mage, 1, 0, nil, "")
		if checks := checkTiming(rec); len(checks) != 0 {
			t.Errorf("%d checks", len(checks))
		}
	})
}

func TestTimingPass(t *testing.T) {
	for _, c := range []struct {
		auras []int32
		want  string
	}{
		{nil, "bare"},
		{[]int32{12664}, "talents"},
		{[]int32{12664, 2825}, "bloodlust"},
	} {
		rec := handTiming(t, warrior, 1, 47450, c.auras, "")
		if got, err := rec.pass(); err != nil || got != c.want {
			t.Errorf("auras %v: pass %q (%v), want %q", c.auras, got, err, c.want)
		}
	}
}

// A full simval.jsonl recapture also picks up TestSimvalTiming's records. They have to give the
// replay's checks no work and key like any other record.
func TestTimingRecordInTheReplayFixture(t *testing.T) {
	line, err := json.Marshal(map[string]any{
		"schema": 1, "command": "timing", "spell": map[string]any{"id": 47465, "name": "Rend"},
		"attacker": map[string]any{"name": "Bot", "isPlayer": true, "class": 1, "regen": map[string]any{"flat": []float64{0}}},
		"target":   map[string]any{"name": "Dummy", "entry": 999000},
		"cast":     map[string]any{"ms": 0}, "cooldown": map[string]any{"ms": 0},
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
		t.Errorf("the replay checked a timing record: %+v", checks)
	}
	if got, want := records[0].key(), (recordKey{"timing", 47465, warrior, 999000}); got != want {
		t.Errorf("key %v, want %v", got, want)
	}
}
