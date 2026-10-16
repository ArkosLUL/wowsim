package optimizer

import (
	"context"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
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

// The metrics a tank run weighs. The measurements keep samples of all of them, so their deltas pair.
var tankMetrics = []Metric{MetricDTPS, MetricTMI, MetricTPS, MetricPDeath, MetricHealthMargin}

// The ones normalized on Armor.
var survivalMetrics = []Metric{MetricDTPS, MetricTMI, MetricPDeath, MetricHealthMargin}

// tankOffsets are the stats the measurements add a bump of, with what one epic gem holds of each.
var tankOffsets = []struct {
	name string
	stat stats.Stat
	gem  float64
}{
	{"Stamina", stats.Stamina, 30}, {"Agility", stats.Agility, 20}, {"Dodge", stats.Dodge, 20}, {"Defense", stats.Defense, 20},
}

// seCount is how many standard errors n clears in the direction J wants.
func seCount(m Metric, n Estimate) float64 {
	if n.SE == 0 {
		return 0
	}
	if lowerIsBetter[m] {
		return -n.Mean / n.SE
	}
	return n.Mean / n.SE
}

// armorNormalizers are the survival metrics' normalizers from a pair of evals at -/+ the Armor offset.
func armorNormalizers(down, up *Evaluation) (out [NumMetrics]Estimate) {
	span := 2 * normalizerOffset(stats.Armor)
	for _, m := range survivalMetrics {
		d := Delta(down, up, m)
		out[m] = Estimate{Mean: d.Mean / span, SE: d.SE / span}
	}
	return out
}

func normalizersText(norms [NumMetrics]Estimate) string {
	parts := make([]string, len(survivalMetrics))
	for i, m := range survivalMetrics {
		parts[i] = fmt.Sprintf("%s %.4g ± %.2g (%.1f se)", metricName(m), norms[m].Mean, norms[m].SE, seCount(m, norms[m]))
	}
	return strings.Join(parts, "; ")
}

func coefficientsText(obj *Objective) string {
	var parts []string
	for m, c := range obj.coef {
		if c != 0 {
			parts = append(parts, fmt.Sprintf("%s %.4g", metricName(Metric(m)), c))
		}
	}
	return strings.Join(parts, ", ")
}

// byTerm splits J(b) - J(a), per point of bump, into each counted metric's share.
func byTerm(obj *Objective, a, b *Evaluation, bump float64) string {
	var parts []string
	for m, c := range obj.coef {
		if c != 0 {
			parts = append(parts, fmt.Sprintf("%s %+.4f", metricName(Metric(m)), c*Delta(a, b, Metric(m)).Mean/bump))
		}
	}
	return strings.Join(parts, ", ")
}

// marginPerStamina is what bump Stamina moves the health margin by, per point, against the max health
// it adds, and how many iterations move by all of it: less means something scaled with max health.
func marginPerStamina(a, b *Evaluation, health, bump float64) string {
	d := Delta(a, b, MetricHealthMargin)
	as, bs := a.samples[MetricHealthMargin], b.samples[MetricHealthMargin]
	n := min(len(as), len(bs))
	whole, less := 0, 0
	for i := 0; i < n; i++ {
		switch shift := float64(bs[i]) - float64(as[i]); {
		case math.Abs(shift-health) < 2:
			whole++
		case shift < health:
			less++
		}
	}
	return fmt.Sprintf("margin %+.2f ± %.2f against %.2f health (%.3f of it); %d of %d iterations move by all of it, %d by less",
		d.Mean/bump, d.SE/bump, health/bump, d.Mean/health, whole, n, less)
}

// timesToLive sims the points in the run's time-to-live fight: healing off, 60 s from the pull.
func timesToLive(tb testing.TB, r *Request, points []Point, iterations int) []Estimate {
	tb.Helper()
	evals, err := NewSimEvaluator(r).timeToLiveEvaluator(r).Evaluate(context.Background(), points, iterations)
	if err != nil {
		tb.Fatal(err)
	}
	out := make([]Estimate, len(evals))
	for i, e := range evals {
		out[i] = e.TimeToLive
	}
	return out
}

// TestMeasureTankModel prints, per raider and phase, the tank fight in the gear as worn: damage
// taken, chance of death, health margin and time to live; the objective's normalizers at Normal and
// at Quick; what one more point of stamina, agility, dodge and defense rating is worth there, term by
// term; and what the margin gains per point of Stamina against the health it adds. It asserts only
// that the objective keeps DTPS and TMI: death chance drops out wherever the worn gear all but
// always or never dies.
//
//	BIS_REQUESTS=<dir> go test -tags with_db -run TestMeasureTankModel -v ./sim/optimizer/
func TestMeasureTankModel(t *testing.T) {
	ctx := context.Background()
	budget := EffortBudget(proto.OptimizerEffort_OptimizerEffortNormal)
	t.Logf("Normal budget: %+v", budget)
	iterations := budget.Iterations
	const bump = 100.0

	for _, name := range []string{"Bulwark", "Felesta"} {
		for phase := int32(1); phase <= 5; phase++ {
			tag := fmt.Sprintf("%s P%d", name, phase)
			r := tankFightRequest(t, name, phase)
			eval := NewSimEvaluator(r, tankMetrics...)
			obj, err := NewObjective(ctx, eval, r, iterations)
			if err != nil {
				t.Fatalf("%s: %v", tag, err)
			}
			t.Logf("%s: pinned %.0f HPS, defensives at %.2f, weights %v, warnings %v", tag,
				r.Target().GetHealingModel().GetHps(), r.Target().GetCooldowns().GetHpPercentForDefensives(), obj.Weights, obj.Warnings)
			t.Logf("%s normalizers per Armor point: %s; coefficients %s", tag, normalizersText(obj.Normalizers), coefficientsText(obj))
			for _, m := range []Metric{MetricDTPS, MetricTMI} {
				if obj.coef[m] == 0 {
					t.Errorf("%s: %s left out of the objective", tag, metricName(m))
				}
			}

			// what a Quick run measures at the same gear, its own shard spans included
			quick := *r
			quick.Settings = goproto.Clone(r.Settings).(*proto.OptimizerSettings)
			quick.Settings.Effort = proto.OptimizerEffort_OptimizerEffortQuick
			quickObj, err := NewObjective(ctx, NewSimEvaluator(&quick, tankMetrics...), &quick, QuickIterations)
			if err != nil {
				t.Fatalf("%s at Quick: %v", tag, err)
			}
			t.Logf("%s normalizers at Quick's %d iterations: %s; coefficients %s; warnings %v", tag, QuickIterations,
				normalizersText(quickObj.Normalizers), coefficientsText(quickObj), quickObj.Warnings)

			points := []Point{{Loadout: r.Equipped}}
			for _, o := range tankOffsets {
				var offset stats.Stats
				offset[o.stat] = bump
				points = append(points, Point{Loadout: r.Equipped, Offset: offset})
			}
			evals, err := eval.Evaluate(ctx, points, iterations)
			if err != nil {
				t.Fatal(err)
			}
			worn := evals[0]
			sheet, err := playerSheet(r.Base, r.TargetIndex, r.Equipped, stats.Stats{})
			if err != nil {
				t.Fatal(err)
			}
			more, err := playerSheet(r.Base, r.TargetIndex, r.Equipped, points[1].Offset)
			if err != nil {
				t.Fatal(err)
			}
			lives := timesToLive(t, r, points[:2], iterations)
			margin := worn.Metrics[MetricHealthMargin]
			t.Logf("%s worn: health %.0f armor %.0f, DTPS %.0f TMI %.0f TPS %.0f death %.4f margin %.0f ± %.0f, time to live %.2f ± %.2f s (%.2f with +%g Stamina)", tag,
				sheet.FinalStats[stats.Health], sheet.FinalStats[stats.Armor],
				worn.Metrics[MetricDTPS].Mean, worn.Metrics[MetricTMI].Mean, worn.Metrics[MetricTPS].Mean, worn.Metrics[MetricPDeath].Mean,
				margin.Mean, margin.SE, lives[0].Mean, lives[0].SE, lives[1].Mean, bump)
			t.Logf("%s worn, per Stamina point: %s", tag,
				marginPerStamina(worn, evals[1], more.FinalStats[stats.Health]-sheet.FinalStats[stats.Health], bump))
			for i, o := range tankOffsets {
				e := evals[i+1]
				d := obj.Delta(worn, e)
				t.Logf("%s: dJ per %s point %+.4f (se %.4f), by term %s; dDTPS %+.3f dTMI %+.3f dDeath %+.6f dMargin %+.2f",
					tag, o.name, d.Mean/bump, d.SE/bump, byTerm(obj, worn, e, bump),
					Delta(worn, e, MetricDTPS).Mean/bump, Delta(worn, e, MetricTMI).Mean/bump,
					Delta(worn, e, MetricPDeath).Mean/bump, Delta(worn, e, MetricHealthMargin).Mean/bump)
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
