package restoration

import (
	"testing"

	_ "github.com/wowsims/wotlk/sim/common"
	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
	"github.com/wowsims/wotlk/sim/core/stats"
)

func init() {
	RegisterRestorationShaman()
}

// The APL only autocasts cooldowns and never heals, so the suite checks stats and item heals only.
func TestRestoration(t *testing.T) {
	core.RunTestSuite(t, t.Name(), core.JoinTestGenerators(
		core.FullCharacterTestSuiteGenerator(core.CharacterSuiteConfig{
			Class: proto.Class_ClassShaman,
			Race:  proto.Race_RaceTroll,

			GearSet:     core.GetGearSet("../../../ui/restoration_shaman/gear_sets", "p1"),
			Talents:     StandardTalents,
			Glyphs:      StandardGlyphs,
			Consumes:    FullConsumes,
			SpecOptions: core.SpecOptionsCombo{Label: "Standard", SpecOptions: PlayerOptionsStandard},
			Rotation:    core.RotationCombo{Label: "Default", Rotation: DefaultRotation},

			ItemFilter: core.ItemFilter{
				WeaponTypes: []proto.WeaponType{
					proto.WeaponType_WeaponTypeAxe,
					proto.WeaponType_WeaponTypeDagger,
					proto.WeaponType_WeaponTypeFist,
					proto.WeaponType_WeaponTypeMace,
					proto.WeaponType_WeaponTypeOffHand,
					proto.WeaponType_WeaponTypeShield,
					proto.WeaponType_WeaponTypeStaff,
				},
				ArmorType: proto.ArmorType_ArmorTypeMail,
				RangedWeaponTypes: []proto.RangedWeaponType{
					proto.RangedWeaponType_RangedWeaponTypeTotem,
				},
			},
		}),
		&core.SingleDpsTestGenerator{Name: "Talents-HealingGrace-Average", Request: healingGraceTalentsRequest()},
	))
}

// Flametongue Totem's 58655 is a raid area aura, so another party gets Enhancing Totems' 165 too.
func TestEnhancingTotemsReachTheRaid(t *testing.T) {
	otherPartySpellPower := func(talents string) float64 {
		shaman := &proto.Player{
			Class:         proto.Class_ClassShaman,
			Race:          proto.Race_RaceTroll,
			Equipment:     &proto.EquipmentSpec{},
			Spec:          PlayerOptionsStandard,
			TalentsString: talents,
		}
		other := &proto.Player{
			Class:     proto.Class_ClassShaman,
			Race:      proto.Race_RaceTroll,
			Equipment: &proto.EquipmentSpec{},
			Spec: &proto.Player_RestorationShaman{RestorationShaman: &proto.RestorationShaman{
				Options: &proto.RestorationShaman_Options{Totems: &proto.ShamanTotems{}},
			}},
		}
		result := core.ComputeStats(&proto.ComputeStatsRequest{
			Raid: &proto.Raid{Parties: []*proto.Party{
				{Players: []*proto.Player{shaman}},
				{Players: []*proto.Player{other}},
			}},
			Encounter: core.MakeSingleTargetEncounter(0),
		})
		if result.ErrorResult != "" {
			t.Fatal(result.ErrorResult)
		}
		return result.RaidStats.Parties[1].Players[0].FinalStats.Stats[stats.SpellPower]
	}

	if got := otherPartySpellPower("-3") - otherPartySpellPower(""); got != 21 {
		t.Errorf("3 points of Enhancing Totems add %v spell power in the other party, want 21 (165 over 144)", got)
	}
}

func BenchmarkSimulate(b *testing.B) {
	rsr := &proto.RaidSimRequest{
		Raid: core.SinglePlayerRaidProto(
			&proto.Player{
				Race:          proto.Race_RaceOrc,
				Class:         proto.Class_ClassShaman,
				Equipment:     core.GetGearSet("../../../ui/restoration_shaman/gear_sets", "p1").GearSet,
				Consumes:      FullConsumes,
				Spec:          PlayerOptionsStandard,
				Buffs:         core.FullIndividualBuffs,
				TalentsString: StandardTalents,
				Glyphs:        StandardGlyphs,
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
		SimOptions: core.AverageDefaultSimTestOptions,
	}

	core.RaidBenchmark(b, rsr)
}

var StandardTalents = "-3020503-50005331335310501122331251"
var StandardGlyphs = &proto.Glyphs{
	Major1: int32(proto.ShamanMajorGlyph_GlyphOfChainHeal),
	Major2: int32(proto.ShamanMajorGlyph_GlyphOfEarthShield),
	Major3: int32(proto.ShamanMajorGlyph_GlyphOfEarthlivingWeapon),
}

var BasicTotems = &proto.ShamanTotems{
	Earth: proto.EarthTotem_TremorTotem,
	Air:   proto.AirTotem_WrathOfAirTotem,
	Water: proto.WaterTotem_ManaSpringTotem,
	Fire:  proto.FireTotem_FlametongueTotem,
}

var restoShamOptions = &proto.RestorationShaman_Options{
	Shield: proto.ShamanShield_WaterShield,
	Totems: BasicTotems,
}
var PlayerOptionsStandard = &proto.Player_RestorationShaman{
	RestorationShaman: &proto.RestorationShaman{
		Options: restoShamOptions,
	},
}

var FullConsumes = &proto.Consumes{
	Flask:           proto.Flask_FlaskOfBlindingLight,
	Food:            proto.Food_FoodBlackenedBasilisk,
	DefaultPotion:   proto.Potions_SuperManaPotion,
	PrepopPotion:    proto.Potions_DestructionPotion,
	DefaultConjured: proto.Conjured_ConjuredDarkRune,
}

var DefaultRotation = core.APLRotationFromJsonString(`{
	"type": "TypeAPL",
	"priorityList": [
		{"action":{"autocastOtherCooldowns":{}}}
	]
}`)

// Totemic Focus and Healing Grace, which no other build takes, on TestRestoration's default player healing
// a target dummy
func healingGraceTalentsRequest() *proto.RaidSimRequest {
	rsr := &proto.RaidSimRequest{
		Raid: core.SinglePlayerRaidProto(
			&proto.Player{
				Race:               proto.Race_RaceTroll,
				Class:              proto.Class_ClassShaman,
				Equipment:          core.GetGearSet("../../../ui/restoration_shaman/gear_sets", "p1").GearSet,
				Consumes:           FullConsumes,
				Spec:               PlayerOptionsStandard,
				Buffs:              core.FullIndividualBuffs,
				Rotation:           core.APLRotationFromJsonString(healingGraceTalentsAPL),
				TalentsString:      HealingGraceTalents,
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

// StandardTalents with Improved Healing Wave and Healing Focus, which this APL never uses, in Totemic Focus
// and Healing Grace
var HealingGraceTalents = "-3020503-05035301335310501122331251"

// Healing Stream Totem, then Riptide on cooldown and Chain Heal
const healingGraceTalentsAPL = `{"type":"TypeAPL",
"prepullActions":[
	{"action":{"castSpell":{"spellId":{"spellId":58757}}},"doAtValue":{"const":{"val":"-1s"}}}
],
"priorityList":[
	{"action":{"autocastOtherCooldowns":{}}},
	{"action":{"castSpell":{"spellId":{"spellId":61301}}}},
	{"action":{"castSpell":{"spellId":{"spellId":55459}}}}
]}`
