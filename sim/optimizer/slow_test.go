//go:build optimizer_slow

package optimizer

import (
	"context"
	"flag"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
	goproto "google.golang.org/protobuf/proto"
)

// The slow suite optimizes four presets over realistic pools, at Quick and Normal, and checks that
// the pick is no worse than the preset in a fresh paired sim on other random numbers. Run it with:
//
//	tools/acore/dock.sh exec go test --tags=with_db,optimizer_slow -count=1 -timeout 90m -run TestOptimizerSlow -v ./sim/optimizer/
//
// -decisions also logs what each stage decides. Each case logs one line to compare from run to run,
// J in reference-stat points and the deltas paired over slowCheckIterations:
//
//	slow: spec=… phase=… effort=… candidates=… J_preset=… J_opt=… delta=…±… dps_delta=…±… improved=… sims=… wall=…s

// slowCheckIterations is how long the independent check sims each loadout.
const slowCheckIterations = 10000

var decisions = flag.Bool("decisions", false, "log what each stage of the slow suite's runs decides")

type slowCase struct {
	spec  string
	phase int32
	// The player, gear set to the phase's preset.
	player func(tb testing.TB) *proto.Player
}

var slowCases = []slowCase{
	{"fury", 1, func(tb testing.TB) *proto.Player {
		player := presetOptimizeRequest(tb, "fury_p1").Base.Raid.Parties[0].Players[0]
		player.Professions = []proto.Profession{proto.Profession_Jewelcrafting, proto.Profession_Engineering}
		return player
	}},
	{"combat_rogue", 3, func(tb testing.TB) *proto.Player {
		return &proto.Player{
			Name:          "Combat",
			Race:          proto.Race_RaceHuman,
			Class:         proto.Class_ClassRogue,
			Equipment:     core.GetGearSet("../../ui/rogue/gear_sets", "p3_combat").GearSet,
			Rotation:      core.GetAplRotation("../../ui/rogue/apls", "combat_expose").Rotation,
			TalentsString: "00532000523-0252051050035010223100501251",
			Glyphs: &proto.Glyphs{
				Major1: int32(proto.RogueMajorGlyph_GlyphOfKillingSpree),
				Major2: int32(proto.RogueMajorGlyph_GlyphOfTricksOfTheTrade),
				Major3: int32(proto.RogueMajorGlyph_GlyphOfRupture),
			},
			Spec: &proto.Player_Rogue{Rogue: &proto.Rogue{Options: &proto.Rogue_Options{
				MhImbue: proto.Rogue_Options_DeadlyPoison,
				OhImbue: proto.Rogue_Options_InstantPoison,
			}}},
			Consumes: &proto.Consumes{
				Flask:           proto.Flask_FlaskOfEndlessRage,
				DefaultPotion:   proto.Potions_PotionOfSpeed,
				DefaultConjured: proto.Conjured_ConjuredRogueThistleTea,
				Food:            proto.Food_FoodMegaMammothMeal,
			},
			Buffs:       core.FullIndividualBuffs,
			Professions: []proto.Profession{proto.Profession_Engineering, proto.Profession_Jewelcrafting},
		}
	}},
	{"fire_mage", 3, func(tb testing.TB) *proto.Player {
		return &proto.Player{
			Name:          "Fire",
			Race:          proto.Race_RaceTroll,
			Class:         proto.Class_ClassMage,
			Equipment:     core.GetGearSet("../../ui/mage/gear_sets", "p3_fire_horde").GearSet,
			Rotation:      core.GetAplRotation("../../ui/mage/apls", "fire").Rotation,
			TalentsString: "23000503110003-0055030012303331053120301351",
			Glyphs: &proto.Glyphs{
				Major1: int32(proto.MageMajorGlyph_GlyphOfFireball),
				Major2: int32(proto.MageMajorGlyph_GlyphOfMoltenArmor),
				Major3: int32(proto.MageMajorGlyph_GlyphOfLivingBomb),
			},
			Spec: &proto.Player_Mage{Mage: &proto.Mage{Options: &proto.Mage_Options{Armor: proto.Mage_Options_MoltenArmor}}},
			Consumes: &proto.Consumes{
				Flask:         proto.Flask_FlaskOfTheFrostWyrm,
				Food:          proto.Food_FoodFirecrackerSalmon,
				DefaultPotion: proto.Potions_PotionOfSpeed,
			},
			Buffs:       core.FullIndividualBuffs,
			Professions: []proto.Profession{proto.Profession_Tailoring, proto.Profession_Engineering},
		}
	}},
	{"retribution", 4, func(tb testing.TB) *proto.Player {
		return &proto.Player{
			Name:          "Ret",
			Race:          proto.Race_RaceHuman,
			Class:         proto.Class_ClassPaladin,
			Equipment:     core.GetGearSet("../../ui/retribution_paladin/gear_sets", "p4").GearSet,
			Rotation:      core.GetAplRotation("../../ui/retribution_paladin/apls", "default").Rotation,
			TalentsString: "050501-05-05232051203331302133231331",
			Glyphs: &proto.Glyphs{
				Major1: int32(proto.PaladinMajorGlyph_GlyphOfSealOfVengeance),
				Major2: int32(proto.PaladinMajorGlyph_GlyphOfJudgement),
				Major3: int32(proto.PaladinMajorGlyph_GlyphOfReckoning),
				Minor1: int32(proto.PaladinMinorGlyph_GlyphOfSenseUndead),
				Minor2: int32(proto.PaladinMinorGlyph_GlyphOfLayOnHands),
				Minor3: int32(proto.PaladinMinorGlyph_GlyphOfBlessingOfKings),
			},
			Spec: &proto.Player_RetributionPaladin{RetributionPaladin: &proto.RetributionPaladin{Options: &proto.RetributionPaladin_Options{
				Judgement: proto.PaladinJudgement_JudgementOfWisdom,
				Seal:      proto.PaladinSeal_Vengeance,
				Aura:      proto.PaladinAura_RetributionAura,
			}}},
			Consumes: &proto.Consumes{
				Flask:           proto.Flask_FlaskOfEndlessRage,
				DefaultPotion:   proto.Potions_PotionOfSpeed,
				DefaultConjured: proto.Conjured_ConjuredDarkRune,
				Food:            proto.Food_FoodDragonfinFilet,
			},
			Buffs:       core.FullIndividualBuffs,
			Professions: []proto.Profession{proto.Profession_Jewelcrafting, proto.Profession_Engineering},
		}
	}},
}

// slowRequest is the case's player alone in a raid with full buffs, against the default encounter,
// over its realistic pool.
func slowRequest(tb testing.TB, c slowCase, effort proto.OptimizerEffort) *proto.OptimizeGearRequest {
	player := c.player(tb)
	req := presetOptimizeRequest(tb, "fury_p1")
	req.Base.Raid = core.SinglePlayerRaidProto(player, core.FullPartyBuffs, core.FullRaidBuffs, core.FullDebuffs)
	req.Settings = &proto.OptimizerSettings{
		ContentPhase: c.phase,
		Effort:       effort,
		RacialMode:   proto.OptimizerRacialMode_OptimizerRacialKeepCurrent,
	}
	req.Pool = realisticPool(tb, player, c.phase, nil)
	return req
}

func TestOptimizerSlow(t *testing.T) {
	for _, c := range slowCases {
		for _, effort := range []proto.OptimizerEffort{proto.OptimizerEffort_OptimizerEffortQuick, proto.OptimizerEffort_OptimizerEffortNormal} {
			name := strings.TrimPrefix(effort.String(), "OptimizerEffort")
			t.Run(fmt.Sprintf("%s_p%d/%s", c.spec, c.phase, name), func(t *testing.T) {
				req := slowRequest(t, c, effort)
				if *decisions {
					traceHook = t.Logf
					defer func() { traceHook = nil }()
				}
				poolSize := 0
				for _, sp := range req.Pool.Slots {
					poolSize += len(sp.ItemIds)
				}
				start := time.Now()
				var stages []string
				stageStart := time.Now()
				lastStage := ""
				result := Optimize(context.Background(), req, func(p *proto.OptimizerProgress) {
					if p.Stage != lastStage {
						if lastStage != "" {
							stages = append(stages, fmt.Sprintf("%s %.1fs", lastStage, time.Since(stageStart).Seconds()))
						}
						lastStage, stageStart = p.Stage, time.Now()
					}
				})
				wall := time.Since(start)
				stages = append(stages, fmt.Sprintf("%s %.1fs", lastStage, time.Since(stageStart).Seconds()))
				if result.ErrorResult != "" {
					t.Fatal(result.ErrorResult)
				}
				t.Logf("%d candidates, stages: %v", poolSize, stages)
				for _, w := range result.Warnings {
					t.Logf("warning: %s", w)
				}

				// an independent paired check on other random numbers, so the search's own noise
				// can't flatter its pick
				check := goproto.Clone(req).(*proto.OptimizeGearRequest)
				check.Base.SimOptions.RandomSeed = 9001
				r, err := PrepareRequest(check)
				if err != nil {
					t.Fatal(err)
				}
				best, err := LoadoutFromProto(result.Best.Equipment, result.Best.RacialTraits)
				if err != nil {
					t.Fatal(err)
				}
				keep, _ := WeightedMetrics(r.Settings)
				eval := NewSimEvaluator(r, keep...)
				obj, err := NewObjective(context.Background(), eval, r, slowCheckIterations)
				if err != nil {
					t.Fatal(err)
				}
				evals, err := eval.Evaluate(context.Background(), []Point{{Loadout: r.Seed}, {Loadout: best}}, slowCheckIterations)
				if err != nil {
					t.Fatal(err)
				}
				jPreset, jOpt := obj.Score(evals[0]), obj.Score(evals[1])
				d := obj.Delta(evals[0], evals[1])
				dps := Delta(evals[0], evals[1], MetricDPS)
				t.Logf("slow: spec=%s phase=%d effort=%s candidates=%d J_preset=%.1f J_opt=%.1f delta=%+.1f±%.1f dps_delta=%+.1f±%.1f improved=%v sims=%d wall=%.1fs",
					c.spec, c.phase, name, poolSize, jPreset.Mean, jOpt.Mean, d.Mean, d.SE, dps.Mean, dps.SE, result.Improved, result.TotalSims, wall.Seconds())
				if d.Mean < -2*d.SE {
					t.Errorf("the pick scores %.1f ± %.1f under the preset", -d.Mean, d.SE)
				}
			})
		}
	}
}
