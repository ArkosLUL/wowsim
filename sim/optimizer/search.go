package optimizer

import (
	"errors"
	"fmt"
	"math"
	"math/rand"
	"runtime/debug"
	"slices"
	"sync"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
	"github.com/wowsims/wotlk/sim/core/stats"
)

// searcher finds the loadouts the surrogate rates best, among the options pruning leaves: a few
// annealing runs from each start (the seed, a greedy fill, the warm starts, each priced set), each
// finished by an exact coordinate polish, then the best loadout with each slot's runners-up forced in.
type searcher struct {
	s    *surrogate
	r    *run
	pool *Pool

	// What each slot can hold, after pruning.
	opts [NumSlots][]*option
	// Every priced candidate, before pruning.
	all [NumSlots][]*option

	lo, hi stats.Stats
	gb     *gemBounds
	plans  map[uint64]*gemPlan
	// Scratch set counts for trial scores.
	counts []int8
	rng    *rand.Rand
	// The most any ring or trinket pair adds beyond its items, which candBounds leaves out.
	pairSlack float64
}

// fork is a copy that shares the read-only parts, for another goroutine.
func (se *searcher) fork(seed int64) *searcher {
	out := *se
	out.plans = map[uint64]*gemPlan{}
	out.counts = make([]int8, len(se.s.sets))
	out.rng = rand.New(rand.NewSource(seed))
	return &out
}

// parallel runs job(i) for i < n, each on its own fork, on up to the run's workers. Seeds are drawn
// up front, so the results don't depend on scheduling.
func (se *searcher) parallel(n int, job func(fork *searcher, i int)) {
	seeds := make([]int64, n)
	for i := range seeds {
		seeds[i] = se.rng.Int63()
	}
	runParallel(n, int(se.r.r.Settings.GetWorkers()), func(i int) {
		job(se.fork(seeds[i]), i)
	})
}

// runParallel runs job(i) for i < n on up to workers goroutines. A panic in a job comes back up in
// the caller once they're all done, so Optimize's recover still catches it.
func runParallel(n, workers int, job func(i int)) {
	var (
		wg       sync.WaitGroup
		mu       sync.Mutex
		panicked any
		stack    []byte
	)
	sem := make(chan struct{}, max(1, workers))
	for i := 0; i < n; i++ {
		wg.Add(1)
		sem <- struct{}{}
		go func(i int) {
			defer func() {
				if p := recover(); p != nil {
					mu.Lock()
					if panicked == nil {
						panicked, stack = p, debug.Stack()
					}
					mu.Unlock()
				}
				<-sem
				wg.Done()
			}()
			job(i)
		}(i)
	}
	wg.Wait()
	if panicked != nil {
		panic(fmt.Sprintf("%v\nStack Trace:\n%s", panicked, stack))
	}
}

// option is a candidate with bounds on what it adds to J (see candBounds).
type option struct {
	cand   *Candidate
	lb, ub float64
}

// Pruning keeps every option that could be among a slot's best k, whatever the rest of the gear:
// more than one, since the equip rules (unique items, limit groups) can rule out the best.
const (
	keepSingle = 2
	keepPaired = 3
)

func newSearcher(s *surrogate, r *run) *searcher {
	se := &searcher{s: s, r: r, pool: s.pool, plans: map[uint64]*gemPlan{}, counts: make([]int8, len(s.sets)), rng: r.rng}
	se.lo, se.hi = s.slopeBox()
	se.gb = s.newGemBounds(se.lo, se.hi)
	for _, corr := range s.pairs {
		se.pairSlack = max(se.pairSlack, corr)
	}
	for slot := range se.pool.Slots {
		for _, cand := range se.pool.Slots[slot] {
			lb, ub, ok := s.candBounds(proto.ItemSlot(slot), cand, se.lo, se.hi, se.gb)
			if ok || se.pool.Locked[slot] {
				se.all[slot] = append(se.all[slot], &option{cand, lb, ub})
			}
		}
	}
	for _, group := range se.groups() {
		for _, o := range group.opts {
			if o.ub >= group.threshold || se.pool.Locked[group.slot] {
				se.opts[group.slot] = append(se.opts[group.slot], o)
			}
		}
	}
	return se
}

// pruneGroup is a set of options competing for one slot, and the k-th best lower bound among them.
type pruneGroup struct {
	slot      proto.ItemSlot
	opts      []*option
	threshold float64
}

// groups splits each slot's options into the groups pruning compares: main hands that fill both hands
// apart from the rest, since a one-hander comes with an off hand.
func (se *searcher) groups() []pruneGroup {
	var out []pruneGroup
	for slot := range se.all {
		slot := proto.ItemSlot(slot)
		k := keepSingle
		switch slot {
		case proto.ItemSlot_ItemSlotFinger1, proto.ItemSlot_ItemSlotFinger2, proto.ItemSlot_ItemSlotTrinket1,
			proto.ItemSlot_ItemSlotTrinket2, proto.ItemSlot_ItemSlotMainHand, proto.ItemSlot_ItemSlotOffHand:
			k = keepPaired
		}
		var split [2][]*option
		for _, o := range se.all[slot] {
			both := slot == proto.ItemSlot_ItemSlotMainHand && se.pool.fillsHands(&o.cand.Item)
			split[boolIndex(both)] = append(split[boolIndex(both)], o)
		}
		for _, opts := range split {
			if len(opts) == 0 {
				continue
			}
			lbs := make([]float64, len(opts))
			for i, o := range opts {
				lbs[i] = o.lb
			}
			slices.Sort(lbs)
			threshold := math.Inf(-1)
			if len(lbs) >= k {
				threshold = lbs[len(lbs)-k]
			}
			out = append(out, pruneGroup{slot, opts, threshold})
		}
	}
	return out
}

func boolIndex(b bool) int {
	if b {
		return 1
	}
	return 0
}

// contenders are the measured items that survive pruning, keyed as effects, with the middle of their
// bounds: the residuals worth measuring again at full iterations.
func (se *searcher) contenders() map[effectKey]float64 {
	out := map[effectKey]float64{}
	for _, group := range se.groups() {
		for _, o := range group.opts {
			fam := familySlot(group.slot)
			if _, measured := se.s.items[fam][o.cand.Item.ID]; !measured || o.ub < group.threshold {
				continue
			}
			k := effectKey{effectItem, fam, o.cand.Item.ID}
			out[k] = max(out[k], (o.lb+o.ub)/2)
		}
	}
	return out
}

// dominatedWeapons are weapons without an effect that at least keepPaired others beat outright: the
// same type and hand, about the same speed, no less damage per swing or per second, and a stat lower
// bound over this one's upper bound. They're never worth a sim.
func (s *surrogate) dominatedWeapons(slot proto.ItemSlot, cands []*Candidate, lo, hi stats.Stats, gb *gemBounds) map[int32]bool {
	type weapon struct {
		cand          *Candidate
		lb, ub        float64
		avg, dps, spd float64
		effect        bool
	}
	var ws []weapon
	for _, cand := range cands {
		item := &cand.Item
		if item.WeaponDamageMax <= 0 {
			continue
		}
		lb, ub := s.candStatBounds(slot, cand, lo, hi, gb)
		avg := (item.WeaponDamageMin + item.WeaponDamageMax) / 2
		ws = append(ws, weapon{cand, lb, ub, avg, weaponDPS(item), item.SwingSpeed, ItemHasEffect(item.ID)})
	}
	out := map[int32]bool{}
	for i, a := range ws {
		if a.effect {
			continue
		}
		beaten := 0
		for j, b := range ws {
			if i == j || b.cand.Item.ID == a.cand.Item.ID {
				continue
			}
			ia, ib := &a.cand.Item, &b.cand.Item
			if ia.WeaponType != ib.WeaponType || ia.HandType != ib.HandType || ia.RangedWeaponType != ib.RangedWeaponType ||
				math.Abs(a.spd-b.spd) > 0.2 || b.avg < a.avg || b.dps < a.dps || b.lb < a.ub {
				continue
			}
			if beaten++; beaten >= keepPaired {
				out[ia.ID] = true
				break
			}
		}
	}
	return out
}

// state is a loadout with its per-slot stats and residuals, so a one-slot change rescores in O(1).
type state struct {
	l      Loadout
	stats  [NumSlots]stats.Stats
	res    [NumSlots]float64
	total  stats.Stats
	resSum float64
	// The ring and trinket pairs' correction (see surrogate.pairs).
	pairs  float64
	counts []int8
	value  float64
}

func (se *searcher) newState(l Loadout) *state {
	st := &state{l: l, counts: make([]int8, len(se.s.sets))}
	for slot, c := range l.Items {
		st.stats[slot] = se.s.choiceStats(proto.ItemSlot(slot), c)
		st.res[slot] = se.s.choiceResidual(proto.ItemSlot(slot), c)
		st.total = st.total.Add(st.stats[slot])
		st.resSum += st.res[slot]
		if i, ok := se.s.setOf[c.ItemID]; ok && c.ItemID != 0 {
			st.counts[i]++
		}
	}
	st.pairs = se.s.pairCorrection(&st.l)
	st.value = se.s.score(st.total, st.resSum+st.pairs, st.counts)
	return st
}

func (st *state) clone() *state {
	out := *st
	out.counts = slices.Clone(st.counts)
	return &out
}

// set puts c in slot and rescores.
func (se *searcher) set(st *state, slot proto.ItemSlot, c ItemChoice) {
	old := st.l.Items[slot]
	if i, ok := se.s.setOf[old.ItemID]; ok && old.ItemID != 0 {
		st.counts[i]--
	}
	if i, ok := se.s.setOf[c.ItemID]; ok && c.ItemID != 0 {
		st.counts[i]++
	}
	cs := se.s.choiceStats(slot, c)
	cr := se.s.choiceResidual(slot, c)
	st.total = st.total.Subtract(st.stats[slot]).Add(cs)
	st.resSum += cr - st.res[slot]
	st.stats[slot], st.res[slot] = cs, cr
	st.l.Items[slot] = c
	st.pairs = se.s.pairCorrection(&st.l)
	st.value = se.s.score(st.total, st.resSum+st.pairs, st.counts)
}

// change is one slot's new choice.
type change struct {
	slot   proto.ItemSlot
	choice ItemChoice
}

// trial is what st would score with the changes, without making them.
func (se *searcher) trial(st *state, changes ...change) float64 {
	total, res, pairs := st.total, st.resSum, st.pairs
	copy(se.counts, st.counts)
	pairChanged := false
	for _, ch := range changes {
		old := st.l.Items[ch.slot]
		if i, ok := se.s.setOf[old.ItemID]; ok && old.ItemID != 0 {
			se.counts[i]--
		}
		if i, ok := se.s.setOf[ch.choice.ItemID]; ok && ch.choice.ItemID != 0 {
			se.counts[i]++
		}
		total = total.Subtract(st.stats[ch.slot]).Add(se.s.choiceStats(ch.slot, ch.choice))
		res += se.s.choiceResidual(ch.slot, ch.choice) - st.res[ch.slot]
		pairChanged = pairChanged || familySlot(ch.slot) != ch.slot || slices.Contains(pairSlots, ch.slot)
	}
	if pairChanged && len(se.s.pairs) > 0 {
		l := st.l
		for _, ch := range changes {
			l.Items[ch.slot] = ch.choice
		}
		pairs = se.s.pairCorrection(&l)
	}
	return se.s.score(total, res+pairs, se.counts)
}

// apply makes the changes if the result breaks no rule, and says whether it did. A broken gem rule
// (a meta that won't activate, too many unique or jewelcrafter gems) gets one try at fixing: the
// exact gem DP over the changed slots.
func (se *searcher) apply(st *state, changes ...change) bool {
	old := make([]change, len(changes))
	for i, ch := range changes {
		old[i] = change{ch.slot, st.l.Items[ch.slot]}
		se.set(st, ch.slot, ch.choice)
	}
	err := se.check(st.l)
	if err == nil {
		return true
	}
	if se.gemRuleBroken(err) {
		slots := make([]proto.ItemSlot, len(changes))
		for i, ch := range changes {
			slots[i] = ch.slot
		}
		if regemmed, _, gemErr := se.pool.Gem(st.l, slots, se.linear(st.total)); gemErr == nil && se.check(regemmed) == nil {
			for _, slot := range slots {
				se.set(st, slot, regemmed.Items[slot])
			}
			return true
		}
	}
	for i := len(old) - 1; i >= 0; i-- {
		se.set(st, old[i].slot, old[i].choice)
	}
	return false
}

// check is CheckGear plus the surrogate's own rule: nothing it couldn't price.
func (se *searcher) check(l Loadout) error {
	if err := se.pool.CheckGear(l); err != nil {
		return err
	}
	for slot, c := range l.Items {
		if !se.pool.Locked[slot] && !se.s.priced(proto.ItemSlot(slot), c) {
			return errUnpriced
		}
	}
	return nil
}

var errUnpriced = errors.New("the loadout holds something the sims didn't price")

func (se *searcher) gemRuleBroken(err error) bool {
	var re *RuleError
	if !errors.As(err, &re) {
		return false
	}
	switch re.Rule {
	case RuleMeta:
		return true
	case RuleUnique, RuleLimitGroup, RuleProfession, RuleExcluded, RulePool, RuleSocket:
		return se.pool.gems[re.ID] != nil || isGem(re.ID)
	}
	return false
}

func isGem(id int32) bool {
	_, ok := core.LookupGem(id)
	return ok
}

// linear prices a stat vector at total's slopes, for the gem DP.
func (se *searcher) linear(total stats.Stats) func(stats.Stats) float64 {
	v := se.s.slopes(total)
	return func(x stats.Stats) float64 { return dot(v, x) }
}

func (se *searcher) plan(total stats.Stats) (stats.Stats, *gemPlan) {
	v := se.s.slopes(total)
	key := se.s.regime(total)
	gp := se.plans[key]
	if gp == nil {
		gp = se.s.newGemPlan(v)
		se.plans[key] = gp
	}
	return v, gp
}

// configure picks cand's enchant and reforge by trial score, one after the other, with gems from the
// plan for the slopes cand would sit at.
func (se *searcher) configure(st *state, slot proto.ItemSlot, cand *Candidate, extra ...change) ItemChoice {
	around := st.total.Subtract(st.stats[slot]).Add(cand.Item.Stats)
	v, gp := se.plan(around)
	meta := int32(0)
	if metaSocket(cand) >= 0 {
		meta = se.s.headMeta(st.l)
	}
	c := se.s.quickChoice(slot, cand, v, gp, meta)
	if len(se.s.floors) == 0 {
		return se.configureFast(st, slot, cand, c, extra)
	}
	tryAll := func(options []ItemChoice) {
		best := se.trial(st, append([]change{{slot, c}}, extra...)...)
		for _, o := range options {
			if val := se.trial(st, append([]change{{slot, o}}, extra...)...); val > best {
				best, c = val, o
			}
		}
	}
	enchants := []ItemChoice{withEnchant(c, 0)}
	for _, e := range cand.Enchants {
		if !se.s.unpricedEnchants[slot][e.ID] {
			enchants = append(enchants, withEnchant(c, e.ID))
		}
	}
	tryAll(enchants)
	reforges := []ItemChoice{withReforge(c, 0, 0)}
	for _, r := range cand.Reforges {
		reforges = append(reforges, withReforge(c, r.From, r.To))
	}
	tryAll(reforges)
	return c
}

// configureFast is configure's trials without a whole rescore each. An enchant or reforge only moves
// a few stats and nothing else the score counts but the enchant's residual, so each costs just those
// stats' curves. Floors tie every stat together, so with any configure rescores in full.
func (se *searcher) configureFast(st *state, slot proto.ItemSlot, cand *Candidate, c ItemChoice, extra []change) ItemChoice {
	total := st.total
	for _, ch := range append([]change{{slot, c}}, extra...) {
		total = total.Subtract(st.stats[ch.slot]).Add(se.s.choiceStats(ch.slot, ch.choice))
	}
	var none stats.Stats
	// best keeps the option that gains the most over the current one, the first on a tie
	var best float64
	var pick int
	try := func(i int, gain float64) {
		if gain > best {
			best, pick = gain, i
		}
	}

	cur, curRes := &none, 0.0
	if e := cand.enchant(c.Enchant); e != nil {
		cur, curRes = &e.Stats, se.s.enchants[slot][e.ID].Mean
	}
	best, pick = 0, -2
	if c.Enchant != 0 {
		try(-1, se.s.statsGain(&total, cur, &none)-curRes)
	}
	for i := range cand.Enchants {
		e := &cand.Enchants[i]
		if e.ID != c.Enchant && !se.s.unpricedEnchants[slot][e.ID] {
			try(i, se.s.statsGain(&total, cur, &e.Stats)+se.s.enchants[slot][e.ID].Mean-curRes)
		}
	}
	switch {
	case pick == -1:
		total, c.Enchant = total.Subtract(*cur), 0
	case pick >= 0:
		total, c.Enchant = total.Subtract(*cur).Add(cand.Enchants[pick].Stats), cand.Enchants[pick].ID
	}

	cur = &none
	if r := cand.reforge(c.ReforgeFrom, c.ReforgeTo); r != nil {
		cur = &r.Stats
	}
	best, pick = 0, -2
	if c.ReforgeFrom != 0 || c.ReforgeTo != 0 {
		try(-1, se.s.statsGain(&total, cur, &none))
	}
	for i := range cand.Reforges {
		if r := &cand.Reforges[i]; r.From != c.ReforgeFrom || r.To != c.ReforgeTo {
			try(i, se.s.statsGain(&total, cur, &r.Stats))
		}
	}
	switch {
	case pick == -1:
		c.ReforgeFrom, c.ReforgeTo = 0, 0
	case pick >= 0:
		c.ReforgeFrom, c.ReforgeTo = cand.Reforges[pick].From, cand.Reforges[pick].To
	}
	return c
}

func (c *Candidate) enchant(id int32) *EnchantOption {
	for i := range c.Enchants {
		if c.Enchants[i].ID == id {
			return &c.Enchants[i]
		}
	}
	return nil
}

func (c *Candidate) reforge(from, to int32) *ReforgeOption {
	for i := range c.Reforges {
		if c.Reforges[i].From == from && c.Reforges[i].To == to {
			return &c.Reforges[i]
		}
	}
	return nil
}

func withEnchant(c ItemChoice, id int32) ItemChoice {
	c.Enchant = id
	return c
}

func withReforge(c ItemChoice, from, to int32) ItemChoice {
	c.ReforgeFrom, c.ReforgeTo = from, to
	return c
}

// improvement is how much better a move must score to count, against float noise.
const improvement = 1e-9

// bestItem moves slot to its best option, configured, if that beats the current choice and breaks no
// rule. A main hand that fills both hands empties the off hand; a one-hander into an empty off hand
// brings its best off hand along.
func (se *searcher) bestItem(st *state, slot proto.ItemSlot) bool {
	mh, oh := proto.ItemSlot_ItemSlotMainHand, proto.ItemSlot_ItemSlotOffHand
	if slot == oh {
		if item, ok := se.pool.item(mh, st.l.Items[mh].ItemID); ok && st.l.Items[mh].ItemID != 0 && se.pool.fillsHands(item) {
			return false
		}
	}
	type move struct {
		value   float64
		changes []change
	}
	var moves []move
	// skip options whose upper bound, on top of the slot emptied, can't beat what's there. Not for
	// weapons: their moves change the other hand too.
	prune := slot != mh && slot != oh
	emptied := 0.0
	if prune {
		emptied = se.trial(st, change{slot, ItemChoice{}}) + se.pairSlack
	}
	for _, o := range se.opts[slot] {
		if prune && emptied+o.ub <= st.value+improvement {
			continue
		}
		var extra []change
		if slot == mh && se.pool.fillsHands(&o.cand.Item) && st.l.Items[oh].ItemID != 0 {
			extra = []change{{oh, ItemChoice{}}}
		}
		c := se.configure(st, slot, o.cand, extra...)
		changes := append([]change{{slot, c}}, extra...)
		if slot == mh && len(extra) == 0 && st.l.Items[oh].ItemID == 0 && len(se.opts[oh]) > 0 && !se.pool.fillsHands(&o.cand.Item) {
			if ohChange, ok := se.bestPartner(st, c); ok {
				changes = append(changes, ohChange)
			}
		}
		moves = append(moves, move{se.trial(st, changes...), changes})
	}
	slices.SortStableFunc(moves, func(a, b move) int {
		switch {
		case a.value > b.value:
			return -1
		case a.value < b.value:
			return 1
		}
		return 0
	})
	for _, m := range moves {
		if m.value <= st.value+improvement {
			return false
		}
		if se.apply(st, m.changes...) {
			return true
		}
	}
	return false
}

// bestPartner is the best off hand for a new main-hand choice, with the off hand empty now.
func (se *searcher) bestPartner(st *state, mhChoice ItemChoice) (change, bool) {
	mh, oh := proto.ItemSlot_ItemSlotMainHand, proto.ItemSlot_ItemSlotOffHand
	best, found := math.Inf(-1), false
	var out change
	for _, o := range se.opts[oh] {
		c := se.configure(st, oh, o.cand, change{mh, mhChoice})
		if val := se.trial(st, change{mh, mhChoice}, change{oh, c}); val > best {
			best, out, found = val, change{oh, c}, true
		}
	}
	return out, found
}

// gemSweep tries every gem the DP would consider in every socket, one socket at a time, and keeps the
// best that breaks no rule. It walks up to breakpoints the linear DP overshoots.
func (se *searcher) gemSweep(st *state, locked *[NumSlots]bool) bool {
	improved := false
	shortlist := se.pool.gemOptions(se.linear(st.total), MaxGems*NumSlots)
	var metas []int32
	for _, g := range se.pool.Gems {
		if g.Gem.Color == proto.GemColor_GemColorMeta && !se.s.unpricedGems[g.Gem.ID] {
			metas = append(metas, g.Gem.ID)
		}
	}
	for slot, c := range st.l.Items {
		if c.ItemID == 0 || se.pool.Locked[slot] || locked[slot] {
			continue
		}
		cand := se.pool.candidate(proto.ItemSlot(slot), c.ItemID)
		if cand == nil {
			continue
		}
		for i, color := range cand.Sockets {
			if i >= MaxGems {
				break
			}
			var ids []int32
			if color == proto.GemColor_GemColorMeta {
				ids = metas
			} else {
				for _, o := range shortlist {
					if !se.s.unpricedGems[o.id] {
						ids = append(ids, o.id)
					}
				}
			}
			for _, id := range ids {
				trial := st.l.Items[slot]
				trial.Gems[i] = id
				if se.trial(st, change{proto.ItemSlot(slot), trial}) > st.value+improvement && se.apply(st, change{proto.ItemSlot(slot), trial}) {
					improved = true
				}
			}
		}
	}
	return improved
}

// regemAll reruns the exact gem DP over every free slot at the current slopes, and keeps it if it
// scores better.
func (se *searcher) regemAll(st *state, locked *[NumSlots]bool) bool {
	var slots []proto.ItemSlot
	for slot, c := range st.l.Items {
		if c.ItemID != 0 && !se.pool.Locked[slot] && !locked[slot] {
			slots = append(slots, proto.ItemSlot(slot))
		}
	}
	regemmed, _, err := se.pool.Gem(st.l, slots, se.linear(st.total))
	if err != nil || regemmed == st.l {
		return false
	}
	var changes []change
	for _, slot := range slots {
		if regemmed.Items[slot] != st.l.Items[slot] {
			changes = append(changes, change{slot, regemmed.Items[slot]})
		}
	}
	if se.trial(st, changes...) <= st.value+improvement {
		return false
	}
	return se.apply(st, changes...)
}

// polish is exact coordinate ascent: each free slot's best option, a whole-loadout regem, then a
// socket-by-socket gem sweep, until nothing improves. Slots in locked keep their choice.
func (se *searcher) polish(st *state, locked *[NumSlots]bool) {
	for round := 0; round < polishRounds; round++ {
		if se.r.ctx.Err() != nil {
			return
		}
		improved := false
		for slot := range st.l.Items {
			if !se.pool.Locked[slot] && !locked[slot] && se.bestItem(st, proto.ItemSlot(slot)) {
				improved = true
			}
		}
		if se.regemAll(st, locked) {
			improved = true
		}
		if se.gemSweep(st, locked) {
			improved = true
		}
		if !improved {
			return
		}
	}
}

const polishRounds = 20

// annealSteps is how many moves one annealing run makes, by effort.
func annealSteps(effort proto.OptimizerEffort) int {
	switch effort {
	case proto.OptimizerEffort_OptimizerEffortQuick:
		return 10000
	case proto.OptimizerEffort_OptimizerEffortThorough:
		return 40000
	}
	return 20000
}

// annealRuns is how many annealing runs each start gets, on different random numbers, by effort.
func annealRuns(effort proto.OptimizerEffort) int {
	switch effort {
	case proto.OptimizerEffort_OptimizerEffortQuick:
		return 3
	case proto.OptimizerEffort_OptimizerEffortThorough:
		return 8
	}
	return 5
}

// randomMove proposes one change: another item in a slot, another enchant or reforge, or another gem.
func (se *searcher) randomMove(st *state) []change {
	rng := se.rng
	var free []proto.ItemSlot
	for slot := range st.l.Items {
		if !se.pool.Locked[slot] && len(se.opts[slot]) > 0 {
			free = append(free, proto.ItemSlot(slot))
		}
	}
	if len(free) == 0 {
		return nil
	}
	slot := free[rng.Intn(len(free))]
	cur := st.l.Items[slot]
	mh, oh := proto.ItemSlot_ItemSlotMainHand, proto.ItemSlot_ItemSlotOffHand
	switch roll := rng.Float64(); {
	case roll < 0.6 || cur.ItemID == 0:
		o := se.opts[slot][rng.Intn(len(se.opts[slot]))]
		if slot == oh {
			if item, ok := se.pool.item(mh, st.l.Items[mh].ItemID); ok && st.l.Items[mh].ItemID != 0 && se.pool.fillsHands(item) {
				return nil
			}
		}
		around := st.total.Subtract(st.stats[slot]).Add(o.cand.Item.Stats)
		v, gp := se.plan(around)
		meta := int32(0)
		if metaSocket(o.cand) >= 0 {
			meta = se.s.headMeta(st.l)
		}
		changes := []change{{slot, se.s.quickChoice(slot, o.cand, v, gp, meta)}}
		if slot == mh && se.pool.fillsHands(&o.cand.Item) && st.l.Items[oh].ItemID != 0 {
			changes = append(changes, change{oh, ItemChoice{}})
		}
		return changes
	case roll < 0.8:
		cand := se.pool.candidate(slot, cur.ItemID)
		if cand == nil {
			return nil
		}
		c := cur
		if rng.Intn(2) == 0 && len(cand.Enchants) > 0 {
			i := rng.Intn(len(cand.Enchants) + 1)
			c.Enchant = 0
			if i < len(cand.Enchants) && !se.s.unpricedEnchants[slot][cand.Enchants[i].ID] {
				c.Enchant = cand.Enchants[i].ID
			}
		} else if len(cand.Reforges) > 0 {
			i := rng.Intn(len(cand.Reforges) + 1)
			c.ReforgeFrom, c.ReforgeTo = 0, 0
			if i < len(cand.Reforges) {
				c.ReforgeFrom, c.ReforgeTo = cand.Reforges[i].From, cand.Reforges[i].To
			}
		}
		return []change{{slot, c}}
	default:
		cand := se.pool.candidate(slot, cur.ItemID)
		if cand == nil || len(cand.Sockets) == 0 || len(se.pool.Gems) == 0 {
			return nil
		}
		i := rng.Intn(min(len(cand.Sockets), MaxGems))
		g := se.pool.Gems[rng.Intn(len(se.pool.Gems))]
		if (g.Gem.Color == proto.GemColor_GemColorMeta) != (cand.Sockets[i] == proto.GemColor_GemColorMeta) || se.s.unpricedGems[g.Gem.ID] {
			return nil
		}
		c := cur
		c.Gems[i] = g.Gem.ID
		return []change{{slot, c}}
	}
}

// anneal walks from start with Metropolis acceptance, cooling geometrically by 1000x, and returns the
// best loadout it passed.
func (se *searcher) anneal(start *state, steps int) *state {
	cur, best := start.clone(), start.clone()
	rng := se.rng
	// start hot enough to take a typical losing move
	var losses []float64
	for i := 0; i < 64; i++ {
		if changes := se.randomMove(cur); changes != nil {
			if d := se.trial(cur, changes...) - cur.value; d < 0 {
				losses = append(losses, -d)
			}
		}
	}
	temperature := 1.0
	if len(losses) > 0 {
		slices.Sort(losses)
		temperature = max(losses[len(losses)/2], 1e-6)
	}
	for step := 0; step < steps; step++ {
		if step%512 == 0 && se.r.ctx.Err() != nil {
			break
		}
		changes := se.randomMove(cur)
		if changes == nil {
			continue
		}
		t := temperature * math.Pow(1e-3, float64(step)/float64(steps))
		d := se.trial(cur, changes...) - cur.value
		if d < 0 && rng.Float64() >= math.Exp(d/t) {
			continue
		}
		if se.apply(cur, changes...) && cur.value > best.value+improvement {
			best = cur.clone()
		}
	}
	return best
}

// repair makes l legal and priced: choices outside the options become the slot's best option by lower
// bound, then items that break a rule are dropped and gems that break one are redone by the DP. It
// returns nil when l can't be fixed.
func (se *searcher) repair(l Loadout) *state {
	out := l
	for slot, c := range l.Items {
		if se.pool.Locked[slot] {
			out.Items[slot] = se.s.seed.Items[slot]
			continue
		}
		if c.ItemID == 0 {
			continue
		}
		if se.pool.candidate(proto.ItemSlot(slot), c.ItemID) != nil && se.s.priced(proto.ItemSlot(slot), c) &&
			slices.ContainsFunc(se.opts[slot], func(o *option) bool { return o.cand.Item.ID == c.ItemID }) {
			if _, err := se.pool.checkSlot(out, proto.ItemSlot(slot)); err == nil {
				continue
			}
		}
		out.Items[slot] = ItemChoice{}
	}
	st := se.newState(out)
	for attempt := 0; attempt < 2*NumSlots; attempt++ {
		err := se.check(st.l)
		if err == nil {
			return st
		}
		var re *RuleError
		switch {
		case se.gemRuleBroken(err):
			var slots []proto.ItemSlot
			for slot, c := range st.l.Items {
				if c.ItemID != 0 && !se.pool.Locked[slot] {
					slots = append(slots, proto.ItemSlot(slot))
				}
			}
			regemmed, _, gemErr := se.pool.Gem(st.l, slots, se.linear(st.total))
			if gemErr != nil {
				// no gemming activates the meta, so drop it
				regemmed = st.l
				for slot, c := range regemmed.Items {
					if cand := se.pool.candidate(proto.ItemSlot(slot), c.ItemID); cand != nil && !se.pool.Locked[slot] {
						if i := metaSocket(cand); i >= 0 {
							regemmed.Items[slot].Gems[i] = 0
						}
					}
				}
			}
			st = se.newState(regemmed)
		case errors.As(err, &re) && re.Slot >= 0 && !se.pool.Locked[re.Slot]:
			se.set(st, re.Slot, ItemChoice{})
		default:
			return nil
		}
	}
	return nil
}

// starts are the loadouts annealing begins from: the seed, a greedy fill from nothing, the warm
// starts, and a greedy fill with each priced set's pieces forced in.
func (se *searcher) starts() []*state {
	var out []*state
	add := func(st *state) {
		if st != nil {
			out = append(out, st)
		}
	}
	add(se.repair(se.s.seed))
	var empty Loadout
	empty.RacialTraits = se.s.seed.RacialTraits
	greedy := se.repair(empty)
	if greedy != nil {
		var none [NumSlots]bool
		se.polish(greedy, &none)
		add(greedy)
	}
	for _, l := range se.r.r.WarmStarts {
		add(se.repair(l))
	}
	for _, set := range se.s.sets {
		if greedy == nil || !set.hasTwo && !set.hasFour {
			continue
		}
		st := greedy.clone()
		for slot := range se.opts {
			var piece *option
			for _, o := range se.opts[slot] {
				if o.cand.Item.SetName == set.name && (piece == nil || o.lb > piece.lb) {
					piece = o
				}
			}
			if piece != nil && !se.pool.Locked[slot] {
				se.apply(st, change{proto.ItemSlot(slot), se.configure(st, proto.ItemSlot(slot), piece.cand)})
			}
		}
		add(st)
	}
	return out
}

// search returns up to maxTop loadouts the surrogate rates best, all different sets (see setKey),
// best first.
func (se *searcher) search() ([]Loadout, error) {
	effort := se.r.asked.Settings.GetEffort()
	steps, runs := annealSteps(effort), annealRuns(effort)
	starts := se.starts()
	locals := make([]*state, len(starts)*runs)
	se.parallel(len(locals), func(f *searcher, i int) {
		var none [NumSlots]bool
		st := f.anneal(starts[i/runs], steps)
		f.polish(st, &none)
		locals[i] = st
	})
	if err := se.r.ctx.Err(); err != nil {
		return nil, err
	}
	if len(locals) == 0 {
		return nil, nil
	}
	best := locals[0]
	for _, st := range locals[1:] {
		if st.value > best.value {
			best = st
		}
	}

	// the best with each slot's runners-up forced in, the rest polished around them
	type forced struct {
		slot    proto.ItemSlot
		changes []change
	}
	var alts []forced
	for slot := range best.l.Items {
		slot := proto.ItemSlot(slot)
		if se.pool.Locked[slot] {
			continue
		}
		for _, changes := range se.runnersUp(best, slot, forcedRunnersUp) {
			alts = append(alts, forced{slot, changes})
		}
	}
	polished := make([]*state, len(alts))
	se.parallel(len(alts), func(f *searcher, i int) {
		st := best.clone()
		if !f.apply(st, alts[i].changes...) {
			return
		}
		var locked [NumSlots]bool
		locked[alts[i].slot] = true
		f.polish(st, &locked)
		polished[i] = st
	})
	if err := se.r.ctx.Err(); err != nil {
		return nil, err
	}
	candidates := slices.Clone(locals)
	for _, st := range polished {
		if st != nil {
			candidates = append(candidates, st)
		}
	}

	slices.SortStableFunc(candidates, func(a, b *state) int {
		switch {
		case a.value > b.value:
			return -1
		case a.value < b.value:
			return 1
		}
		return 0
	})
	var out []Loadout
	seen := map[[NumSlots]int32]bool{}
	for _, st := range candidates {
		key := setKey(st.l)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, st.l)
		if len(out) == maxTop {
			break
		}
	}
	return out, nil
}

// forcedRunnersUp is how many runners-up per slot the search forces into its best set and polishes.
const forcedRunnersUp = 3

// runnersUp are the best configured moves to another item in slot, from st, best first. Each is a
// change to that slot alone, plus the off hand when a main hand fills both hands; moves that break
// a rule are skipped.
func (se *searcher) runnersUp(st *state, slot proto.ItemSlot, n int) [][]change {
	mh, oh := proto.ItemSlot_ItemSlotMainHand, proto.ItemSlot_ItemSlotOffHand
	type move struct {
		value   float64
		changes []change
	}
	var moves []move
	current := st.l.Items[slot].ItemID
	for _, o := range se.opts[slot] {
		if o.cand.Item.ID == current {
			continue
		}
		var extra []change
		if slot == mh && se.pool.fillsHands(&o.cand.Item) && st.l.Items[oh].ItemID != 0 {
			extra = []change{{oh, ItemChoice{}}}
		}
		c := se.configure(st, slot, o.cand, extra...)
		changes := append([]change{{slot, c}}, extra...)
		moves = append(moves, move{se.trial(st, changes...), changes})
	}
	slices.SortStableFunc(moves, func(a, b move) int {
		switch {
		case a.value > b.value:
			return -1
		case a.value < b.value:
			return 1
		}
		return 0
	})
	var out [][]change
	for _, m := range moves {
		if len(out) == n {
			break
		}
		trial := st.clone()
		if se.apply(trial, m.changes...) {
			var fixed []change
			for _, ch := range m.changes {
				fixed = append(fixed, change{ch.slot, trial.l.Items[ch.slot]})
			}
			out = append(out, fixed)
		}
	}
	return out
}

// setKey is l's items by slot, with each ring and trinket pair in id order: swapping the two sims
// the same.
func setKey(l Loadout) [NumSlots]int32 {
	var key [NumSlots]int32
	for slot, c := range l.Items {
		key[slot] = c.ItemID
	}
	for _, a := range pairSlots {
		if key[a] > key[a+1] {
			key[a], key[a+1] = key[a+1], key[a]
		}
	}
	return key
}
