package harness

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"text/tabwriter"
	"time"
)

// Report is one harness run.
type Report struct {
	Started  time.Time `json:"started"`
	Finished time.Time `json:"finished"`
	Args     []string  `json:"args"`
	// The sim's commit, from -commit or else the build.
	Commit     string `json:"commit,omitempty"`
	GoVersion  string `json:"goVersion"`
	NumCPU     int    `json:"numCPU"`
	Iterations int32  `json:"iterations"`
	// Whatever -note said, e.g. what else ran.
	Note string `json:"note,omitempty"`
	// /proc/loadavg when the run started and ended.
	LoadStart string  `json:"loadStart,omitempty"`
	LoadEnd   string  `json:"loadEnd,omitempty"`
	Points    []Point `json:"points"`
}

// Point is a scenario's reps at one GOMAXPROCS.
type Point struct {
	Scenario string `json:"scenario"`
	Kind     Kind   `json:"kind"`
	Procs    int    `json:"gomaxprocs"`
	// The optimizer's Settings.Workers.
	Workers int      `json:"workers,omitempty"`
	Samples []Sample `json:"samples"`
	// The CPU profile of the point's last run, relative to the report.
	Profile string `json:"cpuProfile,omitempty"`
}

// Sample is one timed run.
type Sample struct {
	Wall float64 `json:"wallSeconds"`
	// User and system, over all threads.
	CPU float64 `json:"cpuSeconds"`
	// CPU over wall and GOMAXPROCS.
	Util float64 `json:"utilization"`
	// The GC's share of the CPU the runtime counted, idle-priority mark workers left out.
	GCShare float64 `json:"gcShare"`
	Alloc   uint64  `json:"allocBytes"`
	// Live and unswept heap objects, sampled every 10 ms.
	PeakHeap uint64 `json:"peakHeapBytes"`

	// The optimizer's only. Busy is the evaluator's workers' time spent simming, over wall x workers.
	Workers int     `json:"workers,omitempty"`
	Sims    int64   `json:"sims,omitempty"`
	Busy    float64 `json:"evaluatorBusy,omitempty"`
	Stages  []Stage `json:"stages,omitempty"`
}

// Stage is one optimizer stage in one run.
type Stage struct {
	Name  string  `json:"name"`
	Start float64 `json:"startSeconds"`
	Wall  float64 `json:"wallSeconds"`
	Sims  int64   `json:"sims"`
	Busy  float64 `json:"evaluatorBusy"`
	// CPU over wall and GOMAXPROCS, like the sample's.
	Util float64 `json:"utilization,omitempty"`
}

func ReadReport(file string) (*Report, error) {
	data, err := os.ReadFile(file)
	if err != nil {
		return nil, err
	}
	rep := &Report{}
	if err := json.Unmarshal(data, rep); err != nil {
		return nil, fmt.Errorf("%s: %w", file, err)
	}
	return rep, nil
}

// Save writes the report whole, so a run that stops early still leaves the points it finished.
func (rep *Report) Save(file string) error {
	data, err := json.MarshalIndent(rep, "", "  ")
	if err != nil {
		return err
	}
	tmp := file + ".tmp"
	if err := os.WriteFile(tmp, append(data, '\n'), 0666); err != nil {
		return err
	}
	return os.Rename(tmp, file)
}

func values[T any](items []T, get func(T) float64) []float64 {
	v := make([]float64, len(items))
	for i, item := range items {
		v[i] = get(item)
	}
	return v
}

func medianOf(v []float64) float64 {
	if len(v) == 0 {
		return 0
	}
	v = slices.Clone(v)
	sort.Float64s(v)
	if n := len(v); n%2 == 0 {
		return (v[n/2-1] + v[n/2]) / 2
	}
	return v[len(v)/2]
}

func median(samples []Sample, get func(Sample) float64) float64 {
	return medianOf(values(samples, get))
}

func wallOf(s Sample) float64     { return s.Wall }
func cpuOf(s Sample) float64      { return s.CPU }
func utilOf(s Sample) float64     { return s.Util }
func gcOf(s Sample) float64       { return s.GCShare }
func allocOf(s Sample) float64    { return float64(s.Alloc) }
func peakHeapOf(s Sample) float64 { return float64(s.PeakHeap) }
func simsOf(s Sample) float64     { return float64(s.Sims) }
func busyOf(s Sample) float64     { return s.Busy }

// Write prints each point's medians, then each optimizer point's stages.
func (rep *Report) Write(w io.Writer) error {
	fmt.Fprintf(w, "%s, %d iterations, GOMAXPROCS sweep on %d CPUs, %s\n", rep.Started.Format(time.DateTime), rep.Iterations, rep.NumCPU, rep.GoVersion)
	if rep.Commit != "" {
		fmt.Fprintf(w, "commit: %s\n", rep.Commit)
	}
	if rep.Note != "" {
		fmt.Fprintf(w, "note: %s\n", rep.Note)
	}
	if rep.LoadStart != "" {
		fmt.Fprintf(w, "load average at the start: %s; at the end: %s\n", rep.LoadStart, rep.LoadEnd)
	}
	fmt.Fprintln(w, "medians over the reps; speedup against the scenario's narrowest point")
	fmt.Fprintln(w)

	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "scenario\tprocs\tworkers\treps\twall\tspeedup\tcpu\tutil\tgc\talloc\tpeak heap\tsims\tbusy\t")
	narrowest := map[string]Point{}
	for _, p := range rep.Points {
		if n, ok := narrowest[p.Scenario]; !ok || p.Procs < n.Procs {
			narrowest[p.Scenario] = p
		}
	}
	for _, p := range rep.Points {
		wall := median(p.Samples, wallOf)
		workers, sims, busy := "", "", ""
		if p.Kind == KindOptimizer {
			workers = fmt.Sprint(p.Workers)
			sims = fmt.Sprintf("%.0f", median(p.Samples, simsOf))
			busy = percent(median(p.Samples, busyOf))
		}
		fmt.Fprintf(tw, "%s\t%d\t%s\t%d\t%s\t%.2f\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t\n", p.Scenario, p.Procs, workers, len(p.Samples),
			secs(wall), median(narrowest[p.Scenario].Samples, wallOf)/wall, secs(median(p.Samples, cpuOf)), percent(median(p.Samples, utilOf)),
			percent(median(p.Samples, gcOf)), bytes(median(p.Samples, allocOf)), bytes(median(p.Samples, peakHeapOf)), sims, busy)
	}
	if err := tw.Flush(); err != nil {
		return err
	}

	for _, p := range rep.Points {
		stages := stageMedians(p.Samples)
		if len(stages) == 0 {
			continue
		}
		fmt.Fprintf(w, "\n%s at %d procs, %d workers\n", p.Scenario, p.Procs, p.Workers)
		tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
		fmt.Fprintln(tw, "  stage\tstart\twall\tsims\tbusy\tutil\t")
		for _, s := range stages {
			fmt.Fprintf(tw, "  %s\t%s\t%s\t%d\t%s\t%s\t\n", s.Name, secs(s.Start), secs(s.Wall), s.Sims, percent(s.Busy), percent(s.Util))
		}
		if err := tw.Flush(); err != nil {
			return err
		}
	}
	return nil
}

// stageMedians is each stage's medians over the samples that ran it, in the order they ran.
func stageMedians(samples []Sample) []Stage {
	var names []string
	byName := map[string][]Stage{}
	for _, s := range samples {
		for _, st := range s.Stages {
			if !slices.Contains(names, st.Name) {
				names = append(names, st.Name)
			}
			byName[st.Name] = append(byName[st.Name], st)
		}
	}
	out := make([]Stage, 0, len(names))
	for _, name := range names {
		runs := byName[name]
		out = append(out, Stage{
			Name:  name,
			Start: medianOf(values(runs, func(s Stage) float64 { return s.Start })),
			Wall:  medianOf(values(runs, func(s Stage) float64 { return s.Wall })),
			Sims:  int64(medianOf(values(runs, func(s Stage) float64 { return float64(s.Sims) }))),
			Busy:  medianOf(values(runs, func(s Stage) float64 { return s.Busy })),
			Util:  medianOf(values(runs, func(s Stage) float64 { return s.Util })),
		})
	}
	return out
}

func secs(s float64) string {
	return fmt.Sprintf("%.2fs", s)
}

func percent(f float64) string {
	return fmt.Sprintf("%.0f%%", 100*f)
}

func bytes(b float64) string {
	switch {
	case b >= 1e9:
		return fmt.Sprintf("%.2f GB", b/1e9)
	case b >= 1e6:
		return fmt.Sprintf("%.0f MB", b/1e6)
	}
	return fmt.Sprintf("%.0f kB", b/1e3)
}

func writeText(file string, write func(io.Writer) error) error {
	if err := os.MkdirAll(filepath.Dir(file), 0777); err != nil {
		return err
	}
	f, err := os.Create(file)
	if err != nil {
		return err
	}
	if err := write(f); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}
