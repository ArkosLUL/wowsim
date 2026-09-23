package harness

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/wowsims/wotlk/sim"
	"github.com/wowsims/wotlk/sim/core"
)

func init() {
	sim.RegisterAll()
}

func TestScenarios(t *testing.T) {
	scenarios, err := Scenarios(3000)
	if err != nil {
		t.Fatal(err)
	}
	kinds := map[Kind]int{}
	seen := map[string]bool{}
	for _, s := range scenarios {
		if seen[s.Name] {
			t.Errorf("two scenarios named %s", s.Name)
		}
		seen[s.Name] = true
		kinds[s.Kind]++
	}
	// 12 specs and the raid; the slow suite's six requests at Quick and Normal
	want := map[Kind]int{KindSim: 13, KindStatWeights: 1, KindBulk: 1, KindOptimizer: 12}
	for kind, n := range want {
		if kinds[kind] != n {
			t.Errorf("%d %s scenarios, want %d", kinds[kind], kind, n)
		}
	}
}

func report(points ...Point) *Report {
	return &Report{Iterations: 3000, NumCPU: 16, Points: points}
}

func walls(scenario string, procs int, walls ...float64) Point {
	p := Point{Scenario: scenario, Kind: KindSim, Procs: procs}
	for _, w := range walls {
		p.Samples = append(p.Samples, Sample{Wall: w, CPU: w, Alloc: 1e9, PeakHeap: 1e8})
	}
	return p
}

func wallVerdicts(c *Comparison) map[string]int {
	out := map[string]int{}
	for _, m := range c.Moves {
		if m.Metric == "wall" {
			out[m.Scenario] = m.Verdict
		}
	}
	return out
}

func TestCompare(t *testing.T) {
	base := report(
		walls("same", 1, 10, 10.2, 9.9),
		walls("slower", 1, 10, 10.2, 9.9),
		walls("faster", 1, 10, 10.2, 9.9),
		walls("noisy", 1, 10, 13, 9),
		walls("under the floor", 1, 10),
		walls("only in the baseline", 1, 10),
	)
	cur := report(
		walls("same", 1, 10.1, 9.95, 10.15),
		walls("slower", 1, 12, 12.1, 11.9),
		walls("faster", 1, 8, 8.1, 7.9),
		walls("noisy", 1, 11.5, 11, 11.2),
		walls("under the floor", 1, 10.4),
		walls("only in this run", 1, 10),
	)
	c := Compare(base, cur, DefaultFloor)
	got := wallVerdicts(c)
	want := map[string]int{"same": 0, "slower": 1, "faster": -1, "noisy": 0, "under the floor": 0}
	for name, verdict := range want {
		if got[name] != verdict {
			t.Errorf("%s: verdict %d, want %d", name, got[name], verdict)
		}
	}
	// cpu moved with wall; alloc and peak heap stayed put
	if c.Worse() != 2 {
		t.Errorf("%d worse, want wall and cpu of one point", c.Worse())
	}
	if len(c.OnlyBase) != 1 || len(c.OnlyNew) != 1 {
		t.Errorf("only in the baseline %v, only in this run %v, want one each", c.OnlyBase, c.OnlyNew)
	}
	var out strings.Builder
	if err := c.Write(&out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "wall +20.0% slower, cpu +20.0% more") {
		t.Errorf("the comparison doesn't name the slower point's moves:\n%s", out.String())
	}

	if c := Compare(cur, cur, DefaultFloor); c.Worse() != 0 || len(c.Warnings) != 0 {
		t.Errorf("a report against itself has %d worse and warns %q", c.Worse(), c.Warnings)
	}
	fewer := report(slices.Clone(cur.Points)...)
	fewer.Points[0].Workers = 15
	if c := Compare(cur, fewer, DefaultFloor); len(c.Warnings) != 1 {
		t.Errorf("a point that ran other workers warns %q, want one warning", c.Warnings)
	}

	// peak heap only counts past its own, higher floor
	heap := func(peak float64) *Report {
		p := walls("heap", 16, 10, 10, 10)
		for i := range p.Samples {
			p.Samples[i].PeakHeap = uint64(peak * (1 + 0.01*float64(i)))
		}
		return report(p)
	}
	if c := Compare(heap(1e8), heap(1.15e8), DefaultFloor); c.Worse() != 0 {
		t.Errorf("a 15%% bigger peak heap counts as %d worse, want none", c.Worse())
	}
	if c := Compare(heap(1e8), heap(1.4e8), DefaultFloor); c.Worse() != 1 {
		t.Errorf("a 40%% bigger peak heap counts as %d worse, want 1", c.Worse())
	}
}

func TestRun(t *testing.T) {
	if !core.WITH_DB {
		t.Skip("needs the with_db item data")
	}
	dir := t.TempDir()
	rep, err := Run(context.Background(), Config{
		Iterations:  50,
		Match:       regexp.MustCompile(`^sim/rogue$`),
		Procs:       []int{1, 2},
		NormalProcs: []int{2},
		Reps:        2,
		NormalReps:  1,
		SweepReps:   1,
		Dir:         dir,
		Profiles:    true,
		Log:         io.Discard,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Points) != 2 {
		t.Fatalf("%d points, want one per GOMAXPROCS", len(rep.Points))
	}
	for i, p := range rep.Points {
		// Reps at the widest point, SweepReps at the other
		if want := i + 1; len(p.Samples) != want {
			t.Errorf("%d procs: %d samples, want %d", p.Procs, len(p.Samples), want)
		}
		for _, s := range p.Samples {
			if s.Wall <= 0 || s.CPU <= 0 || s.Alloc == 0 || s.PeakHeap == 0 {
				t.Errorf("%d procs: sample %+v is missing a measurement", p.Procs, s)
			}
		}
	}
	if profile := rep.Points[1].Profile; profile == "" {
		t.Error("no profile at the widest point")
	} else if _, err := os.Stat(filepath.Join(dir, profile)); err != nil {
		t.Error(err)
	}

	saved, err := ReadReport(filepath.Join(dir, "report.json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(saved.Points) != 2 || saved.Finished.IsZero() {
		t.Errorf("the saved report has %d points and finished at %v", len(saved.Points), saved.Finished)
	}
	text, err := os.ReadFile(filepath.Join(dir, "report.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(text), "sim/rogue") {
		t.Errorf("report.txt doesn't list the scenario:\n%s", text)
	}
}
