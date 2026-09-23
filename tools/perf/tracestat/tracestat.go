// Package tracestat reads a runtime/trace and works out how many threads the program kept busy, and
// how much of that was the garbage collector, per time window and per trace region.
package tracestat

import (
	"cmp"
	"errors"
	"fmt"
	"io"
	"slices"
	"sort"
	"strings"
	"text/tabwriter"
	"time"

	"golang.org/x/exp/trace"
)

// Span is one stretch of the trace.
type Span struct {
	// A region's type, from trace.StartRegion; empty for windows and the total.
	Name string
	// Since the trace's first event.
	Start, End time.Duration
	// Time goroutines spent running, summed over goroutines. Idle-priority GC workers don't count:
	// they only run on Ps nothing else wanted.
	Busy time.Duration
	// The part of Busy spent in GC mark workers and mark assists.
	GC time.Duration
	// GOMAXPROCS summed over the span, the most Busy could be. 0 when the trace never reports it.
	Capacity time.Duration
}

func (s Span) wall() time.Duration {
	return s.End - s.Start
}

// Threads is how many goroutines ran at once, on average.
func (s Span) Threads() float64 {
	if s.wall() <= 0 {
		return 0
	}
	return float64(s.Busy) / float64(s.wall())
}

// Procs is GOMAXPROCS, averaged over the span.
func (s Span) Procs() float64 {
	if s.wall() <= 0 {
		return 0
	}
	return float64(s.Capacity) / float64(s.wall())
}

// Utilization is Busy over Capacity.
func (s Span) Utilization() float64 {
	if s.Capacity <= 0 {
		return 0
	}
	return float64(s.Busy) / float64(s.Capacity)
}

// GCShare is GC over Busy.
func (s Span) GCShare() float64 {
	if s.Busy <= 0 {
		return 0
	}
	return float64(s.GC) / float64(s.Busy)
}

// Report is a whole trace's spans.
type Report struct {
	Total Span
	// Consecutive windows from the trace's start; the last one ends with the trace.
	Windows []Span
	// Every region, in the order they began. One whose begin isn't in the trace starts at 0, and one
	// still open at the end ends there.
	Regions []Span
}

type class int

const (
	classApp class = iota
	classGC
	classIdleGC
	numClasses
)

type goroutine struct {
	running bool
	since   trace.Time
	class   class
	// In a GC mark assist, which counts as GC while the goroutine runs.
	assist  bool
	regions []openRegion
}

type openRegion struct {
	name  string
	start trace.Time
}

type analyzer struct {
	start, last trace.Time
	started     bool
	goroutines  map[trace.GoID]*goroutine
	// Changes in how many goroutines of each class run, and in GOMAXPROCS.
	running [numClasses][]change
	procs   []change
	regions []region
}

type change struct {
	at    trace.Time
	delta int64
}

type region struct {
	name       string
	start, end trace.Time
}

// Analyze reads a trace and splits it into windows of the given length; 0 makes none.
func Analyze(r io.Reader, window time.Duration) (*Report, error) {
	reader, err := trace.NewReader(r)
	if err != nil {
		return nil, err
	}
	a := &analyzer{goroutines: map[trace.GoID]*goroutine{}}
	for {
		ev, err := reader.ReadEvent()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, err
		}
		a.event(ev)
	}
	if !a.started {
		return nil, errors.New("the trace has no events")
	}
	a.finish()
	return a.report(window), nil
}

func (a *analyzer) event(ev trace.Event) {
	t := ev.Time()
	if !a.started {
		a.start, a.last, a.started = t, t, true
	}
	// the reader's order is the one to trust; keep time from running backwards within it
	t = max(t, a.last)
	a.last = t

	switch ev.Kind() {
	case trace.EventMetric:
		if m := ev.Metric(); m.Name == "/sched/gomaxprocs:threads" {
			a.setProcs(t, int64(m.Value.Uint64()))
		}
	case trace.EventStateTransition:
		st := ev.StateTransition()
		if st.Resource.Kind != trace.ResourceGoroutine {
			return
		}
		_, to := st.Goroutine()
		id := st.Resource.Goroutine()
		g := a.goroutine(id)
		switch {
		case to == trace.GoRunning && !g.running:
			g.running, g.since, g.class = true, t, classApp
			if g.assist {
				g.class = classGC
			}
		case to != trace.GoRunning && g.running:
			a.stop(g, t)
		}
		if to == trace.GoNotExist && len(g.regions) == 0 {
			delete(a.goroutines, id)
		}
	case trace.EventLabel:
		// the runtime labels a GC mark worker right as it starts running
		l := ev.Label()
		if l.Resource.Kind != trace.ResourceGoroutine || !strings.HasPrefix(l.Label, "GC (") {
			return
		}
		if g := a.goroutine(l.Resource.Goroutine()); g.running {
			g.class = classGC
			if l.Label == "GC (idle)" {
				g.class = classIdleGC
			}
		}
	case trace.EventRangeBegin, trace.EventRangeActive, trace.EventRangeEnd:
		rg := ev.Range()
		if rg.Name != "GC mark assist" || rg.Scope.Kind != trace.ResourceGoroutine {
			return
		}
		g := a.goroutine(rg.Scope.Goroutine())
		g.assist = ev.Kind() != trace.EventRangeEnd
		want := classApp
		if g.assist {
			want = classGC
		}
		if g.running && g.class != classIdleGC && g.class != want {
			a.stop(g, t)
			g.running, g.since, g.class = true, t, want
		}
	case trace.EventRegionBegin:
		g := a.goroutine(ev.Goroutine())
		g.regions = append(g.regions, openRegion{name: ev.Region().Type, start: t})
	case trace.EventRegionEnd:
		g := a.goroutine(ev.Goroutine())
		name := ev.Region().Type
		start := a.start
		if n := len(g.regions); n > 0 && g.regions[n-1].name == name {
			start = g.regions[n-1].start
			g.regions = g.regions[:n-1]
		}
		a.regions = append(a.regions, region{name: name, start: start, end: t})
	}
}

func (a *analyzer) goroutine(id trace.GoID) *goroutine {
	g := a.goroutines[id]
	if g == nil {
		g = &goroutine{}
		a.goroutines[id] = g
	}
	return g
}

func (a *analyzer) stop(g *goroutine, t trace.Time) {
	a.running[g.class] = append(a.running[g.class], change{g.since, 1}, change{t, -1})
	g.running = false
}

func (a *analyzer) setProcs(t trace.Time, n int64) {
	level := int64(0)
	for _, c := range a.procs {
		level += c.delta
	}
	if len(a.procs) == 0 {
		// the first report stands for the stretch before it too
		t = a.start
	}
	if n != level {
		a.procs = append(a.procs, change{t, n - level})
	}
}

func (a *analyzer) finish() {
	for _, g := range a.goroutines {
		if g.running {
			a.stop(g, a.last)
		}
		for _, r := range g.regions {
			a.regions = append(a.regions, region{name: r.name, start: r.start, end: a.last})
		}
	}
	slices.SortStableFunc(a.regions, func(x, y region) int {
		return cmp.Compare(x.start, y.start)
	})
}

func (a *analyzer) report(window time.Duration) *Report {
	var running [numClasses]integral
	for c := range running {
		running[c] = integrate(a.running[c])
	}
	capacity := integrate(a.procs)
	span := func(name string, from, to trace.Time) Span {
		app := running[classApp].between(from, to)
		gc := running[classGC].between(from, to)
		return Span{
			Name:     name,
			Start:    from.Sub(a.start),
			End:      to.Sub(a.start),
			Busy:     time.Duration(app + gc),
			GC:       time.Duration(gc),
			Capacity: time.Duration(capacity.between(from, to)),
		}
	}

	rep := &Report{Total: span("", a.start, a.last)}
	if window > 0 {
		for from := a.start; from < a.last; from += trace.Time(window) {
			rep.Windows = append(rep.Windows, span("", from, min(from+trace.Time(window), a.last)))
		}
	}
	for _, r := range a.regions {
		rep.Regions = append(rep.Regions, span(r.name, r.start, r.end))
	}
	return rep
}

// integral is a step function's running integral, over the points where it changes.
type integral struct {
	at    []trace.Time
	sum   []int64 // up to at[i]
	level []int64 // from at[i] on
}

func integrate(changes []change) integral {
	changes = slices.Clone(changes)
	slices.SortStableFunc(changes, func(x, y change) int {
		return cmp.Compare(x.at, y.at)
	})
	var out integral
	var sum, level int64
	for i, c := range changes {
		if i > 0 {
			sum += level * int64(c.at-changes[i-1].at)
		}
		level += c.delta
		if n := len(out.at); n > 0 && out.at[n-1] == c.at {
			out.level[n-1] = level
			continue
		}
		out.at = append(out.at, c.at)
		out.sum = append(out.sum, sum)
		out.level = append(out.level, level)
	}
	return out
}

func (in integral) upTo(t trace.Time) int64 {
	i := sort.Search(len(in.at), func(i int) bool { return in.at[i] > t }) - 1
	if i < 0 {
		return 0
	}
	return in.sum[i] + in.level[i]*int64(t-in.at[i])
}

func (in integral) between(from, to trace.Time) int64 {
	return in.upTo(to) - in.upTo(from)
}

// Write prints the windows, then the regions, then the total.
func (rep *Report) Write(w io.Writer) error {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', tabwriter.AlignRight)
	fmt.Fprintf(w, "busy: goroutines running at once, over GOMAXPROCS (idle-priority GC workers left out); gc: their share in GC\n\n")
	if len(rep.Windows) > 0 {
		hasRegions := len(rep.Regions) > 0
		fmt.Fprint(tw, "time\tbusy\tutil\tgc\t")
		if hasRegions {
			fmt.Fprint(tw, "region\t")
		}
		fmt.Fprintln(tw)
		for _, s := range rep.Windows {
			fmt.Fprintf(tw, "%s\t%s\t", seconds(s.Start), rep.busy(s))
			if hasRegions {
				fmt.Fprintf(tw, "%s\t", rep.regionAt(s))
			}
			fmt.Fprintln(tw)
		}
		fmt.Fprintln(tw)
	}
	fmt.Fprint(tw, "region\tstart\twall\tbusy\tutil\tgc\t\n")
	for _, s := range rep.Regions {
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t\n", s.Name, seconds(s.Start), seconds(s.wall()), rep.busy(s))
	}
	fmt.Fprintf(tw, "total\t%s\t%s\t%s\t\n", seconds(rep.Total.Start), seconds(rep.Total.wall()), rep.busy(rep.Total))
	return tw.Flush()
}

// busy is the busy, util and gc columns.
func (rep *Report) busy(s Span) string {
	util := "-"
	if s.Capacity > 0 {
		util = fmt.Sprintf("%.0f%%", 100*s.Utilization())
	}
	return fmt.Sprintf("%.1f/%.0f\t%s\t%.0f%%", s.Threads(), s.Procs(), util, 100*s.GCShare())
}

// regionAt names the region that covers most of s, or "-".
func (rep *Report) regionAt(s Span) string {
	best, name := time.Duration(0), "-"
	for _, r := range rep.Regions {
		if overlap := min(r.End, s.End) - max(r.Start, s.Start); overlap > best {
			best, name = overlap, r.Name
		}
	}
	return name
}

func seconds(d time.Duration) string {
	return fmt.Sprintf("%.2fs", d.Seconds())
}
