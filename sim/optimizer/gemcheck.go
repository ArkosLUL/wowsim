package optimizer

import (
	"fmt"
	"maps"
	"math"
	"slices"
	"strings"
	"time"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
	"github.com/wowsims/wotlk/sim/core/stats"
)

// gemTwins are copies of l, one for each distinct gem in its free non-meta sockets: Gem regems the
// slots holding that gem with it and the banned ones out of the running, so its sockets take the
// runners-up gemOptions drops for it, or whatever else value rates next, while socket bonuses and the
// meta still hold. A twin that breaks a rule, stat floors included, or sims the same as l or another
// twin (gemmingOf) is left out.
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
	seen := map[gemming]bool{p.gemmingOf(l): true}
	for _, g := range order {
		without := maps.Clone(banned)
		if without == nil {
			without = map[int32]bool{}
		}
		without[g] = true
		twin, _, err := p.gemWithout(l, holders[g], value, without)
		if err != nil {
			continue
		}
		key := p.gemmingOf(twin)
		if seen[key] || p.Check(twin) != nil {
			continue
		}
		seen[key] = true
		twins = append(twins, twin)
	}
	return twins
}

// gemming is what of a loadout's gems the sim can tell apart: per slot the item, enchant and reforge
// and whether the socket bonus is on, each weapon's own gems where the item swap is on, and the other
// gems counted together, each as simGem. Loadouts with the same gemming sim the same.
type gemming struct {
	items  [NumSlots]ItemChoice
	bonus  [NumSlots]bool
	pooled [NumSlots * MaxGems]int32
	race   proto.Race
}

func (p *Pool) gemmingOf(l Loadout) gemming {
	g := gemming{race: l.RacialTraits}
	n := 0
	for slot, c := range l.Items {
		g.bonus[slot] = p.bonusOn(proto.ItemSlot(slot), c)
		for i, id := range c.Gems {
			c.Gems[i] = p.simGem(id)
		}
		switch proto.ItemSlot(slot) {
		case proto.ItemSlot_ItemSlotMainHand, proto.ItemSlot_ItemSlotOffHand, proto.ItemSlot_ItemSlotRanged:
			if p.weaponSwap {
				slices.Sort(c.Gems[:])
				break
			}
			fallthrough
		default:
			for _, id := range c.Gems {
				if id != 0 {
					g.pooled[n] = id
					n++
				}
			}
			c.Gems = [MaxGems]int32{}
		}
		g.items[slot] = c
	}
	slices.Sort(g.pooled[:n])
	return g
}

// simGem is the pool's first gem the sim can't tell from id: same stats and color, and neither has an
// effect. Otherwise id itself.
func (p *Pool) simGem(id int32) int32 {
	if id == 0 || ItemHasEffect(id) {
		return id
	}
	gem := p.gem(id)
	for _, g := range p.Gems {
		if !g.NeedsSim && g.Gem.Color == gem.Color && g.Gem.Stats == gem.Stats {
			return g.Gem.ID
		}
	}
	return id
}

// bonusOn says whether c's socket bonus is on: a gem of a matching color in every socket of the item's
// own, as choiceStats has it.
func (p *Pool) bonusOn(slot proto.ItemSlot, c ItemChoice) bool {
	if c.ItemID == 0 {
		return false
	}
	item, ok := p.item(slot, c.ItemID)
	if !ok || len(item.GemSockets) == 0 {
		return false
	}
	for i, color := range item.GemSockets {
		if i >= MaxGems || c.Gems[i] == 0 || !core.ColorIntersects(color, gemColor(p, c.Gems[i])) {
			return false
		}
	}
	return true
}

// slopesAt is the price of each gear stat at l's slopes, the way the search's gem DP prices one there.
func (s *surrogate) slopesAt(l Loadout) stats.Stats {
	return s.slopes(s.loadoutStats(l), s.racialDelta(&l))
}

// linearAt prices a stat vector at l's slopes.
func (s *surrogate) linearAt(l Loadout) func(stats.Stats) float64 {
	return linearPrice(s.slopesAt(l))
}

func linearPrice(v stats.Stats) func(stats.Stats) float64 {
	return func(x stats.Stats) float64 { return dot(v, x) }
}

// statSocket is one gem's worth of a stat the pool's gems carry.
type statSocket struct {
	name  string
	stats stats.Stats
}

// statSockets is a socket of each stat the pool's non-meta gems carry, skip's left out, in stat order:
// the most of it one free gem holds, or any gem where no free one has it. Stats every gem carries at
// the same amount sim as one socket, since no gem can tell them apart (a hit gem's melee and spell
// rating, Void Sphere's resistances).
func (p *Pool) statSockets(skip map[int32]bool) []statSocket {
	var gems []*GemCandidate
	for _, g := range p.Gems {
		if g.Gem.Color != proto.GemColor_GemColorMeta && !skip[g.Gem.ID] {
			gems = append(gems, g)
		}
	}
	var groups [][]stats.Stat
	for st := stats.Stat(0); st < stats.Len; st++ {
		if !slices.ContainsFunc(gems, func(g *GemCandidate) bool { return g.Gem.Stats[st] > 0 }) {
			continue
		}
		i := slices.IndexFunc(groups, func(group []stats.Stat) bool {
			return !slices.ContainsFunc(gems, func(g *GemCandidate) bool { return g.Gem.Stats[st] != g.Gem.Stats[group[0]] })
		})
		if i < 0 {
			groups = append(groups, []stats.Stat{st})
		} else {
			groups[i] = append(groups[i], st)
		}
	}
	out := make([]statSocket, 0, len(groups))
	for _, group := range groups {
		lead := group[0]
		var best *GemCandidate
		for _, g := range gems {
			if g.Gem.Stats[lead] <= 0 {
				continue
			}
			free := p.freeGem(g)
			if best == nil || free && !p.freeGem(best) || free == p.freeGem(best) && g.Gem.Stats[lead] > best.Gem.Stats[lead] {
				best = g
			}
		}
		var socket statSocket
		names := make([]string, len(group))
		for i, st := range group {
			socket.stats[st] = best.Gem.Stats[st]
			names[i] = st.StatName()
		}
		socket.name = fmt.Sprintf("%g %s", best.Gem.Stats[lead], strings.Join(names, "+"))
		out = append(out, socket)
	}
	return out
}

// regemAt regems every free slot of l with the gems and meta worth most at the prices v, banned gems
// left out, so socket bonuses and the meta's colors re-solve with them. Where that drops a stat floor
// l meets, the floor's own stats get a premium on top of v (a Lagrange multiplier), bisected to the
// least that keeps the floor, so the regem gives up only what the floor can spare. Premiums can miss a
// gemming that keeps two floors competing for the same sockets (Bulwark P1's Defense and hit, a few
// points over each), so then only the slots whose gems and socket bonus move no floor regem. False
// when no regem keeps every rule.
func (r *run) regemAt(l Loadout, s *surrogate, v stats.Stats, banned map[int32]bool) (Loadout, bool) {
	p := r.pool
	var slots, spare []proto.ItemSlot
	for slot, c := range l.Items {
		if c.ItemID == 0 || p.Locked[slot] {
			continue
		}
		slots = append(slots, proto.ItemSlot(slot))
		if !s.feedsFloor(proto.ItemSlot(slot), c) {
			spare = append(spare, proto.ItemSlot(slot))
		}
	}
	if out, ok := r.regemSlots(l, s, v, banned, slots); ok {
		return out, true
	}
	if len(spare) == 0 || len(spare) == len(slots) {
		return l, false
	}
	return r.regemSlots(l, s, v, banned, spare)
}

// feedsFloor says whether c's gems or socket bonus move any of s's floors.
func (s *surrogate) feedsFloor(slot proto.ItemSlot, c ItemChoice) bool {
	var parts []stats.Stats
	if item, ok := s.pool.item(slot, c.ItemID); ok && s.pool.bonusOn(slot, c) {
		parts = append(parts, item.SocketBonus)
	}
	for _, id := range c.Gems {
		if gem := s.pool.gem(id); id != 0 && gem.Color != proto.GemColor_GemColorMeta {
			parts = append(parts, gem.Stats)
		}
	}
	for _, x := range parts {
		for i := range s.floors {
			if s.floors[i].moves(&x) > 0 {
				return true
			}
		}
	}
	return false
}

// regemSlots is regemAt over the given slots.
func (r *run) regemSlots(l Loadout, s *surrogate, v stats.Stats, banned map[int32]bool, slots []proto.ItemSlot) (Loadout, bool) {
	p := r.pool
	premium := make([]float64, len(s.floors))
	solve := func(l Loadout) (Loadout, float64, bool) {
		w := v
		for i, m := range premium {
			if m != 0 {
				w = w.Add(s.floors[i].gradient.Multiply(m))
			}
		}
		out, value, err := p.gemWithout(l, slots, linearPrice(w), banned)
		return out, value, err == nil
	}

	// the meta's worth its stats at v plus what the sims priced its effect at
	out, ok, best := l, false, math.Inf(-1)
	metas := []int32{0}
	slot, socket, hasMeta := p.freeMetaSocket(l)
	if hasMeta {
		metas = p.metaOptions(l.Items[slot].Gems[socket], banned)
	}
	for _, meta := range metas {
		with := l
		if hasMeta {
			with.Items[slot].Gems[socket] = meta
		}
		regemmed, value, fits := solve(with)
		if meta != 0 {
			value += dot(v, p.gem(meta).Stats) + s.gems[meta].Mean
		}
		if fits && value > best {
			out, best, ok = regemmed, value, true
		}
	}
	if !ok {
		return l, false
	}

	if len(s.floors) > 0 {
		sheet, err := p.finalStats(l)
		if err != nil {
			return l, false
		}
		base := s.loadoutStats(l)
		// a floor's sheet reading in x: l's, moved by the gear stats x changes along the floor's gradient
		misses := func(i int, x Loadout) bool {
			f := &s.floors[i]
			moved := s.loadoutStats(x).Subtract(base)
			return sheet[f.stat]+f.moves(&moved) < f.min
		}
		missed := func(x Loadout) int {
			for i := range s.floors {
				if misses(i, x) {
					return i
				}
			}
			return -1
		}
		top := s.floorWeight
		for _, price := range v {
			top = max(top, 100*(math.Abs(price)+1))
		}
		for round := 0; round <= premiumRounds*len(s.floors); round++ {
			i := missed(out)
			if i < 0 {
				break
			}
			lo, hi := premium[i], top
			premium[i] = hi
			kept, _, fits := solve(out)
			if !fits || misses(i, kept) {
				return l, false
			}
			for step := 0; step < premiumSteps; step++ {
				premium[i] = (lo + hi) / 2
				if trial, _, fits := solve(out); fits && !misses(i, trial) {
					hi, kept = premium[i], trial
				} else {
					lo = premium[i]
				}
			}
			premium[i], out = hi, kept
		}
	}
	return out, p.Check(out) == nil
}

// premiumSteps halve a floor premium's bracket: 20 take it to a millionth of its top.
const premiumSteps = 20

// premiumRounds is how many premium raises per floor regemSlots tries, plus one, before it gives up.
// A var so tests can make the rounds run out.
var premiumRounds = 2

// freeMetaSocket is where l holds a meta socket the search may regem.
func (p *Pool) freeMetaSocket(l Loadout) (proto.ItemSlot, int, bool) {
	for slot, c := range l.Items {
		if c.ItemID == 0 || p.Locked[slot] {
			continue
		}
		if cand := p.candidate(proto.ItemSlot(slot), c.ItemID); cand != nil {
			if i := metaSocket(cand); i >= 0 {
				return proto.ItemSlot(slot), i, true
			}
		}
	}
	return 0, 0, false
}

// metaOptions is current, then every meta in the pool but banned ones.
func (p *Pool) metaOptions(current int32, banned map[int32]bool) []int32 {
	out := []int32{current}
	for _, g := range p.Gems {
		if g.Gem.Color == proto.GemColor_GemColorMeta && g.Gem.ID != current && !banned[g.Gem.ID] {
			out = append(out, g.Gem.ID)
		}
	}
	return out
}

// gemTwin is a copy of the pick with other gems, and what put it forward ("" for gemTwins').
type gemTwin struct {
	loadout Loadout
	why     string
}

// priceTwins sims a socket of each stat the pool's gems carry (statSockets) on the pick, paired with
// it, and regems the pick (regemAt) at the prices those set: once per stat the slopes price under its
// socket's sim, raised to it, then once with every gem stat at its socket's sim. Each regem that
// differs from the pick is a twin. The slopes come from curves measured around the seed, so a stat
// worth more at the pick (a tank's Stamina) stays underpriced; where the seed sits far from the pick
// they can overprice every stat at once, and only the sim's own prices find it.
func (r *run) priceTwins(pick *verifiedLoadout, s *surrogate, v stats.Stats) ([]gemTwin, error) {
	l := pick.loadout
	if r.pool.Check(l) != nil || !r.pool.hasFreeSockets(l) {
		return nil, nil
	}
	sockets := r.pool.statSockets(s.unpricedGems)
	if len(sockets) == 0 {
		return nil, nil
	}
	points := make([]Point, len(sockets))
	for i, socket := range sockets {
		points[i] = Point{Loadout: l, Offset: socket.stats}
	}
	evals, err := r.evaluate(points, r.budget.Iterations)
	if err != nil {
		return nil, err
	}
	var twins []gemTwin
	seen := map[gemming]bool{r.pool.gemmingOf(l): true}
	add := func(price stats.Stats, why func(Loadout) string) {
		twin, ok := r.regemAt(l, s, price, s.unpricedGems)
		if key := r.pool.gemmingOf(twin); ok && !seen[key] {
			seen[key] = true
			twins = append(twins, gemTwin{twin, why(twin)})
		}
	}
	// the search's gems needn't be what the DP picks at the pick's slopes, so a raise can land on just
	// that regem: name the slopes for it then, not the raise
	var atSlopes *gemming
	raised := func(line string) func(Loadout) string {
		return func(twin Loadout) string {
			if atSlopes == nil {
				regemmed, _ := r.regemAt(l, s, v, s.unpricedGems)
				key := r.pool.gemmingOf(regemmed)
				atSlopes = &key
			}
			if r.pool.gemmingOf(twin) == *atSlopes {
				return "every free socket regemmed at the pick's slopes"
			}
			return line
		}
	}
	type socketSim struct {
		socket statSocket
		line   string
	}
	var sims []socketSim
	// every gem stat at what its socket sims; the stats no gem carries at 0
	var simmed stats.Stats
	measured := false
	for i, socket := range sockets {
		if evals[i] == nil {
			continue
		}
		d := r.obj.Delta(pick.eval, evals[i])
		priced := dot(v, socket.stats)
		line := fmt.Sprintf("a socket of %s sims %+.1f ± %.1f at the pick, where the slopes price it %+.1f", socket.name, d.Mean, d.SE, priced)
		r.trace("gem check: %s", line)
		sims = append(sims, socketSim{socket, line})
		norm := dot(socket.stats, socket.stats)
		simmed = simmed.Add(socket.stats.Multiply(d.Mean / norm))
		measured = measured || abs(d.Mean) > 2*d.SE
		if d.Mean > priced {
			// the least change to v that prices the socket at its sim
			add(v.Add(socket.stats.Multiply((d.Mean-priced)/norm)), raised(line))
		}
	}
	if !measured {
		// no socket the sim can tell from nothing: those prices are noise
		return twins, nil
	}
	add(simmed, func(twin Loadout) string {
		gained := s.loadoutStats(twin).Subtract(s.loadoutStats(l))
		var lines []string
		for _, sim := range sims {
			if dot(gained, sim.socket.stats) > 0 {
				lines = append(lines, sim.line)
			}
		}
		return "every gem stat priced at what a socket of it sims at the pick; " + strings.Join(lines, "; ")
	})
	return twins, nil
}

// hasFreeSockets says whether l holds a non-meta socket the search may regem.
func (p *Pool) hasFreeSockets(l Loadout) bool {
	for slot, c := range l.Items {
		if c.ItemID == 0 || p.Locked[slot] {
			continue
		}
		if cand := p.candidate(proto.ItemSlot(slot), c.ItemID); cand != nil && slices.ContainsFunc(cand.Sockets, func(color proto.GemColor) bool {
			return color != proto.GemColor_GemColorMeta
		}) {
			return true
		}
	}
	return false
}

// checkGems sims verified[0]'s gem twins paired with it at the run's iterations: gemTwins, each of its
// gems out for the runners-up, and priceTwins, its gems re-solved at prices the sim sets at the pick.
// The search never sims gems one by one, so this gives the sim the last word. The best mean takes the
// pick's place only past accepts, since the best of a dozen twins tied with the pick beats it on noise
// in most runs, and must beat the seed while the pick does. The twins join verified, so on a tank run
// the ones within 2 se of the pick go to time to live too (breakTie).
func (r *run) checkGems(verified []*verifiedLoadout) ([]*verifiedLoadout, error) {
	if len(verified) == 0 || r.pool == nil {
		return verified, nil
	}
	pick := verified[0]
	s := r.surrogates[pick.loadout.RacialTraits]
	if s == nil {
		return verified, nil
	}
	if traceHook != nil {
		simmed, start := r.eval.simmed(), time.Now()
		defer func() {
			r.trace("gem check: %d iterations simmed, %.1f s", r.eval.simmed()-simmed, time.Since(start).Seconds())
		}()
	}
	v := s.slopesAt(pick.loadout)
	var twins []gemTwin
	index := map[gemming]int{}
	for _, l := range r.pool.gemTwins(pick.loadout, linearPrice(v), s.unpricedGems) {
		index[r.pool.gemmingOf(l)] = len(twins)
		twins = append(twins, gemTwin{loadout: l})
	}
	priced, err := r.priceTwins(pick, s, v)
	if err != nil {
		return verified, err
	}
	for _, t := range priced {
		key := r.pool.gemmingOf(t.loadout)
		if i, ok := index[key]; ok {
			twins[i].why = t.why
			continue
		}
		index[key] = len(twins)
		twins = append(twins, t)
	}
	already := map[gemming]bool{r.pool.gemmingOf(r.r.Seed): true}
	for _, done := range verified {
		already[r.pool.gemmingOf(done.loadout)] = true
	}
	twins = slices.DeleteFunc(twins, func(t gemTwin) bool { return already[r.pool.gemmingOf(t.loadout)] })
	if len(twins) == 0 {
		return verified, nil
	}
	points := make([]Point, len(twins))
	for i, t := range twins {
		points[i] = Point{Loadout: t.loadout}
	}
	evals, err := r.evaluate(points, r.budget.Iterations)
	if err != nil {
		return verified, err
	}
	mustBeatSeed := r.beatsSeed(pick.eval)
	winner, why, best := -1, "", Estimate{}
	for i, t := range twins {
		eval := evals[i]
		if eval == nil {
			continue
		}
		verified = append(verified, &verifiedLoadout{t.loadout, eval})
		d := r.obj.Delta(pick.eval, eval)
		note := ""
		if t.why != "" {
			note = "; " + t.why
		}
		r.trace("gem twin %s: %+.1f ± %.1f over the pick%s", r.pool.gemSwap(pick.loadout, t.loadout), d.Mean, d.SE, note)
		if r.accepts(d) && d.Mean > best.Mean && (!mustBeatSeed || r.beatsSeed(eval)) {
			winner, why, best = len(verified)-1, t.why, d
		}
	}
	if winner < 0 {
		return verified, nil
	}
	w := verified[winner]
	copy(verified[1:winner+1], verified[:winner])
	verified[0] = w
	msg := fmt.Sprintf("%s scores %+.1f ± %.1f over the gems the search picked, the best of %d gem swaps simmed at %d iterations, so the pick takes it",
		r.pool.gemSwap(pick.loadout, w.loadout), best.Mean, best.SE, len(twins), r.budget.Iterations)
	if why != "" {
		msg += " (" + why + ")"
	}
	r.warn("%s", msg)
	return verified, nil
}

// recheckGems runs the gem check on a best the neighborhood just adopted over before, when the move
// changed any gems: an item it brings in comes gemmed at the slopes, like the search's. A twin that
// wins becomes the best; every twin joins verified.
func (r *run) recheckGems(before Loadout, verified []*verifiedLoadout) ([]*verifiedLoadout, error) {
	moved := false
	for slot := range before.Items {
		moved = moved || before.Items[slot].Gems != r.best.Items[slot].Gems
	}
	if !moved {
		return verified, nil
	}
	checked, err := r.checkGems([]*verifiedLoadout{{r.best, r.bestEval}})
	if err != nil {
		return verified, err
	}
	for _, v := range checked {
		if v.loadout != r.best {
			verified = append(verified, v)
		}
	}
	r.best, r.bestEval = checked[0].loadout, checked[0].eval
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
