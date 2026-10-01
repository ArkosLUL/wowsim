package tank

import (
	"testing"

	_ "github.com/wowsims/wotlk/sim/common" // imported to get item effects included.
	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
	"github.com/wowsims/wotlk/sim/encounters"
)

func init() {
	RegisterTankDeathknight()
}

func TestBloodTank(t *testing.T) {
	core.RunTestSuite(t, t.Name(), core.JoinTestGenerators(
		core.FullCharacterTestSuiteGenerator(core.CharacterSuiteConfig{
			Class:      proto.Class_ClassDeathknight,
			Race:       proto.Race_RaceOrc,
			OtherRaces: []proto.Race{proto.Race_RaceHuman},

			GearSet:     core.GetGearSet("../../../ui/tank_deathknight/gear_sets", "p1_blood"),
			Talents:     BloodTankTalents,
			Glyphs:      Glyphs,
			Consumes:    FullConsumes,
			SpecOptions: core.SpecOptionsCombo{Label: "Basic", SpecOptions: PlayerOptionsBloodTank},
			Rotation:    core.GetAplRotation("../../../ui/tank_deathknight/apls", "blood_icy_touch"),

			IsTank:          true,
			InFrontOfTarget: true,

			ItemFilter: core.ItemFilter{
				ArmorType: proto.ArmorType_ArmorTypePlate,

				WeaponTypes: []proto.WeaponType{
					proto.WeaponType_WeaponTypeAxe,
					proto.WeaponType_WeaponTypeSword,
					proto.WeaponType_WeaponTypeMace,
				},
			},
		}),
		&core.SingleDpsTestGenerator{Name: "GenericBoss", Request: genericBossRequest()},
		&core.SingleDpsTestGenerator{Name: "BloodAggro", Request: bloodAggroRequest()},
	))
}

// blood_aggro casts Dancing Rune Weapon and Heart Strike, which BloodTankTalents doesn't take, so it runs
// on the UI's Blood Aggro build, otherwise as the suite's default player
func bloodAggroRequest() *proto.RaidSimRequest {
	rsr := &proto.RaidSimRequest{
		Raid: core.SinglePlayerRaidProto(
			&proto.Player{
				Race:               proto.Race_RaceOrc,
				Class:              proto.Class_ClassDeathknight,
				Equipment:          core.GetGearSet("../../../ui/tank_deathknight/gear_sets", "p1_blood").GearSet,
				Consumes:           FullConsumes,
				Spec:               PlayerOptionsBloodTank,
				Buffs:              core.FullIndividualBuffs,
				Rotation:           core.GetAplRotation("../../../ui/tank_deathknight/apls", "blood_aggro").Rotation,
				TalentsString:      BloodAggroTalents,
				Glyphs:             BloodAggroGlyphs,
				Profession1:        proto.Profession_Engineering,
				InFrontOfTarget:    true,
				DistanceFromTarget: 30,
				ReactionTimeMs:     150,
				ChannelClipDelayMs: 50,
			},
			core.FullPartyBuffs,
			core.FullRaidBuffs,
			core.FullDebuffs),
		Encounter:  core.MakeSingleTargetEncounter(0),
		SimOptions: core.DefaultSimTestOptions,
	}
	rsr.Raid.Tanks = []*proto.UnitReference{{Type: proto.UnitReference_Player, Index: 0}}
	return rsr
}

// The generic AzerothCore boss, on the same gear and rotation as TestBloodTank's default, alongside
// the Classic-style target that suite still keeps.
func genericBossRequest() *proto.RaidSimRequest {
	rsr := &proto.RaidSimRequest{
		Raid: core.SinglePlayerRaidProto(
			&proto.Player{
				Race:            proto.Race_RaceOrc,
				Class:           proto.Class_ClassDeathknight,
				Equipment:       core.GetGearSet("../../../ui/tank_deathknight/gear_sets", "p1_blood").GearSet,
				Consumes:        FullConsumes,
				Spec:            PlayerOptionsBloodTank,
				Buffs:           core.FullIndividualBuffs,
				Rotation:        core.GetAplRotation("../../../ui/tank_deathknight/apls", "blood_icy_touch").Rotation,
				TalentsString:   BloodTankTalents,
				Glyphs:          Glyphs,
				InFrontOfTarget: true,
			},
			core.FullPartyBuffs,
			core.FullRaidBuffs,
			core.FullDebuffs),
		Encounter: &proto.Encounter{
			Duration: 300,
			Targets:  []*proto.Target{encounters.GenericBossTarget(encounters.GenericBossDamageModifier)},
		},
		SimOptions: core.DefaultSimTestOptions,
	}
	rsr.Raid.Tanks = []*proto.UnitReference{{Type: proto.UnitReference_Player, Index: 0}}
	return rsr
}

var BloodTankTalents = "005510153330330220102013-3050505100023101-002"
var Glyphs = &proto.Glyphs{
	Major1: int32(proto.DeathknightMajorGlyph_GlyphOfDarkCommand),
	Major2: int32(proto.DeathknightMajorGlyph_GlyphOfObliterate),
	Major3: int32(proto.DeathknightMajorGlyph_GlyphOfVampiricBlood),
}

var BloodAggroTalents = "0355220530303303201020131301--0052003050032"
var BloodAggroGlyphs = &proto.Glyphs{
	Major1: int32(proto.DeathknightMajorGlyph_GlyphOfDancingRuneWeapon),
	Major2: int32(proto.DeathknightMajorGlyph_GlyphOfRuneStrike),
	Major3: int32(proto.DeathknightMajorGlyph_GlyphOfDarkCommand),
}

var PlayerOptionsBloodTank = &proto.Player_TankDeathknight{
	TankDeathknight: &proto.TankDeathknight{
		Options: &proto.TankDeathknight_Options{
			StartingRunicPower: 0,
		},
	},
}

var FullRaidBuffs = &proto.RaidBuffs{
	GiftOfTheWild:         proto.TristateEffect_TristateEffectImproved,
	PowerWordFortitude:    proto.TristateEffect_TristateEffectImproved,
	AbominationsMight:     true,
	SwiftRetribution:      true,
	Bloodlust:             true,
	StrengthOfEarthTotem:  proto.TristateEffect_TristateEffectImproved,
	LeaderOfThePack:       proto.TristateEffect_TristateEffectImproved,
	SanctifiedRetribution: true,
	DevotionAura:          proto.TristateEffect_TristateEffectImproved,
	RetributionAura:       true,
	IcyTalons:             true,
}
var FullPartyBuffs = &proto.PartyBuffs{
	HeroicPresence: true,
}
var FullIndividualBuffs = &proto.IndividualBuffs{
	BlessingOfKings:     true,
	BlessingOfMight:     proto.TristateEffect_TristateEffectImproved,
	BlessingOfSanctuary: true,
}

var FullConsumes = &proto.Consumes{
	Flask:         proto.Flask_FlaskOfStoneblood,
	DefaultPotion: proto.Potions_IndestructiblePotion,
	PrepopPotion:  proto.Potions_IndestructiblePotion,
	Food:          proto.Food_FoodDragonfinFilet,
}

var FullDebuffs = &proto.Debuffs{
	SunderArmor:        true,
	Mangle:             true,
	DemoralizingShout:  proto.TristateEffect_TristateEffectImproved,
	JudgementOfLight:   true,
	FaerieFire:         proto.TristateEffect_TristateEffectRegular,
	Misery:             true,
	FrostFever:         proto.TristateEffect_TristateEffectImproved,
	BloodFrenzy:        true,
	EbonPlaguebringer:  true,
	HeartOfTheCrusader: true,
}
