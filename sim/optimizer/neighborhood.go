package optimizer

import (
	"slices"

	"github.com/wowsims/wotlk/sim/core/proto"
)

// neighborhoodRounds caps how often the neighborhood can move the best and look again. A var, not a
// const, so tests can shrink it to reach the last round without a long adoption chain.
var neighborhoodRounds = 5

// alternativesPerSlot is how many runners-up per slot the neighborhood sims and reports.
// firstRunnersUp of them get simmed before the rest: all at once they can push a round under full
// iterations, and a short round can't move the best.
const (
	alternativesPerSlot = 5
	firstRunnersUp      = 3
)

// raceSE is how many standard errors apart a race step treats as decided: a gap that wide is not
// going to close with more iterations, so there's nothing left to learn from simming it further.
const raceSE = 3

// neighborhood sims the best loadout's alternatives: per slot, the surrogate's top
// alternativesPerSlot other items, each with the rest of the best unchanged, paired with the best. A
// round sims each slot's first firstRunnersUp, then the rest only if none of those moved the best.
// Each batch races its alternatives against the best in DefaultShardIterations shards (race),
// dropping one once it's decided or can't clear the acceptance bar, so a batch stops as soon as
// there's nothing left to decide instead of always spending the round's full iterations. A batch's
// leading alternative that passes the acceptance test gets topped up and rechecked (topUp) before it
// replaces the best, so a race that decided early on a small sample still has to hold up at the
// round's full iterations; only then does it become the new best, with its own neighborhood simmed in
// turn. On the round cap, there's no round left to look again, so the cap's own first and rest batches
// both run against whatever is current when they're reached, rebasing the round's other alternatives
// onto each adoption instead of leaving them pointing at a pick that's since been replaced. The
// alternatives returned are the last round's, around the final best, best first within each slot;
// verified gains every best the rounds adopt.
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

	// race sims batch's alternatives against r.best, in DefaultShardIterations shards, dropping one
	// once it's decided: more than raceSE standard errors from the best, whichever way, or (still too
	// close to call against the best) its upper confidence bound can't clear the acceptance bar
	// either. Each survivor's eval is set on it directly. It stops there, at r.budget.Iterations (or
	// best's already-simmed iterations, whichever is more, since verify's own race can leave best past
	// r.budget.Iterations), or wherever the run's remaining budget runs out first: the target caps the
	// race, it doesn't schedule it, so a batch that decides early costs less than a full round and a
	// batch that never does still stops at the cap.
	race := func(batch []*alternative) (bestEval, seedEval *Evaluation, err error) {
		active := slices.Clone(batch)
		target := max(r.budget.Iterations, r.eval.iterations(Point{Loadout: r.best}))
		iterations := 0
		for {
			step := min(DefaultShardIterations, target-iterations)
			if step <= 0 {
				break
			}
			next := iterations + step
			points := []Point{{Loadout: r.best}, {Loadout: r.r.Seed}}
			for _, a := range active {
				points = append(points, Point{Loadout: a.loadout})
			}
			if iterations > 0 && r.eval.cost(points, next) > r.remaining() {
				break
			}
			evals, err := r.evaluate(points, next)
			if err != nil {
				return nil, nil, err
			}
			iterations = next
			bestEval, seedEval = evals[0], evals[1]
			var undecided []*alternative
			for i, a := range active {
				if evals[i+2] == nil {
					continue
				}
				a.eval = evals[i+2]
				d := r.obj.Delta(bestEval, a.eval)
				if abs(d.Mean) > raceSE*d.SE || d.Mean+raceSE*d.SE <= acceptFraction*r.ownJ() {
					continue
				}
				undecided = append(undecided, a)
			}
			active = undecided
			if len(active) == 0 || iterations >= target {
				break
			}
		}
		return bestEval, seedEval, nil
	}

	// topUp confirms a race's winner before it replaces the best: it raises best, l and the seed to
	// r.budget.Iterations together, or to best's already-simmed iterations if verify's race left it
	// higher (or as much of either as what's left of the run's budget affords), so a gap the race
	// decided on a handful of shards still has to hold up at the round's full iterations, and adopting
	// never leaves best less precisely known than it already was.
	topUp := func(l Loadout) (bestEval, eval, seedEval *Evaluation, err error) {
		iterations := max(r.budget.Iterations, r.eval.iterations(Point{Loadout: r.best}))
		points := []Point{{Loadout: r.best}, {Loadout: l}, {Loadout: r.r.Seed}}
		for iterations > DefaultShardIterations && r.eval.cost(points, iterations) > r.remaining() {
			iterations /= 2
		}
		evals, err := r.evaluate(points, iterations)
		if err != nil {
			return nil, nil, nil, err
		}
		return evals[0], evals[1], evals[2], nil
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
		for _, batch := range [][]*alternative{first, rest} {
			if len(batch) == 0 {
				continue
			}
			bestEval, seedEval, err := race(batch)
			if err != nil {
				return nil, verified, err
			}
			// a later batch can run fewer iterations than the first, and a race may decide early
			r.bestEval, r.seedEval = mostIterations(r.bestEval, bestEval), mostIterations(r.seedEval, seedEval)
			var top *alternative
			for _, a := range batch {
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
			if top != nil && r.accepts(top.delta) {
				toppedBest, toppedTop, toppedSeed, err := topUp(top.loadout)
				if err != nil {
					return nil, verified, err
				}
				r.bestEval, r.seedEval = mostIterations(r.bestEval, toppedBest), mostIterations(r.seedEval, toppedSeed)
				if toppedBest == nil || toppedTop == nil {
					continue
				}
				top.delta = r.obj.Delta(toppedBest, toppedTop)
				top.eval = toppedTop
				if !r.accepts(top.delta) || !r.beatsSeed(top.eval) {
					continue
				}
				r.best, r.bestEval = top.loadout, top.eval
				verified = append(verified, &verifiedLoadout{top.loadout, top.eval})
				r.sortVerified(verified)
				r.report()
				if round < neighborhoodRounds-1 {
					continue rounds
				}
				// no round left to scan a fresh neighborhood around this pick: rebase what's
				// already simmed onto it instead of reporting alternatives against the pick it
				// just replaced, which is why a Quick tank run could list a runner-up well above
				// the loadout it reports as best
				for _, a := range alts {
					if a == top || a.eval == nil {
						continue
					}
					a.delta = r.obj.Delta(top.eval, a.eval)
					if r.raidMode() {
						a.raidDelta = Delta(top.eval, a.eval, MetricDPS)
					}
				}
				alts = slices.DeleteFunc(alts, func(a *alternative) bool { return a == top })
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
