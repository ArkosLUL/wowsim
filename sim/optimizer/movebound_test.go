package optimizer

import (
	"math/rand"
	"testing"

	"github.com/wowsims/wotlk/sim/core/proto"
)

// bestItem and bestPartner only skip moves their bound says can't win, so a bound under any move it
// covers would change picks. Every move from random gear, under a floor that's missed or met, an Orc's
// weapon specialization in play, scores at most its bound, the off hand bestPartner adds included.
func TestMoveBoundsHold(t *testing.T) {
	rn, s := crSurrogate(t)
	se := newSearcher(s, rn)
	mh, oh := proto.ItemSlot_ItemSlotMainHand, proto.ItemSlot_ItemSlotOffHand
	rng := rand.New(rand.NewSource(5))
	floor := s.floors[0].min
	checked, pruned := 0, 0
	for k := 0; k < 40; k++ {
		// half the time the floor sits far under any gear, so only the curves count
		s.floors[0].min = floor
		if k%2 == 1 {
			s.floors[0].min = floor - 500
		}
		l := s.seed
		for slot := range l.Items {
			if choices := kaChoicesSample(s.pool, proto.ItemSlot(slot), rng, 8); len(choices) > 0 && rng.Intn(4) > 0 {
				l.Items[slot] = choices[rng.Intn(len(choices))]
			} else if rng.Intn(3) == 0 {
				l.Items[slot] = ItemChoice{}
			}
		}
		st := se.newState(l)
		for slot := range st.l.Items {
			slot := proto.ItemSlot(slot)
			if slot == oh && st.l.Items[mh].ItemID != 0 && s.pool.fillsHands(s.handItem(mh, st.l.Items[mh].ItemID)) {
				continue
			}
			bounds := se.moveBounds(st, slot)
			for _, o := range se.opts[slot] {
				ub := bounds.of(o)
				if ub <= st.value+improvement {
					pruned++
				}
				var extra []change
				if slot == mh && s.pool.fillsHands(&o.cand.Item) && st.l.Items[oh].ItemID != 0 {
					extra = []change{{oh, ItemChoice{}}}
				}
				partner := slot == mh && len(extra) == 0 && st.l.Items[oh].ItemID == 0 && len(se.opts[oh]) > 0 && !s.pool.fillsHands(&o.cand.Item)
				choices := append(kaChoicesSample(s.pool, slot, rng, 4*len(s.pool.Slots[slot])), se.configure(st, slot, o.cand, extra...))
				for _, c := range choices {
					if c.ItemID != o.cand.Item.ID {
						continue
					}
					changes := append([]change{{slot, c}}, extra...)
					if !partner {
						checked++
						if v := se.trial(st, changes...); v > ub {
							t.Errorf("%s %+v scores %.3f, over its bound %.3f", slot, c, v, ub)
						}
						continue
					}
					// the partner's own bound, as bestPartner reads it, holds too
					bare, gaps := se.moveBase(st, &se.partnerGaps, changes...)
					for _, p := range se.opts[oh] {
						pub := se.moveBound(bare, gaps, se.racialWith(&o.cand.Item, &p.cand.Item), p)
						for _, pc := range append(kaChoicesSample(s.pool, oh, rng, 2*len(s.pool.Slots[oh])), se.configure(st, oh, p.cand, changes...)) {
							if pc.ItemID != p.cand.Item.ID {
								continue
							}
							checked++
							v := se.trial(st, append(changes, change{oh, pc})...)
							if v > ub {
								t.Errorf("%+v with %+v scores %.3f, over the main hand's bound %.3f", c, pc, v, ub)
							}
							if v > pub {
								t.Errorf("%+v with %+v scores %.3f, over the off hand's bound %.3f", c, pc, v, pub)
							}
						}
					}
				}
			}
		}
	}
	s.floors[0].min = floor
	if checked == 0 || pruned == 0 {
		t.Fatalf("checked %d moves and pruned %d: the test doesn't reach what it means to", checked, pruned)
	}
	t.Logf("%d moves checked, %d options pruned", checked, pruned)
}
