package dps

import (
	"path/filepath"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
)

// The golden suites' characters. root is the checkout's root: every file a suite reads is named from
// there, so tools can build one from another directory than this package's.

func BloodSuite(root string) core.CharacterSuiteConfig {
	return core.CharacterSuiteConfig{
		Class:      proto.Class_ClassDeathknight,
		Race:       proto.Race_RaceOrc,
		OtherRaces: []proto.Race{proto.Race_RaceHuman},

		GearSet:     core.GetGearSet(filepath.Join(root, gearSetsDir), "p3_blood"),
		Talents:     BloodTalents,
		Glyphs:      BloodDefaultGlyphs,
		Consumes:    FullConsumes,
		SpecOptions: core.SpecOptionsCombo{Label: "Basic", SpecOptions: PlayerOptionsBlood},
		Rotation:    suiteRotation(root, "blood_dps"),

		OtherRotations: []core.RotationCombo{
			suiteRotation(root, "blood_pesti_aoe"),
		},

		ItemFilter: ItemFilter,
	}
}

func UnholySuite(root string) core.CharacterSuiteConfig {
	return core.CharacterSuiteConfig{
		Class:      proto.Class_ClassDeathknight,
		Race:       proto.Race_RaceOrc,
		OtherRaces: []proto.Race{proto.Race_RaceHuman},

		GearSet:     core.GetGearSet(filepath.Join(root, gearSetsDir), "p3_uh_dw"),
		Talents:     UnholyTalents,
		Glyphs:      UnholyDefaultGlyphs,
		Consumes:    FullConsumes,
		SpecOptions: core.SpecOptionsCombo{Label: "Basic", SpecOptions: PlayerOptionsUnholy},
		Rotation:    suiteRotation(root, "uh_2h_ss"),

		OtherRotations: []core.RotationCombo{
			suiteRotation(root, "uh_dnd_aoe"),
			suiteRotation(root, "unholy_dw_ss"),
		},

		ItemFilter: ItemFilter,
	}
}

func FrostSuite(root string) core.CharacterSuiteConfig {
	return core.CharacterSuiteConfig{
		Class:      proto.Class_ClassDeathknight,
		Race:       proto.Race_RaceOrc,
		OtherRaces: []proto.Race{proto.Race_RaceHuman},

		GearSet:     core.GetGearSet(filepath.Join(root, gearSetsDir), "p3_frost"),
		Talents:     FrostTalents,
		Glyphs:      FrostDefaultGlyphs,
		Consumes:    FullConsumes,
		SpecOptions: core.SpecOptionsCombo{Label: "Basic", SpecOptions: PlayerOptionsFrost},
		OtherSpecOptions: []core.SpecOptionsCombo{
			{Label: "Desync", SpecOptions: PlayerOptionsDesyncFrost},
		},
		Rotation: suiteRotation(root, "frost_bl_pesti"),

		ItemFilter: ItemFilter,
	}
}

func FrostUHSuite(root string) core.CharacterSuiteConfig {
	return core.CharacterSuiteConfig{
		Class:      proto.Class_ClassDeathknight,
		Race:       proto.Race_RaceOrc,
		OtherRaces: []proto.Race{proto.Race_RaceHuman},

		GearSet:     core.GetGearSet(filepath.Join(root, gearSetsDir), "p3_frost"),
		Talents:     FrostUHTalents,
		Glyphs:      FrostUHDefaultGlyphs,
		Consumes:    FullConsumes,
		SpecOptions: core.SpecOptionsCombo{Label: "Basic", SpecOptions: PlayerOptionsFrost},
		Rotation:    suiteRotation(root, "frost_uh_pesti"),

		ItemFilter: ItemFilter,
	}
}

// spelled out in each config: tools/simval reads the suites' gear from the source
const gearSetsDir = "ui/deathknight/gear_sets"

func suiteRotation(root, name string) core.RotationCombo {
	return core.GetAplRotation(filepath.Join(root, "ui/deathknight/apls"), name)
}

var BloodTalents = "2305120530003303231023001351--230220305003"
var BloodDefaultGlyphs = &proto.Glyphs{
	Major1: int32(proto.DeathknightMajorGlyph_GlyphOfDancingRuneWeapon),
	Major2: int32(proto.DeathknightMajorGlyph_GlyphOfDeathStrike),
	Major3: int32(proto.DeathknightMajorGlyph_GlyphOfDisease),
	// No interesting minor glyphs.
}

var FrostTalents = "23050005-32005350352203012300033101351"
var FrostDefaultGlyphs = &proto.Glyphs{
	Major1: int32(proto.DeathknightMajorGlyph_GlyphOfFrostStrike),
	Major2: int32(proto.DeathknightMajorGlyph_GlyphOfObliterate),
	Major3: int32(proto.DeathknightMajorGlyph_GlyphOfDisease),
	// No interesting minor glyphs.
}

var FrostUHTalents = "01-32002350342203012300033101351-230200305003"
var FrostUHDefaultGlyphs = &proto.Glyphs{
	Major1: int32(proto.DeathknightMajorGlyph_GlyphOfFrostStrike),
	Major2: int32(proto.DeathknightMajorGlyph_GlyphOfObliterate),
	Major3: int32(proto.DeathknightMajorGlyph_GlyphOfDisease),
	// No interesting minor glyphs.
}

var UnholyTalents = "-320033500002-2300303050032152000150013133151"
var UnholyDefaultGlyphs = &proto.Glyphs{
	Major1: int32(proto.DeathknightMajorGlyph_GlyphOfTheGhoul),
	Major2: int32(proto.DeathknightMajorGlyph_GlyphOfDarkDeath),
	Major3: int32(proto.DeathknightMajorGlyph_GlyphOfDeathAndDecay),
	// No interesting minor glyphs.
}

var PlayerOptionsBlood = &proto.Player_Deathknight{
	Deathknight: &proto.Deathknight{
		Options: deathKnightOptions,
	},
}

var PlayerOptionsUnholy = &proto.Player_Deathknight{
	Deathknight: &proto.Deathknight{
		Options: deathKnightOptions,
	},
}

var PlayerOptionsFrost = &proto.Player_Deathknight{
	Deathknight: &proto.Deathknight{
		Options: deathKnightOptions,
	},
}

var PlayerOptionsDesyncFrost = &proto.Player_Deathknight{
	Deathknight: &proto.Deathknight{
		Options: deathKnightOptions,
	},
}

var deathKnightOptions = &proto.Deathknight_Options{
	UnholyFrenzyTarget: &proto.UnitReference{Type: proto.UnitReference_Player, Index: 0},
	DrwPestiApply:      true,
	StartingRunicPower: 0,
	PetUptime:          1,
}

var FullConsumes = &proto.Consumes{
	Flask:         proto.Flask_FlaskOfEndlessRage,
	DefaultPotion: proto.Potions_PotionOfSpeed,
	PrepopPotion:  proto.Potions_PotionOfSpeed,
	Food:          proto.Food_FoodDragonfinFilet,
}

var ItemFilter = core.ItemFilter{
	ArmorType: proto.ArmorType_ArmorTypePlate,

	WeaponTypes: []proto.WeaponType{
		proto.WeaponType_WeaponTypeAxe,
		proto.WeaponType_WeaponTypeSword,
		proto.WeaponType_WeaponTypeMace,
	},
}
