package main

import (
	"fmt"
	"path/filepath"
	"slices"
	"testing"
)

// recordKey names a record by what it probed, since the bot's name changes every run: the command, the
// spell id (the record's own, or the one a procs record was asked about), the player's server class and
// the dummy's entry.
type recordKey struct {
	command string
	spell   int32
	class   uint8
	dummy   uint32
}

func (r record) key() recordKey {
	player, other := r.Attacker, r.Target
	if !player.IsPlayer {
		player, other = other, player
	}
	spell := r.Spell.ID
	if spell == 0 {
		spell = r.ProcSpellID
	}
	return recordKey{r.Command, spell, player.Class, other.Entry}
}

func (k recordKey) String() string {
	return fmt.Sprintf("%s spell %d class %d dummy %d", k.command, k.spell, k.class, k.dummy)
}

const (
	boss, humanoid, beast, level80 = 999000, 999001, 999002, 999003

	warrior, paladin, hunter, rogue, priest, deathKnight, shaman, mage, warlock, druid = 1, 2, 3, 4, 5, 6, 7, 8, 9, 11
)

// expectedRecords is what a full e2e run records. The records a probe can't be replayed for sit in
// droppedRecords instead, each with a row in the fixture README.
var expectedRecords = func() []recordKey {
	keys := []recordKey{
		{"info", 0, warrior, boss}, {"taken", 0, warrior, boss}, {"armor", 0, warrior, boss},
		{"melee", 0, warrior, humanoid}, {"melee", 0, warrior, beast}, {"melee", 0, warrior, level80},
		{"yellow", 47450, warrior, boss}, {"yellow", 47450, warrior, humanoid},
		{"spell", 42842, warrior, boss}, {"spell", 42842, warrior, level80}, {"spell", 48156, warrior, boss},
		{"procs", 47450, warrior, boss}, {"procs", 42842, warrior, boss}, {"procs", 49052, warrior, boss},
		{"yellow", 47465, warrior, boss}, {"yellow", 64382, warrior, boss},

		{"info", 0, hunter, boss}, {"yellow", 75, hunter, boss}, {"yellow", 49052, hunter, boss},
		{"procs", 49052, hunter, boss},

		{"yellow", 35395, paladin, boss}, {"yellow", 53385, paladin, boss},
		{"spell", 31803, paladin, boss}, {"spell", 53408, paladin, boss},

		{"spell", 57970, rogue, boss}, {"yellow", 48672, rogue, boss},

		{"spell", 48127, priest, boss}, {"spell", 48125, priest, boss}, {"spell", 58381, priest, boss},

		{"info", 0, deathKnight, boss}, {"yellow", 55271, deathKnight, boss}, {"yellow", 51425, deathKnight, boss},
		{"spell", 49909, deathKnight, boss}, {"spell", 49895, deathKnight, boss}, {"spell", 47632, deathKnight, boss},
		{"spell", 55095, deathKnight, boss}, {"spell", 55078, deathKnight, boss},

		{"yellow", 17364, shaman, boss}, {"yellow", 32175, shaman, boss}, {"yellow", 32176, shaman, boss},
		{"spell", 60043, shaman, boss}, {"spell", 49233, shaman, boss},

		{"spell", 42842, mage, boss}, {"spell", 42833, mage, boss}, {"spell", 55362, mage, boss},

		{"spell", 59164, warlock, boss}, {"spell", 47855, warlock, boss},

		{"spell", 48463, druid, boss}, {"spell", 48465, druid, boss}, {"yellow", 48572, druid, boss},
		{"spell", 16857, druid, boss}, {"spell", 60089, druid, boss}, {"spell", 53227, druid, boss},
		{"procs", 48463, druid, boss}, {"procs", 48572, druid, boss},

		{"info", 0, shaman, boss}, {"info", 0, warlock, boss}, // the PFU1 passives
	}

	// the avoidance talent tests: the dummy hits a tank
	for _, class := range []uint8{warrior, paladin, deathKnight, druid} {
		keys = append(keys, recordKey{"taken", 0, class, boss})
	}
	// the base stats characters, bare and then with rating auras
	for _, class := range []uint8{warrior, paladin, hunter, rogue, priest, deathKnight, shaman, mage, warlock, druid} {
		keys = append(keys, recordKey{"info", 0, class, boss}, recordKey{"armor", 0, class, boss})
	}
	// the unarmed white swings
	for _, class := range []uint8{warrior, paladin, hunter, rogue} {
		keys = append(keys, recordKey{"melee", 0, class, boss})
	}
	return keys
}()

// droppedRecords are expected records that disagree with the sim and were removed from the fixture.
var droppedRecords = []recordKey{{"melee", 0, warrior, boss}}

func TestFixtureHasEveryE2ERecord(t *testing.T) {
	records, err := readRecords(filepath.Join("..", "..", fixtureDir, "simval.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	have := map[recordKey]bool{}
	for _, rec := range records {
		have[rec.key()] = true
	}

	var missing []string
	for _, key := range expectedRecords {
		if !have[key] && !slices.Contains(droppedRecords, key) {
			missing = append(missing, key.String())
		}
	}
	slices.Sort(missing)
	missing = slices.Compact(missing)

	for _, key := range missing {
		t.Errorf("no record for %s", key)
	}
}
