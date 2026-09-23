package tracestat

import (
	"bytes"
	"context"
	"runtime"
	"runtime/trace"
	"strings"
	"sync"
	"testing"
	"time"
)

// spin keeps n goroutines running for d.
func spin(n int, d time.Duration) {
	var wg sync.WaitGroup
	for range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for start := time.Now(); time.Since(start) < d; {
			}
		}()
	}
	wg.Wait()
}

var sink [][]byte

// collect runs a few forced GCs over a heap worth marking.
func collect() {
	for range 5 {
		sink = nil
		for range 20000 {
			sink = append(sink, make([]byte, 256))
		}
		runtime.GC()
	}
	sink = nil
}

// traceWorkload traces a spin, a sleep and some GC, each in its own region.
func traceWorkload(t *testing.T, spinners int) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	if err := trace.Start(&buf); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	trace.WithRegion(ctx, "spin", func() { spin(spinners, 300*time.Millisecond) })
	trace.WithRegion(ctx, "sleep", func() { time.Sleep(200 * time.Millisecond) })
	trace.WithRegion(ctx, "collect", collect)
	trace.Stop()
	return &buf
}

func TestAnalyze(t *testing.T) {
	procs := runtime.GOMAXPROCS(0)
	spinners := min(4, procs)
	report, err := Analyze(traceWorkload(t, spinners), 100*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}

	var names []string
	regions := map[string]Span{}
	for _, r := range report.Regions {
		names = append(names, r.Name)
		regions[r.Name] = r
	}
	if got := strings.Join(names, " "); got != "spin sleep collect" {
		t.Fatalf("regions %q, want spin, sleep and collect in order", got)
	}

	spinning := regions["spin"]
	if got := spinning.Threads(); got < 0.8*float64(spinners) || got > float64(spinners)+1 {
		t.Errorf("spin kept %.2f threads busy, want about %d", got, spinners)
	}
	if got := spinning.Procs(); got != float64(procs) {
		t.Errorf("spin saw GOMAXPROCS %.2f, want %d", got, procs)
	}
	if got, want := spinning.Utilization(), float64(spinners)/float64(procs); got < 0.8*want || got > want+1/float64(procs) {
		t.Errorf("spin utilization %.2f, want about %.2f", got, want)
	}
	if got := regions["sleep"].Threads(); got > 0.2 {
		t.Errorf("sleep kept %.2f threads busy, want about none", got)
	}
	if got := regions["collect"].GCShare(); got <= 0 {
		t.Errorf("collect spent %.2f in GC, want some", got)
	}

	// windows tile the whole trace
	if len(report.Windows) == 0 || report.Windows[0].Start != 0 || report.Windows[len(report.Windows)-1].End != report.Total.End {
		t.Fatalf("windows %v don't cover the trace's %v", report.Windows, report.Total.End)
	}
	for i := 1; i < len(report.Windows); i++ {
		if report.Windows[i].Start != report.Windows[i-1].End {
			t.Fatalf("window %d starts at %v, the one before ends at %v", i, report.Windows[i].Start, report.Windows[i-1].End)
		}
	}
	var busy, gc time.Duration
	for _, w := range report.Windows {
		busy += w.Busy
		gc += w.GC
	}
	if busy != report.Total.Busy || gc != report.Total.GC {
		t.Errorf("windows add up to %v busy and %v GC, the total says %v and %v", busy, gc, report.Total.Busy, report.Total.GC)
	}

	var out bytes.Buffer
	if err := report.Write(&out); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"region", "spin", "sleep", "collect", "total"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("output has no %q:\n%s", want, out.String())
		}
	}
}

func TestAnalyzeNoWindows(t *testing.T) {
	report, err := Analyze(traceWorkload(t, 1), 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Windows) != 0 || len(report.Regions) != 3 {
		t.Errorf("%d windows and %d regions, want none and 3", len(report.Windows), len(report.Regions))
	}
}

func TestAnalyzeNotATrace(t *testing.T) {
	if _, err := Analyze(strings.NewReader("not a trace"), time.Second); err == nil {
		t.Error("a file that isn't a trace didn't fail")
	}
}
