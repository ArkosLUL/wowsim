package rogue

import (
	"testing"

	_ "github.com/wowsims/wotlk/sim/common" // imported to get item effects included.
	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
)

func init() {
	RegisterRogue()
}

func TestCombat(t *testing.T) {
	core.RunTestSuite(t, t.Name(), core.JoinTestGenerators(
		core.FullCharacterTestSuiteGenerator(core.CharacterSuiteConfig{
			Class:       proto.Class_ClassRogue,
			Race:        proto.Race_RaceHuman,
			OtherRaces:  []proto.Race{proto.Race_RaceOrc},
			GearSet:     core.GetGearSet("../../ui/rogue/gear_sets", "p1_combat"),
			Talents:     CombatTalents,
			Glyphs:      CombatGlyphs,
			Consumes:    FullConsumes,
			SpecOptions: core.SpecOptionsCombo{Label: "MH Deadly OH Instant", SpecOptions: PlayerOptionsCombatDI},
			OtherSpecOptions: []core.SpecOptionsCombo{
				{Label: "MH Instant OH Deadly", SpecOptions: PlayerOptionsCombatID},
				{Label: "MH Instant OH Instant", SpecOptions: PlayerOptionsCombatII},
				{Label: "MH Deadly OH Deadly", SpecOptions: PlayerOptionsCombatDD},
			},
			Rotation: core.GetAplRotation("../../ui/rogue/apls", "combat_expose"),
			OtherRotations: []core.RotationCombo{
				core.GetAplRotation("../../ui/rogue/apls", "combat_cleave_snd"),
				core.GetAplRotation("../../ui/rogue/apls", "combat_cleave_snd_expose"),
				dropping(core.GetAplRotation("../../ui/rogue/apls", "fan_aoe"), map[int32]string{
					14177: "Cold Blood: an Assassination talent the Combat build lacks, in an AoE rotation both specs share",
				}),
			},
			ItemFilter: core.ItemFilter{
				ArmorType: proto.ArmorType_ArmorTypeLeather,
				RangedWeaponTypes: []proto.RangedWeaponType{
					proto.RangedWeaponType_RangedWeaponTypeBow,
					proto.RangedWeaponType_RangedWeaponTypeCrossbow,
					proto.RangedWeaponType_RangedWeaponTypeGun,
				},
			},
		}),
		&core.SingleDpsTestGenerator{Name: "Talents-HackAndSlash-Average", Request: hackAndSlashTalentsRequest()},
	))
}

func TestAssassination(t *testing.T) {
	core.RunTestSuite(t, t.Name(), core.JoinTestGenerators(
		core.FullCharacterTestSuiteGenerator(core.CharacterSuiteConfig{
			Class:       proto.Class_ClassRogue,
			Race:        proto.Race_RaceHuman,
			OtherRaces:  []proto.Race{proto.Race_RaceOrc},
			GearSet:     core.GetGearSet("../../ui/rogue/gear_sets", "p1_assassination"),
			Talents:     AssassinationTalents,
			Glyphs:      AssassinationGlyphs,
			Consumes:    FullConsumes,
			SpecOptions: core.SpecOptionsCombo{Label: "Assassination", SpecOptions: PlayerOptionsAssassinationDI},
			OtherSpecOptions: []core.SpecOptionsCombo{
				{Label: "MH Instant OH Deadly", SpecOptions: PlayerOptionsAssassinationID},
				{Label: "MH Instant OH Instant", SpecOptions: PlayerOptionsAssassinationII},
				{Label: "MH Deadly OH Deadly", SpecOptions: PlayerOptionsAssassinationDD},
			},
			Rotation: dropping(core.GetAplRotation("../../ui/rogue/apls", "rupture_mutilate_expose"), noBladeFlurry),
			OtherRotations: []core.RotationCombo{
				dropping(core.GetAplRotation("../../ui/rogue/apls", "rupture_mutilate"), noBladeFlurry),
				dropping(core.GetAplRotation("../../ui/rogue/apls", "mutilate"), noBladeFlurry),
				dropping(core.GetAplRotation("../../ui/rogue/apls", "mutilate_expose"), noBladeFlurry),
			},

			ItemFilter: core.ItemFilter{
				ArmorType: proto.ArmorType_ArmorTypeLeather,
				RangedWeaponTypes: []proto.RangedWeaponType{
					proto.RangedWeaponType_RangedWeaponTypeBow,
					proto.RangedWeaponType_RangedWeaponTypeCrossbow,
					proto.RangedWeaponType_RangedWeaponTypeGun,
				},
				WeaponTypes: []proto.WeaponType{
					proto.WeaponType_WeaponTypeDagger,
				},
			},
		}),
		&core.SingleDpsTestGenerator{Name: "Talents-TurnTheTables-Average", Request: turnTheTablesTalentsRequest()},
	))
}

// Runs a Combat APL, as there's no Subtlety one, so Subtlety's own abilities never cast.
func TestSubtlety(t *testing.T) {
	core.RunTestSuite(t, t.Name(), core.JoinTestGenerators(
		core.FullCharacterTestSuiteGenerator(core.CharacterSuiteConfig{
			Class:       proto.Class_ClassRogue,
			Race:        proto.Race_RaceBloodElf,
			OtherRaces:  []proto.Race{proto.Race_RaceOrc},
			GearSet:     core.GetGearSet("../../ui/rogue/gear_sets", "p2_hemosub"),
			Talents:     SubtletyTalents,
			Glyphs:      SubtletyGlyphs,
			Consumes:    FullConsumes,
			SpecOptions: core.SpecOptionsCombo{Label: "Subtlety", SpecOptions: PlayerOptionsSubtletyID},
			Rotation: dropping(core.GetAplRotation("../../ui/rogue/apls", "combat_expose"), map[int32]string{
				13750: "Adrenaline Rush: a Combat talent, and the suite runs a Combat rotation",
				13877: "Blade Flurry: a Combat talent, and the suite runs a Combat rotation",
				51690: "Killing Spree: a Combat talent, and the suite runs a Combat rotation",
			}),
			ItemFilter: core.ItemFilter{
				ArmorType: proto.ArmorType_ArmorTypeLeather,
				RangedWeaponTypes: []proto.RangedWeaponType{
					proto.RangedWeaponType_RangedWeaponTypeBow,
					proto.RangedWeaponType_RangedWeaponTypeCrossbow,
					proto.RangedWeaponType_RangedWeaponTypeGun,
				},
				WeaponTypes: []proto.WeaponType{
					proto.WeaponType_WeaponTypeDagger,
					proto.WeaponType_WeaponTypeFist,
				},
			},
		}),
		&core.SingleDpsTestGenerator{Name: "Talents-ShadowDance-Average", Request: shadowDanceTalentsRequest()},
	))
}

// The mutilate rotations cast Blade Flurry, a Combat talent, while Slice and Dice is up.
var noBladeFlurry = map[int32]string{13877: "Blade Flurry: a Combat talent the Assassination build lacks"}

func dropping(rotation core.RotationCombo, spells map[int32]string) core.RotationCombo {
	rotation.DroppedSpells = spells
	return rotation
}

// talentsAverageRequest runs player the way Average-Default runs a suite's default player
func talentsAverageRequest(player *proto.Player) *proto.RaidSimRequest {
	return &proto.RaidSimRequest{
		Raid:       core.SinglePlayerRaidProto(player, core.FullPartyBuffs, core.FullRaidBuffs, core.FullDebuffs),
		Encounter:  core.MakeSingleTargetEncounter(5),
		SimOptions: core.AverageDefaultSimTestOptions,
	}
}

// Hack and Slash and Mace Specialization, which no other build takes, on TestCombat's default player with
// a sword and a mace
func hackAndSlashTalentsRequest() *proto.RaidSimRequest {
	gear := core.GetGearSet("../../ui/rogue/gear_sets", "p1_combat").GearSet
	gear.Items[proto.ItemSlot_ItemSlotMainHand] = &proto.ItemSpec{Id: 40407, Enchant: 3789} // Silent Crusader
	gear.Items[proto.ItemSlot_ItemSlotOffHand] = &proto.ItemSpec{Id: 40264, Enchant: 3789}  // Split Greathammer
	return talentsAverageRequest(&proto.Player{
		Race:               proto.Race_RaceHuman,
		Class:              proto.Class_ClassRogue,
		Equipment:          gear,
		Consumes:           FullConsumes,
		Spec:               PlayerOptionsCombatDI,
		Buffs:              core.FullIndividualBuffs,
		Rotation:           core.GetAplRotation("../../ui/rogue/apls", "combat_expose").Rotation,
		TalentsString:      HackAndSlashTalents,
		Glyphs:             CombatGlyphs,
		Profession1:        proto.Profession_Engineering,
		DistanceFromTarget: 30,
		ReactionTimeMs:     150,
		ChannelClipDelayMs: 50,
	})
}

// Turn the Tables and Quick Recovery, which no other build takes, on TestAssassination's default player
func turnTheTablesTalentsRequest() *proto.RaidSimRequest {
	return talentsAverageRequest(&proto.Player{
		Race:               proto.Race_RaceHuman,
		Class:              proto.Class_ClassRogue,
		Equipment:          core.GetGearSet("../../ui/rogue/gear_sets", "p1_assassination").GearSet,
		Consumes:           FullConsumes,
		Spec:               PlayerOptionsAssassinationDI,
		Buffs:              core.FullIndividualBuffs,
		Rotation:           core.APLRotationFromJsonString(turnTheTablesTalentsAPL),
		TalentsString:      TurnTheTablesTalents,
		Glyphs:             AssassinationGlyphs,
		Profession1:        proto.Profession_Engineering,
		DistanceFromTarget: 30,
		ReactionTimeMs:     150,
		ChannelClipDelayMs: 50,
	})
}

// Ghostly Strike and Shadow Dance, and Vigor and Improved Expose Armor in the 20 Assassination points,
// which no other build takes, on TestSubtlety's default player
func shadowDanceTalentsRequest() *proto.RaidSimRequest {
	return talentsAverageRequest(&proto.Player{
		Race:               proto.Race_RaceBloodElf,
		Class:              proto.Class_ClassRogue,
		Equipment:          core.GetGearSet("../../ui/rogue/gear_sets", "p2_hemosub").GearSet,
		Consumes:           FullConsumes,
		Spec:               PlayerOptionsSubtletyID,
		Buffs:              core.FullIndividualBuffs,
		Rotation:           core.APLRotationFromJsonString(shadowDanceTalentsAPL),
		TalentsString:      ShadowDanceTalents,
		Glyphs:             SubtletyGlyphs,
		Profession1:        proto.Profession_Engineering,
		DistanceFromTarget: 30,
		ReactionTimeMs:     150,
		ChannelClipDelayMs: 50,
	})
}

// CombatTalents with Close Quarters Combat, Blade Twisting, Savage Combat and a point of Improved Poisons
// in Hack and Slash and Mace Specialization
var HackAndSlashTalents = "00532000522-0252051000035515203100501051"

// AssassinationTalents with Fleet Footed, which the sim doesn't read, in Quick Recovery, and Close Quarters
// Combat in Turn the Tables
var TurnTheTablesTalents = "005303005350102520103331351-005005-502"

// rupture_mutilate_expose without Blade Flurry
const turnTheTablesTalentsAPL = `{"type":"TypeAPL",
"prepullActions":[
	{"action":{"castSpell":{"spellId":{"otherId":"OtherActionPotion"}}},"doAtValue":{"const":{"val":"-1s"}}},
	{"action":{"activateAura":{"auraId":{"spellId":58426}}},"doAtValue":{"const":{"val":"-1s"}}}
],
"priorityList":[
	{"action":{"autocastOtherCooldowns":{}}},
	{"action":{"condition":{"cmp":{"op":"OpLe","lhs":{"auraRemainingTime":{"sourceUnit":{"type":"CurrentTarget"},"auraId":{"spellId":8647}}},"rhs":{"const":{"val":"1s"}}}},"castSpell":{"spellId":{"spellId":8647}}}},
	{"action":{"condition":{"cmp":{"op":"OpLe","lhs":{"auraRemainingTime":{"auraId":{"spellId":6774}}},"rhs":{"const":{"val":"1s"}}}},"castSpell":{"spellId":{"spellId":6774}}}},
	{"action":{"condition":{"cmp":{"op":"OpLe","lhs":{"auraRemainingTime":{"sourceUnit":{"type":"CurrentTarget"},"auraId":{"spellId":8647}}},"rhs":{"const":{"val":"1s"}}}},"sequence":{"actions":[{"castSpell":{"spellId":{"spellId":48666}}},{"castSpell":{"spellId":{"spellId":8647}}}]}}},
	{"action":{"condition":{"cmp":{"op":"OpLe","lhs":{"auraRemainingTime":{"auraId":{"spellId":6774}}},"rhs":{"const":{"val":"1s"}}}},"sequence":{"actions":[{"castSpell":{"spellId":{"spellId":48666}}},{"castSpell":{"spellId":{"spellId":6774}}}]}}},
	{"action":{"condition":{"cmp":{"op":"OpLe","lhs":{"auraRemainingTime":{"auraId":{"spellId":51662}}},"rhs":{"const":{"val":"1s"}}}},"castSpell":{"spellId":{"spellId":51662}}}},
	{"action":{"condition":{"not":{"val":{"auraIsActive":{"auraId":{"spellId":58426}}}}},"castSpell":{"spellId":{"spellId":26889}}}},
	{"action":{"condition":{"auraIsActive":{"auraId":{"spellId":6774}}},"castSpell":{"spellId":{"itemId":40211}}}},
	{"action":{"condition":{"auraIsActive":{"auraId":{"spellId":6774}}},"castSpell":{"spellId":{"spellId":54758}}}},
	{"action":{"condition":{"cmp":{"op":"OpLe","lhs":{"currentEnergy":{}},"rhs":{"const":{"val":"50"}}}},"castSpell":{"spellId":{"spellId":57934}}}},
	{"action":{"condition":{"cmp":{"op":"OpLe","lhs":{"currentEnergy":{}},"rhs":{"const":{"val":"50"}}}},"castSpell":{"spellId":{"itemId":7676}}}},
	{"action":{"condition":{"and":{"vals":[{"cmp":{"op":"OpGe","lhs":{"currentComboPoints":{}},"rhs":{"const":{"val":"4"}}}},{"not":{"val":{"auraIsActive":{"sourceUnit":{"type":"CurrentTarget"},"auraId":{"spellId":48672}}}}}]}},"castSpell":{"spellId":{"spellId":48672}}}},
	{"action":{"condition":{"or":{"vals":[{"cmp":{"op":"OpEq","lhs":{"currentComboPoints":{}},"rhs":{"const":{"val":"5"}}}},{"cmp":{"op":"OpLe","lhs":{"remainingTime":{}},"rhs":{"const":{"val":"5s"}}}}]}},"castSpell":{"spellId":{"spellId":14177}}}},
	{"action":{"condition":{"and":{"vals":[{"cmp":{"op":"OpGe","lhs":{"currentComboPoints":{}},"rhs":{"const":{"val":"4"}}}},{"or":{"vals":[{"not":{"val":{"auraIsActive":{"auraId":{"spellId":57993}}}}},{"cmp":{"op":"OpGe","lhs":{"currentEnergy":{}},"rhs":{"const":{"val":"85"}}}}]}}]}},"castSpell":{"spellId":{"spellId":57993}}}},
	{"action":{"condition":{"cmp":{"op":"OpLe","lhs":{"currentComboPoints":{}},"rhs":{"const":{"val":"3"}}}},"castSpell":{"spellId":{"spellId":48666}}}}
]}`

// SubtletyTalents with Improved Eviscerate and 3 points of Improved Poisons in Vigor, Improved Expose Armor,
// Ghostly Strike and Shadow Dance. Master of Deception's extra point, which the sim doesn't read, reaches
// Shadow Dance's row.
var ShadowDanceTalents = "00532012232--5220032130320121350115031131"

// Shadow Dance at 2 combo points or fewer, then Premeditation, with Ghostly Strike and Hemorrhage as
// builders. The fist weapon rules out Ambush.
const shadowDanceTalentsAPL = `{"type":"TypeAPL",
"prepullActions":[
	{"action":{"castSpell":{"spellId":{"otherId":"OtherActionPotion"}}},"doAtValue":{"const":{"val":"-1s"}}}
],
"priorityList":[
	{"action":{"autocastOtherCooldowns":{}}},
	{"action":{"condition":{"cmp":{"op":"OpLe","lhs":{"auraRemainingTime":{"auraId":{"spellId":6774}}},"rhs":{"const":{"val":"1s"}}}},"castSpell":{"spellId":{"spellId":6774}}}},
	{"action":{"condition":{"cmp":{"op":"OpLe","lhs":{"auraRemainingTime":{"sourceUnit":{"type":"CurrentTarget"},"auraId":{"spellId":8647}}},"rhs":{"const":{"val":"1s"}}}},"castSpell":{"spellId":{"spellId":8647}}}},
	{"action":{"condition":{"and":{"vals":[{"cmp":{"op":"OpEq","lhs":{"currentComboPoints":{}},"rhs":{"const":{"val":"5"}}}},{"not":{"val":{"dotIsActive":{"spellId":{"spellId":48672}}}}}]}},"castSpell":{"spellId":{"spellId":48672}}}},
	{"action":{"condition":{"cmp":{"op":"OpEq","lhs":{"currentComboPoints":{}},"rhs":{"const":{"val":"5"}}}},"castSpell":{"spellId":{"spellId":48668}}}},
	{"action":{"condition":{"cmp":{"op":"OpLe","lhs":{"currentComboPoints":{}},"rhs":{"const":{"val":"2"}}}},"castSpell":{"spellId":{"spellId":51713}}}},
	{"action":{"castSpell":{"spellId":{"spellId":14183}}}},
	{"action":{"castSpell":{"spellId":{"spellId":14278}}}},
	{"action":{"condition":{"cmp":{"op":"OpLt","lhs":{"currentComboPoints":{}},"rhs":{"const":{"val":"5"}}}},"castSpell":{"spellId":{"spellId":48660}}}}
]}`

type AttackType int

const (
	Poison AttackType = iota
	MHAuto
	OHAuto
	Builder
	Finisher
)

func GenerateCriticalDamageMultiplierTestCase(
	t *testing.T,
	testName string,
	equipment *proto.EquipmentSpec,
	talents string,
	spec *proto.Player_Rogue,
	attackType AttackType,
	expectedMultiplier float64) {
	raid := core.SinglePlayerRaidProto(core.WithSpec(&proto.Player{
		Class:         proto.Class_ClassRogue,
		Race:          proto.Race_RaceOrc,
		Equipment:     equipment,
		TalentsString: talents,
		Rotation:      core.GetAplRotation("../../ui/rogue/apls", "combat_expose").Rotation,
	}, spec), nil, nil, nil)
	encounter := core.MakeSingleTargetEncounter(0.0)
	env, _, _ := core.NewEnvironment(raid, encounter, false)
	agent := env.Raid.Parties[0].Players[0]
	rog := agent.(RogueAgent).GetRogue()
	actualMultiplier := 0.0
	switch attackType {
	case Poison:
		actualMultiplier = rog.SpellCritMultiplier()
	case MHAuto:
		actualMultiplier = rog.MeleeCritMultiplier(false)
	case OHAuto:
		actualMultiplier = rog.MeleeCritMultiplier(false)
	case Builder:
		actualMultiplier = rog.MeleeCritMultiplier(true)
	case Finisher:
		actualMultiplier = rog.MeleeCritMultiplier(false)
	}
	t.Run(testName, func(t *testing.T) {
		if !core.WithinToleranceFloat64(expectedMultiplier, actualMultiplier, 0.0001) {
			t.Logf("Crit damage multiplier for %s expected %f but was %f", testName, expectedMultiplier, actualMultiplier)
			t.Fail()
		}
	})

}

// Verifies the critical damage multipliers conform to
// https://github.com/where-fore/rogue-wotlk/issues/31
func TestCritDamageMultipliers(t *testing.T) {
	// Poison, no RED
	GenerateCriticalDamageMultiplierTestCase(t, "Poison", GearWithoutRED, CombatNoPotWTalents, PlayerOptionsNoPotW, Poison, 1.5)
	// Poison, with RED
	GenerateCriticalDamageMultiplierTestCase(t, "PoisonRED", GearWithRED, CombatNoPotWTalents, PlayerOptionsNoPotW, Poison, 1.545000)
	// Poison, with RED & PotW
	GenerateCriticalDamageMultiplierTestCase(t, "PoisonREDPotW", GearWithRED, CombatTalents, PlayerOptionsCombatDI, Poison, 1.854000)
	// Auto, no RED, no Lethality, no PotW
	GenerateCriticalDamageMultiplierTestCase(t, "Auto", GearWithoutRED, CombatNoLethalityNoPotWTalents, PlayerOptionsNoLethalityNoPotW, MHAuto, 2.0)
	// Auto, RED, no Lethality, no PotW
	GenerateCriticalDamageMultiplierTestCase(t, "AutoRED", GearWithRED, CombatNoLethalityNoPotWTalents, PlayerOptionsNoLethalityNoPotW, MHAuto, 2.06)
	// Auto, RED, no Lethality, PotW
	GenerateCriticalDamageMultiplierTestCase(t, "AutoREDPotW", GearWithRED, CombatNoLethalityTalents, PlayerOptionsNoLethality, MHAuto, 2.472)
	// Builder, no RED, Lethality, no PotW
	GenerateCriticalDamageMultiplierTestCase(t, "BuilderLethality", GearWithoutRED, CombatNoPotWTalents, PlayerOptionsNoPotW, Builder, 2.3)
	// Builder, RED, Lethality, no PotW
	GenerateCriticalDamageMultiplierTestCase(t, "BuilderREDLethality", GearWithRED, CombatNoPotWTalents, PlayerOptionsNoPotW, Builder, 2.378000)
	// Builder, no RED, Lethality, PotW
	GenerateCriticalDamageMultiplierTestCase(t, "BuilderLethalityPotW", GearWithoutRED, CombatTalents, PlayerOptionsCombatDI, Builder, 2.820000)
	// Builder, RED, Lethality, PotW
	GenerateCriticalDamageMultiplierTestCase(t, "BuilderREDLethalityPotW", GearWithRED, CombatTalents, PlayerOptionsCombatDI, Builder, 2.913600)
	// Finisher, no RED, Lethality, PotW
	GenerateCriticalDamageMultiplierTestCase(t, "FinisherLethalityPotW", GearWithoutRED, CombatTalents, PlayerOptionsCombatDI, Finisher, 2.4)
	// Finisher, no RED, Lethality, PotW
	GenerateCriticalDamageMultiplierTestCase(t, "FinisherREDLethalityPotW", GearWithRED, CombatTalents, PlayerOptionsCombatDI, Finisher, 2.472)
}

// TestCombat's default player, as its FullBuffs LongSingleTarget test runs it
func BenchmarkSimulate(b *testing.B) {
	rsr := &proto.RaidSimRequest{
		Raid: core.SinglePlayerRaidProto(
			&proto.Player{
				Race:               proto.Race_RaceHuman,
				Class:              proto.Class_ClassRogue,
				Equipment:          core.GetGearSet("../../ui/rogue/gear_sets", "p1_combat").GearSet,
				TalentsString:      CombatTalents,
				Glyphs:             CombatGlyphs,
				Consumes:           FullConsumes,
				Spec:               PlayerOptionsCombatDI,
				Buffs:              core.FullIndividualBuffs,
				Rotation:           core.GetAplRotation("../../ui/rogue/apls", "combat_expose").Rotation,
				Profession1:        proto.Profession_Engineering,
				DistanceFromTarget: 30,
				ReactionTimeMs:     150,
				ChannelClipDelayMs: 50,
			},
			core.FullPartyBuffs,
			core.FullRaidBuffs,
			core.FullDebuffs),
		Encounter: core.MakeSingleTargetEncounter(0),
	}

	core.RaidBenchmarkIterations(b, rsr, 1, 100)
}

var CombatTalents = "00532000523-0252051050035010223100501251"
var CombatNoLethalityTalents = "00532000023-0252051050035010223100501251"
var CombatNoPotWTalents = "00532000523-0252051050035010223100501201"
var CombatNoLethalityNoPotWTalents = "00532000023-0252051050035010223100501201"
var AssassinationTalents = "005303005352100520103331051-005005003-502"
var SubtletyTalents = "30532000235--512003203032012135011503113"
var CombatGlyphs = &proto.Glyphs{
	Major1: int32(proto.RogueMajorGlyph_GlyphOfKillingSpree),
	Major2: int32(proto.RogueMajorGlyph_GlyphOfTricksOfTheTrade),
	Major3: int32(proto.RogueMajorGlyph_GlyphOfRupture),
}
var AssassinationGlyphs = &proto.Glyphs{
	Major1: int32(proto.RogueMajorGlyph_GlyphOfMutilate),
	Major2: int32(proto.RogueMajorGlyph_GlyphOfTricksOfTheTrade),
	Major3: int32(proto.RogueMajorGlyph_GlyphOfHungerForBlood),
}
var SubtletyGlyphs = &proto.Glyphs{
	Major1: int32(proto.RogueMajorGlyph_GlyphOfHemorrhage),
	Major2: int32(proto.RogueMajorGlyph_GlyphOfEviscerate),
	Major3: int32(proto.RogueMajorGlyph_GlyphOfRupture),
}

var PlayerOptionsCombatDI = &proto.Player_Rogue{
	Rogue: &proto.Rogue{
		Options: DeadlyInstant,
	},
}
var PlayerOptionsCombatDD = &proto.Player_Rogue{
	Rogue: &proto.Rogue{
		Options: DeadlyDeadly,
	},
}
var PlayerOptionsCombatID = &proto.Player_Rogue{
	Rogue: &proto.Rogue{
		Options: InstantDeadly,
	},
}
var PlayerOptionsCombatII = &proto.Player_Rogue{
	Rogue: &proto.Rogue{
		Options: InstantInstant,
	},
}

var PlayerOptionsNoLethality = &proto.Player_Rogue{
	Rogue: &proto.Rogue{
		Options: DeadlyInstant,
	},
}

var PlayerOptionsNoPotW = &proto.Player_Rogue{
	Rogue: &proto.Rogue{
		Options: DeadlyInstant,
	},
}

var PlayerOptionsNoLethalityNoPotW = &proto.Player_Rogue{
	Rogue: &proto.Rogue{
		Options: DeadlyInstant,
	},
}

var PlayerOptionsAssassinationDI = &proto.Player_Rogue{
	Rogue: &proto.Rogue{
		Options: DeadlyInstant,
	},
}
var PlayerOptionsAssassinationDD = &proto.Player_Rogue{
	Rogue: &proto.Rogue{
		Options: DeadlyDeadly,
	},
}
var PlayerOptionsAssassinationID = &proto.Player_Rogue{
	Rogue: &proto.Rogue{
		Options: InstantDeadly,
	},
}
var PlayerOptionsAssassinationII = &proto.Player_Rogue{
	Rogue: &proto.Rogue{
		Options: InstantInstant,
	},
}

var PlayerOptionsSubtletyID = &proto.Player_Rogue{
	Rogue: &proto.Rogue{
		Options: InstantDeadly,
	},
}

var DeadlyInstant = &proto.Rogue_Options{
	MhImbue: proto.Rogue_Options_DeadlyPoison,
	OhImbue: proto.Rogue_Options_InstantPoison,
}
var InstantDeadly = &proto.Rogue_Options{
	MhImbue: proto.Rogue_Options_InstantPoison,
	OhImbue: proto.Rogue_Options_DeadlyPoison,
}
var InstantInstant = &proto.Rogue_Options{
	MhImbue: proto.Rogue_Options_InstantPoison,
	OhImbue: proto.Rogue_Options_InstantPoison,
}
var DeadlyDeadly = &proto.Rogue_Options{
	MhImbue: proto.Rogue_Options_DeadlyPoison,
	OhImbue: proto.Rogue_Options_DeadlyPoison,
}

var FullConsumes = &proto.Consumes{
	Flask:           proto.Flask_FlaskOfEndlessRage,
	DefaultPotion:   proto.Potions_PotionOfSpeed,
	DefaultConjured: proto.Conjured_ConjuredRogueThistleTea,
}

var GearWithoutRED = core.EquipmentSpecFromJsonString(`{"items":[
	{"id":37293,"enchant":3817,"gems":[41339,40088]},
	{"id":37861},
	{"id":37139,"enchant":3808,"gems":[36766]},
	{"id":36947,"enchant":3605},
	{"id":37165,"enchant":3832,"gems":[40044,36766]},
	{"id":44203,"enchant":3845,"gems":[0]},
	{"id":37409,"enchant":3604,"gems":[0]},
	{"id":37194,"gems":[40014,40157]},
	{"id":37644,"enchant":3823},
	{"id":44297,"enchant":3606},
	{"id":43251,"gems":[40136]},
	{"id":37642},
	{"id":37390},
	{"id":37166},
	{"id":37693,"enchant":3789},
	{"id":37856,"enchant":3789},
	{"id":37191}
]}`)
var GearWithRED = core.EquipmentSpecFromJsonString(`{"items":[
	{"id":37293,"enchant":3817,"gems":[41398,40088]},
	{"id":37861},
	{"id":37139,"enchant":3808,"gems":[36766]},
	{"id":36947,"enchant":3605},
	{"id":37165,"enchant":3832,"gems":[40044,36766]},
	{"id":44203,"enchant":3845,"gems":[0]},
	{"id":37409,"enchant":3604,"gems":[0]},
	{"id":37194,"gems":[40014,40157]},
	{"id":37644,"enchant":3823},
	{"id":44297,"enchant":3606},
	{"id":43251,"gems":[40136]},
	{"id":37642},
	{"id":37390},
	{"id":37166},
	{"id":37693,"enchant":3789},
	{"id":37856,"enchant":3789},
	{"id":37191}
]}`)
