package mage

import (
	"testing"

	_ "github.com/wowsims/wotlk/sim/common"
	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
)

func init() {
	RegisterMage()
}

func TestArcane(t *testing.T) {
	core.RunTestSuite(t, t.Name(), core.JoinTestGenerators(
		core.FullCharacterTestSuiteGenerator(core.CharacterSuiteConfig{
			Class: proto.Class_ClassMage,
			Race:  proto.Race_RaceTroll,

			GearSet:     core.GetGearSet("../../ui/mage/gear_sets", "p3_arcane_alliance"),
			Talents:     ArcaneTalents,
			Glyphs:      ArcaneGlyphs,
			Consumes:    FullArcaneConsumes,
			SpecOptions: core.SpecOptionsCombo{Label: "Arcane", SpecOptions: PlayerOptionsArcane},
			Rotation:    core.GetAplRotation("../../ui/mage/apls", "arcane"),
			OtherRotations: []core.RotationCombo{
				core.GetAplRotation("../../ui/mage/apls", "arcane_aoe"),
			},

			ItemFilter: ItemFilter,
		}),
		&core.SingleDpsTestGenerator{Name: "Talents-ArcaneBarrage-Average", Request: arcaneBarrageTalentsRequest()},
	))
}

func TestFire(t *testing.T) {
	core.RunTestSuite(t, t.Name(), core.JoinTestGenerators(
		core.FullCharacterTestSuiteGenerator(core.CharacterSuiteConfig{
			Class: proto.Class_ClassMage,
			Race:  proto.Race_RaceTroll,

			GearSet:     core.GetGearSet("../../ui/mage/gear_sets", "p3_fire_alliance"),
			Talents:     FireTalents,
			Glyphs:      FireGlyphs,
			Consumes:    FullFireConsumes,
			SpecOptions: core.SpecOptionsCombo{Label: "Fire", SpecOptions: PlayerOptionsFire},
			Rotation:    core.GetAplRotation("../../ui/mage/apls", "fire"),
			OtherRotations: []core.RotationCombo{
				core.GetAplRotation("../../ui/mage/apls", "fire_aoe"),
			},

			ItemFilter: ItemFilter,
		}),
		&core.SingleDpsTestGenerator{Name: "Talents-Firestarter-Average", Request: firestarterTalentsRequest()},
	))
}

// talentsAverageRequest runs player the way Average-Default runs a suite's default player
func talentsAverageRequest(player *proto.Player) *proto.RaidSimRequest {
	return &proto.RaidSimRequest{
		Raid:       core.SinglePlayerRaidProto(player, core.FullPartyBuffs, core.FullRaidBuffs, core.FullDebuffs),
		Encounter:  core.MakeSingleTargetEncounter(5),
		SimOptions: core.AverageDefaultSimTestOptions,
	}
}

// Arcane Barrage, which no other build takes, on TestArcane's default player
func arcaneBarrageTalentsRequest() *proto.RaidSimRequest {
	return talentsAverageRequest(&proto.Player{
		Race:               proto.Race_RaceTroll,
		Class:              proto.Class_ClassMage,
		Equipment:          core.GetGearSet("../../ui/mage/gear_sets", "p3_arcane_alliance").GearSet,
		Consumes:           FullArcaneConsumes,
		Spec:               PlayerOptionsArcane,
		Buffs:              core.FullIndividualBuffs,
		Rotation:           core.APLRotationFromJsonString(arcaneBarrageTalentsAPL),
		TalentsString:      ArcaneBarrageTalents,
		Glyphs:             ArcaneGlyphs,
		Profession1:        proto.Profession_Engineering,
		DistanceFromTarget: 30,
		ReactionTimeMs:     150,
		ChannelClipDelayMs: 50,
	})
}

// Firestarter and Improved Fire Blast, plus Permafrost, Improved Blizzard and Cold as Ice in 27 Frost
// points, which no other build takes, on TestFire's default player
func firestarterTalentsRequest() *proto.RaidSimRequest {
	return talentsAverageRequest(&proto.Player{
		Race:               proto.Race_RaceTroll,
		Class:              proto.Class_ClassMage,
		Equipment:          core.GetGearSet("../../ui/mage/gear_sets", "p3_fire_alliance").GearSet,
		Consumes:           FullFireConsumes,
		Spec:               PlayerOptionsFire,
		Buffs:              core.FullIndividualBuffs,
		Rotation:           core.APLRotationFromJsonString(firestarterTalentsAPL),
		TalentsString:      FirestarterTalents,
		Glyphs:             FirestarterGlyphs,
		Profession1:        proto.Profession_Engineering,
		DistanceFromTarget: 30,
		ReactionTimeMs:     150,
		ChannelClipDelayMs: 50,
	})
}

// ArcaneTalents with Magic Attunement's point in Arcane Barrage
var ArcaneBarrageTalents = "230005033100330150323102505321-03-023303001"

// arcane, with Arcane Barrage at four Arcane Blast stacks when Missile Barrage hasn't procced
const arcaneBarrageTalentsAPL = `{"type":"TypeAPL",
"prepullActions":[
	{"action":{"castSpell":{"spellId":{"spellId":55342}}},"doAtValue":{"const":{"val":"-2s"}}},
	{"action":{"castSpell":{"spellId":{"otherId":"OtherActionPotion"}}},"doAtValue":{"const":{"val":"-1s"}}}
],
"priorityList":[
	{"action":{"autocastOtherCooldowns":{}}},
	{"action":{"condition":{"not":{"val":{"auraIsActive":{"auraId":{"spellId":12472}}}}},"castSpell":{"spellId":{"spellId":26297}}}},
	{"action":{"condition":{"not":{"val":{"auraIsActive":{"auraId":{"spellId":12472}}}}},"castSpell":{"spellId":{"spellId":54758}}}},
	{"action":{"condition":{"not":{"val":{"auraIsActive":{"auraId":{"spellId":12472}}}}},"castSpell":{"spellId":{"otherId":"OtherActionPotion"}}}},
	{"action":{"condition":{"cmp":{"op":"OpLt","lhs":{"auraNumStacks":{"auraId":{"spellId":36032}}},"rhs":{"const":{"val":"4"}}}},"castSpell":{"spellId":{"spellId":42897}}}},
	{"action":{"condition":{"auraIsActiveWithReactionTime":{"auraId":{"spellId":44401}}},"castSpell":{"spellId":{"spellId":42846}}}},
	{"action":{"castSpell":{"spellId":{"spellId":44781}}}},
	{"action":{"condition":{"cmp":{"op":"OpLe","lhs":{"currentManaPercent":{}},"rhs":{"const":{"val":"25%"}}}},"castSpell":{"spellId":{"spellId":12051}}}},
	{"action":{"condition":{"cmp":{"op":"OpGt","lhs":{"currentManaPercent":{}},"rhs":{"const":{"val":"25%"}}}},"castSpell":{"spellId":{"spellId":42897}}}},
	{"action":{"castSpell":{"spellId":{"spellId":42846}}}}
]}`

var FirestarterTalents = "-2355030010303331050120121-02330333130301002"
var FirestarterGlyphs = &proto.Glyphs{
	Major1: int32(proto.MageMajorGlyph_GlyphOfFireball),
	Major2: int32(proto.MageMajorGlyph_GlyphOfMoltenArmor),
	Major3: int32(proto.MageMajorGlyph_GlyphOfMirrorImage),
}

// Firestarter's Flamestrike after Dragon's Breath or Blast Wave, Blizzard about once a minute, Fire Blast
// on cooldown
const firestarterTalentsAPL = `{"type":"TypeAPL",
"prepullActions":[
	{"action":{"castSpell":{"spellId":{"spellId":55342}}},"doAtValue":{"const":{"val":"-2s"}}},
	{"action":{"castSpell":{"spellId":{"otherId":"OtherActionPotion"}}},"doAtValue":{"const":{"val":"-1s"}}}
],
"priorityList":[
	{"action":{"autocastOtherCooldowns":{}}},
	{"action":{"condition":{"auraShouldRefresh":{"auraId":{"spellId":12873},"maxOverlap":{"const":{"val":"4s"}}}},"castSpell":{"spellId":{"spellId":42859}}}},
	{"action":{"condition":{"auraIsActive":{"auraId":{"spellId":54741}}},"castSpell":{"spellId":{"spellId":42926,"tag":9}}}},
	{"action":{"castSpell":{"spellId":{"spellId":42950}}}},
	{"action":{"castSpell":{"spellId":{"spellId":42945}}}},
	{"action":{"condition":{"cmp":{"op":"OpLt","lhs":{"spellCpm":{"spellId":{"spellId":42940}}},"rhs":{"const":{"val":"1"}}}},"castSpell":{"spellId":{"spellId":42940}}}},
	{"action":{"castSpell":{"spellId":{"spellId":42873}}}},
	{"action":{"castSpell":{"spellId":{"spellId":42833}}}}
]}`

func TestFrostFire(t *testing.T) {
	core.RunTestSuite(t, t.Name(), core.FullCharacterTestSuiteGenerator(core.CharacterSuiteConfig{
		Class: proto.Class_ClassMage,
		Race:  proto.Race_RaceTroll,

		GearSet:     core.GetGearSet("../../ui/mage/gear_sets", "p3_ffb_alliance"),
		Talents:     FrostFireTalents,
		Glyphs:      FrostFireGlyphs,
		Consumes:    FullFireConsumes,
		SpecOptions: core.SpecOptionsCombo{Label: "Fire", SpecOptions: PlayerOptionsFire},
		Rotation:    core.GetAplRotation("../../ui/mage/apls", "frostfire"),

		ItemFilter: ItemFilter,
	}))
}

func TestFrost(t *testing.T) {
	core.RunTestSuite(t, t.Name(), core.FullCharacterTestSuiteGenerator(core.CharacterSuiteConfig{
		Class: proto.Class_ClassMage,
		Race:  proto.Race_RaceTroll,

		GearSet:     core.GetGearSet("../../ui/mage/gear_sets", "p3_frost_alliance"),
		Talents:     FrostTalents,
		Glyphs:      FrostGlyphs,
		Consumes:    FullFrostConsumes,
		SpecOptions: core.SpecOptionsCombo{Label: "Frost", SpecOptions: PlayerOptionsFrost},
		Rotation:    core.GetAplRotation("../../ui/mage/apls", "frost"),
		OtherRotations: []core.RotationCombo{
			core.GetAplRotation("../../ui/mage/apls", "frost_aoe"),
		},

		ItemFilter: ItemFilter,
	}))
}

var ItemFilter = core.ItemFilter{
	WeaponTypes: []proto.WeaponType{
		proto.WeaponType_WeaponTypeDagger,
		proto.WeaponType_WeaponTypeSword,
		proto.WeaponType_WeaponTypeOffHand,
		proto.WeaponType_WeaponTypeStaff,
	},
	ArmorType: proto.ArmorType_ArmorTypeCloth,
	RangedWeaponTypes: []proto.RangedWeaponType{
		proto.RangedWeaponType_RangedWeaponTypeWand,
	},
}

var ArcaneTalents = "23000513310033015032310250532-03-023303001"
var FireTalents = "23000503110003-0055030012303331053120301351"
var FrostFireTalents = "23000503110003-0055030012303331053120301351"
var FrostTalents = "23000503110003--0533030310233100030152231351"
var ArcaneGlyphs = &proto.Glyphs{
	Major1: int32(proto.MageMajorGlyph_GlyphOfArcaneBlast),
	Major2: int32(proto.MageMajorGlyph_GlyphOfArcaneMissiles),
	Major3: int32(proto.MageMajorGlyph_GlyphOfMoltenArmor),
}
var FireGlyphs = &proto.Glyphs{
	Major1: int32(proto.MageMajorGlyph_GlyphOfFireball),
	Major2: int32(proto.MageMajorGlyph_GlyphOfMoltenArmor),
	Major3: int32(proto.MageMajorGlyph_GlyphOfLivingBomb),
}
var FrostFireGlyphs = &proto.Glyphs{
	Major1: int32(proto.MageMajorGlyph_GlyphOfFrostfire),
	Major2: int32(proto.MageMajorGlyph_GlyphOfMoltenArmor),
	Major3: int32(proto.MageMajorGlyph_GlyphOfLivingBomb),
}
var FrostGlyphs = &proto.Glyphs{
	Major1: int32(proto.MageMajorGlyph_GlyphOfFrostbolt),
	Major3: int32(proto.MageMajorGlyph_GlyphOfEternalWater),
	Major2: int32(proto.MageMajorGlyph_GlyphOfMoltenArmor),
}

var PlayerOptionsFire = &proto.Player_Mage{
	Mage: &proto.Mage{
		Options: &proto.Mage_Options{
			Armor: proto.Mage_Options_MoltenArmor,
		},
	},
}

var PlayerOptionsFrost = &proto.Player_Mage{
	Mage: &proto.Mage{
		Options: &proto.Mage_Options{
			Armor: proto.Mage_Options_MageArmor,
		},
	},
}

var PlayerOptionsArcane = &proto.Player_Mage{
	Mage: &proto.Mage{
		Options: &proto.Mage_Options{
			Armor: proto.Mage_Options_MoltenArmor,
		},
	},
}

var FullFireConsumes = &proto.Consumes{
	Flask:         proto.Flask_FlaskOfTheFrostWyrm,
	Food:          proto.Food_FoodFirecrackerSalmon,
	DefaultPotion: proto.Potions_PotionOfSpeed,
}
var FullFrostConsumes = FullFireConsumes

var FullArcaneConsumes = FullFireConsumes
