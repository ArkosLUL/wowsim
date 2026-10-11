package main

import (
	"path/filepath"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
)

// preset is a golden suite's character: the config its test hands core.FullCharacterTestSuiteGenerator, so
// the rows built from it are the suite's own. The suites keep theirs in _test.go files, which nothing can
// import, so each one is copied here and TestPresetsRunTheirSuitesRows holds the copy to the golden.
type preset struct {
	// the golden file without its extension, from the checkout's root
	suite string
	// the spec's ui directory, which holds its gear_sets and apls
	ui     string
	config func(root string) core.CharacterSuiteConfig
}

var presets = map[string]preset{
	"blood": {"sim/deathknight/dps/TestBlood", "ui/deathknight", func(root string) core.CharacterSuiteConfig {
		return deathKnight(root, "p3_blood", "2305120530003303231023001351--230220305003", &proto.Glyphs{
			Major1: int32(proto.DeathknightMajorGlyph_GlyphOfDancingRuneWeapon),
			Major2: int32(proto.DeathknightMajorGlyph_GlyphOfDeathStrike),
			Major3: int32(proto.DeathknightMajorGlyph_GlyphOfDisease),
		}, nil, "blood_dps")
	}},
	"unholy": {"sim/deathknight/dps/TestUnholy", "ui/deathknight", func(root string) core.CharacterSuiteConfig {
		return deathKnight(root, "p3_uh_dw", "-320033500002-2300303050032152000150013133151", &proto.Glyphs{
			Major1: int32(proto.DeathknightMajorGlyph_GlyphOfTheGhoul),
			Major2: int32(proto.DeathknightMajorGlyph_GlyphOfDarkDeath),
			Major3: int32(proto.DeathknightMajorGlyph_GlyphOfDeathAndDecay),
		}, nil, "uh_2h_ss", "uh_dnd_aoe", "unholy_dw_ss")
	}},
	"frost": {"sim/deathknight/dps/TestFrost", "ui/deathknight", func(root string) core.CharacterSuiteConfig {
		return deathKnight(root, "p3_frost", "23050005-32005350352203012300033101351", frostGlyphs, []string{"Desync"},
			"frost_bl_pesti", "frost_uh_pesti")
	}},
	"frostuh": {"sim/deathknight/dps/TestFrostUH", "ui/deathknight", func(root string) core.CharacterSuiteConfig {
		return deathKnight(root, "p3_frost", "01-32002350342203012300033101351-230200305003", frostGlyphs, nil,
			"frost_uh_pesti")
	}},
}

var frostGlyphs = &proto.Glyphs{
	Major1: int32(proto.DeathknightMajorGlyph_GlyphOfFrostStrike),
	Major2: int32(proto.DeathknightMajorGlyph_GlyphOfObliterate),
	Major3: int32(proto.DeathknightMajorGlyph_GlyphOfDisease),
}

// deathKnight is a sim/deathknight/dps suite's config: its gear set, talents, glyphs, the labels of its other
// spec options (all the same options) and its rotations, the default first.
func deathKnight(root, gear, talents string, glyphs *proto.Glyphs, otherOptions []string, rotations ...string) core.CharacterSuiteConfig {
	options := func(label string) core.SpecOptionsCombo {
		return core.SpecOptionsCombo{Label: label, SpecOptions: &proto.Player_Deathknight{Deathknight: &proto.Deathknight{
			Options: &proto.Deathknight_Options{
				UnholyFrenzyTarget: &proto.UnitReference{Type: proto.UnitReference_Player, Index: 0},
				DrwPestiApply:      true,
				StartingRunicPower: 0,
				PetUptime:          1,
			},
		}}}
	}
	config := core.CharacterSuiteConfig{
		Class:      proto.Class_ClassDeathknight,
		Race:       proto.Race_RaceOrc,
		OtherRaces: []proto.Race{proto.Race_RaceHuman},

		GearSet: core.GetGearSet(filepath.Join(root, "ui/deathknight/gear_sets"), gear),
		Talents: talents,
		Glyphs:  glyphs,
		Consumes: &proto.Consumes{
			Flask:         proto.Flask_FlaskOfEndlessRage,
			DefaultPotion: proto.Potions_PotionOfSpeed,
			PrepopPotion:  proto.Potions_PotionOfSpeed,
			Food:          proto.Food_FoodDragonfinFilet,
		},
		SpecOptions: options("Basic"),
		Rotation:    core.GetAplRotation(filepath.Join(root, "ui/deathknight/apls"), rotations[0]),
	}
	for _, label := range otherOptions {
		config.OtherSpecOptions = append(config.OtherSpecOptions, options(label))
	}
	for _, name := range rotations[1:] {
		config.OtherRotations = append(config.OtherRotations, core.GetAplRotation(filepath.Join(root, "ui/deathknight/apls"), name))
	}
	return config
}
