//go:build optimizer_slow

package optimizer

import (
	"context"
	"testing"

	"github.com/wowsims/wotlk/sim/core/proto"
	goproto "google.golang.org/protobuf/proto"
)

// Fury P1 Normal over the slow suite's pool, and over the same pool with nothing cut, then both picks
// in one independent paired check like the slow suite's. The cut only drops items neither pick
// wears, so the cut pool's pick shouldn't score under the uncut one's. Run it with:
//
//	tools/acore/dock.sh exec go test --tags=with_db,optimizer_slow -count=1 -timeout 30m -run TestCurvesPoolCut -v ./sim/optimizer/
//
// Its name stays clear of -run TestOptimizerSlow, which would run it with the suite.
func TestCurvesPoolCut(t *testing.T) {
	c := slowCases[0]
	effort := proto.OptimizerEffort_OptimizerEffortNormal
	if *decisions {
		traceHook = t.Logf
		defer func() { traceHook = nil }()
	}
	cutReq := slowRequest(t, c, effort)
	uncutReq := curvesUncutRequest(t, c, effort)
	var picks []Loadout
	for _, req := range []*proto.OptimizeGearRequest{uncutReq, cutReq} {
		size := 0
		for _, sp := range req.Pool.Slots {
			size += len(sp.ItemIds)
		}
		t.Logf("optimizing over %d candidates", size)
		result := Optimize(context.Background(), req, nil)
		if result.ErrorResult != "" {
			t.Fatal(result.ErrorResult)
		}
		best, err := LoadoutFromProto(result.Best.Equipment, result.Best.RacialTraits)
		if err != nil {
			t.Fatal(err)
		}
		t.Logf("%d candidates: pick %v, sims=%d", size, setKey(best), result.TotalSims)
		picks = append(picks, best)
	}

	check := goproto.Clone(cutReq).(*proto.OptimizeGearRequest)
	check.Base.SimOptions.RandomSeed = 9001
	r, err := PrepareRequest(check)
	if err != nil {
		t.Fatal(err)
	}
	keep, _ := WeightedMetrics(r.Settings)
	eval := NewSimEvaluator(r, keep...)
	obj, err := NewObjective(context.Background(), eval, r, slowCheckIterations)
	if err != nil {
		t.Fatal(err)
	}
	points := []Point{{Loadout: r.Equipped}, {Loadout: picks[0]}, {Loadout: picks[1]}}
	evals, err := eval.Evaluate(context.Background(), points, slowCheckIterations)
	if err != nil {
		t.Fatal(err)
	}
	uncut, cut := obj.Delta(evals[0], evals[1]), obj.Delta(evals[0], evals[2])
	t.Logf("J_preset=%.1f uncut delta=%+.1f±%.1f cut delta=%+.1f±%.1f", obj.Score(evals[0]).Mean, uncut.Mean, uncut.SE, cut.Mean, cut.SE)
	if d := obj.Delta(evals[1], evals[2]); d.Mean < -2*d.SE {
		t.Errorf("the cut pool's pick scores %.1f ± %.1f under the uncut pool's", -d.Mean, d.SE)
	}
}

// curvesUncutRequest is slowRequest with every item the realistic pool offers, outclassed or not.
func curvesUncutRequest(tb testing.TB, c slowCase, effort proto.OptimizerEffort) *proto.OptimizeGearRequest {
	req := slowRequest(tb, c, effort)
	player := c.player(tb)
	req.Pool = realisticPool(tb, player, c.phase, nil)
	trimSeedToPool(tb, player, req.Pool)
	req.Base.Raid.Parties[0].Players[0].Equipment = player.Equipment
	return req
}
