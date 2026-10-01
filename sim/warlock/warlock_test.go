package warlock

import (
	"fmt"
	"slices"
	"testing"

	_ "github.com/wowsims/wotlk/sim/common"
	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
)

func init() {
	RegisterWarlock()
}

func TestAffliction(t *testing.T) {
	core.RunTestSuite(t, t.Name(), core.JoinTestGenerators(
		core.FullCharacterTestSuiteGenerator(core.CharacterSuiteConfig{
			Class: proto.Class_ClassWarlock,
			Race:  proto.Race_RaceOrc,

			GearSet:     core.GetGearSet("../../ui/warlock/gear_sets", "p4_affliction"),
			Talents:     AfflictionTalents,
			Glyphs:      AfflictionGlyphs,
			Consumes:    FullConsumes,
			SpecOptions: core.SpecOptionsCombo{Label: "Affliction Warlock", SpecOptions: DefaultAfflictionWarlock},
			Rotation:    core.GetAplRotation("../../ui/warlock/apls", "affliction"),

			ItemFilter: ItemFilter,
		}),
		&core.SingleDpsTestGenerator{Name: "Talents-DarkPact-Average", Request: darkPactTalentsRequest()},
	))
}

func TestDemonology(t *testing.T) {
	core.RunTestSuite(t, t.Name(), core.JoinTestGenerators(
		core.FullCharacterTestSuiteGenerator(core.CharacterSuiteConfig{
			Class: proto.Class_ClassWarlock,
			Race:  proto.Race_RaceOrc,

			GearSet:     core.GetGearSet("../../ui/warlock/gear_sets", "p4_demo"),
			Talents:     DemonologyTalents,
			Glyphs:      DemonologyGlyphs,
			Consumes:    FullConsumes,
			SpecOptions: core.SpecOptionsCombo{Label: "Demonology Warlock", SpecOptions: DefaultDemonologyWarlock},
			Rotation:    core.GetAplRotation("../../ui/warlock/apls", "demo"),

			ItemFilter: ItemFilter,
		}),
		&core.SingleDpsTestGenerator{Name: "Talents-ImprovedDemonicTactics-Average", Request: improvedDemonicTacticsTalentsRequest()},
	))
}

func TestDestruction(t *testing.T) {
	core.RunTestSuite(t, t.Name(), core.JoinTestGenerators(
		core.FullCharacterTestSuiteGenerator(core.CharacterSuiteConfig{
			Class: proto.Class_ClassWarlock,
			Race:  proto.Race_RaceOrc,

			GearSet:     core.GetGearSet("../../ui/warlock/gear_sets", "p4_destro"),
			Talents:     DestructionTalents,
			Glyphs:      DestructionGlyphs,
			Consumes:    FullConsumes,
			SpecOptions: core.SpecOptionsCombo{Label: "Destruction Warlock", SpecOptions: DefaultDestroWarlock},
			Rotation:    core.GetAplRotation("../../ui/warlock/apls", "destro"),
			ItemFilter:  ItemFilter,
		}),
		&core.SingleDpsTestGenerator{Name: "Talents-Shadowburn-Average", Request: shadowburnTalentsRequest()},
	))
}

var ItemFilter = core.ItemFilter{
	WeaponTypes: []proto.WeaponType{
		proto.WeaponType_WeaponTypeSword,
		proto.WeaponType_WeaponTypeDagger,
	},
	HandTypes: []proto.HandType{
		proto.HandType_HandTypeOffHand,
	},
	ArmorType: proto.ArmorType_ArmorTypeCloth,
	RangedWeaponTypes: []proto.RangedWeaponType{
		proto.RangedWeaponType_RangedWeaponTypeWand,
	},
}

var AfflictionTalents = "2350002030023510253500331151--550000051"
var DemonologyTalents = "-203203301035012530135201351-550000052"
var DestructionTalents = "-03310030003-05203205210331051335230351"
var AfflictionGlyphs = &proto.Glyphs{
	Major1: int32(proto.WarlockMajorGlyph_GlyphOfQuickDecay),
	Major2: int32(proto.WarlockMajorGlyph_GlyphOfLifeTap),
	Major3: int32(proto.WarlockMajorGlyph_GlyphOfHaunt),
}
var DemonologyGlyphs = &proto.Glyphs{
	Major1: int32(proto.WarlockMajorGlyph_GlyphOfQuickDecay),
	Major2: int32(proto.WarlockMajorGlyph_GlyphOfLifeTap),
	Major3: int32(proto.WarlockMajorGlyph_GlyphOfFelguard),
}
var DestructionGlyphs = &proto.Glyphs{
	Major1: int32(proto.WarlockMajorGlyph_GlyphOfConflagrate),
	Major2: int32(proto.WarlockMajorGlyph_GlyphOfLifeTap),
	Major3: int32(proto.WarlockMajorGlyph_GlyphOfIncinerate),
}

var defaultDestroOptions = &proto.Warlock_Options{
	Armor:        proto.Warlock_Options_FelArmor,
	Summon:       proto.Warlock_Options_Imp,
	WeaponImbue:  proto.Warlock_Options_GrandFirestone,
	DetonateSeed: true,
}

var DefaultDestroWarlock = &proto.Player_Warlock{
	Warlock: &proto.Warlock{
		Options: defaultDestroOptions,
	},
}

// ---------------------------------------
var DefaultAfflictionWarlock = &proto.Player_Warlock{
	Warlock: &proto.Warlock{
		Options: defaultAfflictionOptions,
	},
}

var defaultAfflictionOptions = &proto.Warlock_Options{
	Armor:        proto.Warlock_Options_FelArmor,
	Summon:       proto.Warlock_Options_Felhunter,
	WeaponImbue:  proto.Warlock_Options_GrandSpellstone,
	DetonateSeed: true,
}

// ---------------------------------------
var DefaultDemonologyWarlock = &proto.Player_Warlock{
	Warlock: &proto.Warlock{
		Options: defaultDemonologyOptions,
	},
}

var defaultDemonologyOptions = &proto.Warlock_Options{
	Armor:        proto.Warlock_Options_FelArmor,
	Summon:       proto.Warlock_Options_Felguard,
	WeaponImbue:  proto.Warlock_Options_GrandSpellstone,
	DetonateSeed: true,
}

// ---------------------------------------------------------

var FullConsumes = &proto.Consumes{
	Flask:         proto.Flask_FlaskOfTheFrostWyrm,
	DefaultPotion: proto.Potions_PotionOfWildMagic,
	PrepopPotion:  proto.Potions_PotionOfWildMagic,
	Food:          proto.Food_FoodFishFeast,
}

// talentsAverageRequest runs player the way Average-Default runs a suite's default player
func talentsAverageRequest(player *proto.Player) *proto.RaidSimRequest {
	return &proto.RaidSimRequest{
		Raid:       core.SinglePlayerRaidProto(player, core.FullPartyBuffs, core.FullRaidBuffs, core.FullDebuffs),
		Encounter:  core.MakeSingleTargetEncounter(5),
		SimOptions: core.AverageDefaultSimTestOptions,
	}
}

// withActionsBefore puts actions, APL priority list items as JSON, ahead of rotation's first cast of spellID
func withActionsBefore(rotation *proto.APLRotation, spellID int32, actions string) *proto.APLRotation {
	added := core.APLRotationFromJsonString(`{"type":"TypeAPL","priorityList":[` + actions + `]}`).PriorityList
	at := slices.IndexFunc(rotation.PriorityList, func(item *proto.APLListItem) bool {
		return item.GetAction().GetCastSpell().GetSpellId().GetSpellId() == spellID
	})
	if at < 0 {
		panic(fmt.Sprintf("the rotation never casts %d", spellID))
	}
	rotation.PriorityList = slices.Insert(rotation.PriorityList, at, added...)
	return rotation
}

// Improved Drain Soul, Improved Life Tap, Amplify Curse and Dark Pact, which no other build takes, on
// TestAffliction's default player
func darkPactTalentsRequest() *proto.RaidSimRequest {
	return talentsAverageRequest(&proto.Player{
		Race:               proto.Race_RaceOrc,
		Class:              proto.Class_ClassWarlock,
		Equipment:          core.GetGearSet("../../ui/warlock/gear_sets", "p4_affliction").GearSet,
		Consumes:           FullConsumes,
		Spec:               DefaultAfflictionWarlock,
		Buffs:              core.FullIndividualBuffs,
		Rotation:           withActionsBefore(core.GetAplRotation("../../ui/warlock/apls", "affliction").Rotation, 57946, darkPactActions),
		TalentsString:      DarkPactTalents,
		Glyphs:             AfflictionGlyphs,
		Profession1:        proto.Profession_Engineering,
		DistanceFromTarget: 30,
		ReactionTimeMs:     150,
		ChannelClipDelayMs: 50,
	})
}

// Improved Demonic Tactics, which no other build takes, on TestDemonology's default player
func improvedDemonicTacticsTalentsRequest() *proto.RaidSimRequest {
	return talentsAverageRequest(&proto.Player{
		Race:               proto.Race_RaceOrc,
		Class:              proto.Class_ClassWarlock,
		Equipment:          core.GetGearSet("../../ui/warlock/gear_sets", "p4_demo").GearSet,
		Consumes:           FullConsumes,
		Spec:               DefaultDemonologyWarlock,
		Buffs:              core.FullIndividualBuffs,
		Rotation:           core.GetAplRotation("../../ui/warlock/apls", "demo").Rotation,
		TalentsString:      ImprovedDemonicTacticsTalents,
		Glyphs:             DemonologyGlyphs,
		Profession1:        proto.Profession_Engineering,
		DistanceFromTarget: 30,
		ReactionTimeMs:     150,
		ChannelClipDelayMs: 50,
	})
}

// Shadowburn and Improved Searing Pain, which no other build takes, on TestDestruction's default player
func shadowburnTalentsRequest() *proto.RaidSimRequest {
	return talentsAverageRequest(&proto.Player{
		Race:               proto.Race_RaceOrc,
		Class:              proto.Class_ClassWarlock,
		Equipment:          core.GetGearSet("../../ui/warlock/gear_sets", "p4_destro").GearSet,
		Consumes:           FullConsumes,
		Spec:               DefaultDestroWarlock,
		Buffs:              core.FullIndividualBuffs,
		Rotation:           withActionsBefore(core.GetAplRotation("../../ui/warlock/apls", "destro").Rotation, 47838, shadowburnActions),
		TalentsString:      ShadowburnTalents,
		Glyphs:             DestructionGlyphs,
		Profession1:        proto.Profession_Engineering,
		DistanceFromTarget: 30,
		ReactionTimeMs:     150,
		ChannelClipDelayMs: 50,
	})
}

// AfflictionTalents with Fel Concentration and Intensity, which the sim doesn't read, and Soul Siphon in
// Improved Drain Soul, Improved Life Tap, Amplify Curse and Dark Pact
var DarkPactTalents = "2350220001023510253510331151--55000005"

// Dark Pact refreshes Glyph of Life Tap instead of Life Tap while the felhunter has the mana
const darkPactActions = `{"action":{"condition":{"and":{"vals":[{"cmp":{"op":"OpLt","lhs":{"auraRemainingTime":{"auraId":{"spellId":63321}}},"rhs":{"const":{"val":"2s"}}}},{"cmp":{"op":"OpGe","lhs":{"currentMana":{"sourceUnit":{"type":"Pet","owner":{"type":"Self"}}}},"rhs":{"const":{"val":"1500"}}}}]}},"castSpell":{"spellId":{"spellId":59092}}}}`

// DemonologyTalents with Intensity, which the sim doesn't read, and a point of Demonic Knowledge in Improved
// Demonic Tactics
var ImprovedDemonicTacticsTalents = "-203203301035012530125231351-55000005"

// DestructionTalents with Demonic Power and Improved Soul Leech in Shadowburn and Improved Searing Pain
var ShadowburnTalents = "-03310030003-05203015213331051335030351"

// Shadowburn on cooldown, and Searing Pain to bring Pyroclasm back, ahead of the Incinerate filler
const shadowburnActions = `{"action":{"castSpell":{"spellId":{"spellId":47827}}}},
	{"action":{"condition":{"not":{"val":{"auraIsActive":{"auraId":{"spellId":63244}}}}},"castSpell":{"spellId":{"spellId":47815}}}}`
