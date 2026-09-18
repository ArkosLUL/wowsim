package main

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
	"github.com/wowsims/wotlk/sim/optimizer"
	googleProto "google.golang.org/protobuf/proto"
)

func newTestServer(t *testing.T, runOptimizer func(context.Context, *proto.OptimizeGearRequest, chan *proto.ProgressMetrics)) *httptest.Server {
	return serveTest(t, &server{
		asyncProgresses: map[string]*asyncProgress{},
		runOptimizer:    runOptimizer,
	})
}

func serveTest(t *testing.T, s *server) *httptest.Server {
	mux := http.NewServeMux()
	s.setupAsyncServer(mux)
	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)
	return ts
}

func post(t *testing.T, ts *httptest.Server, route string, msg googleProto.Message) (int, []byte) {
	t.Helper()
	body, err := googleProto.Marshal(msg)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.Post(ts.URL+route, "application/x-protobuf", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	out, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	return resp.StatusCode, out
}

func startOptimizer(t *testing.T, ts *httptest.Server, req *proto.OptimizeGearRequest) (int, string) {
	t.Helper()
	status, body := post(t, ts, "/optimizeGearAsync", req)
	started := &proto.AsyncAPIResult{}
	if err := googleProto.Unmarshal(body, started); err != nil || started.ProgressId == "" {
		t.Fatalf("status %d, body %q isn't an AsyncAPIResult: %v", status, body, err)
	}
	return status, started.ProgressId
}

// pollProgress polls /asyncProgress, like net_worker.js does, until until() is happy with a reply.
func pollProgress(t *testing.T, ts *httptest.Server, progressID string, until func(*proto.ProgressMetrics) bool) *proto.ProgressMetrics {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		status, body := post(t, ts, "/asyncProgress", &proto.AsyncAPIResult{ProgressId: progressID})
		if status != http.StatusOK {
			t.Fatalf("/asyncProgress answered %d", status)
		}
		progress := &proto.ProgressMetrics{}
		if err := googleProto.Unmarshal(body, progress); err != nil {
			t.Fatal(err)
		}
		if until(progress) {
			return progress
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("progress %s never got there", progressID)
	return nil
}

func TestOptimizeGearAsync(t *testing.T) {
	ts := newTestServer(t, nil)
	req := &proto.OptimizeGearRequest{
		Base: &proto.RaidSimRequest{
			Raid: core.SinglePlayerRaidProto(
				&proto.Player{
					Race:      proto.Race_RaceTroll,
					Class:     proto.Class_ClassShaman,
					Equipment: p1Equip,
					Spec:      basicSpec,
				},
				&proto.PartyBuffs{},
				&proto.RaidBuffs{},
				&proto.Debuffs{}),
			Encounter:  &proto.Encounter{Duration: 120, Targets: []*proto.Target{{}}},
			SimOptions: &proto.SimOptions{Iterations: 100, RandomSeed: 1},
		},
		Settings: &proto.OptimizerSettings{ContentPhase: 1, Workers: 1000},
		Pool:     &proto.CandidatePool{CatalogDate: "2026-09-18"},
	}

	status, id := startOptimizer(t, ts, req)
	if status != http.StatusOK {
		t.Fatalf("/optimizeGearAsync answered %d", status)
	}
	result := pollProgress(t, ts, id, isFinalProgress).FinalOptimizeResult
	if result == nil || result.ErrorResult != "" {
		t.Fatalf("final result = %v", result)
	}

	want, err := optimizer.LoadoutFromProto(p1Equip, proto.Race_RaceTroll)
	if err != nil {
		t.Fatal(err)
	}
	got, err := optimizer.LoadoutFromProto(result.Best.GetEquipment(), result.Best.GetRacialTraits())
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Errorf("best = %+v, want the seed %+v", got, want)
	}
	if wantWorkers := int32(max(1, runtime.GOMAXPROCS(0)-1)); result.Settings.GetWorkers() != wantWorkers {
		t.Errorf("workers = %d, want GOMAXPROCS - 1 = %d", result.Settings.GetWorkers(), wantWorkers)
	}
	if result.CatalogDate != "2026-09-18" {
		t.Errorf("catalog date = %q", result.CatalogDate)
	}

	if status, _ := post(t, ts, "/asyncProgress", &proto.AsyncAPIResult{ProgressId: id}); status != http.StatusNoContent {
		t.Errorf("a fetched final result should be gone, got %d", status)
	}
}

// blockingOptimizer reports once, then waits for a cancel, like a long search would.
func blockingOptimizer(started *sync.WaitGroup) func(context.Context, *proto.OptimizeGearRequest, chan *proto.ProgressMetrics) {
	return func(ctx context.Context, _ *proto.OptimizeGearRequest, reporter chan *proto.ProgressMetrics) {
		started.Done()
		go func() {
			defer close(reporter)
			reporter <- &proto.ProgressMetrics{OptimizerProgress: &proto.OptimizerProgress{Stage: "Search"}}
			<-ctx.Done()
			reporter <- &proto.ProgressMetrics{FinalOptimizeResult: &proto.OptimizerResult{Cancelled: true}}
		}()
	}
}

func TestOptimizeGearAsyncOneAtATime(t *testing.T) {
	var started sync.WaitGroup
	ts := newTestServer(t, blockingOptimizer(&started))
	req := &proto.OptimizeGearRequest{}

	started.Add(1)
	status, first := startOptimizer(t, ts, req)
	if status != http.StatusOK {
		t.Fatalf("first run answered %d", status)
	}
	started.Wait()
	pollProgress(t, ts, first, func(p *proto.ProgressMetrics) bool { return p.OptimizerProgress != nil })

	status, refused := startOptimizer(t, ts, req)
	if status != http.StatusConflict {
		t.Errorf("second run answered %d, want 409", status)
	}
	refusal := pollProgress(t, ts, refused, isFinalProgress).FinalOptimizeResult
	if !strings.Contains(refusal.GetErrorResult(), "already running") || !strings.Contains(refusal.GetErrorResult(), first) {
		t.Errorf("second run's error = %q, want one naming the running optimization", refusal.GetErrorResult())
	}

	if status, _ := post(t, ts, "/cancelAsync", &proto.AsyncAPIResult{ProgressId: "nope"}); status != http.StatusNotFound {
		t.Errorf("cancelling an unknown id answered %d, want 404", status)
	}
	if status, _ := post(t, ts, "/cancelAsync", &proto.AsyncAPIResult{ProgressId: first}); status != http.StatusOK {
		t.Fatalf("cancel answered %d", status)
	}
	if result := pollProgress(t, ts, first, isFinalProgress).FinalOptimizeResult; !result.Cancelled {
		t.Errorf("cancelled run's result = %v", result)
	}

	// seeing the final result means the slot is free again
	started.Add(1)
	status, third := startOptimizer(t, ts, req)
	if status != http.StatusOK {
		t.Fatalf("run after the cancel answered %d", status)
	}
	started.Wait()
	post(t, ts, "/cancelAsync", &proto.AsyncAPIResult{ProgressId: third})
	pollProgress(t, ts, third, isFinalProgress)
}

func TestOptimizeGearAsyncCancelsAbandonedRun(t *testing.T) {
	var started sync.WaitGroup
	ts := serveTest(t, &server{
		asyncProgresses:       map[string]*asyncProgress{},
		runOptimizer:          blockingOptimizer(&started),
		optimizerAbandonAfter: 50 * time.Millisecond,
	})
	req := &proto.OptimizeGearRequest{}

	started.Add(1)
	status, abandoned := startOptimizer(t, ts, req)
	if status != http.StatusOK {
		t.Fatalf("first run answered %d", status)
	}
	started.Wait()

	// nobody polls the first run, so the slot frees up once the watchdog cancels it
	var next string
	for deadline := time.Now().Add(10 * time.Second); next == ""; time.Sleep(10 * time.Millisecond) {
		if time.Now().After(deadline) {
			t.Fatal("the abandoned run still holds the slot")
		}
		started.Add(1)
		if status, id := startOptimizer(t, ts, req); status == http.StatusOK {
			next = id
		} else {
			started.Done()
			pollProgress(t, ts, id, isFinalProgress)
		}
	}
	started.Wait()

	if result := pollProgress(t, ts, abandoned, isFinalProgress).FinalOptimizeResult; !result.Cancelled {
		t.Errorf("abandoned run's result = %v, want it kept and marked cancelled", result)
	}
	post(t, ts, "/cancelAsync", &proto.AsyncAPIResult{ProgressId: next})
	pollProgress(t, ts, next, isFinalProgress)
}
