package optimizer

import (
	"context"
	"fmt"
	"math"
	"runtime/debug"
	"sync"
	"sync/atomic"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
	"github.com/wowsims/wotlk/sim/core/stats"
	"github.com/wowsims/wotlk/sim/optimizer/raidctx"
	goproto "google.golang.org/protobuf/proto"
)

// Point is one thing to sim: a loadout, plus offsets on the target's bonus stats (only response
// curves and the objective's normalizers set those). It's comparable, so it keys the evaluator's cache.
type Point struct {
	Loadout Loadout
	Offset  stats.Stats
}

// Evaluator sims points and returns their metrics with standard errors. All evaluations from one
// evaluator are paired: iteration i of every point runs on the same random numbers, so a Delta
// between two of them is far less noisy than either one.
type Evaluator interface {
	// Evaluate returns one evaluation per point, in order, each over at least iterations iterations.
	// One failed sim fails the whole call with a *SimError naming its point, so drop that point and
	// call again.
	Evaluate(ctx context.Context, points []Point, iterations int) ([]*Evaluation, error)
}

// SimError is a sim that failed: a panic in core, or an error result.
type SimError struct {
	Point   Point
	Message string
}

func (e *SimError) Error() string {
	return "sim failed: " + e.Message
}

// Evaluation is one point's results. Evaluators hand out shared ones, so never change one.
type Evaluation struct {
	Iterations int
	// Mean and standard error per metric.
	Metrics [NumMetrics]Estimate

	// Per-iteration values of the metrics the evaluator keeps, for pairing. Death chance never has
	// any: core only reports how many iterations the target died in.
	samples [NumMetrics][]float32
	sum     [NumMetrics]float64
	sumSq   [NumMetrics]float64
	deaths  int
}

// shard is one sim's iterations, before they're merged into an Evaluation.
type shard struct {
	n       int
	samples [NumMetrics][]float32
	sum     [NumMetrics]float64
	sumSq   [NumMetrics]float64
	deaths  int
}

// newShard takes per-iteration values for every metric but death chance, which comes as a count.
func newShard(values [NumMetrics][]float64, deaths int, keep [NumMetrics]bool) (shard, error) {
	s := shard{n: len(values[MetricDPS]), deaths: deaths}
	for m, v := range values {
		if Metric(m) == MetricPDeath {
			continue
		}
		if len(v) != s.n {
			return s, fmt.Errorf("got %d values for metric %d but %d for DPS", len(v), m, s.n)
		}
		for _, x := range v {
			s.sum[m] += x
			s.sumSq[m] += x * x
		}
		if keep[m] {
			s.samples[m] = make([]float32, len(v))
			for i, x := range v {
				s.samples[m][i] = float32(x)
			}
		}
	}
	return s, nil
}

// extend returns a new evaluation with the shards' iterations after e's. e can be nil.
func (e *Evaluation) extend(shards []shard) *Evaluation {
	out := &Evaluation{}
	if e != nil {
		out.Iterations = e.Iterations
		out.sum, out.sumSq, out.deaths = e.sum, e.sumSq, e.deaths
	}
	total := out.Iterations
	for _, s := range shards {
		total += s.n
	}
	for m := range out.samples {
		kept := len(shards) > 0 && shards[0].samples[m] != nil
		if e != nil && e.Iterations > 0 {
			kept = e.samples[m] != nil
		}
		if kept {
			out.samples[m] = make([]float32, 0, total)
			if e != nil {
				out.samples[m] = append(out.samples[m], e.samples[m]...)
			}
		}
	}
	for _, s := range shards {
		out.Iterations += s.n
		out.deaths += s.deaths
		for m := range out.sum {
			out.sum[m] += s.sum[m]
			out.sumSq[m] += s.sumSq[m]
			if out.samples[m] != nil {
				out.samples[m] = append(out.samples[m], s.samples[m]...)
			}
		}
	}

	n := float64(out.Iterations)
	if n == 0 {
		return out
	}
	for m := range out.Metrics {
		if Metric(m) == MetricPDeath {
			p := float64(out.deaths) / n
			out.Metrics[m] = Estimate{Mean: p, SE: math.Sqrt(p * (1 - p) / n)}
			continue
		}
		out.Metrics[m] = meanAndSE(out.Iterations, out.sum[m], out.sumSq[m])
	}
	return out
}

func meanAndSE(n int, sum, sumSq float64) Estimate {
	mean := sum / float64(n)
	if n < 2 {
		return Estimate{Mean: mean}
	}
	variance := max(0, (sumSq-float64(n)*mean*mean)/float64(n-1))
	return Estimate{Mean: mean, SE: math.Sqrt(variance / float64(n))}
}

// Delta is b's metric minus a's, paired over the iterations both ran.
func Delta(a, b *Evaluation, m Metric) Estimate {
	var c Metrics
	c[m] = 1
	return combinedDelta(a, b, c)
}

// combinedDelta is the sum of c[m] * (b[m] - a[m]). Metrics both evaluations kept samples for are
// paired iteration by iteration, over the iterations both ran; the rest count as independent.
// Means come from the exact sums when both ran the same iterations; samples are float32.
func combinedDelta(a, b *Evaluation, c Metrics) Estimate {
	n := min(a.Iterations, b.Iterations)
	var paired []int
	var mean, variance float64
	for m, cm := range c {
		if cm == 0 {
			continue
		}
		if n > 0 && len(a.samples[m]) >= n && len(b.samples[m]) >= n {
			paired = append(paired, m)
			if a.Iterations == b.Iterations {
				mean += cm * (b.Metrics[m].Mean - a.Metrics[m].Mean)
			}
			continue
		}
		mean += cm * (b.Metrics[m].Mean - a.Metrics[m].Mean)
		variance += cm * cm * (a.Metrics[m].SE*a.Metrics[m].SE + b.Metrics[m].SE*b.Metrics[m].SE)
	}
	if len(paired) > 0 {
		var sum, sumSq float64
		for i := 0; i < n; i++ {
			d := 0.0
			for _, m := range paired {
				d += c[m] * (float64(b.samples[m][i]) - float64(a.samples[m][i]))
			}
			sum += d
			sumSq += d * d
		}
		est := meanAndSE(n, sum, sumSq)
		if a.Iterations != b.Iterations {
			mean += est.Mean
		}
		variance += est.SE * est.SE
	}
	return Estimate{Mean: mean, SE: math.Sqrt(variance)}
}

// combinedScore is the sum of c[m] * e[m]. Kept metrics combine per iteration for the standard
// error, so their covariance counts; the rest add up as independent.
func combinedScore(e *Evaluation, c Metrics) Estimate {
	var mean, variance float64
	var kept []int
	for m, cm := range c {
		if cm == 0 {
			continue
		}
		mean += cm * e.Metrics[m].Mean
		if e.Iterations > 0 && len(e.samples[m]) == e.Iterations {
			kept = append(kept, m)
			continue
		}
		variance += cm * cm * e.Metrics[m].SE * e.Metrics[m].SE
	}
	if len(kept) > 0 {
		var sum, sumSq float64
		for i := 0; i < e.Iterations; i++ {
			v := 0.0
			for _, m := range kept {
				v += c[m] * float64(e.samples[m][i])
			}
			sum += v
			sumSq += v * v
		}
		est := meanAndSE(e.Iterations, sum, sumSq)
		variance += est.SE * est.SE
	}
	return Estimate{Mean: mean, SE: math.Sqrt(variance)}
}

// DefaultShardIterations is how many iterations one sim runs. Building a sim costs about 1% of
// that many Fury or Arcane iterations (BenchmarkOptimizerEval's /setup against /iteration), and
// shards this small keep every worker busy on a handful of points and notice a cancel quickly.
const DefaultShardIterations = 250

// defaultRandomSeed stands in when the request has none, so runs stay reproducible.
const defaultRandomSeed = 1

// SimEvaluator evaluates the request's target in the real sim.
type SimEvaluator struct {
	base        *proto.RaidSimRequest
	targetIndex int
	seed        int64
	shardSize   int
	keep        [NumMetrics]bool
	// True for a raid-contribution evaluator: MetricDPS comes from raidctx.RaidDPS instead of the
	// target's own player metrics.
	raidWide bool
	// One token per worker, shared by concurrent Evaluate calls.
	workers chan struct{}

	mu    sync.Mutex
	cache map[Point]*Evaluation

	simmed atomic.Int64
}

// NewSimEvaluator sims r's target on r.Settings.Workers goroutines. It keeps per-iteration values
// for the given metrics, so deltas in them come out paired; the others still get means and
// standard errors.
func NewSimEvaluator(r *Request, keep ...Metric) *SimEvaluator {
	e := &SimEvaluator{
		base:        r.Base,
		targetIndex: r.TargetIndex,
		seed:        r.Base.GetSimOptions().GetRandomSeed(),
		shardSize:   DefaultShardIterations,
		workers:     make(chan struct{}, max(1, int(r.Settings.GetWorkers()))),
		cache:       make(map[Point]*Evaluation),
	}
	if e.seed == 0 {
		e.seed = defaultRandomSeed
	}
	for _, m := range keep {
		if m != MetricPDeath {
			e.keep[m] = true
		}
	}
	return e
}

// NewRaidEvaluator sims r's target inside the whole raid r.Base names, at r.TargetIndex: r is the
// asked request (the real roster), not the derived individual context. MetricDPS comes back as the
// raid's total DPS (raidctx.RaidDPS), which is what OptimizerObjectiveRaidDps scores a DPS raider's
// gear against; the other metrics still read the target's own, same as NewSimEvaluator.
func NewRaidEvaluator(r *Request, keep ...Metric) *SimEvaluator {
	e := NewSimEvaluator(r, keep...)
	e.raidWide = true
	return e
}

// SimmedIterations counts the iterations this evaluator has run; cache hits don't count.
func (e *SimEvaluator) SimmedIterations() int64 {
	return e.simmed.Load()
}

// Evaluate rounds iterations up to whole shards. Shard k always covers iterations k*m to
// (k+1)*m - 1 with RandomSeed + k*m, so a cached point only sims the shards it's missing, and the
// result doesn't depend on the worker count.
func (e *SimEvaluator) Evaluate(ctx context.Context, points []Point, iterations int) ([]*Evaluation, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	shards := max(1, (iterations+e.shardSize-1)/e.shardSize)

	type pending struct {
		point  Point
		prev   *Evaluation
		first  int
		shards []shard
	}
	type job struct {
		p *pending
		k int
	}

	e.mu.Lock()
	var todo []*pending
	var jobs []job
	seen := make(map[Point]bool, len(points))
	for _, p := range points {
		if seen[p] {
			continue
		}
		seen[p] = true
		prev := e.cache[p]
		have := 0
		if prev != nil {
			have = prev.Iterations / e.shardSize
		}
		if have >= shards {
			continue
		}
		pd := &pending{point: p, prev: prev, first: have, shards: make([]shard, shards-have)}
		todo = append(todo, pd)
		for k := have; k < shards; k++ {
			jobs = append(jobs, job{pd, k})
		}
	}
	e.mu.Unlock()

	var firstErr *SimError
	if len(jobs) > 0 {
		runCtx, cancel := context.WithCancel(ctx)
		defer cancel()
		var (
			errMu sync.Mutex
			wg    sync.WaitGroup
		)
		queue := make(chan job)
		for range min(cap(e.workers), len(jobs)) {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for j := range queue {
					if runCtx.Err() != nil {
						continue
					}
					select {
					case e.workers <- struct{}{}:
					case <-runCtx.Done():
						continue
					}
					s, err := e.runShard(j.p.point, j.k)
					<-e.workers
					if err != nil {
						errMu.Lock()
						if firstErr == nil {
							firstErr = err
						}
						errMu.Unlock()
						cancel()
						continue
					}
					j.p.shards[j.k-j.p.first] = s
				}
			}()
		}
	feed:
		for _, j := range jobs {
			select {
			case queue <- j:
			case <-runCtx.Done():
				break feed
			}
		}
		close(queue)
		wg.Wait()
	}

	e.mu.Lock()
	defer e.mu.Unlock()
	// after an error or a cancel, whatever run of shards finished first still counts
	for _, pd := range todo {
		done := 0
		for done < len(pd.shards) && pd.shards[done].n > 0 {
			done++
		}
		if done == 0 {
			continue
		}
		ev := pd.prev.extend(pd.shards[:done])
		// a concurrent call may have gone further already
		if cur := e.cache[pd.point]; cur == nil || cur.Iterations < ev.Iterations {
			e.cache[pd.point] = ev
		}
	}
	// failures aren't cached: some core panics are one-offs
	if firstErr != nil {
		return nil, firstErr
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	out := make([]*Evaluation, len(points))
	for i, p := range points {
		out[i] = e.cache[p]
	}
	return out, nil
}

func (e *SimEvaluator) runShard(p Point, k int) (s shard, err *SimError) {
	defer func() {
		// IsTest turns off core's own recover
		if r := recover(); r != nil {
			err = &SimError{Point: p, Message: fmt.Sprintf("%v\nStack Trace:\n%s", r, debug.Stack())}
		}
	}()

	result := core.RunRaidSim(e.request(p, k))
	if result.ErrorResult != "" {
		return s, &SimError{Point: p, Message: result.ErrorResult}
	}
	metrics := result.GetRaidMetrics()
	parties := metrics.GetParties()
	if e.targetIndex/5 >= len(parties) || e.targetIndex%5 >= len(parties[e.targetIndex/5].GetPlayers()) {
		return s, &SimError{Point: p, Message: fmt.Sprintf("the sim reported no metrics for raid index %d", e.targetIndex)}
	}
	m := parties[e.targetIndex/5].GetPlayers()[e.targetIndex%5]
	values := [NumMetrics][]float64{
		MetricDPS:  m.GetDps().GetAllValues(),
		MetricHPS:  m.GetHps().GetAllValues(),
		MetricTPS:  m.GetThreat().GetAllValues(),
		MetricDTPS: m.GetDtps().GetAllValues(),
		MetricTMI:  m.GetTmi().GetAllValues(),
	}
	if e.raidWide {
		values[MetricDPS] = raidctx.RaidDPS(metrics)
	}
	n := len(values[MetricDPS])
	// the cache counts whole shards, and an empty one would never finish
	if n != e.shardSize {
		return s, &SimError{Point: p, Message: fmt.Sprintf("the sim reported %d iterations, want %d", n, e.shardSize)}
	}
	s, verr := newShard(values, int(math.Round(m.GetChanceOfDeath()*float64(n))), e.keep)
	if verr != nil {
		return s, &SimError{Point: p, Message: verr.Error()}
	}
	e.simmed.Add(int64(n))
	return s, nil
}

// request clones the base for every sim: core's GetRaidBuffs writes into the raid it's given.
func (e *SimEvaluator) request(p Point, k int) *proto.RaidSimRequest {
	rsr := goproto.Clone(e.base).(*proto.RaidSimRequest)
	player := rsr.Raid.Parties[e.targetIndex/5].Players[e.targetIndex%5]
	player.Equipment = p.Loadout.Equipment()
	player.RacialTraits = p.Loadout.RacialTraits
	if p.Offset != (stats.Stats{}) {
		if player.BonusStats == nil {
			player.BonusStats = &proto.UnitStats{}
		}
		player.BonusStats.Stats = stats.FromFloatArray(player.BonusStats.Stats).Add(p.Offset).ToFloatArray()
	}
	rsr.SimOptions = &proto.SimOptions{
		Iterations: int32(e.shardSize),
		RandomSeed: e.seed + int64(k*e.shardSize),
		// one random stream per label, reseeded every iteration, is what pairs the points
		IsTest:        true,
		SaveAllValues: true,
	}
	return rsr
}

// Budget is how much simming one optimization gets.
type Budget struct {
	// Iterations per evaluation.
	Iterations int
	// Evaluations for the whole run.
	Evaluations int
}

// Effort budgets, from BenchmarkOptimizerEval on 16 threads: Fury P1, the slower preset, lands a
// Quick run in about 6 s, Normal in 1.5 min and Thorough in 7.5, inside the investigation's
// targets. Arcane P3 takes 40% of that. Keep iterations whole shards.
const (
	QuickIterations     = 500
	QuickEvaluations    = 250
	NormalIterations    = 4000
	NormalEvaluations   = 500
	ThoroughIterations  = 10000
	ThoroughEvaluations = 1000
)

// EffortBudget returns the budget for an effort; unknown efforts get Normal's.
func EffortBudget(effort proto.OptimizerEffort) Budget {
	switch effort {
	case proto.OptimizerEffort_OptimizerEffortQuick:
		return Budget{Iterations: QuickIterations, Evaluations: QuickEvaluations}
	case proto.OptimizerEffort_OptimizerEffortThorough:
		return Budget{Iterations: ThoroughIterations, Evaluations: ThoroughEvaluations}
	default:
		return Budget{Iterations: NormalIterations, Evaluations: NormalEvaluations}
	}
}
