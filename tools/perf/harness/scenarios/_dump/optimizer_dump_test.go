//go:build optimizer_slow

package optimizer

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
	"google.golang.org/protobuf/encoding/protojson"
	goproto "google.golang.org/protobuf/proto"
)

// dumpOptimizeRequest writes req to dir/name, dropping each catalog row's sources: the search never
// reads them, and they're most of the file.
func dumpOptimizeRequest(t *testing.T, dir, name string, req *proto.OptimizeGearRequest) {
	t.Helper()
	for i, row := range req.Pool.CatalogItems {
		row = goproto.Clone(row).(*proto.CatalogItem)
		row.Sources = nil
		req.Pool.CatalogItems[i] = row
	}
	data, err := protojson.Marshal(req)
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := json.Compact(&out, data); err != nil {
		t.Fatal(err)
	}
	out.WriteByte('\n')
	if err := os.WriteFile(filepath.Join(dir, name), out.Bytes(), 0666); err != nil {
		t.Fatal(err)
	}
}

// TestPerfDumpSlowRequests writes the slow suite's requests to $PERF_DUMP_DIR as
// opt_<spec>_p<phase>.json, at Quick.
func TestPerfDumpSlowRequests(t *testing.T) {
	dir := os.Getenv("PERF_DUMP_DIR")
	if dir == "" {
		t.Skip("PERF_DUMP_DIR isn't set")
	}
	for _, c := range slowCases {
		req := slowRequest(t, c, proto.OptimizerEffort_OptimizerEffortQuick)
		dumpOptimizeRequest(t, dir, fmt.Sprintf("opt_%s_p%d.json", c.spec, c.phase), req)
	}
}

// TestPerfDumpRaid25Requests writes a tank run and a raid-mode (stage 2) request to $PERF_DUMP_DIR,
// both against raidctx's synthetic 25-player raid fixture instead of a live roster, which the repo
// never commits. Both derive a solo context internally (raidctx.Derive): the tank run measures that
// path's own cost, on top of an ordinary own-metrics run; the raid-mode one additionally scores every
// sim against the real 25-player raid, the harness's only scenario that does.
func TestPerfDumpRaid25Requests(t *testing.T) {
	dir := os.Getenv("PERF_DUMP_DIR")
	if dir == "" {
		t.Skip("PERF_DUMP_DIR isn't set")
	}
	const phase = 1
	base := loadRaid25(t)

	tankReq := &proto.OptimizeGearRequest{
		Base:            goproto.Clone(base).(*proto.RaidSimRequest),
		TargetRaidIndex: 0, // Tankwar, Protection Warrior
		Settings: &proto.OptimizerSettings{
			ContentPhase:        phase,
			Effort:              proto.OptimizerEffort_OptimizerEffortQuick,
			TankSurvival:        0.7,
			RequireCritImmunity: true,
		},
	}
	tankReq.Base.Raid.Tanks = []*proto.UnitReference{{Type: proto.UnitReference_Player, Index: 0}}
	tankReq.Base.Encounter = tankEncounter(t, phase)
	tankTarget, err := raidPlayer(tankReq.Base.Raid, 0)
	if err != nil {
		t.Fatal(err)
	}
	tankTarget.HealingModel = tankHealing(tankReq.Base.Encounter.Targets[0])
	tankReq.Pool = realisticPool(t, tankTarget, phase, nil)
	tankReq.Equipped = goproto.Clone(tankTarget.Equipment).(*proto.EquipmentSpec)
	trimSeedToPool(t, tankTarget, tankReq.Pool)
	dumpOptimizeRequest(t, dir, fmt.Sprintf("opt_raid25_tank_p%d.json", phase), tankReq)

	raidReq := &proto.OptimizeGearRequest{
		Base:            goproto.Clone(base).(*proto.RaidSimRequest),
		TargetRaidIndex: 5, // Fury, a DPS warrior
		Settings: &proto.OptimizerSettings{
			ContentPhase: phase,
			Effort:       proto.OptimizerEffort_OptimizerEffortQuick,
			Objective:    proto.OptimizerObjective_OptimizerObjectiveRaidDps,
		},
	}
	raidTarget, err := raidPlayer(raidReq.Base.Raid, 5)
	if err != nil {
		t.Fatal(err)
	}
	raidReq.Pool = realisticPool(t, raidTarget, phase, nil)
	raidReq.Equipped = goproto.Clone(raidTarget.Equipment).(*proto.EquipmentSpec)
	trimSeedToPool(t, raidTarget, raidReq.Pool)
	dumpOptimizeRequest(t, dir, fmt.Sprintf("opt_raid25_raiddps_p%d.json", phase), raidReq)
}

// loadRaid25 is raidctx's synthetic 25-player raid, built from the ui/<spec> P1 presets and the
// talents in the spec tests (sim/optimizer/raidctx/testdata/raid25.json). It needs the with_db item
// data, like every realistic pool.
func loadRaid25(tb testing.TB) *proto.RaidSimRequest {
	tb.Helper()
	if !core.WITH_DB {
		tb.Skip("needs the with_db item data")
	}
	data, err := os.ReadFile("raidctx/testdata/raid25.json")
	if err != nil {
		tb.Fatal(err)
	}
	req := &proto.RaidSimRequest{}
	if err := protojson.Unmarshal(data, req); err != nil {
		tb.Fatal(err)
	}
	return req
}
