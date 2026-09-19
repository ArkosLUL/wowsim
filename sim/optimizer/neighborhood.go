package optimizer

import (
	"slices"

	"github.com/wowsims/wotlk/sim/core/proto"
)

// neighborhoodRounds caps how often the neighborhood can move the best and look again.
const neighborhoodRounds = 5

// neighborhood sims the best loadout's alternatives: per slot, the surrogate's top runnersUp other
// items, each with the rest of the best unchanged, paired with the best. An alternative that passes
// the acceptance test against the best and beats the seed becomes the new best, and its neighborhood
// is simmed in turn. The alternatives returned are the last round's, around the final best, best
// first within each slot; verified gains every best the rounds adopt.
func (r *run) neighborhood(s *surrogate, verified []*verifiedLoadout) ([]*proto.OptimizerSlotAlternative, []*verifiedLoadout, error) {
	se := newSearcher(s, r)
	type alternative struct {
		slot    proto.ItemSlot
		choice  ItemChoice
		loadout Loadout
		eval    *Evaluation
		delta   Estimate
	}
	for round := 0; round < neighborhoodRounds; round++ {
		if err := se.check(r.best); err != nil {
			r.warn("no alternatives: the pick breaks a rule (%v)", err)
			return nil, verified, nil
		}
		st := se.newState(r.best)
		var alts []*alternative
		for slot := range r.best.Items {
			slot := proto.ItemSlot(slot)
			if r.pool.Locked[slot] {
				continue
			}
			for _, changes := range se.runnersUp(st, slot, runnersUp) {
				l := r.best
				for _, ch := range changes {
					l.Items[ch.slot] = ch.choice
				}
				alts = append(alts, &alternative{slot: slot, choice: changes[0].choice, loadout: l})
			}
		}
		if len(alts) == 0 {
			return nil, verified, nil
		}

		points := []Point{{Loadout: r.best}, {Loadout: r.r.Seed}}
		for _, a := range alts {
			points = append(points, Point{Loadout: a.loadout})
		}
		iterations := max(r.budget.Iterations, r.eval.iterations(points[0]))
		for iterations > DefaultShardIterations && r.eval.cost(points, iterations) > r.remaining() {
			iterations /= 2
		}
		// a short round still reports alternatives, but too noisy to move the best
		full := iterations >= r.budget.Iterations
		evals, err := r.evaluate(points, iterations)
		if err != nil {
			return nil, verified, err
		}
		bestEval := evals[0]
		r.bestEval, r.seedEval = bestEval, evals[1]
		var top *alternative
		for i, a := range alts {
			a.eval = evals[i+2]
			if a.eval == nil {
				continue
			}
			a.delta = r.obj.Delta(bestEval, a.eval)
			// the search only sees floors through its penalty, so a runner-up can miss one
			if (top == nil || a.delta.Mean > top.delta.Mean) && r.pool.checkFloors(a.loadout) == nil {
				top = a
			}
		}
		if full && top != nil && r.accepts(top.delta) && r.beatsSeed(top.eval) && round < neighborhoodRounds-1 {
			r.best, r.bestEval = top.loadout, top.eval
			verified = append(verified, &verifiedLoadout{top.loadout, top.eval})
			r.sortVerified(verified)
			r.report()
			continue
		}

		var out []*proto.OptimizerSlotAlternative
		slices.SortStableFunc(alts, func(a, b *alternative) int {
			if a.slot != b.slot {
				return int(a.slot) - int(b.slot)
			}
			switch {
			case a.delta.Mean > b.delta.Mean:
				return -1
			case a.delta.Mean < b.delta.Mean:
				return 1
			}
			return 0
		})
		for _, a := range alts {
			if a.eval == nil {
				continue
			}
			out = append(out, &proto.OptimizerSlotAlternative{
				Slot:         a.slot,
				Item:         a.choice.ToProto(),
				ScoreDelta:   a.delta.Mean,
				ScoreDeltaSe: a.delta.SE,
			})
		}
		return out, verified, nil
	}
	return nil, verified, nil
}
