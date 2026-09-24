// Package harness times the sim's and the optimizer's entry points on fixed requests across a
// GOMAXPROCS sweep, and compares the report with a baseline.
package harness

import (
	"compress/gzip"
	"context"
	"embed"
	"errors"
	"fmt"
	"io"
	"path"
	"runtime"
	"strings"
	"time"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
	"github.com/wowsims/wotlk/sim/optimizer"
	"google.golang.org/protobuf/encoding/protojson"
	goproto "google.golang.org/protobuf/proto"
)

// Written by scenarios/update.sh.
//
//go:embed scenarios/*.json.gz
var scenarioFiles embed.FS

type Kind string

const (
	KindSim         Kind = "sim"
	KindStatWeights Kind = "statweights"
	KindBulk        Kind = "bulk"
	KindOptimizer   Kind = "optimizer"
)

// The spec stat weights and the bulk sim run on.
const secondarySpec = "rogue"

// A fixed seed makes every rep sim the same fights.
const randomSeed = 101

// Scenario is one request to one entry point.
type Scenario struct {
	Name string
	Kind Kind
	// Simmed at a few iterations before the timed runs, so the first rep doesn't pay one-time setup.
	warmup *proto.RaidSimRequest
	run    func(ctx context.Context, workers int) (*Sample, error)
}

// Scenarios lists every scenario, sims and stat weights and bulk at the given iterations.
func Scenarios(iterations int32) ([]*Scenario, error) {
	names, err := scenarioFiles.ReadDir("scenarios")
	if err != nil {
		return nil, err
	}
	var sims, quick, normal []*Scenario
	var secondary *proto.RaidSimRequest
	for _, entry := range names {
		file := entry.Name()
		base := strings.TrimSuffix(file, ".json.gz")
		switch {
		case strings.HasPrefix(base, "sim_"):
			req := &proto.RaidSimRequest{}
			if err := load(file, req); err != nil {
				return nil, err
			}
			req.SimOptions = &proto.SimOptions{Iterations: iterations, RandomSeed: randomSeed}
			name := strings.TrimPrefix(base, "sim_")
			if name == secondarySpec {
				secondary = req
			}
			sims = append(sims, simScenario(name, req))
		case strings.HasPrefix(base, "opt_"):
			req := &proto.OptimizeGearRequest{}
			if err := load(file, req); err != nil {
				return nil, err
			}
			name := strings.TrimPrefix(base, "opt_")
			quick = append(quick, optimizerScenario(name, req, proto.OptimizerEffort_OptimizerEffortQuick))
			normal = append(normal, optimizerScenario(name, req, proto.OptimizerEffort_OptimizerEffortNormal))
		}
	}
	if secondary == nil {
		return nil, fmt.Errorf("no sim_%s request for stat weights and the bulk sim", secondarySpec)
	}
	out := sims
	out = append(out, statWeightsScenario(secondarySpec, secondary), bulkScenario(secondarySpec, secondary))
	out = append(out, quick...)
	return append(out, normal...), nil
}

func load(file string, m goproto.Message) error {
	f, err := scenarioFiles.Open(path.Join("scenarios", file))
	if err != nil {
		return err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return fmt.Errorf("%s: %w", file, err)
	}
	data, err := io.ReadAll(gz)
	if err != nil {
		return fmt.Errorf("%s: %w", file, err)
	}
	if err := protojson.Unmarshal(data, m); err != nil {
		return fmt.Errorf("%s: %w", file, err)
	}
	return nil
}

func warmupOf(req *proto.RaidSimRequest) *proto.RaidSimRequest {
	req = goproto.Clone(req).(*proto.RaidSimRequest)
	req.SimOptions = &proto.SimOptions{Iterations: 100, RandomSeed: randomSeed}
	return req
}

// simScenario is the Simulate button: /raidSimAsync.
func simScenario(name string, req *proto.RaidSimRequest) *Scenario {
	return &Scenario{
		Name:   "sim/" + name,
		Kind:   KindSim,
		warmup: warmupOf(req),
		run: func(ctx context.Context, _ int) (*Sample, error) {
			return nil, simRaid(goproto.Clone(req).(*proto.RaidSimRequest))
		},
	}
}

func simRaid(req *proto.RaidSimRequest) error {
	progress := make(chan *proto.ProgressMetrics, 100)
	core.RunRaidSimAsync(req, progress)
	for p := range progress {
		if r := p.FinalRaidResult; r != nil {
			if r.ErrorResult != "" {
				return errors.New(r.ErrorResult)
			}
			return nil
		}
	}
	return errors.New("the sim ended without a result")
}

// statWeightsScenario is the stat weights button on the rogue UI's EP stats: /statWeightsAsync.
func statWeightsScenario(name string, base *proto.RaidSimRequest) *Scenario {
	party := base.Raid.Parties[0]
	req := &proto.StatWeightsRequest{
		Player:     party.Players[0],
		RaidBuffs:  base.Raid.Buffs,
		PartyBuffs: party.Buffs,
		Debuffs:    base.Raid.Debuffs,
		Encounter:  base.Encounter,
		SimOptions: base.SimOptions,
		StatsToWeigh: []proto.Stat{
			proto.Stat_StatAgility, proto.Stat_StatStrength, proto.Stat_StatAttackPower, proto.Stat_StatMeleeHit,
			proto.Stat_StatMeleeCrit, proto.Stat_StatSpellHit, proto.Stat_StatSpellCrit, proto.Stat_StatMeleeHaste,
			proto.Stat_StatArmorPenetration, proto.Stat_StatExpertise,
		},
		PseudoStatsToWeigh: []proto.PseudoStat{proto.PseudoStat_PseudoStatMainHandDps, proto.PseudoStat_PseudoStatOffHandDps},
		EpReferenceStat:    proto.Stat_StatAttackPower,
	}
	return &Scenario{
		Name:   "statweights/" + name,
		Kind:   KindStatWeights,
		warmup: warmupOf(base),
		run: func(ctx context.Context, _ int) (*Sample, error) {
			progress := make(chan *proto.ProgressMetrics, 100)
			core.StatWeightsAsync(goproto.Clone(req).(*proto.StatWeightsRequest), progress)
			for p := range progress {
				if p.FinalWeightResult != nil {
					return nil, nil
				}
			}
			return nil, errors.New("stat weights ended without a result")
		},
	}
}

// bulkScenario is the bulk tab at its defaults (combinations, fast mode, auto enchant) on six of the
// P2 combat set's items: /bulkSimAsync.
func bulkScenario(name string, base *proto.RaidSimRequest) *Scenario {
	settings := goproto.Clone(base).(*proto.RaidSimRequest)
	// the bulk sim only counts named players, and the UI always names them
	settings.Raid.Parties[0].Players[0].Name = name
	req := &proto.BulkSimRequest{
		BaseSettings: settings,
		BulkSettings: &proto.BulkSettings{
			Items: []*proto.ItemSpec{
				{Id: 45517, Gems: []int32{39999}},
				{Id: 45461, Gems: []int32{40053}},
				{Id: 45608, Gems: []int32{39999}},
				{Id: 46048, Gems: []int32{39999}},
				{Id: 45609},
				{Id: 45931},
			},
			Combinations:       true,
			FastMode:           true,
			AutoEnchant:        true,
			IterationsPerCombo: base.SimOptions.Iterations,
		},
	}
	return &Scenario{
		Name:   "bulk/" + name,
		Kind:   KindBulk,
		warmup: warmupOf(base),
		run: func(ctx context.Context, _ int) (*Sample, error) {
			progress := make(chan *proto.ProgressMetrics, 100)
			core.RunBulkSimAsync(ctx, goproto.Clone(req).(*proto.BulkSimRequest), progress)
			for p := range progress {
				if r := p.FinalBulkResult; r != nil {
					if r.ErrorResult != "" {
						return nil, errors.New(r.ErrorResult)
					}
					return nil, nil
				}
			}
			return nil, errors.New("the bulk sim ended without a result")
		},
	}
}

// optimizerScenario is the optimizer on one of its slow suite's requests. The web server runs it
// through optimizer.RunAsync, which forwards Optimize's progress.
func optimizerScenario(name string, req *proto.OptimizeGearRequest, effort proto.OptimizerEffort) *Scenario {
	req = goproto.Clone(req).(*proto.OptimizeGearRequest)
	if req.Settings == nil {
		req.Settings = &proto.OptimizerSettings{}
	}
	req.Settings.Effort = effort
	return &Scenario{
		Name:   "optimizer/" + strings.ToLower(strings.TrimPrefix(effort.String(), "OptimizerEffort")) + "/" + name,
		Kind:   KindOptimizer,
		warmup: warmupOf(req.Base),
		run: func(ctx context.Context, workers int) (*Sample, error) {
			return optimize(ctx, goproto.Clone(req).(*proto.OptimizeGearRequest), workers)
		},
	}
}

// stageMark is where a stage began.
type stageMark struct {
	name string
	at   time.Time
	busy time.Duration
	cpu  time.Duration
	sims int64
}

func optimize(ctx context.Context, req *proto.OptimizeGearRequest, workers int) (*Sample, error) {
	req.Settings.Workers = int32(workers)
	start := time.Now()
	startBusy := optimizer.SimBusyTime()
	startCPU, err := cpuTime()
	if err != nil {
		return nil, err
	}
	// it only fails where getrusage doesn't exist, and the first read worked
	cpuNow := func() time.Duration { d, _ := cpuTime(); return d }
	// every run starts in Setup, which may end before the first progress call
	cur := stageMark{name: "Setup", at: start, busy: startBusy, cpu: startCPU}
	var stages []Stage
	end := func(at time.Time, busy, cpu time.Duration, sims int64) {
		wall := at.Sub(cur.at)
		stages = append(stages, Stage{
			Name:  cur.name,
			Start: cur.at.Sub(start).Seconds(),
			Wall:  wall.Seconds(),
			Sims:  sims - cur.sims,
			Busy:  busyFraction(busy-cur.busy, wall, workers),
			Util:  busyFraction(cpu-cur.cpu, wall, runtime.GOMAXPROCS(0)),
		})
	}
	// Optimize makes these calls one at a time, and one right as each stage starts
	result := optimizer.Optimize(ctx, req, func(p *proto.OptimizerProgress) {
		if p.Stage == cur.name {
			return
		}
		now, busy, cpu := time.Now(), optimizer.SimBusyTime(), cpuNow()
		end(now, busy, cpu, int64(p.CompletedSims))
		cur = stageMark{name: p.Stage, at: now, busy: busy, cpu: cpu, sims: int64(p.CompletedSims)}
	})
	now, busy, cpu := time.Now(), optimizer.SimBusyTime(), cpuNow()
	if result.ErrorResult != "" {
		return nil, errors.New(result.ErrorResult)
	}
	if result.Cancelled {
		return nil, context.Cause(ctx)
	}
	end(now, busy, cpu, int64(result.TotalSims))
	wall := now.Sub(start)
	return &Sample{
		Workers: workers,
		Sims:    int64(result.TotalSims),
		Busy:    busyFraction(busy-startBusy, wall, workers),
		Stages:  stages,
	}, nil
}

// busyFraction is how much of the workers' time went to simming, or of the threads' to running.
func busyFraction(busy, wall time.Duration, workers int) float64 {
	if wall <= 0 || workers <= 0 {
		return 0
	}
	return float64(busy) / (float64(wall) * float64(workers))
}
