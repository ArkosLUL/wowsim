package optimizer

import (
	"bufio"
	"math"
	"os"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
	"github.com/wowsims/wotlk/sim/core/stats"
	"github.com/wowsims/wotlk/sim/encounters"
)

type tankBoss struct {
	encounters.OptimizerTankBoss
	difficulty proto.RaidDifficulty
}

func ability(id int32, school proto.SpellSchool, damage, periodMin, periodMax float64) encounters.OptimizerTankAbility {
	return encounters.OptimizerTankAbility{SpellID: id, School: school, Damage: damage, PeriodMin: periodMin, PeriodMax: periodMax}
}

// missable is ability for a spell that rolls the server's magic hit.
func missable(id int32, school proto.SpellSchool, damage, periodMin, periodMax float64) encounters.OptimizerTankAbility {
	a := ability(id, school, damage, periodMin, periodMax)
	a.CanMiss = true
	return a
}

// The tank bosses ui/core/optimizer/pool_builder.ts sends per content phase, off each phase boss's
// live creature_template row (DamageModifier, BaseAttackTime; 0 falls back to the server's 2000 ms
// default) and boss script: Kel'Thuzad 25 (30061), Algalon 25 (33070), Anub'arak 25H (35616), the
// Lich King 25H (39168), Halion 25H (39945). TestTankBossesMirrorUI checks this stays in sync with the
// UI's table.
var tankBosses = map[int32]tankBoss{
	1: {encounters.OptimizerTankBoss{
		Name: "Kel'Thuzad", DamageModifier: 70, AttackTimeMs: 2000, DamageExp2: 177.438, AttackPower: 783,
		Abilities: []encounters.OptimizerTankAbility{
			{SpellID: 55802, School: proto.SpellSchool_SpellSchoolFrost, Damage: 30000, PeriodMin: 2, PeriodMax: 10, CastSeconds: 2, LandChance: 0.5, ResetsSwing: true, CanMiss: true},
			missable(55807, proto.SpellSchool_SpellSchoolFrost, 8000, 15, 30),
		},
	}, proto.RaidDifficulty_RaidDifficulty25Normal},
	2: {encounters.OptimizerTankBoss{
		Name: "Algalon", DamageModifier: 285, AttackTimeMs: 1000, DualWield: true, NoParryHaste: true,
		Abilities: []encounters.OptimizerTankAbility{
			missable(64592, proto.SpellSchool_SpellSchoolPhysical, 35000, 3, 4.5),
			ability(64412, proto.SpellSchool_SpellSchoolArcane, 9500, 15.5, 15.5),
			missable(65108, proto.SpellSchool_SpellSchoolShadow, 21000, 25, 25),
		},
	}, proto.RaidDifficulty_RaidDifficulty25Normal},
	3: {encounters.OptimizerTankBoss{
		Name: "Anub'arak", DamageModifier: 149.1, NoParryHaste: true,
		Abilities: []encounters.OptimizerTankAbility{
			{SpellID: 66012, School: proto.SpellSchool_SpellSchoolFrost, Damage: 20788, PeriodMin: 15, PeriodMax: 20, ResetsSwing: true},
			{SpellID: 68510, School: proto.SpellSchool_SpellSchoolFrost, Damage: 6000, PeriodMin: 3, PeriodMax: 3, LandChance: 0.18, CanMiss: true},
		},
	}, proto.RaidDifficulty_RaidDifficulty25Heroic},
	4: {encounters.OptimizerTankBoss{
		Name: "the Lich King", DamageModifier: 492.9, AttackTimeMs: 1500, NoParryHaste: true, SuppressDodge: true,
		Abilities: []encounters.OptimizerTankAbility{
			{SpellID: 73799, School: proto.SpellSchool_SpellSchoolShadow, Damage: 103081, PeriodMin: 30.5, PeriodMax: 30.5, ResetsSwing: true,
				FollowUpSeconds: 5, FollowUpDamage: 70000, FollowUpHaste: 2, FollowUpHasteSeconds: 5, SwapsTank: true},
			{SpellID: 73781, School: proto.SpellSchool_SpellSchoolShadow, Damage: 18750, PeriodMin: 22.5, PeriodMax: 22.5, CastSeconds: 2, ResetsSwing: true, CanMiss: true, Raidwide: true},
		},
	}, proto.RaidDifficulty_RaidDifficulty25Heroic},
	5: {encounters.OptimizerTankBoss{
		Name: "Halion", DamageModifier: 385.4, AttackTimeMs: 1500, HitChance: 5, EnemyDodgeMultiplier: 0.8,
		Abilities: []encounters.OptimizerTankAbility{
			{SpellID: 74528, School: proto.SpellSchool_SpellSchoolFire, Damage: 45000, PeriodMin: 10, PeriodMax: 12, CastSeconds: 2, ResetsSwing: true, CanMiss: true},
		},
	}, proto.RaidDifficulty_RaidDifficulty25Heroic},
}

func tankEncounter(tb testing.TB, phase int32) *proto.Encounter {
	tb.Helper()
	boss, ok := tankBosses[phase]
	if !ok {
		tb.Fatalf("no tank boss for phase %d", phase)
	}
	return &proto.Encounter{
		Duration:             180,
		DurationVariation:    5,
		ExecuteProportion_20: 0.2,
		ExecuteProportion_25: 0.25,
		ExecuteProportion_35: 0.35,
		Targets:              []*proto.Target{encounters.OptimizerTankBossTarget(boss.OptimizerTankBoss)},
		RaidDifficulty:       boss.difficulty,
	}
}

// tankHealing is the healing a tank takes from the boss it fights, the way the UI's pool builder
// derives it per phase (ui/core/optimizer/pool_builder.ts healingModelForBoss): a cadence with some
// variation and no HPS, which the optimizer's pinHealingModel fills in from the gear as equipped.
func tankHealing(boss *proto.Target) *proto.HealingModel {
	cadence := 1.5 * boss.SwingSpeed
	if boss.DualWield {
		cadence /= 2
	}
	return &proto.HealingModel{CadenceSeconds: cadence, CadenceVariation: tankHealCadenceVariation * cadence, BurstWindow: 6}
}

// ui/core/optimizer/pool_builder.ts TANK_HEAL_CADENCE_VARIATION.
const tankHealCadenceVariation = 0.5

var (
	bossHeader  = regexp.MustCompile(`^\s*(\d+):\s*\{\s*$`)
	bossField   = regexp.MustCompile(`^\s*(\w+):\s*(.+?),\s*$`)
	abilityLine = regexp.MustCompile(`^\s*\{(.*)\},\s*$`)
	// one `key: value` of an ability line; values are quoted strings or bare words and numbers
	abilityPair = regexp.MustCompile(`(\w+):\s*('[^']*'|"[^"]*"|[\w.]+)`)
)

func uiNumber(tb testing.TB, raw string) float64 {
	tb.Helper()
	v, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		tb.Fatalf("bad number %q: %v", raw, err)
	}
	return v
}

func uiSchool(tb testing.TB, raw string) proto.SpellSchool {
	tb.Helper()
	school, ok := proto.SpellSchool_value[strings.TrimPrefix(raw, "SpellSchool.")]
	if !ok {
		tb.Fatalf("unknown SpellSchool %q", raw)
	}
	return proto.SpellSchool(school)
}

func readTankBossesFromUI(tb testing.TB) map[int32]tankBoss {
	tb.Helper()
	f, err := os.Open("../../ui/core/optimizer/pool_builder.ts")
	if err != nil {
		tb.Fatal(err)
	}
	defer f.Close()

	result := map[int32]tankBoss{}
	started, inAbilities := false, false
	var phase int32
	var cur *tankBoss
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)
		if !started {
			started = strings.HasPrefix(trimmed, "export const TANK_BOSSES")
			continue
		}
		switch {
		case cur == nil && strings.HasPrefix(trimmed, "};"):
			if len(result) == 0 {
				tb.Fatal("TANK_BOSSES has no boss")
			}
			return result
		case cur == nil:
			if m := bossHeader.FindStringSubmatch(line); m != nil {
				n, _ := strconv.Atoi(m[1])
				phase, cur = int32(n), &tankBoss{}
			}
		case inAbilities && trimmed == "],":
			inAbilities = false
		case inAbilities:
			m := abilityLine.FindStringSubmatch(line)
			if m == nil {
				if !strings.HasPrefix(trimmed, "//") {
					tb.Fatalf("unreadable ability line %q", line)
				}
				continue
			}
			a := encounters.OptimizerTankAbility{}
			for _, kv := range abilityPair.FindAllStringSubmatch(m[1], -1) {
				key, raw := kv[1], strings.Trim(kv[2], `'"`)
				switch key {
				case "name":
				case "spellId":
					a.SpellID = int32(uiNumber(tb, raw))
				case "school":
					a.School = uiSchool(tb, raw)
				case "damage":
					a.Damage = uiNumber(tb, raw)
				case "periodMin":
					a.PeriodMin = uiNumber(tb, raw)
				case "periodMax":
					a.PeriodMax = uiNumber(tb, raw)
				case "castSeconds":
					a.CastSeconds = uiNumber(tb, raw)
				case "landChance":
					a.LandChance = uiNumber(tb, raw)
				case "resetsSwing":
					a.ResetsSwing = raw == "true"
				case "followUpSeconds":
					a.FollowUpSeconds = uiNumber(tb, raw)
				case "followUpDamage":
					a.FollowUpDamage = uiNumber(tb, raw)
				case "followUpHaste":
					a.FollowUpHaste = uiNumber(tb, raw)
				case "followUpHasteSeconds":
					a.FollowUpHasteSeconds = uiNumber(tb, raw)
				case "canMiss":
					a.CanMiss = raw == "true"
				case "swapsTank":
					a.SwapsTank = raw == "true"
				case "raidwide":
					a.Raidwide = raw == "true"
				default:
					tb.Fatalf("unknown ability field %q in %q", key, line)
				}
			}
			cur.Abilities = append(cur.Abilities, a)
		case trimmed == "abilities: [":
			inAbilities = true
		case trimmed == "},":
			result[phase], cur = *cur, nil
		default:
			m := bossField.FindStringSubmatch(line)
			if m == nil {
				tb.Fatalf("unreadable TANK_BOSSES line %q", line)
			}
			key, raw := m[1], strings.Trim(m[2], `'"`)
			switch key {
			case "name":
				cur.Name = raw
			case "damageModifier":
				cur.DamageModifier = uiNumber(tb, raw)
			case "attackTimeMs":
				cur.AttackTimeMs = int32(uiNumber(tb, raw))
			case "difficulty":
				d, ok := proto.RaidDifficulty_value[strings.TrimPrefix(raw, "RaidDifficulty.")]
				if !ok {
					tb.Fatalf("unknown RaidDifficulty %q", raw)
				}
				cur.difficulty = proto.RaidDifficulty(d)
			case "damageExp2":
				cur.DamageExp2 = uiNumber(tb, raw)
			case "attackPower":
				cur.AttackPower = uiNumber(tb, raw)
			case "dualWield":
				cur.DualWield = raw == "true"
			case "noParryHaste":
				cur.NoParryHaste = raw == "true"
			case "suppressDodge":
				cur.SuppressDodge = raw == "true"
			case "hitChance":
				cur.HitChance = uiNumber(tb, raw)
			case "enemyDodgeMultiplier":
				cur.EnemyDodgeMultiplier = uiNumber(tb, raw)
			default:
				tb.Fatalf("unknown TANK_BOSSES field %q", key)
			}
		}
	}
	if err := scanner.Err(); err != nil {
		tb.Fatal(err)
	}
	tb.Fatal("pool_builder.ts has no TANK_BOSSES")
	return nil
}

// TestTankBossesMirrorUI fails when the UI's TANK_BOSSES (ui/core/optimizer/pool_builder.ts) and this
// package's Go mirror drift apart: a phase only one side has, or one with different boss numbers.
func TestTankBossesMirrorUI(t *testing.T) {
	ui := readTankBossesFromUI(t)
	for phase, want := range ui {
		if got, ok := tankBosses[phase]; !ok {
			t.Errorf("phase %d: the UI has a boss, tankBosses doesn't", phase)
		} else if !reflect.DeepEqual(got, want) {
			t.Errorf("phase %d: tankBosses = %+v, the UI has %+v", phase, got, want)
		}
	}
	for phase := range tankBosses {
		if _, ok := ui[phase]; !ok {
			t.Errorf("phase %d: tankBosses has a boss, the UI doesn't", phase)
		}
	}
}

// tankOptimizeRequest is the tank alone in a raid, holding the phase's boss, with the tank settings
// the UI sends: the default slider and crit immunity on.
func tankOptimizeRequest(tb testing.TB, player *proto.Player, phase int32) *proto.OptimizeGearRequest {
	tb.Helper()
	if !core.WITH_DB {
		tb.Skip("needs the with_db item data")
	}
	raid := core.SinglePlayerRaidProto(player, core.FullPartyBuffs, core.FullRaidBuffs, core.FullDebuffs)
	raid.Tanks = []*proto.UnitReference{{Type: proto.UnitReference_Player, Index: 0}}
	return &proto.OptimizeGearRequest{
		Base: &proto.RaidSimRequest{
			Raid:       raid,
			Encounter:  tankEncounter(tb, phase),
			SimOptions: &proto.SimOptions{RandomSeed: 101},
		},
		Settings: &proto.OptimizerSettings{
			ContentPhase:        phase,
			Effort:              proto.OptimizerEffort_OptimizerEffortQuick,
			TankSurvival:        0.7,
			RequireCritImmunity: true,
			RacialMode:          proto.OptimizerRacialMode_OptimizerRacialKeepCurrent,
		},
	}
}

var protPaladinTalents = "-05005135200132311333312321-511302012003"

func protPaladin(tb testing.TB, gearSet string) *proto.Player {
	tb.Helper()
	player := &proto.Player{
		Name:          "Prot",
		Race:          proto.Race_RaceHuman,
		Class:         proto.Class_ClassPaladin,
		Equipment:     core.GetGearSet("../../ui/protection_paladin/gear_sets", gearSet).GearSet,
		Rotation:      core.GetAplRotation("../../ui/protection_paladin/apls", "default").Rotation,
		TalentsString: protPaladinTalents,
		Glyphs: &proto.Glyphs{
			Major1: int32(proto.PaladinMajorGlyph_GlyphOfSealOfVengeance),
			Major2: int32(proto.PaladinMajorGlyph_GlyphOfRighteousDefense),
			Major3: int32(proto.PaladinMajorGlyph_GlyphOfDivinePlea),
			Minor1: int32(proto.PaladinMinorGlyph_GlyphOfLayOnHands),
			Minor2: int32(proto.PaladinMinorGlyph_GlyphOfSenseUndead),
		},
		Spec: &proto.Player_ProtectionPaladin{ProtectionPaladin: &proto.ProtectionPaladin{Options: &proto.ProtectionPaladin_Options{
			Judgement: proto.PaladinJudgement_JudgementOfWisdom,
			Seal:      proto.PaladinSeal_Vengeance,
			Aura:      proto.PaladinAura_RetributionAura,
		}}},
		Consumes: &proto.Consumes{
			Flask:           proto.Flask_FlaskOfStoneblood,
			Food:            proto.Food_FoodDragonfinFilet,
			DefaultPotion:   proto.Potions_IndestructiblePotion,
			PrepopPotion:    proto.Potions_IndestructiblePotion,
			DefaultConjured: proto.Conjured_ConjuredDarkRune,
		},
		Buffs:           core.FullIndividualBuffs,
		Professions:     []proto.Profession{proto.Profession_Blacksmithing, proto.Profession_Engineering},
		InFrontOfTarget: true,
		HealingModel:    &proto.HealingModel{CadenceSeconds: 2.5, BurstWindow: 6},
	}
	return player
}

// Feral tank talents, and the same without Survival of the Fittest: the feral tree's 18th talent
// (proto.DruidTalents.survival_of_the_fittest), 3 points of -2% crit taken each.
const (
	feralTankTalents       = "-503232132322010353120300313511-20350001"
	feralTankTalentsNoSotF = "-503232132322010350120300313511-20350001"
)

func feralTank(tb testing.TB, talents string) *proto.Player {
	tb.Helper()
	return &proto.Player{
		Name:          "Bear",
		Race:          proto.Race_RaceTauren,
		Class:         proto.Class_ClassDruid,
		Equipment:     core.GetGearSet("../../ui/feral_tank_druid/gear_sets", "p2").GearSet,
		Rotation:      core.GetAplRotation("../../ui/feral_tank_druid/apls", "default").Rotation,
		TalentsString: talents,
		Glyphs: &proto.Glyphs{
			Major1: int32(proto.DruidMajorGlyph_GlyphOfMaul),
			Major2: int32(proto.DruidMajorGlyph_GlyphOfSurvivalInstincts),
			Major3: int32(proto.DruidMajorGlyph_GlyphOfFrenziedRegeneration),
		},
		Spec: &proto.Player_FeralTankDruid{FeralTankDruid: &proto.FeralTankDruid{Options: &proto.FeralTankDruid_Options{
			InnervateTarget: &proto.UnitReference{},
			StartingRage:    20,
		}}},
		Consumes: &proto.Consumes{
			BattleElixir:    proto.BattleElixir_GurusElixir,
			GuardianElixir:  proto.GuardianElixir_GiftOfArthas,
			Food:            proto.Food_FoodBlackenedDragonfin,
			DefaultPotion:   proto.Potions_IndestructiblePotion,
			DefaultConjured: proto.Conjured_ConjuredHealthstone,
		},
		Buffs:           core.FullIndividualBuffs,
		InFrontOfTarget: true,
		HealingModel:    &proto.HealingModel{CadenceSeconds: 2.5, BurstWindow: 6},
	}
}

// D* is the exact point where the boss stops critting: immune at it, critable one rating under.
func TestRequiredDefenseIsTheThreshold(t *testing.T) {
	req := tankOptimizeRequest(t, protPaladin(t, "p3"), 3)
	r, err := PrepareRequest(req)
	if err != nil {
		t.Fatal(err)
	}
	seed, err := playerSheet(r.Base, r.TargetIndex, r.Seed, stats.Stats{})
	if err != nil {
		t.Fatal(err)
	}
	if !seed.MeleeAttacker {
		t.Fatal("Anub'arak doesn't swing at the paladin, so there's no crit to measure")
	}
	got, err := requiredDefense(r)
	if err != nil {
		t.Fatal(err)
	}
	if got <= 0 {
		t.Fatalf("D* is %g", got)
	}
	at := func(bonus float64) float64 {
		t.Helper()
		var offset stats.Stats
		offset[stats.Defense] = bonus
		sheet, err := playerSheet(r.Base, r.TargetIndex, r.Seed, offset)
		if err != nil {
			t.Fatal(err)
		}
		return sheet.MeleeCritTakenChance
	}
	// bonus stats move the sheet's Defense one for one, so D* sits this far from the seed's
	bonus := got - seed.FinalStats[stats.Defense]
	if chance := at(bonus); chance != 0 {
		t.Errorf("at D* (%.0f Defense, %+.0f from the seed) the boss still crits %.3f%%", got, bonus, chance*100)
	}
	if at(bonus-1) == 0 {
		t.Errorf("D* is %.0f Defense, but one rating less already stops the crits", got)
	}
	t.Logf("prot paladin P3: %.0f Defense on the sheet, D* %.0f, boss crit %.3f%%", seed.FinalStats[stats.Defense], got, seed.MeleeCritTakenChance*100)
}

// Nobody is crit immune by accident: with no tank assignment the boss swings at no one, so the
// sheet says the crit chance is unknown instead of reporting a flat 0.
func TestNoTankAssignmentLeavesTheCritUnknown(t *testing.T) {
	req := tankOptimizeRequest(t, protPaladin(t, "p3"), 3)
	req.Base.Raid.Tanks = nil
	r, err := PrepareRequest(req)
	if err != nil {
		t.Fatal(err)
	}
	sheet, err := playerSheet(r.Base, r.TargetIndex, r.Seed, stats.Stats{})
	if err != nil {
		t.Fatal(err)
	}
	if sheet.MeleeAttacker || sheet.MeleeCritTakenChance != 0 {
		t.Errorf("melee attacker = %v at %.3f%% crit, want nothing swinging", sheet.MeleeAttacker, sheet.MeleeCritTakenChance*100)
	}
	if _, err := requiredDefense(r); err == nil {
		t.Error("D* came back for a boss that swings at nobody")
	}
}

// Survival of the Fittest takes 6% off the crit the boss lands, so the same bear needs less defense
// to shut it out. Bare gear, so nothing but the talent separates the two.
func TestSurvivalOfTheFittestLowersRequiredDefense(t *testing.T) {
	bare := func(talents string) (*Request, core.PlayerSheet, float64) {
		t.Helper()
		req := tankOptimizeRequest(t, feralTank(t, talents), 2)
		req.Base.Raid.Parties[0].Players[0].Equipment = &proto.EquipmentSpec{}
		r, err := PrepareRequest(req)
		if err != nil {
			t.Fatal(err)
		}
		sheet, err := playerSheet(r.Base, r.TargetIndex, r.Seed, stats.Stats{})
		if err != nil {
			t.Fatal(err)
		}
		defense, err := requiredDefense(r)
		if err != nil {
			t.Fatal(err)
		}
		return r, sheet, defense
	}
	_, withSheet, with := bare(feralTankTalents)
	_, withoutSheet, without := bare(feralTankTalentsNoSotF)
	t.Logf("bare bear: crit %.3f%% with the talent, %.3f%% without; D* %.0f against %.0f",
		withSheet.MeleeCritTakenChance*100, withoutSheet.MeleeCritTakenChance*100, with, without)

	if with >= without {
		t.Errorf("the talent needs %.0f Defense and no talent needs %.0f; it should need less", with, without)
	}
	if withSheet.MeleeCritTakenChance != 0 {
		t.Errorf("3/3 Survival of the Fittest is 6%% of crit taken against a 5.6%% boss, so a bare bear should already be immune; it takes %.3f%%",
			withSheet.MeleeCritTakenChance*100)
	}
	// the crit left over turns into defense skill at PercentPerSkillPoint each
	want := withoutSheet.MeleeCritTakenChance * 100 / core.PercentPerSkillPoint * core.DefenseRatingPerDefense
	if got := without - with; math.Abs(got-want) > 2*core.DefenseRatingPerDefense {
		t.Errorf("the talent saves %.0f Defense rating, want about %.0f", got, want)
	}
}
