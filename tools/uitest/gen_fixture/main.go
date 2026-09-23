// Runs a RaidSimRequest and writes the SimRun the detailed results page expects, so the Playwright
// tests can replay a real result instead of simulating one. See tools/uitest/README.md.
package main

import (
	"bytes"
	"compress/gzip"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
	"google.golang.org/protobuf/encoding/protojson"

	"github.com/wowsims/wotlk/sim"
)

func main() {
	sim.RegisterAll()

	infile := flag.String("infile", "", "RaidSimRequest in protojson format")
	outfile := flag.String("outfile", "", "where to write the SimRun")
	iterations := flag.Int("iterations", 100, "iterations to run")
	seed := flag.Int64("seed", 101, "random seed, so the fixture is reproducible")
	logs := flag.Bool("logs", true, "capture the first iteration's combat log, which the Log and Timeline tabs need")
	duration := flag.Float64("duration", 0, "override the encounter duration in seconds, which is what a log fixture's size hangs on")
	flag.Parse()

	if *infile == "" || *outfile == "" {
		log.Fatal("both -infile and -outfile are required")
	}

	data, err := os.ReadFile(*infile)
	if err != nil {
		log.Fatalf("reading %s: %v", *infile, err)
	}

	request := &proto.RaidSimRequest{}
	if err := (protojson.UnmarshalOptions{DiscardUnknown: true}).Unmarshal(data, request); err != nil {
		log.Fatalf("parsing %s: %v", *infile, err)
	}

	if *duration > 0 && request.Encounter != nil {
		request.Encounter.Duration = *duration
		request.Encounter.DurationVariation = 0
	}

	request.SimOptions = &proto.SimOptions{
		Iterations:          int32(*iterations),
		RandomSeed:          *seed,
		DebugFirstIteration: *logs,
	}

	result := core.RunRaidSim(request)
	if result.ErrorResult != "" {
		log.Fatalf("the sim failed: %s", result.ErrorResult)
	}

	run, err := protojson.Marshal(&proto.SimRun{Request: request, Result: result})
	if err != nil {
		log.Fatalf("marshalling the run: %v", err)
	}
	if strings.HasSuffix(*outfile, ".gz") {
		var buf bytes.Buffer
		zw, err := gzip.NewWriterLevel(&buf, gzip.BestCompression)
		if err != nil {
			log.Fatalf("compressing: %v", err)
		}
		if _, err := zw.Write(run); err != nil {
			log.Fatalf("compressing: %v", err)
		}
		if err := zw.Close(); err != nil {
			log.Fatalf("compressing: %v", err)
		}
		run = buf.Bytes()
	}

	if err := os.WriteFile(*outfile, run, 0o644); err != nil {
		log.Fatalf("writing %s: %v", *outfile, err)
	}

	players := 0
	for _, party := range result.RaidMetrics.Parties {
		for _, player := range party.Players {
			// empty raid slots come back as blank metrics
			if player.Name != "" {
				players++
			}
		}
	}
	fmt.Printf("wrote %s (%d bytes), %d players over %d iterations\n", *outfile, len(run), players, *iterations)
}
