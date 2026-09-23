package cmd

import (
	"compress/gzip"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/wowsims/wotlk/sim"
	"github.com/wowsims/wotlk/sim/core/proto"
	exptrace "golang.org/x/exp/trace"
	"google.golang.org/protobuf/encoding/protojson"
)

func init() {
	sim.RegisterAll()
}

// profileArgs asks for all three profiles in dir.
func profileArgs(dir string) []string {
	return []string{
		"--cpuprofile", filepath.Join(dir, "cpu.pprof"),
		"--memprofile", filepath.Join(dir, "mem.pprof"),
		"--trace", filepath.Join(dir, "run.trace"),
	}
}

func checkProfiles(t *testing.T, dir string) {
	t.Helper()
	for _, name := range []string{"cpu.pprof", "mem.pprof"} {
		f, err := os.Open(filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		// pprof profiles are gzipped protobufs
		gz, err := gzip.NewReader(f)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if data, err := io.ReadAll(gz); err != nil || len(data) == 0 {
			t.Errorf("%s: %d bytes (%v), want a profile", name, len(data), err)
		}
	}

	f, err := os.Open(filepath.Join(dir, "run.trace"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	r, err := exptrace.NewReader(f)
	if err != nil {
		t.Fatalf("run.trace: %v", err)
	}
	events := 0
	for {
		_, err := r.ReadEvent()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatalf("run.trace, after %d events: %v", events, err)
		}
		events++
	}
	if events == 0 {
		t.Error("run.trace has no events")
	}
}

func TestOptimizeCommandProfiles(t *testing.T) {
	dir := t.TempDir()
	args := append([]string{"--infile", writeRequest(t, optimizeTestRequest()), "--outfile", filepath.Join(dir, "result.json")}, profileArgs(dir)...)
	if _, err := runOptimize(t, args...); err != nil {
		t.Fatal(err)
	}
	checkProfiles(t, dir)
}

func TestSimCommandProfiles(t *testing.T) {
	// the optimizer test's warrior, with just enough to sim: white hits only
	req := optimizeTestRequest().Base
	player := req.Raid.Parties[0].Players[0]
	player.Spec = &proto.Player_Warrior{Warrior: &proto.Warrior{Options: &proto.Warrior_Options{}}}
	player.Rotation = &proto.APLRotation{Type: proto.APLRotation_TypeAPL}
	req.SimOptions = &proto.SimOptions{Iterations: 20, RandomSeed: 1}
	data, err := protojson.Marshal(req)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	infile := filepath.Join(dir, "request.json")
	if err := os.WriteFile(infile, data, 0666); err != nil {
		t.Fatal(err)
	}

	outfile := filepath.Join(dir, "result.json")
	simCmd.SetArgs(append([]string{"--infile", infile, "--outfile", outfile}, profileArgs(dir)...))
	if err := simCmd.Execute(); err != nil {
		t.Fatal(err)
	}
	out, err := os.ReadFile(outfile)
	if err != nil {
		t.Fatal(err)
	}
	result := &proto.RaidSimResult{}
	if err := protojson.Unmarshal(out, result); err != nil {
		t.Fatalf("output isn't a RaidSimResult: %v", err)
	}
	if result.ErrorResult != "" || result.GetRaidMetrics().GetDps().GetAvg() <= 0 {
		t.Fatalf("error %q, raid DPS %v; want a sim that ran", result.ErrorResult, result.GetRaidMetrics().GetDps().GetAvg())
	}
	checkProfiles(t, dir)
}
