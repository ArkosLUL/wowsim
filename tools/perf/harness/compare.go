package harness

import (
	"fmt"
	"io"
	"math"
	"slices"
	"strings"
	"text/tabwriter"
)

// DefaultFloor is the smallest change Compare flags, whatever the reps' spread: a run on an idle
// machine moves its medians by a few percent.
const DefaultFloor = 0.05

type metric struct {
	name string
	get  func(Sample) float64
	// Raises the floor for this metric.
	floor float64
	// How a flagged rise and fall read.
	up, down string
}

var comparedMetrics = []metric{
	{"wall", wallOf, 0, "slower", "faster"},
	{"cpu", cpuOf, 0, "more", "less"},
	{"alloc", allocOf, 0, "more", "less"},
	// GC timing moves an optimizer run's peak by 10% or more from one run of the harness to the next
	{"peak heap", peakHeapOf, 0.25, "bigger", "smaller"},
}

// Move is one metric of a point both reports have.
type Move struct {
	Scenario string
	Procs    int
	Metric   string
	// Medians.
	Base, New float64
	// New over Base, less 1.
	Change float64
	// The wider of the two reports' spreads, max less min over the median.
	Noise float64
	// 1 worse (up), -1 better (down), 0 within the noise.
	Verdict int
}

// Comparison is a report against a baseline.
type Comparison struct {
	Floor float64
	// Per point both reports have, per compared metric, in the new report's order.
	Moves []Move
	// Points only one report has, as scenario@procs.
	OnlyBase, OnlyNew []string
	Warnings          []string
}

// Compare flags a metric that moved past both the floor and the run-to-run noise: its median
// changed by more than either, and the two reports' samples don't overlap.
func Compare(base, cur *Report, floor float64) *Comparison {
	c := &Comparison{Floor: floor}
	if base.Iterations != cur.Iterations {
		c.Warnings = append(c.Warnings, fmt.Sprintf("the baseline simmed %d iterations, this run %d", base.Iterations, cur.Iterations))
	}
	if base.NumCPU != cur.NumCPU {
		c.Warnings = append(c.Warnings, fmt.Sprintf("the baseline ran on %d CPUs, this run on %d", base.NumCPU, cur.NumCPU))
	}
	key := func(p Point) string { return fmt.Sprintf("%s@%d", p.Scenario, p.Procs) }
	basePoints := map[string]Point{}
	for _, p := range base.Points {
		basePoints[key(p)] = p
	}
	seen := map[string]bool{}
	for _, p := range cur.Points {
		k := key(p)
		seen[k] = true
		b, ok := basePoints[k]
		if !ok {
			c.OnlyNew = append(c.OnlyNew, k)
			continue
		}
		if b.Workers != p.Workers {
			c.Warnings = append(c.Warnings, fmt.Sprintf("%s: the baseline ran %d optimizer workers, this run %d", k, b.Workers, p.Workers))
		}
		for _, m := range comparedMetrics {
			bv, nv := values(b.Samples, m.get), values(p.Samples, m.get)
			change, noise, verdict := judge(bv, nv, max(floor, m.floor))
			c.Moves = append(c.Moves, Move{
				Scenario: p.Scenario, Procs: p.Procs, Metric: m.name,
				Base: medianOf(bv), New: medianOf(nv), Change: change, Noise: noise, Verdict: verdict,
			})
		}
	}
	for _, p := range base.Points {
		if !seen[key(p)] {
			c.OnlyBase = append(c.OnlyBase, key(p))
		}
	}
	return c
}

func judge(base, cur []float64, floor float64) (change, noise float64, verdict int) {
	if len(base) == 0 || len(cur) == 0 {
		return 0, 0, 0
	}
	mb, mn := medianOf(base), medianOf(cur)
	if mb <= 0 {
		return 0, 0, 0
	}
	change = mn/mb - 1
	noise = max(spread(base), spread(cur))
	if math.Abs(change) <= max(floor, noise) {
		return change, noise, 0
	}
	switch {
	case slices.Min(cur) > slices.Max(base):
		verdict = 1
	case slices.Max(cur) < slices.Min(base):
		verdict = -1
	}
	return change, noise, verdict
}

func spread(v []float64) float64 {
	m := medianOf(v)
	if m <= 0 {
		return 0
	}
	return (slices.Max(v) - slices.Min(v)) / m
}

// Worse counts the metrics flagged as up: for every compared metric, higher is worse.
func (c *Comparison) Worse() int {
	n := 0
	for _, m := range c.Moves {
		if m.Verdict > 0 {
			n++
		}
	}
	return n
}

// Write prints each point's wall time, and every metric that moved past the noise.
func (c *Comparison) Write(w io.Writer) error {
	for _, warning := range c.Warnings {
		fmt.Fprintf(w, "warning: %s\n", warning)
	}
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "scenario\tprocs\twall base\twall new\tchange\tnoise\tpast the noise\t")
	better := 0
	for i := 0; i < len(c.Moves); i += len(comparedMetrics) {
		point := c.Moves[i : i+len(comparedMetrics)]
		wall := point[0]
		var moved []string
		for j, m := range point {
			switch m.Verdict {
			case 1:
				moved = append(moved, fmt.Sprintf("%s %+.1f%% %s", m.Metric, 100*m.Change, comparedMetrics[j].up))
			case -1:
				moved = append(moved, fmt.Sprintf("%s %+.1f%% %s", m.Metric, 100*m.Change, comparedMetrics[j].down))
				better++
			}
		}
		fmt.Fprintf(tw, "%s\t%d\t%s\t%s\t%+.1f%%\t%.1f%%\t%s\t\n", wall.Scenario, wall.Procs, secs(wall.Base), secs(wall.New),
			100*wall.Change, 100*wall.Noise, strings.Join(moved, ", "))
	}
	if err := tw.Flush(); err != nil {
		return err
	}
	if len(c.OnlyBase) > 0 {
		fmt.Fprintf(w, "only in the baseline: %s\n", strings.Join(c.OnlyBase, " "))
	}
	if len(c.OnlyNew) > 0 {
		fmt.Fprintf(w, "not in the baseline: %s\n", strings.Join(c.OnlyNew, " "))
	}
	fmt.Fprintf(w, "%d worse and %d better past the noise (and the %.0f%% floor)\n", c.Worse(), better, 100*c.Floor)
	return nil
}
