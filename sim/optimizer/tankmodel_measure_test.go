package optimizer

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
	"github.com/wowsims/wotlk/sim/core/stats"
	goproto "google.golang.org/protobuf/proto"
)

// pageRequest is the request the raid page's BiS Batch posted for the named raider and content phase,
// out of the directory testdata/capture_requests.py filled. A request rebuilt here from the page's
// saved raid isn't the same fight: the save holds no rotation (the page resolves TypeAuto as it
// builds the request) and none of the buffs the page derives. The requests hold the roster and are
// never committed, so the directory comes from the environment.
func pageRequest(tb testing.TB, name string, phase int32) *proto.OptimizeGearRequest {
	tb.Helper()
	dir := os.Getenv("BIS_REQUESTS")
	if dir == "" {
		tb.Skip("BIS_REQUESTS names a directory of requests saved by testdata/capture_requests.py")
	}
	paths, err := filepath.Glob(filepath.Join(dir, "*.bin"))
	if err != nil {
		tb.Fatal(err)
	}
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			tb.Fatal(err)
		}
		req := &proto.OptimizeGearRequest{}
		if err := goproto.Unmarshal(data, req); err != nil {
			tb.Fatalf("%s: %v", path, err)
		}
		index := int(req.GetTargetRaidIndex())
		parties := req.GetBase().GetRaid().GetParties()
		if index/5 >= len(parties) || index%5 >= len(parties[index/5].GetPlayers()) {
			tb.Fatalf("%s: no player at raid index %d", path, index)
		}
		if parties[index/5].Players[index%5].GetName() == name && req.GetSettings().GetContentPhase() == phase {
			return req
		}
	}
	tb.Fatalf("no request for %s at phase %d in %s", name, phase, dir)
	return nil
}

// tankFightRequest is that request the way a run sims it: the tank alone, with what the raid gives it
// (simmedRequest), and the healing pinned.
func tankFightRequest(tb testing.TB, name string, phase int32) *Request {
	tb.Helper()
	if !core.WITH_DB {
		tb.Skip("needs the with_db item data")
	}
	asked, err := PrepareRequest(pageRequest(tb, name, phase))
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

// TestMeasureTankModel prints, per raider and phase, the tank fight's damage taken and chance of
// death in the gear as worn, the objective's normalizers, and what one more point of stamina,
// agility, dodge and defense rating is worth there. It asserts only that the objective keeps DTPS
// and TMI: death chance drops out wherever the worn gear all but always or never dies.
//
//	BIS_REQUESTS=<dir> go test -tags with_db -run TestMeasureTankModel -v ./sim/optimizer/
func TestMeasureTankModel(t *testing.T) {
	budget := EffortBudget(proto.OptimizerEffort_OptimizerEffortNormal)
	t.Logf("Normal budget: %+v", budget)
	iterations := budget.Iterations
	offsets := []struct {
		name string
		stat stats.Stat
	}{
		{"Stamina", stats.Stamina}, {"Agility", stats.Agility}, {"Dodge", stats.Dodge}, {"Defense", stats.Defense},
	}
	const bump = 100.0

	for _, name := range []string{"Bulwark", "Felesta"} {
		for phase := int32(1); phase <= 5; phase++ {
			r := tankFightRequest(t, name, phase)
			eval := NewSimEvaluator(r, MetricDTPS, MetricTMI, MetricTPS, MetricPDeath)
			obj, err := NewObjective(context.Background(), eval, r, iterations)
			if err != nil {
				t.Fatalf("%s phase %d: %v", name, phase, err)
			}
			t.Logf("%s phase %d: pinned %.0f HPS, weights %v normalizers %+v warnings %v", name, phase,
				r.Target().GetHealingModel().GetHps(), obj.Weights, obj.Normalizers, obj.Warnings)
			for _, m := range []Metric{MetricDTPS, MetricTMI, MetricPDeath} {
				norm := obj.Normalizers[m]
				t.Logf("%s phase %d: %s moves %+.3g per Armor point, %.1f se", name, phase, metricName(m), norm.Mean, -norm.Mean/norm.SE)
				if obj.coef[m] == 0 && m != MetricPDeath {
					t.Errorf("%s phase %d: %s left out of the objective", name, phase, metricName(m))
				}
			}
			points := []Point{{Loadout: r.Equipped}}
			for _, o := range offsets {
				var offset stats.Stats
				offset[o.stat] = bump
				points = append(points, Point{Loadout: r.Equipped, Offset: offset})
			}
			evals, err := eval.Evaluate(context.Background(), points, iterations)
			if err != nil {
				t.Fatal(err)
			}
			worn := evals[0]
			sheet, err := playerSheet(r.Base, r.TargetIndex, r.Equipped, stats.Stats{})
			if err != nil {
				t.Fatal(err)
			}
			t.Logf("%s phase %d worn: health %.0f armor %.0f, DTPS %.0f TMI %.0f TPS %.0f death %.4f", name, phase,
				sheet.FinalStats[stats.Health], sheet.FinalStats[stats.Armor],
				worn.Metrics[MetricDTPS].Mean, worn.Metrics[MetricTMI].Mean, worn.Metrics[MetricTPS].Mean, worn.Metrics[MetricPDeath].Mean)
			for i, o := range offsets {
				d := obj.Delta(worn, evals[i+1])
				t.Logf("%s phase %d: dJ per %s point %+.4f (se %.4f); dDTPS %+.3f dTMI %+.3f dDeath %+.6f",
					name, phase, o.name, d.Mean/bump, d.SE/bump,
					Delta(worn, evals[i+1], MetricDTPS).Mean/bump, Delta(worn, evals[i+1], MetricTMI).Mean/bump,
					Delta(worn, evals[i+1], MetricPDeath).Mean/bump)
			}
		}
	}
}

// The multiple of the worn gear's DTPS that pinHealingModel heals for.
const pinnedHealingMultiple = 1.5

// TestMeasureTankDeathTerm prints, per raider and phase, what the death normalizer comes to in the
// gear as worn under each thing that could rescue it: a wider Armor offset, more iterations, and
// healing at another multiple of the worn gear's DTPS. It asserts nothing.
//
//	BIS_REQUESTS=<dir> go test -tags with_db -run TestMeasureTankDeathTerm -v ./sim/optimizer/
func TestMeasureTankDeathTerm(t *testing.T) {
	iterations := EffortBudget(proto.OptimizerEffort_OptimizerEffortNormal).Iterations
	ctx := context.Background()
	for _, name := range []string{"Bulwark", "Felesta"} {
		for phase := int32(1); phase <= 5; phase++ {
			r := tankFightRequest(t, name, phase)
			measure := func(what string, offset float64, iterations int) {
				var down, up stats.Stats
				down[stats.Armor], up[stats.Armor] = -offset, offset
				points := []Point{{Loadout: r.Equipped}, {Loadout: r.Equipped, Offset: down}, {Loadout: r.Equipped, Offset: up}}
				// a new evaluator each time: a cached evaluation wouldn't know the healing changed
				evals, err := NewSimEvaluator(r, MetricPDeath).Evaluate(ctx, points, iterations)
				if err != nil {
					t.Fatalf("%s phase %d, healing %s: %v", name, phase, what, err)
				}
				d := Delta(evals[1], evals[2], MetricPDeath)
				se := 0.0
				if d.SE > 0 {
					se = -d.Mean / d.SE
				}
				t.Logf("%s phase %d, healing %s, %d iterations: worn death %.4f, %.4f and %.4f at -/+%g Armor, %.1f se", name, phase, what,
					iterations, evals[0].Metrics[MetricPDeath].Mean, evals[1].Metrics[MetricPDeath].Mean, evals[2].Metrics[MetricPDeath].Mean, offset, se)
			}

			for _, offset := range []float64{normalizerOffset(stats.Armor), 4000, 8000} {
				measure("as pinned", offset, iterations)
			}
			measure("as pinned", normalizerOffset(stats.Armor), 4*iterations)

			pinned := r.Target().GetHealingModel()
			for _, multiple := range []float64{0.75, 1, 1.25, 2, 2.5, 3, 4} {
				healing := goproto.Clone(pinned).(*proto.HealingModel)
				healing.Hps = pinned.GetHps() * multiple / pinnedHealingMultiple
				r.Target().HealingModel = healing
				measure(fmt.Sprintf("%g times DTPS (%.0f HPS)", multiple, healing.Hps), normalizerOffset(stats.Armor), iterations)
			}
		}
	}
}
