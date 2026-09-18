package optimizer

import (
	"context"
	"testing"
	"time"

	"github.com/wowsims/wotlk/sim/core/proto"
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

func TestOptimizeReturnsSeed(t *testing.T) {
	req := testRequest()
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
	if result.Improved || result.Cancelled {
		t.Errorf("improved = %v, cancelled = %v; the stub should do neither", result.Improved, result.Cancelled)
	}
	if result.TargetRaidIndex != 6 || result.CatalogDate != "2026-09-18" || result.SimCommit == "" {
		t.Errorf("target %d, catalog date %q, sim commit %q", result.TargetRaidIndex, result.CatalogDate, result.SimCommit)
	}
	if result.Settings.GetContentPhase() != 1 || result.Settings.GetWorkers() <= 0 {
		t.Errorf("settings = %v, want the request's with defaults", result.Settings)
	}
	if len(progress) == 0 {
		t.Error("no progress reported")
	}
}

func TestOptimizeCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	result := Optimize(ctx, testRequest(), nil)
	if result.ErrorResult != "" || !result.Cancelled || result.Best == nil {
		t.Errorf("cancelled run = %v, want the seed with cancelled set", result)
	}
}

func TestOptimizeBadRequest(t *testing.T) {
	req := testRequest()
	req.TargetRaidIndex = 5
	result := Optimize(context.Background(), req, nil)
	if result.ErrorResult == "" || result.Best != nil || result.SimCommit == "" {
		t.Errorf("bad request = %v, want an error result", result)
	}
}

func TestRunAsync(t *testing.T) {
	req := testRequest()
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
