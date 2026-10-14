package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// SPELL_ATTR2_CANT_CRIT, in Attributes[2].
const attr2CantCrit = 0x20000000

// loadCritCapable reads each spell's critCapable out of the live server's spell capture:
// SpellInfo::IsCritCapable, and false for a CANT_CRIT spell too.
var loadCritCapable = sync.OnceValues(func() (map[int32]bool, error) {
	root, err := checkoutRoot()
	if err != nil {
		return nil, err
	}
	f, err := os.Open(filepath.Join(root, spelldumpPath))
	if err != nil {
		return nil, err
	}
	defer f.Close()

	capable := map[int32]bool{}
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64<<10), 8<<20)
	for scanner.Scan() {
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}
		var spell struct {
			ID          int32 `json:"id"`
			CritCapable *bool `json:"critCapable"`
		}
		if err := json.Unmarshal(line, &spell); err != nil {
			return nil, fmt.Errorf("%s: %w", spelldumpPath, err)
		}
		if spell.CritCapable == nil {
			return nil, fmt.Errorf("%s: spell %d has no critCapable", spelldumpPath, spell.ID)
		}
		capable[spell.ID] = *spell.CritCapable
	}
	return capable, scanner.Err()
})

// critCapable reports whether the server rolls a crit for the spell at all. known is false for an id
// the capture lacks, which is then assumed capable.
func critCapable(spellID int32) (capable, known bool) {
	spells, err := loadCritCapable()
	if err != nil {
		panic(err)
	}
	capable, known = spells[spellID]
	if !known {
		return true, false
	}
	return capable, true
}
