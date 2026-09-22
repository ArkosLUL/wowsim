package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The combat tables against the server's own numbers, from a recording of
// mod-sim-validation's e2e suite. Skips when there is no fixture, since capturing
// one needs a live worldserver with the module enabled: stream simval.jsonl out
// of the worldserver while e2e/run.sh runs (the suite deletes its own records on
// cleanup), then run tools/simval -fixture -records <capture>.
func TestAgainstServerRecords(t *testing.T) {
	path := filepath.Join("..", "..", fixtureDir, "simval.jsonl")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Skipf("no recording at %s; capture one with tools/simval -fixture", path)
	}

	records, err := readRecords(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) == 0 {
		t.Fatalf("%s is empty", path)
	}

	var checked int
	for _, rec := range records {
		for _, check := range CheckRecord(rec) {
			checked++
			if !check.Passed {
				t.Errorf("%s: %s: %s", rec.label(), check.Name, check.Detail)
			}
		}
	}

	if checked == 0 {
		t.Fatalf("%s has %d records but nothing to check", path, len(records))
	}
	t.Logf("%d checks over %d records", checked, len(records))
}

func spellRecord(t *testing.T, body string) record {
	t.Helper()
	var rec record
	if err := json.Unmarshal([]byte(body), &rec); err != nil {
		t.Fatal(err)
	}
	return rec
}

func namedCheck(t *testing.T, checks []Check, name string) Check {
	t.Helper()
	for _, check := range checks {
		if check.Name == name {
			return check
		}
	}
	t.Fatalf("no %q check among %d", name, len(checks))
	return Check{}
}

// ALWAYS_HIT returns before DeriveMagicTable fills anything in, so the server reports no miss
// threshold at all and the sim, which never misses with the flag either, has to expect the same.
func TestCheckSpellExpectsNoMissOnAnAlwaysHitSpell(t *testing.T) {
	rec := spellRecord(t, `{
		"command": "spell",
		"iterations": 300000,
		"attacker": {"level": 80, "levelForOther": 80, "isPlayer": true, "hitMods": {"spell": 0}},
		"target": {"level": 83, "levelForOther": 83},
		"spell": {"id": 60089, "name": "Faerie Fire (Feral)", "schoolMask": 8},
		"derived": {"magic": {"autoHit": true, "missThreshold": 0}, "partialResistsApply": true}
	}`)

	if check := namedCheck(t, CheckRecord(rec), "spell miss threshold"); !check.Passed {
		t.Errorf("spell miss threshold: %s", check.Detail)
	}
}

// A binary spell's resist goes into its hit roll instead, so every landed hit sits in the unresisted
// bucket and there is no partial distribution to compare against.
func TestCheckSpellExpectsNoPartialResistsOnABinarySpell(t *testing.T) {
	rec := spellRecord(t, `{
		"command": "spell",
		"iterations": 300000,
		"attacker": {"level": 80, "levelForOther": 80, "isPlayer": true, "hitMods": {"spell": 0}},
		"target": {"level": 83, "levelForOther": 83},
		"spell": {"id": 53227, "name": "Typhoon", "schoolMask": 8, "binary": true},
		"derived": {"magic": {"missThreshold": 1700}, "partialResistsApply": false},
		"resists": {"buckets": [300000, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0], "meanResistedFraction": 0}
	}`)

	checks := CheckRecord(rec)
	if check := namedCheck(t, checks, "no partial resists"); !check.Passed {
		t.Errorf("no partial resists: %s", check.Detail)
	}
	for _, check := range checks {
		if strings.HasSuffix(check.Name, "% resist") || check.Name == "mean resisted" {
			t.Errorf("%q: a binary spell has no partial resist distribution to check", check.Name)
		}
	}
}
