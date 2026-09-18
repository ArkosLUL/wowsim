package main

import (
	"os"
	"path/filepath"
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
