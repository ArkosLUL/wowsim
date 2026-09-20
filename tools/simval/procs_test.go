package main

import (
	"testing"
)

// A live `.simval procs` capture of the module's TestSimvalProcPPM run: a warrior
// with Judgement of Wisdom (15 PPM) and Hand of Justice (1 PPM with
// PROC_ATTR_REDUCE_PROC_60), probed for Frostbolt, Heroic Strike and Steady Shot.
// Those three pick a different PPM basis each, so the generated proc rows and the
// basis rules are both covered without a live server.
//
// The committed replay fixture carries no aura the generated tables know, which is
// why this capture is kept separately. Recapture it with
// `e2e/run.sh TestSimvalProcPPM` while streaming simval.jsonl out of the
// worldserver.
const procsFixture = "testdata/procs.jsonl"

func TestProcChancesAgainstServer(t *testing.T) {
	records, err := readRecords(procsFixture)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) == 0 {
		t.Fatalf("%s is empty", procsFixture)
	}

	var checked int
	var sawPPM, sawFlat, sawSpellBasis bool
	for _, rec := range records {
		if rec.Command != "procs" {
			t.Errorf("%s holds a %q record", procsFixture, rec.Command)
			continue
		}
		for _, check := range CheckRecord(rec) {
			checked++
			if !check.Passed {
				t.Errorf("%s: %s: %s", rec.label(), check.Name, check.Detail)
			}
		}
		for _, proc := range rec.AuraProcs {
			switch {
			case proc.ProcEntry.ProcsPerMinute > 0:
				sawPPM = true
			case proc.ProcEntry.Chance > 0:
				sawFlat = true
			}
			if _, ok := proc.Chance["spell"]; ok {
				sawSpellBasis = true
			}
		}
	}

	if checked == 0 {
		t.Fatalf("%s produced no checks", procsFixture)
	}
	if !sawPPM || !sawFlat || !sawSpellBasis {
		t.Errorf("fixture covers ppm=%v flat=%v spell basis=%v; it should cover all three",
			sawPPM, sawFlat, sawSpellBasis)
	}
	t.Logf("%d checks over %d records", checked, len(records))
}

func TestPPMBasisPicksTheWeaponOrTheCast(t *testing.T) {
	records, err := readRecords(procsFixture)
	if err != nil {
		t.Fatal(err)
	}

	// Frostbolt is SPELL_DAMAGE_CLASS_MAGIC with a 3 s base cast, Heroic Strike is
	// melee class and Steady Shot uses the ranged slot.
	want := map[int32]int32{42842: 3000, 47450: 1900, 49052: 2000}
	seen := map[int32]bool{}
	for _, rec := range records {
		expected, ok := want[rec.ProcSpellID]
		if !ok {
			continue
		}
		seen[rec.ProcSpellID] = true
		basis, ok := ppmBasisMs(rec, "spell")
		if !ok {
			t.Errorf("spell %d has no PPM basis", rec.ProcSpellID)
			continue
		}
		if basis != expected {
			t.Errorf("spell %d PPM basis = %d ms, want %d", rec.ProcSpellID, basis, expected)
		}
	}
	for id := range want {
		if !seen[id] {
			t.Errorf("%s has no record for spell %d", procsFixture, id)
		}
	}
}
