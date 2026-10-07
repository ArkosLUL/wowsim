package optimizer

import (
	"math"
	"slices"

	"github.com/wowsims/wotlk/sim/core/proto"
)

// unpricedShare caps what priceUnpriced sims, as a share of the run's budget. A var for tests.
var unpricedShare = 0.05

// priceUnpriced sims, at Quick only, the unpriced items that could rank among a slot's
// alternativesPerSlot around the best, so the neighborhood ranks them on a measured residual.
// Could rank: the stats-only trial plus the most a measured item of its family adds beyond its
// stats reaches the slot's last priced alternative. Slots take turns, each best stats first, up to
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
	unpriced := map[effectKey]bool{}
	family := map[effectKey]*effectFamily{}
	variant := map[effectKey]Loadout{}
	for _, f := range families {
		for i, k := range f.keys {
			family[k], variant[k] = f, f.variants[i]
			if f.emptiedOH[i] {
				fx.emptiedOH[k] = true
			}
			if k.kind == effectItem && !s.hasResidual(k.slot, k.id) {
				unpriced[k] = true
			}
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
	mh, oh := proto.ItemSlot_ItemSlotMainHand, proto.ItemSlot_ItemSlotOffHand
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
			// the move runnersUp would make
			var extra []change
			if slot == mh && r.pool.fillsHands(&cand.Item) && st.l.Items[oh].ItemID != 0 {
				extra = []change{{oh, ItemChoice{}}}
			}
			changes := append([]change{{slot, se.configure(st, slot, cand, extra...)}}, extra...)
			l := st.l
			for _, ch := range changes {
				l.Items[ch.slot] = ch.choice
			}
			if err := r.pool.CheckGear(l); err != nil && !se.gemRuleBroken(err) {
				continue
			}
			// an item can be both measured and unpriced, when a later sim of it failed
			list = append(list, screened{k, se.trial(st, changes...) - s.items[fam][id].Mean})
		}
		if len(list) == 0 {
			continue
		}
		threshold := math.Inf(-1)
		if alts := se.runnersUp(st, slot, alternativesPerSlot, se.all[slot]); len(alts) == alternativesPerSlot {
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
			next := append(slices.Clone(points), Point{Loadout: family[k].base}, Point{Loadout: variant[k]})
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
	seedOH := seedS.seed.Items[oh].ItemID
	priced := 0
	for _, k := range order {
		est, ok := fx.raw[k]
		if !ok {
			continue
		}
		if fx.emptiedOH[k] && itemNeedsSim(seedOH) {
			ohRes := s.items[oh][seedOH]
			est = Estimate{Mean: est.Mean + ohRes.Mean, SE: math.Hypot(est.SE, ohRes.SE)}
		}
		s.items[k.slot][k.id] = est
		delete(s.unpriced[k.slot], k.id)
		priced++
		r.trace("alternatives: priced %s %d at %+.1f ± %.1f", k.slot, k.id, est.Mean, est.SE)
	}
	r.trace("alternatives: priced %d of %d unpriced items, %d could rank, in %d iterations", priced, len(unpriced), couldRank, spent)
	return nil
}

func newEffects() *effects {
	return &effects{
		raw:       map[effectKey]Estimate{},
		family:    map[effectKey]*effectFamily{},
		variant:   map[effectKey]Loadout{},
		emptiedOH: map[effectKey]bool{},
		flat:      map[proto.ItemSlot]Estimate{},
		missing:   map[effectKey]bool{},
		pairBase:  map[proto.ItemSlot]Loadout{},
	}
}

// unmeasured is s as measureEffects found it, before any residual, set or pair, so effectFamilies
// lays out the families, bases and variants it did then.
func (s *surrogate) unmeasured() *surrogate {
	out := *s
	out.gems, out.unpricedGems = map[int32]Estimate{}, map[int32]bool{}
	out.sets, out.setOf, out.pairs = nil, map[int32]int{}, map[pairKey]float64{}
	for slot := range out.items {
		out.items[slot] = map[int32]Estimate{}
		out.enchants[slot] = map[int32]Estimate{}
		out.unpriced[slot] = map[int32]bool{}
		out.unpricedEnchants[slot] = map[int32]bool{}
	}
	return &out
}
