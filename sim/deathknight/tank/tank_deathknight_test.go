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
		&core.SingleDpsTestGenerator{Name: "Talents-FrostPresence-Average", Request: frostPresenceTalentsRequest()},
		&core.SingleDpsTestGenerator{Name: "Talents-UnholyPresence-Average", Request: unholyPresenceTalentsRequest()},
	))
}

// Deathchill also buffs Frost Strike's hits, which a DK without the talent never registers.
func TestDeathchillWithoutFrostStrike(t *testing.T) {
	rsr := frostPresenceTalentsRequest()
	player := rsr.Raid.Parties[0].Players[0]
	player.TalentsString = "005500153330131-30505051002033112300203100" // FrostPresenceTalents without Frost Strike
	rsr.SimOptions = &proto.SimOptions{Iterations: 1, IsTest: true, RandomSeed: 101}

	result := core.RunRaidSim(rsr)
	if result.ErrorResult != "" {
		t.Fatal(result.ErrorResult)
	}
	for _, action := range result.RaidMetrics.Parties[0].Players[0].Actions {
		if action.Id.GetSpellId() == 49796 {
			return
		}
	}
	t.Error("Deathchill never cast")
}

// talentsAverageRequest runs player the way Average-Default runs the suite's default player
func talentsAverageRequest(player *proto.Player) *proto.RaidSimRequest {
	rsr := &proto.RaidSimRequest{
		Raid:       core.SinglePlayerRaidProto(player, core.FullPartyBuffs, core.FullRaidBuffs, core.FullDebuffs),
		Encounter:  core.MakeSingleTargetEncounter(5),
		SimOptions: core.AverageDefaultSimTestOptions,
	}
	rsr.Raid.Tanks = []*proto.UnitReference{{Type: proto.UnitReference_Player, Index: 0}}
	return rsr
}

// Mark of Blood, Deathchill and Improved Frost Presence, which no other build takes, in Frost Presence
func frostPresenceTalentsRequest() *proto.RaidSimRequest {
	return talentsAverageRequest(&proto.Player{
		Race:               proto.Race_RaceOrc,
		Class:              proto.Class_ClassDeathknight,
		Equipment:          core.GetGearSet("../../../ui/tank_deathknight/gear_sets", "p1_blood").GearSet,
		Consumes:           FullConsumes,
		Spec:               PlayerOptionsBloodTank,
		Buffs:              core.FullIndividualBuffs,
		Rotation:           core.APLRotationFromJsonString(frostPresenceTalentsAPL),
		TalentsString:      FrostPresenceTalents,
		Glyphs:             FrostPresenceGlyphs,
		Profession1:        proto.Profession_Engineering,
		InFrontOfTarget:    true,
		DistanceFromTarget: 30,
		ReactionTimeMs:     150,
		ChannelClipDelayMs: 50,
	})
}

// Improved Blood Presence, Outbreak, Reaping and Improved Unholy Presence, which no other build takes, in
// Unholy Presence: Improved Unholy Presence only shortens rune cooldowns there
func unholyPresenceTalentsRequest() *proto.RaidSimRequest {
	return talentsAverageRequest(&proto.Player{
		Race:               proto.Race_RaceOrc,
		Class:              proto.Class_ClassDeathknight,
		Equipment:          core.GetGearSet("../../../ui/tank_deathknight/gear_sets", "p1_blood").GearSet,
		Consumes:           FullConsumes,
		Spec:               PlayerOptionsBloodTank,
		Buffs:              core.FullIndividualBuffs,
		Rotation:           core.APLRotationFromJsonString(unholyPresenceTalentsAPL),
		TalentsString:      UnholyPresenceTalents,
		Glyphs:             UnholyPresenceGlyphs,
		Profession1:        proto.Profession_Engineering,
		InFrontOfTarget:    true,
		DistanceFromTarget: 30,
		ReactionTimeMs:     150,
		ChannelClipDelayMs: 50,
	})
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

var FrostPresenceTalents = "005500153330131-30505051002033112300203101"
var FrostPresenceGlyphs = &proto.Glyphs{
	Major1: int32(proto.DeathknightMajorGlyph_GlyphOfDarkCommand),
	Major2: int32(proto.DeathknightMajorGlyph_GlyphOfFrostStrike),
	Major3: int32(proto.DeathknightMajorGlyph_GlyphOfIcyTouch),
}

// blood_icy_touch without Vampiric Blood, plus Mark of Blood, Deathchill and Frost Strike
const frostPresenceTalentsAPL = `{"type":"TypeAPL",
"prepullActions":[
	{"action":{"castSpell":{"spellId":{"spellId":48263}}},"doAtValue":{"const":{"val":"-10s"}}},
	{"action":{"castSpell":{"spellId":{"spellId":42650}}},"doAtValue":{"const":{"val":"-6s"}}},
	{"action":{"castSpell":{"spellId":{"otherId":"OtherActionPotion"}}},"doAtValue":{"const":{"val":"-1s"}}}
],
"priorityList":[
	{"action":{"autocastOtherCooldowns":{}}},
	{"action":{"condition":{"cmp":{"op":"OpLe","lhs":{"currentHealthPercent":{}},"rhs":{"const":{"val":"40%"}}}},"castSpell":{"spellId":{"spellId":48792}}}},
	{"action":{"condition":{"cmp":{"op":"OpLe","lhs":{"currentHealthPercent":{}},"rhs":{"const":{"val":"60%"}}}},"castSpell":{"spellId":{"spellId":48982}}}},
	{"action":{"condition":{"cmp":{"op":"OpLe","lhs":{"currentHealthPercent":{}},"rhs":{"const":{"val":"60%"}}}},"castSpell":{"spellId":{"spellId":48707}}}},
	{"action":{"condition":{"cmp":{"op":"OpLe","lhs":{"currentHealthPercent":{}},"rhs":{"const":{"val":"60%"}}}},"castSpell":{"spellId":{"spellId":48743}}}},
	{"action":{"castSpell":{"spellId":{"spellId":49005}}}},
	{"action":{"castSpell":{"spellId":{"spellId":49796}}}},
	{"action":{"condition":{"cmp":{"op":"OpGe","lhs":{"currentRunicPower":{}},"rhs":{"const":{"val":"40"}}}},"castSpell":{"spellId":{"spellId":56815}}}},
	{"action":{"condition":{"not":{"val":{"dotIsActive":{"spellId":{"spellId":55095}}}}},"castSpell":{"spellId":{"spellId":49909}}}},
	{"action":{"condition":{"not":{"val":{"dotIsActive":{"spellId":{"spellId":55078}}}}},"castSpell":{"spellId":{"tag":1,"spellId":49921}}}},
	{"action":{"condition":{"cmp":{"op":"OpLe","lhs":{"dotRemainingTime":{"spellId":{"spellId":55078}}},"rhs":{"const":{"val":"3s"}}}},"castSpell":{"spellId":{"spellId":50842}}}},
	{"action":{"condition":{"and":{"vals":[{"cmp":{"op":"OpGt","lhs":{"currentNonDeathRuneCount":{"runeType":"RuneFrost"}},"rhs":{"const":{"val":"0"}}}},{"cmp":{"op":"OpGt","lhs":{"currentNonDeathRuneCount":{"runeType":"RuneUnholy"}},"rhs":{"const":{"val":"0"}}}}]}},"castSpell":{"spellId":{"tag":1,"spellId":49924}}}},
	{"action":{"condition":{"cmp":{"op":"OpGt","lhs":{"currentRuneCount":{"runeType":"RuneDeath"}},"rhs":{"const":{"val":"0"}}}},"castSpell":{"spellId":{"spellId":49909}}}},
	{"action":{"condition":{"or":{"vals":[{"cmp":{"op":"OpGt","lhs":{"currentNonDeathRuneCount":{"runeType":"RuneBlood"}},"rhs":{"const":{"val":"1"}}}},{"spellIsReady":{"spellId":{"spellId":47568}}}]}},"castSpell":{"spellId":{"tag":1,"spellId":49930}}}},
	{"action":{"condition":{"cmp":{"op":"OpGe","lhs":{"currentRunicPower":{}},"rhs":{"const":{"val":"60"}}}},"castSpell":{"spellId":{"tag":1,"spellId":55268}}}},
	{"action":{"castSpell":{"spellId":{"spellId":46584}}}},
	{"action":{"castSpell":{"spellId":{"spellId":47568}}}},
	{"action":{"condition":{"cmp":{"op":"OpGe","lhs":{"currentRunicPower":{}},"rhs":{"const":{"val":"80"}}}},"castSpell":{"spellId":{"spellId":49895}}}}
]}`

var UnholyPresenceTalents = "00550015333023000002--23523013000301520030002031"
var UnholyPresenceGlyphs = &proto.Glyphs{
	Major1: int32(proto.DeathknightMajorGlyph_GlyphOfDarkCommand),
	Major2: int32(proto.DeathknightMajorGlyph_GlyphOfPlagueStrike),
	Major3: int32(proto.DeathknightMajorGlyph_GlyphOfBoneShield),
}

// blood_icy_touch in Unholy Presence, without Vampiric Blood, plus Bone Shield
const unholyPresenceTalentsAPL = `{"type":"TypeAPL",
"prepullActions":[
	{"action":{"castSpell":{"spellId":{"spellId":49222}}},"doAtValue":{"const":{"val":"-20s"}}},
	{"action":{"castSpell":{"spellId":{"spellId":48265}}},"doAtValue":{"const":{"val":"-10s"}}},
	{"action":{"castSpell":{"spellId":{"spellId":42650}}},"doAtValue":{"const":{"val":"-6s"}}},
	{"action":{"castSpell":{"spellId":{"otherId":"OtherActionPotion"}}},"doAtValue":{"const":{"val":"-1s"}}}
],
"priorityList":[
	{"action":{"autocastOtherCooldowns":{}}},
	{"action":{"condition":{"cmp":{"op":"OpLe","lhs":{"currentHealthPercent":{}},"rhs":{"const":{"val":"40%"}}}},"castSpell":{"spellId":{"spellId":48792}}}},
	{"action":{"condition":{"cmp":{"op":"OpLe","lhs":{"currentHealthPercent":{}},"rhs":{"const":{"val":"60%"}}}},"castSpell":{"spellId":{"spellId":48982}}}},
	{"action":{"condition":{"cmp":{"op":"OpLe","lhs":{"currentHealthPercent":{}},"rhs":{"const":{"val":"60%"}}}},"castSpell":{"spellId":{"spellId":48707}}}},
	{"action":{"condition":{"cmp":{"op":"OpLe","lhs":{"currentHealthPercent":{}},"rhs":{"const":{"val":"60%"}}}},"castSpell":{"spellId":{"spellId":48743}}}},
	{"action":{"condition":{"not":{"val":{"auraIsActive":{"auraId":{"spellId":49222}}}}},"castSpell":{"spellId":{"spellId":49222}}}},
	{"action":{"condition":{"cmp":{"op":"OpGe","lhs":{"currentRunicPower":{}},"rhs":{"const":{"val":"40"}}}},"castSpell":{"spellId":{"spellId":56815}}}},
	{"action":{"condition":{"not":{"val":{"dotIsActive":{"spellId":{"spellId":55095}}}}},"castSpell":{"spellId":{"spellId":49909}}}},
	{"action":{"condition":{"not":{"val":{"dotIsActive":{"spellId":{"spellId":55078}}}}},"castSpell":{"spellId":{"tag":1,"spellId":49921}}}},
	{"action":{"condition":{"cmp":{"op":"OpLe","lhs":{"dotRemainingTime":{"spellId":{"spellId":55078}}},"rhs":{"const":{"val":"3s"}}}},"castSpell":{"spellId":{"spellId":50842}}}},
	{"action":{"condition":{"and":{"vals":[{"cmp":{"op":"OpGt","lhs":{"currentNonDeathRuneCount":{"runeType":"RuneFrost"}},"rhs":{"const":{"val":"0"}}}},{"cmp":{"op":"OpGt","lhs":{"currentNonDeathRuneCount":{"runeType":"RuneUnholy"}},"rhs":{"const":{"val":"0"}}}}]}},"castSpell":{"spellId":{"tag":1,"spellId":49924}}}},
	{"action":{"condition":{"cmp":{"op":"OpGt","lhs":{"currentRuneCount":{"runeType":"RuneDeath"}},"rhs":{"const":{"val":"0"}}}},"castSpell":{"spellId":{"spellId":49909}}}},
	{"action":{"condition":{"or":{"vals":[{"cmp":{"op":"OpGt","lhs":{"currentNonDeathRuneCount":{"runeType":"RuneBlood"}},"rhs":{"const":{"val":"1"}}}},{"spellIsReady":{"spellId":{"spellId":47568}}}]}},"castSpell":{"spellId":{"tag":1,"spellId":49930}}}},
	{"action":{"castSpell":{"spellId":{"spellId":46584}}}},
	{"action":{"castSpell":{"spellId":{"spellId":47568}}}},
	{"action":{"condition":{"cmp":{"op":"OpGe","lhs":{"currentRunicPower":{}},"rhs":{"const":{"val":"80"}}}},"castSpell":{"spellId":{"spellId":49895}}}}
]}`

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
