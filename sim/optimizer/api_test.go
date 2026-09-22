package optimizer

import (
	"context"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/wowsims/wotlk/sim/core/proto"
	"github.com/wowsims/wotlk/sim/optimizer/raidctx"
	goproto "google.golang.org/protobuf/proto"
)

func seedOf(t *testing.T, req *proto.OptimizeGearRequest) Loadout {
	t.Helper()
	player := req.Base.Raid.Parties[req.TargetRaidIndex/5].Players[req.TargetRaidIndex%5]
	l, err := LoadoutFromProto(player.Equipment, player.Race)
	if err != nil {
		t.Fatal(err)
	}
	return l
}

// loneRequest is testRequest with the target alone in the raid and nothing in its pool to change.
func loneRequest() *proto.OptimizeGearRequest {
	req := testRequest()
	req.Base.Raid.Parties[0].Players = nil
	req.Pool.Slots, req.Pool.GemIds = nil, nil
	return req
}

func hasWarning(result *proto.OptimizerResult, part string) bool {
	return slices.ContainsFunc(result.Warnings, func(w string) bool { return strings.Contains(w, part) })
}

func TestOptimizeWithNothingToChange(t *testing.T) {
	req := loneRequest()
	var progress []*proto.OptimizerProgress
	result := Optimize(context.Background(), req, func(p *proto.OptimizerProgress) {
		progress = append(progress, p)
	})

	if result.ErrorResult != "" {
		t.Fatal(result.ErrorResult)
	}
	want := seedOf(t, req)
	for name, loadout := range map[string]*proto.OptimizerLoadoutResult{"best": result.Best, "seed": result.Seed} {
		got, err := LoadoutFromProto(loadout.GetEquipment(), loadout.GetRacialTraits())
		if err != nil {
			t.Fatal(err)
		}
		if got != want {
			t.Errorf("%s = %+v, want the seed %+v", name, got, want)
		}
	}
	if result.Improved || result.Cancelled || result.TotalSims != 0 {
		t.Errorf("improved = %v, cancelled = %v, %d sims; want none of them", result.Improved, result.Cancelled, result.TotalSims)
	}
	if result.TargetRaidIndex != 6 || result.CatalogDate != "2026-09-18" || result.SimCommit == "" {
		t.Errorf("target %d, catalog date %q, sim commit %q", result.TargetRaidIndex, result.CatalogDate, result.SimCommit)
	}
	if result.Settings.GetContentPhase() != 1 || result.Settings.GetWorkers() <= 0 {
		t.Errorf("settings = %v, want the request's with defaults", result.Settings)
	}
	if !hasWarning(result, "nothing to change") {
		t.Errorf("warnings = %q, want one saying there's nothing to change", result.Warnings)
	}
	// it returns before the objective, so there's nothing to score a racial screen against
	if len(result.RacialScreen) != 0 {
		t.Errorf("racial screen = %v, want none", result.RacialScreen)
	}
	if len(progress) == 0 || progress[0].Stage != stages[0] {
		t.Fatalf("progress = %v, want it to start at %q", progress, stages[0])
	}
	if p := progress[0]; p.CompletedSteps != 1 || p.TotalSteps != int32(len(stages)) {
		t.Errorf("setup reads step %d of %d, want 1 of %d", p.CompletedSteps, p.TotalSteps, len(stages))
	}
}

// Steps count the stage underway, from 1 at Setup to all of them at the last stage.
func TestProgressSteps(t *testing.T) {
	r, err := PrepareRequest(kaRequest(proto.OptimizerEffort_OptimizerEffortQuick))
	if err != nil {
		t.Fatal(err)
	}
	steps := map[string]int32{}
	result := optimize(context.Background(), r, r, newKnownEvaluator(kaMetrics), func(p *proto.OptimizerProgress) {
		if p.TotalSteps != int32(len(stages)) {
			t.Errorf("%s: %d total steps, want %d", p.Stage, p.TotalSteps, len(stages))
		}
		steps[p.Stage] = p.CompletedSteps
	}, time.Now())
	if result.ErrorResult != "" {
		t.Fatal(result.ErrorResult)
	}
	for i, stage := range stages {
		if got, ok := steps[stage]; !ok || got != int32(i+1) {
			t.Errorf("%s reads step %d (reported: %v), want %d", stage, got, ok, i+1)
		}
	}
}

func TestOptimizeCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	req := testRequest()
	result := Optimize(ctx, req, nil)
	if result.ErrorResult != "" || !result.Cancelled || result.Best == nil {
		t.Fatalf("cancelled run = %v, want the seed with cancelled set", result)
	}
	if got, err := LoadoutFromProto(result.Best.Equipment, result.Best.RacialTraits); err != nil || got != seedOf(t, req) {
		t.Errorf("best = %+v (%v), want the seed", got, err)
	}
}

// A cancel in the middle of a run still returns a result: the seed, until a pick passes verification.
func TestOptimizeCancelledWhileSimming(t *testing.T) {
	r, err := PrepareRequest(kaRequest(proto.OptimizerEffort_OptimizerEffortQuick))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	result := optimize(ctx, r, r, newKnownEvaluator(kaMetrics), func(p *proto.OptimizerProgress) {
		if p.Stage == "Effects" {
			cancel()
		}
	}, time.Now())
	if result.ErrorResult != "" || !result.Cancelled || result.Best == nil || result.Seed == nil {
		t.Fatalf("result = %v, want a cancelled one with best and seed", result)
	}
	if result.Improved {
		t.Error("a run cancelled before verification can't have improved")
	}
}

func TestOptimizeBadRequest(t *testing.T) {
	req := testRequest()
	req.TargetRaidIndex = 5
	result := Optimize(context.Background(), req, nil)
	if result.ErrorResult == "" || result.Best != nil || result.SimCommit == "" {
		t.Errorf("bad request = %v, want an error result", result)
	}

	// testRequest's other raider has no spec, so deriving what it gives fails
	result = Optimize(context.Background(), testRequest(), nil)
	if !strings.Contains(result.ErrorResult, "deriving raid index 6") {
		t.Errorf("error_result = %q, want the failed derive", result.ErrorResult)
	}
}

// A raid-DPS run with nothing to change still returns cleanly, and the pick's raid_dps_delta is
// exactly 0 against itself.
func TestOptimizeRaidDpsWithNothingToChange(t *testing.T) {
	req := loneRequest()
	req.Settings.Objective = proto.OptimizerObjective_OptimizerObjectiveRaidDps
	result := Optimize(context.Background(), req, nil)
	if result.ErrorResult != "" {
		t.Fatal(result.ErrorResult)
	}
	if result.Improved {
		t.Error("nothing was in the pool to change")
	}
	if result.Best.RaidDpsDelta != 0 || result.Best.RaidDpsDeltaSe != 0 {
		t.Errorf("best raid_dps_delta = %v ± %v, want 0", result.Best.RaidDpsDelta, result.Best.RaidDpsDeltaSe)
	}
	if result.Seed.RaidDpsDelta != 0 || result.Seed.RaidDpsDeltaSe != 0 {
		t.Errorf("seed raid_dps_delta = %v ± %v, want 0", result.Seed.RaidDpsDelta, result.Seed.RaidDpsDeltaSe)
	}
}

// A raid batch sims the target alone in its derived context; a lone target keeps its own buffs.
func TestSimmedRequest(t *testing.T) {
	lone := presetRequest(t, "fury_p1")
	if simmed, err := simmedRequest(lone); err != nil || simmed != lone {
		t.Errorf("lone target: simmed %p (%v), want the request itself %p", simmed, err, lone)
	}

	req := presetOptimizeRequest(t, "fury_p1")
	mage := presetOptimizeRequest(t, "arcane_p3").Base.Raid.Parties[0].Players[0]
	req.Base.Raid.Parties[0].Players = []*proto.Player{mage, req.Base.Raid.Parties[0].Players[0]}
	req.Base.Raid.Buffs = &proto.RaidBuffs{}
	req.TargetRaidIndex = 1
	r, err := PrepareRequest(req)
	if err != nil {
		t.Fatal(err)
	}
	simmed, err := simmedRequest(r)
	if err != nil {
		t.Fatal(err)
	}
	if simmed.TargetIndex != raidctx.TargetIndex || simmed.Target().Name != "Fury" || simmed.Seed != r.Seed {
		t.Errorf("simmed target %d %q, want the fury warrior at %d with the same seed", simmed.TargetIndex, simmed.Target().Name, raidctx.TargetIndex)
	}
	if len(simmed.Base.Raid.Parties[0].Players) != 1 || !simmed.Base.Raid.Buffs.GetArcaneBrilliance() {
		t.Errorf("derived first party %v, raid buffs %v; want the target alone with the mage's Arcane Brilliance",
			simmed.Base.Raid.Parties[0].Players, simmed.Base.Raid.Buffs)
	}
	if r.Base.Raid.Parties[0].Players[0].Name != "Arcane" {
		t.Error("deriving changed the prepared request")
	}
}

func TestRunAsync(t *testing.T) {
	req := loneRequest()
	reporter := make(chan *proto.ProgressMetrics, 10)
	RunAsync(context.Background(), req, reporter)

	var got []*proto.ProgressMetrics
	timeout := time.After(10 * time.Second)
	for done := false; !done; {
		select {
		case p, ok := <-reporter:
			if !ok {
				done = true
				break
			}
			got = append(got, p)
		case <-timeout:
			t.Fatal("RunAsync never closed its channel")
		}
	}

	if len(got) < 2 {
		t.Fatalf("got %d messages, want progress then the final result", len(got))
	}
	last := got[len(got)-1]
	if last.FinalOptimizeResult == nil || last.FinalOptimizeResult.ErrorResult != "" {
		t.Fatalf("last message = %v, want a final result", last)
	}
	for _, p := range got[:len(got)-1] {
		if p.OptimizerProgress == nil || p.FinalOptimizeResult != nil {
			t.Errorf("progress message = %v", p)
		}
	}
	if !goproto.Equal(last.FinalOptimizeResult.Best.Equipment, Optimize(context.Background(), req, nil).Best.Equipment) {
		t.Error("RunAsync's result differs from Optimize's")
	}
}
