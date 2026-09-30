package optimizer

import (
	"context"
	"math/rand"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
)

// Feral bear relic ids from the P2 pool. Corruptor and White Stag proc Attack Power off Mangle, and
// Raven Goddess is a flat melee crit bonus in bear form, so all three help a bear. Crying Wind,
// Worship, Ravenous Beast, Shooting Star, Steadfast Renewal and Unseen Moon only boost spells a bear
// never casts (Insect Swarm, Rip, Shred, Starfire, Wrath and Moonfire).
const (
	idolCorruptor        = 45509
	idolWhiteStag        = 32257
	idolRavenGoddess     = 32387
	idolCryingWind       = 45270
	idolWorship          = 39757
	idolRavenousBeast    = 40713
	idolShootingStar     = 40321
	idolSteadfastRenewal = 40712
	idolUnseenMoon       = 33510
)

var (
	relicHelpful = []int32{idolCorruptor, idolWhiteStag, idolRavenGoddess}
	relicInert   = []int32{idolCryingWind, idolWorship, idolRavenousBeast, idolShootingStar, idolSteadfastRenewal, idolUnseenMoon}
)

// feralTankRelicRequest is a Quick Feral Tank P2 run over its realistic pool, wearing the P2
// preset's own idol, Corruptor. The pool carries thousands of candidates across every slot, the
// same budget pressure that leaves most of a relic family unmeasured without BIS-relic's fix.
func feralTankRelicRequest(tb testing.TB) *proto.OptimizeGearRequest {
	tb.Helper()
	player := feralTank(tb, feralTankTalents)

	req := presetOptimizeRequest(tb, "fury_p1")
	req.Base.Raid = core.SinglePlayerRaidProto(player, core.FullPartyBuffs, core.FullRaidBuffs, core.FullDebuffs)
	req.Base.Raid.Tanks = []*proto.UnitReference{{Type: proto.UnitReference_Player, Index: 0}}
	req.Base.Encounter = tankEncounter(tb, 2)
	player.HealingModel = tankHealing(req.Base.Encounter.Targets[0])

	req.Settings = &proto.OptimizerSettings{
		ContentPhase: 2,
		Effort:       proto.OptimizerEffort_OptimizerEffortQuick,
		RacialMode:   proto.OptimizerRacialMode_OptimizerRacialKeepCurrent,
		TankSurvival: 0.7,
	}
	req.Pool = realisticPool(tb, player, 2, nil)
	return req
}

// The search prunes a relic slot down to its few genuinely competitive candidates before
// alternatives are even considered, same as any other slot, so its residuals are the real test:
// every candidate priced on its own effect, not left as a shared 0.00 placeholder by a budget cut
// that a relic's stat-less prior can't steer. The Feral Tank P2 case keeps its own idol, Corruptor,
// over five others that do nothing for a bear, and prices two other idols that do help it
// differently from Corruptor and from each other.
func TestRelicResidualsScoreByEffect(t *testing.T) {
	req := feralTankRelicRequest(t)
	r, err := PrepareRequest(req)
	if err != nil {
		t.Fatal(err)
	}
	pool, err := CompilePool(r)
	if err != nil {
		t.Fatal(err)
	}
	metrics, err := WeightedMetrics(r.Settings)
	if err != nil {
		t.Fatal(err)
	}

	rn := &run{
		ctx:    context.Background(),
		asked:  r,
		r:      r,
		budget: EffortBudget(proto.OptimizerEffort_OptimizerEffortQuick),
		rng:    rand.New(rand.NewSource(1)),
		pool:   pool,
		best:   r.Seed,
		ref:    r.Equipped,
	}
	rn.total = int64(rn.budget.Evaluations) * int64(rn.budget.Iterations)
	rn.screen = roundShards(rn.budget.Iterations / 4)
	rn.eval = &countingEvaluator{inner: NewSimEvaluator(r, metrics...), have: map[Point]int{}}
	if rn.obj, err = NewObjective(rn.ctx, rn.eval, r, rn.budget.Iterations); err != nil {
		t.Fatal(err)
	}
	evals, err := rn.evaluate([]Point{{Loadout: r.Seed}}, rn.budget.Iterations)
	if err != nil {
		t.Fatal(err)
	}
	rn.seedEval, rn.bestEval = evals[0], evals[0]
	rn.seedJ = rn.obj.Score(rn.seedEval)

	resp, _, err := rn.measureCurves(rn.newStatScreen())
	if err != nil {
		t.Fatal(err)
	}
	s := newSurrogate(pool, r.Seed, resp)
	if err := rn.measureEffects(s); err != nil {
		t.Fatal(err)
	}

	ranged := proto.ItemSlot_ItemSlotRanged
	priced := map[int32]float64{}
	for _, id := range append(append([]int32{}, relicHelpful...), relicInert...) {
		if s.unpriced[ranged][id] {
			t.Errorf("idol %d was never simmed, so a budget cut could still drop a relic candidate", id)
			continue
		}
		priced[id] = s.items[ranged][id].Mean
	}
	for _, id := range relicHelpful {
		if v := priced[id]; v <= 1 {
			t.Errorf("idol %d (helps a bear) priced at %+.1f, want a real gain over doing nothing", id, v)
		}
	}
	for _, id := range relicInert {
		if v := priced[id]; abs(v) > 1 {
			t.Errorf("idol %d (a bear never casts its spell) priced at %+.1f, want it to price the same as doing nothing", id, v)
		}
	}
	if priced[idolCorruptor] == priced[idolWhiteStag] {
		t.Errorf("Corruptor and White Stag both price at %+.1f, want their different procs to price differently", priced[idolCorruptor])
	}
}

// A run whose seed wears an idol that does nothing for a bear still finds and keeps one that does,
// every candidate priced along the way, not just whichever few a budget cut happened to keep.
func TestOptimizeFeralTankFindsAHelpfulRelic(t *testing.T) {
	req := feralTankRelicRequest(t)
	req.Base.Raid.Parties[0].Players[0].Equipment.Items[int(proto.ItemSlot_ItemSlotRanged)] = &proto.ItemSpec{Id: idolWorship}

	r, err := PrepareRequest(req)
	if err != nil {
		t.Fatal(err)
	}
	metrics, err := WeightedMetrics(r.Settings)
	if err != nil {
		t.Fatal(err)
	}
	ranged := proto.ItemSlot_ItemSlotRanged
	rec := &effectsRecorder{inner: NewSimEvaluator(r, metrics...), slot: ranged, seen: map[int32]bool{}}
	result := optimize(context.Background(), r, r, rec, func(p *proto.OptimizerProgress) {
		rec.mu.Lock()
		rec.stage = p.Stage
		rec.mu.Unlock()
	}, time.Now())
	if result.ErrorResult != "" {
		t.Fatal(result.ErrorResult)
	}
	for _, w := range result.Warnings {
		t.Log(w)
	}

	for _, id := range append(append([]int32{}, relicHelpful...), relicInert...) {
		if id == idolWorship {
			continue // the seed: never simmed against itself
		}
		if !rec.seen[id] {
			t.Errorf("the Effects stage never simmed idol %d, so a budget cut could still drop a relic candidate", id)
		}
	}

	best := result.Best.GetEquipment().Items[int(ranged)].GetId()
	if !slices.Contains(relicHelpful, best) {
		t.Errorf("picked idol %d, want one whose effect helps a bear (%v)", best, relicHelpful)
	}
}

// Only relics skip the budget cut. A wand or a warrior's bow still ranks by its stats, and stays a
// weapon family, so the probes can price weapon damage that doesn't matter flat.
func TestOnlyRelicsSkipTheBudgetCut(t *testing.T) {
	for _, tc := range []struct {
		name  string
		req   func(*testing.T) *proto.OptimizeGearRequest
		relic bool
	}{
		{"feral tank idols", func(t *testing.T) *proto.OptimizeGearRequest { return feralTankRelicRequest(t) }, true},
		{"arcane mage wands", func(t *testing.T) *proto.OptimizeGearRequest { return realCapRequest(t, "arcane_p3", 3, false) }, false},
		{"fury warrior ranged weapons", func(t *testing.T) *proto.OptimizeGearRequest { return realCapRequest(t, "fury_p1", 1, false) }, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r, err := PrepareRequest(tc.req(t))
			if err != nil {
				t.Fatal(err)
			}
			pool, err := CompilePool(r)
			if err != nil {
				t.Fatal(err)
			}
			s := newSurrogate(pool, r.Seed, Response{})
			rn := &run{asked: r, r: r, pool: pool}
			v := s.slopes(s.seedStats)
			fx := &effects{raw: map[effectKey]Estimate{}, missing: map[effectKey]bool{}, pairBase: map[proto.ItemSlot]Loadout{}}
			families := rn.effectFamilies(s, v, s.newGemPlan(v), fx)
			i := slices.IndexFunc(families, func(f *effectFamily) bool { return f.name == proto.ItemSlot_ItemSlotRanged.String()+" items" })
			if i < 0 {
				t.Fatal("no ranged slot family")
			}
			if f := families[i]; f.full != tc.relic || f.weapons == tc.relic {
				t.Errorf("%d variants: full %v and weapons %v, want full %v and weapons %v", len(f.keys), f.full, f.weapons, tc.relic, !tc.relic)
			}
		})
	}
}

// effectsRecorder notes every distinct item id the Effects stage sims in slot.
type effectsRecorder struct {
	inner Evaluator
	slot  proto.ItemSlot

	mu    sync.Mutex
	stage string
	seen  map[int32]bool
}

func (g *effectsRecorder) Evaluate(ctx context.Context, points []Point, iterations int) ([]*Evaluation, error) {
	g.mu.Lock()
	if g.stage == "Effects" {
		for _, p := range points {
			if id := p.Loadout.Items[g.slot].ItemID; id != 0 {
				g.seen[id] = true
			}
		}
	}
	g.mu.Unlock()
	return g.inner.Evaluate(ctx, points, iterations)
}
