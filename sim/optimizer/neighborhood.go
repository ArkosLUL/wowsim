package optimizer

import (
	"slices"

	"github.com/wowsims/wotlk/sim/core/proto"
)

// neighborhoodRounds caps how often the neighborhood can move the best and look again.
const neighborhoodRounds = 5

// alternativesPerSlot is how many runners-up per slot the neighborhood sims and reports.
// firstRunnersUp of them get simmed before the rest: all at once they can push a round under full
// iterations, and a short round can't move the best.
const (
	alternativesPerSlot = 5
	firstRunnersUp      = 3
)

// neighborhood sims the best loadout's alternatives: per slot, the surrogate's top
// alternativesPerSlot other items, each with the rest of the best unchanged, paired with the best. A
// round sims each slot's first firstRunnersUp, then the rest only if none of those moved the best. An
// alternative that passes the acceptance test against the best and beats the seed becomes the new
// best, and its neighborhood is simmed in turn. The alternatives returned are the last round's,
// around the final best, best first within each slot; verified gains every best the rounds adopt.
func (r *run) neighborhood(s *surrogate, verified []*verifiedLoadout) ([]*proto.OptimizerSlotAlternative, []*verifiedLoadout, error) {
	se := newSearcher(s, r)
	type alternative struct {
		slot      proto.ItemSlot
		choice    ItemChoice
		loadout   Loadout
		eval      *Evaluation
		delta     Estimate
		raidDelta Estimate
	}
rounds:
	for round := 0; round < neighborhoodRounds; round++ {
		if err := se.check(r.best); err != nil {
			r.warn("no alternatives: the pick breaks a rule (%v)", err)
			return nil, verified, nil
		}
		st := se.newState(r.best)
		var first, rest []*alternative
		for slot := range r.best.Items {
			slot := proto.ItemSlot(slot)
			if r.pool.Locked[slot] {
				continue
			}
			for i, changes := range se.runnersUp(st, slot, alternativesPerSlot) {
				l := r.best
				for _, ch := range changes {
					l.Items[ch.slot] = ch.choice
				}
				a := &alternative{slot: slot, choice: changes[0].choice, loadout: l}
				if i < firstRunnersUp {
					first = append(first, a)
				} else {
					rest = append(rest, a)
				}
			}
		}
		if len(first) == 0 {
			return nil, verified, nil
		}

		var alts []*alternative
		iterations := max(r.budget.Iterations, r.eval.iterations(Point{Loadout: r.best}))
		for _, batch := range [][]*alternative{first, rest} {
			if len(batch) == 0 {
				continue
			}
			points := []Point{{Loadout: r.best}, {Loadout: r.r.Seed}}
			for _, a := range batch {
				points = append(points, Point{Loadout: a.loadout})
			}
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
			// a later batch can run fewer iterations than the first, and Evaluate may return just those
			r.bestEval, r.seedEval = mostIterations(r.bestEval, bestEval), mostIterations(r.seedEval, evals[1])
			var top *alternative
			for i, a := range batch {
				a.eval = evals[i+2]
				if a.eval == nil {
					continue
				}
				a.delta = r.obj.Delta(bestEval, a.eval)
				if r.raidMode() {
					a.raidDelta = Delta(bestEval, a.eval, MetricDPS)
				}
				// the search only sees floors through its penalty, so a runner-up can miss one
				if (top == nil || a.delta.Mean > top.delta.Mean) && r.pool.checkFloors(a.loadout) == nil {
					top = a
				}
			}
			alts = append(alts, batch...)
			if full && top != nil && r.accepts(top.delta) && r.beatsSeed(top.eval) && round < neighborhoodRounds-1 {
				r.best, r.bestEval = top.loadout, top.eval
				verified = append(verified, &verifiedLoadout{top.loadout, top.eval})
				r.sortVerified(verified)
				r.report()
				continue rounds
			}
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
				Slot:           a.slot,
				Item:           a.choice.ToProto(),
				ScoreDelta:     a.delta.Mean,
				ScoreDeltaSe:   a.delta.SE,
				RaidDpsDelta:   a.raidDelta.Mean,
				RaidDpsDeltaSe: a.raidDelta.SE,
			})
		}
		return out, verified, nil
	}
	return nil, verified, nil
}

func mostIterations(a, b *Evaluation) *Evaluation {
	if a == nil || (b != nil && b.Iterations > a.Iterations) {
		return b
	}
	return a
}
