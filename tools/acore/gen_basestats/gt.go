package main

import (
	"fmt"
	"math"
	"os"
	"path/filepath"

	"github.com/wowsims/wotlk/tools/database/azerothcore"
)

// DBCStructure.h: gt tables hold GT_MAX_LEVEL rows per class or rating, and the class rating scalars
// GT_MAX_RATING per class.
const (
	gtMaxLevel  = 100
	gtMaxRating = 32
	maxClassID  = 11
)

// gtTables are the game tables the server reads player stats from, at one level.
type gtTables struct {
	// Rating needed for 1% (or 1 skill point), per CombatRating.
	combatRatings []float32
	// classScalars[classID][cr] multiplies a rating's effect for that class.
	classScalars map[int][]float32

	meleeCritBase, meleeCritPerAgi map[int]float32
	spellCritBase, spellCritPerInt map[int]float32
	regenMPPerSpirit               map[int]float32
}

func loadGT(dir string, level, numRatings int) (*gtTables, error) {
	open := func(name string, fields int) (*azerothcore.DBCFile, error) {
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			return nil, err
		}
		f, err := azerothcore.ParseDBC(data)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", name, err)
		}
		if f.FieldCount != fields {
			return nil, fmt.Errorf("%s: %d fields, expected %d", name, f.FieldCount, fields)
		}
		return f, nil
	}
	float := func(f *azerothcore.DBCFile, row, field int) float32 {
		return math.Float32frombits(f.Uint32(row, field))
	}
	// Tables without an id column are indexed by row, like the server's LookupEntry on them.
	byRow := func(f *azerothcore.DBCFile, name string, row int) (float32, error) {
		if row < 0 || row >= f.RecordCount {
			return 0, fmt.Errorf("%s: no row %d", name, row)
		}
		return float(f, row, 0), nil
	}

	gt := &gtTables{
		classScalars:     map[int][]float32{},
		meleeCritBase:    map[int]float32{},
		meleeCritPerAgi:  map[int]float32{},
		spellCritBase:    map[int]float32{},
		spellCritPerInt:  map[int]float32{},
		regenMPPerSpirit: map[int]float32{},
	}

	ratings, err := open("gtCombatRatings.dbc", 1)
	if err != nil {
		return nil, err
	}
	for cr := 0; cr < numRatings; cr++ {
		v, err := byRow(ratings, "gtCombatRatings", cr*gtMaxLevel+level-1)
		if err != nil {
			return nil, err
		}
		gt.combatRatings = append(gt.combatRatings, v)
	}

	// Keyed by its id column: (class-1)*GT_MAX_RATING + cr + 1 (Player::GetRatingMultiplier).
	scalars, err := open("gtOCTClassCombatRatingScalar.dbc", 2)
	if err != nil {
		return nil, err
	}
	scalarByID := map[uint32]float32{}
	for row := 0; row < scalars.RecordCount; row++ {
		scalarByID[scalars.Uint32(row, 0)] = float(scalars, row, 1)
	}

	perClass := []struct {
		file string
		out  map[int]float32
		row  func(classID int) int
	}{
		{"gtChanceToMeleeCritBase.dbc", gt.meleeCritBase, func(c int) int { return c - 1 }},
		{"gtChanceToMeleeCrit.dbc", gt.meleeCritPerAgi, func(c int) int { return (c-1)*gtMaxLevel + level - 1 }},
		{"gtChanceToSpellCritBase.dbc", gt.spellCritBase, func(c int) int { return c - 1 }},
		{"gtChanceToSpellCrit.dbc", gt.spellCritPerInt, func(c int) int { return (c-1)*gtMaxLevel + level - 1 }},
		{"gtRegenMPPerSpt.dbc", gt.regenMPPerSpirit, func(c int) int { return (c-1)*gtMaxLevel + level - 1 }},
	}
	for _, t := range perClass {
		f, err := open(t.file, 1)
		if err != nil {
			return nil, err
		}
		for _, c := range classes {
			v, err := byRow(f, t.file, t.row(c.serverID))
			if err != nil {
				return nil, err
			}
			t.out[c.serverID] = v
		}
	}

	for _, c := range classes {
		row := make([]float32, numRatings)
		for cr := range row {
			v, ok := scalarByID[uint32((c.serverID-1)*gtMaxRating+cr+1)]
			if !ok {
				return nil, fmt.Errorf("gtOCTClassCombatRatingScalar: no entry for class %d rating %d", c.serverID, cr)
			}
			row[cr] = v
		}
		gt.classScalars[c.serverID] = row
	}
	return gt, nil
}
