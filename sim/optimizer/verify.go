package optimizer

import (
	"slices"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
)

// verifiedLoadout is a loadout the sim scored, paired with the seed.
type verifiedLoadout struct {
	loadout Loadout
	eval    *Evaluation
}

// Shares of the run's budget verification may take: the first pass over the search's picks, then
// the race between the ones too close to call.
const (
	verifyShare = 0.20
	raceShare   = 0.10
	raceRounds  = 4
)

// verify sims the search's picks at full iterations, paired with the seed, then races the ones within
// 2 standard errors of the best: every round doubles their iterations (the seed's too, so deltas keep
// pairing), until one stands out or the race's share runs out. The best replaces the seed only when it
// beats it (see beatsSeed). The result is sorted best first; loadouts that break a rule, stat floors
// included, aren't simmed.
func (r *run) verify(s *surrogate, candidates []Loadout) ([]*verifiedLoadout, error) {
	var picks []Loadout
	for _, l := range candidates {
		if l == r.r.Seed || r.pool.Check(l) != nil {
			continue
		}
		picks = append(picks, l)
	}
	iterations := r.budget.Iterations
	share := min(verifyShare*float64(r.total), float64(r.remaining()))
	for len(picks) > 1 && float64(r.eval.cost(pointsOf(picks), iterations)) > share {
		picks = picks[:len(picks)-1]
	}
	if len(picks) == 0 {
		if r.seedBroken != nil {
			r.warn("the search found nothing that keeps every rule, so the seed stays")
		}
		return nil, nil
	}
	evals, err := r.evaluate(pointsOf(picks), iterations)
	if err != nil {
		return nil, err
	}
	var verified []*verifiedLoadout
	for i, l := range picks {
		if evals[i] != nil {
			verified = append(verified, &verifiedLoadout{l, evals[i]})
		}
	}
	if len(verified) == 0 {
		return nil, nil
	}
	r.sortVerified(verified)

	raceBudget := min(raceShare*float64(r.total), float64(r.remaining()))
	for round := 0; round < raceRounds && len(verified) > 1; round++ {
		best := verified[0]
		var racing []*verifiedLoadout
		for _, v := range verified[1:] {
			if d := r.obj.Delta(best.eval, v.eval); d.Mean+2*d.SE >= 0 {
				racing = append(racing, v)
			}
		}
		if len(racing) == 0 {
			break
		}
		iterations *= 2
		racing = append([]*verifiedLoadout{best}, racing...)
		points := []Point{{Loadout: r.r.Seed}}
		for _, v := range racing {
			points = append(points, Point{Loadout: v.loadout})
		}
		cost := float64(r.eval.cost(points, iterations))
		if cost > raceBudget {
			break
		}
		raceBudget -= cost
		evals, err := r.evaluate(points, iterations)
		if err != nil {
			return nil, err
		}
		r.seedEval = evals[0]
		for i, v := range racing {
			if evals[i+1] != nil {
				v.eval = evals[i+1]
			}
		}
		r.sortVerified(verified)
	}

	for i, v := range verified {
		d := r.obj.Delta(r.seedEval, v.eval)
		r.trace("verified %d: surrogate %+.1f, sim %+.1f ± %.1f over %d iterations", i, s.value(v.loadout)-s.value(r.r.Seed), d.Mean, d.SE, v.eval.Iterations)
		if traceHook != nil {
			for slot, c := range v.loadout.Items {
				old := r.r.Seed.Items[slot]
				if c == old {
					continue
				}
				item, _ := core.LookupItem(c.ItemID)
				r.trace("    %s: %d %q (res %+.1f) %+v, seed %d (res %+.1f) %+v", proto.ItemSlot(slot), c.ItemID, item.Name,
					s.choiceResidual(proto.ItemSlot(slot), c), c, old.ItemID, s.choiceResidual(proto.ItemSlot(slot), old), old)
			}
		}
	}
	if r.beatsSeed(verified[0].eval) {
		r.best, r.bestEval = verified[0].loadout, verified[0].eval
	}
	r.report()
	return verified, nil
}

// sortVerified orders by paired delta from the seed, best first.
func (r *run) sortVerified(verified []*verifiedLoadout) {
	slices.SortStableFunc(verified, func(a, b *verifiedLoadout) int {
		da, db := r.obj.Delta(r.seedEval, a.eval).Mean, r.obj.Delta(r.seedEval, b.eval).Mean
		switch {
		case da > db:
			return -1
		case da < db:
			return 1
		}
		return 0
	})
}
