package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wowsims/wotlk/sim/core/proto"
	"github.com/wowsims/wotlk/sim/optimizer"
	"google.golang.org/protobuf/encoding/protojson"
)

// The request carries its own made-up items, so this doesn't depend on the with_db item data.
func optimizeTestRequest() *proto.OptimizeGearRequest {
	equipment := &proto.EquipmentSpec{Items: make([]*proto.ItemSpec, optimizer.NumSlots)}
	for slot := range equipment.Items {
		equipment.Items[slot] = &proto.ItemSpec{}
	}
	equipment.Items[proto.ItemSlot_ItemSlotHead] = &proto.ItemSpec{Id: 9300001, Enchant: 9300003, Gems: []int32{9300002}}

	return &proto.OptimizeGearRequest{
		Base: &proto.RaidSimRequest{
			Raid: &proto.Raid{Parties: []*proto.Party{{Players: []*proto.Player{{
				Name:         "Target",
				Race:         proto.Race_RaceOrc,
				RacialTraits: proto.Race_RaceHuman,
				Class:        proto.Class_ClassWarrior,
				Equipment:    equipment,
				Database: &proto.SimDatabase{
					Items:    []*proto.SimItem{{Id: 9300001, Type: proto.ItemType_ItemTypeHead, GemSockets: []proto.GemColor{proto.GemColor_GemColorRed}}},
					Gems:     []*proto.SimGem{{Id: 9300002, Color: proto.GemColor_GemColorRed}},
					Enchants: []*proto.SimEnchant{{EffectId: 9300003}},
				},
			}}}}},
			Encounter: &proto.Encounter{Duration: 180, Targets: []*proto.Target{{}}},
		},
		Settings: &proto.OptimizerSettings{ContentPhase: 2},
		Pool:     &proto.CandidatePool{CatalogDate: "2026-09-18"},
	}
}

func writeRequest(t *testing.T, req *proto.OptimizeGearRequest) string {
	t.Helper()
	data, err := protojson.Marshal(req)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "request.json")
	if err := os.WriteFile(path, data, 0666); err != nil {
		t.Fatal(err)
	}
	return path
}

func runOptimize(t *testing.T, args ...string) (string, error) {
	t.Helper()
	cmd := newOptimizeCommand()
	var stdout bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs(args)
	err := cmd.Execute()
	return stdout.String(), err
}

func readResult(t *testing.T, data []byte) *proto.OptimizerResult {
	t.Helper()
	result := &proto.OptimizerResult{}
	if err := protojson.Unmarshal(data, result); err != nil {
		t.Fatalf("output isn't an OptimizerResult: %v\n%s", err, data)
	}
	return result
}

func TestOptimizeCommand(t *testing.T) {
	req := optimizeTestRequest()
	outfile := filepath.Join(t.TempDir(), "result.json")
	if _, err := runOptimize(t, "--infile", writeRequest(t, req), "--outfile", outfile, "--workers", "2", "--verbose"); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(outfile)
	if err != nil {
		t.Fatal(err)
	}
	result := readResult(t, data)

	target := req.Base.Raid.Parties[0].Players[0]
	want, err := optimizer.LoadoutFromProto(target.Equipment, target.RacialTraits)
	if err != nil {
		t.Fatal(err)
	}
	got, err := optimizer.LoadoutFromProto(result.GetBest().GetEquipment(), result.GetBest().GetRacialTraits())
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Errorf("best = %+v, want the seed %+v", got, want)
	}
	if result.Settings.GetWorkers() != 2 || result.Settings.GetContentPhase() != 2 || result.CatalogDate != "2026-09-18" {
		t.Errorf("settings %v, catalog date %q; want the request's, with --workers applied", result.Settings, result.CatalogDate)
	}
}

func TestOptimizeCommandStdout(t *testing.T) {
	stdout, err := runOptimize(t, "--infile", writeRequest(t, optimizeTestRequest()))
	if err != nil {
		t.Fatal(err)
	}
	if result := readResult(t, []byte(stdout)); result.GetBest().GetRacialTraits() != proto.Race_RaceHuman {
		t.Errorf("best racial traits = %v, want the seed's human", result.GetBest().GetRacialTraits())
	}
}

func TestOptimizeCommandFails(t *testing.T) {
	req := optimizeTestRequest()
	req.TargetRaidIndex = 3
	outfile := filepath.Join(t.TempDir(), "result.json")
	_, err := runOptimize(t, "--infile", writeRequest(t, req), "--outfile", outfile)
	if err == nil || !strings.Contains(err.Error(), "empty raid slot") {
		t.Fatalf("err = %v, want the empty raid slot error", err)
	}
	data, err := os.ReadFile(outfile)
	if err != nil {
		t.Fatal(err)
	}
	if result := readResult(t, data); !strings.Contains(result.ErrorResult, "empty raid slot") {
		t.Errorf("error_result = %q, want the same error", result.ErrorResult)
	}

	if _, err := runOptimize(t, "--infile", filepath.Join(t.TempDir(), "missing.json")); err == nil {
		t.Error("a missing input file didn't fail")
	}
}
