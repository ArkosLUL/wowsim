//go:build optimizer_slow

package optimizer

import (
	"context"
	"flag"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
	"github.com/wowsims/wotlk/sim/core/stats"
	goproto "google.golang.org/protobuf/proto"
)

// The slow suite optimizes four presets over realistic pools, at Quick and Normal, and checks that
// the pick is no worse than the preset in a fresh paired sim on other random numbers. Run it with:
//
//	tools/acore/dock.sh exec go test --tags=with_db,optimizer_slow -count=1 -timeout 90m -run TestOptimizerSlow -v ./sim/optimizer/
//
// -decisions also logs what each stage decides. Each case logs one line to compare from run to run,
// J in reference-stat points and the deltas paired over slowCheckIterations. J's normalizers are
// measured fresh each run, so the line prints the preset's DPS and each normalizer with its SE too:
// a J_preset that moves with its normalizer while dps_preset holds is noise, not a regression.
//
//	slow: spec=… phase=… effort=… candidates=… J_preset=… J_opt=… delta=…±… dps_preset=… dps_delta=…±… norm_dps=…±…/AP improved=… sims=… wall=…s

// slowCheckIterations is how long the independent check sims each loadout.
const slowCheckIterations = 10000

var decisions = flag.Bool("decisions", false, "log what each stage of the slow suite's runs decides")

type slowCase struct {
	spec  string
	phase int32
	// The player, gear set to the phase's preset.
	player func(tb testing.TB) *proto.Player
	// Tanks hold the phase's boss on the survival/threat slider, and have to come out crit immune.
	tank bool
}

var slowCases = []slowCase{
	{spec: "fury", phase: 1, player: func(tb testing.TB) *proto.Player {
		player := presetOptimizeRequest(tb, "fury_p1").Base.Raid.Parties[0].Players[0]
		player.Professions = []proto.Profession{proto.Profession_Jewelcrafting, proto.Profession_Engineering}
		return player
	}},
	{spec: "combat_rogue", phase: 3, player: func(tb testing.TB) *proto.Player {
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
	{spec: "fire_mage", phase: 3, player: func(tb testing.TB) *proto.Player {
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
	{spec: "retribution", phase: 4, player: func(tb testing.TB) *proto.Player {
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
				Major3: int32(proto.PaladinMajorGlyph_GlyphOfConsecration),
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
	{spec: "prot_paladin", phase: 3, player: func(tb testing.TB) *proto.Player { return protPaladin(tb, "p3") }, tank: true},
	{spec: "feral_tank", phase: 2, player: func(tb testing.TB) *proto.Player { return feralTank(tb, feralTankTalents) }, tank: true},
}

// tankHealing is the healing a tank takes from the boss it fights, the way the UI's pool builder
// re-derives it per phase (ui/core/optimizer/pool_builder.ts healingModelForBoss). A concrete Hps
// also keeps core's presim out of every sim shard.
func tankHealing(boss *proto.Target) *proto.HealingModel {
	cadence, hps := 1.5*boss.SwingSpeed, 0.175*boss.MinBaseDamage/boss.SwingSpeed
	if boss.DualWield {
		cadence /= 2
		hps *= 1.5
	}
	return &proto.HealingModel{Hps: hps, CadenceSeconds: cadence, BurstWindow: 6}
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
	if c.tank {
		req.Base.Raid.Tanks = []*proto.UnitReference{{Type: proto.UnitReference_Player, Index: 0}}
		req.Base.Encounter = tankEncounter(tb, c.phase)
		player.HealingModel = tankHealing(req.Base.Encounter.Targets[0])
		req.Settings.TankSurvival = 0.7
		req.Settings.RequireCritImmunity = true
	}
	req.Pool = realisticPool(tb, player, c.phase, nil)
	trimSeedToPool(tb, player, req.Pool)
	return req
}

// trimSeedToPool drops whatever the pool doesn't offer, the way the UI's pool builder trims the
// seed before it sends a request (ui/core/optimizer/pool_builder.ts SeedTrimmer). Presets mix
// factions and the odd PvP piece, and a seed the equip rules reject makes every pick count as an
// improvement whatever it scores, which is exactly what this suite is trying to measure.
func trimSeedToPool(tb testing.TB, player *proto.Player, pool *proto.CandidatePool) {
	slots := map[proto.ItemSlot]*proto.SlotPool{}
	for _, sp := range pool.Slots {
		slots[sp.Slot] = sp
	}
	gems := map[int32]bool{}
	for _, id := range pool.GemIds {
		gems[id] = true
	}
	offers := func(slot proto.ItemSlot, id int32) bool {
		return slices.Contains(slots[slot].GetItemIds(), id)
	}
	items := player.Equipment.GetItems()
	for slot := range items {
		spec := items[slot]
		if spec.GetId() == 0 {
			continue
		}
		if !offers(proto.ItemSlot(slot), spec.Id) {
			tb.Logf("seed: the pool doesn't offer item %d in %s", spec.Id, proto.ItemSlot(slot))
			items[slot] = &proto.ItemSpec{}
			continue
		}
		enchanted := false
		for _, option := range slots[proto.ItemSlot(slot)].GetEnchantOptions() {
			enchanted = enchanted || option.ItemId == spec.Id && slices.Contains(option.EnchantIds, spec.Enchant)
		}
		if spec.Enchant != 0 && !enchanted {
			tb.Logf("seed: the pool doesn't offer enchant %d on item %d", spec.Enchant, spec.Id)
			spec.Enchant = 0
		}
		for i, gem := range spec.Gems {
			if gem != 0 && !gems[gem] {
				tb.Logf("seed: the pool doesn't offer gem %d", gem)
				spec.Gems[i] = 0
			}
		}
	}
	// core moves a lone second ring or trinket up, and the optimizer won't take a seed it would
	// rearrange
	for _, pair := range [][2]proto.ItemSlot{
		{proto.ItemSlot_ItemSlotFinger1, proto.ItemSlot_ItemSlotFinger2},
		{proto.ItemSlot_ItemSlotTrinket1, proto.ItemSlot_ItemSlotTrinket2},
	} {
		first, second := pair[0], pair[1]
		if items[first].GetId() != 0 || items[second].GetId() == 0 {
			continue
		}
		if offers(first, items[second].Id) {
			items[first], items[second] = items[second], &proto.ItemSpec{}
		} else {
			items[second] = &proto.ItemSpec{}
		}
	}
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
				t.Logf("slow: spec=%s phase=%d effort=%s candidates=%d J_preset=%.1f J_opt=%.1f delta=%+.1f±%.1f dps_preset=%.1f dps_delta=%+.1f±%.1f %s improved=%v sims=%d wall=%.1fs",
					c.spec, c.phase, name, poolSize, jPreset.Mean, jOpt.Mean, d.Mean, d.SE, evals[0].Metrics[MetricDPS].Mean, dps.Mean, dps.SE,
					normalizerFields(obj), result.Improved, result.TotalSims, wall.Seconds())
				if d.Mean < -2*d.SE {
					t.Errorf("the pick scores %.1f ± %.1f under the preset", -d.Mean, d.SE)
				}
				if c.tank {
					sheet, err := playerSheet(r.Base, r.TargetIndex, best, stats.Stats{})
					if err != nil {
						t.Fatal(err)
					}
					t.Logf("tank: %.0f Defense on the sheet, boss crit %.3f%%", sheet.FinalStats[stats.Defense], sheet.MeleeCritTakenChance*100)
					// without this a boss that swings at nobody would pass the crit check on a flat 0
					if !sheet.MeleeAttacker {
						t.Error("nothing in the encounter swings at the tank")
					}
					if sheet.MeleeCritTakenChance != 0 {
						t.Errorf("crit immunity was required, but the boss crits the pick %.3f%% of the time", sheet.MeleeCritTakenChance*100)
					}
				}
			})
		}
	}
}

// normalizerFields is each weighted metric's normalizer, per point of its reference stat, e.g.
// "norm_dps=…±…/AP".
func normalizerFields(obj *Objective) string {
	var fields []string
	for m, w := range obj.Weights {
		if w == 0 {
			continue
		}
		n := obj.Normalizers[m]
		name := strings.ReplaceAll(strings.ToLower(metricName(Metric(m))), " ", "_")
		fields = append(fields, fmt.Sprintf("norm_%s=%.4g±%.2g/%s", name, n.Mean, n.SE, statAbbrev(obj.ReferenceStats[m])))
	}
	return strings.Join(fields, " ")
}

func statAbbrev(s stats.Stat) string {
	switch s {
	case stats.AttackPower:
		return "AP"
	case stats.RangedAttackPower:
		return "RAP"
	case stats.SpellPower:
		return "SP"
	}
	return s.StatName()
}
