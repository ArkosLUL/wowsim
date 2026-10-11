package main

import (
	"github.com/wowsims/wotlk/sim/core"
	dpsDeathKnight "github.com/wowsims/wotlk/sim/deathknight/dps"
)

// preset is a golden suite's character: the config its test hands core.FullCharacterTestSuiteGenerator, so
// the rows built from it are the suite's own.
type preset struct {
	// the spec's ui directory, which holds its gear_sets and apls
	ui     string
	config func(root string) core.CharacterSuiteConfig
}

var presets = map[string]preset{
	"blood":   {"ui/deathknight", dpsDeathKnight.BloodSuite},
	"unholy":  {"ui/deathknight", dpsDeathKnight.UnholySuite},
	"frost":   {"ui/deathknight", dpsDeathKnight.FrostSuite},
	"frostuh": {"ui/deathknight", dpsDeathKnight.FrostUHSuite},
}
