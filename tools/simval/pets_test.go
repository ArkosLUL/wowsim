package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func petsFixturePath() string { return filepath.Join("..", "..", fixtureDir, petsFixture) }

// TestPetsRecords replays a live TestPetfixRecords capture: the Greater Fire Elemental's and a Mirror
// Image's stats, pools, spells, crit and costs, the elemental's swing, an Imp's and a Water Elemental's
// stats, pools and crit roll, and a Shadowfiend's swing, plain and under Shadowcrawl. Skips without the fixture.
func TestPetsRecords(t *testing.T) {
	path := petsFixturePath()
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Skipf("no capture at %s; record one with mod-sim-validation's TestPetfixRecords", path)
	}
	records, err := readPetsRecords(path)
	if err != nil {
		t.Fatal(err)
	}
	checked := 0
	for _, rec := range records {
		checks := checkPetsRecord(rec)
		if len(checks) == 0 {
			t.Errorf("%s: nothing compared", rec.label())
		}
		for _, check := range checks {
			checked++
			if !check.Passed {
				t.Errorf("%s: %s: %s", rec.label(), check.Name, check.Detail)
				continue
			}
			t.Logf("%s: %s: %s", rec.label(), check.Name, check.Detail)
		}
	}
	t.Logf("%d checks over %d records", checked, len(records))
}

// petsProbes are TestPetfixRecords' probes: the command, the spell of one that takes one, the guardian, and
// the aura it was given for the probe.
var petsProbes = []petsKey{
	{"info", 0, 15438, 0},
	{"damage", 57984, 15438, 0}, {"damage", 12470, 15438, 0}, {"damage", 13376, 15438, 0},
	{"spell", 57984, 15438, 0}, {"spell", 13376, 15438, 0},
	{"melee", 0, 15438, 0},
	{"timing", 57984, 15438, 0}, {"timing", 12470, 15438, 0},

	{"info", 0, 31216, 0},
	{"damage", 59638, 31216, 0}, {"damage", 59637, 31216, 0},
	{"spell", 59638, 31216, 0},
	{"timing", 59638, 31216, 0}, {"timing", 59637, 31216, 0},

	{"info", 0, 416, 0}, {"spell", 47964, 416, 0},
	{"info", 0, 510, 0}, {"spell", 31707, 510, 0},
	{"melee", 0, 19668, 0}, {"melee", 0, 19668, 63619},
}

func TestPetsFixtureHasEveryProbe(t *testing.T) {
	path := petsFixturePath()
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Skipf("no capture at %s", path)
	}
	records, err := readPetsRecords(path)
	if err != nil {
		t.Fatal(err)
	}
	have := map[petsKey]int{}
	for _, rec := range records {
		have[rec.key()]++
	}
	for _, key := range petsProbes {
		if _, ok := simPets[key.entry]; !ok {
			t.Errorf("pet %d has no sim pet to replay it on", key.entry)
		}
		if have[key] != 1 {
			t.Errorf("%s: %d records, want 1", key, have[key])
		}
	}
	if len(records) != len(petsProbes) {
		t.Errorf("%d records, TestPetfixRecords takes %d", len(records), len(petsProbes))
	}
}

// The replay has to fail a record the sim's pet doesn't match: each case moves one server number away.
func TestPetsReplayCatchesAWrongRecord(t *testing.T) {
	path := petsFixturePath()
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Skipf("no capture at %s", path)
	}
	records, err := readPetsRecords(path)
	if err != nil {
		t.Fatal(err)
	}
	find := func(key petsKey) petsRecord {
		t.Helper()
		i := slices.IndexFunc(records, func(r petsRecord) bool { return r.key() == key })
		if i < 0 {
			t.Fatalf("no record of %s", key)
		}
		return records[i]
	}
	derived := func(rec petsRecord, field string, value any) json.RawMessage {
		t.Helper()
		var m map[string]any
		if err := json.Unmarshal(rec.Derived, &m); err != nil {
			t.Fatal(err)
		}
		m[field] = value
		out, err := json.Marshal(m)
		if err != nil {
			t.Fatal(err)
		}
		return out
	}

	for _, c := range []struct {
		name  string
		key   petsKey
		check string
		move  func(*petsRecord)
	}{
		{"the old 8% image crit", petsKey{"spell", 59638, 31216, 0}, "spell crit %", func(r *petsRecord) {
			r.Derived = derived(*r, "critTaken", 8.0)
		}},
		{"an image without the class's base mana", petsKey{"info", 0, 31216, 0}, "max mana", func(r *petsRecord) {
			r.maxPower -= 8814 - 2428
		}},
		{"an elemental with a tenth more health", petsKey{"info", 0, 15438, 0}, "max health", func(r *petsRecord) {
			r.Attacker.MaxHealth *= 1.1
		}},
		{"a swing off the sheet's range", petsKey{"melee", 0, 15438, 0}, "mean swing", func(r *petsRecord) {
			// 500-660 a swing where the server rolls 700-1020
			normal := r.Outcomes["normal"]
			normal.MeanDamage *= 580.0 / 860
			normal.MeanResisted *= 580.0 / 860
			r.Outcomes = map[string]petHits{"normal": normal, "crit": r.Outcomes["crit"]}
		}},
		{"a glancing guardian", petsKey{"melee", 0, 15438, 0}, "white glancing", func(r *petsRecord) {
			r.Derived = derived(*r, "glancingBp", 2500)
		}},
		{"a fiend swinging the sheet's range", petsKey{"melee", 0, 19668, 0}, "mean swing", func(r *petsRecord) {
			// 332-492 a swing where the server rolls 391-631
			normal := r.Outcomes["normal"]
			normal.MeanDamage *= 412.0 / 511
			normal.MeanResisted *= 412.0 / 511
			r.Outcomes = map[string]petHits{"normal": normal, "crit": r.Outcomes["crit"]}
		}},
		{"a Shadowcrawl that adds nothing", petsKey{"melee", 0, 19668, 63619}, "mean swing", func(r *petsRecord) {
			// the plain 391-631 where the server rolls 15% more
			normal := r.Outcomes["normal"]
			normal.MeanDamage /= 1.15
			normal.MeanResisted /= 1.15
			r.Outcomes = map[string]petHits{"normal": normal, "crit": r.Outcomes["crit"]}
		}},
		{"a Shadowcrawl that counts twice", petsKey{"melee", 0, 19668, 63619}, "mean swing", func(r *petsRecord) {
			normal := r.Outcomes["normal"]
			normal.MeanDamage *= 1.15
			normal.MeanResisted *= 1.15
			r.Outcomes = map[string]petHits{"normal": normal, "crit": r.Outcomes["crit"]}
		}},
		{"a Fire Shield that never misses", petsKey{"spell", 13376, 15438, 0}, "rolled miss", func(r *petsRecord) {
			r.HitResults = map[string]counted{"none": {Count: r.Iterations}}
		}},
		{"a dearer Fire Nova", petsKey{"timing", 12470, 15438, 0}, "cost", func(r *petsRecord) {
			cast := *r.cast
			cast.Cost.Amount += 10
			r.cast = &cast
		}},
	} {
		t.Run(c.name, func(t *testing.T) {
			rec := find(c.key)
			c.move(&rec)
			failed := false
			for _, check := range checkPetsRecord(rec) {
				switch {
				case check.Name == c.check && !check.Passed:
					failed = true
				case !check.Passed:
					t.Errorf("%s fails too: %s", check.Name, check.Detail)
				}
			}
			if !failed {
				t.Errorf("%s still passes", c.check)
			}
		})
	}
}
