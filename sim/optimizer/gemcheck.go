package optimizer

import (
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/wowsims/wotlk/sim/core/proto"
	"github.com/wowsims/wotlk/sim/core/stats"
)

// gemTwins are copies of l, one for each distinct gem in its free non-meta sockets: Gem regems the
// slots holding that gem with it and the banned ones out of the running, so its sockets take the
// runners-up gemOptions drops for it, or whatever else value rates next, while socket bonuses and the
// meta still hold. A twin that breaks a rule, stat floors included, or repeats l or another twin is
// left out.
func (p *Pool) gemTwins(l Loadout, value func(stats.Stats) float64, banned map[int32]bool) []Loadout {
	var order []int32
	holders := map[int32][]proto.ItemSlot{}
	for slot, c := range l.Items {
		if c.ItemID == 0 || p.Locked[slot] {
			continue
		}
		item, ok := p.item(proto.ItemSlot(slot), c.ItemID)
		if !ok {
			continue
		}
		sockets := p.sockets(item)
		for i, g := range c.Gems {
			if g == 0 || i >= len(sockets) || sockets[i] == proto.GemColor_GemColorMeta {
				continue
			}
			held, seen := holders[g]
			if !seen {
				order = append(order, g)
			}
			if !slices.Contains(held, proto.ItemSlot(slot)) {
				holders[g] = append(held, proto.ItemSlot(slot))
			}
		}
	}
	var twins []Loadout
	for _, g := range order {
		without := maps.Clone(banned)
		if without == nil {
			without = map[int32]bool{}
		}
		without[g] = true
		twin, _, err := p.gemWithout(l, holders[g], value, without)
		if err != nil || twin == l || slices.Contains(twins, twin) || p.Check(twin) != nil {
			continue
		}
		twins = append(twins, twin)
	}
	return twins
}

// linearAt prices a stat vector at l's slopes, the way the search's gem DP prices one there.
func (s *surrogate) linearAt(l Loadout) func(stats.Stats) float64 {
	v := s.slopes(s.loadoutStats(l), s.racialDelta(&l))
	return func(x stats.Stats) float64 { return dot(v, x) }
}

// checkGems sims verified[0]'s gem twins (gemTwins) paired with it at the run's iterations, and the
// twin with the best mean over it goes first. Gems are priced off the curves and never simmed one by
// one, so a small or noisy edge in the slopes can fill every socket with one gem; this gives the sim
// the last word. While verified[0] beats the seed, a twin has to as well. The twins join verified,
// so a tank's tie on score takes them to time to live too (breakTie).
func (r *run) checkGems(verified []*verifiedLoadout) ([]*verifiedLoadout, error) {
	if len(verified) == 0 || r.pool == nil {
		return verified, nil
	}
	pick := verified[0]
	s := r.surrogates[pick.loadout.RacialTraits]
	if s == nil {
		return verified, nil
	}
	twins := slices.DeleteFunc(r.pool.gemTwins(pick.loadout, s.linearAt(pick.loadout), s.unpricedGems), func(l Loadout) bool {
		return l == r.r.Seed || slices.ContainsFunc(verified, func(v *verifiedLoadout) bool { return v.loadout == l })
	})
	if len(twins) == 0 {
		return verified, nil
	}
	evals, err := r.evaluate(pointsOf(twins), r.budget.Iterations)
	if err != nil {
		return verified, err
	}
	mustBeatSeed := r.beatsSeed(pick.eval)
	winner, best := -1, Estimate{}
	for i, l := range twins {
		eval := evals[i]
		if eval == nil {
			continue
		}
		verified = append(verified, &verifiedLoadout{l, eval})
		d := r.obj.Delta(pick.eval, eval)
		r.trace("gem twin %s: %+.1f ± %.1f over the pick", r.pool.gemSwap(pick.loadout, l), d.Mean, d.SE)
		if d.Mean > best.Mean && (!mustBeatSeed || r.beatsSeed(eval)) {
			winner, best = len(verified)-1, d
		}
	}
	if winner < 0 {
		return verified, nil
	}
	v := verified[winner]
	copy(verified[1:winner+1], verified[:winner])
	verified[0] = v
	r.warn("%s scores %+.1f ± %.1f over the gems the search picked, the best of %d gem swaps simmed at %d iterations, so the pick takes it",
		r.pool.gemSwap(pick.loadout, v.loadout), best.Mean, best.SE, len(twins), r.budget.Iterations)
	return verified, nil
}

// gemSwap names the gems from has and to doesn't, then the ones it gets instead, with their counts.
func (p *Pool) gemSwap(from, to Loadout) string {
	count := func(l Loadout) map[int32]int {
		n := map[int32]int{}
		for _, c := range l.Items {
			for _, g := range c.Gems {
				if g != 0 {
					n[g]++
				}
			}
		}
		return n
	}
	before, after := count(from), count(to)
	list := func(a, b map[int32]int) string {
		var ids []int32
		for id, n := range a {
			if n > b[id] {
				ids = append(ids, id)
			}
		}
		if len(ids) == 0 {
			return "nothing"
		}
		slices.Sort(ids)
		parts := make([]string, len(ids))
		for i, id := range ids {
			name := p.gem(id).Name
			if name == "" {
				name = fmt.Sprintf("gem %d", id)
			}
			parts[i] = fmt.Sprintf("%d %s", a[id]-b[id], name)
		}
		return strings.Join(parts, ", ")
	}
	return fmt.Sprintf("swapping %s for %s", list(before, after), list(after, before))
}
