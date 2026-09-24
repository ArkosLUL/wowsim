package optimizer

import (
	"context"
	"testing"

	"github.com/wowsims/wotlk/sim/core/proto"
	"github.com/wowsims/wotlk/sim/core/stats"
)

// BenchmarkOptimizerEval measures what the effort budgets cost, per preset:
//   - setup: building one sim, from one-iteration shards
//   - iteration: one iteration on one thread, from DefaultShardIterations-iteration shards
//   - quick, normal, thorough: one evaluation at that effort's iterations with every worker busy,
//     and the whole budget's run time at that pace, on this machine's threads
//
// It's slow, so run it on its own:
//
//	tools/acore/dock.sh exec go test --tags=with_db -run '^$' -bench BenchmarkOptimizerEval ./sim/optimizer/
func BenchmarkOptimizerEval(b *testing.B) {
	efforts := []struct {
		name   string
		effort proto.OptimizerEffort
	}{
		{"quick", proto.OptimizerEffort_OptimizerEffortQuick},
		{"normal", proto.OptimizerEffort_OptimizerEffortNormal},
		{"thorough", proto.OptimizerEffort_OptimizerEffortThorough},
	}
	for _, preset := range []string{"fury_p1", "arcane_p3"} {
		b.Run(preset+"/setup", func(b *testing.B) {
			benchShards(b, preset, 1)
		})
		b.Run(preset+"/iteration", func(b *testing.B) {
			benchShards(b, preset, DefaultShardIterations)
		})
		for _, effort := range efforts {
			b.Run(preset+"/"+effort.name, func(b *testing.B) {
				budget := EffortBudget(effort.effort)
				r := presetRequest(b, preset)
				r.Settings.Effort = effort.effort
				workers := int(r.Settings.Workers)
				// one point per worker; each op gets a fresh evaluator, so nothing's cached
				points := make([]Point, workers)
				for i := range points {
					points[i] = withOffset(r.Seed, stats.Stamina, float64(i+1))
				}
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					if _, err := NewSimEvaluator(r, MetricDPS).Evaluate(context.Background(), points, budget.Iterations); err != nil {
						b.Fatal(err)
					}
				}
				perEvaluation := b.Elapsed().Seconds() / float64(b.N*workers)
				run := perEvaluation * float64(budget.Evaluations)
				b.ReportMetric(perEvaluation, "s/evaluation")
				b.ReportMetric(run, "s/run")
				b.ReportMetric(float64(workers), "threads")
			})
		}
	}
}

// benchShards runs shards of the given size back to back on one goroutine.
func benchShards(b *testing.B, preset string, size int) {
	r := presetRequest(b, preset)
	e := NewSimEvaluator(r, MetricDPS)
	seed := Point{Loadout: r.Seed}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := e.runSpan(seed, i*size, size); err != nil {
			b.Fatal(err)
		}
	}
	b.ReportMetric(float64(b.Elapsed().Nanoseconds())/float64(b.N*size), "ns/iteration")
}
