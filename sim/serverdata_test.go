package sim

import (
	"fmt"
	"maps"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
	googleProto "google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
)

// serverDataPreset is one golden suite's character: enough of its class test's config to register the
// spells its tests register. Consumes, buffs and debuffs are the same for everyone.
type serverDataPreset struct {
	name    string
	class   proto.Class
	race    proto.Race
	ui      string // ui/<dir>, for gear_sets and apls
	gear    string
	apl     string // none for the healers, which only autocast cooldowns
	talents string
	glyphs  *proto.Glyphs
	spec    any
	tank    bool
}

func glyphs(major1, major2, major3 int32, minors ...int32) *proto.Glyphs {
	g := &proto.Glyphs{Major1: major1, Major2: major2, Major3: major3}
	slots := []*int32{&g.Minor1, &g.Minor2, &g.Minor3}
	for i, m := range minors {
		*slots[i] = m
	}
	return g
}

// classGlyphs is every glyph each class can take. A preset takes three majors and three minors, so
// without this the rest, and the spells some of them gate, never register.
var classGlyphs = map[proto.Class]struct{ major, minor []int32 }{
	proto.Class_ClassDeathknight: {glyphIDs(proto.DeathknightMajorGlyph_value), glyphIDs(proto.DeathknightMinorGlyph_value)},
	proto.Class_ClassDruid:       {glyphIDs(proto.DruidMajorGlyph_value), glyphIDs(proto.DruidMinorGlyph_value)},
	proto.Class_ClassHunter:      {glyphIDs(proto.HunterMajorGlyph_value), glyphIDs(proto.HunterMinorGlyph_value)},
	proto.Class_ClassMage:        {glyphIDs(proto.MageMajorGlyph_value), glyphIDs(proto.MageMinorGlyph_value)},
	proto.Class_ClassPaladin:     {glyphIDs(proto.PaladinMajorGlyph_value), glyphIDs(proto.PaladinMinorGlyph_value)},
	proto.Class_ClassPriest:      {glyphIDs(proto.PriestMajorGlyph_value), glyphIDs(proto.PriestMinorGlyph_value)},
	proto.Class_ClassRogue:       {glyphIDs(proto.RogueMajorGlyph_value), glyphIDs(proto.RogueMinorGlyph_value)},
	proto.Class_ClassShaman:      {glyphIDs(proto.ShamanMajorGlyph_value), glyphIDs(proto.ShamanMinorGlyph_value)},
	proto.Class_ClassWarlock:     {glyphIDs(proto.WarlockMajorGlyph_value), glyphIDs(proto.WarlockMinorGlyph_value)},
	proto.Class_ClassWarrior:     {glyphIDs(proto.WarriorMajorGlyph_value), glyphIDs(proto.WarriorMinorGlyph_value)},
}

func glyphIDs(values map[string]int32) []int32 {
	ids := slices.Sorted(maps.Values(values))
	return ids[1:] // the None value is 0, so it sorts first
}

// glyphVariants walks the preset's class through every glyph it can take, three majors and three minors
// at a time. It drops the preset's own glyphs, which the preset itself already covers.
func glyphVariants(p serverDataPreset) []serverDataPreset {
	g := classGlyphs[p.class]
	variants := make([]serverDataPreset, 0, (max(len(g.major), len(g.minor))+2)/3)
	for i := 0; i < len(g.major) || i < len(g.minor); i += 3 {
		v := p
		v.name = fmt.Sprintf("%s, glyphs %d", p.name, i/3+1)
		v.glyphs = &proto.Glyphs{}
		slots := []*int32{&v.glyphs.Major1, &v.glyphs.Major2, &v.glyphs.Major3,
			&v.glyphs.Minor1, &v.glyphs.Minor2, &v.glyphs.Minor3}
		for j := range 3 {
			if i+j < len(g.major) {
				*slots[j] = g.major[i+j]
			}
			if i+j < len(g.minor) {
				*slots[3+j] = g.minor[i+j]
			}
		}
		variants = append(variants, v)
	}
	return variants
}

var (
	dkOptions = &proto.Player_Deathknight{Deathknight: &proto.Deathknight{Options: &proto.Deathknight_Options{
		UnholyFrenzyTarget: &proto.UnitReference{Type: proto.UnitReference_Player, Index: 0},
		DrwPestiApply:      true,
		PetUptime:          1,
	}}}
	mageOptions = func(armor proto.Mage_Options_ArmorType) *proto.Player_Mage {
		return &proto.Player_Mage{Mage: &proto.Mage{Options: &proto.Mage_Options{Armor: armor}}}
	}
	paladinOptions = &proto.RetributionPaladin_Options{
		Judgement: proto.PaladinJudgement_JudgementOfWisdom,
		Seal:      proto.PaladinSeal_Vengeance,
		Aura:      proto.PaladinAura_RetributionAura,
	}
	rogueOptions = func(mh, oh proto.Rogue_Options_PoisonImbue) *proto.Player_Rogue {
		return &proto.Player_Rogue{Rogue: &proto.Rogue{Options: &proto.Rogue_Options{MhImbue: mh, OhImbue: oh}}}
	}
	shamanTotems = func(fire proto.FireTotem, fireElemental bool) *proto.ShamanTotems {
		return &proto.ShamanTotems{Earth: proto.EarthTotem_TremorTotem, Air: proto.AirTotem_WrathOfAirTotem,
			Water: proto.WaterTotem_ManaSpringTotem, Fire: fire, UseFireElemental: fireElemental}
	}
	warlockOptions = func(summon proto.Warlock_Options_Summon, imbue proto.Warlock_Options_WeaponImbue) *proto.Player_Warlock {
		return &proto.Player_Warlock{Warlock: &proto.Warlock{Options: &proto.Warlock_Options{
			Armor: proto.Warlock_Options_FelArmor, Summon: summon, WeaponImbue: imbue, DetonateSeed: true}}}
	}
	warriorOptions = &proto.Player_Warrior{Warrior: &proto.Warrior{Options: &proto.Warrior_Options{
		StartingRage: 50, UseRecklessness: true, UseShatteringThrow: true, Shout: proto.WarriorShout_WarriorShoutBattle,
	}}}
	priestHealer = &proto.HealingPriest_Options{UseInnerFire: true, UseShadowfiend: true, RapturesPerMinute: 5}
)

// One per golden suite, copied from the class tests.
var serverDataPresets = []serverDataPreset{
	{"Blood", proto.Class_ClassDeathknight, proto.Race_RaceOrc, "deathknight", "p3_blood", "blood_dps",
		"2305120530003303231023001351--230220305003", glyphs(int32(proto.DeathknightMajorGlyph_GlyphOfDancingRuneWeapon),
			int32(proto.DeathknightMajorGlyph_GlyphOfDeathStrike), int32(proto.DeathknightMajorGlyph_GlyphOfDisease)), dkOptions, false},
	{"Unholy", proto.Class_ClassDeathknight, proto.Race_RaceOrc, "deathknight", "p3_uh_dw", "uh_2h_ss",
		"-320043500002-2300303050032152000150013133051", glyphs(int32(proto.DeathknightMajorGlyph_GlyphOfTheGhoul),
			int32(proto.DeathknightMajorGlyph_GlyphOfDarkDeath), int32(proto.DeathknightMajorGlyph_GlyphOfDeathAndDecay)), dkOptions, false},
	{"Frost", proto.Class_ClassDeathknight, proto.Race_RaceOrc, "deathknight", "p3_frost", "frost_bl_pesti",
		"23050005-32005350352203012300033101351", glyphs(int32(proto.DeathknightMajorGlyph_GlyphOfFrostStrike),
			int32(proto.DeathknightMajorGlyph_GlyphOfObliterate), int32(proto.DeathknightMajorGlyph_GlyphOfDisease)), dkOptions, false},
	{"FrostUH", proto.Class_ClassDeathknight, proto.Race_RaceOrc, "deathknight", "p3_frost", "frost_uh_pesti",
		"01-32002350342203012300033101351-230200305003", glyphs(int32(proto.DeathknightMajorGlyph_GlyphOfFrostStrike),
			int32(proto.DeathknightMajorGlyph_GlyphOfObliterate), int32(proto.DeathknightMajorGlyph_GlyphOfDisease)), dkOptions, false},
	{"BloodTank", proto.Class_ClassDeathknight, proto.Race_RaceOrc, "tank_deathknight", "p1_blood", "blood_icy_touch",
		"005510153330330220102013-3050505100023101-002", glyphs(int32(proto.DeathknightMajorGlyph_GlyphOfDarkCommand),
			int32(proto.DeathknightMajorGlyph_GlyphOfObliterate), int32(proto.DeathknightMajorGlyph_GlyphOfVampiricBlood)),
		&proto.Player_TankDeathknight{TankDeathknight: &proto.TankDeathknight{Options: &proto.TankDeathknight_Options{}}}, true},

	{"Balance", proto.Class_ClassDruid, proto.Race_RaceTauren, "balance_druid", "p1", "basic_p3",
		"5012203115331303213315311231--205003012", glyphs(int32(proto.DruidMajorGlyph_GlyphOfStarfire),
			int32(proto.DruidMajorGlyph_GlyphOfInsectSwarm), int32(proto.DruidMajorGlyph_GlyphOfStarfall), int32(proto.DruidMinorGlyph_GlyphOfTyphoon)),
		&proto.Player_BalanceDruid{BalanceDruid: &proto.BalanceDruid{Options: &proto.BalanceDruid_Options{OkfUptime: 0.2}}}, false},
	{"BalancePhase3", proto.Class_ClassDruid, proto.Race_RaceTauren, "balance_druid", "p3_alliance", "basic_p3",
		"5102233115331303213305311031--205003002", glyphs(int32(proto.DruidMajorGlyph_GlyphOfStarfire),
			int32(proto.DruidMajorGlyph_GlyphOfMoonfire), int32(proto.DruidMajorGlyph_GlyphOfStarfall)),
		&proto.Player_BalanceDruid{BalanceDruid: &proto.BalanceDruid{Options: &proto.BalanceDruid_Options{OkfUptime: 0.2}}}, false},
	{"Feral", proto.Class_ClassDruid, proto.Race_RaceTauren, "feral_druid", "p3", "default",
		"-503202132322010053120230310511-205503012", glyphs(int32(proto.DruidMajorGlyph_GlyphOfOmenOfClarity),
			int32(proto.DruidMajorGlyph_GlyphOfShred), int32(proto.DruidMajorGlyph_GlyphOfBerserk), int32(proto.DruidMinorGlyph_GlyphOfTheWild)),
		&proto.Player_FeralDruid{FeralDruid: &proto.FeralDruid{
			Options:  &proto.FeralDruid_Options{InnervateTarget: &proto.UnitReference{}, LatencyMs: 100, AssumeBleedActive: true},
			Rotation: &proto.FeralDruid_Rotation{UseRake: true, UseBite: true, MinCombosForRip: 5, MinCombosForBite: 5, MaintainFaerieFire: true},
		}}, false},
	{"Restoration druid", proto.Class_ClassDruid, proto.Race_RaceTauren, "restoration_druid", "p1", "",
		"05320031103--230023312131502331050313051", glyphs(int32(proto.DruidMajorGlyph_GlyphOfWildGrowth),
			int32(proto.DruidMajorGlyph_GlyphOfSwiftmend), int32(proto.DruidMajorGlyph_GlyphOfNourish)),
		&proto.Player_RestorationDruid{RestorationDruid: &proto.RestorationDruid{Options: &proto.RestorationDruid_Options{
			InnervateTarget: &proto.UnitReference{Type: proto.UnitReference_Player, Index: 0}}}}, false},
	{"FeralTank", proto.Class_ClassDruid, proto.Race_RaceTauren, "feral_tank_druid", "p1", "default",
		"-503232132322010353120300313511-20350001", glyphs(int32(proto.DruidMajorGlyph_GlyphOfMaul),
			int32(proto.DruidMajorGlyph_GlyphOfSurvivalInstincts), int32(proto.DruidMajorGlyph_GlyphOfFrenziedRegeneration)),
		&proto.Player_FeralTankDruid{FeralTankDruid: &proto.FeralTankDruid{Options: &proto.FeralTankDruid_Options{
			InnervateTarget: &proto.UnitReference{}, StartingRage: 20}}}, true},

	{"BM", proto.Class_ClassHunter, proto.Race_RaceOrc, "hunter", "p1_sv", "bm",
		"51200201515012233110531351-005305-5", glyphs(int32(proto.HunterMajorGlyph_GlyphOfBestialWrath),
			int32(proto.HunterMajorGlyph_GlyphOfSteadyShot), int32(proto.HunterMajorGlyph_GlyphOfSerpentSting)), hunterOptions, false},
	{"MM", proto.Class_ClassHunter, proto.Race_RaceOrc, "hunter", "p1_mm", "mm",
		"502-035335131030013233035031051-5000002", glyphs(int32(proto.HunterMajorGlyph_GlyphOfSerpentSting),
			int32(proto.HunterMajorGlyph_GlyphOfSteadyShot), int32(proto.HunterMajorGlyph_GlyphOfChimeraShot)), hunterOptions, false},
	{"SV", proto.Class_ClassHunter, proto.Race_RaceOrc, "hunter", "p1_sv", "sv",
		"-015305101-5000032500033330532135301311", glyphs(int32(proto.HunterMajorGlyph_GlyphOfSerpentSting),
			int32(proto.HunterMajorGlyph_GlyphOfExplosiveShot), int32(proto.HunterMajorGlyph_GlyphOfKillShot)), hunterOptions, false},

	{"Arcane", proto.Class_ClassMage, proto.Race_RaceTroll, "mage", "p3_arcane_alliance", "arcane",
		"23000513310033015032310250532-03-023303001", glyphs(int32(proto.MageMajorGlyph_GlyphOfArcaneBlast),
			int32(proto.MageMajorGlyph_GlyphOfArcaneMissiles), int32(proto.MageMajorGlyph_GlyphOfMoltenArmor)),
		mageOptions(proto.Mage_Options_MoltenArmor), false},
	{"Fire", proto.Class_ClassMage, proto.Race_RaceTroll, "mage", "p3_fire_alliance", "fire",
		"23000503110003-0055030012303331053120301351", glyphs(int32(proto.MageMajorGlyph_GlyphOfFireball),
			int32(proto.MageMajorGlyph_GlyphOfMoltenArmor), int32(proto.MageMajorGlyph_GlyphOfLivingBomb)),
		mageOptions(proto.Mage_Options_MoltenArmor), false},
	{"FrostFire", proto.Class_ClassMage, proto.Race_RaceTroll, "mage", "p3_ffb_alliance", "frostfire",
		"23000503110003-0055030012303331053120301351", glyphs(int32(proto.MageMajorGlyph_GlyphOfFrostfire),
			int32(proto.MageMajorGlyph_GlyphOfMoltenArmor), int32(proto.MageMajorGlyph_GlyphOfLivingBomb)),
		mageOptions(proto.Mage_Options_MoltenArmor), false},
	{"Frost mage", proto.Class_ClassMage, proto.Race_RaceTroll, "mage", "p3_frost_alliance", "frost",
		"23000503110003--0533030310233100030152231351", glyphs(int32(proto.MageMajorGlyph_GlyphOfFrostbolt),
			int32(proto.MageMajorGlyph_GlyphOfMoltenArmor), int32(proto.MageMajorGlyph_GlyphOfEternalWater)),
		mageOptions(proto.Mage_Options_MageArmor), false},

	{"Holy paladin", proto.Class_ClassPaladin, proto.Race_RaceBloodElf, "holy_paladin", "p1", "",
		"50350151020013053100515221-50023131203", glyphs(int32(proto.PaladinMajorGlyph_GlyphOfHolyLight),
			int32(proto.PaladinMajorGlyph_GlyphOfSealOfWisdom), int32(proto.PaladinMajorGlyph_GlyphOfBeaconOfLight),
			int32(proto.PaladinMinorGlyph_GlyphOfLayOnHands), int32(proto.PaladinMinorGlyph_GlyphOfSenseUndead)),
		&proto.Player_HolyPaladin{HolyPaladin: &proto.HolyPaladin{Options: &proto.HolyPaladin_Options{
			Judgement: proto.PaladinJudgement_JudgementOfWisdom, Aura: proto.PaladinAura_DevotionAura}}}, false},
	{"Protection paladin", proto.Class_ClassPaladin, proto.Race_RaceBloodElf, "protection_paladin", "p1", "default",
		"-05005135200132311333312321-511302012003", glyphs(int32(proto.PaladinMajorGlyph_GlyphOfSealOfVengeance),
			int32(proto.PaladinMajorGlyph_GlyphOfRighteousDefense), int32(proto.PaladinMajorGlyph_GlyphOfDivinePlea),
			int32(proto.PaladinMinorGlyph_GlyphOfLayOnHands), int32(proto.PaladinMinorGlyph_GlyphOfSenseUndead)),
		&proto.Player_ProtectionPaladin{ProtectionPaladin: &proto.ProtectionPaladin{Options: &proto.ProtectionPaladin_Options{
			Judgement: paladinOptions.Judgement, Seal: paladinOptions.Seal, Aura: paladinOptions.Aura}}}, true},
	{"Retribution", proto.Class_ClassPaladin, proto.Race_RaceBloodElf, "retribution_paladin", "p1", "default",
		"050501-05-05232051203331302133231331", glyphs(int32(proto.PaladinMajorGlyph_GlyphOfSealOfVengeance),
			int32(proto.PaladinMajorGlyph_GlyphOfJudgement), int32(proto.PaladinMajorGlyph_GlyphOfConsecration),
			int32(proto.PaladinMinorGlyph_GlyphOfSenseUndead), int32(proto.PaladinMinorGlyph_GlyphOfLayOnHands),
			int32(proto.PaladinMinorGlyph_GlyphOfBlessingOfKings)),
		&proto.Player_RetributionPaladin{RetributionPaladin: &proto.RetributionPaladin{Options: paladinOptions}}, false},

	{"Disc", proto.Class_ClassPriest, proto.Race_RaceUndead, "healing_priest", "p1_disc", "disc",
		"0503203130300512301313231251-2351010303", glyphs(int32(proto.PriestMajorGlyph_GlyphOfPowerWordShield),
			int32(proto.PriestMajorGlyph_GlyphOfFlashHeal), int32(proto.PriestMajorGlyph_GlyphOfPenance)),
		&proto.Player_HealingPriest{HealingPriest: &proto.HealingPriest{Options: priestHealer}}, false},
	{"Holy priest", proto.Class_ClassPriest, proto.Race_RaceUndead, "healing_priest", "p1_holy", "holy",
		"05032031103-234051032002152530004311051", glyphs(int32(proto.PriestMajorGlyph_GlyphOfPrayerOfHealing),
			int32(proto.PriestMajorGlyph_GlyphOfRenew), int32(proto.PriestMajorGlyph_GlyphOfCircleOfHealing)),
		&proto.Player_HealingPriest{HealingPriest: &proto.HealingPriest{Options: priestHealer}}, false},
	{"Shadow", proto.Class_ClassPriest, proto.Race_RaceUndead, "shadow_priest", "p1", "default",
		"05032031--325023051223010323151301351", glyphs(int32(proto.PriestMajorGlyph_GlyphOfShadow),
			int32(proto.PriestMajorGlyph_GlyphOfMindFlay), int32(proto.PriestMajorGlyph_GlyphOfDispersion)),
		&proto.Player_ShadowPriest{ShadowPriest: &proto.ShadowPriest{Options: &proto.ShadowPriest_Options{
			Armor: proto.ShadowPriest_Options_InnerFire}}}, false},
	{"Smite", proto.Class_ClassPriest, proto.Race_RaceUndead, "smite_priest", "p1", "default",
		"05332031013005023310001-005551002020152-00502", glyphs(int32(proto.PriestMajorGlyph_GlyphOfSmite),
			int32(proto.PriestMajorGlyph_GlyphOfHolyNova), int32(proto.PriestMajorGlyph_GlyphOfShadowWordDeath)),
		&proto.Player_SmitePriest{SmitePriest: &proto.SmitePriest{Options: &proto.SmitePriest_Options{
			UseInnerFire: true, UseShadowfiend: true}}}, false},

	{"Combat", proto.Class_ClassRogue, proto.Race_RaceHuman, "rogue", "p1_combat", "combat_expose",
		"00532000523-0252051050035010223100501251", glyphs(int32(proto.RogueMajorGlyph_GlyphOfKillingSpree),
			int32(proto.RogueMajorGlyph_GlyphOfTricksOfTheTrade), int32(proto.RogueMajorGlyph_GlyphOfRupture)),
		rogueOptions(proto.Rogue_Options_DeadlyPoison, proto.Rogue_Options_InstantPoison), false},
	{"Assassination", proto.Class_ClassRogue, proto.Race_RaceHuman, "rogue", "p1_assassination", "rupture_mutilate_expose",
		"005303005352100520103331051-005005003-502", glyphs(int32(proto.RogueMajorGlyph_GlyphOfMutilate),
			int32(proto.RogueMajorGlyph_GlyphOfTricksOfTheTrade), int32(proto.RogueMajorGlyph_GlyphOfHungerForBlood)),
		rogueOptions(proto.Rogue_Options_DeadlyPoison, proto.Rogue_Options_InstantPoison), false},
	{"Subtlety", proto.Class_ClassRogue, proto.Race_RaceBloodElf, "rogue", "p2_hemosub", "combat_expose",
		"30532000235--512003203032012135011503113", glyphs(int32(proto.RogueMajorGlyph_GlyphOfHemorrhage),
			int32(proto.RogueMajorGlyph_GlyphOfEviscerate), int32(proto.RogueMajorGlyph_GlyphOfRupture)),
		rogueOptions(proto.Rogue_Options_InstantPoison, proto.Rogue_Options_DeadlyPoison), false},

	{"Elemental", proto.Class_ClassShaman, proto.Race_RaceTroll, "elemental_shaman", "p1", "default",
		"0532001523212351322301351-005052031", glyphs(int32(proto.ShamanMajorGlyph_GlyphOfLava),
			int32(proto.ShamanMajorGlyph_GlyphOfTotemOfWrath), int32(proto.ShamanMajorGlyph_GlyphOfLightningBolt)),
		&proto.Player_ElementalShaman{ElementalShaman: &proto.ElementalShaman{Options: &proto.ElementalShaman_Options{
			Shield: proto.ShamanShield_WaterShield, Totems: shamanTotems(proto.FireTotem_TotemOfWrath, true)}}}, false},
	{"Enhancement", proto.Class_ClassShaman, proto.Race_RaceTroll, "enhancement_shaman", "p1", "default_ft",
		"053030152-30405003105021333031131031051", glyphs(int32(proto.ShamanMajorGlyph_GlyphOfFireNova),
			int32(proto.ShamanMajorGlyph_GlyphOfFlametongueWeapon), int32(proto.ShamanMajorGlyph_GlyphOfFeralSpirit)),
		&proto.Player_EnhancementShaman{EnhancementShaman: &proto.EnhancementShaman{Options: &proto.EnhancementShaman_Options{
			Shield: proto.ShamanShield_LightningShield, SyncType: proto.ShamanSyncType_Auto,
			ImbueMh: proto.ShamanImbue_FlametongueWeaponDownrank, ImbueOh: proto.ShamanImbue_FlametongueWeapon,
			Totems: &proto.ShamanTotems{Earth: proto.EarthTotem_StrengthOfEarthTotem, Air: proto.AirTotem_WindfuryTotem,
				Water: proto.WaterTotem_ManaSpringTotem, Fire: proto.FireTotem_MagmaTotem, UseFireElemental: true}}}}, false},
	{"Restoration shaman", proto.Class_ClassShaman, proto.Race_RaceTroll, "restoration_shaman", "p1", "",
		"-3020503-50005331335310501122331251", glyphs(int32(proto.ShamanMajorGlyph_GlyphOfChainHeal),
			int32(proto.ShamanMajorGlyph_GlyphOfEarthShield), int32(proto.ShamanMajorGlyph_GlyphOfEarthlivingWeapon)),
		&proto.Player_RestorationShaman{RestorationShaman: &proto.RestorationShaman{Options: &proto.RestorationShaman_Options{
			Shield: proto.ShamanShield_WaterShield, Totems: shamanTotems(proto.FireTotem_FlametongueTotem, false)}}}, false},

	{"Affliction", proto.Class_ClassWarlock, proto.Race_RaceOrc, "warlock", "p4_affliction", "affliction",
		"2350002030023510253500331151--550000051", glyphs(int32(proto.WarlockMajorGlyph_GlyphOfQuickDecay),
			int32(proto.WarlockMajorGlyph_GlyphOfLifeTap), int32(proto.WarlockMajorGlyph_GlyphOfHaunt)),
		warlockOptions(proto.Warlock_Options_Felhunter, proto.Warlock_Options_GrandSpellstone), false},
	{"Demonology", proto.Class_ClassWarlock, proto.Race_RaceOrc, "warlock", "p4_demo", "demo",
		"-203203301035012530135201351-550000052", glyphs(int32(proto.WarlockMajorGlyph_GlyphOfQuickDecay),
			int32(proto.WarlockMajorGlyph_GlyphOfLifeTap), int32(proto.WarlockMajorGlyph_GlyphOfFelguard)),
		warlockOptions(proto.Warlock_Options_Felguard, proto.Warlock_Options_GrandSpellstone), false},
	{"Destruction", proto.Class_ClassWarlock, proto.Race_RaceOrc, "warlock", "p4_destro", "destro",
		"-03310030003-05203205210331051335230351", glyphs(int32(proto.WarlockMajorGlyph_GlyphOfConflagrate),
			int32(proto.WarlockMajorGlyph_GlyphOfLifeTap), int32(proto.WarlockMajorGlyph_GlyphOfIncinerate)),
		warlockOptions(proto.Warlock_Options_Imp, proto.Warlock_Options_GrandFirestone), false},

	{"Fury", proto.Class_ClassWarrior, proto.Race_RaceOrc, "warrior", "p1_fury", "fury",
		"302023102331-305053000520310053120500351", glyphs(int32(proto.WarriorMajorGlyph_GlyphOfWhirlwind),
			int32(proto.WarriorMajorGlyph_GlyphOfHeroicStrike), int32(proto.WarriorMajorGlyph_GlyphOfRending),
			int32(proto.WarriorMinorGlyph_GlyphOfShatteringThrow)), warriorOptions, false},
	{"Arms", proto.Class_ClassWarrior, proto.Race_RaceOrc, "warrior", "p1_arms", "arms",
		"3022032023335100102012213231251-305-2033", glyphs(int32(proto.WarriorMajorGlyph_GlyphOfRending),
			int32(proto.WarriorMajorGlyph_GlyphOfMortalStrike), int32(proto.WarriorMajorGlyph_GlyphOfExecution),
			int32(proto.WarriorMinorGlyph_GlyphOfShatteringThrow)), warriorOptions, false},
	{"ProtectionWarrior", proto.Class_ClassWarrior, proto.Race_RaceOrc, "protection_warrior", "p1_balanced", "default",
		"2500030023-302-053351225000012521030113321", glyphs(int32(proto.WarriorMajorGlyph_GlyphOfBlocking),
			int32(proto.WarriorMajorGlyph_GlyphOfDevastate), int32(proto.WarriorMajorGlyph_GlyphOfVigilance)),
		&proto.Player_ProtectionWarrior{ProtectionWarrior: &proto.ProtectionWarrior{Options: &proto.ProtectionWarrior_Options{
			Shout: proto.WarriorShout_WarriorShoutCommanding}}}, true},
}

var hunterOptions = &proto.Player_Hunter{Hunter: &proto.Hunter{Options: &proto.Hunter_Options{
	Ammo:    proto.Hunter_Options_SaroniteRazorheads,
	PetType: proto.Hunter_Options_Wolf,
	PetTalents: &proto.HunterPetTalents{CobraReflexes: 2, Dive: true, SpikedCollar: 3, BoarsSpeed: true,
		CullingTheHerd: 3, SpidersBite: 3, Rabid: true, CallOfTheWild: true, WildHunt: 1},
	PetUptime:            0.9,
	TimeToTrapWeaveMs:    2000,
	SniperTrainingUptime: 0.8,
	UseHuntersMark:       true,
}}}

// everyPetTalent is one point in every hunter pet talent, for the few that gate an ability. One point is
// enough: a gate only checks that the talent is taken.
var everyPetTalent = func() *proto.HunterPetTalents {
	talents := (&proto.HunterPetTalents{}).ProtoReflect()
	fields := talents.Descriptor().Fields()
	for i := range fields.Len() {
		switch field := fields.Get(i); field.Kind() {
		case protoreflect.BoolKind:
			talents.Set(field, protoreflect.ValueOfBool(true))
		case protoreflect.Int32Kind:
			talents.Set(field, protoreflect.ValueOfInt32(1))
		}
	}
	return talents.Interface().(*proto.HunterPetTalents)
}()

// Everything that registers a consumable, whichever class it suits.
var serverDataConsumes = &proto.Consumes{
	Flask:           proto.Flask_FlaskOfEndlessRage,
	Food:            proto.Food_FoodFishFeast,
	DefaultPotion:   proto.Potions_PotionOfSpeed,
	PrepopPotion:    proto.Potions_PotionOfSpeed,
	DefaultConjured: proto.Conjured_ConjuredDarkRune,
	PetFood:         proto.PetFood_PetFoodKiblersBits,
	ThermalSapper:   true,
	FillerExplosive: proto.Explosive_ExplosiveSaroniteBomb,
}

// Every external cooldown once, on top of the golden suites' buffs.
var serverDataBuffs = func() *proto.IndividualBuffs {
	b := googleProto.Clone(core.FullIndividualBuffs).(*proto.IndividualBuffs)
	b.HymnOfHope, b.HandOfSalvation, b.Rapture, b.Innervates, b.PowerInfusions, b.UnholyFrenzy = 1, 1, 1, 1, 1, 1
	b.RevitalizeRejuvination, b.RevitalizeWildGrowth, b.TricksOfTheTrades, b.DivineGuardians = 1, 1, 1, 1
	b.PainSuppressions, b.HandOfSacrifices, b.GuardianSpirits, b.ShatteringThrows = 1, 1, 1, 1
	b.FocusMagic = true
	return b
}()

var healerRotation = core.APLRotationFromJsonString(`{"type":"TypeAPL","priorityList":[{"action":{"autocastOtherCooldowns":{}}}]}`)

func presetNamed(name string) serverDataPreset {
	return serverDataPresets[slices.IndexFunc(serverDataPresets, func(p serverDataPreset) bool { return p.name == name })]
}

func (p serverDataPreset) raid() *proto.Raid {
	rotation := healerRotation
	if p.apl != "" {
		rotation = core.GetAplRotation("../ui/"+p.ui+"/apls", p.apl).Rotation
	}
	player := core.WithSpec(&proto.Player{
		Name:               p.name,
		Class:              p.class,
		Race:               p.race,
		Equipment:          core.GetGearSet("../ui/"+p.ui+"/gear_sets", p.gear).GearSet,
		Consumes:           serverDataConsumes,
		Buffs:              serverDataBuffs,
		TalentsString:      p.talents,
		Glyphs:             p.glyphs,
		Profession1:        proto.Profession_Engineering,
		Rotation:           rotation,
		InFrontOfTarget:    p.tank,
		DistanceFromTarget: 30,
		ReactionTimeMs:     150,
		ChannelClipDelayMs: 50,
	}, p.spec)
	raid := core.SinglePlayerRaidProto(player, core.FullPartyBuffs, core.FullRaidBuffs, core.FullDebuffs)
	if p.tank {
		raid.Tanks = append(raid.Tanks, &proto.UnitReference{Type: proto.UnitReference_Player, Index: 0})
	}
	return raid
}

// The class and shared allowlists are registered by init functions the imports of register_all.go run.
func TestServerDataConflicts(t *testing.T) {
	used := map[*core.ServerConflictAllowance]bool{}
	registered := map[core.ActionID]bool{}
	uncovered := map[string]string{} // entry to add, first preset that needs it
	mismatched := map[string]string{}

	check := func(label string, env *core.Environment) {
		for _, unit := range env.AllUnits {
			for _, spell := range unit.Spellbook {
				registered[spell.ActionID] = true
				for _, c := range spell.ServerConflicts() {
					switch a := c.Allowed; {
					case a == nil:
						spell := fmt.Sprintf("core.ActionID{SpellID: %d}", c.Spell.SpellID)
						if c.Spell.Tag != 0 {
							spell = fmt.Sprintf("core.ActionID{SpellID: %d, Tag: %d}", c.Spell.SpellID, c.Spell.Tag)
						}
						line := fmt.Sprintf("{Spell: %s, Field: core.Server%s, Sim: %d, Server: %d, Why: \"\"},",
							spell, c.Field, c.Sim, c.Server)
						if _, ok := uncovered[line]; !ok {
							uncovered[line] = label + ", " + unit.Label
						}
					case a.Sim != c.Sim || a.Server != c.Server:
						mismatched[fmt.Sprintf("%s, the entry says sim %d, server %d", c, a.Sim, a.Server)] = label + ", " + unit.Label
					default:
						used[a] = true
					}
				}
			}
		}
	}

	presets := slices.Clone(serverDataPresets)
	// every pet a hunter or warlock can bring
	for name, pet := range proto.Hunter_Options_PetType_value {
		if p := presetNamed("BM"); pet != 0 {
			options := googleProto.Clone(hunterOptions.Hunter).(*proto.Hunter)
			options.Options.PetType = proto.Hunter_Options_PetType(pet)
			p.name, p.spec = "BM with a "+name, &proto.Player_Hunter{Hunter: options}
			presets = append(presets, p)
		}
	}
	for name, summon := range proto.Warlock_Options_Summon_value {
		if p := presetNamed("Affliction"); summon != 0 {
			p.name, p.spec = "Affliction with a "+name, warlockOptions(proto.Warlock_Options_Summon(summon), proto.Warlock_Options_GrandSpellstone)
			presets = append(presets, p)
		}
	}
	// every race on every class: the presets between them pick six of the ten, and a racial can register
	// a different spell per class (Arcane Torrent, Blood Fury)
	swept := map[proto.Class]bool{}
	for _, p := range serverDataPresets {
		if swept[p.class] {
			continue
		}
		swept[p.class] = true
		for name, race := range proto.Race_value {
			if race != 0 {
				v := p
				v.name, v.race = fmt.Sprintf("%s as a %s", p.name, name), proto.Race(race)
				presets = append(presets, v)
			}
		}
	}
	// the hunter pet talents that gate an ability
	{
		p := presetNamed("BM")
		options := googleProto.Clone(hunterOptions.Hunter).(*proto.Hunter)
		options.Options.PetTalents = everyPetTalent
		p.name, p.spec = "BM with every pet talent", &proto.Player_Hunter{Hunter: options}
		presets = append(presets, p)
	}
	// every glyph each class can take
	for _, p := range serverDataPresets {
		presets = append(presets, glyphVariants(p)...)
	}
	for _, p := range presets {
		env, _, _ := core.NewEnvironment(p.raid(), core.MakeSingleTargetEncounter(0), false)
		check(p.name, env)
	}
	// every boss AI, against a tank
	tank := presetNamed("ProtectionWarrior")
	for _, encounter := range core.PresetEncounters {
		targets := make([]*proto.Target, len(encounter.Targets))
		for i, target := range encounter.Targets {
			targets[i] = target.Target
		}
		env, _, _ := core.NewEnvironment(tank.raid(), &proto.Encounter{Duration: 300, Targets: targets}, false)
		check(encounter.Path, env)
	}

	for _, line := range slices.Sorted(maps.Keys(uncovered)) {
		t.Errorf("no entry covers this conflict (%s):\n\t%s", uncovered[line], line)
	}
	for _, line := range slices.Sorted(maps.Keys(mismatched)) {
		t.Errorf("stale entry values (%s): %s", mismatched[line], line)
	}
	for _, a := range core.ServerConflictAllowances() {
		switch {
		case used[a]:
		case registered[a.Spell]:
			t.Errorf("%s %s registers without that conflict now, so its entry is stale", a.Spell, a.Field)
		default:
			t.Logf("no preset registers %s, so its %s entry is unchecked", a.Spell, a.Field)
		}
		if strings.TrimSpace(a.Why) == "" {
			t.Errorf("%s %s: the entry needs a reason", a.Spell, a.Field)
		}
	}
}

// RegisterSpell against spells no entry covers, and one an entry keeps as declared.
func TestServerDataApplied(t *testing.T) {
	env, _, _ := core.NewEnvironment(presetNamed("Fire").raid(), core.MakeSingleTargetEncounter(0), false)
	unit := &env.Raid.Parties[0].Players[0].GetCharacter().Unit
	register := func(config core.SpellConfig) *core.Spell {
		t.Helper()
		config.ActionID.Tag = 99 // only the untagged entries reach it
		spell := unit.RegisterSpell(config)
		if spell.ServerSpell() == nil {
			t.Fatalf("%s has no server data", spell.ActionID)
		}
		return spell
	}
	onGCD := core.CastConfig{DefaultCast: core.Cast{GCD: core.GCDDefault}}

	// Frostbolt: 3 s, and talents or glyphs reach 2.4 s at the least
	frostbolt := register(core.SpellConfig{ActionID: core.ActionID{SpellID: 42842},
		Cast: core.CastConfig{DefaultCast: core.Cast{GCD: core.GCDDefault, CastTime: 2500 * time.Millisecond}}})
	if frostbolt.DefaultCast.CastTime != 2500*time.Millisecond || len(frostbolt.ServerConflicts()) != 0 {
		t.Errorf("a cast time talents reach stays: %v, %v", frostbolt.DefaultCast.CastTime, frostbolt.ServerConflicts())
	}
	frostbolt = register(core.SpellConfig{ActionID: core.ActionID{SpellID: 42842}, Flags: core.SpellFlagBinary,
		Cast: core.CastConfig{DefaultCast: core.Cast{GCD: core.GCDDefault, CastTime: time.Second}}})
	if frostbolt.DefaultCast.CastTime != 3*time.Second || frostbolt.Flags.Matches(core.SpellFlagBinary) {
		t.Errorf("the server's cast time and flags apply: %v, %b", frostbolt.DefaultCast.CastTime, frostbolt.Flags)
	}
	if got := frostbolt.ServerConflicts(); len(got) != 2 || got[0].Field != core.ServerBinary || got[1].Field != core.ServerCastTime ||
		got[1].Sim != 1000 || got[1].Server != 3000 || got[1].Allowed != nil {
		t.Errorf("conflicts %v", got)
	}

	// Steady Shot is binary on the server; an undeclared flag is just added
	steady := register(core.SpellConfig{ActionID: core.ActionID{SpellID: 49052},
		Cast: core.CastConfig{DefaultCast: core.Cast{GCD: core.GCDDefault, CastTime: 2 * time.Second}}})
	if !steady.Flags.Matches(core.SpellFlagBinary) || len(steady.ServerConflicts()) != 0 {
		t.Errorf("Steady Shot: %b, %v", steady.Flags, steady.ServerConflicts())
	}

	// Blood Plague: SPELL_ATTR3_ALWAYS_HIT, triggered, so no timing is checked
	bloodPlague := register(core.SpellConfig{ActionID: core.ActionID{SpellID: 55078}})
	if !bloodPlague.Flags.Matches(core.SpellFlagAlwaysHit) || len(bloodPlague.ServerConflicts()) != 0 {
		t.Errorf("Blood Plague: %b, %v", bloodPlague.Flags, bloodPlague.ServerConflicts())
	}

	// Envenom: SPELL_ATTR4_NO_CAST_LOG means no partial resists, but a physical spell keeps its armor
	for school, want := range map[core.SpellSchool]bool{core.SpellSchoolNature: true, core.SpellSchoolPhysical: false} {
		envenom := register(core.SpellConfig{ActionID: core.ActionID{SpellID: 57993}, SpellSchool: school})
		if envenom.Flags.Matches(core.SpellFlagIgnoreResists) != want {
			t.Errorf("Envenom as %v: IgnoreResists %v, want %v", school, !want, want)
		}
	}

	// Mortal Strike: a 6 s category cooldown the sim left out gets its own timer
	mortalStrike := register(core.SpellConfig{ActionID: core.ActionID{SpellID: 47486}, Cast: onGCD})
	if mortalStrike.CD.Timer == nil || mortalStrike.CD.Duration != 6*time.Second {
		t.Errorf("Mortal Strike cooldown %+v", mortalStrike.CD)
	}

	// Spirit Strike: its 1.5 s GCD has no StartRecoveryCategory, so it holds back nothing the unit's GCD does
	spiritStrike := register(core.SpellConfig{ActionID: core.ActionID{SpellID: 61198},
		Cast: core.CastConfig{CD: core.Cooldown{Timer: unit.NewTimer(), Duration: 10 * time.Second}}})
	if spiritStrike.DefaultCast.GCD != 0 || len(spiritStrike.ServerConflicts()) != 0 {
		t.Errorf("Spirit Strike: GCD %v, %v", spiritStrike.DefaultCast.GCD, spiritStrike.ServerConflicts())
	}

	// Death Coil: the DK entry turns the binary flag of the dummy cast down, and it still counts as a conflict
	deathCoil := register(core.SpellConfig{ActionID: core.ActionID{SpellID: 49895}, Cast: onGCD})
	if got := deathCoil.ServerConflicts(); deathCoil.Flags.Matches(core.SpellFlagBinary) || len(got) != 1 ||
		got[0].Field != core.ServerBinary || got[0].Sim != 0 || got[0].Server != 1 || got[0].Allowed == nil {
		t.Errorf("Death Coil: %b, %v", deathCoil.Flags, got)
	}

	optOut := unit.RegisterSpell(core.SpellConfig{ActionID: core.ActionID{SpellID: 42842, Tag: 99},
		Flags: core.SpellFlagNoServerData | core.SpellFlagBinary, Cast: onGCD})
	if optOut.ServerSpell() != nil || !optOut.Flags.Matches(core.SpellFlagBinary) || optOut.DefaultCast.CastTime != 0 {
		t.Errorf("SpellFlagNoServerData leaves the spell alone: %b, %v", optOut.Flags, optOut.DefaultCast)
	}

	missile := unit.RegisterSpell(core.SpellConfig{ActionID: core.ActionID{OtherID: proto.OtherAction_OtherActionAttack, Tag: 99},
		MissileSpeed: 24})
	for distance, want := range map[float64]time.Duration{0: 208 * time.Millisecond, 30: 1250 * time.Millisecond} {
		unit.DistanceFromTarget = distance
		if got := missile.TravelTime(); got != want {
			t.Errorf("travel time at %v yd: %v, want %v", distance, got, want)
		}
	}
}
