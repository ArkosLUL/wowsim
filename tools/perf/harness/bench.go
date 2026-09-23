package harness

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"runtime/debug"
	"runtime/pprof"
	"slices"
	"strings"
	"time"
)

// The scenarios NormalProcs and NormalReps apply to, the longest ones.
const normalPrefix = "optimizer/normal/"

// Config is what to run.
type Config struct {
	Iterations int32
	// Scenarios whose name matches; nil runs them all.
	Match *regexp.Regexp
	// The GOMAXPROCS sweep, and the optimizer at Normal's.
	Procs, NormalProcs []int
	// Timed runs at a scenario's widest point, where the comparison needs the spread most (the
	// optimizer at Normal's), and at every other point.
	Reps, NormalReps, SweepReps int
	// The optimizer's workers; 0 is GOMAXPROCS at each point.
	Workers int
	// Where the report, its text summary and the profiles go.
	Dir string
	// CPU profile the last run at each scenario's widest point.
	Profiles bool
	Note     string
	// The sim's commit; empty takes the one the build recorded, if any.
	Commit string
	Args   []string
	// Progress, one line per run.
	Log io.Writer
}

// Selected is the scenarios cfg picks.
func Selected(cfg Config) ([]*Scenario, error) {
	all, err := Scenarios(cfg.Iterations)
	if err != nil {
		return nil, err
	}
	var out []*Scenario
	for _, s := range all {
		if cfg.Match == nil || cfg.Match.MatchString(s.Name) {
			out = append(out, s)
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no scenario matches %q", cfg.Match)
	}
	return out, nil
}

// Run times the selected scenarios and writes report.json and report.txt to cfg.Dir, the JSON after
// every point.
func Run(ctx context.Context, cfg Config) (*Report, error) {
	scenarios, err := Selected(cfg)
	if err != nil {
		return nil, err
	}
	if len(cfg.Procs) == 0 || len(cfg.NormalProcs) == 0 || min(cfg.Reps, cfg.NormalReps, cfg.SweepReps) < 1 {
		return nil, errors.New("every scenario needs at least one GOMAXPROCS point, and every point one rep")
	}
	if err := os.MkdirAll(cfg.Dir, 0777); err != nil {
		return nil, err
	}
	rep := &Report{
		Started:    time.Now(),
		Args:       cfg.Args,
		Commit:     cmp.Or(cfg.Commit, vcsRevision()),
		GoVersion:  runtime.Version(),
		NumCPU:     runtime.NumCPU(),
		Iterations: cfg.Iterations,
		Note:       cfg.Note,
		LoadStart:  loadAverage(),
	}
	jsonFile := filepath.Join(cfg.Dir, "report.json")
	defer runtime.GOMAXPROCS(runtime.GOMAXPROCS(0))

	for _, s := range scenarios {
		sweep, widestReps := cfg.Procs, cfg.Reps
		if strings.HasPrefix(s.Name, normalPrefix) {
			sweep, widestReps = cfg.NormalProcs, cfg.NormalReps
		}
		widest := slices.Max(sweep)
		runtime.GOMAXPROCS(widest)
		if err := simRaid(s.warmup); err != nil {
			return rep, fmt.Errorf("%s: warming up: %w", s.Name, err)
		}
		for _, procs := range sweep {
			runtime.GOMAXPROCS(procs)
			workers := cfg.Workers
			if workers <= 0 {
				workers = procs
			}
			p := Point{Scenario: s.Name, Kind: s.Kind, Procs: procs}
			if s.Kind == KindOptimizer {
				p.Workers = workers
			}
			reps := cfg.SweepReps
			if procs == widest {
				reps = widestReps
			}
			for i := range reps {
				if err := ctx.Err(); err != nil {
					return rep, err
				}
				var profileFile string
				if cfg.Profiles && procs == widest && i == reps-1 {
					profileFile = strings.ReplaceAll(s.Name, "/", "_") + ".cpu.pprof"
				}
				sample, err := profiled(filepath.Join(cfg.Dir, profileFile), profileFile != "", func() (Sample, error) {
					return measure(func() (*Sample, error) { return s.run(ctx, workers) })
				})
				if err != nil {
					return rep, fmt.Errorf("%s at %d procs: %w", s.Name, procs, err)
				}
				p.Samples = append(p.Samples, sample)
				p.Profile = profileFile
				fmt.Fprintf(cfg.Log, "%s procs=%d rep %d/%d: %s, util %s\n", s.Name, procs, i+1, reps, secs(sample.Wall), percent(sample.Util))
			}
			rep.Points = append(rep.Points, p)
			if err := rep.Save(jsonFile); err != nil {
				return rep, err
			}
		}
	}

	rep.Finished = time.Now()
	rep.LoadEnd = loadAverage()
	if err := rep.Save(jsonFile); err != nil {
		return rep, err
	}
	return rep, writeText(filepath.Join(cfg.Dir, "report.txt"), rep.Write)
}

// profiled runs fn under the CPU profiler when on is set. Starting and stopping it stays out of the
// timing; its sampling doesn't, which is why only one run per scenario carries it.
func profiled(file string, on bool, fn func() (Sample, error)) (Sample, error) {
	if !on {
		return fn()
	}
	f, err := os.Create(file)
	if err != nil {
		return Sample{}, err
	}
	defer f.Close()
	if err := pprof.StartCPUProfile(f); err != nil {
		return Sample{}, err
	}
	sample, err := fn()
	pprof.StopCPUProfile()
	return sample, err
}

func vcsRevision() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return ""
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
	if revision != "" && modified {
		revision += "-dirty"
	}
	return revision
}

// loadAverage is the machine's 1, 5 and 15 minute load averages; in Docker Desktop, its VM's.
func loadAverage() string {
	data, err := os.ReadFile("/proc/loadavg")
	if err != nil {
		return ""
	}
	fields := strings.Fields(string(data))
	if len(fields) < 3 {
		return ""
	}
	return strings.Join(fields[:3], " ")
}
