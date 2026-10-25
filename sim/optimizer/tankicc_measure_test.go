package optimizer

import (
	"context"
	"math"
	"os"
	"testing"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
	"github.com/wowsims/wotlk/sim/core/stats"
)

// The tank DK page's talent presets (ui/tank_deathknight/presets.ts), which the BiS presets are built on.
var (
	bloodTankPresetTalents = "005512153330030320102013-3050505000023-005"
	frostTankPresetTalents = "005510003-3050535000223301030023310035-005"
)

// tankDeathknight is the DK tank page's player in the named BiS gear set: the page's blood_icy_touch
// rotation, consumes and defensives threshold, and presetgen's Draenei with the Night Elf racials the
// P4 presets picked.
func tankDeathknight(tb testing.TB, gearSet, talents string) *proto.Player {
	tb.Helper()
	return &proto.Player{
		Name:          "Tank",
		Race:          proto.Race_RaceDraenei,
		RacialTraits:  proto.Race_RaceNightElf,
		Class:         proto.Class_ClassDeathknight,
		Equipment:     core.GetGearSet("../../ui/tank_deathknight/gear_sets", gearSet).GearSet,
		Rotation:      core.GetAplRotation("../../ui/tank_deathknight/apls", "blood_icy_touch").Rotation,
		TalentsString: talents,
		Glyphs: &proto.Glyphs{
			Major1: int32(proto.DeathknightMajorGlyph_GlyphOfDisease),
			Major2: int32(proto.DeathknightMajorGlyph_GlyphOfRuneStrike),
			Major3: int32(proto.DeathknightMajorGlyph_GlyphOfDarkCommand),
			Minor1: int32(proto.DeathknightMinorGlyph_GlyphOfHornOfWinter),
			Minor2: int32(proto.DeathknightMinorGlyph_GlyphOfBloodTap),
			Minor3: int32(proto.DeathknightMinorGlyph_GlyphOfRaiseDead),
		},
		Spec: &proto.Player_TankDeathknight{TankDeathknight: &proto.TankDeathknight{Options: &proto.TankDeathknight_Options{}}},
		Consumes: &proto.Consumes{
			Flask:         proto.Flask_FlaskOfStoneblood,
			Food:          proto.Food_FoodDragonfinFilet,
			DefaultPotion: proto.Potions_IndestructiblePotion,
			PrepopPotion:  proto.Potions_IndestructiblePotion,
		},
		Buffs:           core.FullIndividualBuffs,
		InFrontOfTarget: true,
		Cooldowns:       &proto.Cooldowns{HpPercentForDefensives: 0.35},
	}
}

// tankFight is req the way a run sims it: the tank alone with what the raid gives it (simmedRequest),
// the healing cadence the pool builder takes from the boss, and the healing pinned.
func tankFight(tb testing.TB, req *proto.OptimizeGearRequest) *Request {
	tb.Helper()
	req.Base.Raid.Parties[0].Players[0].HealingModel = tankHealing(req.Base.Encounter.Targets[0])
	asked, err := PrepareRequest(req)
	if err != nil {
		tb.Fatal(err)
	}
	r, err := simmedRequest(asked)
	if err != nil {
		tb.Fatal(err)
	}
	if err := (&run{r: r}).pinHealingModel(); err != nil {
		tb.Fatal(err)
	}
	return r
}

// regem returns l with every gem from replaced by to.
func regem(l Loadout, from, to int32) Loadout {
	for i := range l.Items {
		for g := range l.Items[i].Gems {
			if l.Items[i].Gems[g] == from {
				l.Items[i].Gems[g] = to
			}
		}
	}
	return l
}

// TestMeasureTankIcecrown prints, for the worn P4 BiS tank death knights (blood and frost) in the Lich
// King fight without Icecrown's buff and with it: chance of death, health margin, the death normalizer,
// what one more rating point of Stamina, Defense, Dodge, Parry and Agility is worth by term, and what
// swapping the defense gems for Stamina ones or Thick King's Amber does to J. A measurement, not a
// check, so it only runs with OPTIMIZER_ICC set:
//
//	tools/acore/dock.sh exec env OPTIMIZER_ICC=1 go test --tags=with_db -count=1 -run TestMeasureTankIcecrown -v ./sim/optimizer/
func TestMeasureTankIcecrown(t *testing.T) {
	if os.Getenv("OPTIMIZER_ICC") == "" {
		t.Skip("set OPTIMIZER_ICC to run it")
	}
	const phase int32 = 4
	for _, c := range []struct{ tag, gear, talents string }{
		{"P4 blood", "p4_bis_blood", bloodTankPresetTalents},
		{"P4 frost", "p4_bis_frost", frostTankPresetTalents},
	} {
		for _, buffed := range []bool{false, true} {
			req := tankOptimizeRequest(t, tankDeathknight(t, c.gear, c.talents), phase)
			req.Base.Raid.Buffs.StrengthOfWrynn = buffed
			tag := c.tag + " without the buff"
			if buffed {
				tag = c.tag + " with the buff"
			}
			measureIcecrownFight(t, tag, tankFight(t, req))
		}
	}
}

func measureIcecrownFight(t *testing.T, tag string, r *Request) {
	const (
		enduringEyeOfZul    = 40167
		regalDreadstone     = 40138
		solidMajesticZircon = 40119
		thickKingsAmber     = 40126
		bump                = 100.0
	)
	offsets := []struct {
		name string
		stat stats.Stat
	}{{"Stamina", stats.Stamina}, {"Defense", stats.Defense}, {"Dodge", stats.Dodge}, {"Parry", stats.Parry}, {"Agility", stats.Agility}}
	swaps := []struct {
		name     string
		from, to int32
	}{
		{"Enduring Eye of Zul -> Solid Majestic Zircon", enduringEyeOfZul, solidMajesticZircon},
		{"Enduring Eye of Zul -> Thick King's Amber", enduringEyeOfZul, thickKingsAmber},
		{"Regal Dreadstone -> Solid Majestic Zircon", regalDreadstone, solidMajesticZircon},
	}
	iterations := EffortBudget(proto.OptimizerEffort_OptimizerEffortNormal).Iterations
	ctx := context.Background()

	eval := NewSimEvaluator(r, tankMetrics...)
	obj, err := NewObjective(ctx, eval, r, iterations)
	if err != nil {
		t.Fatalf("%s: %v", tag, err)
	}
	sheet, err := playerSheet(r.Base, r.TargetIndex, r.Equipped, stats.Stats{})
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("%s: health %.0f Stamina %.0f Defense %.0f Dodge %.0f Parry %.0f Agility %.0f; pinned %.0f HPS, defensives at %.2f, weights %v, warnings %v",
		tag, sheet.FinalStats[stats.Health], sheet.FinalStats[stats.Stamina], sheet.FinalStats[stats.Defense], sheet.FinalStats[stats.Dodge],
		sheet.FinalStats[stats.Parry], sheet.FinalStats[stats.Agility],
		r.Target().GetHealingModel().GetHps(), r.Target().GetCooldowns().GetHpPercentForDefensives(), obj.Weights, obj.Warnings)
	t.Logf("%s normalizers per Armor point: %s; coefficients %s", tag, normalizersText(obj.Normalizers), coefficientsText(obj))
	dm, err := measureDeath(ctx, eval, r, r.Equipped, iterations)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("%s death normalizer: %.4g ± %.2g per Armor point (%.1f se), dies %.4f (%.4f at -%g Armor, %.4f at +%g)", tag,
		dm.Norm.Mean, dm.Norm.SE, -dm.Norm.Mean/max(dm.Norm.SE, 1e-12), dm.Dies, dm.Down, normalizerOffset(stats.Armor), dm.Up, normalizerOffset(stats.Armor))

	points := []Point{{Loadout: r.Equipped}}
	for _, o := range offsets {
		var offset stats.Stats
		offset[o.stat] = bump
		points = append(points, Point{Loadout: r.Equipped, Offset: offset})
	}
	for _, s := range swaps {
		points = append(points, Point{Loadout: regem(r.Equipped, s.from, s.to)})
	}
	evals, err := eval.Evaluate(ctx, points, iterations)
	if err != nil {
		t.Fatal(err)
	}
	worn := evals[0]
	lives := timesToLive(t, r, points[:1], iterations)
	margin := worn.Metrics[MetricHealthMargin]
	t.Logf("%s worn: DTPS %.0f TMI %.1f TPS %.0f death %.4f margin %.0f ± %.0f, time to live %.2f ± %.2f s", tag,
		worn.Metrics[MetricDTPS].Mean, worn.Metrics[MetricTMI].Mean, worn.Metrics[MetricTPS].Mean,
		worn.Metrics[MetricPDeath].Mean, margin.Mean, margin.SE, lives[0].Mean, lives[0].SE)
	for i, o := range offsets {
		e := evals[i+1]
		d := obj.Delta(worn, e)
		t.Logf("%s: dJ per %s point %+.4f (se %.4f), by term %s; dDeath %+.6f dMargin %+.2f", tag, o.name, d.Mean/bump, d.SE/bump,
			byTerm(obj, worn, e, bump), Delta(worn, e, MetricPDeath).Mean/bump, Delta(worn, e, MetricHealthMargin).Mean/bump)
	}
	for i, s := range swaps {
		e := evals[len(offsets)+1+i]
		after, err := playerSheet(r.Base, r.TargetIndex, points[len(offsets)+1+i].Loadout, stats.Stats{})
		if err != nil {
			t.Fatal(err)
		}
		d := obj.Delta(worn, e)
		t.Logf("%s: %s: dJ %+.1f (se %.1f), health %+.0f Defense %+.0f; death %.4f -> %.4f, margin %+.0f", tag, s.name, d.Mean, d.SE,
			after.FinalStats[stats.Health]-sheet.FinalStats[stats.Health], after.FinalStats[stats.Defense]-sheet.FinalStats[stats.Defense],
			worn.Metrics[MetricPDeath].Mean, e.Metrics[MetricPDeath].Mean, Delta(worn, e, MetricHealthMargin).Mean)
	}
}

// Only the Lich King's tank fight is inside Icecrown, and the request the pin and the search sim
// carries the buff: 30% more health on the same gear.
func TestOnlyThePhase4TankFightHasTheIcecrownBuff(t *testing.T) {
	healthAt := func(phase int32) (float64, bool) {
		r := tankFight(t, tankOptimizeRequest(t, tankDeathknight(t, "p4_bis_blood", bloodTankPresetTalents), phase))
		if hps := r.Target().GetHealingModel().GetHps(); hps <= 0 {
			t.Errorf("phase %d: pinned healing %.0f HPS", phase, hps)
		}
		sheet, err := playerSheet(r.Base, r.TargetIndex, r.Equipped, stats.Stats{})
		if err != nil {
			t.Fatal(err)
		}
		return sheet.FinalStats[stats.Health], r.Base.Raid.Buffs.GetStrengthOfWrynn()
	}
	health4, buff4 := healthAt(4)
	health5, buff5 := healthAt(5)
	if !buff4 || buff5 {
		t.Errorf("Strength of Wrynn at phase 4 %v and phase 5 %v, want true and false", buff4, buff5)
	}
	if ratio := health4 / health5; math.Abs(ratio-1.3) > 0.001 {
		t.Errorf("health at phase 4 %.0f against %.0f at phase 5 is %.4f times, want 1.3", health4, health5, ratio)
	}
}
