package dps

import (
	"fmt"
	"slices"
	"testing"

	_ "github.com/wowsims/wotlk/sim/common" // imported to get item effects included.
	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
)

func init() {
	RegisterDpsWarrior()
}

func TestFury(t *testing.T) {
	core.RunTestSuite(t, t.Name(), core.JoinTestGenerators(
		core.FullCharacterTestSuiteGenerator(core.CharacterSuiteConfig{
			Class:      proto.Class_ClassWarrior,
			Race:       proto.Race_RaceOrc,
			OtherRaces: []proto.Race{proto.Race_RaceHuman},

			Talents:     FuryTalents,
			Glyphs:      FuryGlyphs,
			GearSet:     core.GetGearSet("../../../ui/warrior/gear_sets", "p1_fury"),
			Consumes:    FullConsumes,
			SpecOptions: core.SpecOptionsCombo{Label: "Basic", SpecOptions: PlayerOptionsFury},
			Rotation:    core.GetAplRotation("../../../ui/warrior/apls", "fury"),

			ItemFilter: core.ItemFilter{
				ArmorType: proto.ArmorType_ArmorTypePlate,

				WeaponTypes: []proto.WeaponType{
					proto.WeaponType_WeaponTypeAxe,
					proto.WeaponType_WeaponTypeSword,
					proto.WeaponType_WeaponTypeMace,
					proto.WeaponType_WeaponTypeDagger,
					proto.WeaponType_WeaponTypeFist,
				},
			},
		}),
		&core.SingleDpsTestGenerator{Name: "Talents-SwordSpecialization-Average", Request: swordSpecializationTalentsRequest()},
	))
}

func TestArms(t *testing.T) {
	core.RunTestSuite(t, t.Name(), core.FullCharacterTestSuiteGenerator(core.CharacterSuiteConfig{
		Class:      proto.Class_ClassWarrior,
		Race:       proto.Race_RaceOrc,
		OtherRaces: []proto.Race{proto.Race_RaceHuman},

		Talents:     ArmsTalents,
		Glyphs:      ArmsGlyphs,
		GearSet:     core.GetGearSet("../../../ui/warrior/gear_sets", "p1_arms"),
		Consumes:    FullConsumes,
		SpecOptions: core.SpecOptionsCombo{Label: "Basic", SpecOptions: PlayerOptionsArms},
		Rotation:    core.GetAplRotation("../../../ui/warrior/apls", "arms"),

		ItemFilter: core.ItemFilter{
			ArmorType: proto.ArmorType_ArmorTypePlate,

			WeaponTypes: []proto.WeaponType{
				proto.WeaponType_WeaponTypeAxe,
				proto.WeaponType_WeaponTypeSword,
				proto.WeaponType_WeaponTypeMace,
				proto.WeaponType_WeaponTypeDagger,
				proto.WeaponType_WeaponTypeFist,
			},
		},
	}))
}

func BenchmarkSimulate(b *testing.B) {
	rsr := &proto.RaidSimRequest{
		Raid: core.SinglePlayerRaidProto(
			&proto.Player{
				Race:          proto.Race_RaceOrc,
				Class:         proto.Class_ClassWarrior,
				Equipment:     core.GetGearSet("../../../ui/warrior/gear_sets", "p1_fury").GearSet,
				Consumes:      FullConsumes,
				Spec:          PlayerOptionsFury,
				TalentsString: FuryTalents,
				Buffs:         core.FullIndividualBuffs,
				Rotation:      core.GetAplRotation("../../../ui/warrior/apls", "fury").Rotation,
			},
			core.FullPartyBuffs,
			core.FullRaidBuffs,
			core.FullDebuffs),
		Encounter: &proto.Encounter{
			Duration: 300,
			Targets: []*proto.Target{
				core.NewDefaultTarget(),
			},
		},
	}

	core.RaidBenchmarkIterations(b, rsr, 1, 100)
}

var FuryTalents = "302023102331-305053000520310053120500351"
var FuryGlyphs = &proto.Glyphs{
	Major1: int32(proto.WarriorMajorGlyph_GlyphOfWhirlwind),
	Major2: int32(proto.WarriorMajorGlyph_GlyphOfHeroicStrike),
	Major3: int32(proto.WarriorMajorGlyph_GlyphOfRending),
}
var ArmsTalents = "3022032023335100102012213231251-305-2033"
var ArmsGlyphs = &proto.Glyphs{
	Major1: int32(proto.WarriorMajorGlyph_GlyphOfRending),
	Major2: int32(proto.WarriorMajorGlyph_GlyphOfMortalStrike),
	Major3: int32(proto.WarriorMajorGlyph_GlyphOfExecution),
}

var PlayerOptionsArms = &proto.Player_Warrior{
	Warrior: &proto.Warrior{
		Options: warriorOptions,
	},
}

var PlayerOptionsFury = &proto.Player_Warrior{
	Warrior: &proto.Warrior{
		Options: warriorOptions,
	},
}

var warriorOptions = &proto.Warrior_Options{
	StartingRage:       50,
	UseRecklessness:    true,
	UseShatteringThrow: true,
	Shout:              proto.WarriorShout_WarriorShoutBattle,
}

var FullConsumes = &proto.Consumes{
	Flask:         proto.Flask_FlaskOfEndlessRage,
	DefaultPotion: proto.Potions_PotionOfSpeed,
	PrepopPotion:  proto.Potions_PotionOfSpeed,
	Food:          proto.Food_FoodFishFeast,
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

// Anger Management, Sword Specialization, Mace Specialization and Improved Berserker Rage, which no other
// build takes, on TestFury's default player with a one-handed sword and mace
func swordSpecializationTalentsRequest() *proto.RaidSimRequest {
	gear := core.GetGearSet("../../../ui/warrior/gear_sets", "p1_fury").GearSet
	gear.Items[proto.ItemSlot_ItemSlotMainHand] = &proto.ItemSpec{Id: 40407, Enchant: 3789} // Silent Crusader
	gear.Items[proto.ItemSlot_ItemSlotOffHand] = &proto.ItemSpec{Id: 40189, Enchant: 3789}  // Angry Dread
	return &proto.RaidSimRequest{
		Raid: core.SinglePlayerRaidProto(
			&proto.Player{
				Race:               proto.Race_RaceOrc,
				Class:              proto.Class_ClassWarrior,
				Equipment:          gear,
				Consumes:           FullConsumes,
				Spec:               PlayerOptionsFury,
				Buffs:              core.FullIndividualBuffs,
				Rotation:           withActionsBefore(core.GetAplRotation("../../../ui/warrior/apls", "fury").Rotation, 2687, berserkerRageActions),
				TalentsString:      SwordSpecializationTalents,
				Glyphs:             FuryGlyphs,
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

// FuryTalents as 31 Arms and 40 Fury, without Titan's Grip, Bloodsurge, Unending Fury, Improved Berserker
// Stance or Two-Handed Weapon Specialization
var SwordSpecializationTalents = "30220321230200551-30505300052031025312"

// Berserker Rage on cooldown at 60 rage or less, the fury APL only casts it before the pull
const berserkerRageActions = `{"action":{"condition":{"cmp":{"op":"OpLe","lhs":{"currentRage":{}},"rhs":{"const":{"val":"60"}}}},"castSpell":{"spellId":{"spellId":18499}}}}`
