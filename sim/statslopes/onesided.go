package statslopes

import (
	"errors"
	"fmt"
	"runtime"
	"runtime/debug"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
	googleProto "google.golang.org/protobuf/proto"
)

// OneSidedSteps are the centered steps' deltas with no shift: the live run measures at the gear as
// it is, caps and all.
func OneSidedSteps(r Role) []Step {
	var out []Step
	for _, st := range r.Stats {
		out = append(out, Step{Name: st.String(), Index: int(st), Delta: stepDelta(st)})
	}
	for _, ps := range r.PseudoStats {
		out = append(out, Step{Name: ps.String(), Index: int(ps), Pseudo: true, Delta: pseudoDelta})
	}
	return out
}

// lowDelta stops the low step at 0 rating: below that the sim clamps some stats (armor penetration
// at 0%), which would read as losing nothing.
func lowDelta(s Step, final []float64) float64 {
	if s.Pseudo {
		return s.Delta
	}
	return max(0, min(s.Delta, final[s.Index]))
}

// MeasureAsync answers the StatSlopesAsync RPC: a progress report per finished sim, then
// FinalSlopesResult.
func MeasureAsync(request *proto.StatSlopesRequest, progress chan *proto.ProgressMetrics) {
	go func() {
		result := Measure(request, func(done, total int32) {
			iterations := request.GetSimOptions().GetIterations()
			progress <- &proto.ProgressMetrics{CompletedSims: done, TotalSims: total,
				CompletedIterations: done * iterations, TotalIterations: total * iterations}
		})
		progress <- &proto.ProgressMetrics{FinalSlopesResult: result}
	}()
}

// which sim: slope -1 is the gear as it is; side +1 the high end, -1 the low one
type simKey struct {
	slope, side int
}

type simJob struct {
	simKey
	raid *proto.Raid
}

type simDone struct {
	simKey
	metrics Metrics
	err     error
}

// Measure is the one-sided slopes of the raid's first player: gain = (sim(+Delta) − sim(0)) / Delta,
// loss = (sim(0) − sim(−LowDelta)) / LowDelta, every sim at the request's one seed so they all see
// the same rolls. report is called after each sim.
func Measure(request *proto.StatSlopesRequest, report func(done, total int32)) (result *proto.StatSlopesResult) {
	defer func() {
		if e := recover(); e != nil {
			result = &proto.StatSlopesResult{ErrorResult: fmt.Sprintf("%v\nStack Trace:\n%s", e, debug.Stack())}
		}
	}()
	raid, encounter := request.GetRaid(), request.GetEncounter()
	if len(raid.GetParties()) == 0 || len(raid.Parties[0].GetPlayers()) == 0 {
		return &proto.StatSlopesResult{ErrorResult: "no player in the raid"}
	}
	iterations := request.GetSimOptions().GetIterations()
	if iterations < 1 {
		return &proto.StatSlopesResult{ErrorResult: "no iterations to sim"}
	}
	role, err := RoleOf(core.PlayerProtoToSpec(raid.Parties[0].Players[0]))
	if err != nil {
		return &proto.StatSlopesResult{ErrorResult: err.Error()}
	}

	final := FinalStats(raid, encounter)
	steps := OneSidedSteps(role)
	slopes := make([]*proto.StatSlope, len(steps))
	jobs := []simJob{{simKey{slope: -1}, raid}}
	for i, s := range steps {
		slopes[i] = &proto.StatSlope{Pseudo: s.Pseudo, Delta: s.Delta, LowDelta: lowDelta(s, final)}
		if s.Pseudo {
			slopes[i].PseudoStat = proto.PseudoStat(s.Index)
		} else {
			slopes[i].Stat = proto.Stat(s.Index)
		}
		jobs = append(jobs, simJob{simKey{i, 1}, Shifted(raid, s, s.Delta)})
		if slopes[i].LowDelta > 0 {
			jobs = append(jobs, simJob{simKey{i, -1}, Shifted(raid, s, -slopes[i].LowDelta)})
		}
	}

	runs, err := runAll(jobs, encounter, iterations, request.GetSimOptions().GetRandomSeed(), report)
	if err != nil {
		return &proto.StatSlopesResult{ErrorResult: err.Error()}
	}
	base := runs[simKey{slope: -1}]
	for i, s := range slopes {
		s.Gain = metricsProto(perPoint(base, runs[simKey{i, 1}], s.Delta))
		if s.LowDelta > 0 {
			s.Loss = metricsProto(perPoint(runs[simKey{i, -1}], base, s.LowDelta))
		}
	}
	return &proto.StatSlopesResult{Slopes: slopes, Base: metricsProto(base)}
}

// runAll runs one sim per core, leaving one core free. Each sim is single threaded so a panic in an
// IsTest sim, which core doesn't recover, lands in runOne's recover instead of killing the server.
func runAll(jobs []simJob, encounter *proto.Encounter, iterations int32, seed int64, report func(done, total int32)) (map[simKey]Metrics, error) {
	todo := make(chan simJob, len(jobs))
	for _, j := range jobs {
		todo <- j
	}
	close(todo)
	done := make(chan simDone)
	for range max(1, min(len(jobs), runtime.GOMAXPROCS(0)-1)) {
		go func() {
			for j := range todo {
				done <- runOne(j, encounter, iterations, seed)
			}
		}()
	}

	runs := make(map[simKey]Metrics, len(jobs))
	var errs []error
	for n := range len(jobs) {
		d := <-done
		if d.err != nil {
			errs = append(errs, d.err)
		}
		runs[d.simKey] = d.metrics
		if report != nil {
			report(int32(n+1), int32(len(jobs)))
		}
	}
	return runs, errors.Join(errs...)
}

func runOne(j simJob, encounter *proto.Encounter, iterations int32, seed int64) (d simDone) {
	d.simKey = j.simKey
	defer func() {
		if e := recover(); e != nil {
			d.err = fmt.Errorf("sim panicked: %v\nStack Trace:\n%s", e, debug.Stack())
		}
	}()
	// cloned per sim: NewEncounter writes to the proto, and these sims run in parallel
	r := core.RunRaidSim(simRequest(j.raid, googleProto.Clone(encounter).(*proto.Encounter), iterations, seed))
	if r.ErrorResult != "" {
		d.err = fmt.Errorf("sim: %s", r.ErrorResult)
		return d
	}
	d.metrics = metricsOf(r.RaidMetrics.Parties[0].Players[0])
	return d
}

func metricsProto(m Metrics) *proto.StatSlopeMetrics {
	return &proto.StatSlopeMetrics{Dps: m.Dps, Tps: m.Tps, Tmi: m.Tmi, Hps: m.Hps}
}
