package healing

import (
	"testing"

	_ "github.com/wowsims/wotlk/sim/common" // imported to get caster sets included.
	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
)

func init() {
	RegisterHealingPriest()
}

func TestDisc(t *testing.T) {
	core.RunTestSuite(t, t.Name(), core.FullCharacterTestSuiteGenerator(core.CharacterSuiteConfig{
		Class:    proto.Class_ClassPriest,
		Race:     proto.Race_RaceUndead,
		IsHealer: true,

		GearSet:     core.GetGearSet("../../../ui/healing_priest/gear_sets", "p1_disc"),
		Talents:     DiscTalents,
		Glyphs:      DiscGlyphs,
		Consumes:    FullConsumes,
		SpecOptions: core.SpecOptionsCombo{Label: "Disc", SpecOptions: PlayerOptionsDisc},
		Rotation:    core.GetAplRotation("../../../ui/healing_priest/apls", "disc"),

		ItemFilter: core.ItemFilter{
			WeaponTypes: []proto.WeaponType{
				proto.WeaponType_WeaponTypeDagger,
				proto.WeaponType_WeaponTypeMace,
				proto.WeaponType_WeaponTypeOffHand,
				proto.WeaponType_WeaponTypeStaff,
			},
			ArmorType: proto.ArmorType_ArmorTypeCloth,
			RangedWeaponTypes: []proto.RangedWeaponType{
				proto.RangedWeaponType_RangedWeaponTypeWand,
			},
		},

		EPReferenceStat: proto.Stat_StatSpellPower,
		StatsToWeigh: []proto.Stat{
			proto.Stat_StatIntellect,
			proto.Stat_StatSpellPower,
			proto.Stat_StatSpellHaste,
			proto.Stat_StatSpellCrit,
		},
	}))
}

func TestHoly(t *testing.T) {
	core.RunTestSuite(t, t.Name(), core.JoinTestGenerators(
		core.FullCharacterTestSuiteGenerator(core.CharacterSuiteConfig{
			Class:    proto.Class_ClassPriest,
			Race:     proto.Race_RaceUndead,
			IsHealer: true,

			GearSet:     core.GetGearSet("../../../ui/healing_priest/gear_sets", "p1_holy"),
			Talents:     HolyTalents,
			Glyphs:      HolyGlyphs,
			Consumes:    FullConsumes,
			SpecOptions: core.SpecOptionsCombo{Label: "Holy", SpecOptions: PlayerOptionsHoly},
			Rotation:    core.GetAplRotation("../../../ui/healing_priest/apls", "holy"),

			ItemFilter: core.ItemFilter{
				WeaponTypes: []proto.WeaponType{
					proto.WeaponType_WeaponTypeDagger,
					proto.WeaponType_WeaponTypeMace,
					proto.WeaponType_WeaponTypeOffHand,
					proto.WeaponType_WeaponTypeStaff,
				},
				ArmorType: proto.ArmorType_ArmorTypeCloth,
				RangedWeaponTypes: []proto.RangedWeaponType{
					proto.RangedWeaponType_RangedWeaponTypeWand,
				},
			},
		}),
		&core.SingleDpsTestGenerator{Name: "Talents-BlessedResilience-Average", Request: blessedResilienceTalentsRequest()},
	))
}

// Blessed Resilience, which no other build takes, on TestHoly's default player. The holy APL's heals show
// its healing bonus.
func blessedResilienceTalentsRequest() *proto.RaidSimRequest {
	rsr := &proto.RaidSimRequest{
		Raid: core.SinglePlayerRaidProto(
			&proto.Player{
				Race:               proto.Race_RaceUndead,
				Class:              proto.Class_ClassPriest,
				Equipment:          core.GetGearSet("../../../ui/healing_priest/gear_sets", "p1_holy").GearSet,
				Consumes:           FullConsumes,
				Spec:               PlayerOptionsHoly,
				Buffs:              core.FullIndividualBuffs,
				Rotation:           core.GetAplRotation("../../../ui/healing_priest/apls", "holy").Rotation,
				TalentsString:      BlessedResilienceTalents,
				Glyphs:             HolyGlyphs,
				Profession1:        proto.Profession_Engineering,
				DistanceFromTarget: 30,
				ReactionTimeMs:     150,
				ChannelClipDelayMs: 50,
			},
			core.FullPartyBuffs,
			core.FullRaidBuffs,
			core.FullDebuffs),
		Encounter:  core.MakeSingleTargetEncounter(5),
		SimOptions: core.AverageDefaultSimTestOptions,
	}
	rsr.Raid.TargetDummies = 1
	return rsr
}

// HolyTalents with Serendipity's points in Blessed Resilience
var BlessedResilienceTalents = "05032031103-234051032002152530304011051"

var DiscTalents = "0503203130300512301313231251-2351010303"
var DiscGlyphs = &proto.Glyphs{
	Major1: int32(proto.PriestMajorGlyph_GlyphOfPowerWordShield),
	Major2: int32(proto.PriestMajorGlyph_GlyphOfFlashHeal),
	Major3: int32(proto.PriestMajorGlyph_GlyphOfPenance),
	// No interesting minor glyphs.
}

var HolyTalents = "05032031103-234051032002152530004311051"
var HolyGlyphs = &proto.Glyphs{
	Major1: int32(proto.PriestMajorGlyph_GlyphOfPrayerOfHealing),
	Major2: int32(proto.PriestMajorGlyph_GlyphOfRenew),
	Major3: int32(proto.PriestMajorGlyph_GlyphOfCircleOfHealing),
	// No interesting minor glyphs.
}

var FullConsumes = &proto.Consumes{
	Flask:         proto.Flask_FlaskOfTheFrostWyrm,
	Food:          proto.Food_FoodFishFeast,
	DefaultPotion: proto.Potions_RunicManaInjector,
	PrepopPotion:  proto.Potions_PotionOfWildMagic,
}

var PlayerOptionsDisc = &proto.Player_HealingPriest{
	HealingPriest: &proto.HealingPriest{
		Options: &proto.HealingPriest_Options{
			UseInnerFire:      true,
			UseShadowfiend:    true,
			RapturesPerMinute: 5,
		},
	},
}

var PlayerOptionsHoly = &proto.Player_HealingPriest{
	HealingPriest: &proto.HealingPriest{
		Options: &proto.HealingPriest_Options{
			UseInnerFire:   true,
			UseShadowfiend: true,
		},
	},
}
