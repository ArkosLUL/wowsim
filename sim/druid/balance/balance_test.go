package balance

import (
	"testing"

	_ "github.com/wowsims/wotlk/sim/common" // imported to get caster sets included. (we use spellfire here)
	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
)

func init() {
	RegisterBalanceDruid()
}

func TestBalance(t *testing.T) {
	core.RunTestSuite(t, t.Name(), core.JoinTestGenerators(
		core.FullCharacterTestSuiteGenerator(core.CharacterSuiteConfig{
			Class: proto.Class_ClassDruid,
			Race:  proto.Race_RaceTauren,

			GearSet: core.GetGearSet("../../../ui/balance_druid/gear_sets", "p1"),
			OtherGearSets: []core.GearSetCombo{
				core.GetGearSet("../../../ui/balance_druid/gear_sets", "p2"),
				core.GetGearSet("../../../ui/balance_druid/gear_sets", "p3_alliance"),
			},
			Talents:     StandardTalents,
			Glyphs:      StandardGlyphs,
			Consumes:    FullConsumes,
			SpecOptions: core.SpecOptionsCombo{Label: "Default", SpecOptions: PlayerOptionsAdaptive},
			Rotation:    core.GetAplRotation("../../../ui/balance_druid/apls", "basic_p3"),

			ItemFilter: ItemFilter,
		}),
		&core.SingleDpsTestGenerator{Name: "Talents-Dreamstate-Average", Request: dreamstateTalentsRequest()},
	))
}

// Dreamstate and Natural Perfection, which no other build takes, on TestBalance's default player. Natural
// Perfection needs 30 points in Restoration, so this moonkin has no Eclipse or Starfall.
func dreamstateTalentsRequest() *proto.RaidSimRequest {
	return &proto.RaidSimRequest{
		Raid: core.SinglePlayerRaidProto(
			&proto.Player{
				Race:               proto.Race_RaceTauren,
				Class:              proto.Class_ClassDruid,
				Equipment:          core.GetGearSet("../../../ui/balance_druid/gear_sets", "p1").GearSet,
				Consumes:           FullConsumes,
				Spec:               PlayerOptionsAdaptive,
				Buffs:              core.FullIndividualBuffs,
				Rotation:           core.APLRotationFromJsonString(dreamstateTalentsAPL),
				TalentsString:      DreamstateTalents,
				Glyphs:             DreamstateGlyphs,
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
}

var DreamstateTalents = "5012203115331033212--2050033125315000003"
var DreamstateGlyphs = &proto.Glyphs{
	Major1: int32(proto.DruidMajorGlyph_GlyphOfStarfire),
	Major2: int32(proto.DruidMajorGlyph_GlyphOfInsectSwarm),
	Major3: int32(proto.DruidMajorGlyph_GlyphOfMoonfire),
}

// basic_p3 without its Eclipse, Starfall and Force of Nature lines, with a Starfire filler. Nature's Swiftness goes
// on Wrath: its class mask leaves out Starfire.
const dreamstateTalentsAPL = `{"type":"TypeAPL",
"prepullActions":[
	{"action":{"castSpell":{"spellId":{"otherId":"OtherActionPotion"}}},"doAtValue":{"const":{"val":"-1.5s"}}},
	{"action":{"castSpell":{"spellId":{"spellId":48461}}},"doAtValue":{"const":{"val":"-1.5s"}}}
],
"priorityList":[
	{"action":{"condition":{"cmp":{"op":"OpGt","lhs":{"currentTime":{}},"rhs":{"const":{"val":"5s"}}}},"castSpell":{"spellId":{"spellId":2825,"tag":-1}}}},
	{"action":{"condition":{"cmp":{"op":"OpLt","lhs":{"remainingTime":{}},"rhs":{"const":{"val":"0.5s"}}}},"castSpell":{"spellId":{"spellId":48463}}}},
	{"action":{"autocastOtherCooldowns":{}}},
	{"action":{"condition":{"not":{"val":{"dotIsActive":{"spellId":{"spellId":48463}}}}},"castSpell":{"spellId":{"spellId":48463}}}},
	{"action":{"condition":{"not":{"val":{"dotIsActive":{"spellId":{"spellId":48468}}}}},"castSpell":{"spellId":{"spellId":48468}}}},
	{"action":{"condition":{"auraIsActive":{"auraId":{"spellId":17116}}},"castSpell":{"spellId":{"spellId":48461}}}},
	{"action":{"castSpell":{"spellId":{"spellId":48465}}}}
]}`

func TestBalancePhase3(t *testing.T) {
	core.RunTestSuite(t, t.Name(), core.FullCharacterTestSuiteGenerator(core.CharacterSuiteConfig{
		Class: proto.Class_ClassDruid,
		Race:  proto.Race_RaceTauren,

		GearSet: core.GetGearSet("../../../ui/balance_druid/gear_sets", "p3_alliance"),
		Talents: "5102233115331303213305311031--205003002",
		Glyphs: &proto.Glyphs{
			Major1: int32(proto.DruidMajorGlyph_GlyphOfStarfire),
			Major2: int32(proto.DruidMajorGlyph_GlyphOfMoonfire),
			Major3: int32(proto.DruidMajorGlyph_GlyphOfStarfall),
		},
		Consumes:    FullConsumes,
		SpecOptions: core.SpecOptionsCombo{Label: "Default", SpecOptions: PlayerOptionsAdaptive},
		Rotation:    core.GetAplRotation("../../../ui/balance_druid/apls", "basic_p3"),

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
	}))
}

var StandardTalents = "5012203105331303213315311231--205003012"
var StandardGlyphs = &proto.Glyphs{
	Major1: int32(proto.DruidMajorGlyph_GlyphOfStarfire),
	Major2: int32(proto.DruidMajorGlyph_GlyphOfInsectSwarm),
	Major3: int32(proto.DruidMajorGlyph_GlyphOfStarfall),
	Minor1: int32(proto.DruidMinorGlyph_GlyphOfTyphoon),
}

var FullConsumes = &proto.Consumes{
	Flask:         proto.Flask_FlaskOfTheFrostWyrm,
	Food:          proto.Food_FoodFishFeast,
	DefaultPotion: proto.Potions_PotionOfSpeed,
	PrepopPotion:  proto.Potions_PotionOfWildMagic,
}

var PlayerOptionsAdaptive = &proto.Player_BalanceDruid{
	BalanceDruid: &proto.BalanceDruid{
		Options: &proto.BalanceDruid_Options{
			OkfUptime: 0.2,
		},
	},
}

var ItemFilter = core.ItemFilter{
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
}
