// Command simval compares this sim with the live AzerothCore server.
//
// Without a subcommand it replays mod-sim-validation's recordings through the
// sim's combat tables and reports where the two disagree. The module writes one
// JSON line per `.simval` command to the worldserver's <LogsDir>/simval/simval.jsonl,
// with a snapshot of both units and the table the server derived from them. This
// rebuilds that table from the snapshot and compares, so a mismatch points at the
// sim's formula rather than at noise.
//
// `chronicle` reads a mod-chronicle raw combat log of a real fight and reports the
// player's DPS, ability breakdown, swing and tick intervals and aura uptimes,
// checking the DPS against the sim's result for the same setup.
//
//	tools/acore/dock.sh run ./tools/simval
//	tools/acore/dock.sh run ./tools/simval -fixture
//	tools/acore/dock.sh run ./tools/simval chronicle -sim 8123 <log>
package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	fixtureDir   = "sim/core/testdata/simval"
	chronicleDir = "sim/core/testdata/chronicle"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "chronicle" {
		os.Exit(runChronicle(os.Args[2:]))
	}
	os.Exit(runReplay(os.Args[1:]))
}

func runReplay(args []string) int {
	flags := flag.NewFlagSet("simval", flag.ExitOnError)
	records := flags.String("records", "/ac/env/dist/logs/simval/simval.jsonl", "the module's JSONL recording")
	fixture := flags.Bool("fixture", false, "also copy the records into "+fixtureDir)
	verbose := flags.Bool("v", false, "print passing checks too")
	_ = flags.Parse(args)

	lines, err := readRecords(*records)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	if len(lines) == 0 {
		fmt.Fprintf(os.Stderr, "%s has no records. Run the module's e2e suite against the live server first.\n", *records)
		return 1
	}

	failed := report(os.Stdout, lines, *verbose)

	if *fixture {
		if err := writeFixture(*records); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
	}
	if failed > 0 {
		return 1
	}
	return 0
}

func runChronicle(args []string) int {
	flags := flag.NewFlagSet("simval chronicle", flag.ExitOnError)
	source := flags.String("source", "", "the player to measure; needed when the log has more than one")
	target := flags.String("target", "", "only count damage dealt to this unit")
	simDPS := flags.Float64("sim", 0, "the sim's DPS for the same gear, talents and rotation")
	gap := flags.Float64("gap", 5, "a pause longer than this many seconds is left out of the active duration")
	top := flags.Int("top", 20, "how many ability rows to print")
	verbose := flags.Bool("v", false, "also print swing intervals, tick intervals and aura uptimes")
	flags.Usage = func() {
		fmt.Fprintf(flags.Output(), "usage: simval chronicle [flags] <log file>\n\n"+
			"Reads a mod-chronicle raw log; captured runs live in %s.\n\n", chronicleDir)
		flags.PrintDefaults()
	}
	_ = flags.Parse(args)

	if flags.NArg() != 1 {
		flags.Usage()
		return 2
	}

	log, err := readChronicleLog(flags.Arg(0))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	opts := chronicleOptions{
		Source: *source, Target: *target, SimDPS: *simDPS,
		GapSec: *gap, Top: *top, Verbose: *verbose,
	}
	stats, err := analyzeChronicle(log, opts)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	if reportChronicle(os.Stdout, log, stats, opts) > 0 {
		return 1
	}
	return 0
}

func readRecords(path string) ([]record, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var records []record
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64<<10), 8<<20)
	for line := 1; scanner.Scan(); line++ {
		text := strings.TrimSpace(scanner.Text())
		if text == "" {
			continue
		}
		var rec record
		if err := json.Unmarshal([]byte(text), &rec); err != nil {
			return nil, fmt.Errorf("%s line %d: %w", path, line, err)
		}
		records = append(records, rec)
	}
	return records, scanner.Err()
}

// report prints one block per record and returns how many checks failed.
func report(out *os.File, records []record, verbose bool) int {
	var total, failed int

	for _, rec := range records {
		checks := CheckRecord(rec)
		if len(checks) == 0 {
			continue
		}

		var lines []string
		for _, check := range checks {
			total++
			if !check.Passed {
				failed++
			}
			if verbose || !check.Passed {
				status := "PASS"
				if !check.Passed {
					status = "FAIL"
				}
				lines = append(lines, fmt.Sprintf("  %s %-24s %s", status, check.Name, check.Detail))
			}
		}

		if len(lines) > 0 {
			fmt.Fprintln(out, rec.label())
			fmt.Fprintln(out, strings.Join(lines, "\n"))
		}
	}

	fmt.Fprintf(out, "%d of %d checks passed\n", total-failed, total)
	return failed
}

// writeFixture keeps a copy of the recording next to the tests, so the comparison
// can run without a live server.
func writeFixture(records string) error {
	if err := os.MkdirAll(fixtureDir, 0o755); err != nil {
		return err
	}

	data, err := os.ReadFile(records)
	if err != nil {
		return err
	}

	dest := filepath.Join(fixtureDir, "simval.jsonl")
	if err := os.WriteFile(dest, data, 0o644); err != nil {
		return err
	}

	fmt.Printf("saved %s\n", dest)
	return nil
}
