package optimizer

import (
	"context"
	"math"
	"math/rand"
	"sync"
	"testing"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
	"github.com/wowsims/wotlk/sim/core/stats"
)

// Effect trinkets for the unpriced tests, clear of the other tests' ids. uqProc is worth 59 in all,
// just under the seed's 60 attack power trinket and over the pool's best plain one (58); uqWeak is
// worth 15.
const (
	uqProc    = 9700401 // 10 attack power, +49 DPS effect
	uqWeak    = 9700402 // 5 attack power, +10 DPS effect
	uqProcDPS = 49
	uqWeakDPS = 10
)

var uqOnce sync.Once

func uqRegister() {
	uqOnce.Do(func() {
		core.AddToDatabase(&proto.SimDatabase{Items: []*proto.SimItem{
			kaItem(uqProc, proto.ItemType_ItemTypeTrinket, stats.Stats{stats.AttackPower: 10}, ""),
			kaItem(uqWeak, proto.ItemType_ItemTypeTrinket, stats.Stats{stats.AttackPower: 5}, ""),
		}})
		core.NewItemEffect(uqProc, func(core.Agent) {})
		core.NewItemEffect(uqWeak, func(core.Agent) {})
	})
}

func uqMetrics(p Point) (Metrics, error) {
	m, err := kaMetrics(p)
	for _, c := range p.Loadout.Items {
		switch c.ItemID {
		case uqProc:
			m[MetricDPS] += uqProcDPS
		case uqWeak:
			m[MetricDPS] += uqWeakDPS
		}
	}
	return m, err
}

// uqRun is a run at effort that can only change trinket 1, set up the way verify leaves it: the seed
// is the best, at full iterations. The curves price the pool's plain trinkets; the effect trinkets
// went unpriced, the way Quick's screening budget leaves them. extra goes in trinket 1's pool too.
func uqRun(t *testing.T, effort proto.OptimizerEffort, extra ...int32) (*run, *surrogate) {
	t.Helper()
	nbRegister()
	uqRegister()
	req := kaRequest(effort)
	for _, sp := range req.Pool.Slots {
		if sp.Slot == proto.ItemSlot_ItemSlotTrinket1 {
			sp.ItemIds = append(append(nbAlternativesPool(), uqProc, uqWeak), extra...)
		}
	}
	for slot := proto.ItemSlot(0); int(slot) < NumSlots; slot++ {
		if slot != proto.ItemSlot_ItemSlotTrinket1 {
			req.Settings.LockedSlots = append(req.Settings.LockedSlots, slot)
		}
	}
	r, err := PrepareRequest(req)
	if err != nil {
		t.Fatal(err)
	}
	pool, err := CompilePool(r)
	if err != nil {
		t.Fatal(err)
	}
	budget := EffortBudget(effort)
	rn := &run{
		ctx:    context.Background(),
		asked:  r,
		r:      r,
		budget: budget,
		screen: roundShards(budget.Iterations / 4),
		rng:    rand.New(rand.NewSource(1)),
		pool:   pool,
		best:   r.Seed,
	}
	rn.eval = &countingEvaluator{inner: newKnownEvaluator(uqMetrics), have: map[Point]int{}}
	if rn.obj, err = NewObjective(rn.ctx, rn.eval, r, budget.Iterations); err != nil {
		t.Fatal(err)
	}
	evals, err := rn.evaluate([]Point{{Loadout: r.Seed}}, budget.Iterations)
	if err != nil {
		t.Fatal(err)
	}
	rn.seedEval, rn.bestEval = evals[0], evals[0]
	rn.seedJ = rn.obj.Score(rn.seedEval)
	rn.total = rn.eval.simmed() + int64(budget.Evaluations)*int64(budget.Iterations)

	// J is the fake's DPS, a point of attack power apiece
	s := newSurrogate(pool, r.Seed, Response{stats.AttackPower: {Stat: stats.AttackPower, SlopeBelow: 1, SlopeAbove: 1}})
	trinket1 := proto.ItemSlot_ItemSlotTrinket1
	s.unpriced[trinket1][uqProc] = true
	s.unpriced[trinket1][uqWeak] = true
	return rn, s
}

// Quick prices the effect trinket, whose stats alone would rank it under every plain one, and reports
// it as trinket 1's best alternative. It doesn't beat the pick, so the pick stays.
func TestQuickReportsAnUnpricedRunnerUp(t *testing.T) {
	rn, s := uqRun(t, proto.OptimizerEffort_OptimizerEffortQuick)
	seed := rn.best
	alts, verified, err := rn.neighborhood(s, nil)
	if err != nil {
		t.Fatal(err)
	}
	if rn.best != seed || len(verified) != 0 {
		t.Fatalf("the pick moved %d times, to trinket 1 %d", len(verified), rn.best.Items[proto.ItemSlot_ItemSlotTrinket1].ItemID)
	}
	if len(alts) == 0 || alts[0].Item.GetId() != uqProc {
		t.Fatalf("trinket 1's alternatives are %v, want %d first", uqAltIDs(alts), uqProc)
	}
	if d := alts[0].ScoreDelta; d > 0 || d < -2 {
		t.Errorf("trinket %d scores %+.1f against the pick, want about -1", uqProc, d)
	}
}

// An allowance that covers one sim prices the unpriced item with the better stats, and the run's
// budget grows by its cost, so the race after it has what it had.
func TestQuickPricesBestStatsFirst(t *testing.T) {
	rn, s := uqRun(t, proto.OptimizerEffort_OptimizerEffortQuick)
	old := unpricedShare
	defer func() { unpricedShare = old }()
	// the family's base and one variant, which rounding mustn't cut
	unpricedShare = float64(2*rn.screen+1) / float64(rn.total)
	left := rn.remaining()
	if err := rn.priceUnpriced(s); err != nil {
		t.Fatal(err)
	}
	if rn.remaining() != left {
		t.Errorf("pricing left %d of the budget, want the %d it found", rn.remaining(), left)
	}
	trinket1 := proto.ItemSlot_ItemSlotTrinket1
	if s.unpriced[trinket1][uqProc] || !s.unpriced[trinket1][uqWeak] {
		t.Errorf("trinket %d unpriced %v, trinket %d unpriced %v: want only %d priced", uqProc, s.unpriced[trinket1][uqProc], uqWeak, s.unpriced[trinket1][uqWeak], uqProc)
	}
}

// An unpriced item whose stats plus the most its family's measured items add can't reach the slot's
// fifth alternative isn't simmed, even with allowance to spare.
func TestQuickSkipsUnpricedItemsThatCantRank(t *testing.T) {
	rn, s := uqRun(t, proto.OptimizerEffort_OptimizerEffortQuick, kaTrinketA)
	trinket1 := proto.ItemSlot_ItemSlotTrinket1
	// the family's only measured residual, at most +27: the fifth alternative sits 25 under the seed,
	// uqProc's stats 50 under and uqWeak's 55
	s.items[trinket1][kaTrinketA] = Estimate{Mean: 5, SE: 11}
	simmed := rn.eval.simmed()
	if err := rn.priceUnpriced(s); err != nil {
		t.Fatal(err)
	}
	if s.unpriced[trinket1][uqProc] || !s.unpriced[trinket1][uqWeak] {
		t.Errorf("trinket %d unpriced %v, trinket %d unpriced %v: want only %d priced", uqProc, s.unpriced[trinket1][uqProc], uqWeak, s.unpriced[trinket1][uqWeak], uqProc)
	}
	if spent := rn.eval.simmed() - simmed; spent > 2*int64(rn.screen) {
		t.Errorf("pricing simmed %d iterations, want the family's base and one variant at most", spent)
	}
}

// Normal leaves unpriced items unpriced, so they stay out of its alternatives as before.
func TestNormalLeavesUnpricedItemsAlone(t *testing.T) {
	rn, s := uqRun(t, proto.OptimizerEffort_OptimizerEffortNormal)
	simmed := rn.eval.simmed()
	if err := rn.priceUnpriced(s); err != nil {
		t.Fatal(err)
	}
	if rn.eval.simmed() != simmed || !s.unpriced[proto.ItemSlot_ItemSlotTrinket1][uqProc] {
		t.Errorf("Normal simmed %d iterations and priced trinket %d", rn.eval.simmed()-simmed, uqProc)
	}
	alts, _, err := rn.neighborhood(s, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, a := range alts {
		if a.Item.GetId() == uqProc || a.Item.GetId() == uqWeak {
			t.Errorf("Normal reported the unpriced trinket %d", a.Item.GetId())
		}
	}
}

// Beside a two-hand pick the screen values an unpriced one-hander with the off hand runnersUp pairs
// it with: alone the light sword's stats rank it under the strong great sword's (-40 against +20 over
// the pick's), with the sword beside it over them (+130). An allowance for one sim prices it first.
func TestQuickScreensAOneHanderWithItsOffHand(t *testing.T) {
	req := ohpRequest(ohpGreat, 0, []int32{ohpGreat, ohpGreat3, ohpLight}, []int32{ohpSword})
	req.Settings.Effort = proto.OptimizerEffort_OptimizerEffortQuick
	rn, s := ohpRun(t, req)
	mh := proto.ItemSlot_ItemSlotMainHand
	for _, id := range []int32{ohpGreat3, ohpLight} {
		delete(s.items[mh], id)
		s.unpriced[mh][id] = true
	}
	// a fresh count, so what measureEffects simmed costs a sim again
	rn.eval = &countingEvaluator{inner: rn.eval.inner, have: map[Point]int{}}
	old := unpricedShare
	defer func() { unpricedShare = old }()
	// the family's base, the seed, and one variant
	unpricedShare = float64(2*rn.screen+1) / float64(rn.total)
	if err := rn.priceUnpriced(s); err != nil {
		t.Fatal(err)
	}
	if s.unpriced[mh][ohpLight] || !s.unpriced[mh][ohpGreat3] {
		t.Errorf("main hand %d unpriced %v, %d unpriced %v: want only %d priced", ohpLight, s.unpriced[mh][ohpLight], ohpGreat3, s.unpriced[mh][ohpGreat3], ohpLight)
	}
}

// A weapon priced late leaves the weapon talents' term to the term, as measureEffects does: a Sword
// Specialization warrior's off-hand sword, priced again, gets back the residual it had.
func TestQuickPricesALateWeaponWithoutItsTerm(t *testing.T) {
	mh, oh := proto.ItemSlot_ItemSlotMainHand, proto.ItemSlot_ItemSlotOffHand
	req := kaRequest(proto.OptimizerEffort_OptimizerEffortQuick)
	req.Base.Raid.Parties[0].Players[0].TalentsString = withTalent(rtFuryNoTG, 0, wtSwordSpec, 5)
	rn, s := wtFakeRun(t, req, func(p Point) (Metrics, error) {
		m, err := kaMetrics(p)
		if err == nil && p.Loadout.Items[mh].ItemID == kaSword && p.Loadout.Items[oh].ItemID == kaSword {
			m[MetricDPS] += 150
		}
		return m, err
	})
	if len(s.weaponTerms) != 1 {
		t.Fatalf("weapon terms %+v, want Sword Specialization's", s.weaponTerms)
	}
	want, ok := s.items[oh][kaSword]
	if !ok {
		t.Fatal("measureEffects left the off-hand sword unpriced")
	}
	delete(s.items[oh], kaSword)
	s.unpriced[oh][kaSword] = true
	if err := rn.priceUnpriced(s); err != nil {
		t.Fatal(err)
	}
	got, ok := s.items[oh][kaSword]
	if !ok || s.unpriced[oh][kaSword] {
		t.Fatal("the off-hand sword wasn't priced again")
	}
	// the talent probes may have simmed the variant past what measureEffects read
	if math.Abs(got.Mean-want.Mean) > 5 {
		t.Errorf("the off-hand sword priced again at %+.2f, had %+.2f (the term is %+.2f)", got.Mean, want.Mean, s.weaponTerms[0].both.Mean)
	}
}

func uqAltIDs(alts []*proto.OptimizerSlotAlternative) []int32 {
	var out []int32
	for _, a := range alts {
		out = append(out, a.Item.GetId())
	}
	return out
}
