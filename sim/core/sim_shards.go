package core

import (
	"fmt"
	"log"
	"runtime"
	"runtime/debug"
	"strings"
	"sync"
	"time"

	"github.com/wowsims/wotlk/sim/core/proto"
	googleProto "google.golang.org/protobuf/proto"
)

// raidSimShards is how many shards RunRaidSimAsync splits a sim into: one per thread but one, which
// stays free to serve requests. Wasm runs on one thread, so it gets one shard.
func raidSimShards() int {
	return max(1, runtime.GOMAXPROCS(0)-1)
}

// shardSpan is a run of iterations: the first one and how many.
type shardSpan struct {
	first, iterations int32
}

// splitIterations cuts iterations into up to n contiguous spans. When they don't divide evenly, the
// first spans get one more.
func splitIterations(iterations int32, n int) []shardSpan {
	n = max(1, min(n, int(iterations)))
	spans := make([]shardSpan, n)
	size, extra := iterations/int32(n), iterations%int32(n)
	var first int32
	for k := range spans {
		spans[k] = shardSpan{first: first, iterations: size}
		if int32(k) < extra {
			spans[k].iterations++
		}
		first += spans[k].iterations
	}
	return spans
}

// shardRequest is rsr cut down to one span. A sim reseeds iteration i with RandomSeed + i, so
// starting the span at RandomSeed + first gives its iterations the seeds they get in one sim.
func shardRequest(rsr *proto.RaidSimRequest, span shardSpan) *proto.RaidSimRequest {
	req := googleProto.Clone(rsr).(*proto.RaidSimRequest)
	if req.SimOptions == nil {
		req.SimOptions = &proto.SimOptions{}
	}
	req.SimOptions.Iterations = span.iterations
	req.SimOptions.RandomSeed += int64(span.first)
	if span.first > 0 {
		req.SimOptions.DebugFirstIteration = false
	}
	return req
}

type raidSimShard struct {
	sim    *Simulation
	result *proto.RaidSimResult // the error when sim is nil
}

// runRaidSimShard runs one span the way runSim runs a whole request, and keeps the sim for the
// merge. Keep the two in step: TestRaidSimOneShardMatchesRunRaidSim compares them.
func runRaidSimShard(rsr *proto.RaidSimRequest, span shardSpan, presimDone func(), report func(*proto.ProgressMetrics)) (shard raidSimShard) {
	if !rsr.GetSimOptions().GetIsTest() {
		defer func() {
			if err := recover(); err != nil {
				shard = raidSimShard{result: &proto.RaidSimResult{ErrorResult: panicMessage(err)}}
			}
		}()
	}

	req := shardRequest(rsr, span)
	sim := NewSim(req)
	presimResult := sim.runPresims(req)
	presimDone()
	if presimResult != nil && presimResult.ErrorResult != "" {
		return raidSimShard{result: presimResult}
	}
	if sim.Encounter.EndFightAtHealth > 0 && presimResult != nil {
		sim.BaseDuration = time.Duration(presimResult.AvgIterationDuration) * time.Second
		sim.Duration = time.Duration(presimResult.AvgIterationDuration) * time.Second
		sim.Encounter.DurationIsEstimate = false
	}
	sim.ProgressReport = report
	return raidSimShard{sim: sim, result: sim.run()}
}

// panicMessage is the ErrorResult for a recovered panic, with the stack.
func panicMessage(err any) string {
	return fmt.Sprint(err) + "\nStack Trace:\n" + string(debug.Stack())
}

// shardProgress adds up the shards' progress reports.
type shardProgress struct {
	mu        sync.Mutex
	presims   int
	completed []int32
	dps, hps  []float64
}

func newShardProgress(shards int) *shardProgress {
	return &shardProgress{
		presims:   shards,
		completed: make([]int32, shards),
		dps:       make([]float64, shards),
		hps:       make([]float64, shards),
	}
}

func (p *shardProgress) presimDone() {
	p.mu.Lock()
	p.presims--
	p.mu.Unlock()
}

func (p *shardProgress) report(k int, pm *proto.ProgressMetrics) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.completed[k] = pm.CompletedIterations
	p.dps[k] = pm.Dps
	p.hps[k] = pm.Hps
	// the final report leaves Hps out
	if result := pm.FinalRaidResult; result != nil {
		p.hps[k] = result.GetRaidMetrics().GetHps().GetAvg()
	}
}

func (p *shardProgress) metrics(total int32) *proto.ProgressMetrics {
	p.mu.Lock()
	defer p.mu.Unlock()
	pm := &proto.ProgressMetrics{TotalIterations: total, PresimRunning: p.presims > 0}
	var dps, hps float64
	for k, done := range p.completed {
		pm.CompletedIterations += done
		dps += p.dps[k] * float64(done)
		hps += p.hps[k] * float64(done)
	}
	if pm.CompletedIterations > 0 {
		pm.Dps = dps / float64(pm.CompletedIterations)
		pm.Hps = hps / float64(pm.CompletedIterations)
	}
	return pm
}

// runRaidSimShards splits rsr's iterations across up to maxShards sims running side by side, and
// merges their results. Every shard replays its iterations' seeds from one sim, so the result only
// moves from runSim's through what a sim carries from one iteration to the next, and float rounding.
// It closes progress when done, after the final result.
func runRaidSimShards(rsr *proto.RaidSimRequest, progress chan *proto.ProgressMetrics, maxShards int) (result *proto.RaidSimResult) {
	total := rsr.GetSimOptions().GetIterations()
	send := func(pm *proto.ProgressMetrics) {
		if progress != nil {
			progress <- pm
		}
	}
	if progress != nil {
		defer close(progress)
	}
	if !rsr.GetSimOptions().GetIsTest() {
		defer func() {
			if err := recover(); err != nil {
				result = &proto.RaidSimResult{ErrorResult: panicMessage(err)}
				send(&proto.ProgressMetrics{TotalIterations: total, FinalRaidResult: result})
			}
		}()
	}

	start := time.Now()
	spans := splitIterations(total, maxShards)
	send(&proto.ProgressMetrics{TotalIterations: total, PresimRunning: true})

	tracker := newShardProgress(len(spans))
	shards := make([]raidSimShard, len(spans))
	done := make(chan int, len(spans))
	for k, span := range spans {
		go func() {
			var report func(*proto.ProgressMetrics)
			if progress != nil {
				report = func(pm *proto.ProgressMetrics) { tracker.report(k, pm) }
			}
			shards[k] = runRaidSimShard(rsr, span, tracker.presimDone, report)
			done <- k
		}()
	}

	var tick <-chan time.Time
	if progress != nil {
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()
		tick = ticker.C
	}
	for running := len(spans); running > 0; {
		select {
		case k := <-done:
			running--
			if shards[k].sim == nil {
				// no way to stop the other shards: they run out in the background, unread
				result = shards[k].result
				send(&proto.ProgressMetrics{TotalIterations: total, FinalRaidResult: result})
				return result
			}
		case <-tick:
			send(tracker.metrics(total))
		}
	}

	result = mergeRaidSimShards(shards)
	send(&proto.ProgressMetrics{
		TotalIterations:     total,
		CompletedIterations: total,
		Dps:                 result.RaidMetrics.Dps.Avg,
		FinalRaidResult:     result,
	})
	if total > 3000 {
		log.Printf("running %d iterations on %d shards took %s", total, len(spans), time.Since(start))
	}
	return result
}

// mergeRaidSimShards merges every shard into the first one's sim and builds the result from it. The
// log is every shard's log in order, which with DebugFirstIteration is only the first shard's.
func mergeRaidSimShards(shards []raidSimShard) *proto.RaidSimResult {
	sim := shards[0].sim
	var logs strings.Builder
	var total int32
	for k, shard := range shards {
		if k > 0 {
			sim.mergeShard(shard.sim)
		}
		logs.WriteString(shard.result.Logs)
		total += shard.sim.Options.Iterations
	}

	avgDuration := shards[0].result.AvgIterationDuration
	if len(shards) > 1 {
		avgDuration = 0
		for _, shard := range shards {
			avgDuration += shard.result.AvgIterationDuration * (float64(shard.sim.Options.Iterations) / float64(total))
		}
	}

	return &proto.RaidSimResult{
		RaidMetrics:            sim.Raid.GetMetrics(),
		EncounterMetrics:       sim.Encounter.GetMetricsProto(),
		Logs:                   logs.String(),
		FirstIterationDuration: shards[0].result.FirstIterationDuration,
		AvgIterationDuration:   avgDuration,
	}
}

// mergeShard adds o's metrics, from iterations after sim's, to sim's. Both sims come from the same
// request, so their parties and units line up by position.
func (sim *Simulation) mergeShard(o *Simulation) {
	if len(sim.AllUnits) != len(o.AllUnits) || len(sim.Raid.Parties) != len(o.Raid.Parties) {
		panic("sim shards don't have the same units")
	}
	sim.Raid.dpsMetrics.merge(&o.Raid.dpsMetrics)
	sim.Raid.hpsMetrics.merge(&o.Raid.hpsMetrics)
	for i, party := range sim.Raid.Parties {
		party.dpsMetrics.merge(&o.Raid.Parties[i].dpsMetrics)
		party.hpsMetrics.merge(&o.Raid.Parties[i].hpsMetrics)
	}
	for i, unit := range sim.AllUnits {
		other := o.AllUnits[i]
		if unit.Label != other.Label {
			panic(fmt.Sprintf("sim shards have %q and %q at unit %d", unit.Label, other.Label, i))
		}
		unit.Metrics.merge(&other.Metrics)
		unit.auraTracker.mergeAuraMetrics(&other.auraTracker)
	}
}

// Auras can be registered mid-sim, so shards don't always hold the same ones. Labels are unique per
// unit. An aura only o has joins as a bare aura holding its metrics: the sim is done, and nothing
// but GetMetricsProto reads it.
func (at *auraTracker) mergeAuraMetrics(o *auraTracker) {
	byLabel := make(map[string]*Aura, len(at.auras))
	for _, aura := range at.auras {
		byLabel[aura.Label] = aura
	}
	for _, aura := range o.auras {
		if mine := byLabel[aura.Label]; mine != nil {
			mine.metrics.merge(&aura.metrics)
		} else {
			at.auras = append(at.auras, &Aura{Label: aura.Label, metrics: aura.metrics})
		}
	}
}
