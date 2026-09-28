package optimizer

import (
	"bufio"
	"math"
	"os"
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
	name           string
	damageModifier float64
	attackTimeMs   int32
	difficulty     proto.RaidDifficulty
}

// The tank bosses ui/core/optimizer/pool_builder.ts sends per content phase, off each phase boss's
// live creature_template row (DamageModifier, BaseAttackTime; 0 falls back to the server's 2000 ms
// default): Patchwerk 25 (29324), Algalon 25 (33070), Anub'arak 25H (35616), the Lich King 25H
// (39168), Halion 25H (39945). TestTankBossesMirrorUI checks this stays in sync with the UI's table.
var tankBosses = map[int32]tankBoss{
	1: {"Patchwerk", 70, 1200, proto.RaidDifficulty_RaidDifficulty25Normal},
	2: {"Algalon", 285, 1000, proto.RaidDifficulty_RaidDifficulty25Normal},
	3: {"Anub'arak", 149.1, 0, proto.RaidDifficulty_RaidDifficulty25Heroic},
	4: {"the Lich King", 492.9, 1500, proto.RaidDifficulty_RaidDifficulty25Heroic},
	5: {"Halion", 385.4, 1500, proto.RaidDifficulty_RaidDifficulty25Heroic},
}

func tankEncounter(tb testing.TB, phase int32) *proto.Encounter {
	tb.Helper()
	boss, ok := tankBosses[phase]
	if !ok {
		tb.Fatalf("no tank boss for phase %d", phase)
	}
	target := encounters.GenericBossTargetAt(boss.damageModifier, boss.attackTimeMs)
	target.Name = boss.name
	return &proto.Encounter{
		Duration:             180,
		DurationVariation:    5,
		ExecuteProportion_20: 0.2,
		ExecuteProportion_25: 0.25,
		ExecuteProportion_35: 0.35,
		Targets:              []*proto.Target{target},
		RaidDifficulty:       boss.difficulty,
	}
}

// tankBossLine matches one TANK_BOSSES entry in pool_builder.ts, e.g.
// `1: { name: 'Patchwerk', damageModifier: 70, attackTimeMs: 1200, difficulty: RaidDifficulty.RaidDifficulty25Normal },`
var tankBossLine = regexp.MustCompile(`^\s*(\d+):\s*\{\s*name:\s*(?:'([^']*)'|"([^"]*)"),\s*damageModifier:\s*([0-9.]+),\s*attackTimeMs:\s*(\d+),\s*difficulty:\s*RaidDifficulty\.(\w+)\s*\},?\s*$`)

func readTankBossesFromUI(tb testing.TB) map[int32]tankBoss {
	tb.Helper()
	f, err := os.Open("../../ui/core/optimizer/pool_builder.ts")
	if err != nil {
		tb.Fatal(err)
	}
	defer f.Close()

	result := map[int32]tankBoss{}
	started := false
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if !started {
			started = strings.HasPrefix(strings.TrimSpace(line), "export const TANK_BOSSES")
			continue
		}
		if strings.HasPrefix(strings.TrimSpace(line), "};") {
			break
		}
		m := tankBossLine.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		phase, err := strconv.Atoi(m[1])
		if err != nil {
			tb.Fatalf("bad phase in %q: %v", line, err)
		}
		modifier, err := strconv.ParseFloat(m[4], 64)
		if err != nil {
			tb.Fatalf("bad damageModifier in %q: %v", line, err)
		}
		attackTime, err := strconv.Atoi(m[5])
		if err != nil {
			tb.Fatalf("bad attackTimeMs in %q: %v", line, err)
		}
		difficulty, ok := proto.RaidDifficulty_value[m[6]]
		if !ok {
			tb.Fatalf("unknown RaidDifficulty %q", m[6])
		}
		result[int32(phase)] = tankBoss{m[2] + m[3], modifier, int32(attackTime), proto.RaidDifficulty(difficulty)}
	}
	if err := scanner.Err(); err != nil {
		tb.Fatal(err)
	}
	if !started {
		tb.Fatal("pool_builder.ts has no TANK_BOSSES")
	}
	return result
}

// TestTankBossesMirrorUI fails when the UI's TANK_BOSSES (ui/core/optimizer/pool_builder.ts) and this
// package's Go mirror drift apart: a phase only one side has, or one with different boss numbers.
func TestTankBossesMirrorUI(t *testing.T) {
	ui := readTankBossesFromUI(t)
	for phase, want := range ui {
		if got, ok := tankBosses[phase]; !ok {
			t.Errorf("phase %d: the UI has a boss, tankBosses doesn't", phase)
		} else if got != want {
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
