package optimizer

import (
	"math"
	"slices"

	"github.com/wowsims/wotlk/sim/core/proto"
)

// unpricedShare caps what priceUnpriced sims, as a share of the run's budget. A var for tests.
var unpricedShare = 0.05

// priceUnpriced sims, at Quick only, the unpriced items that could rank among a slot's
// candidatesPerSlot around the best, so the neighborhood ranks them on a measured residual.
// Could rank: the trial of the move runnersUp would rank, the off hand a one-hander brings included,
// less the item's own residual, plus the most a measured item of its family adds beyond its stats
// reaches the slot's last priced candidate. Slots take turns, each best stats first, up to
// unpricedShare; the run's budget grows by what they cost, so the race keeps what it had.
func (r *run) priceUnpriced(s *surrogate) error {
	if r.asked.Settings.GetEffort() != proto.OptimizerEffort_OptimizerEffortQuick {
		return nil
	}
	se := newSearcher(s, r)
	if se.check(r.best) != nil {
		return nil
	}
	// the families measureEffects laid out, on the seed's race, so new residuals compare with the rest
	seedS := s
	if base := r.surrogates[r.r.Seed.RacialTraits]; base != nil {
		seedS = base
	}
	scratch := seedS.unmeasured()
	v0 := scratch.slopes(scratch.seedStats, 0)
	fx := newEffects()
	families := r.effectFamilies(scratch, v0, scratch.newGemPlan(v0), fx)
	fx.index(families)
	unpriced := map[effectKey]bool{}
	for k := range fx.family {
		if k.kind == effectItem && !s.hasResidual(k.slot, k.id) {
			unpriced[k] = true
		}
	}
	if len(unpriced) == 0 {
		return nil
	}

	most := map[proto.ItemSlot]float64{}
	for fs := range s.items {
		for id, est := range s.items[fs] {
			if s.unpriced[fs][id] {
				continue
			}
			if cur, ok := most[proto.ItemSlot(fs)]; !ok || est.Mean+2*est.SE > cur {
				most[proto.ItemSlot(fs)] = est.Mean + 2*est.SE
			}
		}
	}

	type screened struct {
		key   effectKey
		stats float64
	}
	st := se.newState(r.best)
	// CheckGear, not se.check: the item is unpriced
	legal := func(changes []change) bool {
		l := st.l
		for _, ch := range changes {
			l.Items[ch.slot] = ch.choice
		}
		err := r.pool.CheckGear(l)
		return err == nil || se.gemRuleBroken(err)
	}
	var perSlot [][]screened
	couldRank := 0
	for slot := range r.best.Items {
		slot := proto.ItemSlot(slot)
		if r.pool.Locked[slot] {
			continue
		}
		fam := familySlot(slot)
		var list []screened
		for _, cand := range r.pool.Slots[slot] {
			id := cand.Item.ID
			k := effectKey{effectItem, fam, id}
			if !unpriced[k] || id == st.l.Items[slot].ItemID {
				continue
			}
			// the move runnersUp would rank, off hand included
			changes, paired := se.moveTo(st, slot, cand)
			if !legal(changes) {
				// runnersUp would pair it with a legal off hand instead, worth no less than none
				if !paired || !legal(changes[:1]) {
					continue
				}
				changes = changes[:1]
			}
			// an item can be both measured and unpriced, when a later sim of it failed
			list = append(list, screened{k, se.trial(st, changes...) - s.items[fam][id].Mean})
		}
		if len(list) == 0 {
			continue
		}
		threshold := math.Inf(-1)
		if alts := se.runnersUp(st, slot, candidatesPerSlot, se.all[slot]); len(alts) == candidatesPerSlot {
			threshold = se.trial(st, alts[len(alts)-1]...)
		}
		// a family with nothing measured gives no bound: all of it could rank
		if bound, ok := most[fam]; ok {
			list = slices.DeleteFunc(list, func(c screened) bool { return c.stats+bound < threshold })
		}
		couldRank += len(list)
		slices.SortStableFunc(list, func(a, b screened) int {
			switch {
			case a.stats > b.stats:
				return -1
			case a.stats < b.stats:
				return 1
			}
			return 0
		})
		perSlot = append(perSlot, list)
	}

	allowance := int64(unpricedShare * float64(r.total))
	chosen := map[effectKey]bool{}
	var order []effectKey
	var points []Point
	for rank, more := 0, true; more; rank++ {
		more = false
		for _, list := range perSlot {
			if rank >= len(list) {
				continue
			}
			more = true
			k := list[rank].key
			if chosen[k] {
				continue
			}
			next := append(slices.Clone(points), Point{Loadout: fx.family[k].base}, Point{Loadout: fx.variant[k]})
			if r.eval.cost(next, r.screen) > allowance {
				continue
			}
			chosen[k], points = true, next
			order = append(order, k)
		}
	}
	if len(order) == 0 {
		r.trace("alternatives: none of %d unpriced items priced, %d could rank", len(unpriced), couldRank)
		return nil
	}
	var measure []*effectFamily
	for _, f := range families {
		if sub := f.subset(func(i int) bool { return chosen[f.keys[i]] }); len(sub.keys) > 0 {
			measure = append(measure, sub)
		}
	}
	before := r.eval.simmed()
	err := r.measureFamilies(scratch, fx, measure, r.screen)
	spent := r.eval.simmed() - before
	r.total += spent
	if err != nil {
		return err
	}

	// applyEffects' item step for these keys only: all of fx would unprice its misses again, weapons
	// fx.flat priced included
	priced := 0
	for _, k := range order {
		est, ok := fx.raw[k]
		if !ok {
			continue
		}
		est = s.itemResidual(fx, k, est)
		s.items[k.slot][k.id] = est
		delete(s.unpriced[k.slot], k.id)
		priced++
		r.trace("alternatives: priced %s %d at %+.1f ± %.1f", k.slot, k.id, est.Mean, est.SE)
	}
	r.trace("alternatives: priced %d of %d unpriced items, %d could rank, in %d iterations", priced, len(unpriced), couldRank, spent)
	return nil
}

// unmeasured is s as measureEffects found it, before any residual, set, pair or weapon term, so
// effectFamilies lays out the families, bases and variants it did then.
func (s *surrogate) unmeasured() *surrogate {
	out := *s
	out.gems, out.unpricedGems = map[int32]Estimate{}, map[int32]bool{}
	out.sets, out.setOf, out.pairs = nil, map[int32]int{}, map[pairKey]float64{}
	out.weaponTerms, out.seedWeapon, out.emptiedRefund = nil, 0, Estimate{}
	for slot := range out.items {
		out.items[slot] = map[int32]Estimate{}
		out.enchants[slot] = map[int32]Estimate{}
		out.unpriced[slot] = map[int32]bool{}
		out.unpricedEnchants[slot] = map[int32]bool{}
	}
	return &out
}
