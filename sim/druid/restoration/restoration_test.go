package restoration

import (
	"testing"

	_ "github.com/wowsims/wotlk/sim/common" // imported to get caster sets included. (we use spellfire here)
	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
)

func init() {
	RegisterRestorationDruid()
}

// The page's default APL healing a target dummy.
func TestRestoration(t *testing.T) {
	core.RunTestSuite(t, t.Name(), core.JoinTestGenerators(
		core.FullCharacterTestSuiteGenerator(core.CharacterSuiteConfig{
			Class:    proto.Class_ClassDruid,
			Race:     proto.Race_RaceTauren,
			IsHealer: true,

			GearSet:     core.GetGearSet("../../../ui/restoration_druid/gear_sets", "p1"),
			Talents:     StandardTalents,
			Glyphs:      StandardGlyphs,
			Consumes:    FullConsumes,
			SpecOptions: core.SpecOptionsCombo{Label: "Standard", SpecOptions: PlayerOptionsStandard},
			Rotation:    core.RotationCombo{Label: "Default", Rotation: DefaultRotation},

			ItemFilter: core.ItemFilter{
				WeaponTypes: []proto.WeaponType{
					proto.WeaponType_WeaponTypeDagger,
					proto.WeaponType_WeaponTypeMace,
					proto.WeaponType_WeaponTypeOffHand,
					proto.WeaponType_WeaponTypeStaff,
					proto.WeaponType_WeaponTypePolearm,
				},
				ArmorType: proto.ArmorType_ArmorTypeLeather,
				RangedWeaponTypes: []proto.RangedWeaponType{
					proto.RangedWeaponType_RangedWeaponTypeIdol,
				},
			},
		}),
		&core.SingleDpsTestGenerator{Name: "Talents-LivingSeed-Average", Request: livingSeedTalentsRequest()},
	))
}

var StandardTalents = "05320031103--230023312131502331050313051"

// StandardTalents with Improved Tree of Life's 3 points in Living Seed, which no other build takes
var LivingSeedTalents = "05320031103--230023312131502331053310051"

// TestRestoration's default player on LivingSeedTalents, healing a target dummy
func livingSeedTalentsRequest() *proto.RaidSimRequest {
	rsr := &proto.RaidSimRequest{
		Raid: core.SinglePlayerRaidProto(
			&proto.Player{
				Race:               proto.Race_RaceTauren,
				Class:              proto.Class_ClassDruid,
				Equipment:          core.GetGearSet("../../../ui/restoration_druid/gear_sets", "p1").GearSet,
				Consumes:           FullConsumes,
				Spec:               PlayerOptionsStandard,
				Buffs:              core.FullIndividualBuffs,
				Rotation:           DefaultRotation,
				TalentsString:      LivingSeedTalents,
				Glyphs:             StandardGlyphs,
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

var StandardGlyphs = &proto.Glyphs{
	Major1: int32(proto.DruidMajorGlyph_GlyphOfWildGrowth),
	Major2: int32(proto.DruidMajorGlyph_GlyphOfSwiftmend),
	Major3: int32(proto.DruidMajorGlyph_GlyphOfNourish),
}

var FullConsumes = &proto.Consumes{
	Flask:           proto.Flask_FlaskOfBlindingLight,
	Food:            proto.Food_FoodBlackenedBasilisk,
	DefaultPotion:   proto.Potions_SuperManaPotion,
	PrepopPotion:    proto.Potions_DestructionPotion,
	DefaultConjured: proto.Conjured_ConjuredDarkRune,
}

var PlayerOptionsStandard = &proto.Player_RestorationDruid{
	RestorationDruid: &proto.RestorationDruid{
		Options: &proto.RestorationDruid_Options{
			InnervateTarget: &proto.UnitReference{Type: proto.UnitReference_Player, Index: 0}, // self innervate
		},
	},
}

// The page's default APL.
var DefaultRotation = core.GetAplRotation("../../../ui/restoration_druid/apls", "default").Rotation
