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
			// truncation only ever lowers the server's number, so the sim landing under it gets just
			// 0.1% or 1 point
			name: "Frostbolt under the server at the bottom",
			rec: handRecord(t, mage, 1, 42842, nil, "", `
				{"index": 1, "kind": "damage", "coefficient": 0.857, "pctDone": 1, "base": {"min": 803, "max": 865},
					"taken": {"min": 1250, "max": 1310}, "crit": {"amount": 10000, "result": 15000}}`),
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
