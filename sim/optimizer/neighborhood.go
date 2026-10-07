package optimizer

import (
	"math"
	"slices"

	"github.com/wowsims/wotlk/sim/core/proto"
)

// neighborhoodRounds caps how often the neighborhood can move the best and look again. A var, not a
// const, so tests can shrink it to reach the last round without a long adoption chain.
var neighborhoodRounds = 5

// alternativesPerSlot is how many alternatives per slot the neighborhood reports, the best five of
// the candidatesPerSlot it sims. firstRunnersUp candidates get simmed before the rest, which only run
// when none of those moved the best. rankedPerSlot is how many a slot's top get simmed to the full
// round before the reported five are picked, so their order isn't a few shards' luck.
const (
	alternativesPerSlot = 5
	candidatesPerSlot   = 15
	firstRunnersUp      = 3
	rankedPerSlot       = alternativesPerSlot + 2
)

// alternativesShare is the budget the neighborhood adds to what the stages before it left, as a share
// of the budget the run started with. A var so tests can change it.
var alternativesShare = 1.0

// raceSE is how many standard errors apart a race step treats as decided: a gap that wide is not
// going to close with more iterations, so there's nothing left to learn from simming it further.
const raceSE = 3

// neighborhood sims the best loadout's alternatives: per slot, the surrogate's top
// candidatesPerSlot other items out of every priced one, pruned or not, each with the rest of the
// best unchanged but for the off hand a main hand empties or brings along (see runnersUp), paired
// with the best. A round sims each slot's first firstRunnersUp, then the rest only if none of those
// moved the best. The surrogate only picks who gets simmed: the reported alternatives are each slot's
// best alternativesPerSlot by measured delta (see rank).
// Each batch races its alternatives against the best in DefaultShardIterations shards (race),
// dropping one once it's decided or can't clear the acceptance bar, so a batch stops as soon as
// there's nothing left to decide instead of always spending the round's full iterations. A batch's
// leading alternative that passes the acceptance test gets topped up and rechecked (topUp) before it
// replaces the best, so a race that decided early on a small sample still has to hold up at the
// round's full iterations; only then does it become the new best, with its own neighborhood simmed in
// turn. On the round cap, there's no round left to look again, so the cap's own first and rest batches
// both run against whatever is current when they're reached, each alternative's move made again on
// the pick it now stands beside, and what was simmed beside a pick since replaced simmed again. The
// alternatives returned are the last round's, each the final best with its change alone (a main
// hand's off hand included), best first within each slot; verified gains every best the rounds adopt.
func (r *run) neighborhood(s *surrogate, verified []*verifiedLoadout) ([]*proto.OptimizerSlotAlternative, []*verifiedLoadout, error) {
	if err := r.priceUnpriced(s); err != nil {
		return nil, verified, err
	}
	se := newSearcher(s, r)
	mh, oh := proto.ItemSlot_ItemSlotMainHand, proto.ItemSlot_ItemSlotOffHand
	type alternative struct {
		slot   proto.ItemSlot
		choice ItemChoice
		// runnersUp's move, to make again on a best adopted since
		changes   []change
		loadout   Loadout
		eval      *Evaluation
		delta     Estimate
		raidDelta Estimate
	}

	// describes says whether a's loadout is r.best with only what a reports changed: its slot, for a
	// main hand the off hand, which it reports whenever that differs from r.best's, and any gems
	// elsewhere the move had to shift.
	describes := func(a *alternative) bool {
		want := r.best
		for _, ch := range a.changes {
			want.Items[ch.slot] = a.loadout.Items[ch.slot]
		}
		if a.slot == mh {
			want.Items[oh] = a.loadout.Items[oh]
		}
		return a.loadout == want
	}
	// rebuild makes a's move again on r.best, unsimmed; false when that breaks a rule there or
	// changes nothing.
	rebuild := func(a *alternative) bool {
		l := r.best
		for _, ch := range a.changes {
			l.Items[ch.slot] = ch.choice
		}
		if l == r.best || se.check(l) != nil {
			return false
		}
		a.loadout, a.eval = l, nil
		return true
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

	// rank picks each slot's reported alternatives, the best alternativesPerSlot by measured delta. A
	// slot's top rankedPerSlot get simmed up to the round's full iterations first (or as many as the
	// run's budget affords), so their order doesn't rest on the few shards race gave one it dropped early.
	rank := func(alts []*alternative) ([]*alternative, error) {
		alts = slices.DeleteFunc(alts, func(a *alternative) bool { return a.eval == nil })
		byDelta := func(a, b *alternative) int {
			if a.slot != b.slot {
				return int(a.slot) - int(b.slot)
			}
			switch {
			case a.delta.Mean > b.delta.Mean:
				return -1
			case a.delta.Mean < b.delta.Mean:
				return 1
			}
			// items that do nothing in the sim tie to the digit; the one with an effect says more
			return boolIndex(ItemHasEffect(b.choice.ItemID)) - boolIndex(ItemHasEffect(a.choice.ItemID))
		}
		// keep returns alts' first n in each slot, in order
		keep := func(alts []*alternative, n int) []*alternative {
			seen := map[proto.ItemSlot]int{}
			return slices.DeleteFunc(alts, func(a *alternative) bool {
				seen[a.slot]++
				return seen[a.slot] > n
			})
		}
		slices.SortStableFunc(alts, byDelta)
		if traceHook != nil {
			for _, a := range alts {
				r.trace("candidate %s: %d, %+.1f ± %.1f at %d iterations, moves %v", a.slot, a.choice.ItemID, a.delta.Mean, a.delta.SE, a.eval.Iterations, a.changes)
			}
		}
		alts = keep(alts, rankedPerSlot)
		iterations := r.budget.Iterations
		points := []Point{{Loadout: r.best}}
		for _, a := range alts {
			points = append(points, Point{Loadout: a.loadout})
		}
		for iterations > DefaultShardIterations && r.eval.cost(points, iterations) > r.remaining() {
			iterations /= 2
		}
		before := r.eval.simmed()
		evals, err := r.evaluate(points, iterations)
		if err != nil {
			return nil, err
		}
		r.trace("alternatives: ranked %d at %d, %d iterations", len(points)-1, iterations, r.eval.simmed()-before)
		if evals[0] != nil {
			r.bestEval = mostIterations(r.bestEval, evals[0])
			for i, a := range alts {
				if e := evals[i+1]; e != nil && e.Iterations >= a.eval.Iterations {
					a.eval = e
					a.delta = r.obj.Delta(evals[0], e)
					if r.raidMode() {
						a.raidDelta = Delta(evals[0], e, MetricDPS)
					}
				}
			}
		}
		slices.SortStableFunc(alts, byDelta)
		alts = keep(alts, alternativesPerSlot)
		if traceHook != nil {
			short, inside := 0, 0
			for i, a := range alts {
				if a.eval.Iterations < iterations {
					short++
				}
				if i > 0 && alts[i-1].slot == a.slot && abs(alts[i-1].delta.Mean-a.delta.Mean) <= 2*math.Hypot(alts[i-1].delta.SE, a.delta.SE) {
					inside++
				}
			}
			r.trace("alternatives: %d reported, %d under %d iterations, %d adjacent pairs inside 2 se", len(alts), short, iterations, inside)
		}
		return alts, nil
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
			for i, changes := range se.runnersUp(st, slot, candidatesPerSlot, se.all[slot]) {
				l := r.best
				for _, ch := range changes {
					l.Items[ch.slot] = ch.choice
				}
				a := &alternative{slot: slot, choice: changes[0].choice, changes: changes, loadout: l}
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
			// on the round cap, the first batch can have replaced the best the rest were built on
			batch = slices.DeleteFunc(batch, func(a *alternative) bool { return !describes(a) && !rebuild(a) })
			if len(batch) == 0 {
				continue
			}
			before := r.eval.simmed()
			bestEval, seedEval, err := race(batch)
			if err != nil {
				return nil, verified, err
			}
			r.trace("alternatives: raced %d, %d iterations", len(batch), r.eval.simmed()-before)
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
				r.trace("alternatives: round %d adopts %s %d, %+.1f ± %.1f", round, top.slot, top.choice.ItemID, top.delta.Mean, top.delta.SE)
				r.best, r.bestEval = top.loadout, top.eval
				verified = append(verified, &verifiedLoadout{top.loadout, top.eval})
				r.sortVerified(verified)
				r.report()
				if round < neighborhoodRounds-1 {
					continue rounds
				}
				// no round left to scan a fresh neighborhood around this pick: score what's already
				// simmed against it instead of the pick it just replaced, which is why a Quick tank
				// run could list a runner-up well above the loadout it reports as best. A loadout
				// that isn't this pick with its own change gets rebuilt below.
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

		// what was simmed beside a best adopted since gets its move again on the final best and a
		// race of its own, never adopted: there's no round left. Gear and delta then both describe
		// the final best with the alternative's change.
		var stale []*alternative
		alts = slices.DeleteFunc(alts, func(a *alternative) bool {
			if a.eval == nil || describes(a) {
				return false
			}
			if !rebuild(a) {
				return true
			}
			stale = append(stale, a)
			return false
		})
		if len(stale) > 0 {
			bestEval, seedEval, err := race(stale)
			if err != nil {
				return nil, verified, err
			}
			r.bestEval, r.seedEval = mostIterations(r.bestEval, bestEval), mostIterations(r.seedEval, seedEval)
			for _, a := range stale {
				if a.eval == nil {
					continue
				}
				a.delta = r.obj.Delta(bestEval, a.eval)
				if r.raidMode() {
					a.raidDelta = Delta(bestEval, a.eval, MetricDPS)
				}
			}
			r.trace("alternatives: %d simmed again against the pick the round cap adopted", len(stale))
		}

		var out []*proto.OptimizerSlotAlternative
		alts, err := rank(alts)
		if err != nil {
			return nil, verified, err
		}
		for _, a := range alts {
			r.trace("alternative %s: %d, off hand %d, %+.1f ± %.1f", a.slot, a.choice.ItemID, a.loadout.Items[oh].ItemID, a.delta.Mean, a.delta.SE)
			alt := &proto.OptimizerSlotAlternative{
				Slot:           a.slot,
				Item:           a.choice.ToProto(),
				ScoreDelta:     a.delta.Mean,
				ScoreDeltaSe:   a.delta.SE,
				RaidDpsDelta:   a.raidDelta.Mean,
				RaidDpsDeltaSe: a.raidDelta.SE,
			}
			// an empty one when a two-hander drops the best's off hand
			if a.slot == mh && a.loadout.Items[oh] != r.best.Items[oh] {
				alt.OffHand = a.loadout.Items[oh].ToProto()
			}
			out = append(out, alt)
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
