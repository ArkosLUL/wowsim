package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wowsims/wotlk/sim/core"
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

// No armor, no attack power and a 10 to 12 weapon leave the rolls 10, 11 and 12: the server truncates
// the range and rounds armor up, so the 0.7 glance floors 7.7 to 7 and 8.4 to 8, and a level 83
// creature blocks 41.
func TestWhiteDamageModelOnHandCheckedRolls(t *testing.T) {
	body := func(normal, glancing float64) record {
		return spellRecord(t, fmt.Sprintf(`{
			"command": "melee",
			"attacker": {"level": 80, "attacks": [{"type": "mainhand", "unhastedMs": 2000, "hastedMs": 2000, "damageMin": 10, "damageMax": 12}]},
			"target": {"level": 83, "resistances": [0]},
			"outcomes": {
				"normal": {"count": 1000, "meanDamage": %v},
				"crit": {"count": 1000, "meanDamage": 22},
				"glancing": {"count": 1000, "meanDamage": %v},
				"block": {"count": 1000, "meanDamage": 0}
			}
		}`, normal, glancing))
	}

	for _, check := range whiteDamageChecks(body(11, 7.0+1.0/3), 0, 2) {
		if !check.Passed {
			t.Errorf("%s: %s", check.Name, check.Detail)
		}
	}
	failed := 0
	for _, check := range whiteDamageChecks(body(12, 7.0+1.0/3), 0, 2) {
		if !check.Passed {
			failed++
		}
	}
	if failed != 1 {
		t.Errorf("a normal mean of 12 should fail only the normal check, %d failed", failed)
	}
}

func TestNakedNeedsABareCharacter(t *testing.T) {
	bare := func() unitSnapshot {
		var u unitSnapshot
		u.IsPlayer = true
		u.Sheet = &struct {
			CritMainhand float64   `json:"critMainhand"`
			SpellCrit    []float64 `json:"spellCrit"`
			Dodge        float64   `json:"dodge"`
			Parry        float64   `json:"parry"`
			Block        float64   `json:"block"`
			RealDodge    float64   `json:"realDodge"`
		}{}
		return u
	}
	if !bare().naked() {
		t.Fatal("a player with nothing equipped, no rating and no aura isn't naked")
	}

	rated := bare()
	rated.Ratings = append(rated.Ratings, struct {
		Name   string  `json:"name"`
		Rating float64 `json:"rating"`
		Bonus  float64 `json:"bonus"`
	}{Name: "hitMelee", Rating: 8})
	if rated.naked() {
		t.Error("a hit rating from gear still counts as naked")
	}

	buffed := bare()
	buffed.Auras = []aura{{ID: 71557, DurationMs: 20000}, {ID: 21156, DurationMs: -1}}
	if !buffed.naked() {
		t.Error("the base stats test's rating aura and a passive don't make a character geared")
	}
	buffed.Auras = append(buffed.Auras, aura{ID: 12345, DurationMs: 20000})
	if buffed.naked() {
		t.Error("an unknown timed buff still counts as naked")
	}
}

func TestCritCapable(t *testing.T) {
	for _, c := range []struct {
		id      int32
		capable bool
	}{
		{47450, true},  // Heroic Strike: weapon damage
		{49895, false}, // Death Coil cast: dummy effect
		{47465, false}, // Rend: a periodic aura alone
	} {
		if got, known := critCapable(c.id); !known || got != c.capable {
			t.Errorf("spell %d: capable %v known %v, want %v", c.id, got, known, c.capable)
		}
	}
	if capable, known := critCapable(1); known || !capable {
		t.Errorf("an unknown id is assumed capable, got %v known %v", capable, known)
	}
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

// A positive spell never rolls the table against the dummies, so the server counted no miss at all.
func TestCheckSpellSkipsTheRolledMissOfAPositiveSpellOnADummy(t *testing.T) {
	rec := spellRecord(t, `{
		"command": "spell",
		"iterations": 300000,
		"attacker": {"level": 80, "levelForOther": 80, "isPlayer": true, "hitMods": {"spell": 0}},
		"target": {"entry": 999000, "level": 83, "levelForOther": 83},
		"spell": {"id": 49895, "name": "Death Coil", "schoolMask": 32, "positive": true},
		"hitResults": {"miss": {"count": 0}},
		"derived": {"magic": {"missThreshold": 1700}, "partialResistsApply": true}
	}`)

	checks := CheckRecord(rec)
	if check := namedCheck(t, checks, "spell miss threshold"); !check.Passed {
		t.Errorf("spell miss threshold: %s", check.Detail)
	}
	for _, check := range checks {
		if check.Name == "rolled spell miss" {
			t.Errorf("rolled spell miss was checked: %s", check.Detail)
		}
	}
}

// A ranged class spell with no ranged weapon (a judgement) rolls with level*5 skill, though the
// snapshot's ranged weapon skill is 0.
func TestCheckSpellRollsARangedClassSpellWithoutAWeaponAtLevelSkill(t *testing.T) {
	const body = `{
		"command": "spell",
		"iterations": 300000,
		"attacker": {"level": 80, "levelForOther": 80, "maxSkillForOther": 400, "isPlayer": true,
			"attacks": [{"type": "ranged", "weaponSkillVsOther": 0}]},
		"target": {"level": 83, "levelForOther": 83, "maxSkillForOther": 415, "defenseSkillVsOther": 415,
			"attacks": [{"type": "ranged", "missChanceTaken": 5}]},
		"spell": {"id": 53408, "name": "Judgement of Wisdom", "schoolMask": 2, "dmgClass": 3, "attackType": "mainhand"},
		"derived": {"yellow": {"attackType": "ranged", "missBp": %d}}
	}`

	rec := spellRecord(t, fmt.Sprintf(body, 0))
	attack := rec.Attacker.attack("ranged")
	opts := core.YellowOptions{Ranged: true, NoDodge: true, NoParry: true}
	missAt := func(skill int32) int32 {
		attack.WeaponSkillVsOther = skill
		return core.YellowMeleeTableBP(meleeInput(rec, attack), opts).Miss
	}
	if missAt(0) == missAt(400) {
		t.Fatal("the weapon skill doesn't move the miss chance, so this test can't tell the two apart")
	}

	rec = spellRecord(t, fmt.Sprintf(body, missAt(400)))
	checks := CheckRecord(rec)
	if check := namedCheck(t, checks, "yellow miss"); !check.Passed {
		t.Errorf("yellow miss: %s", check.Detail)
	}
	for _, check := range checks {
		if check.Name == "table attack type" {
			t.Errorf("table attack type: %s", check.Detail)
		}
	}
}

// A spell that can't crit reports neither a done nor a taken crit chance, so there is nothing to suppress.
func TestCheckSpellExpectsNoCritSuppressionOnASpellThatCannotCrit(t *testing.T) {
	rec := spellRecord(t, `{
		"command": "yellow",
		"iterations": 300000,
		"attacker": {"level": 80, "levelForOther": 80, "maxSkillForOther": 400, "isPlayer": true,
			"attacks": [{"type": "mainhand", "weaponSkillVsOther": 400}]},
		"target": {"level": 83, "levelForOther": 83, "maxSkillForOther": 415, "defenseSkillVsOther": 415},
		"spell": {"id": 47465, "name": "Rend", "schoolMask": 1, "dmgClass": 2, "attackType": "mainhand"},
		"derived": {"yellow": {"alwaysHit": true}, "critDone": 0, "critTaken": 0}
	}`)

	if check := namedCheck(t, CheckRecord(rec), "crit suppression"); !check.Passed {
		t.Errorf("crit suppression: %s", check.Detail)
	}
}
