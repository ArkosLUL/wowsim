package optimizer

import (
	"fmt"
	"slices"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
	"github.com/wowsims/wotlk/sim/core/stats"
)

// Gem refills the non-meta sockets of the given slots with the gems worth the most, and returns
// the new loadout with what the regemmed slots' gems and socket bonuses are worth. Locked slots,
// the other slots and meta sockets keep their gems, which still count toward the limits and the
// meta.
//
// value prices a stat vector. The DP adds up gems and socket bonuses one at a time, so it's exact
// for a linear value: EP weights, or response-curve slopes around the current loadout. One slot
// makes it the per-item DP; every slot regems the whole loadout.
//
// The result keeps every rule gems touch: sockets, professions, unique gems, limit groups (3
// jewelcrafter gems) and the socketed meta's colors. It fails when no gemming activates the meta.
// The rest of l should already pass CheckGear.
func (p *Pool) Gem(l Loadout, slots []proto.ItemSlot, value func(stats.Stats) float64) (Loadout, float64, error) {
	var regem [NumSlots]bool
	var items [NumSlots]*core.Item
	for s, c := range l.Items {
		if c.ItemID == 0 {
			continue
		}
		item, ok := p.item(proto.ItemSlot(s), c.ItemID)
		if !ok {
			return l, 0, fmt.Errorf("slot %s: item %d isn't in the database", proto.ItemSlot(s), c.ItemID)
		}
		items[s] = item
	}
	for _, slot := range slots {
		if slot >= 0 && int(slot) < NumSlots && !p.Locked[slot] && items[slot] != nil {
			regem[slot] = true
		}
	}

	dp := newGemDP(p, l, items, regem, value)
	best, err := dp.solve()
	if err != nil {
		return l, 0, err
	}

	out := l
	for s := range out.Items {
		if !regem[s] {
			continue
		}
		sockets := p.sockets(items[s])
		var gems [MaxGems]int32
		for i := range sockets {
			if sockets[i] == proto.GemColor_GemColorMeta {
				gems[i] = l.Items[s].Gems[i]
			}
		}
		out.Items[s].Gems = gems
	}
	for _, pick := range best.picks {
		out.Items[pick.slot].Gems[pick.socket] = pick.gem
	}
	return out, best.value + dp.fixedBonus, nil
}

// gemOption is a gem the DP can put in a non-meta socket.
type gemOption struct {
	id     int32
	value  float64
	color  proto.GemColor
	colors [3]int32
	group  int // index into gemDP.groupMax, -1 for none
	unique int // index into gemDP's unique flags, -1 when it isn't unique
}

// gemStep is one socket the DP fills.
type gemStep struct {
	slot   proto.ItemSlot
	socket int
	color  proto.GemColor
	// It's one of the item's own sockets, so it counts for the socket bonus.
	own bool
	// First and last step of its item. The first resets the bonus flag to what the item's fixed
	// (meta) sockets allow; the last pays the bonus when every own socket matched.
	first, last  bool
	startMatched bool
	bonus        float64
}

type gemDP struct {
	options []gemOption
	steps   []gemStep
	// Meta requirements, as constraints on the state's color sums.
	constraints []*proto.MetaColorConstraint
	groupMax    []int8
	numUniques  int
	start       []int8
	// Socket bonuses of regemmed items with nothing to fill: already settled.
	fixedBonus float64
}

func newGemDP(p *Pool, l Loadout, items [NumSlots]*core.Item, regem [NumSlots]bool, value func(stats.Stats) float64) *gemDP {
	dp := &gemDP{}

	// the meta requirements and what the fixed gems already bring to them
	var fixedColors [3]int32
	fixedGroups := map[int32]int{}
	fixedGems := map[int32]bool{}
	for s, c := range l.Items {
		item := items[s]
		if item == nil {
			continue
		}
		fixedGroups[p.catalog[c.ItemID].GetLimitCategory()]++
		sockets := p.sockets(item)
		for _, gem := range p.socketed(c, item) {
			if gem.color == proto.GemColor_GemColorMeta {
				dp.constraints = append(dp.constraints, p.metas[gem.id].GetConstraints()...)
			}
		}
		for i, id := range c.Gems {
			if id == 0 || i >= len(sockets) || regem[s] && sockets[i] != proto.GemColor_GemColorMeta {
				continue
			}
			color := gemColor(p, id)
			colors := gemColors(color)
			for k := range fixedColors {
				fixedColors[k] += colors[k]
			}
			fixedGroups[p.catalog[id].GetLimitCategory()]++
			fixedGems[id] = true
		}
	}

	for s := range l.Items {
		if !regem[s] {
			continue
		}
		item := items[s]
		sockets := p.sockets(item)
		own := len(item.GemSockets)
		matched := own > 0
		for i := 0; i < own && i < len(sockets); i++ {
			if sockets[i] == proto.GemColor_GemColorMeta {
				gem := l.Items[s].Gems[i]
				matched = matched && gem != 0 && core.ColorIntersects(sockets[i], gemColor(p, gem))
			}
		}
		bonus := value(item.SocketBonus)
		first := len(dp.steps)
		for i, color := range sockets {
			if color == proto.GemColor_GemColorMeta {
				continue
			}
			dp.steps = append(dp.steps, gemStep{slot: proto.ItemSlot(s), socket: i, color: color, own: i < own, startMatched: matched, bonus: bonus})
		}
		if len(dp.steps) == first {
			if matched {
				dp.fixedBonus += bonus
			}
			continue
		}
		dp.steps[first].first = true
		dp.steps[len(dp.steps)-1].last = true
	}

	dp.options = p.gemOptions(value, len(dp.steps))
	groupIndex := map[int32]int{}
	uniqueIndex := map[int32]int{}
	for i := range dp.options {
		o := &dp.options[i]
		row := p.catalog[o.id]
		o.group, o.unique = -1, -1
		if category := row.GetLimitCategory(); category != 0 && p.limitGroups[category] != nil {
			if _, ok := groupIndex[category]; !ok {
				groupIndex[category] = len(dp.groupMax)
				dp.groupMax = append(dp.groupMax, int8(min(p.limitGroups[category].MaxEquipped, 127)))
			}
			o.group = groupIndex[category]
		}
		if row.GetUniqueEquipped() {
			if _, ok := uniqueIndex[o.id]; !ok {
				uniqueIndex[o.id] = dp.numUniques
				dp.numUniques++
			}
			o.unique = uniqueIndex[o.id]
		}
	}

	// state: constraint sums, then group counts, then unique flags, then the bonus flag
	dp.start = make([]int8, len(dp.constraints)+len(dp.groupMax)+dp.numUniques+1)
	for i, c := range dp.constraints {
		dp.start[i] = dp.clamp(i, c.Red*fixedColors[0]+c.Yellow*fixedColors[1]+c.Blue*fixedColors[2])
	}
	for category, g := range groupIndex {
		dp.start[len(dp.constraints)+g] = int8(min(fixedGroups[category], 127))
	}
	for id, u := range uniqueIndex {
		if fixedGems[id] {
			dp.start[len(dp.constraints)+len(dp.groupMax)+u] = 1
		}
	}
	return dp
}

func gemColor(p *Pool, id int32) proto.GemColor {
	if gem := p.gems[id]; gem != nil {
		return gem.Gem.Color
	}
	gem, _ := core.LookupGem(id)
	return gem.Color
}

// gemOptions keeps the gems that can matter. Gems with the same colors, limit group and
// uniqueness only differ by value: of a reusable kind the best is enough, of a unique kind the
// best one per socket. A limited gem no better than a free one of the same colors never helps.
func (p *Pool) gemOptions(value func(stats.Stats) float64, sockets int) []gemOption {
	type kind struct {
		colors   [3]int32
		category int32
		unique   bool
	}
	byKind := map[kind][]gemOption{}
	var kinds []kind
	for _, g := range p.Gems {
		if g.Gem.Color == proto.GemColor_GemColorMeta {
			continue
		}
		row := p.catalog[g.Gem.ID]
		k := kind{colors: gemColors(g.Gem.Color), unique: row.GetUniqueEquipped()}
		if category := row.GetLimitCategory(); p.limitGroups[category] != nil {
			k.category = category
		}
		if byKind[k] == nil {
			kinds = append(kinds, k)
		}
		byKind[k] = append(byKind[k], gemOption{id: g.Gem.ID, value: value(g.Gem.Stats), color: g.Gem.Color, colors: k.colors})
	}

	var out []gemOption
	for _, k := range kinds {
		options := byKind[k]
		// stable, so ties keep pool order
		slices.SortStableFunc(options, func(a, b gemOption) int {
			switch {
			case a.value > b.value:
				return -1
			case a.value < b.value:
				return 1
			}
			return 0
		})
		keep := 1
		if k.unique {
			keep = sockets
		}
		options = options[:min(keep, len(options))]
		if k.unique || k.category != 0 {
			if free := byKind[kind{colors: k.colors}]; len(free) > 0 {
				best := free[0].value
				for _, o := range free {
					best = max(best, o.value)
				}
				options = slices.DeleteFunc(options, func(o gemOption) bool { return o.value <= best })
			}
		}
		out = append(out, options...)
	}
	return out
}

// clamp caps a constraint sum at its target when no gem can lower it again: past the target, the
// exact sum doesn't matter.
func (dp *gemDP) clamp(i int, sum int32) int8 {
	c := dp.constraints[i]
	if c.Red >= 0 && c.Yellow >= 0 && c.Blue >= 0 {
		sum = min(sum, c.MinTotal)
	}
	return int8(max(min(sum, 127), -128))
}

type gemPick struct {
	slot   proto.ItemSlot
	socket int
	gem    int32
}

type gemSolution struct {
	value float64
	picks []gemPick
}

type dpEntry struct {
	state []int8
	value float64
	prev  int
	gem   int32
}

func (dp *gemDP) solve() (gemSolution, error) {
	nc, ng := len(dp.constraints), len(dp.groupMax)
	flag := len(dp.start) - 1
	layers := [][]dpEntry{{{state: dp.start, prev: -1}}}
	for _, step := range dp.steps {
		prev := layers[len(layers)-1]
		var next []dpEntry
		index := map[string]int{}
		add := func(state []int8, value float64, from int, gem int32) {
			if step.last {
				if state[flag] == 1 {
					value += step.bonus
				}
				// bonus settled, so clear the flag: states that only differ there merge
				state[flag] = 0
			}
			key := string(uint8Slice(state))
			if i, ok := index[key]; ok {
				if value > next[i].value {
					next[i] = dpEntry{state, value, from, gem}
				}
				return
			}
			index[key] = len(next)
			next = append(next, dpEntry{state, value, from, gem})
		}
		for from, entry := range prev {
			base := slices.Clone(entry.state)
			if step.first {
				base[flag] = boolInt8(step.startMatched)
			}

			empty := slices.Clone(base)
			if step.own {
				empty[flag] = 0
			}
			add(empty, entry.value, from, 0)

			for _, o := range dp.options {
				state := slices.Clone(base)
				ok := true
				for i, c := range dp.constraints {
					state[i] = dp.clamp(i, int32(state[i])+c.Red*o.colors[0]+c.Yellow*o.colors[1]+c.Blue*o.colors[2])
				}
				if o.group >= 0 {
					state[nc+o.group]++
					ok = state[nc+o.group] <= dp.groupMax[o.group]
				}
				if o.unique >= 0 {
					ok = ok && state[nc+ng+o.unique] == 0
					state[nc+ng+o.unique] = 1
				}
				if !ok {
					continue
				}
				if step.own && !core.ColorIntersects(step.color, o.color) {
					state[flag] = 0
				}
				add(state, entry.value+o.value, from, o.id)
			}
		}
		layers = append(layers, next)
	}

	last := layers[len(layers)-1]
	best := -1
	for i, entry := range last {
		if !dp.meetsConstraints(entry.state) {
			continue
		}
		if best < 0 || entry.value > last[best].value {
			best = i
		}
	}
	if best < 0 {
		return gemSolution{}, fmt.Errorf("no gems for these sockets activate the socketed meta")
	}

	solution := gemSolution{value: last[best].value}
	for k, i := len(dp.steps)-1, best; k >= 0; k-- {
		entry := layers[k+1][i]
		if entry.gem != 0 {
			solution.picks = append(solution.picks, gemPick{dp.steps[k].slot, dp.steps[k].socket, entry.gem})
		}
		i = entry.prev
	}
	return solution, nil
}

func (dp *gemDP) meetsConstraints(state []int8) bool {
	for i, c := range dp.constraints {
		if int32(state[i]) < c.MinTotal {
			return false
		}
	}
	return true
}

func boolInt8(b bool) int8 {
	if b {
		return 1
	}
	return 0
}

func uint8Slice(s []int8) []byte {
	out := make([]byte, len(s))
	for i, v := range s {
		out[i] = byte(v)
	}
	return out
}
