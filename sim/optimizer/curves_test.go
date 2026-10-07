package optimizer

import (
	"context"
	"math"
	"math/rand"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
	"github.com/wowsims/wotlk/sim/core/stats"
)

// Made-up items for the lattice tests: in each of six slots a strength piece and a haste piece, and
// on the feet a strength piece and a haste piece that never beats it.
const (
	cvStrength  = 9731000 // plus the slot's index in cvSlots
	cvHaste     = 9732000
	cvFeet      = 9733001
	cvFeetHaste = 9733002
)

var cvSlots = []proto.ItemType{proto.ItemType_ItemTypeHead, proto.ItemType_ItemTypeShoulder, proto.ItemType_ItemTypeChest,
	proto.ItemType_ItemTypeWrist, proto.ItemType_ItemTypeHands, proto.ItemType_ItemTypeLegs}

var cvOnce sync.Once

func cvRegister() {
	cvOnce.Do(func() {
		db := &proto.SimDatabase{}
		for i, t := range cvSlots {
			// the strength pieces differ a little, so one set of haste slots is best
			db.Items = append(db.Items,
				kaItem(cvStrength+int32(i), t, stats.Stats{stats.Strength: 31 + 0.1*float64(i)}, ""),
				kaItem(cvHaste+int32(i), t, stats.Stats{stats.MeleeHaste: 45}, ""))
		}
		db.Items = append(db.Items,
			kaItem(cvFeet, proto.ItemType_ItemTypeFeet, stats.Stats{stats.Strength: 300}, ""),
			kaItem(cvFeetHaste, proto.ItemType_ItemTypeFeet, stats.Stats{stats.MeleeHaste: 1000}, ""))
		core.AddToDatabase(db)
	})
}

// cvMetrics is linear in strength and attack power, and haste fades, so the best loadout holds a few
// haste pieces and the curve between knots bends.
func cvMetrics(p Point) (Metrics, error) {
	g := gearStats(p.Loadout).Add(p.Offset)
	return Metrics{MetricDPS: 3000 + g[stats.AttackPower] + 2*g[stats.Strength] + 600*(1-math.Exp(-g[stats.MeleeHaste]/250))}, nil
}

// cvRequest starts from every strength piece, at Quick. With feetHaste the pool also offers the feet's
// haste piece, which stretches haste's range from 270 to 1270 though no good loadout wears it.
func cvRequest(feetHaste bool) *proto.OptimizeGearRequest {
	cvRegister()
	seed := map[proto.ItemSlot]*proto.ItemSpec{proto.ItemSlot_ItemSlotFeet: {Id: cvFeet}}
	var slots []*proto.SlotPool
	for i, t := range cvSlots {
		slot := core.ItemTypeToSlot(t)
		seed[slot] = &proto.ItemSpec{Id: cvStrength + int32(i)}
		slots = append(slots, &proto.SlotPool{Slot: slot, ItemIds: []int32{cvStrength + int32(i), cvHaste + int32(i)}})
	}
	feet := &proto.SlotPool{Slot: proto.ItemSlot_ItemSlotFeet, ItemIds: []int32{cvFeet}}
	if feetHaste {
		feet.ItemIds = append(feet.ItemIds, cvFeetHaste)
	}
	target := &proto.Player{
		Name:          "Target",
		Race:          proto.Race_RaceOrc,
		Class:         proto.Class_ClassWarrior,
		TalentsString: rtFuryNoTG,
		Equipment:     testEquipment(seed),
		Spec:          &proto.Player_Warrior{Warrior: &proto.Warrior{Options: &proto.Warrior_Options{}}},
		Rotation:      &proto.APLRotation{Type: proto.APLRotation_TypeAPL},
	}
	return &proto.OptimizeGearRequest{
		Base: &proto.RaidSimRequest{
			Raid:       &proto.Raid{Parties: []*proto.Party{{Players: []*proto.Player{target}}}},
			Encounter:  &proto.Encounter{Duration: 180, Targets: []*proto.Target{{}}},
			SimOptions: &proto.SimOptions{RandomSeed: 7},
		},
		Settings: &proto.OptimizerSettings{ContentPhase: 1, Effort: proto.OptimizerEffort_OptimizerEffortQuick,
			RacialMode: proto.OptimizerRacialMode_OptimizerRacialKeepCurrent, DisableStatCaps: true},
		Pool: &proto.CandidatePool{Slots: append(slots, feet), CatalogDate: "2026-10-07"},
	}
}

// cvSearchRun takes req's run through the search on the fake, and returns the curves and the
// candidates, best first.
func cvSearchRun(t *testing.T, req *proto.OptimizeGearRequest) (Response, []Loadout) {
	t.Helper()
	r, err := PrepareRequest(req)
	if err != nil {
		t.Fatal(err)
	}
	pool, err := CompilePool(r)
	if err != nil {
		t.Fatal(err)
	}
	rn := &run{
		ctx:    context.Background(),
		asked:  r,
		r:      r,
		budget: EffortBudget(r.Settings.GetEffort()),
		rng:    rand.New(rand.NewSource(1)),
		pool:   pool,
		best:   r.Seed,
		ref:    r.Equipped,
	}
	rn.total = int64(rn.budget.Evaluations) * int64(rn.budget.Iterations)
	rn.screen = roundShards(rn.budget.Iterations / 4)
	rn.eval = &countingEvaluator{inner: newKnownEvaluator(cvMetrics), have: map[Point]int{}}
	if rn.obj, err = NewObjective(rn.ctx, rn.eval, r, rn.budget.Iterations); err != nil {
		t.Fatal(err)
	}
	evals, err := rn.evaluate([]Point{{Loadout: r.Seed}}, rn.budget.Iterations)
	if err != nil {
		t.Fatal(err)
	}
	rn.seedEval, rn.bestEval = evals[0], evals[0]
	rn.seedJ = rn.obj.Score(rn.seedEval)
	resp, rangeStats, err := rn.measureCurves(rn.newStatScreen())
	if err != nil {
		t.Fatal(err)
	}
	s := newSurrogate(pool, r.Seed, resp)
	if err := s.setFloorsAndCeilings(nil, nil, rangeStats); err != nil {
		t.Fatal(err)
	}
	if err := rn.measureEffects(s); err != nil {
		t.Fatal(err)
	}
	candidates, err := rn.searchRaces(s)
	if err != nil {
		t.Fatal(err)
	}
	if len(candidates) == 0 {
		t.Fatal("the search found nothing")
	}
	return resp, candidates
}

// The feet's haste piece stretches haste's range from 270 to 1270, though no good loadout wears it.
// Haste's knots up to 270 stay where they were, and the search's pick with them.
func TestCurvesIgnoreAnUnusedExtremeItem(t *testing.T) {
	without, withoutPicks := cvSearchRun(t, cvRequest(false))
	with, withPicks := cvSearchRun(t, cvRequest(true))
	if withoutPicks[0] != withPicks[0] {
		t.Errorf("the pick moved with the feet's haste piece in the pool:\n without %v\n with    %v", setKey(withoutPicks[0]), setKey(withPicks[0]))
	}
	hasteWithout, hasteWith := without[stats.MeleeHaste], with[stats.MeleeHaste]
	if hasteWithout == nil || hasteWith == nil {
		t.Fatal("no haste curve")
	}
	for _, k := range hasteWithout.Knots {
		// the fake's own noise per point depends on the order points first come up, so it isn't exact
		if !slices.ContainsFunc(hasteWith.Knots, func(o Knot) bool { return o.Offset == k.Offset && near(o.Delta.Mean, k.Delta.Mean, 0.1) }) {
			t.Errorf("haste knot %+v is gone with the feet's haste piece in the pool: %v", k, hasteWith.Knots)
		}
	}
	if last := hasteWith.Knots[len(hasteWith.Knots)-1]; last.Offset < 1270 {
		t.Errorf("the haste curve stops at %g, short of the feet's haste piece", last.Offset)
	}
}

// Hit caps at +40, then is worth less up to +2000, far past the +300 the pool reaches. The far bend
// sits on long segments whose slopes the sims pin down tightly, so it scores far more standard
// errors than the cap, which still has to get the bisection.
func TestCurvesBisectACapInRange(t *testing.T) {
	fake := newFakeEvaluator(func(p Point) Metrics {
		hit := p.Offset[stats.MeleeHit]
		return Metrics{MetricDPS: 5000 + 3*min(hit, 40) + max(min(hit, 2000)-40, 0)}
	})
	resp, err := MeasureResponse(context.Background(), fake, dpsObjective(), Loadout{}, map[stats.Stat]ResponseRange{
		stats.MeleeHit: {Min: -100, Max: 300, Unit: 16, Ratio: 4},
	}, 1000)
	if err != nil {
		t.Fatal(err)
	}
	hit := resp[stats.MeleeHit]
	if hit == nil || !near(hit.Breakpoint, 40, 2) || hit.SlopeAbove >= hit.SlopeBelow {
		t.Errorf("hit curve = %+v, want the break at 40, flatter above it", hit)
	}
}

// A range with offsets but no lattice is an error, not an endless loop.
func TestMeasureResponseNeedsALattice(t *testing.T) {
	fake := newFakeEvaluator(func(p Point) Metrics { return Metrics{MetricDPS: 5000} })
	_, err := MeasureResponse(context.Background(), fake, dpsObjective(), Loadout{}, map[stats.Stat]ResponseRange{
		stats.Strength: {Min: -10, Max: 10},
	}, 100)
	if err == nil || !strings.Contains(err.Error(), "lattice") {
		t.Errorf("err = %v, want one about the missing lattice", err)
	}
}

// A lattice runs out to its first knot at or past Max, and down to Min, skipping a knot within a
// quarter of itself of Min.
func TestInitialOffsetsOnTheLattice(t *testing.T) {
	for _, tc := range []struct {
		stat stats.Stat
		rng  ResponseRange
		want []float64
	}{
		{stats.Strength, ResponseRange{Min: -632, Max: 1280, Unit: 64, Ratio: 4}, []float64{64, 256, 1024, 4096, -64, -256, -632}},
		{stats.Strength, ResponseRange{Min: -632, Max: 1255, Unit: 64, Ratio: 4}, []float64{64, 256, 1024, 4096, -64, -256, -632}},
		// a cap-prone stat goes one knot further
		{stats.MeleeHit, ResponseRange{Min: -141, Max: 1603, Unit: 32, Ratio: 4}, []float64{32, 128, 512, 2048, 8192, -32, -141}},
		{stats.MeleeHaste, ResponseRange{Min: 0, Max: 270, Unit: 64, Ratio: 8}, []float64{64, 512}},
		{stats.Expertise, ResponseRange{Min: -40, Max: 0, Unit: 32, Ratio: 8}, []float64{-32, -40}},
	} {
		if got := initialOffsets(tc.stat, tc.rng); !slices.Equal(got, tc.want) {
			t.Errorf("%s %+v: offsets %v, want %v", tc.stat.StatName(), tc.rng, got, tc.want)
		}
	}
}

// A lattice's unit is about what one of the seed's slots holds of a stat, else what a candidate
// holding any does, and Quick's starts twice as far out and spreads twice as fast.
func TestKnotLattice(t *testing.T) {
	for _, effort := range []proto.OptimizerEffort{proto.OptimizerEffort_OptimizerEffortNormal, proto.OptimizerEffort_OptimizerEffortQuick} {
		req := cvRequest(true)
		req.Settings.Effort = effort
		r, err := PrepareRequest(req)
		if err != nil {
			t.Fatal(err)
		}
		pool, err := CompilePool(r)
		if err != nil {
			t.Fatal(err)
		}
		rn := &run{asked: r, r: r, pool: pool}
		units, ratio := rn.knotLattice(newSurrogate(pool, r.Seed, nil))
		// the seed's slots hold 31 strength each and the feet 300; no seed slot holds haste, and the
		// candidates holding any mostly hold 45
		want, wantRatio := stats.Stats{stats.Strength: 32, stats.MeleeHaste: 32, stats.Agility: minKnotUnit}, 4.0
		if effort == proto.OptimizerEffort_OptimizerEffortQuick {
			want, wantRatio = want.Multiply(2), 8
		}
		for _, st := range []stats.Stat{stats.Strength, stats.MeleeHaste, stats.Agility} {
			if units[st] != want[st] {
				t.Errorf("%s: %s unit %g, want %g", effort, st.StatName(), units[st], want[st])
			}
		}
		if ratio != wantRatio {
			t.Errorf("%s: ratio %g, want %g", effort, ratio, wantRatio)
		}
	}
}
