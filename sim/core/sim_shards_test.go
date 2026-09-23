package core

import (
	"math"
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/wowsims/wotlk/sim/core/proto"
	googleProto "google.golang.org/protobuf/proto"
)

func TestSplitIterations(t *testing.T) {
	for _, tc := range []struct {
		iterations int32
		shards     int
		want       []shardSpan
	}{
		{10, 3, []shardSpan{{0, 4}, {4, 3}, {7, 3}}},
		{3000, 15, func() (spans []shardSpan) {
			for k := range int32(15) {
				spans = append(spans, shardSpan{k * 200, 200})
			}
			return spans
		}()},
		{2, 15, []shardSpan{{0, 1}, {1, 1}}},
		{7, 1, []shardSpan{{0, 7}}},
		{5, 0, []shardSpan{{0, 5}}},
		{0, 4, []shardSpan{{0, 0}}},
	} {
		if got := splitIterations(tc.iterations, tc.shards); !slices.Equal(got, tc.want) {
			t.Errorf("splitIterations(%d, %d) = %v, want %v", tc.iterations, tc.shards, got, tc.want)
		}
	}
}

func TestShardRequestSeedsFromTheSpan(t *testing.T) {
	rsr := &proto.RaidSimRequest{SimOptions: &proto.SimOptions{Iterations: 100, RandomSeed: 101, DebugFirstIteration: true}}
	first := shardRequest(rsr, shardSpan{first: 0, iterations: 34})
	later := shardRequest(rsr, shardSpan{first: 34, iterations: 33})
	if got := first.SimOptions; got.Iterations != 34 || got.RandomSeed != 101 || !got.DebugFirstIteration {
		t.Errorf("first shard's options = %v", got)
	}
	if got := later.SimOptions; got.Iterations != 33 || got.RandomSeed != 135 || got.DebugFirstIteration {
		t.Errorf("later shard's options = %v", got)
	}
	if rsr.SimOptions.Iterations != 100 || rsr.SimOptions.RandomSeed != 101 {
		t.Errorf("shardRequest changed the request: %v", rsr.SimOptions)
	}
}

// Ties at the max and the min straddle the split, to check which seed each side keeps.
func TestDistributionMetricsMergeMatchesOneStream(t *testing.T) {
	values := []float64{5, 9, 1, 3, 9, 1, 4}
	sim := &Simulation{
		Options:  &proto.SimOptions{SaveAllValues: true, Iterations: int32(len(values))},
		Duration: time.Second,
		rand:     NewSplitMix(0),
	}
	add := func(d *DistributionMetrics, from, to int) {
		for i := from; i < to; i++ {
			sim.rand.Seed(int64(100 + i))
			d.Total = values[i]
			d.doneIteration(sim)
		}
	}

	oneStream := NewDistributionMetrics()
	add(&oneStream, 0, len(values))
	merged, second := NewDistributionMetrics(), NewDistributionMetrics()
	add(&merged, 0, 3)
	add(&second, 3, len(values))
	merged.merge(&second)

	want, got := oneStream.ToProto(), merged.ToProto()
	if got.MaxSeed != 101 || got.MinSeed != 105 {
		t.Errorf("merged max seed %d, min seed %d, want 101 and 105", got.MaxSeed, got.MinSeed)
	}
	if math.Abs(got.Avg-want.Avg) > 1e-12 || math.Abs(got.Stdev-want.Stdev) > 1e-12 {
		t.Errorf("merged avg %v stdev %v, want %v and %v", got.Avg, got.Stdev, want.Avg, want.Stdev)
	}
	rounded := googleProto.Clone(got).(*proto.DistributionMetrics)
	rounded.Avg, rounded.Stdev = want.Avg, want.Stdev
	if !googleProto.Equal(rounded, want) {
		t.Errorf("merged = %v, want %v", rounded, want)
	}

	empty := NewDistributionMetrics()
	merged.merge(&empty)
	if again := merged.ToProto(); !googleProto.Equal(again, got) {
		t.Errorf("merging an empty distribution changed it: %v", again)
	}
	empty.merge(&merged)
	if into := empty.ToProto(); !googleProto.Equal(into, got) {
		t.Errorf("merging into an empty distribution = %v, want %v", into, got)
	}
}

func testAura(label string, spellID int32, uptimes ...float64) *Aura {
	aura := &Aura{Label: label, metrics: AuraMetrics{ID: ActionID{SpellID: spellID}}}
	for _, uptime := range uptimes {
		aura.metrics.add(uptime)
		aura.metrics.procsSum++
	}
	return aura
}

func TestMergeAuraMetricsByLabel(t *testing.T) {
	// "b" and "c" share a spell id, and o holds its auras in another order plus one more
	at := &auraTracker{auras: []*Aura{testAura("a", 1, 10, 20), testAura("b", 2, 5), testAura("c", 2, 7)}}
	o := &auraTracker{auras: []*Aura{testAura("c", 2, 1), testAura("d", 4, 3), testAura("a", 1, 30)}}
	at.mergeAuraMetrics(o)

	got := map[string]*proto.AuraMetrics{}
	for _, aura := range at.auras {
		got[aura.Label] = aura.metrics.ToProto()
	}
	for label, want := range map[string]struct{ uptime, procs float64 }{
		"a": {20, 1},
		"b": {5, 1},
		"c": {4, 1},
		"d": {3, 1},
	} {
		if m := got[label]; m == nil || m.UptimeSecondsAvg != want.uptime || m.ProcsAvg != want.procs {
			t.Errorf("aura %s = %v, want uptime %v, procs %v", label, m, want.uptime, want.procs)
		}
	}
	if len(at.auras) != 4 {
		t.Errorf("merged tracker has %d auras, want 4", len(at.auras))
	}
}

func TestUnitMetricsMergeResources(t *testing.T) {
	mana, rage := proto.ResourceType_ResourceTypeMana, proto.ResourceType_ResourceTypeRage
	resource := func(spellID int32, kind proto.ResourceType, events int32, gain float64) *ResourceMetrics {
		return &ResourceMetrics{ActionID: ActionID{SpellID: spellID}, Type: kind, Events: events, Gain: gain, ActualGain: gain}
	}
	um := NewUnitMetrics()
	um.resources = []*ResourceMetrics{resource(1, mana, 1, 10), resource(1, mana, 2, 20), resource(1, rage, 3, 30)}
	o := NewUnitMetrics()
	o.resources = []*ResourceMetrics{resource(1, rage, 1, 1), resource(1, mana, 1, 1), resource(1, mana, 1, 2), resource(1, mana, 5, 50), resource(2, mana, 1, 7)}
	um.merge(&o)

	want := []*ResourceMetrics{resource(1, mana, 2, 11), resource(1, mana, 3, 22), resource(1, rage, 4, 31), resource(1, mana, 5, 50), resource(2, mana, 1, 7)}
	if len(um.resources) != len(want) {
		t.Fatalf("merged %d resources, want %d", len(um.resources), len(want))
	}
	for i, r := range um.resources {
		if !googleProto.Equal(r.ToProto(), want[i].ToProto()) {
			t.Errorf("merged resource %d = %v, want %v", i, r.ToProto(), want[i].ToProto())
		}
	}
	um.resources[3].Events++
	if o.resources[3].Events != 5 {
		t.Errorf("the merged resource shares its counts with the other shard's")
	}
}

// every count but UnitIndex set to n, so a field the merge misses shows up
func targetedActionMetrics(unitIndex int32, n int) TargetedActionMetrics {
	tam := TargetedActionMetrics{UnitIndex: unitIndex}
	v := reflect.ValueOf(&tam).Elem()
	for i := range v.NumField() {
		switch field := v.Field(i); {
		case v.Type().Field(i).Name == "UnitIndex":
		case field.CanInt():
			field.SetInt(int64(n))
		case field.CanFloat():
			field.SetFloat(float64(n))
		}
	}
	return tam
}

func TestUnitMetricsMergeActions(t *testing.T) {
	um := NewUnitMetrics()
	um.actions[ActionID{SpellID: 1}] = &ActionMetrics{Targets: []TargetedActionMetrics{targetedActionMetrics(0, 1), targetedActionMetrics(2, 1)}}
	o := NewUnitMetrics()
	o.actions[ActionID{SpellID: 1}] = &ActionMetrics{Targets: []TargetedActionMetrics{targetedActionMetrics(0, 2), targetedActionMetrics(2, 3)}}
	o.actions[ActionID{SpellID: 2}] = &ActionMetrics{IsMelee: true, Targets: []TargetedActionMetrics{targetedActionMetrics(3, 4)}}
	um.merge(&o)

	if got, want := um.actions[ActionID{SpellID: 1}].Targets, []TargetedActionMetrics{targetedActionMetrics(0, 3), targetedActionMetrics(2, 4)}; !slices.Equal(got, want) {
		t.Errorf("merged spell 1 = %+v, want %+v", got, want)
	}
	added := um.actions[ActionID{SpellID: 2}]
	if added == nil || !added.IsMelee || !slices.Equal(added.Targets, []TargetedActionMetrics{targetedActionMetrics(3, 4)}) {
		t.Fatalf("merged spell 2 = %+v", added)
	}
	added.Targets[0].Hits++
	if o.actions[ActionID{SpellID: 2}].Targets[0].Hits != 4 {
		t.Errorf("the merged action shares its targets with the other shard's")
	}
}
