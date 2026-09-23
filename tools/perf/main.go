// Command perf finds where the sim and the optimizer spend their time. README.md has the commands.
package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/wowsims/wotlk/sim"
	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/tools/perf/harness"
	"github.com/wowsims/wotlk/tools/perf/tracestat"
)

const usage = `usage: perf <command> [flags]

  trace [-window 100ms] <file>        threads busy and the GC's share per window and per trace
                                      region, from a runtime/trace
  bench [flags]                       time the scenarios over a GOMAXPROCS sweep, write a report and
                                      compare it with the baseline; bench -h lists the flags
  show <report.json>                  a report's summary
  compare [-floor 0.05] <base> <new>  what moved past the noise from one report to the other

bench needs the item database: go run --tags=with_db ./tools/perf bench
`

func main() {
	if len(os.Args) < 2 {
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}
	var err error
	switch os.Args[1] {
	case "trace":
		err = traceMain(os.Args[2:])
	case "bench":
		err = benchMain(os.Args[2:])
	case "show":
		err = showMain(os.Args[2:])
	case "compare":
		err = compareMain(os.Args[2:])
	default:
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func traceMain(args []string) error {
	fs := flag.NewFlagSet("trace", flag.ExitOnError)
	window := fs.Duration("window", 100*time.Millisecond, "length of each line over time; 0 prints just the regions and the total")
	fs.Parse(args)
	if fs.NArg() != 1 {
		return fmt.Errorf("trace takes one trace file, got %d arguments", fs.NArg())
	}
	f, err := os.Open(fs.Arg(0))
	if err != nil {
		return err
	}
	defer f.Close()
	report, err := tracestat.Analyze(bufio.NewReader(f), *window)
	if err != nil {
		return fmt.Errorf("%s: %w", fs.Arg(0), err)
	}
	return report.Write(os.Stdout)
}

func benchMain(args []string) error {
	fs := flag.NewFlagSet("bench", flag.ExitOnError)
	run := fs.String("run", "", "only the scenarios whose name matches this regexp")
	list := fs.Bool("list", false, "list the scenarios -run picks and stop")
	procs := fs.String("procs", "1,2,4,8,12,16", "the GOMAXPROCS sweep")
	reps := fs.Int("reps", 3, "timed runs at each scenario's widest point")
	sweepReps := fs.Int("sweep-reps", 1, "timed runs at the sweep's other points")
	normalProcs := fs.String("normal-procs", "16", "the sweep for the optimizer at Normal, the longest scenarios")
	normalReps := fs.Int("normal-reps", 1, "timed runs at the optimizer at Normal's widest point")
	workers := fs.Int("workers", 0, "the optimizer's workers; 0 is GOMAXPROCS at each point, the CLI's default (the web server runs one fewer)")
	iterations := fs.Int("iterations", 3000, "iterations for the sims, stat weights and the bulk sim; the UI's default is 3000")
	dir := fs.String("o", "", "where the report and the profiles go (default tmp/perf/bench-<time>)")
	profiles := fs.Bool("profiles", true, "CPU profile the last run at each scenario's widest point")
	baseline := fs.String("baseline", "tools/perf/baseline.json", "compare with this report when it exists; empty skips")
	floor := fs.Float64("floor", harness.DefaultFloor, "the smallest change the comparison flags")
	note := fs.String("note", "", "saved in the report, e.g. what else ran")
	commit := fs.String("commit", "", "the sim's commit, saved in the report; go run doesn't record one")
	fs.Parse(args)
	if fs.NArg() != 0 {
		return fmt.Errorf("bench takes no arguments, got %q", fs.Args())
	}
	if !core.WITH_DB {
		return errors.New("bench needs the item database: go run --tags=with_db ./tools/perf bench")
	}

	sim.RegisterAll()
	cfg := harness.Config{
		Iterations: int32(*iterations),
		Reps:       *reps,
		NormalReps: *normalReps,
		SweepReps:  *sweepReps,
		Workers:    *workers,
		Profiles:   *profiles,
		Note:       *note,
		Commit:     *commit,
		Args:       args,
		Log:        os.Stderr,
	}
	var err error
	if *run != "" {
		if cfg.Match, err = regexp.Compile(*run); err != nil {
			return err
		}
	}
	if cfg.Procs, err = parseProcs(*procs); err != nil {
		return err
	}
	if cfg.NormalProcs, err = parseProcs(*normalProcs); err != nil {
		return err
	}
	if *list {
		scenarios, err := harness.Selected(cfg)
		if err != nil {
			return err
		}
		for _, s := range scenarios {
			fmt.Println(s.Name)
		}
		return nil
	}
	cfg.Dir = *dir
	if cfg.Dir == "" {
		cfg.Dir = "tmp/perf/bench-" + time.Now().Format("20060102-150405")
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	rep, err := harness.Run(ctx, cfg)
	if err != nil {
		if rep != nil && len(rep.Points) > 0 {
			fmt.Fprintf(os.Stderr, "%s/report.json has the points that finished\n", cfg.Dir)
		}
		return err
	}
	if err := rep.Write(os.Stdout); err != nil {
		return err
	}
	fmt.Printf("\nreport in %s\n", cfg.Dir)
	if *baseline == "" {
		return nil
	}
	base, err := harness.ReadReport(*baseline)
	if errors.Is(err, os.ErrNotExist) {
		fmt.Printf("no baseline at %s, so no comparison\n", *baseline)
		return nil
	}
	if err != nil {
		return err
	}
	fmt.Printf("\nagainst %s:\n", *baseline)
	return compare(base, rep, *floor)
}

func parseProcs(s string) ([]int, error) {
	var out []int
	for _, field := range strings.Split(s, ",") {
		n, err := strconv.Atoi(strings.TrimSpace(field))
		if err != nil || n < 1 {
			return nil, fmt.Errorf("a GOMAXPROCS sweep is a comma-separated list of positive numbers, got %q", s)
		}
		out = append(out, n)
	}
	return out, nil
}

func showMain(args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("show takes one report, got %d arguments", len(args))
	}
	rep, err := harness.ReadReport(args[0])
	if err != nil {
		return err
	}
	return rep.Write(os.Stdout)
}

func compareMain(args []string) error {
	fs := flag.NewFlagSet("compare", flag.ExitOnError)
	floor := fs.Float64("floor", harness.DefaultFloor, "the smallest change it flags")
	fs.Parse(args)
	if fs.NArg() != 2 {
		return fmt.Errorf("compare takes a baseline and a report, got %d arguments", fs.NArg())
	}
	base, err := harness.ReadReport(fs.Arg(0))
	if err != nil {
		return err
	}
	cur, err := harness.ReadReport(fs.Arg(1))
	if err != nil {
		return err
	}
	return compare(base, cur, *floor)
}

// compare fails when anything got worse, so the exit status says so.
func compare(base, cur *harness.Report, floor float64) error {
	c := harness.Compare(base, cur, floor)
	if err := c.Write(os.Stdout); err != nil {
		return err
	}
	if n := c.Worse(); n > 0 {
		return fmt.Errorf("%d worse than the baseline", n)
	}
	return nil
}
