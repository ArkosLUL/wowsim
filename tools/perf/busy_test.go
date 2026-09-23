package main

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/wowsims/wotlk/sim"
	"github.com/wowsims/wotlk/sim/core/proto"
	"github.com/wowsims/wotlk/sim/optimizer"
	"google.golang.org/protobuf/encoding/protojson"
)

func init() {
	sim.RegisterAll()
}

func TestSimBusyTime(t *testing.T) {
	data, err := os.ReadFile("../../sim/optimizer/testdata/search/fury_p1.json")
	if err != nil {
		t.Fatal(err)
	}
	req := &proto.OptimizeGearRequest{}
	if err := protojson.Unmarshal(data, req); err != nil {
		t.Fatal(err)
	}
	r, err := optimizer.PrepareRequest(req)
	if err != nil {
		t.Fatal(err)
	}
	eval := optimizer.NewSimEvaluator(r)

	before := optimizer.SimBusyTime()
	start := time.Now()
	if _, err := eval.Evaluate(context.Background(), []optimizer.Point{{Loadout: r.Seed}}, 2*optimizer.DefaultShardIterations); err != nil {
		t.Fatal(err)
	}
	wall := time.Since(start)
	// two shards run on two workers at most
	if busy := optimizer.SimBusyTime() - before; busy <= 0 || busy > 2*wall {
		t.Errorf("simming two shards took %v of worker time in %v, want some and at most twice that", busy, wall)
	}
}
