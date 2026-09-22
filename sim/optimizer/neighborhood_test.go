package optimizer

import (
	"context"
	"math/rand"
	"sync"
	"testing"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
	"github.com/wowsims/wotlk/sim/core/stats"
)

// Plain attack power trinkets, clear of the known-answer ids.
const (
	nbTrinket40 = 9700301
	nbTrinket30 = 9700302
	nbTrinket20 = 9700303
)

var nbOnce sync.Once

func nbRegister() {
	nbOnce.Do(func() {
		core.AddToDatabase(&proto.SimDatabase{Items: []*proto.SimItem{
			kaItem(nbTrinket40, proto.ItemType_ItemTypeTrinket, stats.Stats{stats.AttackPower: 40}, ""),
			kaItem(nbTrinket30, proto.ItemType_ItemTypeTrinket, stats.Stats{stats.AttackPower: 30}, ""),
			kaItem(nbTrinket20, proto.ItemType_ItemTypeTrinket, stats.Stats{stats.AttackPower: 20}, ""),
		}})
	})
}

// nbRun is a known-answer Normal run that can only change trinket 1, set up the way verify leaves it:
// the seed is the best, simmed at full iterations, with budget iterations left. The surrogate ranks
// trinket 1's options by rank, whatever they really sim.
func nbRun(t *testing.T, rank map[int32]float64, budget int64) (*run, *surrogate) {
	t.Helper()
	req := kaRequest(proto.OptimizerEffort_OptimizerEffortNormal)
	nbRegister()
	for _, sp := range req.Pool.Slots {
		if sp.Slot == proto.ItemSlot_ItemSlotTrinket1 {
			sp.ItemIds = []int32{kaTrinketA, kaTrinketB, kaTrinketAP, kaTrinketAP2, nbTrinket40, nbTrinket30, nbTrinket20}
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

	rn := &run{
		ctx:    context.Background(),
		asked:  r,
		r:      r,
		budget: EffortBudget(proto.OptimizerEffort_OptimizerEffortNormal),
		rng:    rand.New(rand.NewSource(1)),
		pool:   pool,
		best:   r.Seed,
	}
	rn.eval = &countingEvaluator{inner: newKnownEvaluator(kaMetrics), have: map[Point]int{}}
	if rn.obj, err = NewObjective(rn.ctx, rn.eval, r, rn.budget.Iterations); err != nil {
		t.Fatal(err)
	}
	evals, err := rn.evaluate([]Point{{Loadout: r.Seed}}, rn.budget.Iterations)
	if err != nil {
		t.Fatal(err)
	}
	rn.seedEval, rn.bestEval = evals[0], evals[0]
	rn.seedJ = rn.obj.Score(rn.seedEval)
	rn.total = rn.eval.simmed() + budget

	s := newSurrogate(pool, r.Seed, nil)
	for id, v := range rank {
		// wide enough that pruning keeps every option
		s.items[proto.ItemSlot_ItemSlotTrinket1][id] = Estimate{Mean: v, SE: 100}
	}
	return rn, s
}

// Trinket 1 starts on the 60 AP trinket: B sims +60 DPS over it, A +40, AP2 +12, and the plain ones 20
// to 40 under it.
func TestNeighborhoodMovesTheBest(t *testing.T) {
	for _, tc := range []struct {
		name   string
		rank   map[int32]float64
		budget int64
	}{
		// 3 runners-up at 4000 iterations fit in 15000, all 5 don't
		{"first runners-up on a tight budget", map[int32]float64{kaTrinketB: 50, nbTrinket40: 40, nbTrinket30: 30, nbTrinket20: 20, kaTrinketA: 10, kaTrinketAP2: 5}, 15000},
		{"the rest when the first don't", map[int32]float64{nbTrinket40: 40, nbTrinket30: 30, nbTrinket20: 20, kaTrinketB: 10, kaTrinketA: 5, kaTrinketAP2: 1}, 1_000_000},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rn, s := nbRun(t, tc.rank, tc.budget)
			alts, verified, err := rn.neighborhood(s, nil)
			if err != nil {
				t.Fatal(err)
			}
			if got := rn.best.Items[proto.ItemSlot_ItemSlotTrinket1].ItemID; got != kaTrinketB || len(verified) != 1 {
				t.Errorf("trinket 1 is %d after %d moves, want %d after one", got, len(verified), kaTrinketB)
			}
			if len(alts) != alternativesPerSlot {
				t.Errorf("%d alternatives, want %d", len(alts), alternativesPerSlot)
			}
			for i := 1; i < len(alts); i++ {
				if alts[i].ScoreDelta > alts[i-1].ScoreDelta {
					t.Errorf("alternative %d scores %+.1f, over the one before it (%+.1f)", i, alts[i].ScoreDelta, alts[i-1].ScoreDelta)
				}
			}
			t.Logf("%d sims left of %d", rn.remaining(), tc.budget)
		})
	}
}
