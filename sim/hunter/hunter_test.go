package hunter

import (
	"testing"

	_ "github.com/wowsims/wotlk/sim/common" // imported to get item effects included.
	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
	"github.com/wowsims/wotlk/sim/core/stats"
)

func init() {
	RegisterHunter()
}

func TestBM(t *testing.T) {
	core.RunTestSuite(t, t.Name(), core.FullCharacterTestSuiteGenerator(core.CharacterSuiteConfig{
		Class:      proto.Class_ClassHunter,
		Race:       proto.Race_RaceOrc,
		OtherRaces: []proto.Race{proto.Race_RaceDwarf},

		GearSet:     core.GetGearSet("../../ui/hunter/gear_sets", "p1_sv"),
		Talents:     BMTalents,
		Glyphs:      BMGlyphs,
		Consumes:    FullConsumes,
		SpecOptions: core.SpecOptionsCombo{Label: "Basic", SpecOptions: PlayerOptionsBasic},
		Rotation:    core.GetAplRotation("../../ui/hunter/apls", "bm"),

		ItemFilter: ItemFilter,
	}))
}

func TestMM(t *testing.T) {
	core.RunTestSuite(t, t.Name(), core.FullCharacterTestSuiteGenerator(core.CharacterSuiteConfig{
		Class:      proto.Class_ClassHunter,
		Race:       proto.Race_RaceOrc,
		OtherRaces: []proto.Race{proto.Race_RaceDwarf},

		GearSet:     core.GetGearSet("../../ui/hunter/gear_sets", "p1_mm"),
		Talents:     MMTalents,
		Glyphs:      MMGlyphs,
		Consumes:    FullConsumes,
		SpecOptions: core.SpecOptionsCombo{Label: "Basic", SpecOptions: PlayerOptionsBasic},
		Rotation:    core.GetAplRotation("../../ui/hunter/apls", "mm"),
		OtherRotations: []core.RotationCombo{
			core.GetAplRotation("../../ui/hunter/apls", "mm_advanced"),
		},

		ItemFilter: ItemFilter,
	}))
}

func TestSV(t *testing.T) {
	core.RunTestSuite(t, t.Name(), core.FullCharacterTestSuiteGenerator(core.CharacterSuiteConfig{
		Class:      proto.Class_ClassHunter,
		Race:       proto.Race_RaceOrc,
		OtherRaces: []proto.Race{proto.Race_RaceDwarf},

		GearSet:     core.GetGearSet("../../ui/hunter/gear_sets", "p1_sv"),
		Talents:     SVTalents,
		Glyphs:      SVGlyphs,
		Consumes:    FullConsumes,
		SpecOptions: core.SpecOptionsCombo{Label: "Basic", SpecOptions: PlayerOptionsBasic},
		Rotation:    core.GetAplRotation("../../ui/hunter/apls", "sv"),
		OtherRotations: []core.RotationCombo{
			core.GetAplRotation("../../ui/hunter/apls", "sv_advanced"),
			core.GetAplRotation("../../ui/hunter/apls", "aoe"),
		},

		ItemFilter: ItemFilter,
	}))
}

// a legal ferocity build with all 22 points Beast Mastery gives on this server. The suites'
// FerocityTalents spend 16, so no golden reaches Bloodthirsty, Heart of the Phoenix, Shark Attack or
// Wild Hunt's rank 2.
var beastMastery22PtPetTalents = &proto.HunterPetTalents{
	CobraReflexes:     2,
	Dive:              true,
	Bloodthirsty:      2,
	HeartOfThePheonix: true,
	SpikedCollar:      3,
	BoarsSpeed:        true,
	CullingTheHerd:    3,
	SpidersBite:       3,
	Rabid:             true,
	CallOfTheWild:     true,
	SharkAttack:       2,
	WildHunt:          2,
}

// only checks that a full 22-point pet sims without an error
func TestBeastMastery22PtPetTalents(t *testing.T) {
	player := &proto.Player{
		Race:          proto.Race_RaceOrc,
		Class:         proto.Class_ClassHunter,
		Equipment:     core.GetGearSet("../../ui/hunter/gear_sets", "p1_sv").GearSet,
		Consumes:      FullConsumes,
		TalentsString: BMTalents,
		Glyphs:        BMGlyphs,
		Buffs:         core.FullIndividualBuffs,
		Spec: &proto.Player_Hunter{Hunter: &proto.Hunter{Options: &proto.Hunter_Options{
			Ammo:                 proto.Hunter_Options_SaroniteRazorheads,
			PetType:              proto.Hunter_Options_Wolf,
			PetTalents:           beastMastery22PtPetTalents,
			PetUptime:            0.9,
			TimeToTrapWeaveMs:    2000,
			SniperTrainingUptime: 0.8,
			UseHuntersMark:       true,
		}}},
		Rotation: core.GetAplRotation("../../ui/hunter/apls", "bm").Rotation,
	}

	result := core.RunRaidSim(&proto.RaidSimRequest{
		Raid:       core.SinglePlayerRaidProto(player, core.FullPartyBuffs, core.FullRaidBuffs, core.FullDebuffs),
		Encounter:  core.MakeSingleTargetEncounter(0),
		SimOptions: &proto.SimOptions{Iterations: 20, RandomSeed: 101},
	})
	if result.ErrorResult != "" {
		t.Fatalf("22-point pet talents: %s", result.ErrorResult)
	}
}

// makeStatInheritance truncates Wild Hunt's share to whole percents: stamina 45+45*20*rank/100, AP
// 22+22*15*rank/100. The suites only reach rank 1 (54/25); rank 2 is 63/28.
func TestWildHuntStatInheritance(t *testing.T) {
	ownerStats := stats.Stats{
		stats.Stamina:           1000,
		stats.RangedAttackPower: 5000,
		stats.Armor:             10000,
	}

	for _, tc := range []struct {
		wildHunt   int32
		staminaPct float64
		apPct      float64
	}{
		{0, 45, 22},
		{1, 54, 25},
		{2, 63, 28},
	} {
		hunter := &Hunter{
			Talents: &proto.HunterTalents{},
			Options: &proto.Hunter_Options{PetTalents: &proto.HunterPetTalents{WildHunt: tc.wildHunt}},
		}
		petStats := hunter.makeStatInheritance()(ownerStats)

		wantStamina := ownerStats[stats.Stamina] * tc.staminaPct / 100
		wantAP := ownerStats[stats.RangedAttackPower] * tc.apPct / 100
		if petStats[stats.Stamina] != wantStamina {
			t.Errorf("wildHunt %d: pet stamina = %.4f, want %.4f", tc.wildHunt, petStats[stats.Stamina], wantStamina)
		}
		if petStats[stats.AttackPower] != wantAP {
			t.Errorf("wildHunt %d: pet AP = %.4f, want %.4f", tc.wildHunt, petStats[stats.AttackPower], wantAP)
		}
	}
}

var ItemFilter = core.ItemFilter{
	ArmorType: proto.ArmorType_ArmorTypeMail,
	WeaponTypes: []proto.WeaponType{
		proto.WeaponType_WeaponTypeAxe,
		proto.WeaponType_WeaponTypeDagger,
		proto.WeaponType_WeaponTypeFist,
		proto.WeaponType_WeaponTypeMace,
		proto.WeaponType_WeaponTypeOffHand,
		proto.WeaponType_WeaponTypePolearm,
		proto.WeaponType_WeaponTypeStaff,
		proto.WeaponType_WeaponTypeSword,
	},
	RangedWeaponTypes: []proto.RangedWeaponType{
		proto.RangedWeaponType_RangedWeaponTypeBow,
		proto.RangedWeaponType_RangedWeaponTypeCrossbow,
		proto.RangedWeaponType_RangedWeaponTypeGun,
	},
}

// TestMM's default player, as its FullBuffs LongSingleTarget test runs it
func BenchmarkSimulate(b *testing.B) {
	rsr := &proto.RaidSimRequest{
		Raid: core.SinglePlayerRaidProto(
			&proto.Player{
				Race:               proto.Race_RaceOrc,
				Class:              proto.Class_ClassHunter,
				Equipment:          core.GetGearSet("../../ui/hunter/gear_sets", "p1_mm").GearSet,
				Consumes:           FullConsumes,
				Spec:               PlayerOptionsBasic,
				Glyphs:             MMGlyphs,
				TalentsString:      MMTalents,
				Buffs:              core.FullIndividualBuffs,
				Rotation:           core.GetAplRotation("../../ui/hunter/apls", "mm").Rotation,
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

var FullConsumes = &proto.Consumes{
	Flask:           proto.Flask_FlaskOfRelentlessAssault,
	DefaultPotion:   proto.Potions_HastePotion,
	DefaultConjured: proto.Conjured_ConjuredFlameCap,
	PetFood:         proto.PetFood_PetFoodKiblersBits,
}

var BMTalents = "51200201515012233110531351-005305001-4"
var MMTalents = "502-035335131030013233035031051-5000002"
var SVTalents = "-015305101-5000032500033330532135301311"
var BMGlyphs = &proto.Glyphs{
	Major1: int32(proto.HunterMajorGlyph_GlyphOfBestialWrath),
	Major2: int32(proto.HunterMajorGlyph_GlyphOfSteadyShot),
	Major3: int32(proto.HunterMajorGlyph_GlyphOfSerpentSting),
}
var MMGlyphs = &proto.Glyphs{
	Major1: int32(proto.HunterMajorGlyph_GlyphOfSerpentSting),
	Major2: int32(proto.HunterMajorGlyph_GlyphOfSteadyShot),
	Major3: int32(proto.HunterMajorGlyph_GlyphOfChimeraShot),
}
var SVGlyphs = &proto.Glyphs{
	Major1: int32(proto.HunterMajorGlyph_GlyphOfSerpentSting),
	Major2: int32(proto.HunterMajorGlyph_GlyphOfExplosiveShot),
	Major3: int32(proto.HunterMajorGlyph_GlyphOfKillShot),
}

var FerocityTalents = &proto.HunterPetTalents{
	CobraReflexes:  2,
	Dive:           true,
	SpikedCollar:   3,
	BoarsSpeed:     true,
	CullingTheHerd: 3,
	SpidersBite:    3,
	Rabid:          true,
	CallOfTheWild:  true,
	WildHunt:       1,
}

var PlayerOptionsBasic = &proto.Player_Hunter{
	Hunter: &proto.Hunter{
		Options: &proto.Hunter_Options{
			Ammo:       proto.Hunter_Options_SaroniteRazorheads,
			PetType:    proto.Hunter_Options_Wolf,
			PetTalents: FerocityTalents,
			PetUptime:  0.9,

			TimeToTrapWeaveMs:    2000,
			SniperTrainingUptime: 0.8,
			UseHuntersMark:       true,
		},
	},
}
