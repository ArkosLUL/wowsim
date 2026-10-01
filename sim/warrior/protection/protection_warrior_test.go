package protection

import (
	"testing"

	_ "github.com/wowsims/wotlk/sim/common" // imported to get item effects included.
	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
	"github.com/wowsims/wotlk/sim/encounters"
)

func init() {
	RegisterProtectionWarrior()
}

func TestProtectionWarrior(t *testing.T) {
	core.RunTestSuite(t, t.Name(), core.JoinTestGenerators(
		core.FullCharacterTestSuiteGenerator(core.CharacterSuiteConfig{
			Class:      proto.Class_ClassWarrior,
			Race:       proto.Race_RaceOrc,
			OtherRaces: []proto.Race{proto.Race_RaceHuman},

			GearSet:     core.GetGearSet("../../../ui/protection_warrior/gear_sets", "p1_balanced"),
			Talents:     DefaultTalents,
			Glyphs:      DefaultGlyphs,
			Consumes:    FullConsumes,
			SpecOptions: core.SpecOptionsCombo{Label: "Basic", SpecOptions: PlayerOptionsBasic},
			Rotation:    core.GetAplRotation("../../../ui/protection_warrior/apls", "default"),

			IsTank:          true,
			InFrontOfTarget: true,

			ItemFilter: core.ItemFilter{
				ArmorType: proto.ArmorType_ArmorTypePlate,

				WeaponTypes: []proto.WeaponType{
					proto.WeaponType_WeaponTypeAxe,
					proto.WeaponType_WeaponTypeSword,
					proto.WeaponType_WeaponTypeMace,
					proto.WeaponType_WeaponTypeDagger,
					proto.WeaponType_WeaponTypeFist,
					proto.WeaponType_WeaponTypeShield,
				},
			},

			EPReferenceStat: proto.Stat_StatAttackPower,
			StatsToWeigh: []proto.Stat{
				proto.Stat_StatStrength,
				proto.Stat_StatAttackPower,
				proto.Stat_StatArmor,
				proto.Stat_StatDodge,
				proto.Stat_StatBlockValue,
			},
		}),
		&core.SingleDpsTestGenerator{Name: "GenericBoss", Request: genericBossRequest()},
		&core.SingleDpsTestGenerator{Name: "Talents-FocusedRage-Average", Request: focusedRageTalentsRequest()},
	))
}

// The generic AzerothCore boss, on the same gear and rotation as TestProtectionWarrior's default,
// alongside the Classic-style target that suite still keeps.
func genericBossRequest() *proto.RaidSimRequest {
	rsr := &proto.RaidSimRequest{
		Raid: core.SinglePlayerRaidProto(
			&proto.Player{
				Race:            proto.Race_RaceOrc,
				Class:           proto.Class_ClassWarrior,
				Equipment:       core.GetGearSet("../../../ui/protection_warrior/gear_sets", "p1_balanced").GearSet,
				Consumes:        FullConsumes,
				Spec:            PlayerOptionsBasic,
				Buffs:           core.FullIndividualBuffs,
				Rotation:        core.GetAplRotation("../../../ui/protection_warrior/apls", "default").Rotation,
				TalentsString:   DefaultTalents,
				Glyphs:          DefaultGlyphs,
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

func BenchmarkSimulate(b *testing.B) {
	rsr := &proto.RaidSimRequest{
		Raid: core.SinglePlayerRaidProto(
			&proto.Player{
				Race:          proto.Race_RaceOrc,
				Class:         proto.Class_ClassWarrior,
				Equipment:     core.GetGearSet("../../../ui/protection_warrior/gear_sets", "p1_balanced").GearSet,
				Consumes:      FullConsumes,
				Spec:          PlayerOptionsBasic,
				Buffs:         core.FullIndividualBuffs,
				Rotation:      core.GetAplRotation("../../../ui/protection_warrior/apls", "default").Rotation,
				TalentsString: DefaultTalents,
				Glyphs:        DefaultGlyphs,

				InFrontOfTarget: true,
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

var DefaultTalents = "2500030023-302-053351225000012521030113321"
var DefaultGlyphs = &proto.Glyphs{
	Major1: int32(proto.WarriorMajorGlyph_GlyphOfBlocking),
	Major2: int32(proto.WarriorMajorGlyph_GlyphOfDevastate),
	Major3: int32(proto.WarriorMajorGlyph_GlyphOfVigilance),
}

var PlayerOptionsBasic = &proto.Player_ProtectionWarrior{
	ProtectionWarrior: &proto.ProtectionWarrior{
		Options: warriorOptions,
	},
}

var warriorOptions = &proto.ProtectionWarrior_Options{
	Shout:        proto.WarriorShout_WarriorShoutCommanding,
	StartingRage: 0,
}

var FullConsumes = &proto.Consumes{
	BattleElixir:   proto.BattleElixir_ElixirOfMastery,
	GuardianElixir: proto.GuardianElixir_GiftOfArthas,
}

// Puncture, Improved Disciplines, Focused Rage, Booming Voice and Improved Demoralizing Shout, which no other
// build takes, on TestProtectionWarrior's default player
func focusedRageTalentsRequest() *proto.RaidSimRequest {
	rsr := &proto.RaidSimRequest{
		Raid: core.SinglePlayerRaidProto(
			&proto.Player{
				Race:               proto.Race_RaceOrc,
				Class:              proto.Class_ClassWarrior,
				Equipment:          core.GetGearSet("../../../ui/protection_warrior/gear_sets", "p1_balanced").GearSet,
				Consumes:           FullConsumes,
				Spec:               PlayerOptionsBasic,
				Buffs:              core.FullIndividualBuffs,
				Rotation:           core.GetAplRotation("../../../ui/protection_warrior/apls", "default").Rotation,
				TalentsString:      FocusedRageTalents,
				Glyphs:             DefaultGlyphs,
				Profession1:        proto.Profession_Engineering,
				InFrontOfTarget:    true,
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
	rsr.Raid.Tanks = []*proto.UnitReference{{Type: proto.UnitReference_Player, Index: 0}}
	return rsr
}

// DefaultTalents with Improved Heroic Strike, Tactical Mastery, Impale, Deep Wounds, a point of Deflection,
// Cruelty, and Warbringer and Vigilance, which the sim doesn't read, in Puncture, Improved Disciplines,
// Focused Rage, Booming Voice and Improved Demoralizing Shout
var FocusedRageTalents = "04-3205-053351225003212520330013321"
