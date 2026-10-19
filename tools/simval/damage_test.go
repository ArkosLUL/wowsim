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

	"github.com/wowsims/wotlk/sim/core/proto"
	"github.com/wowsims/wotlk/sim/core/serverdata"
	"github.com/wowsims/wotlk/sim/core/stats"
)

// TestDamageRecords replays a live TestSimvalDamage capture: every unshaped record's probed effects
// against the sim. Skips without the fixture, like TestAgainstServerRecords.
func TestDamageRecords(t *testing.T) {
	path := filepath.Join("..", "..", fixtureDir, damageFixture)
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Skipf("no capture at %s; record one with mod-sim-validation's TestSimvalDamage", path)
	}
	records, err := readDamageRecords(path)
	if err != nil {
		t.Fatal(err)
	}

	var compared, checked int
	for _, rec := range records {
		if rec.Command != "damage" {
			t.Errorf("%s holds a %q record", path, rec.Command)
			continue
		}
		checks := checkDamage(rec)
		if len(checks) > 0 {
			compared++
		}
		for _, check := range checks {
			checked++
			if !check.Passed {
				t.Errorf("%s: %s: %s", rec.label(), check.Name, check.Detail)
			} else {
				t.Logf("%s: %s: %s", rec.label(), check.Name, check.Detail)
			}
		}
	}
	if compared == 0 {
		t.Fatalf("%s has %d records but none to compare", path, len(records))
	}
	t.Logf("%d checks over %d compared records of %d", checked, compared, len(records))
}

// damageKey names a capture's record by what it probed: the spell, the player's class, and whether the
// character carried the talents and glyphs.
type damageKey struct {
	spell  int32
	class  uint8
	modded bool
}

func (k damageKey) String() string {
	pass := "bare"
	if k.modded {
		pass = "with talents"
	}
	return fmt.Sprintf("damage spell %d class %d, %s", k.spell, k.class, pass)
}

// shapedDamageProbes are the spells TestSimvalDamage expects back shaped, so never compared.
var shapedDamageProbes = map[uint8][]int32{
	warrior: {23881}, hunter: {49052}, rogue: {57970}, shaman: {49233}, mage: {42914}, warlock: {47855},
}

// droppedDamageRecords disagree with the sim and were removed from the capture, each with a row in the
// fixture README.
var droppedDamageRecords = []damageKey{}

func TestDamageFixtureHasEveryProbe(t *testing.T) {
	path := filepath.Join("..", "..", fixtureDir, damageFixture)
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Skipf("no capture at %s", path)
	}
	records, err := readDamageRecords(path)
	if err != nil {
		t.Fatal(err)
	}
	have := map[damageKey]bool{}
	for _, rec := range records {
		setup, err := damageSetupFor(rec)
		if err != nil {
			t.Fatalf("%s: %v", rec.label(), err)
		}
		have[damageKey{rec.Spell.ID, rec.Attacker.Class, setup.modded}] = true
	}
	for _, c := range damageProbes {
		for _, spell := range append(slices.Clone(c.spells), shapedDamageProbes[c.class]...) {
			for _, modded := range []bool{false, true} {
				key := damageKey{spell, c.class, modded}
				if !have[key] && !slices.Contains(droppedDamageRecords, key) {
					t.Errorf("no record for %s", key)
				}
			}
		}
	}
}

// damageProbes are TestSimvalDamage's compared spells per class, with the race it plays them on.
var damageProbes = []struct {
	class, race uint8
	spells      []int32
}{
	{warrior, 1, []int32{47502, 57823}},
	{paladin, 1, []int32{48801, 48806, 48819, 48827}},
	{hunter, 4, []int32{49045, 49001, 63672}},
	{rogue, 1, []int32{57965, 57975, 48676}},
	{priest, 5, []int32{48127, 48125, 48300, 48071}},
	{deathKnight, 1, []int32{49909, 47632, 55078, 55095}},
	{shaman, 11, []int32{49238, 49231, 60043}},
	{mage, 1, []int32{42842, 42833, 42873}},
	{warlock, 1, []int32{47809, 47813, 47811}},
	{druid, 4, []int32{48463, 48461, 48465, 48574}},
}

func damageRec(t *testing.T, text string) damageRecord {
	t.Helper()
	var rec damageRecord
	if err := json.Unmarshal([]byte(text), &rec); err != nil {
		t.Fatal(err)
	}
	return rec
}

// handRecord is a record the way the module writes one, for a level 80 character with 520 spell power
// and healing, 1000 melee and 900 ranged attack power, at the level 83 boss dummy (a giant).
func handRecord(t *testing.T, class, race uint8, spellID int32, auras []int32, bonus, effects string) damageRecord {
	t.Helper()
	var auraJSON []string
	for _, id := range append([]int32{54192, 54161, 47436}, auras...) {
		auraJSON = append(auraJSON, fmt.Sprintf(`{"id": %d, "durationMs": -1}`, id))
	}
	if bonus != "" {
		bonus = `"bonus": ` + bonus + ","
	}
	return damageRec(t, fmt.Sprintf(`{
		"schema": 1, "command": "damage",
		"spell": {"id": %d, "name": "spell %d", "schoolMask": 16, "dmgClass": 1, "familyFlags": [0, 0, 0]},
		"shaped": "",
		"attacker": {"name": "Bot", "level": 80, "isPlayer": true, "class": %d, "race": %d,
			"spellPower": {"holy": 520, "fire": 520},
			"attacks": [{"type": "mainhand", "attackPower": 1000}, {"type": "ranged", "attackPower": 900}],
			"auras": [%s]},
		"target": {"name": "Dummy", "entry": 999000, "level": 83, "worldBoss": true, "creatureType": 5,
			"health": 1000000, "maxHealth": 1000000, "resistances": [10643, 0, 0, 0, 0, 0, 0], "auras": []},
		"targetHealthPct": 100, "comboPoints": 0, "levelPenalty": 1,
		"spellPower": {"damage": 520, "healing": 520},
		"attackPower": {"type": "mainhand", "value": 1000},
		%s
		"effects": [%s]
	}`, spellID, spellID, class, race, strings.Join(auraJSON, ","), bonus, effects))
}

func failedChecks(checks []Check) []string {
	var failed []string
	for _, check := range checks {
		if !check.Passed {
			failed = append(failed, check.Name+": "+check.Detail)
		}
	}
	return failed
}

// The server numbers below follow SpellDamageBonusDone by hand: each power bonus truncates to a whole
// number, the multipliers go on their sum with the base, and the result truncates.
func TestDamageReplayOnHandCheckedRecords(t *testing.T) {
	for _, c := range []struct {
		name   string
		rec    damageRecord
		failed []string // check names expected to fail
	}{
		{
			// 803 + int(0.857 * 520 = 445.64) = 1248, and 865 + 445 at the top
			name: "Frostbolt bare",
			rec: handRecord(t, mage, 1, 42842, nil, "", `
				{"index": 0, "kind": "other"},
				{"index": 1, "kind": "damage", "coefficient": 0.857, "pctDone": 1, "base": {"min": 803, "max": 865},
					"taken": {"min": 1248, "max": 1310}, "crit": {"amount": 10000, "result": 15000}}`),
		},
		{
			name: "Frostbolt 2% high at the top",
			rec: handRecord(t, mage, 1, 42842, nil, "", `
				{"index": 1, "kind": "damage", "coefficient": 0.857, "pctDone": 1, "base": {"min": 803, "max": 865},
					"taken": {"min": 1248, "max": 1336}, "crit": {"amount": 10000, "result": 15000}}`),
			failed: []string{"e1 damage max"},
		},
		{
			// the sim truncates where the server does, so it gets 0.1% or 1 point either way
			name: "Frostbolt under the server at the bottom",
			rec: handRecord(t, mage, 1, 42842, nil, "", `
				{"index": 1, "kind": "damage", "coefficient": 0.857, "pctDone": 1, "base": {"min": 803, "max": 865},
					"taken": {"min": 1250, "max": 1310}, "crit": {"amount": 10000, "result": 15000}}`),
			failed: []string{"e1 damage min"},
		},
		{
			name: "Frostbolt over the server at the bottom",
			rec: handRecord(t, mage, 1, 42842, nil, "", `
				{"index": 1, "kind": "damage", "coefficient": 0.857, "pctDone": 1, "base": {"min": 803, "max": 865},
					"taken": {"min": 1246, "max": 1310}, "crit": {"amount": 10000, "result": 15000}}`),
			failed: []string{"e1 damage min"},
		},
		{
			// Improved Thunder Clap 3/3 is a 30% SPELLMOD_DAMAGE: (300 + int(0.12 * 1000)) * 1.3
			name: "Thunder Clap with Improved Thunder Clap",
			rec: handRecord(t, warrior, 1, 47502, []int32{12666}, `{"direct": 0, "dot": 0, "ap": 0.12, "apDot": 0}`, `
				{"index": 0, "kind": "damage", "pctDone": 1, "base": {"min": 300, "max": 300},
					"taken": {"min": 546, "max": 546}, "crit": {"amount": 10000, "result": 20000}}`),
		},
		{
			name: "Thunder Clap without the talent the numbers have",
			rec: handRecord(t, warrior, 1, 47502, nil, `{"direct": 0, "dot": 0, "ap": 0.12, "apDot": 0}`, `
				{"index": 0, "kind": "damage", "pctDone": 1, "base": {"min": 300, "max": 300},
					"taken": {"min": 546, "max": 546}, "crit": {"amount": 10000, "result": 20000}}`),
			failed: []string{"e0 damage min", "e0 damage max"},
		},
		{
			// Glyph of Exorcism adds 20%: (1033 + int(0.15 * 1000) + int(0.15 * 520)) * 1.2 = 1513.2
			name: "Exorcism with its glyph",
			rec: handRecord(t, paladin, 1, 48801, []int32{54934}, `{"direct": 0.15, "dot": 0, "ap": 0.15, "apDot": 0}`, `
				{"index": 0, "kind": "damage", "coefficient": 0.15, "pctDone": 1, "base": {"min": 1033, "max": 1151},
					"taken": {"min": 1513, "max": 1654}, "crit": {"amount": 10000, "result": 15000}}`),
		},
		{
			// 180 + int(0.2 * 520) per tick
			name: "Corruption bare",
			rec: handRecord(t, warlock, 1, 47813, nil, `{"direct": 0, "dot": 0.2, "ap": 0, "apDot": 0}`, `
				{"index": 0, "kind": "dot", "coefficient": 0.2, "pctDone": 1, "base": {"min": 180, "max": 180},
					"taken": {"min": 284, "max": 284}, "crit": {"amount": 10000, "result": 15000}}`),
		},
		{
			// (230 + int(0.1833 * 520)) * 1.15; Shadowform lets the ticks crit for double
			name: "Shadow Word: Pain in Shadowform",
			rec: handRecord(t, priest, 5, 48125, []int32{15473}, `{"direct": 0, "dot": 0.1833, "ap": 0, "apDot": 0}`, `
				{"index": 0, "kind": "dot", "coefficient": 0.1833, "pctDone": 1.15, "base": {"min": 230, "max": 230},
					"taken": {"min": 373, "max": 373}, "crit": {"amount": 10000, "result": 20000}}`),
		},
		{
			// a pure dot isn't crit capable, but its ticks still crit
			name: "Shadow Word: Pain in Shadowform at the spell crit bonus",
			rec: handRecord(t, priest, 5, 48125, []int32{15473}, `{"direct": 0, "dot": 0.1833, "ap": 0, "apDot": 0}`, `
				{"index": 0, "kind": "dot", "coefficient": 0.1833, "pctDone": 1.15, "base": {"min": 230, "max": 230},
					"taken": {"min": 373, "max": 373}, "crit": {"amount": 10000, "result": 15000}}`),
			failed: []string{"e0 dot crit"},
		},
		{
			// a heal on the caster, off healing power: 1896 + int(0.8057 * 520 = 418.96)
			name: "Flash Heal bare",
			rec: handRecord(t, priest, 5, 48071, nil, "", `
				{"index": 0, "kind": "heal", "coefficient": 0.8057, "pctDone": 1, "base": {"min": 1896, "max": 2202},
					"taken": {"min": 2314, "max": 2620}, "crit": {"amount": 10000, "result": 15000}}`),
		},
		{
			// Blood Presence is on the server's character, so the sim's goes on too: (227 + int(0.1 * 1000)) * 1.15
			name: "Icy Touch in Blood Presence",
			rec: handRecord(t, deathKnight, 1, 49909, []int32{48266}, `{"direct": 0, "dot": 0, "ap": 0.1, "apDot": 0}`, `
				{"index": 0, "kind": "damage", "pctDone": 1.15, "base": {"min": 227, "max": 245},
					"taken": {"min": 376, "max": 396}, "crit": {"amount": 10000, "result": 20000}}`),
		},
		{
			// the probe reads 47632's own 600 base points, where a real cast hands it the 443 the sim
			// declares, so only the amount on the probe's base is compared
			name: "Death Coil damage on the probe's own base",
			rec: handRecord(t, deathKnight, 1, 47632, nil, `{"direct": 0, "dot": 0, "ap": 0.15, "apDot": 0}`, `
				{"index": 0, "kind": "damage", "pctDone": 1, "base": {"min": 600, "max": 600},
					"taken": {"min": 750, "max": 750}, "crit": {"amount": 10000, "result": 20000}}`),
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			checks := checkDamage(c.rec)
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

// Sheath of Light adds spell power off attack power, so the record's spell power has to hold with the
// record's attack power in too.
func TestDamageSimHoldsTheRecordsPower(t *testing.T) {
	rec := handRecord(t, paladin, 1, 48801, []int32{53503}, "", `{"index": 0, "kind": "damage", "base": {"min": 1033, "max": 1151}}`)
	d, err := buildDamageSim(rec)
	if err != nil {
		t.Fatal(err)
	}
	d.prepare(rec.SpellPower.Damage)
	for _, c := range []struct {
		stat stats.Stat
		want float64
	}{{stats.SpellPower, 520}, {stats.AttackPower, 1000}, {stats.RangedAttackPower, 900}} {
		if got := d.char.GetStat(c.stat); math.Abs(got-c.want) > 1e-6 {
			t.Errorf("%s %.2f, want %.0f", c.stat.StatName(), got, c.want)
		}
	}
}

func TestShapedDamageRecordIsNotCompared(t *testing.T) {
	rec := handRecord(t, warrior, 1, 23881, nil, "", `{"index": 0, "kind": "unprobed", "shaped": "script effect"}`)
	rec.Shaped = "script spell_warr_bloodthirst"
	if checks := checkDamage(rec); len(checks) != 0 {
		t.Errorf("a shaped record gave %d checks", len(checks))
	}
}

// A full simval.jsonl recapture also picks up TestSimvalDamage's records. They have to parse as the
// replay's records, give no checks, and key like any other record.
func TestDamageRecordInTheReplayFixture(t *testing.T) {
	text := `{"schema": 1, "command": "damage", "spell": {"id": 42842, "name": "Frostbolt", "schoolMask": 16,
		"dmgClass": 1, "attackType": "mainhand", "family": 3, "familyFlags": [32, 0, 0]}, "shaped": "",
		"attacker": {"name": "Bot", "isPlayer": true, "class": 8, "spellPower": {"frost": 520},
			"attacks": [{"type": "mainhand", "attackPower": 1000, "weapon": {"itemId": 1}}], "auras": []},
		"target": {"name": "Dummy", "entry": 999000, "creatureType": 5},
		"spellPower": {"damage": 520, "healing": 520}, "attackPower": {"type": "mainhand", "value": 1000},
		"bonus": {"direct": 0.857, "dot": 0, "ap": 0, "apDot": 0},
		"effects": [{"index": 1, "effect": 2, "aura": 0, "kind": "damage", "victim": "target",
			"base": {"min": 803, "max": 865}, "done": {"min": 1248, "max": 1310},
			"taken": {"min": 1248, "max": 1310}, "crit": {"amount": 10000, "result": 15000}}]}`
	var line bytes.Buffer
	if err := json.Compact(&line, []byte(text)); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "simval.jsonl")
	if err := os.WriteFile(path, append(line.Bytes(), '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
	records, err := readRecords(path)
	if err != nil {
		t.Fatal(err)
	}
	if checks := CheckRecord(records[0]); len(checks) != 0 {
		t.Errorf("the replay checked a damage record: %+v", checks)
	}
	if got, want := records[0].key(), (recordKey{"damage", 42842, mage, 999000}); got != want {
		t.Errorf("key %v, want %v", got, want)
	}
}

// probeRecord is a bare-character record for the spell, each damage or heal effect at its level 80
// range from serverdata. The amounts are left out.
func probeRecord(t *testing.T, class, race uint8, spellID int32) damageRecord {
	t.Helper()
	s := serverdata.SpellByID(spellID)
	if s == nil {
		t.Fatalf("spell %d isn't in serverdata", spellID)
	}
	var effects []string
	for i, e := range s.Effects {
		kind := ""
		switch {
		case e.Effect == 2 || e.Effect == 9:
			kind = "damage"
		case e.Effect == 10:
			kind = "heal"
		case e.Aura == 3 || e.Aura == 53:
			kind = "dot"
		case e.Aura == 8:
			kind = "hot"
		}
		if kind == "" {
			continue
		}
		lo, hi := s.EffectRange(i)
		effects = append(effects, fmt.Sprintf(`{"index": %d, "kind": %q, "base": {"min": %d, "max": %d}}`, i, kind, lo, hi))
	}
	return handRecord(t, class, race, spellID, nil, "", strings.Join(effects, ","))
}

// Every spell TestSimvalDamage compares has a sim declaration for each probed effect, and the sim's
// code runs for it with the base pinned.
func TestDamageReplayCoversTheE2EProbes(t *testing.T) {
	for _, c := range damageProbes {
		for _, id := range c.spells {
			rec := probeRecord(t, c.class, c.race, id)
			d, err := buildDamageSim(rec)
			if err != nil {
				t.Errorf("%d: %v", id, err)
				continue
			}
			for _, e := range rec.Effects {
				decl := d.declaration(e)
				if decl == nil {
					t.Errorf("%d e%d %s: no sim declaration", id, e.Index, e.Kind)
					continue
				}
				for _, base := range []float64{e.Base.Min, e.Base.Max} {
					amount, err := d.amount(*decl, e, base)
					if err != nil || amount < base {
						t.Errorf("%d e%d %s at base %.0f: %.2f, %v", id, e.Index, e.Kind, base, amount, err)
					}
				}
			}
		}
	}
}

// damageMods are the talent and glyph auras TestSimvalDamage's second pass adds per class.
var damageMods = map[uint8][]int32{
	warrior:     {12666, 12799, 46860},
	paladin:     {35397, 54934, 31868},
	hunter:      {19456, 19466, 56337, 63458},
	rogue:       {16515, 51633, 31236},
	priest:      {15310, 15317, 63627, 52803, 15356, 15473},
	deathKnight: {49638, 51456, 49565, 49664},
	shaman:      {16108, 60188, 16161, 51482},
	mage:        {12953, 15047, 15060, 12400, 12469},
	warlock:     {18275, 17814, 30064, 17958, 17834, 59741},
	druid:       {16899, 16822, 57814, 16999, 17073},
}

// The second pass's auras reach the sim as talents and glyphs, and raise every compared spell there as
// the e2e test expects them to on the server.
func TestDamageModsRaiseEveryProbe(t *testing.T) {
	for _, c := range damageProbes {
		for _, id := range c.spells {
			bare := probeRecord(t, c.class, c.race, id)
			modded := bare
			for _, mod := range damageMods[c.class] {
				modded.Attacker.Auras = append(slices.Clone(modded.Attacker.Auras), aura{ID: mod, DurationMs: -1})
			}
			setup, err := damageSetupFor(modded)
			if err != nil || !setup.modded || len(setup.others) != 0 {
				t.Errorf("%d: setup %+v, %v", id, setup, err)
			}
			var sums [2]float64
			for i, rec := range []damageRecord{bare, modded} {
				d, err := buildDamageSim(rec)
				if err != nil {
					t.Fatalf("%d: %v", id, err)
				}
				for _, e := range rec.Effects {
					amount, err := d.amount(*d.declaration(e), e, e.Base.Max)
					if err != nil {
						t.Fatalf("%d e%d: %v", id, e.Index, err)
					}
					sums[i] += amount
				}
			}
			t.Logf("%d: %.2f bare, %.2f with %s (x%.4f)", id, sums[0], sums[1], setup.talentsString(), sums[1]/sums[0])
			if sums[1] <= sums[0] {
				t.Errorf("%d: %.2f with the talents, %.2f without", id, sums[1], sums[0])
			}
		}
	}
}

// TestStrikesRecords replays a live TestSimvalStrikes capture: each unshaped strike's weapon part, each
// pet's stats and spells, and the Ghoul Frenzy timing. Skips without the fixture.
func TestStrikesRecords(t *testing.T) {
	path := filepath.Join("..", "..", fixtureDir, strikesFixture)
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Skipf("no capture at %s; record one with mod-sim-validation's TestSimvalStrikes", path)
	}
	records, err := readStrikesRecords(path)
	if err != nil {
		t.Fatal(err)
	}

	var compared, checked int
	used := map[strikesMismatch]bool{}
	for _, rec := range records {
		key, err := rec.key()
		if err != nil {
			t.Errorf("%s: %v", rec.label(), err)
			continue
		}
		if slices.Contains(droppedStrikesRecords, key) {
			t.Errorf("%s is dropped, but %s still holds it", key, path)
		}
		checks := checkStrikesRecord(rec)
		if len(checks) > 0 {
			compared++
		}
		for _, check := range checks {
			checked++
			mismatch := strikesMismatch{key, check.Name}
			reason, known := knownStrikesMismatches[mismatch]
			switch {
			case !check.Passed && known:
				used[mismatch] = true
				t.Logf("%s: %s: known mismatch, %s: %s", rec.label(), check.Name, reason, check.Detail)
			case !check.Passed:
				t.Errorf("%s: %s: %s", rec.label(), check.Name, check.Detail)
			default:
				t.Logf("%s: %s: %s", rec.label(), check.Name, check.Detail)
			}
		}
	}
	if compared == 0 {
		t.Fatalf("%s has %d records but none to compare", path, len(records))
	}
	for m := range knownStrikesMismatches {
		if !used[m] {
			t.Errorf("%s %q is a known mismatch, but no record of the capture fails it", m.key, m.check)
		}
	}
	t.Logf("%d checks over %d compared records of %d", checked, compared, len(records))
}

// strikesMismatch is one check of one strikes.jsonl probe.
type strikesMismatch struct {
	key   strikesKey
	check string
}

// knownStrikesMismatches are checks where the sim and the server really differ, each with its reason and a row
// in the fixture README. The check still runs and only its failure is let through; an entry no record fails is
// stale.
var knownStrikesMismatches = expandStrikesMismatches([]knownStrikes{})

// The Infernal's info with its owner's buffs on.
var infernalBuffedInfo = strikesKey{"info", 0, warlock, 89, true}

// strikesUnrecorded are probes TestSimvalStrikes takes that strikes.jsonl predates. The fixture test lets
// them be missing and fails once the fixture has one, so the entry goes with the re-record.
var strikesUnrecorded = []strikesKey{}

// knownStrikes is one reason for each of the checks on each of the probes.
type knownStrikes struct {
	keys   []strikesKey
	checks []string
	reason string
}

func expandStrikesMismatches(known []knownStrikes) map[strikesMismatch]string {
	out := map[strikesMismatch]string{}
	for _, k := range known {
		for _, key := range k.keys {
			for _, check := range k.checks {
				out[strikesMismatch{key, check}] = k.reason
			}
		}
	}
	return out
}

// strikesBothPasses keys the spells' strikes, bare and with talents.
func strikesBothPasses(class uint8, spells ...int32) []strikesKey {
	var keys []strikesKey
	for _, spell := range spells {
		keys = append(keys, strikesKey{"damage", spell, class, 0, false}, strikesKey{"damage", spell, class, 0, true})
	}
	return keys
}

// droppedStrikesRecords disagree with the sim and were removed from strikes.jsonl, each with a row in the
// fixture README.
var droppedStrikesRecords = []strikesKey{}

type strikeHand struct {
	kind string // the snapshot's attack type
	item int32
}

// strikesCharacters are TestSimvalStrikes' melee bots: the weapons, the form cast with the auras, the strikes the
// sim compares, those the server gives shaped, and the talent auras of the second pass.
var strikesCharacters = []struct {
	class, race      uint8
	hands            []strikeHand
	form             int32
	compared, shaped []int32
	talents          []int32
}{
	{warrior, 1, []strikeHand{{"mainhand", 40384}}, 0,
		[]int32{47450, 47520, 47486, 50783, 7384, 1680}, []int32{47498}, []int32{12712, 35449, 16494, 20496, 46860}},
	{paladin, 1, []strikeHand{{"mainhand", 40384}}, 0,
		[]int32{35395, 20424, 42463}, []int32{53385, 20467}, []int32{20113, 31868, 35397, 53488}},
	{rogue, 1, []strikeHand{{"mainhand", 40383}, {"offhand", 39714}}, 0,
		[]int32{48638, 48657, 48660, 48665, 48664, 14278, 5940}, nil, []int32{61331, 32601, 14137, 14072, 14159, 13852}},
	{deathKnight, 1, []strikeHand{{"mainhand", 47446}}, 0,
		[]int32{49921, 49930, 55262, 51425, 49924, 55268, 55271, 50463}, []int32{56815},
		[]int32{55108, 49395, 49534, 55237, 49664, 54637}},
	{shaman, 11, []strikeHand{{"mainhand", 39763}, {"offhand", 39468}}, 0,
		[]int32{32175, 32176}, []int32{60103}, []int32{29086}},
	{hunter, 4, []strikeHand{{"mainhand", 40388}, {"ranged", 40385}}, 0,
		[]int32{49050, 49048, 34490, 48996}, []int32{61006, 53209}, []int32{19509, 19490, 24691}},
	{druid, 4, []strikeHand{{"mainhand", 40388}}, 768,
		[]int32{48566, 48572, 62078}, nil, []int32{16999, 17073, 33867, 16949}},
	{druid, 4, []strikeHand{{"mainhand", 40388}}, 9634,
		[]int32{48480, 48564}, nil, []int32{16999, 17073}},
}

// strikesPets are TestSimvalStrikes' pets and guardians: the owner's class and race, the creature, and the spells
// it records `damage` for besides its `info`.
var strikesPets = []struct {
	class, race uint8
	entry       uint32
	spells      []int32
}{
	{hunter, 4, 37294, []int32{52474}},
	{warlock, 1, 416, []int32{47964}},
	{warlock, 1, 17252, []int32{47994}},
	{warlock, 1, 89, []int32{20153}},
	{deathKnight, 1, 26125, []int32{47468}},
	{deathKnight, 1, 27829, []int32{51963}},
	{shaman, 11, 29264, nil},
	{mage, 1, 510, []int32{31707}},
	{druid, 4, 1964, nil},
	{priest, 1, 19668, nil},
}

// strikesTimings are the `timing` probes of TestSimvalStrikes' pet owners, by class.
var strikesTimings = map[uint8][]int32{deathKnight: {63560}}

// strikesOtherProbes are strikes.jsonl's records from other e2e tests, which a re-record of TestSimvalStrikes has to
// keep: TestEcTools5OffHandObliterate's off-hand Obliterate, bare and with Nerves of Cold Steel.
var strikesOtherProbes = strikesBothPasses(deathKnight, 66974)

func expectedStrikesKeys() []strikesKey {
	keys := slices.Clone(strikesOtherProbes)
	for _, c := range strikesCharacters {
		for _, spell := range slices.Concat(c.compared, c.shaped) {
			for _, modded := range []bool{false, true} {
				keys = append(keys, strikesKey{"damage", spell, c.class, 0, modded})
			}
		}
	}
	for _, p := range strikesPets {
		keys = append(keys, strikesKey{"info", 0, p.class, p.entry, false})
		if len(strikesPetBuffs[p.entry]) > 0 {
			keys = append(keys, strikesKey{"info", 0, p.class, p.entry, true})
		}
		for _, spell := range p.spells {
			keys = append(keys, strikesKey{"damage", spell, p.class, p.entry, false})
		}
	}
	for class, spells := range strikesTimings {
		for _, spell := range spells {
			keys = append(keys, strikesKey{"timing", spell, class, 0, false})
		}
	}
	return keys
}

func TestStrikesFixtureHasEveryProbe(t *testing.T) {
	path := filepath.Join("..", "..", fixtureDir, strikesFixture)
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Skipf("no capture at %s", path)
	}
	records, err := readStrikesRecords(path)
	if err != nil {
		t.Fatal(err)
	}
	have := map[strikesKey]bool{}
	for _, rec := range records {
		key, err := rec.key()
		if err != nil {
			t.Fatalf("%s: %v", rec.label(), err)
		}
		have[key] = true
	}
	for _, key := range expectedStrikesKeys() {
		unrecorded := slices.Contains(strikesUnrecorded, key)
		switch {
		case have[key] && unrecorded:
			t.Errorf("%s is recorded now, so take it off strikesUnrecorded", key)
		case !have[key] && !unrecorded && !slices.Contains(droppedStrikesRecords, key):
			t.Errorf("no record for %s", key)
		}
	}
}

func rng(lo, hi float64) damageRange { return damageRange{lo, hi} }

// handWeapon is a weapon part with no done or taken modifier: base, done and taken all the same. The attack power
// is the hand's in handStrike.
func handWeapon(attackType string, physical, normalized bool, apMultiplier, totalPct float64, weapon, fixed damageRange,
	percent float64, base damageRange, crit float64) weaponStrike {
	w := weaponStrike{AttackType: attackType, Physical: physical, Normalized: normalized, APMultiplier: apMultiplier,
		AttackPower: 1000, TotalPct: totalPct, WeaponDamage: weapon, Fixed: fixed, Percent: rng(percent, percent),
		Base: base, Done: base, Taken: base}
	if attackType == "ranged" {
		w.AttackPower = 900
	}
	w.Crit.Amount, w.Crit.Result = 10000, crit
	return w
}

// handStrike is a strike record the way the module writes one: a level 80 character of the class and race wearing
// the weapons, with 520 spell power and 1000 melee and 900 ranged attack power, at the level 83 boss dummy. The
// effects are the spell's own from serverdata, the weapon part as given.
func handStrike(t *testing.T, class, race uint8, spellID int32, auras []int32, hands []strikeHand, w weaponStrike) damageRecord {
	t.Helper()
	s := serverdata.SpellByID(spellID)
	if s == nil {
		t.Fatalf("spell %d isn't in serverdata", spellID)
	}
	attackPower := map[string]int{"mainhand": 1000, "offhand": 1000, "ranged": 900}
	var attacks []string
	worn := map[string]bool{}
	for _, h := range hands {
		attacks = append(attacks, fmt.Sprintf(`{"type": %q, "attackPower": %d, "weapon": {"itemId": %d}}`,
			h.kind, attackPower[h.kind], h.item))
		worn[h.kind] = true
	}
	for _, kind := range []string{"mainhand", "ranged"} {
		if !worn[kind] {
			attacks = append(attacks, fmt.Sprintf(`{"type": %q, "attackPower": %d}`, kind, attackPower[kind]))
		}
	}
	var auraJSON []string
	for _, id := range append([]int32{54192, 54161, 47436}, auras...) {
		auraJSON = append(auraJSON, fmt.Sprintf(`{"id": %d, "durationMs": -1}`, id))
	}
	var effects []string
	for i, e := range s.Effects {
		switch {
		case e.Effect == 0:
		case strikeWeaponEffects[e.Effect]:
			lo, hi := s.EffectRange(i)
			effects = append(effects, fmt.Sprintf(`{"index": %d, "effect": %d, "kind": "unprobed", "shaped": "weapon damage",
				"base": {"min": %d, "max": %d}}`, i, e.Effect, lo, hi))
		default:
			effects = append(effects, fmt.Sprintf(`{"index": %d, "effect": %d, "kind": "other"}`, i, e.Effect))
		}
	}
	apType := "mainhand"
	if w.AttackType == "ranged" {
		apType = "ranged"
	}
	weapon, err := json.Marshal(w)
	if err != nil {
		t.Fatal(err)
	}
	return damageRec(t, fmt.Sprintf(`{
		"schema": 1, "command": "damage",
		"spell": {"id": %d, "name": "spell %d"},
		"shaped": "no direct, periodic or heal effect to probe",
		"attacker": {"name": "Bot", "level": 80, "isPlayer": true, "class": %d, "race": %d,
			"spellPower": {"holy": 520, "fire": 520}, "attacks": [%s], "auras": [%s]},
		"target": {"name": "Dummy", "entry": 999000, "level": 83, "worldBoss": true, "creatureType": 5,
			"health": 1000000, "maxHealth": 1000000, "resistances": [10643, 0, 0, 0, 0, 0, 0], "auras": []},
		"spellPower": {"damage": 520, "healing": 520},
		"attackPower": {"type": %q, "value": %d},
		"effects": [%s],
		"weapon": %s
	}`, spellID, spellID, class, race, strings.Join(attacks, ","), strings.Join(auraJSON, ","), apType, attackPower[apType],
		strings.Join(effects, ","), weapon))
}

func failedCheckNames(checks []Check) []string {
	_, failed := checkNames(checks)
	return failed
}

// The server numbers below follow Spell::EffectWeaponDmg by hand for a bare character, which has no done or taken
// modifier: the weapon's range plus attack power / 14 times the normalized speed or the weapon's own, times the
// hand's percent and truncated; the fixed bonus times the hand's percent, truncated; then the weapon percent,
// truncated.
func TestStrikeReplayOnHandCheckedRecords(t *testing.T) {
	axe := []strikeHand{{"mainhand", 40384}}
	// 606 and 910 plus int(1000 / 14 × 3.3 = 235.71), then 380
	mortalStrike := handWeapon("mainhand", true, true, 3.3, 1, rng(841, 1145), rng(380, 380), 100, rng(1221, 1525), 20000)
	with := func(w weaponStrike, change func(*weaponStrike)) weaponStrike {
		change(&w)
		return w
	}
	for _, c := range []struct {
		name   string
		rec    damageRecord
		failed []string
	}{
		{name: "Mortal Strike bare", rec: handStrike(t, warrior, 1, 47486, nil, axe, mortalStrike)},
		{
			name:   "Mortal Strike 2% high at the top",
			rec:    handStrike(t, warrior, 1, 47486, nil, axe, with(mortalStrike, func(w *weaponStrike) { w.Taken.Max = 1556 })),
			failed: []string{"max"},
		},
		{
			// truncation only lowers the server's number, so the sim under it gets 0.1% or 1 point
			name:   "Mortal Strike under the server at the bottom",
			rec:    handStrike(t, warrior, 1, 47486, nil, axe, with(mortalStrike, func(w *weaponStrike) { w.Taken.Min = 1224 })),
			failed: []string{"min"},
		},
		{
			// the amounts follow the record's own fixed bonus, so only the declaration fails
			name: "Mortal Strike with another fixed bonus",
			rec: handStrike(t, warrior, 1, 47486, nil, axe, with(mortalStrike, func(w *weaponStrike) {
				w.Fixed = rng(390, 390)
				w.Taken = rng(1231, 1535)
			})),
			failed: []string{"fixed"},
		},
		{
			name:   "Mortal Strike with a bigger crit",
			rec:    handStrike(t, warrior, 1, 47486, nil, axe, with(mortalStrike, func(w *weaponStrike) { w.Crit.Result = 23000 })),
			failed: []string{"crit"},
		},
		{
			// holy, so neither normalized nor the hand's percent: 606 and 910 plus int(1000 / 14 × 3.4 = 242.86),
			// int(848 × 0.36) and int(1152 × 0.36)
			name: "Seal of Command",
			rec: handStrike(t, paladin, 1, 20424, nil, axe,
				handWeapon("mainhand", false, false, 3.4, 1, rng(848, 1152), rng(0, 0), 36, rng(305, 414), 20000)),
		},
		{
			// the off hand's percent is 0.5: int((153 + 1000 / 14 × 1.7) × 0.5) = 137, and 285 gives 203; then
			// int(181 × 0.5) = 90
			name: "off-hand Mutilate",
			rec: handStrike(t, rogue, 1, 48664, nil, []strikeHand{{"mainhand", 40383}, {"offhand", 39714}},
				handWeapon("offhand", true, true, 1.7, 0.5, rng(137, 203), rng(181, 181), 100, rng(227, 293), 20000)),
		},
		{
			// 376 and 646 plus 900 / 14 × 2.8 = 180, then 408
			name: "Aimed Shot",
			rec: handStrike(t, hunter, 4, 49050, nil, []strikeHand{{"mainhand", 40388}, {"ranged", 40385}},
				handWeapon("ranged", true, true, 2.8, 1, rng(556, 826), rng(408, 408), 100, rng(964, 1234), 20000)),
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			checks := checkStrike(c.rec)
			if len(checks) == 0 {
				t.Fatal("no checks")
			}
			if failed := failedCheckNames(checks); !slices.Equal(failed, c.failed) {
				t.Errorf("failed %v, want %v: %s", failed, c.failed, strings.Join(failedChecks(checks), "; "))
			}
		})
	}
}

func TestShapedOrWeaponlessStrike(t *testing.T) {
	w := handWeapon("mainhand", true, true, 3.3, 1, rng(841, 1145), rng(242, 242), 120, rng(1300, 1600), 20000)
	w.Shaped = "EffectWeaponDmg: Devastate"
	if checks := checkStrike(handStrike(t, warrior, 1, 47498, nil, []strikeHand{{"mainhand", 40384}}, w)); len(checks) != 0 {
		t.Errorf("a shaped strike gave %d checks", len(checks))
	}
	rec := handStrike(t, warrior, 1, 47486, nil, nil, w)
	rec.Weapon = nil
	if failed := failedCheckNames(checkStrike(rec)); !slices.Equal(failed, []string{"weapon"}) {
		t.Errorf("a record without a weapon part failed %v", failed)
	}
}

// strikeProbe is a strike record for the character's weapons: its fixed bonus and percent from serverdata, the
// amounts a rough guess. A strike needing an off hand is the off hand's, as SimValidation::SpellAttackType has it.
func strikeProbe(t *testing.T, class, race uint8, hands []strikeHand, spellID int32, auras []int32) damageRecord {
	t.Helper()
	s := serverdata.SpellByID(spellID)
	if s == nil {
		t.Fatalf("spell %d isn't in serverdata", spellID)
	}
	const attr3RequiresOffHandWeapon = 0x01000000
	attackType := "mainhand"
	switch {
	case s.DmgClass == serverdata.DmgClassRanged:
		attackType = "ranged"
	case s.DmgClass == serverdata.DmgClassMelee && s.Attributes[3]&attr3RequiresOffHandWeapon != 0:
		attackType = "offhand"
	}
	fixed, percent, normalized := 0.0, 100.0, false
	for i, e := range s.Effects {
		lo, _ := s.EffectRange(i)
		switch e.Effect {
		case 31:
			percent = percent * float64(lo) / 100
		case 17, 58, 121:
			fixed += float64(lo)
			normalized = normalized || e.Effect == 121
		}
	}
	guess := rng(1000, 1500)
	return handStrike(t, class, race, spellID, auras, hands,
		handWeapon(attackType, s.SchoolMask == 1, normalized, 2.4, 1, guess, rng(fixed, fixed), percent, guess, 20000))
}

// Every strike TestSimvalStrikes compares has a sim declaration, and the sim's strike runs with the roll pinned
// to either end.
func TestStrikeReplayCoversTheE2EProbes(t *testing.T) {
	for _, c := range strikesCharacters {
		var auras []int32
		if c.form != 0 {
			auras = []int32{c.form}
		}
		for _, id := range c.compared {
			rec := strikeProbe(t, c.class, c.race, c.hands, id, auras)
			d, spell, err := buildStrikeSim(rec)
			if err != nil {
				t.Errorf("%d: %v", id, err)
				continue
			}
			lo, errLo := d.strikeAmount(spell, rec.Weapon, 0)
			hi, errHi := d.strikeAmount(spell, rec.Weapon, 1)
			if err := firstErr(errLo, errHi); err != nil || lo <= 0 || hi <= lo {
				t.Errorf("%d (%s): %.2f to %.2f, %v", id, spell.ActionID, lo, hi, err)
				continue
			}
			for _, check := range checkStrike(rec) {
				if !check.Passed && (check.Name == "fixed" || check.Name == "percent") {
					t.Errorf("%d (%s): %s: %s", id, spell.ActionID, check.Name, check.Detail)
				}
			}
			t.Logf("%d (%s): %.2f to %.2f", id, spell.ActionID, lo, hi)
		}
	}
}

// The second pass's auras reach the sim as talents: an aura it can't place would leave the sim's strike without
// the talent the server's has.
func TestStrikeTalentAurasAreSimTalents(t *testing.T) {
	for _, c := range strikesCharacters {
		rec := strikeProbe(t, c.class, c.race, c.hands, c.compared[0], c.talents)
		setup, err := damageSetupFor(rec)
		if err != nil || !setup.modded || len(setup.others) != 0 {
			t.Errorf("class %d: talents %s, auras the sim has no talent for %v, %v", c.class, setup.talentsString(), setup.others, err)
		}
	}
}

// handPet is a pet's record the way the module writes one: the creature at level 80 with 300 attack power and 500
// spell power, owned by a level 80 character of the class and race, at the boss dummy. body adds the command's
// own fields.
func handPet(t *testing.T, class, race uint8, entry uint32, command, body string) strikesRecord {
	t.Helper()
	text := fmt.Sprintf(`{
		"schema": 1, "command": %q,
		"attacker": {"name": "Pet", "entry": %d, "level": 80, "isPlayer": false,
			"stats": {"strength": 100, "agility": 100, "stamina": 100, "intellect": 100, "spirit": 100},
			"resistances": [1000, 0, 0, 0, 0, 0, 0],
			"spellPower": {"holy": 500, "fire": 500, "nature": 500, "frost": 500, "shadow": 500, "arcane": 500},
			"attacks": [{"type": "mainhand", "attackPower": 300}], "auras": []},
		"owner": {"name": "Bot", "level": 80, "isPlayer": true, "class": %d, "race": %d,
			"stats": {"strength": 200, "agility": 200, "stamina": 1000, "intellect": 800, "spirit": 300},
			"resistances": [2000, 0, 0, 0, 0, 0, 0],
			"spellPower": {"holy": 600, "fire": 600, "nature": 600, "frost": 600, "shadow": 600, "arcane": 600},
			"attacks": [{"type": "mainhand", "attackPower": 1000}, {"type": "ranged", "attackPower": 900}],
			"auras": [{"id": 54192, "durationMs": -1}, {"id": 54161, "durationMs": -1}, {"id": 47436, "durationMs": -1}]},
		"target": {"name": "Dummy", "entry": 999000, "level": 83, "worldBoss": true, "creatureType": 5,
			"health": 1000000, "maxHealth": 1000000, "resistances": [10643, 0, 0, 0, 0, 0, 0], "auras": []},
		%s
	}`, command, entry, class, race, body)
	var rec strikesRecord
	if err := json.Unmarshal([]byte(text), &rec); err != nil {
		t.Fatalf("%v in %s", err, text)
	}
	return rec
}

// petSpell is a pet's damage record body for the spell with its base, amounts and crit result.
func petSpell(id int32, kind string, coefficient float64, base, taken damageRange, crit float64) string {
	return fmt.Sprintf(`"spell": {"id": %d, "name": "spell %d"}, "shaped": "",
		"spellPower": {"damage": 500, "healing": 500}, "attackPower": {"type": "mainhand", "value": 300},
		"effects": [{"index": 0, "effect": 2, "kind": %q, "coefficient": %g, "base": {"min": %.0f, "max": %.0f},
			"taken": {"min": %.0f, "max": %.0f}, "crit": {"amount": 10000, "result": %.0f}}]`,
		id, id, kind, coefficient, base.Min, base.Max, taken.Min, taken.Max, crit)
}

// A pet's spells compare on the sim's pet with its own spell power pinned. The numbers, crit included, are what
// the sim works out, so they show the pet is the one casting; the live capture holds the server's.
func TestPetSpellReplay(t *testing.T) {
	for _, c := range []struct {
		name   string
		rec    strikesRecord
		failed []string
	}{
		{
			// 203 + int(0.714 × 500 = 357)
			name: "Imp's Firebolt",
			rec:  handPet(t, warlock, 1, 416, "damage", petSpell(47964, "damage", 0.714, rng(203, 227), rng(560, 584), 15000)),
		},
		{
			name:   "Imp's Firebolt off the owner's spell power",
			rec:    handPet(t, warlock, 1, 416, "damage", petSpell(47964, "damage", 0.714, rng(203, 227), rng(631, 655), 15000)),
			failed: []string{"e0 damage min", "e0 damage max"},
		},
		{
			// the sim deals Immolation's damage from its aura's ticks: 40 + 1.35 × 500
			name: "Infernal's Immolation",
			rec:  handPet(t, warlock, 1, 89, "damage", petSpell(20153, "damage", 1.35, rng(40, 40), rng(715, 715), 15000)),
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			checks := checkStrikesRecord(c.rec)
			if len(checks) == 0 {
				t.Fatal("no checks")
			}
			if failed := failedCheckNames(checks); !slices.Equal(failed, c.failed) {
				t.Errorf("failed %v, want %v: %s", failed, c.failed, strings.Join(failedChecks(checks), "; "))
			}
		})
	}
}

// A pet's strike pins the pet's weapon and attack power: the Felguard's Cleave at the bottom is its weapon's 329
// plus 2000 / 14 × 2 s and the 124 fixed, before its multipliers, truncated as the server's done damage is.
func TestPetStrikeReplay(t *testing.T) {
	w := handWeapon("mainhand", true, false, 2, 1, rng(374, 419), rng(124, 124), 100, rng(498, 543), 20000)
	w.AttackPower = 2000
	weapon, err := json.Marshal(w)
	if err != nil {
		t.Fatal(err)
	}
	rec := handPet(t, warlock, 1, 17252, "damage", fmt.Sprintf(`"spell": {"id": 47994, "name": "Cleave"},
		"shaped": "no direct, periodic or heal effect to probe",
		"spellPower": {"damage": 500, "healing": 500}, "attackPower": {"type": "mainhand", "value": 300},
		"effects": [{"index": 0, "effect": 58, "kind": "unprobed", "base": {"min": 124, "max": 124}}],
		"weapon": %s`, weapon))
	d, spell, err := buildStrikeSim(rec.damageRecord)
	if err != nil {
		t.Fatal(err)
	}
	if d.pet == nil || d.pet.pet.Name != "Felguard" {
		t.Fatalf("the strike's caster is %s", d.casterChar().Name)
	}
	lo, err := d.strikeAmount(spell, rec.Weapon, 0)
	if want := math.Trunc(329 + 2000.0/14*2 + 124); err != nil || lo < want || lo > want*1.5 {
		t.Errorf("%.2f, want %.2f before multipliers: %v", lo, want, err)
	}
	if failed := failedCheckNames(checkStrike(rec.damageRecord)); slices.Contains(failed, "fixed") || slices.Contains(failed, "percent") {
		t.Errorf("failed %v", failed)
	}
}

// petInfoBody is an info record's pet part.
func petInfoBody(createStats [5]float64, regen string) string {
	return fmt.Sprintf(`"pet": {"index": 1, "createStats": [%g, %g, %g, %g, %g],
		"scaling": [{"id": 35657, "name": "Mage Pet Scaling 01", "effects": [{"index": 0, "aura": 29, "miscValue": 2, "amount": 300}]}],
		%s}`, createStats[0], createStats[1], createStats[2], createStats[3], createStats[4], regen)
}

// Every pet and guardian TestSimvalStrikes records has a sim pet the replay compares it on, and each of its spells
// a declaration: a strike's weapon part or the spell's effect.
func TestPetReplayCoversTheE2EPets(t *testing.T) {
	for _, p := range strikesPets {
		info := handPet(t, p.class, p.race, p.entry, "info", petInfoBody([5]float64{}, `"regen": null`))
		for _, check := range checkPetInfo(info) {
			if !check.Passed && (check.Name == "sim pet" || check.Name == "base stats") {
				t.Errorf("pet %d: %s: %s", p.entry, check.Name, check.Detail)
			}
			if detail, _, _ := strings.Cut(check.Detail, ";"); !strings.HasPrefix(check.Name, "base ") {
				t.Logf("pet %d: %s: %s", p.entry, check.Name, detail)
			}
		}
		for _, id := range p.spells {
			rec := handPet(t, p.class, p.race, p.entry, "damage", petSpell(id, "damage", 0, rng(1, 1), rng(1, 1), 15000))
			rec.Effects = nil
			s := serverdata.SpellByID(id)
			for i, e := range s.Effects {
				if e.Effect != 0 {
					rec.Effects = append(rec.Effects, damageEffect{Index: int32(i), Effect: e.Effect, Kind: "damage"})
				}
			}
			d, err := buildPetSim(rec.damageRecord)
			if err != nil {
				t.Errorf("pet %d spell %d: %v", p.entry, id, err)
				continue
			}
			if strike := d.strikeDeclaration(); strike != nil {
				t.Logf("pet %d spell %d: strike %s", p.entry, id, strike.ActionID)
				continue
			}
			if decl := d.petDeclaration(rec.Effects[0]); decl == nil {
				t.Errorf("pet %d spell %d: no sim declaration", p.entry, id)
			}
		}
	}
}

// The Infernal's buffed info compares what its owner's buffs added since the bare info, both sim pets scaled
// off the bare owner. Its scaling's immunities keep all four off the sim's pet too.
func TestPetBuffReplay(t *testing.T) {
	infos := func() []strikesRecord {
		bare := handPet(t, warlock, 1, 89, "info", petInfoBody([5]float64{}, `"regen": null`))
		buffed := handPet(t, warlock, 1, 89, "info", petInfoBody([5]float64{}, `"regen": null`))
		for _, b := range strikesPetBuffs[89] {
			buffed.Owner.Auras = append(buffed.Owner.Auras, aura{ID: b.aura, DurationMs: -1})
			buffed.Attacker.Auras = append(buffed.Attacker.Auras, aura{ID: b.aura, DurationMs: -1})
		}
		// the owner's own Devotion Aura and Flametongue Totem, which the Infernal's scaling never takes
		buffed.Owner.Resistances[0] += 1205
		for school := range buffed.Owner.SpellPower {
			buffed.Owner.SpellPower[school] += 144
		}
		records := []strikesRecord{bare, buffed}
		pairBuffedPetInfos(records)
		return records
	}
	records := infos()
	if key, err := records[1].key(); err != nil || key != infernalBuffedInfo || records[0].buffedPetInfo() {
		t.Fatalf("buffed key %v, %v; bare buffed %v", key, err, records[0].buffedPetInfo())
	}

	buffs := &proto.RaidBuffs{}
	for _, b := range strikesPetBuffs[89] {
		b.give(buffs)
	}
	before, err := petSimStats(records[0].damageRecord, nil)
	if err != nil {
		t.Fatal(err)
	}
	after, err := petSimStats(records[0].damageRecord, buffs)
	if err != nil {
		t.Fatal(err)
	}
	added := map[string]float64{}
	for _, name := range petStatNames {
		added[name] = after[name] - before[name]
	}
	for _, name := range petStatNames {
		if added[name] != 0 {
			t.Errorf("the sim's buffs add %.2f %s, want none", added[name], name)
		}
	}

	// the server's buffed pet with the stats the buffs added
	server := func(add map[string]float64) []Check {
		records := infos()
		u := &records[1].Attacker
		u.Stats.Strength += add["Strength"]
		u.Stats.Agility += add["Agility"]
		u.Attacks[0].AttackPower += add["attack power"]
		for school := range u.SpellPower {
			u.SpellPower[school] += add["spell power"]
		}
		u.Resistances[0] += add["armor"]
		return checkStrikesRecord(records[1])
	}
	if failed := failedCheckNames(server(added)); len(failed) != 0 {
		t.Errorf("as the sim has it: failed %v", failed)
	}
	reaching := map[string]float64{"Strength": 155, "Agility": 155, "attack power": 550 + 2*155, "spell power": 144,
		"armor": 1205 + 2*155}
	if failed := failedCheckNames(server(reaching)); !slices.Equal(failed, []string{"Strength from buffs",
		"Agility from buffs", "attack power from buffs", "spell power from buffs", "armor from buffs"}) {
		t.Errorf("every buff reaching it: failed %v", failed)
	}
	lone := infos()[1]
	lone.bare = nil
	if failed := failedCheckNames(checkStrikesRecord(lone)); !slices.Equal(failed, []string{"bare info"}) {
		t.Errorf("no bare info: failed %v", failed)
	}
}

// Base stats are the sim pet's own; the regen tick is the server's: the hunter pet's 24 focus every 4 s.
func TestPetInfoReplay(t *testing.T) {
	waterElemental := func(change float64) strikesRecord {
		rec := handPet(t, mage, 1, 510, "info", petInfoBody([5]float64{}, `"regen": null`))
		d, err := buildPetSim(rec.damageRecord)
		if err != nil {
			t.Fatal(err)
		}
		base := d.pet.pet.GetBaseStats()
		create := [5]float64{base[stats.Strength], base[stats.Agility], base[stats.Stamina], base[stats.Intellect], base[stats.Spirit] + change}
		return handPet(t, mage, 1, 510, "info", petInfoBody(create, `"regen": null`))
	}
	baseFailures := func(checks []Check) []string {
		var failed []string
		for _, name := range failedCheckNames(checks) {
			if strings.HasPrefix(name, "base ") {
				failed = append(failed, name)
			}
		}
		return failed
	}
	if failed := baseFailures(checkPetInfo(waterElemental(0))); len(failed) != 0 {
		t.Errorf("the sim's own base stats failed %v", failed)
	}
	if failed := baseFailures(checkPetInfo(waterElemental(10))); len(failed) != 1 {
		t.Errorf("10 more spirit failed %v", failed)
	}

	focus := func(perTick int) []Check {
		var regen []Check
		body := petInfoBody([5]float64{}, fmt.Sprintf(`"regen": {"power": 2, "intervalMs": 4000, "perTick": %d, "lastManaUse": false}`, perTick))
		for _, check := range checkPetInfo(handPet(t, hunter, 4, 37294, "info", body)) {
			if check.Name == "regen per tick" {
				regen = append(regen, check)
			}
		}
		return regen
	}
	if checks := focus(24); len(checks) != 1 || !checks[0].Passed {
		t.Errorf("24 focus every 4 s: %+v", checks)
	}
	if checks := focus(20); len(checks) != 1 || checks[0].Passed {
		t.Errorf("20 focus every 4 s: %+v", checks)
	}
}

// Ghoul Frenzy's aura goes on the death knight the probe puts it on, and the replay reads the sim's heal over time
// from the talent's spell: a heal every 3 s, 10 of them.
func TestGhoulFrenzyTiming(t *testing.T) {
	ghoulFrenzy := func(amplitudeMs, ticks int) map[string]Check {
		rec := handTiming(t, deathKnight, 1, 63560, []int32{54192, 54161, 47436}, fmt.Sprintf(`
			"aura": {"applies": true, "onCaster": true, "baseMs": 30000, "ms": 30000,
				"effects": [{"index": 0, "periodic": true, "amplitudeMs": %d, "ticks": %d}]}`, amplitudeMs, ticks))
		checks := map[string]Check{}
		for _, check := range replayTiming(rec, buildStrikesTimingSim) {
			checks[check.Name] = check
		}
		return checks
	}
	server := ghoulFrenzy(3000, 10)
	for _, name := range []string{"aura duration", "e0 amplitude", "e0 ticks"} {
		if check, ok := server[name]; !ok || !check.Passed {
			t.Errorf("%s at the server's numbers: %+v", name, check)
		}
	}
	old := ghoulFrenzy(6000, 5)
	for _, name := range []string{"e0 amplitude", "e0 ticks"} {
		if check, ok := old[name]; !ok || check.Passed {
			t.Errorf("%s at 6 s and 5 ticks: %+v", name, check)
		}
	}
}

// A strikes.jsonl line parses with its weapon, owner, pet or timing parts, and keys by what it probed.
func TestStrikesRecordParses(t *testing.T) {
	lines := []string{
		`{"schema":1,"command":"damage","spell":{"id":47486,"name":"Mortal Strike"},"shaped":"no direct, periodic or heal effect to probe",` +
			`"attacker":{"name":"SvStkA","level":80,"isPlayer":true,"class":1,"race":1,"auras":[{"id":12712,"durationMs":-1}]},` +
			`"target":{"name":"Dummy","entry":999000},"effects":[{"index":1,"effect":121,"aura":0,"kind":"unprobed","shaped":"weapon damage","base":{"min":380,"max":380}}],` +
			`"weapon":{"index":1,"shaped":"","attackType":"mainhand","schoolMask":1,"physical":true,"normalized":true,"apMultiplier":3.3,` +
			`"attackPower":1000,"totalPct":1.06,"weaponDamage":{"min":892,"max":1214},"fixed":{"min":380,"max":380},"percent":{"min":100,"max":100},` +
			`"base":{"min":1294,"max":1616},"done":{"min":1294,"max":1616},"taken":{"min":1294,"max":1616},"crit":{"amount":10000,"result":20000}}}`,
		`{"schema":1,"command":"info","attacker":{"name":"Water Elemental","entry":510,"level":80,"isPlayer":false},` +
			`"target":{"name":"Dummy","entry":999000},"owner":{"name":"SvStkB","level":80,"isPlayer":true,"class":8,"race":1},` +
			`"pet":{"index":1,"controlled":[{"index":1,"entry":510,"name":"Water Elemental","isPet":false,"isTotem":false}],` +
			`"createStats":[1,2,3,4,5],"damage":[{"type":"mainhand","min":1,"max":2}],` +
			`"scaling":[{"id":35657,"name":"Mage Pet Scaling 01","effects":[{"index":0,"aura":29,"miscValue":2,"amount":300}]}],` +
			`"regen":{"power":0,"intervalMs":2000,"perTick":40,"regeneratePowerFlag":true,"lastManaUse":false}}}`,
	}
	// plus a real server timing line, like the Ghoul Frenzy one the capture holds
	timing, err := os.ReadFile(filepath.Join("..", "..", fixtureDir, timingFixture))
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range strings.Split(string(timing), "\n") {
		if strings.Contains(line, `"command":"timing"`) {
			lines = append(lines, strings.TrimSpace(line))
			break
		}
	}
	if len(lines) != 3 {
		t.Fatalf("no timing line in %s", timingFixture)
	}
	path := filepath.Join(t.TempDir(), strikesFixture)
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	records, err := readStrikesRecords(path)
	if err != nil {
		t.Fatal(err)
	}
	strike, info := records[0], records[1]
	if w := strike.Weapon; w == nil || w.TotalPct != 1.06 || w.Taken.Max != 1616 || strike.Effects[0].Effect != 121 {
		t.Errorf("strike %+v", strike.Weapon)
	}
	if key, err := strike.key(); err != nil || key != (strikesKey{"damage", 47486, warrior, 0, true}) {
		t.Errorf("strike key %v, %v", key, err)
	}
	if info.Pet == nil || info.Pet.Regen == nil || info.Pet.Regen.PerTick != 40 || info.Pet.Scaling[0].Effects[0].Amount != 300 ||
		info.Pet.CreateStats[4] != 5 {
		t.Errorf("pet %+v", info.Pet)
	}
	if key, err := info.key(); err != nil || key != (strikesKey{"info", 0, mage, 510, false}) {
		t.Errorf("info key %v, %v", key, err)
	}
	rec := records[2]
	if rec.timing == nil || rec.Spell.ID == 0 || rec.timing.Spell.ID != rec.Spell.ID {
		t.Errorf("timing %+v", rec.timing)
	}
	if key, err := rec.key(); err != nil || key.command != "timing" || key.spell != rec.Spell.ID || key.entry != 0 {
		t.Errorf("timing key %v, %v", key, err)
	}
}
