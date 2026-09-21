package optimizer

import (
	"errors"
	"fmt"
	"math"
	"slices"
	"sort"
	"sync"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
	"github.com/wowsims/wotlk/sim/core/stats"
)

// surrogate predicts J(l) - J(seed) without a sim: the response curves over l's gear stats, plus the
// residuals sims measured for what stats don't show (effects, weapon damage, set bonuses), plus a
// penalty for missing a stat floor. It's separable per slot except for the curves' bends and the
// set counts, which the search tracks as running totals.
type surrogate struct {
	pool      *Pool
	seed      Loadout
	seedStats stats.Stats
	curves    [stats.Len]*curve
	// Stats with a curve, in stat order.
	curved []stats.Stat

	// Item residuals by family slot (see familySlot): what an item is worth beyond its stats. Rings
	// and trinkets count from an empty slot, the main hand from the seed's weapon, the other slots
	// from an item without an effect.
	items [NumSlots]map[int32]Estimate
	// Enchant residuals per slot, from an enchant without an effect.
	enchants [NumSlots]map[int32]Estimate
	// Gem residuals, metas included, from an empty socket.
	gems map[int32]Estimate
	// What the sims couldn't price: items, enchants and gems the search must leave alone.
	unpriced         [NumSlots]map[int32]bool
	unpricedEnchants [NumSlots]map[int32]bool
	unpricedGems     map[int32]bool

	sets  []*itemSet
	setOf map[int32]int

	// What a ring or trinket pair is worth beyond its items' own residuals: procs that share an aura
	// or a cooldown don't stack.
	pairs map[pairKey]float64

	floors      []floorTerm
	floorWeight float64
}

// curve is a response curve as straight lines between its simmed knots, carried on past the ends at
// the end segments' slopes. The response curve's own two-line fit is too coarse for the search: ArP's
// value keeps rising toward its cap and a Ret paladin's agility drops off, so one line through far
// knots misprices small changes by up to 2x.
type curve struct {
	// Knot offsets, ascending, and J there.
	x, y []float64
	// slope[i] is the slope below x[i] for i < len(x), above the last knot for i == len(x).
	slope []float64
}

func newCurve(c *ResponseCurve) *curve {
	out := &curve{}
	for _, k := range c.Knots {
		if n := len(out.x); n > 0 && k.Offset <= out.x[n-1] {
			continue
		}
		out.x = append(out.x, k.Offset)
		out.y = append(out.y, k.Delta.Mean)
	}
	if len(out.x) < 2 {
		// no knots to join (a curve built by hand): the fit's two lines
		bp := c.Breakpoint
		switch {
		case bp < 0:
			out.x, out.y = []float64{bp, 0}, []float64{c.Value(bp), 0}
		case bp > 0:
			out.x, out.y = []float64{0, bp}, []float64{0, c.Value(bp)}
		default:
			out.x, out.y = []float64{0}, []float64{0}
		}
		for _, x := range out.x {
			if x <= bp {
				out.slope = append(out.slope, c.SlopeBelow)
			} else {
				out.slope = append(out.slope, c.SlopeAbove)
			}
		}
		out.slope = append(out.slope, c.SlopeAbove)
		return out
	}
	n := len(out.x)
	out.slope = make([]float64, n+1)
	for i := 1; i < n; i++ {
		out.slope[i] = (out.y[i] - out.y[i-1]) / (out.x[i] - out.x[i-1])
	}
	out.slope[0], out.slope[n] = out.slope[1], out.slope[n-1]
	return out
}

// segment is how many knots lie below offset: the index of its slope.
func (c *curve) segment(offset float64) int {
	return sort.SearchFloat64s(c.x, offset)
}

func (c *curve) value(offset float64) float64 {
	i := c.segment(offset)
	if i == len(c.x) {
		return c.y[i-1] + c.slope[i]*(offset-c.x[i-1])
	}
	return c.y[i] + c.slope[i]*(offset-c.x[i])
}

// itemSet is a set whose bonuses the sims priced, as J at 2 and 4 pieces from none.
type itemSet struct {
	name            string
	two, four       Estimate
	hasTwo, hasFour bool
}

func (set *itemSet) value(pieces int) float64 {
	switch {
	case pieces >= 4 && set.hasFour:
		return set.four.Mean
	case pieces >= 2 && set.hasTwo:
		return set.two.Mean
	}
	return 0
}

// jumpBounds bound what one more piece does to the set's value: nothing, 1 to 2 pieces, or 3 to 4.
// Either jump can be negative, a noisy or useless 2pc included.
func (set *itemSet) jumpBounds() (lo, hi float64) {
	twoLo, twoHi := 0.0, 0.0
	if set.hasTwo {
		twoLo, twoHi = set.two.Mean-2*set.two.SE, set.two.Mean+2*set.two.SE
		lo, hi = min(lo, twoLo), max(hi, twoHi)
	}
	if set.hasFour {
		lo = min(lo, set.four.Mean-2*set.four.SE-twoHi)
		hi = max(hi, set.four.Mean+2*set.four.SE-twoLo)
	}
	return lo, hi
}

// floorTerm is a stat floor, with the sheet stat approximated as linear in gear stats around the seed.
type floorTerm struct {
	stat     stats.Stat
	min      float64
	seed     float64
	gradient stats.Stats
}

// pairKey is a ring or trinket pair, by the first slot of the pair, lower item id first.
type pairKey struct {
	slot proto.ItemSlot
	a, b int32
}

// pairSlots are the first slots of the ring and trinket pairs; the second is the next slot.
var pairSlots = []proto.ItemSlot{proto.ItemSlot_ItemSlotFinger1, proto.ItemSlot_ItemSlotTrinket1}

func newPairKey(slot proto.ItemSlot, a, b int32) pairKey {
	if a > b {
		a, b = b, a
	}
	return pairKey{slot, a, b}
}

// pairCorrection is what l's ring and trinket pairs add beyond their items' residuals.
func (s *surrogate) pairCorrection(l *Loadout) float64 {
	if len(s.pairs) == 0 {
		return 0
	}
	total := 0.0
	for _, slot := range pairSlots {
		total += s.pairs[newPairKey(slot, l.Items[slot].ItemID, l.Items[slot+1].ItemID)]
	}
	return total
}

// familySlot is the slot whose residuals an item in slot shares: rings and trinkets are worth the
// same in either slot.
func familySlot(slot proto.ItemSlot) proto.ItemSlot {
	switch slot {
	case proto.ItemSlot_ItemSlotFinger2:
		return proto.ItemSlot_ItemSlotFinger1
	case proto.ItemSlot_ItemSlotTrinket2:
		return proto.ItemSlot_ItemSlotTrinket1
	}
	return slot
}

func newSurrogate(pool *Pool, seed Loadout, resp Response) *surrogate {
	s := &surrogate{
		pool:         pool,
		seed:         seed,
		gems:         map[int32]Estimate{},
		unpricedGems: map[int32]bool{},
		setOf:        map[int32]int{},
		pairs:        map[pairKey]float64{},
	}
	for slot := range s.items {
		s.items[slot] = map[int32]Estimate{}
		s.enchants[slot] = map[int32]Estimate{}
		s.unpriced[slot] = map[int32]bool{}
		s.unpricedEnchants[slot] = map[int32]bool{}
	}
	for st := stats.Stat(0); st < stats.Len; st++ {
		if c := resp[st]; c != nil {
			s.curves[st] = newCurve(c)
			s.curved = append(s.curved, st)
		}
	}
	s.seedStats = s.loadoutStats(seed)
	return s
}

// choiceStats is what core's TotalStats gives for the choice, from the pool's copies where it can.
func (s *surrogate) choiceStats(slot proto.ItemSlot, c ItemChoice) stats.Stats {
	if c.ItemID == 0 {
		return stats.Stats{}
	}
	cand := s.pool.candidate(slot, c.ItemID)
	if cand == nil {
		item := core.NewItem(c.CoreSpec(), s.pool.reforging)
		return item.TotalStats()
	}
	total := cand.Item.Stats
	if c.Enchant != 0 {
		total = total.Add(cand.enchantStats(c.Enchant))
	}
	if c.ReforgeFrom != 0 || c.ReforgeTo != 0 {
		total = total.Add(cand.reforgeStats(c.ReforgeFrom, c.ReforgeTo))
	}
	own := cand.Item.GemSockets
	matched := len(own) > 0
	for i, id := range c.Gems {
		if id == 0 {
			if i < len(own) {
				matched = false
			}
			continue
		}
		gem := s.pool.gem(id)
		total = total.Add(gem.Stats)
		if i < len(own) && !core.ColorIntersects(own[i], gem.Color) {
			matched = false
		}
	}
	if matched {
		total = total.Add(cand.Item.SocketBonus)
	}
	return total
}

func (c *Candidate) enchantStats(id int32) stats.Stats {
	for _, e := range c.Enchants {
		if e.ID == id {
			return e.Stats
		}
	}
	enchant, _ := core.LookupEnchant(id)
	return enchant.Stats
}

func (c *Candidate) reforgeStats(from, to int32) stats.Stats {
	for _, r := range c.Reforges {
		if r.From == from && r.To == to {
			return r.Stats
		}
	}
	return core.ReforgeStats(&c.Item, &proto.ItemReforge{FromStatType: from, ToStatType: to}, c.reforging)
}

// gem is the pool's copy of a gem, else core's.
func (p *Pool) gem(id int32) core.Gem {
	if g := p.gems[id]; g != nil {
		return g.Gem
	}
	gem, _ := core.LookupGem(id)
	return gem
}

func (s *surrogate) loadoutStats(l Loadout) stats.Stats {
	var total stats.Stats
	for slot, c := range l.Items {
		total = total.Add(s.choiceStats(proto.ItemSlot(slot), c))
	}
	return total
}

// choiceResidual is what the sims priced for the choice beyond its stats, set bonuses aside.
func (s *surrogate) choiceResidual(slot proto.ItemSlot, c ItemChoice) float64 {
	if c.ItemID == 0 {
		return 0
	}
	v := s.items[familySlot(slot)][c.ItemID].Mean + s.enchants[slot][c.Enchant].Mean
	for _, g := range c.Gems {
		if g != 0 {
			v += s.gems[g].Mean
		}
	}
	return v
}

// priced says whether the surrogate can value the choice: nothing in it has an effect or weapon
// damage the sims didn't measure.
func (s *surrogate) priced(slot proto.ItemSlot, c ItemChoice) bool {
	if c.ItemID == 0 {
		return true
	}
	if s.unpriced[familySlot(slot)][c.ItemID] || s.unpricedEnchants[slot][c.Enchant] {
		return false
	}
	for _, g := range c.Gems {
		if g != 0 && s.unpricedGems[g] {
			return false
		}
	}
	return true
}

// score is J's predicted change from the seed for gear stats total, residuals res and set counts.
func (s *surrogate) score(total stats.Stats, res float64, counts []int8) float64 {
	v := res + s.curveValue(total.Subtract(s.seedStats))
	for i, n := range counts {
		if n >= 2 {
			v += s.sets[i].value(int(n))
		}
	}
	if len(s.floors) > 0 {
		v -= s.floorWeight * s.floorShortfall(total)
	}
	return v
}

// value is score for a whole loadout.
func (s *surrogate) value(l Loadout) float64 {
	var total stats.Stats
	res := s.pairCorrection(&l)
	counts := make([]int8, len(s.sets))
	for slot, c := range l.Items {
		total = total.Add(s.choiceStats(proto.ItemSlot(slot), c))
		res += s.choiceResidual(proto.ItemSlot(slot), c)
		if i, ok := s.setOf[c.ItemID]; ok && c.ItemID != 0 {
			counts[i]++
		}
	}
	return s.score(total, res, counts)
}

func (s *surrogate) floorShortfall(total stats.Stats) float64 {
	short := 0.0
	for _, f := range s.floors {
		have := f.seed
		for st, g := range f.gradient {
			if g != 0 {
				have += g * (total[st] - s.seedStats[st])
			}
		}
		short += max(0, f.min-have)
	}
	return short
}

// curveValue is what the curves make of gear stats moved by offset from the seed's.
func (s *surrogate) curveValue(offset stats.Stats) float64 {
	v := 0.0
	for _, st := range s.curved {
		if offset[st] != 0 {
			v += s.curves[st].value(offset[st])
		}
	}
	return v
}

// statsGain is what the curves make of total's stats moving by to minus from.
func (s *surrogate) statsGain(total, from, to *stats.Stats) float64 {
	gain := 0.0
	for _, st := range s.curved {
		if d := to[st] - from[st]; d != 0 {
			c, x := s.curves[st], total[st]-s.seedStats[st]
			gain += c.value(x+d) - c.value(x)
		}
	}
	return gain
}

// slopes is J's marginal value per point of each gear stat at total: each curve's slope where total
// sits, plus the floor penalty's pull.
func (s *surrogate) slopes(total stats.Stats) stats.Stats {
	var v stats.Stats
	for _, st := range s.curved {
		c := s.curves[st]
		v[st] = c.slope[c.segment(total[st]-s.seedStats[st])]
	}
	for _, f := range s.floors {
		have := f.seed
		for st, g := range f.gradient {
			if g != 0 {
				have += g * (total[st] - s.seedStats[st])
			}
		}
		if have < f.min {
			v = v.Add(f.gradient.Multiply(s.floorWeight))
		}
	}
	return v
}

// regime keys a set of slopes: the segment each curve is on, and which floors are missed. Gem plans
// only change with it. It's a hash, FNV-1a; a collision would only cost a worse gem plan.
func (s *surrogate) regime(total stats.Stats) uint64 {
	key := uint64(14695981039346656037)
	mix := func(b int) {
		key ^= uint64(b)
		key *= 1099511628211
	}
	for _, st := range s.curved {
		mix(s.curves[st].segment(total[st] - s.seedStats[st]))
	}
	for _, f := range s.floors {
		have := f.seed
		for st, g := range f.gradient {
			if g != 0 {
				have += g * (total[st] - s.seedStats[st])
			}
		}
		mix(boolIndex(have < f.min))
	}
	return key
}

// slopeBox is the smallest and largest slope each stat's curve has, for bounds that hold wherever
// the rest of the gear puts the totals.
func (s *surrogate) slopeBox() (lo, hi stats.Stats) {
	for _, st := range s.curved {
		c := s.curves[st]
		lo[st], hi[st] = slices.Min(c.slope), slices.Max(c.slope)
	}
	for _, f := range s.floors {
		// items that help reach a floor are never pruned, and a stat that pulls away from one costs
		// up to the penalty's weight more while it's missed
		for st, g := range f.gradient {
			switch {
			case g > 0:
				hi[st] = math.Inf(1)
			case g < 0:
				lo[st] += s.floorWeight * g
			}
		}
	}
	return lo, hi
}

func dot(v, x stats.Stats) float64 {
	total := 0.0
	for i, xi := range x {
		if xi != 0 && v[i] != 0 {
			total += v[i] * xi
		}
	}
	return total
}

// boxLow and boxHigh bound dot(v, x) over every v in the box [lo, hi].
func boxLow(lo, hi, x stats.Stats) float64 {
	total := 0.0
	for i, xi := range x {
		switch {
		case xi > 0 && lo[i] != 0:
			total += lo[i] * xi
		case xi < 0 && hi[i] != 0:
			total += hi[i] * xi
		}
	}
	return total
}

func boxHigh(lo, hi, x stats.Stats) float64 {
	total := 0.0
	for i, xi := range x {
		switch {
		case xi > 0 && hi[i] != 0:
			total += hi[i] * xi
		case xi < 0 && lo[i] != 0:
			total += lo[i] * xi
		}
	}
	return total
}

// gemPlan is the best gems for one set of slopes, ignoring the rules that tie sockets together:
// free gems only (no unique or limited ones), and metas whether or not they'd activate. Gem, the
// exact DP, takes over when the plan breaks a rule.
type gemPlan struct {
	anyID  int32
	anyVal float64
	// Best free gem per socket color.
	byColor map[proto.GemColor]gemChoice
	metaID  int32
	metaVal float64
}

type gemChoice struct {
	id    int32
	value float64
}

var socketColors = []proto.GemColor{proto.GemColor_GemColorRed, proto.GemColor_GemColorYellow, proto.GemColor_GemColorBlue, proto.GemColor_GemColorPrismatic}

func (s *surrogate) gemValue(g *GemCandidate, v stats.Stats) float64 {
	return dot(v, g.Gem.Stats) + s.gems[g.Gem.ID].Mean
}

// freeGem is a gem any number of sockets can hold.
func (s *surrogate) freeGem(g *GemCandidate) bool {
	if g.Catalog.GetUniqueEquipped() {
		return false
	}
	category := g.Catalog.GetLimitCategory()
	return category == 0 || s.pool.limitGroups[category] == nil
}

func (s *surrogate) newGemPlan(v stats.Stats) *gemPlan {
	gp := &gemPlan{byColor: map[proto.GemColor]gemChoice{}}
	haveAny, haveMeta := false, false
	for _, g := range s.pool.Gems {
		if s.unpricedGems[g.Gem.ID] {
			continue
		}
		val := s.gemValue(g, v)
		if g.Gem.Color == proto.GemColor_GemColorMeta {
			if !haveMeta || val > gp.metaVal {
				gp.metaID, gp.metaVal, haveMeta = g.Gem.ID, val, true
			}
			continue
		}
		if !s.freeGem(g) {
			continue
		}
		if !haveAny || val > gp.anyVal {
			gp.anyID, gp.anyVal, haveAny = g.Gem.ID, val, true
		}
		for _, color := range socketColors {
			if !core.ColorIntersects(color, g.Gem.Color) {
				continue
			}
			if best, ok := gp.byColor[color]; !ok || val > best.value {
				gp.byColor[color] = gemChoice{g.Gem.ID, val}
			}
		}
	}
	return gp
}

// fill gems the candidate's sockets: every non-meta socket its best free gem, or every own socket a
// gem of its color for the socket bonus, whichever is worth more. A meta socket gets meta, the best
// meta when meta is 0, or stays empty when it's negative.
func (gp *gemPlan) fill(s *surrogate, cand *Candidate, v stats.Stats, meta int32) [MaxGems]int32 {
	var free, matching [MaxGems]int32
	own := len(cand.Item.GemSockets)
	freeVal, matchVal := 0.0, 0.0
	freeMatched, matchMatched := own > 0, own > 0
	for i, color := range cand.Sockets {
		if i >= MaxGems {
			break
		}
		if color == proto.GemColor_GemColorMeta {
			id := meta
			switch {
			case id == 0:
				id = gp.metaID
			case id < 0:
				id = 0
			}
			free[i], matching[i] = id, id
			if id == 0 && i < own {
				freeMatched, matchMatched = false, false
			}
			continue
		}
		free[i] = gp.anyID
		if gp.anyID != 0 {
			freeVal += gp.anyVal
		}
		if i < own && (gp.anyID == 0 || !core.ColorIntersects(color, s.pool.gem(gp.anyID).Color)) {
			freeMatched = false
		}
		pick, ok := gp.byColor[color]
		if i >= own || !ok {
			pick, ok = gemChoice{gp.anyID, gp.anyVal}, gp.anyID != 0
			if i < own {
				matchMatched = false
			}
		}
		if ok {
			matching[i] = pick.id
			matchVal += pick.value
		}
	}
	bonus := dot(v, cand.Item.SocketBonus)
	if freeMatched {
		freeVal += bonus
	}
	if matchMatched {
		matchVal += bonus
	}
	if matchVal > freeVal {
		return matching
	}
	return free
}

// quickChoice configures a candidate for slopes v one part at a time: the enchant and the reforge
// worth the most, then gems by the plan. It's what the search's moves use; the polish tries every
// enchant and reforge against the exact score.
func (s *surrogate) quickChoice(slot proto.ItemSlot, cand *Candidate, v stats.Stats, gp *gemPlan, meta int32) ItemChoice {
	c := ItemChoice{ItemID: cand.Item.ID}
	best := 0.0
	for _, e := range cand.Enchants {
		if s.unpricedEnchants[slot][e.ID] {
			continue
		}
		if val := dot(v, e.Stats) + s.enchants[slot][e.ID].Mean; val > best {
			best, c.Enchant = val, e.ID
		}
	}
	best = 0
	for _, r := range cand.Reforges {
		if val := dot(v, r.Stats); val > best {
			best, c.ReforgeFrom, c.ReforgeTo = val, r.From, r.To
		}
	}
	c.Gems = gp.fill(s, cand, v, meta)
	return c
}

// headMeta is the meta gem in l, if any.
func (s *surrogate) headMeta(l Loadout) int32 {
	for slot, c := range l.Items {
		cand := s.pool.candidate(proto.ItemSlot(slot), c.ItemID)
		if cand == nil {
			continue
		}
		for i, color := range cand.Sockets {
			if color == proto.GemColor_GemColorMeta && i < MaxGems && c.Gems[i] != 0 {
				return c.Gems[i]
			}
		}
	}
	return 0
}

// candBounds bounds what a candidate in slot adds to J, whatever the rest of the gear: its stats,
// its best enchant, reforge and gems, and its residuals, under every slope in the box. Residuals
// count at 2 standard errors. ok is false when a residual it needs is missing.
func (s *surrogate) candBounds(slot proto.ItemSlot, cand *Candidate, lo, hi stats.Stats, gb *gemBounds) (lb, ub float64, ok bool) {
	lb, ub = s.candStatBounds(slot, cand, lo, hi, gb)
	if s.unpriced[familySlot(slot)][cand.Item.ID] {
		return lb, ub, false
	}
	if r, measured := s.items[familySlot(slot)][cand.Item.ID]; measured {
		lb += r.Mean - 2*r.SE
		ub += r.Mean + 2*r.SE
	} else if cand.NeedsSim {
		return lb, ub, false
	}
	if i, inSet := s.setOf[cand.Item.ID]; inSet {
		jumpLo, jumpHi := s.sets[i].jumpBounds()
		lb, ub = lb+jumpLo, ub+jumpHi
	}
	return lb, ub, true
}

// candStatBounds is candBounds without the item's own residual: stats, enchants (effects included),
// reforges and gems.
func (s *surrogate) candStatBounds(slot proto.ItemSlot, cand *Candidate, lo, hi stats.Stats, gb *gemBounds) (lb, ub float64) {
	lb, ub = boxLow(lo, hi, cand.Item.Stats), boxHigh(lo, hi, cand.Item.Stats)

	enchLB, enchUB := 0.0, 0.0
	for _, e := range cand.Enchants {
		if s.unpricedEnchants[slot][e.ID] {
			continue
		}
		r := s.enchants[slot][e.ID]
		enchLB = max(enchLB, boxLow(lo, hi, e.Stats)+r.Mean-2*r.SE)
		enchUB = max(enchUB, boxHigh(lo, hi, e.Stats)+r.Mean+2*r.SE)
	}
	reforgeLB, reforgeUB := 0.0, 0.0
	for _, r := range cand.Reforges {
		reforgeLB = max(reforgeLB, boxLow(lo, hi, r.Stats))
		reforgeUB = max(reforgeUB, boxHigh(lo, hi, r.Stats))
	}
	gemLB, gemUB := gb.bounds(cand, lo, hi)
	return lb + enchLB + reforgeLB + gemLB, ub + enchUB + reforgeUB + gemUB
}

// gemBounds holds per-socket bounds on gem value over the slope box.
type gemBounds struct {
	freeLB  map[proto.GemColor]float64
	anyLB   float64
	allUB   map[proto.GemColor]float64
	anyUB   float64
	metaUB  float64
	hasFree bool
}

func (s *surrogate) newGemBounds(lo, hi stats.Stats) *gemBounds {
	gb := &gemBounds{freeLB: map[proto.GemColor]float64{}, allUB: map[proto.GemColor]float64{}}
	for _, g := range s.pool.Gems {
		if s.unpricedGems[g.Gem.ID] {
			continue
		}
		r := s.gems[g.Gem.ID]
		glb := boxLow(lo, hi, g.Gem.Stats) + r.Mean - 2*r.SE
		gub := boxHigh(lo, hi, g.Gem.Stats) + r.Mean + 2*r.SE
		if g.Gem.Color == proto.GemColor_GemColorMeta {
			gb.metaUB = max(gb.metaUB, gub)
			continue
		}
		gb.anyUB = max(gb.anyUB, gub)
		free := s.freeGem(g)
		if free {
			gb.anyLB = max(gb.anyLB, glb)
			gb.hasFree = true
		}
		for _, color := range socketColors {
			if !core.ColorIntersects(color, g.Gem.Color) {
				continue
			}
			gb.allUB[color] = max(gb.allUB[color], gub)
			if free {
				if cur, ok := gb.freeLB[color]; !ok || glb > cur {
					gb.freeLB[color] = glb
				}
			}
		}
	}
	return gb
}

// bounds: the lower bound is the better of two gemmings any loadout can have (best free gem
// everywhere, or matching colors for the bonus); the upper bound takes every socket's best gem and
// the bonus. A meta socket only counts in the upper bound, since its meta may not activate.
func (gb *gemBounds) bounds(cand *Candidate, lo, hi stats.Stats) (lb, ub float64) {
	own := len(cand.Item.GemSockets)
	anyLB, matchLB, matchOK := 0.0, 0.0, own > 0
	for i, color := range cand.Sockets {
		if i >= MaxGems {
			break
		}
		if color == proto.GemColor_GemColorMeta {
			ub += gb.metaUB
			if i < own {
				matchOK = false
			}
			continue
		}
		ub += max(gb.anyUB, gb.allUB[color])
		if gb.hasFree {
			anyLB += gb.anyLB
		}
		if v, ok := gb.freeLB[color]; ok && i < own {
			matchLB += v
		} else {
			if i < own {
				matchOK = false
			}
			if gb.hasFree {
				matchLB += gb.anyLB
			}
		}
	}
	lb = anyLB
	if matchOK {
		lb = max(lb, matchLB+boxLow(lo, hi, cand.Item.SocketBonus))
	}
	ub += max(0, boxHigh(lo, hi, cand.Item.SocketBonus))
	return lb, ub
}

// statRanges is how far each gear stat can move from the seed's across the pool: per slot, the
// lowest and highest any candidate reaches with some enchant, reforge and gems, less the seed's.
func (s *surrogate) statRanges() map[stats.Stat]ResponseRange {
	var gemLo, gemHi, metaLo, metaHi stats.Stats
	for _, g := range s.pool.Gems {
		lo, hi := &gemLo, &gemHi
		if g.Gem.Color == proto.GemColor_GemColorMeta {
			lo, hi = &metaLo, &metaHi
		}
		for st, x := range g.Gem.Stats {
			lo[st], hi[st] = min(lo[st], x), max(hi[st], x)
		}
	}

	var totalLo, totalHi stats.Stats
	for slot := range s.pool.Slots {
		if s.pool.Locked[slot] {
			continue
		}
		seedStats := s.choiceStats(proto.ItemSlot(slot), s.seed.Items[slot])
		slotLo, slotHi := seedStats, seedStats
		for _, cand := range s.pool.Slots[slot] {
			var lo, hi stats.Stats
			for st := range lo {
				x := cand.Item.Stats[st]
				lo[st], hi[st] = x, x
			}
			addRange := func(xs []stats.Stats) {
				var l, h stats.Stats
				for _, x := range xs {
					for st, v := range x {
						l[st], h[st] = min(l[st], v), max(h[st], v)
					}
				}
				lo, hi = lo.Add(l), hi.Add(h)
			}
			enchants := make([]stats.Stats, 0, len(cand.Enchants))
			for _, e := range cand.Enchants {
				enchants = append(enchants, e.Stats)
			}
			addRange(enchants)
			reforges := make([]stats.Stats, 0, len(cand.Reforges))
			for _, r := range cand.Reforges {
				reforges = append(reforges, r.Stats)
			}
			addRange(reforges)
			for _, color := range cand.Sockets {
				if color == proto.GemColor_GemColorMeta {
					lo, hi = lo.Add(metaLo), hi.Add(metaHi)
				} else {
					lo, hi = lo.Add(gemLo), hi.Add(gemHi)
				}
			}
			addRange([]stats.Stats{cand.Item.SocketBonus})
			for st := range lo {
				slotLo[st], slotHi[st] = min(slotLo[st], lo[st]), max(slotHi[st], hi[st])
			}
		}
		totalLo = totalLo.Add(slotLo.Subtract(seedStats))
		totalHi = totalHi.Add(slotHi.Subtract(seedStats))
	}

	ranges := map[stats.Stat]ResponseRange{}
	for st := stats.Stat(0); st < stats.Len; st++ {
		lo, hi := math.Round(totalLo[st]), math.Round(totalHi[st])
		if lo < 0 || hi > 0 {
			ranges[st] = ResponseRange{Min: min(lo, 0), Max: max(hi, 0)}
		}
	}
	return ranges
}

// setFloors approximates each floored sheet stat as linear in gear stats around the seed, from
// character sheets with each gear stat moved by 100.
func (s *surrogate) setFloors(floors []*proto.StatMinimum, gearStats []stats.Stat) error {
	if len(floors) == 0 {
		return nil
	}
	seedSheet, err := s.pool.finalStats(s.seed)
	if err != nil {
		return err
	}
	const step = 100.0
	gradients := make([]stats.Stats, len(floors))
	for _, st := range gearStats {
		var off stats.Stats
		off[st] = step
		sheet, err := playerSheet(s.pool.base, s.pool.targetIndex, s.seed, off)
		if err != nil {
			return err
		}
		for i, f := range floors {
			gradients[i][st] = (sheet.FinalStats[f.Stat] - seedSheet[f.Stat]) / step
		}
	}
	maxSlope := 0.0
	for _, st := range s.curved {
		maxSlope = max(maxSlope, math.Abs(slices.Min(s.curves[st].slope)), math.Abs(slices.Max(s.curves[st].slope)))
	}
	s.floorWeight = 100 * (maxSlope + 1)
	for i, f := range floors {
		s.floors = append(s.floors, floorTerm{stat: stats.Stat(f.Stat), min: f.MinValue, seed: seedSheet[f.Stat], gradient: gradients[i]})
	}
	return nil
}

// weaponDPS is a weapon's average damage per second; 0 for anything else.
func weaponDPS(item *core.Item) float64 {
	if item.WeaponDamageMax <= 0 || item.SwingSpeed <= 0 {
		return 0
	}
	return (item.WeaponDamageMin + item.WeaponDamageMax) / 2 / item.SwingSpeed
}

// measureCurves measures response curves for the stats the pool moves. Each stat first gets one
// screening sim at the far end of its range (both ends for a cap-prone stat, whose value can sit on
// either side of its cap); a stat that doesn't move J there gets no curve. The screening offsets are
// knots MeasureResponse sims anyway, so the evaluator extends them rather than starting over.
func (r *run) measureCurves() (Response, []stats.Stat, error) {
	seed := r.r.Seed
	ranges := newSurrogate(r.pool, seed, nil).statRanges()
	var rangeStats []stats.Stat
	type screened struct {
		stat   stats.Stat
		offset float64
	}
	var owners []screened
	points := []Point{{Loadout: seed}}
	for st := stats.Stat(0); st < stats.Len; st++ {
		rng, ok := ranges[st]
		if !ok {
			continue
		}
		rangeStats = append(rangeStats, st)
		var ends []float64
		switch {
		case capProneStats[st]:
			for _, end := range []float64{rng.Min, rng.Max} {
				if end != 0 {
					ends = append(ends, end)
				}
			}
		case -rng.Min > rng.Max:
			ends = []float64{rng.Min}
		default:
			ends = []float64{rng.Max}
		}
		for _, end := range ends {
			points = append(points, offsetPoint(seed, st, end))
			owners = append(owners, screened{st, end})
		}
	}
	evals, err := r.evaluate(points, r.screen)
	if err != nil {
		return nil, nil, fmt.Errorf("stat screening sims: %w", err)
	}
	relevant := map[stats.Stat]ResponseRange{}
	for i, o := range owners {
		if evals[i+1] == nil {
			continue
		}
		if d := r.obj.Delta(evals[0], evals[i+1]); abs(d.Mean) > 2*d.SE {
			relevant[o.stat] = ranges[o.stat]
		}
	}

	for {
		resp, err := MeasureResponse(r.ctx, r.eval, r.obj, seed, relevant, r.budget.Iterations)
		if err == nil {
			err = r.addKnots(resp, relevant)
		}
		var simErr *SimError
		if !errors.As(err, &simErr) {
			return resp, rangeStats, err
		}
		dropped := false
		for st := range relevant {
			if simErr.Point.Offset[st] != 0 {
				r.warn("left %s out of the stat curves: its sim failed: %s", st.StatName(), firstLine(simErr.Message))
				delete(relevant, st)
				dropped = true
			}
		}
		if !dropped {
			return nil, nil, err
		}
	}
}

// knotFractions are where addKnots sims, as fractions of the way to each end of a stat's range, by
// effort. Cap-prone stats already have knots every quarter.
func knotFractions(effort proto.OptimizerEffort, capProne bool) []float64 {
	switch {
	case effort == proto.OptimizerEffort_OptimizerEffortQuick && capProne:
		return nil
	case effort == proto.OptimizerEffort_OptimizerEffortQuick:
		return []float64{1.0 / 8}
	case capProne:
		return []float64{1.0 / 16}
	}
	return []float64{1.0 / 4, 1.0 / 16}
}

// minKnotOffset is the smallest offset worth a knot of its own.
const minKnotOffset = 5

// addKnots sims each curve closer to the seed than MeasureResponse does. It only sims the ends of a
// stat's range, which the pool's extremes set, and a stat's value can bend well inside it: a Ret
// paladin's agility is worth 1.5 a point near the seed but 0.66 on average out to the 2300 its
// range reaches, where no loadout goes. The new knots go into the curves' Knots; the fits stay.
func (r *run) addKnots(resp Response, ranges map[stats.Stat]ResponseRange) error {
	seed := r.r.Seed
	type owner struct {
		stat   stats.Stat
		offset float64
	}
	var owners []owner
	points := []Point{{Loadout: seed}}
	for _, st := range statsOf(resp) {
		for _, end := range []float64{ranges[st].Min, ranges[st].Max} {
			for _, f := range knotFractions(r.asked.Settings.GetEffort(), capProneStats[st]) {
				off := math.Round(end * f)
				near := func(k Knot) bool { return abs(k.Offset-off) < abs(off)/4 }
				if abs(off) < minKnotOffset || slices.ContainsFunc(resp[st].Knots, near) {
					continue
				}
				points = append(points, offsetPoint(seed, st, off))
				owners = append(owners, owner{st, off})
			}
		}
	}
	if len(owners) == 0 {
		return nil
	}
	evals, err := r.eval.Evaluate(r.ctx, points, r.budget.Iterations)
	if err != nil {
		return fmt.Errorf("knot sims: %w", err)
	}
	for i, o := range owners {
		c := resp[o.stat]
		c.Knots = append(c.Knots, Knot{Offset: o.offset, Delta: r.obj.Delta(evals[0], evals[i+1])})
		sortKnots(c.Knots)
	}
	return nil
}

// effectKind is what a residual sim prices.
type effectKind int

const (
	effectItem effectKind = iota
	effectEnchant
	effectGem
	effectSetTwo
	effectSetFour
	// A ring or trinket pair; the key's id indexes effects.pairList.
	effectPair
)

type effectKey struct {
	kind effectKind
	// The family slot for items, the slot for enchants.
	slot proto.ItemSlot
	// Item, enchant or gem id, or the set's index in surrogate.sets.
	id int32
}

// effectFamily is a batch of residual sims against one base loadout.
type effectFamily struct {
	name     string
	base     Loadout
	keys     []effectKey
	variants []Loadout
	// Screening order when the budget can't cover every variant: higher first.
	prior []float64
	// The variant also emptied the base's off hand, so its residual includes losing it.
	emptiedOH []bool
	// Weapons, whose damage a probe may show doesn't matter.
	weapons bool
	// Each variant is simmed against base with the variant's stat change as bonus stats, not
	// against base plus the curves (see measureMatched).
	matched bool
}

func (f *effectFamily) add(key effectKey, variant Loadout, prior float64, emptiedOH bool) {
	f.keys = append(f.keys, key)
	f.variants = append(f.variants, variant)
	f.prior = append(f.prior, prior)
	f.emptiedOH = append(f.emptiedOH, emptiedOH)
}

func (f *effectFamily) subset(keep func(i int) bool) *effectFamily {
	out := &effectFamily{name: f.name, base: f.base, weapons: f.weapons, matched: f.matched}
	for i := range f.keys {
		if keep(i) {
			out.add(f.keys[i], f.variants[i], f.prior[i], f.emptiedOH[i])
		}
	}
	return out
}

// effects is what the residual sims found, before the surrogate takes it in.
type effects struct {
	raw map[effectKey]Estimate
	// Where each key was measured, to measure it again at more iterations.
	family  map[effectKey]*effectFamily
	variant map[effectKey]Loadout
	// Keys whose residual includes losing the seed's off hand.
	emptiedOH map[effectKey]bool
	// Weapon families whose damage doesn't matter: every weapon without an effect is worth what the
	// probes averaged.
	flat map[proto.ItemSlot]Estimate
	// Keys that were never measured, or whose sims failed.
	missing map[effectKey]bool
	// The empty-pair bases of the ring and trinket families, by first slot, and the pairs simmed on
	// them.
	pairBase map[proto.ItemSlot]Loadout
	pairList []pairKey
}

// itemNeedsSim is whether an item alone, whatever its gems and enchant, is worth more than its stats.
func itemNeedsSim(id int32) bool {
	if id == 0 {
		return false
	}
	if ItemHasEffect(id) {
		return true
	}
	item, ok := core.LookupItem(id)
	return !ok || item.WeaponDamageMax > 0
}

// fillsHands is whether nothing can go in the target's off hand next to the item.
func (p *Pool) fillsHands(item *core.Item) bool {
	if item.Type != proto.ItemType_ItemTypeWeapon {
		return false
	}
	return fillsBothHands(item) || item.HandType == proto.HandType_HandTypeTwoHand && !p.player.titansGrip
}

// measureEffects prices, with sims against the seed, whatever the curves can't: item effects,
// weapon damage, effect enchants and gems, and set bonuses. Every sim is a residual, the paired delta
// minus what the curves make of the stat change (for set bonuses, the delta from a stat-matched
// twin), and each family of them runs in parallel.
//
// It goes in three waves:
//  1. Weapon probes: the fastest and slowest weapon without an effect in each weapon family. When
//     they're worth the same, damage doesn't matter there (a caster's melee weapon), and only
//     weapons with effects need sims of their own.
//  2. Screening, at r.screen iterations: everything else. When the budget can't cover it, each
//     family keeps its most promising variants, and the rest can't be picked.
//  3. Refinement, at full iterations: the items that survive pruning, most promising first, every
//     enchant, gem and set residual, and pairs of the best ring and trinket effects, whose procs
//     may not stack.
func (r *run) measureEffects(s *surrogate) error {
	v0 := s.slopes(s.seedStats)
	gp0 := s.newGemPlan(v0)
	fx := &effects{
		raw:       map[effectKey]Estimate{},
		family:    map[effectKey]*effectFamily{},
		variant:   map[effectKey]Loadout{},
		emptiedOH: map[effectKey]bool{},
		flat:      map[proto.ItemSlot]Estimate{},
		missing:   map[effectKey]bool{},
		pairBase:  map[proto.ItemSlot]Loadout{},
	}

	families := r.effectFamilies(s, v0, gp0, fx)
	for _, f := range families {
		for i, k := range f.keys {
			fx.family[k] = f
			fx.variant[k] = f.variants[i]
			if f.emptiedOH[i] {
				fx.emptiedOH[k] = true
			}
		}
	}

	// wave 1: weapon probes
	var probes []*effectFamily
	probeKeys := map[*effectFamily][2]int{}
	for _, f := range families {
		if !f.weapons {
			continue
		}
		fast, slow := -1, -1
		dps := make([]float64, len(f.keys))
		for i, k := range f.keys {
			item, ok := core.LookupItem(k.id)
			if !ok || ItemHasEffect(k.id) || item.WeaponDamageMax <= 0 {
				continue
			}
			dps[i] = weaponDPS(&item)
			if fast < 0 || dps[i] > dps[fast] {
				fast = i
			}
			if slow < 0 || dps[i] < dps[slow] {
				slow = i
			}
		}
		if fast < 0 || slow == fast || dps[fast] <= 1.2*dps[slow] {
			continue
		}
		probeKeys[f] = [2]int{fast, slow}
		probes = append(probes, f.subset(func(i int) bool { return i == fast || i == slow }))
	}
	if err := r.measureFamilies(s, fx, probes, r.screen); err != nil {
		return err
	}
	var screen []*effectFamily
	for _, f := range families {
		if pk, probed := probeKeys[f]; probed {
			fast, slow := f.keys[pk[0]], f.keys[pk[1]]
			a, okA := fx.raw[fast]
			b, okB := fx.raw[slow]
			if okA && okB && abs(a.Mean-b.Mean) <= 2*math.Hypot(a.SE, b.SE)+acceptFraction*abs(r.seedJ.Mean) {
				fx.flat[fast.slot] = Estimate{Mean: (a.Mean + b.Mean) / 2, SE: max(a.SE, b.SE)}
				f = f.subset(func(i int) bool { return ItemHasEffect(f.keys[i].id) })
			}
		}
		f = f.subset(func(i int) bool { _, done := fx.raw[f.keys[i]]; return !done })
		if len(f.keys) > 0 {
			screen = append(screen, f)
		}
	}

	// wave 2: screening, trimmed to the budget
	screen = r.trimToBudget(screen, fx, r.screen, screenShare*float64(r.total))
	if err := r.measureFamilies(s, fx, screen, r.screen); err != nil {
		return err
	}
	s.applyEffects(fx)

	// wave 3: refinement
	if r.budget.Iterations > r.screen {
		contenders := newSearcher(s, r).contenders()
		var refine []*effectFamily
		for _, f := range families {
			sub := f.subset(func(i int) bool {
				k := f.keys[i]
				_, measured := fx.raw[k]
				_, contending := contenders[k]
				return measured && (contending || k.kind != effectItem)
			})
			for i, k := range sub.keys {
				sub.prior[i] = contenders[k]
			}
			if len(sub.keys) > 0 {
				refine = append(refine, sub)
			}
		}
		refine = r.trimToBudget(refine, fx, r.budget.Iterations, refineShare*float64(r.total))
		refine = append(refine, r.pairFamilies(fx)...)
		if err := r.measureFamilies(s, fx, refine, r.budget.Iterations); err != nil {
			return err
		}
		s.applyEffects(fx)
	}
	if traceHook != nil {
		for k, corr := range s.pairs {
			if corr != 0 {
				r.trace("pair %v: %+.1f beyond its items", k, corr)
			}
		}
		for _, f := range families {
			r.trace("family %s: %d variants", f.name, len(f.keys))
		}
		for slot := range s.items {
			if len(s.items[slot]) > 0 || len(s.unpriced[slot]) > 0 {
				r.trace("residuals %s: %d priced, %d unpriced", proto.ItemSlot(slot), len(s.items[slot]), len(s.unpriced[slot]))
			}
		}
		for slot, flat := range fx.flat {
			r.trace("weapon damage doesn't matter in %s: %+v", slot, flat)
		}
	}
	return nil
}

// Shares of the run's budget the effect sims may take, and what they leave for the stages after the
// search whatever the curves took: verification, its race and the neighborhood.
const (
	screenShare  = 0.35
	refineShare  = 0.10
	reserveShare = verifyShare + raceShare + 0.05
)

// trimToBudget keeps as many variants as fit in budget iterations. Families with a handful of item
// variants, and families of enchants, gems and sets, keep them all; the rest keep the same fraction
// each, best prior first. The variants cut and never measured are marked missing, so the search
// leaves them alone.
func (r *run) trimToBudget(families []*effectFamily, fx *effects, iterations int, budget float64) []*effectFamily {
	cost := 0.0
	for _, f := range families {
		cost += float64(r.eval.cost(r.pool.familyPoints(f.base, f.variants, f.matched), iterations))
	}
	budget = max(0, min(budget, float64(r.remaining())-reserveShare*float64(r.total)))
	if cost <= budget || cost == 0 {
		return families
	}
	fraction := max(0, budget/cost)
	var out []*effectFamily
	for _, f := range families {
		if len(f.keys) <= 3 || f.keys[0].kind != effectItem {
			out = append(out, f)
			continue
		}
		keep := int(math.Ceil(fraction * float64(len(f.keys))))
		order := make([]int, len(f.keys))
		for i := range order {
			order[i] = i
		}
		slices.SortStableFunc(order, func(a, b int) int {
			switch {
			case f.prior[a] > f.prior[b]:
				return -1
			case f.prior[a] < f.prior[b]:
				return 1
			}
			return 0
		})
		kept := map[int]bool{}
		for _, i := range order[:keep] {
			kept[i] = true
		}
		for i, k := range f.keys {
			if _, measured := fx.raw[k]; !kept[i] && !measured {
				fx.missing[k] = true
			}
		}
		if sub := f.subset(func(i int) bool { return kept[i] }); len(sub.keys) > 0 {
			out = append(out, sub)
		}
	}
	return out
}

// familyPoints are the sims a family's residuals take: base then each variant, or for a matched
// family each variant's twin (base with the variant's stat change as bonus stats) then the variant.
func (p *Pool) familyPoints(base Loadout, variants []Loadout, matched bool) []Point {
	if !matched {
		return append([]Point{{Loadout: base}}, pointsOf(variants)...)
	}
	baseStats := p.gearStats(base)
	points := make([]Point, 0, 2*len(variants))
	for _, v := range variants {
		points = append(points, Point{Loadout: base, Offset: p.gearStats(v).Subtract(baseStats)}, Point{Loadout: v})
	}
	return points
}

// measureResiduals is MeasureResiduals with the surrogate's curves, so a residual holds what they
// miss and nothing the response curve's own fit would add.
func (r *run) measureResiduals(s *surrogate, base Loadout, variants []Loadout, iterations int) ([]Estimate, error) {
	evals, err := r.eval.Evaluate(r.ctx, s.pool.familyPoints(base, variants, false), iterations)
	if err != nil {
		return nil, fmt.Errorf("residual sims: %w", err)
	}
	baseValue := s.curveValue(s.pool.gearStats(base).Subtract(s.seedStats))
	out := make([]Estimate, len(variants))
	for i, v := range variants {
		simmed := r.obj.Delta(evals[0], evals[i+1])
		predicted := s.curveValue(s.pool.gearStats(v).Subtract(s.seedStats)) - baseValue
		out[i] = Estimate{Mean: simmed.Mean - predicted, SE: simmed.SE}
	}
	return out, nil
}

// measureMatched prices each variant against its twin from familyPoints. What's left is only what
// the stats don't show, with no curve error in it, and the two share nearly every random number.
func (r *run) measureMatched(base Loadout, variants []Loadout, iterations int) ([]Estimate, error) {
	evals, err := r.eval.Evaluate(r.ctx, r.pool.familyPoints(base, variants, true), iterations)
	if err != nil {
		return nil, fmt.Errorf("residual sims: %w", err)
	}
	out := make([]Estimate, len(variants))
	for i := range variants {
		out[i] = r.obj.Delta(evals[2*i], evals[2*i+1])
	}
	return out, nil
}

func pointsOf(loadouts []Loadout) []Point {
	out := make([]Point, len(loadouts))
	for i, l := range loadouts {
		out[i] = Point{Loadout: l}
	}
	return out
}

// measureFamilies runs every family's residual sims at once, so the evaluator's workers stay busy.
// A variant whose sim fails is dropped with a warning and its family asks again; a failed base loses
// its whole family.
func (r *run) measureFamilies(s *surrogate, fx *effects, families []*effectFamily, iterations int) error {
	var (
		mu       sync.Mutex
		firstErr error
	)
	// every family at once: the evaluator's own workers bound the sims
	runParallel(len(families), len(families), func(i int) {
		f := families[i]
		keys, variants := slices.Clone(f.keys), slices.Clone(f.variants)
		for len(variants) > 0 {
			var res []Estimate
			var err error
			if f.matched {
				res, err = r.measureMatched(f.base, variants, iterations)
			} else {
				res, err = r.measureResiduals(s, f.base, variants, iterations)
			}
			var simErr *SimError
			if errors.As(err, &simErr) {
				i := slices.IndexFunc(variants, func(l Loadout) bool { return simErr.Point == Point{Loadout: l} })
				if i < 0 && f.matched {
					// a failed twin loses its variant only
					if j := slices.Index(r.pool.familyPoints(f.base, variants, true), simErr.Point); j >= 0 {
						i = j / 2
					}
				}
				mu.Lock()
				if i < 0 {
					r.warn("couldn't price %s: its base loadout's sim failed: %s", f.name, firstLine(simErr.Message))
					for _, k := range keys {
						fx.missing[k] = true
					}
					mu.Unlock()
					return
				}
				r.warn("dropped %s from %s: its sim failed: %s", describeKey(keys[i]), f.name, firstLine(simErr.Message))
				fx.missing[keys[i]] = true
				mu.Unlock()
				keys, variants = slices.Delete(keys, i, i+1), slices.Delete(variants, i, i+1)
				continue
			}
			mu.Lock()
			if err != nil {
				if firstErr == nil {
					firstErr = err
				}
			} else {
				for i, k := range keys {
					fx.raw[k] = res[i]
					delete(fx.missing, k)
				}
			}
			mu.Unlock()
			return
		}
	})
	return firstErr
}

func describeKey(k effectKey) string {
	switch k.kind {
	case effectEnchant:
		return fmt.Sprintf("enchant %d", k.id)
	case effectGem:
		return fmt.Sprintf("gem %d", k.id)
	case effectSetTwo, effectSetFour:
		return fmt.Sprintf("set bonus %d", k.id)
	case effectPair:
		return fmt.Sprintf("pair %d", k.id)
	}
	return fmt.Sprintf("item %d", k.id)
}

// applyEffects takes the measured residuals into the surrogate. Main-hand weapons that had to empty
// the seed's off hand get back what that off hand was worth, so every main hand counts from the same
// place. Anything a family should have priced but didn't is unpriced.
func (s *surrogate) applyEffects(fx *effects) {
	oh := proto.ItemSlot_ItemSlotOffHand
	seedOH := s.seed.Items[oh].ItemID
	for k, est := range fx.raw {
		switch k.kind {
		case effectItem:
			if fx.emptiedOH[k] && itemNeedsSim(seedOH) {
				ohRes := fx.raw[effectKey{effectItem, oh, seedOH}]
				est = Estimate{Mean: est.Mean + ohRes.Mean, SE: math.Hypot(est.SE, ohRes.SE)}
			}
			s.items[k.slot][k.id] = est
			delete(s.unpriced[k.slot], k.id)
		case effectEnchant:
			s.enchants[k.slot][k.id] = est
			delete(s.unpricedEnchants[k.slot], k.id)
		case effectGem:
			s.gems[k.id] = est
			delete(s.unpricedGems, k.id)
		}
	}
	for k := range fx.missing {
		switch k.kind {
		case effectItem:
			s.unpriced[k.slot][k.id] = true
		case effectEnchant:
			s.unpricedEnchants[k.slot][k.id] = true
		case effectGem:
			s.unpricedGems[k.id] = true
		}
	}
	// weapons without an effect, in a family where damage doesn't matter
	for slot, flat := range fx.flat {
		for _, fs := range []proto.ItemSlot{slot, pairedSlot(slot)} {
			for _, cand := range s.pool.Slots[fs] {
				if cand.Item.WeaponDamageMax <= 0 || ItemHasEffect(cand.Item.ID) {
					continue
				}
				if _, measured := fx.raw[effectKey{effectItem, slot, cand.Item.ID}]; !measured {
					s.items[slot][cand.Item.ID] = flat
					delete(s.unpriced[slot], cand.Item.ID)
				}
			}
		}
	}
	// ring and trinket pairs, when they differ from their items' sum by more than the noise
	for i, k := range fx.pairList {
		est, ok := fx.raw[effectKey{effectPair, k.slot, int32(i)}]
		if !ok {
			continue
		}
		a, b := s.items[k.slot][k.a], s.items[k.slot][k.b]
		corr := est.Mean - a.Mean - b.Mean
		if abs(corr) > 2*math.Sqrt(est.SE*est.SE+a.SE*a.SE+b.SE*b.SE) {
			s.pairs[k] = corr
		} else {
			delete(s.pairs, k)
		}
	}
	// set bonuses, less what their pieces are worth on their own
	for i, set := range s.sets {
		for _, kind := range []effectKind{effectSetTwo, effectSetFour} {
			k := effectKey{kind, 0, int32(i)}
			est, ok := fx.raw[k]
			if !ok {
				continue
			}
			base := fx.family[k].base
			for slot, c := range fx.variant[k].Items {
				if c != base.Items[slot] {
					est.Mean -= s.choiceResidual(proto.ItemSlot(slot), c) - s.choiceResidual(proto.ItemSlot(slot), base.Items[slot])
				}
			}
			if kind == effectSetTwo {
				set.two, set.hasTwo = est, true
			} else {
				set.four, set.hasFour = est, true
			}
		}
	}
}

// pairCandidates is how many of the best ring and trinket effects get simmed together, by effort.
func pairCandidates(effort proto.OptimizerEffort) int {
	switch effort {
	case proto.OptimizerEffort_OptimizerEffortQuick:
		return 4
	case proto.OptimizerEffort_OptimizerEffortThorough:
		return 8
	}
	return 6
}

// pairFamilies sims every pair of the ring and trinket effects with the highest residuals, and the
// seed's own pair, on the families' empty-pair bases. Two procs that share an aura or cooldown are
// worth less together than apart (the Darkmoon Card: Greatness variants all proc one aura).
func (r *run) pairFamilies(fx *effects) []*effectFamily {
	type single struct {
		id     int32
		res    float64
		choice ItemChoice
	}
	var out []*effectFamily
	for _, a := range pairSlots {
		base, ok := fx.pairBase[a]
		if !ok {
			continue
		}
		var singles []single
		for k, est := range fx.raw {
			if k.kind == effectItem && k.slot == a && ItemHasEffect(k.id) {
				singles = append(singles, single{k.id, est.Mean, fx.variant[k].Items[a]})
			}
		}
		slices.SortFunc(singles, func(x, y single) int {
			switch {
			case x.res > y.res:
				return -1
			case x.res < y.res:
				return 1
			}
			return int(x.id) - int(y.id)
		})
		top := singles[:min(len(singles), pairCandidates(r.asked.Settings.GetEffort()))]
		pairs := map[pairKey][2]single{}
		for i := range top {
			for j := i + 1; j < len(top); j++ {
				pairs[newPairKey(a, top[i].id, top[j].id)] = [2]single{top[i], top[j]}
			}
		}
		seedA, seedB := r.r.Seed.Items[a], r.r.Seed.Items[a+1]
		if ItemHasEffect(seedA.ItemID) && ItemHasEffect(seedB.ItemID) && seedA.ItemID != seedB.ItemID {
			pairs[newPairKey(a, seedA.ItemID, seedB.ItemID)] = [2]single{{id: seedA.ItemID, choice: seedA}, {id: seedB.ItemID, choice: seedB}}
		}
		keys := make([]pairKey, 0, len(pairs))
		for k := range pairs {
			keys = append(keys, k)
		}
		slices.SortFunc(keys, func(x, y pairKey) int {
			if x.a != y.a {
				return int(x.a) - int(y.a)
			}
			return int(x.b) - int(y.b)
		})
		f := &effectFamily{name: a.String() + " pairs", base: base}
		for _, k := range keys {
			pair := pairs[k]
			variant := base
			variant.Items[a], variant.Items[a+1] = pair[0].choice, pair[1].choice
			f.add(effectKey{effectPair, a, int32(len(fx.pairList))}, variant, 0, false)
			fx.pairList = append(fx.pairList, k)
		}
		if len(f.keys) > 0 {
			out = append(out, f)
		}
	}
	return out
}

// pairedSlot is the other slot of a ring or trinket pair; the slot itself otherwise.
func pairedSlot(slot proto.ItemSlot) proto.ItemSlot {
	switch slot {
	case proto.ItemSlot_ItemSlotFinger1:
		return proto.ItemSlot_ItemSlotFinger2
	case proto.ItemSlot_ItemSlotFinger2:
		return proto.ItemSlot_ItemSlotFinger1
	case proto.ItemSlot_ItemSlotTrinket1:
		return proto.ItemSlot_ItemSlotTrinket2
	case proto.ItemSlot_ItemSlotTrinket2:
		return proto.ItemSlot_ItemSlotTrinket1
	}
	return slot
}

// setScreen is how much J, as a fraction of the seed's, wearing a set's best pieces may cost in stats
// before its bonuses aren't worth a sim.
const setScreen = 0.05

// singleSlots are the slots with one item each and no weapon pairing.
var singleSlots = []proto.ItemSlot{
	proto.ItemSlot_ItemSlotHead, proto.ItemSlot_ItemSlotNeck, proto.ItemSlot_ItemSlotShoulder,
	proto.ItemSlot_ItemSlotBack, proto.ItemSlot_ItemSlotChest, proto.ItemSlot_ItemSlotWrist,
	proto.ItemSlot_ItemSlotHands, proto.ItemSlot_ItemSlotWaist, proto.ItemSlot_ItemSlotLegs,
	proto.ItemSlot_ItemSlotFeet, proto.ItemSlot_ItemSlotRanged,
}

// metaSocket is the index of the item's meta socket, or -1.
func metaSocket(cand *Candidate) int {
	for i, color := range cand.Sockets {
		if color == proto.GemColor_GemColorMeta && i < MaxGems {
			return i
		}
	}
	return -1
}

// keepMeta is quickChoice's meta for a variant of base: base's meta, or none when base has none, so a
// residual never picks up a meta's effect.
func (s *surrogate) keepMeta(base Loadout) int32 {
	if meta := s.headMeta(base); meta != 0 {
		return meta
	}
	return -1
}

// effectFamilies lists the residual sims the pool needs. Items, enchants and gems it can't set up a
// sim for are marked missing in fx.
func (r *run) effectFamilies(s *surrogate, v stats.Stats, gp *gemPlan, fx *effects) []*effectFamily {
	p, seed := s.pool, s.seed
	lo, hi := s.slopeBox()
	gb := s.newGemBounds(lo, hi)
	statLB := func(slot proto.ItemSlot, cand *Candidate) float64 {
		lb, _ := s.candStatBounds(slot, cand, lo, hi, gb)
		return lb
	}
	prior := func(slot proto.ItemSlot, cand *Candidate) float64 {
		// the seed's own items always get their sims, so the search can rebuild the seed
		if id := cand.Item.ID; id == seed.Items[slot].ItemID || id == seed.Items[pairedSlot(slot)].ItemID {
			return math.Inf(1)
		}
		if dps := weaponDPS(&cand.Item); dps > 0 {
			return dps
		}
		_, ub := s.candStatBounds(slot, cand, lo, hi, gb)
		return float64(cand.Catalog.GetProgressionTier())*1e6 + ub
	}
	quick := func(slot proto.ItemSlot, cand *Candidate, base Loadout) ItemChoice {
		return s.quickChoice(slot, cand, v, gp, s.keepMeta(base))
	}
	var out []*effectFamily
	add := func(f *effectFamily) {
		if len(f.keys) > 0 {
			out = append(out, f)
		}
	}

	// rings and trinkets, from an empty pair
	for _, pair := range [][2]proto.ItemSlot{
		{proto.ItemSlot_ItemSlotFinger1, proto.ItemSlot_ItemSlotFinger2},
		{proto.ItemSlot_ItemSlotTrinket1, proto.ItemSlot_ItemSlotTrinket2},
	} {
		a, b := pair[0], pair[1]
		if p.Locked[a] && p.Locked[b] {
			continue
		}
		base, put := seed, a
		switch {
		case p.Locked[a]:
			base.Items[b], put = ItemChoice{}, b
		case p.Locked[b]:
			base.Items[a] = ItemChoice{}
		default:
			base.Items[a], base.Items[b] = ItemChoice{}, ItemChoice{}
		}
		if !p.Locked[a] && !p.Locked[b] {
			fx.pairBase[a] = base
		}
		f := &effectFamily{name: proto.ItemSlot(a).String() + " pair", base: base}
		seen := map[int32]bool{}
		for _, slot := range []proto.ItemSlot{a, b} {
			if p.Locked[slot] {
				continue
			}
			for _, cand := range p.Slots[slot] {
				if !cand.NeedsSim || seen[cand.Item.ID] {
					continue
				}
				seen[cand.Item.ID] = true
				variant := base
				variant.Items[put] = quick(slot, cand, base)
				f.add(effectKey{effectItem, a, cand.Item.ID}, variant, prior(slot, cand), false)
			}
			if c := seed.Items[slot]; itemNeedsSim(c.ItemID) && !seen[c.ItemID] {
				seen[c.ItemID] = true
				variant := base
				variant.Items[put] = c
				f.add(effectKey{effectItem, a, c.ItemID}, variant, math.Inf(1), false)
			}
		}
		add(f)
	}

	// main hands, from the seed's weapon
	mh, oh := proto.ItemSlot_ItemSlotMainHand, proto.ItemSlot_ItemSlotOffHand
	seedMH, seedOH := seed.Items[mh], seed.Items[oh]
	if !p.Locked[mh] {
		if seedMH.ItemID != 0 {
			// main hands count from the seed's, which is worth nothing over itself
			fx.raw[effectKey{effectItem, mh, seedMH.ItemID}] = Estimate{}
		}
		f := &effectFamily{name: "main-hand weapons", base: seed, weapons: true}
		dominated := s.dominatedWeapons(mh, p.Slots[mh], lo, hi, gb)
		for _, cand := range p.Slots[mh] {
			id := cand.Item.ID
			if !cand.NeedsSim || id == seedMH.ItemID || dominated[id] {
				continue
			}
			variant := seed
			variant.Items[mh] = quick(mh, cand, seed)
			emptied := seedOH.ItemID != 0 && (p.fillsHands(&cand.Item) || id == seedOH.ItemID && itemLimit(p.catalog[id]) == 1)
			if emptied {
				if p.Locked[oh] {
					continue
				}
				variant.Items[oh] = ItemChoice{}
			}
			f.add(effectKey{effectItem, mh, id}, variant, prior(mh, cand), emptied)
		}
		add(f)
	}

	// off hands, from an empty one, next to a one-hander
	if !p.Locked[oh] {
		base := seed
		mhItem, ok := p.item(mh, seedMH.ItemID)
		if seedMH.ItemID == 0 || !ok || p.fillsHands(mhItem) {
			var best *Candidate
			bestLB := math.Inf(-1)
			if !p.Locked[mh] {
				for _, cand := range p.Slots[mh] {
					if lb := statLB(mh, cand); !p.fillsHands(&cand.Item) && lb > bestLB {
						best, bestLB = cand, lb
					}
				}
			}
			if best != nil {
				base.Items[mh] = quick(mh, best, base)
			} else {
				base.Items[mh] = ItemChoice{}
			}
		}
		if itemNeedsSim(seedOH.ItemID) {
			base.Items[oh] = ItemChoice{}
		}
		f := &effectFamily{name: "off-hand items", base: base, weapons: true}
		dominated := s.dominatedWeapons(oh, p.Slots[oh], lo, hi, gb)
		seen := map[int32]bool{}
		baseMH := base.Items[mh].ItemID
		for _, cand := range p.Slots[oh] {
			id := cand.Item.ID
			if !cand.NeedsSim || dominated[id] {
				continue
			}
			if baseMH == 0 && cand.Item.Type == proto.ItemType_ItemTypeWeapon && cand.Item.WeaponType != proto.WeaponType_WeaponTypeShield {
				fx.missing[effectKey{effectItem, oh, id}] = true
				continue
			}
			if id == baseMH && itemLimit(p.catalog[id]) == 1 {
				fx.missing[effectKey{effectItem, oh, id}] = true
				continue
			}
			seen[id] = true
			variant := base
			variant.Items[oh] = quick(oh, cand, base)
			f.add(effectKey{effectItem, oh, id}, variant, prior(oh, cand), false)
		}
		if itemNeedsSim(seedOH.ItemID) && !seen[seedOH.ItemID] && base.Items[mh] == seedMH {
			variant := base
			variant.Items[oh] = seedOH
			f.add(effectKey{effectItem, oh, seedOH.ItemID}, variant, math.Inf(1), false)
		}
		add(f)
	}

	// the other slots, from an item without an effect
	for _, slot := range singleSlots {
		if p.Locked[slot] {
			continue
		}
		base := seed
		if itemNeedsSim(seed.Items[slot].ItemID) {
			base.Items[slot] = ItemChoice{}
		}
		f := &effectFamily{name: slot.String() + " items", base: base, weapons: slot == proto.ItemSlot_ItemSlotRanged}
		var dominated map[int32]bool
		if f.weapons {
			dominated = s.dominatedWeapons(slot, p.Slots[slot], lo, hi, gb)
		}
		seen := map[int32]bool{}
		for _, cand := range p.Slots[slot] {
			if !cand.NeedsSim || dominated[cand.Item.ID] {
				continue
			}
			seen[cand.Item.ID] = true
			variant := base
			variant.Items[slot] = quick(slot, cand, base)
			f.add(effectKey{effectItem, slot, cand.Item.ID}, variant, prior(slot, cand), false)
		}
		if c := seed.Items[slot]; itemNeedsSim(c.ItemID) && !seen[c.ItemID] {
			variant := base
			variant.Items[slot] = c
			f.add(effectKey{effectItem, slot, c.ItemID}, variant, math.Inf(1), false)
		}
		add(f)
	}

	// enchants with an effect, on the seed's item or the best one when the seed has none
	for slot := range p.Slots {
		slot := proto.ItemSlot(slot)
		if p.Locked[slot] {
			continue
		}
		var effect []int32
		for _, cand := range p.Slots[slot] {
			for _, e := range cand.Enchants {
				if e.NeedsSim && !slices.Contains(effect, e.ID) {
					effect = append(effect, e.ID)
				}
			}
		}
		if len(effect) == 0 {
			continue
		}
		base := seed
		switch c := seed.Items[slot]; {
		case c.ItemID == 0:
			var best *Candidate
			bestLB := math.Inf(-1)
			for _, cand := range p.Slots[slot] {
				trial := seed
				trial.Items[slot] = ItemChoice{ItemID: cand.Item.ID}
				if lb := statLB(slot, cand); lb > bestLB && p.placementOK(trial) {
					best, bestLB = cand, lb
				}
			}
			if best == nil {
				for _, id := range effect {
					fx.missing[effectKey{effectEnchant, slot, id}] = true
				}
				continue
			}
			base.Items[slot] = quick(slot, best, base)
			base.Items[slot].Enchant = 0
		case EnchantHasEffect(c.Enchant):
			base.Items[slot].Enchant = 0
		}
		f := &effectFamily{name: slot.String() + " enchants", base: base}
		for _, id := range effect {
			variant := base
			variant.Items[slot].Enchant = id
			f.add(effectKey{effectEnchant, slot, id}, variant, 0, false)
		}
		add(f)
	}

	// gems with an effect: metas from an empty meta socket, the rest in place of a seed gem
	var metas, others []int32
	for _, g := range p.Gems {
		if !g.NeedsSim {
			continue
		}
		if g.Gem.Color == proto.GemColor_GemColorMeta {
			metas = append(metas, g.Gem.ID)
		} else {
			others = append(others, g.Gem.ID)
		}
	}
	head := proto.ItemSlot_ItemSlotHead
	if len(metas) > 0 && !p.Locked[head] {
		base := seed
		idx := -1
		if cand := p.candidate(head, seed.Items[head].ItemID); cand != nil {
			idx = metaSocket(cand)
		}
		if idx < 0 {
			var best *Candidate
			bestLB := math.Inf(-1)
			for _, cand := range p.Slots[head] {
				if lb := statLB(head, cand); metaSocket(cand) >= 0 && lb > bestLB {
					best, bestLB = cand, lb
				}
			}
			if best != nil {
				base.Items[head] = s.quickChoice(head, best, v, gp, -1)
				idx = metaSocket(best)
			}
		}
		if idx < 0 {
			for _, id := range metas {
				fx.missing[effectKey{effectGem, 0, id}] = true
			}
		} else {
			base.Items[head].Gems[idx] = 0
			f := &effectFamily{name: "meta gems", base: base}
			for _, id := range metas {
				variant := base
				variant.Items[head].Gems[idx] = id
				f.add(effectKey{effectGem, 0, id}, variant, 0, false)
			}
			add(f)
		}
	}
	if len(others) > 0 {
		slot, socket := -1, -1
	find:
		for sl, c := range seed.Items {
			cand := p.candidate(proto.ItemSlot(sl), c.ItemID)
			if cand == nil || p.Locked[sl] {
				continue
			}
			for i, color := range cand.Sockets {
				if color != proto.GemColor_GemColorMeta && i < MaxGems && !ItemHasEffect(c.Gems[i]) {
					slot, socket = sl, i
					break find
				}
			}
		}
		if slot < 0 {
			for _, id := range others {
				fx.missing[effectKey{effectGem, 0, id}] = true
			}
		} else {
			f := &effectFamily{name: "gems", base: seed}
			for _, id := range others {
				variant := seed
				variant.Items[slot].Gems[socket] = id
				f.add(effectKey{effectGem, 0, id}, variant, 0, false)
			}
			add(f)
		}
	}

	for _, f := range r.setFamilies(s, v, gp, statLB) {
		add(f)
	}
	return out
}

// setFamilies registers the pool's sets with the surrogate and lists sims of their bonuses: the
// set's 2 and 4 best pieces on top of the seed with its own pieces of that set swapped out. A set
// whose best pieces cost more than setScreen of J in stats isn't simmed.
func (r *run) setFamilies(s *surrogate, v stats.Stats, gp *gemPlan, statLB func(proto.ItemSlot, *Candidate) float64) []*effectFamily {
	p, seed := s.pool, s.seed
	type piece struct {
		cand *Candidate
		lb   float64
	}
	bySet := map[string]map[proto.ItemSlot]piece{}
	var names []string
	bestLB := map[proto.ItemSlot]float64{}
	bestPlain := map[proto.ItemSlot]piece{}
	for _, slot := range singleSlots {
		if p.Locked[slot] {
			continue
		}
		for _, cand := range p.Slots[slot] {
			lb := statLB(slot, cand)
			if cur, ok := bestLB[slot]; !ok || lb > cur {
				bestLB[slot] = lb
			}
			name := cand.Item.SetName
			if name == "" {
				if cur, ok := bestPlain[slot]; !ok || lb > cur.lb {
					bestPlain[slot] = piece{cand, lb}
				}
				continue
			}
			if bySet[name] == nil {
				bySet[name] = map[proto.ItemSlot]piece{}
				names = append(names, name)
			}
			if cur, ok := bySet[name][slot]; !ok || lb > cur.lb {
				bySet[name][slot] = piece{cand, lb}
			}
		}
	}

	threshold := setScreen * abs(r.seedJ.Mean)
	var out []*effectFamily
	for _, name := range names {
		pieces := bySet[name]
		if len(pieces) < 2 {
			continue
		}
		index := len(s.sets)
		s.sets = append(s.sets, &itemSet{name: name})
		for slot := range p.Slots {
			for _, cand := range p.Slots[slot] {
				if cand.Item.SetName == name {
					s.setOf[cand.Item.ID] = index
				}
			}
		}
		for _, c := range seed.Items {
			if item, ok := core.LookupItem(c.ItemID); ok && item.SetName == name {
				s.setOf[c.ItemID] = index
			}
		}

		slots := make([]proto.ItemSlot, 0, len(pieces))
		for slot := range pieces {
			slots = append(slots, slot)
		}
		loss := func(slot proto.ItemSlot) float64 { return bestLB[slot] - pieces[slot].lb }
		slices.SortFunc(slots, func(a, b proto.ItemSlot) int {
			switch la, lb := loss(a), loss(b); {
			case la < lb:
				return -1
			case la > lb:
				return 1
			}
			return int(a) - int(b)
		})
		lossOf := func(n int) float64 {
			total := 0.0
			for _, slot := range slots[:n] {
				total += loss(slot)
			}
			return total
		}

		base := seed
		for slot, c := range seed.Items {
			if item, ok := core.LookupItem(c.ItemID); ok && item.SetName == name && !p.Locked[slot] {
				base.Items[slot] = ItemChoice{}
				if plain, ok := bestPlain[proto.ItemSlot(slot)]; ok {
					base.Items[slot] = s.quickChoice(proto.ItemSlot(slot), plain.cand, v, gp, s.keepMeta(seed))
				}
			}
		}
		// swapping in whole sets moves a lot of stats at once, past where the curves stay accurate
		f := &effectFamily{name: "set " + name, base: base, matched: true}
		for _, n := range []int{2, 4} {
			if len(slots) < n || lossOf(n) > threshold {
				continue
			}
			variant := base
			for _, slot := range slots[:n] {
				variant.Items[slot] = s.quickChoice(slot, pieces[slot].cand, v, gp, s.keepMeta(base))
			}
			kind := effectSetTwo
			if n == 4 {
				kind = effectSetFour
			}
			f.add(effectKey{kind, 0, int32(index)}, variant, 0, false)
		}
		if len(f.keys) > 0 {
			out = append(out, f)
		}
	}
	return out
}

// placementOK is whether core would equip l as given: rings, trinkets and weapons where they are.
func (p *Pool) placementOK(l Loadout) bool {
	var items [NumSlots]*core.Item
	for slot, c := range l.Items {
		if c.ItemID == 0 {
			continue
		}
		item, ok := p.item(proto.ItemSlot(slot), c.ItemID)
		if !ok || p.fitsSlot(item, proto.ItemSlot(slot)) != nil {
			return false
		}
		items[slot] = item
	}
	return p.checkPlacement(items) == nil
}
