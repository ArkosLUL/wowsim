package optimizer

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"slices"
	"sort"
	"testing"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
	"github.com/wowsims/wotlk/sim/core/stats"
	"google.golang.org/protobuf/encoding/protojson"
)

type batchPick struct {
	raider string
	phase  int32
	result *proto.OptimizerResult
}

// batchPicks reads the tank runs out of a BiS Batch export (the acbis driver's batch-stage1.json).
// Like the requests it's never committed, so the path comes from the environment.
func batchPicks(tb testing.TB) []batchPick {
	tb.Helper()
	path := os.Getenv("BIS_PICKS")
	if path == "" {
		tb.Skip("BIS_PICKS names a BiS Batch export")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		tb.Fatal(err)
	}
	var export struct {
		Entries []struct {
			Raider       string          `json:"raider"`
			ContentPhase int32           `json:"contentPhase"`
			Stage        int32           `json:"stage"`
			Result       json.RawMessage `json:"result"`
		} `json:"entries"`
	}
	if err := json.Unmarshal(data, &export); err != nil {
		tb.Fatal(err)
	}
	var out []batchPick
	for _, e := range export.Entries {
		result := &proto.OptimizerResult{}
		if err := (protojson.UnmarshalOptions{DiscardUnknown: true}).Unmarshal(e.Result, result); err != nil {
			tb.Fatalf("%s P%d: %v", e.Raider, e.ContentPhase, err)
		}
		if e.Stage == 1 && result.GetSettings().GetTankSurvival() > 0 && result.GetBest().GetEquipment() != nil {
			out = append(out, batchPick{e.Raider, e.ContentPhase, result})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].raider != out[j].raider {
			return out[i].raider < out[j].raider
		}
		return out[i].phase < out[j].phase
	})
	return out
}

// Solid Stormjewel, +30 Stamina, and the jewelcrafter's Solid Dragon's Eye, +51 and 3 at most. The
// server catalog has both from phase 1.
const (
	solidStormjewel = 45880
	solidDragonsEye = 36767
	maxDragonsEyes  = 3
)

// staminaGemmed is l with a Stamina gem in every socket but the meta's: about the most health the
// phase's gear can hold, since it keeps l's items and enchants.
func staminaGemmed(l Loadout, jeweler bool) Loadout {
	eyes := 0
	for slot := range l.Items {
		for i, id := range l.Items[slot].Gems {
			if gem, _ := core.LookupGem(id); id == 0 || gem.Color == proto.GemColor_GemColorMeta {
				continue
			}
			l.Items[slot].Gems[i] = solidStormjewel
			if jeweler && eyes < maxDragonsEyes {
				l.Items[slot].Gems[i] = solidDragonsEye
				eyes++
			}
		}
	}
	return l
}

// TestMeasureTankPicks takes the tank picks of a BiS Batch export back into the fight they were scored
// on and prints, per run: the pick against the worn gear, what one more point of stamina, agility,
// dodge and defense rating is worth at the pick, and its health with every socket a Stamina gem.
// Where the run left death chance out, it also walks the pick's Stamina toward the health at which it
// starts to die, or to survive. It asserts nothing.
//
//	BIS_REQUESTS=<dir> BIS_PICKS=<batch-stage1.json> go test -tags with_db -run TestMeasureTankPicks -v ./sim/optimizer/
func TestMeasureTankPicks(t *testing.T) {
	picks := batchPicks(t)
	iterations := EffortBudget(proto.OptimizerEffort_OptimizerEffortNormal).Iterations
	ctx := context.Background()
	offsets := []struct {
		name string
		stat stats.Stat
		// what one epic gem holds, to compare the stats a socket at a time
		gem float64
	}{
		{"Stamina", stats.Stamina, 30}, {"Agility", stats.Agility, 20}, {"Dodge", stats.Dodge, 20}, {"Defense", stats.Defense, 20},
	}
	const bump = 100.0

	for _, p := range picks {
		tag := fmt.Sprintf("%s P%d", p.raider, p.phase)
		r := tankFightRequest(t, p.raider, p.phase)
		eval := NewSimEvaluator(r, MetricDTPS, MetricTMI, MetricTPS, MetricPDeath)
		obj, err := NewObjective(ctx, eval, r, iterations)
		if err != nil {
			t.Fatalf("%s: %v", tag, err)
		}
		best := p.result.GetBest()
		race := best.GetRacialTraits()
		if race == proto.Race_RaceUnknown {
			race = r.Seed.RacialTraits
		}
		pick, err := LoadoutFromProto(best.GetEquipment(), race)
		if err != nil {
			t.Fatalf("%s: %v", tag, err)
		}
		health := func(l Loadout, offset stats.Stats) float64 {
			sheet, err := playerSheet(r.Base, r.TargetIndex, l, offset)
			if err != nil {
				t.Fatalf("%s: %v", tag, err)
			}
			return sheet.FinalStats[stats.Health]
		}
		stamina := func(amount float64) stats.Stats {
			var offset stats.Stats
			offset[stats.Stamina] = amount
			return offset
		}

		jeweler := slices.Contains(core.ProtoToProfessions(r.Target()), proto.Profession_Jewelcrafting)
		gemmed := staminaGemmed(pick, jeweler)
		points := []Point{{Loadout: r.Equipped}, {Loadout: pick}, {Loadout: gemmed}}
		for _, o := range offsets {
			var offset stats.Stats
			offset[o.stat] = bump
			points = append(points, Point{Loadout: pick, Offset: offset})
		}
		evals, err := eval.Evaluate(ctx, points, iterations)
		if err != nil {
			t.Fatalf("%s: %v", tag, err)
		}
		worn, picked := evals[0], evals[1]
		line := func(what string, l Loadout, e *Evaluation) {
			t.Logf("%s %s: health %.0f, DTPS %.0f TMI %.1f TPS %.0f death %.4f", tag, what, health(l, stats.Stats{}),
				e.Metrics[MetricDTPS].Mean, e.Metrics[MetricTMI].Mean, e.Metrics[MetricTPS].Mean, e.Metrics[MetricPDeath].Mean)
		}
		t.Logf("%s: pinned %.0f HPS, warnings %v", tag, r.Target().GetHealingModel().GetHps(), obj.Warnings)
		line("worn", r.Equipped, worn)
		line("pick", pick, picked)
		line("pick, every socket Stamina", gemmed, evals[2])
		t.Logf("%s as the run scored them: worn death %.4f DTPS %.0f, pick death %.4f DTPS %.0f", tag,
			p.result.GetSeed().GetMetrics().GetPDeath(), p.result.GetSeed().GetMetrics().GetDtps(),
			best.GetMetrics().GetPDeath(), best.GetMetrics().GetDtps())
		d := obj.Delta(worn, picked)
		t.Logf("%s: pick against worn dJ %+.0f (se %.0f)", tag, d.Mean, d.SE)
		for i, o := range offsets {
			d := obj.Delta(picked, evals[3+i])
			t.Logf("%s at the pick: dJ per %s point %+.4f (se %.4f), per socket of %g %+.0f; dDeath %+.6f",
				tag, o.name, d.Mean/bump, d.SE/bump, o.gem, d.Mean/bump*o.gem, Delta(picked, evals[3+i], MetricPDeath).Mean/bump)
		}

		if obj.coef[MetricPDeath] != 0 {
			continue
		}
		// the run scored without death chance: walk the pick's health toward where it would count
		edge, step := 1.0, 250.0
		if picked.Metrics[MetricPDeath].Mean < 0.5 {
			edge, step = 0, -step
		}
		sweep := func(from, step float64, n int) (amounts []float64, deaths []float64) {
			var points []Point
			for i := 1; i <= n; i++ {
				// a self-heal off max health panics once Stamina goes far enough under zero
				if health(pick, stamina(from+float64(i)*step)) < 5000 {
					break
				}
				amounts = append(amounts, from+float64(i)*step)
				points = append(points, Point{Loadout: pick, Offset: stamina(from + float64(i)*step)})
			}
			evals, err := eval.Evaluate(ctx, points, iterations)
			if err != nil {
				t.Fatalf("%s: %v", tag, err)
			}
			for _, e := range evals {
				deaths = append(deaths, e.Metrics[MetricPDeath].Mean)
			}
			return amounts, deaths
		}
		amounts, deaths := sweep(0, step, 24)
		left := -1
		for i, death := range deaths {
			t.Logf("%s pick %+.0f Stamina: health %.0f, death %.4f", tag, amounts[i], health(pick, stamina(amounts[i])), death)
			if left < 0 && death != edge {
				left = i
			}
		}
		if left < 0 {
			t.Logf("%s: death chance stays at %.0f out to %+.0f Stamina", tag, edge, amounts[len(amounts)-1])
			continue
		}
		if picked.Metrics[MetricPDeath].Mean != edge {
			continue
		}
		// the tenths of the step it left in, then that step itself
		fine, fineDeaths := sweep(amounts[left]-step, step/10, 9)
		fine, fineDeaths = append(fine, amounts[left]), append(fineDeaths, deaths[left])
		for i, death := range fineDeaths {
			if death != edge {
				t.Logf("%s: death chance leaves %.0f at %+.0f Stamina, health %.0f (%.4f)", tag, edge, fine[i], health(pick, stamina(fine[i])), death)
				break
			}
		}
	}
}

// TestMeasureTankDeathAtPicks prints, per tank pick of a BiS Batch export, the death normalizer at the
// gear as worn and at the pick (what optimize's second search is gated on), the coefficient it gives
// death chance, and that coefficient capped at the Armor the loadout wears. It asserts nothing.
//
//	BIS_REQUESTS=<dir> BIS_PICKS=<batch-stage1.json> go test -tags with_db -run TestMeasureTankDeathAtPicks -v ./sim/optimizer/
func TestMeasureTankDeathAtPicks(t *testing.T) {
	picks := batchPicks(t)
	iterations := EffortBudget(proto.OptimizerEffort_OptimizerEffortNormal).Iterations
	ctx := context.Background()
	for _, p := range picks {
		tag := fmt.Sprintf("%s P%d", p.raider, p.phase)
		r := tankFightRequest(t, p.raider, p.phase)
		eval := NewSimEvaluator(r, MetricDTPS, MetricTMI, MetricTPS, MetricPDeath)
		best := p.result.GetBest()
		race := best.GetRacialTraits()
		if race == proto.Race_RaceUnknown {
			race = r.Seed.RacialTraits
		}
		pick, err := LoadoutFromProto(best.GetEquipment(), race)
		if err != nil {
			t.Fatalf("%s: %v", tag, err)
		}
		weight := TankWeights(r.Settings.GetTankSurvival())[MetricPDeath]
		for _, at := range []struct {
			what string
			l    Loadout
		}{{"worn", r.Equipped}, {"pick", pick}} {
			dm, err := measureDeath(ctx, eval, r, at.l, iterations)
			if err != nil {
				t.Fatalf("%s: %v", tag, err)
			}
			coef, left := deathCoefficient(weight, dm)
			uncapped := weight / dm.Norm.Mean
			t.Logf("%s %s: dies %.4f (%.4f at -2000 Armor, %.4f at +2000), %.3g ± %.2g per Armor point = %.1f se, Armor %.0f, coefficient %.0f uncapped, %.0f counted; left out: %q",
				tag, at.what, dm.Dies, dm.Down, dm.Up, dm.Norm.Mean, dm.Norm.SE, -dm.Norm.Mean/max(dm.Norm.SE, 1e-12), dm.Armor, uncapped, coef, left)
		}
	}
}
