package optimizer

import (
	"context"
	"fmt"
	"runtime/debug"
	"time"

	"github.com/wowsims/wotlk/sim/core/proto"
	goproto "google.golang.org/protobuf/proto"
)

// SimCommit is the sim's git commit, stamped on every result. Builds can set it with
// -ldflags "-X github.com/wowsims/wotlk/sim/optimizer.SimCommit=<sha>"; without that it comes from
// the Go build's VCS info, which Docker builds don't have (.git is ignored).
var SimCommit string

func init() {
	if SimCommit == "" {
		SimCommit = vcsCommit()
	}
}

func vcsCommit() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "unknown"
	}
	revision, modified := "", false
	for _, setting := range info.Settings {
		switch setting.Key {
		case "vcs.revision":
			revision = setting.Value
		case "vcs.modified":
			modified = setting.Value == "true"
		}
	}
	if revision == "" {
		return "unknown"
	}
	if modified {
		revision += "-dirty"
	}
	return revision
}

// Optimize finds the target's BiS. It always returns a result: failures go in its error_result, and
// a cancelled ctx returns the best found so far with cancelled set.
//
// For now it's a stub that hands back the seed.
func Optimize(ctx context.Context, req *proto.OptimizeGearRequest, progress ProgressFunc) (result *proto.OptimizerResult) {
	start := time.Now()
	defer func() {
		if err := recover(); err != nil {
			result = &proto.OptimizerResult{
				SimCommit:   SimCommit,
				ErrorResult: fmt.Sprintf("%v\nStack Trace:\n%s", err, debug.Stack()),
			}
		}
	}()

	r, err := PrepareRequest(req)
	if err != nil {
		return &proto.OptimizerResult{
			SimCommit:   SimCommit,
			ErrorResult: err.Error(),
		}
	}

	if progress != nil {
		progress(&proto.OptimizerProgress{
			Stage:          "Seed",
			CompletedSteps: 1,
			TotalSteps:     1,
			RacialTraits:   r.Seed.RacialTraits,
			ElapsedSeconds: time.Since(start).Seconds(),
		})
	}

	seed := &proto.OptimizerLoadoutResult{
		Equipment:    r.Seed.Equipment(),
		RacialTraits: r.Seed.RacialTraits,
	}
	return &proto.OptimizerResult{
		Best:            seed,
		Seed:            goproto.Clone(seed).(*proto.OptimizerLoadoutResult),
		Settings:        r.Settings,
		TargetRaidIndex: int32(r.TargetIndex),
		SimCommit:       SimCommit,
		CatalogDate:     r.Pool.CatalogDate,
		ElapsedSeconds:  time.Since(start).Seconds(),
		Cancelled:       ctx.Err() != nil,
	}
}

// RunAsync runs Optimize in the background. Progress goes to reporter as optimizer_progress, then the
// result as final_optimize_result, and then reporter is closed. Drain it until it's closed.
func RunAsync(ctx context.Context, req *proto.OptimizeGearRequest, reporter chan *proto.ProgressMetrics) {
	go func() {
		defer close(reporter)
		result := Optimize(ctx, req, func(p *proto.OptimizerProgress) {
			select {
			case reporter <- &proto.ProgressMetrics{OptimizerProgress: p, CompletedSims: p.CompletedSims, TotalSims: p.TotalSims}:
			default:
				// readers only show the latest update, so skipping one beats stalling the search
			}
		})
		reporter <- &proto.ProgressMetrics{FinalOptimizeResult: result}
	}()
}
