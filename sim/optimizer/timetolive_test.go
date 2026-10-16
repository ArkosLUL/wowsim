package optimizer

import (
	"context"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/wowsims/wotlk/sim/core/proto"
	"github.com/wowsims/wotlk/sim/core/stats"
	goproto "google.golang.org/protobuf/proto"
)

// pinnedTankRequest is the Anub'arak paladin with its healing pinned, as a run sims it.
func pinnedTankRequest(t *testing.T) *Request {
	t.Helper()
	r, err := PrepareRequest(tankOptimizeRequest(t, protPaladin(t, "p3"), 3))
	if err != nil {
		t.Fatal(err)
	}
	if err := (&run{r: r}).pinHealingModel(); err != nil {
		t.Fatal(err)
	}
	return r
}

// The time-to-live fight is the tank's own with the healing off, 60 s long, on a copy of the request.
// A tank that lives through it scores the 60 s exactly; one that dies, the time it died.
func TestTimeToLiveFight(t *testing.T) {
	r := pinnedTankRequest(t)
	before := goproto.Clone(r.Base)
	fight := timeToLiveRequest(r)
	if !goproto.Equal(before, r.Base) {
		t.Fatal("timeToLiveRequest changed the run's request")
	}
	healing := fight.Target().GetHealingModel()
	if healing.GetHps() != 0 || healing.GetCadenceSeconds() != 0 || healing.GetBurstWindow() != r.Target().GetHealingModel().GetBurstWindow() {
		t.Errorf("healing model %v, want no healing and the burst window kept", healing)
	}
	if enc := fight.Base.Encounter; enc.Duration != timeToLiveSeconds || enc.DurationVariation != 0 || enc.UseHealth {
		t.Errorf("encounter %v s ± %v, health %v; want %d s flat", enc.Duration, enc.DurationVariation, enc.UseHealth, timeToLiveSeconds)
	}

	var tough stats.Stats
	tough[stats.Health] = 1_000_000
	evals := evaluate(t, NewSimEvaluator(r).timeToLiveEvaluator(r), 500, Point{Loadout: r.Seed}, Point{Loadout: r.Seed, Offset: tough})
	if dies := evals[0].TimeToLive; !(dies.Mean > 0 && dies.Mean < timeToLiveSeconds && dies.SE > 0) {
		t.Errorf("unhealed, the paladin lives %+v s; want under %d", dies, timeToLiveSeconds)
	}
	if lives := evals[1].TimeToLive; lives != (Estimate{Mean: timeToLiveSeconds}) {
		t.Errorf("with a million more health it lives %+v s, want the whole %d", lives, timeToLiveSeconds)
	}
	t.Logf("unhealed: %+v s", evals[0].TimeToLive)
}

// The healed fight measures a health margin only where the tank has a healing model; without one the
// evaluator reads it as 0.
func TestHealthMarginNeedsTheHealingModel(t *testing.T) {
	r := pinnedTankRequest(t)
	healed := evaluate(t, NewSimEvaluator(r, MetricHealthMargin), 500, Point{Loadout: r.Seed})[0]
	if margin := healed.Metrics[MetricHealthMargin]; margin.Mean == 0 || margin.SE == 0 || len(healed.samples[MetricHealthMargin]) != 500 {
		t.Errorf("healed margin %+v over %d samples", margin, len(healed.samples[MetricHealthMargin]))
	}
	t.Logf("healed margin %+v, death chance %+v", healed.Metrics[MetricHealthMargin], healed.Metrics[MetricPDeath])

	bare := *r
	bare.Base = goproto.Clone(r.Base).(*proto.RaidSimRequest)
	bare.Target().HealingModel = nil
	unhealed := evaluate(t, NewSimEvaluator(&bare, MetricHealthMargin), 500, Point{Loadout: r.Seed})[0]
	if margin := unhealed.Metrics[MetricHealthMargin]; margin != (Estimate{}) || unhealed.TimeToLive != (Estimate{}) {
		t.Errorf("without a healing model: margin %+v, time to live %+v; want none", margin, unhealed.TimeToLive)
	}
}

// livesEvaluator is a fake time-to-live fight: every iteration of a point lives lives(point) seconds.
type livesEvaluator struct {
	lives func(Point) float64
	calls *int
}

func (f livesEvaluator) Evaluate(_ context.Context, points []Point, iterations int) ([]*Evaluation, error) {
	*f.calls++
	out := make([]*Evaluation, len(points))
	for i, p := range points {
		s, err := newShard([NumMetrics][]float64{MetricDPS: make([]float64, iterations), MetricHPS: make([]float64, iterations),
			MetricTPS: make([]float64, iterations), MetricDTPS: make([]float64, iterations), MetricTMI: make([]float64, iterations)}, [NumMetrics]bool{})
		if err != nil {
			return nil, err
		}
		lives := make([]float64, iterations)
		for it := range lives {
			lives[it] = f.lives(p)
		}
		s.lives.add(lives)
		out[i] = (*Evaluation)(nil).extend([]shard{s})
	}
	return out, nil
}

// tieEvaluator is a fakeEvaluator whose time-to-live fight is a livesEvaluator.
type tieEvaluator struct {
	*fakeEvaluator
	lives      func(Point) float64
	livesCalls int
}

func (f *tieEvaluator) timeToLiveEvaluator(*Request) Evaluator {
	return livesEvaluator{lives: f.lives, calls: &f.livesCalls}
}

// tieRun is a tank run over three verified heads, best first: two tied on score, then one well behind,
// all better than the seed's. Set the fake's lives before breaking the tie.
func tieRun(t *testing.T) (*run, *tieEvaluator, []*verifiedLoadout) {
	t.Helper()
	const (
		tiedA = 90011
		tiedB = 90012
		worse = 90013
	)
	r := tankRequest(t, &proto.OptimizerMetrics{Tps: 0.3, Dtps: 0.7})
	r.Target().HealingModel = &proto.HealingModel{CadenceSeconds: 2.5, BurstWindow: 6}
	fake := &tieEvaluator{fakeEvaluator: newFakeEvaluator(func(p Point) Metrics {
		m := Metrics{MetricTPS: 3000 + p.Offset[stats.AttackPower], MetricDTPS: 1000 - 0.05*p.Offset[stats.Armor]}
		switch p.Loadout.Items[proto.ItemSlot_ItemSlotHead].ItemID {
		case tiedA:
			m[MetricDTPS] -= 100
		case tiedB:
			m[MetricDTPS] -= 99.9
		case worse:
			m[MetricDTPS] -= 50
		}
		return m
	})}
	fake.shared, fake.own = 0.01, 5

	run := &run{ctx: context.Background(), asked: r, r: r, budget: EffortBudget(proto.OptimizerEffort_OptimizerEffortQuick)}
	run.eval = &countingEvaluator{inner: fake, have: map[Point]int{}}
	obj, err := NewObjective(context.Background(), run.eval, r, 1000)
	if err != nil {
		t.Fatal(err)
	}
	run.obj = obj
	wearing := func(id int32) Loadout {
		l := r.Seed
		l.Items[proto.ItemSlot_ItemSlotHead].ItemID = id
		return l
	}
	points := []Point{{Loadout: r.Seed}, {Loadout: wearing(tiedA)}, {Loadout: wearing(tiedB)}, {Loadout: wearing(worse)}}
	evals := evaluate(t, run.eval, 1000, points...)
	run.seedEval = evals[0]
	var verified []*verifiedLoadout
	for i, p := range points[1:] {
		verified = append(verified, &verifiedLoadout{p.Loadout, evals[i+1]})
	}
	run.sortVerified(verified)
	if d := obj.Delta(verified[0].eval, verified[1].eval); d.Mean+2*d.SE < 0 || verified[2].loadout != wearing(worse) {
		t.Fatalf("the fixture's two best heads aren't tied (%+v) or the third isn't behind", d)
	}
	return run, fake, verified
}

func livesOf(lives map[Loadout]float64) func(Point) float64 {
	return func(p Point) float64 { return lives[p.Loadout] }
}

// Two loadouts within 2 standard errors of each other on score go to time to live, and the
// longer-lived one stands, whichever scored a hair better.
func TestTieBreaksOnTimeToLive(t *testing.T) {
	run, fake, verified := tieRun(t)
	byScore, runnerUp, behind := verified[0], verified[1], verified[2]
	fake.lives = livesOf(map[Loadout]float64{byScore.loadout: 21.5, runnerUp.loadout: 34.25, behind.loadout: 60})
	run.breakTie(verified)
	if fake.livesCalls != 1 {
		t.Errorf("simmed the time-to-live fight %d times, want once", fake.livesCalls)
	}
	if verified[0] != runnerUp || verified[1] != byScore || verified[2] != behind {
		t.Error("the longer-lived of the tied two didn't go first, ahead of the best score")
	}
	warnings := run.allWarnings()
	if len(warnings) != 1 || !strings.Contains(warnings[0], "2 loadouts tied") || !strings.Contains(warnings[0], "34.2") ||
		!strings.Contains(warnings[0], "21.5") || strings.Contains(warnings[0], "60.0") {
		t.Errorf("warnings %q, want one naming both tied loadouts' times and not the third's", warnings)
	}
	if got := run.livesSimmed(); got != 2*int64(run.budget.Iterations) {
		t.Errorf("counted %d time-to-live iterations, want 2 x %d", got, run.budget.Iterations)
	}
}

// The best by score keeps its place when it also lives longest, and the tie is still named.
func TestTieKeepsTheBestWhenItLivesLongest(t *testing.T) {
	run, fake, verified := tieRun(t)
	byScore := verified[0]
	fake.lives = livesOf(map[Loadout]float64{byScore.loadout: 40, verified[1].loadout: 30})
	run.breakTie(verified)
	if verified[0] != byScore || len(run.allWarnings()) != 1 {
		t.Errorf("the best by score moved, or the tie went unnamed: %q", run.allWarnings())
	}
}

// No tie, no time-to-live sims; and a run that isn't a tank's never breaks one.
func TestTieBreakOnlyForATanksTie(t *testing.T) {
	run, fake, verified := tieRun(t)
	fake.lives = livesOf(map[Loadout]float64{verified[0].loadout: 10, verified[2].loadout: 60})
	clear := []*verifiedLoadout{verified[0], verified[2]}
	run.breakTie(clear)
	if fake.livesCalls != 0 || clear[0] != verified[0] {
		t.Errorf("a clear winner went to time to live (%d sims)", fake.livesCalls)
	}

	run, fake, verified = tieRun(t)
	fake.lives = livesOf(map[Loadout]float64{verified[0].loadout: 10, verified[1].loadout: 60})
	run.r.Target().HealingModel = nil
	byScore := verified[0]
	run.breakTie(verified)
	if fake.livesCalls != 0 || verified[0] != byScore {
		t.Errorf("a target without a healing model went to time to live (%d sims)", fake.livesCalls)
	}
}

// A seed or equipped gear that sims NaN fails the run, naming the metric and the gear.
func TestNaNFailsTheRun(t *testing.T) {
	for _, tc := range []struct {
		name string
		at   func(r *Request) Loadout
		want string
	}{
		{"the seed", func(r *Request) Loadout { return r.Seed }, "the gear the search starts from"},
		{"the gear as equipped", func(r *Request) Loadout { return r.Equipped }, "the gear as equipped"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := kaRequest(proto.OptimizerEffort_OptimizerEffortQuick)
			req.Equipped = goproto.Clone(req.Base.Raid.Parties[0].Players[0].Equipment).(*proto.EquipmentSpec)
			req.Equipped.Items[proto.ItemSlot_ItemSlotHead] = &proto.ItemSpec{}
			r, err := PrepareRequest(req)
			if err != nil {
				t.Fatal(err)
			}
			if r.Seed == r.Equipped {
				t.Fatal("the fixture needs the gear as equipped to differ from the seed")
			}
			nan := tc.at(r)
			fake := newFakeEvaluator(func(p Point) Metrics {
				m, _ := kaMetrics(p)
				if p == (Point{Loadout: nan}) {
					m[MetricDPS] = math.NaN()
				}
				return m
			})
			result := optimize(context.Background(), r, r, fake, nil, time.Now())
			if !strings.Contains(result.ErrorResult, tc.want) || !strings.Contains(result.ErrorResult, "DPS as NaN") {
				t.Errorf("error %q, want one naming %s and DPS", result.ErrorResult, tc.want)
			}
			if tc.at(r) == r.Equipped && !strings.Contains(result.ErrorResult, "empty: Head") {
				t.Errorf("error %q doesn't name the empty head", result.ErrorResult)
			}
		})
	}
}

func TestCalibrationGapFailsOnNaN(t *testing.T) {
	var l Loadout
	l.Items[proto.ItemSlot_ItemSlotHead].ItemID = 1
	for _, tc := range []struct {
		own, real, gap float64
		fails          bool
	}{
		{1100, 1000, 0.1, false},
		{1100, 0, 0, false},
		{1100, math.NaN(), 0, true},
		{math.NaN(), 1000, 0, true},
	} {
		gap, err := calibrationGap(l, tc.own, tc.real)
		if (err != nil) != tc.fails || !near(gap, tc.gap, 1e-12) {
			t.Errorf("own %v, real %v: gap %v, %v", tc.own, tc.real, gap, err)
		}
		if err != nil && (!strings.Contains(err.Error(), "NaN") || !strings.Contains(err.Error(), "empty: Neck")) {
			t.Errorf("error %q doesn't say NaN or name the loadout's empty slots", err)
		}
	}
}
