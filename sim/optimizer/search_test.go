package optimizer

import (
	"context"
	"errors"
	"fmt"
	"math"
	"math/rand"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
	"github.com/wowsims/wotlk/sim/core/stats"
	goproto "google.golang.org/protobuf/proto"
)

// Made-up items for the known-answer tests, clear of the other tests' ids.
const (
	kaHeadMeta   = 9700001 // meta and red sockets
	kaHeadSet    = 9700002
	kaHeadPlain  = 9700003
	kaShoulder   = 9700011 // crit, reforgeable
	kaShoulder2  = 9700012 // set
	kaChest      = 9700021 // yellow socket
	kaChestSet   = 9700022
	kaRing       = 9700031
	kaRingHit    = 9700032
	kaRingUnique = 9700033
	kaTrinketA   = 9700041 // +80 DPS effect
	kaTrinketB   = 9700042 // +60 DPS effect
	kaTrinketAP  = 9700043
	kaTrinketAP2 = 9700044
	kaTrinketBad = 9700045 // its sims fail
	kaSword      = 9700051
	kaAxe        = 9700052
	kaGreat      = 9700053 // two-hander
	kaDagger     = 9700054

	kaRed       = 9700101
	kaYellow    = 9700102
	kaMetaProc  = 9700103 // +40 DPS effect, needs a red gem
	kaMetaPlain = 9700104

	kaBerserk = 9700201 // weapon enchant, +25 DPS effect

	kaSetName = "Known Answer Battlegear"
)

func kaItem(id int32, t proto.ItemType, s stats.Stats, set string, sockets ...proto.GemColor) *proto.SimItem {
	return &proto.SimItem{Id: id, Type: t, Stats: s.ToFloatArray(), GemSockets: sockets, SetName: set,
		SocketBonus: stats.Stats{stats.Strength: 6}.ToFloatArray()}
}

func kaWeapon(id int32, hand proto.HandType, wt proto.WeaponType, lo, hi, speed float64, s stats.Stats) *proto.SimItem {
	return &proto.SimItem{Id: id, Type: proto.ItemType_ItemTypeWeapon, HandType: hand, WeaponType: wt,
		WeaponDamageMin: lo, WeaponDamageMax: hi, WeaponSpeed: speed, Stats: s.ToFloatArray()}
}

var kaDatabase = &proto.SimDatabase{
	Items: []*proto.SimItem{
		kaItem(kaHeadMeta, proto.ItemType_ItemTypeHead, stats.Stats{stats.Strength: 40}, "", proto.GemColor_GemColorMeta, proto.GemColor_GemColorRed),
		kaItem(kaHeadSet, proto.ItemType_ItemTypeHead, stats.Stats{stats.Strength: 38, stats.MeleeHit: 20}, kaSetName),
		kaItem(kaHeadPlain, proto.ItemType_ItemTypeHead, stats.Stats{stats.Strength: 50}, ""),
		kaItem(kaShoulder, proto.ItemType_ItemTypeShoulder, stats.Stats{stats.Strength: 30, stats.MeleeCrit: 20, stats.SpellCrit: 20}, ""),
		kaItem(kaShoulder2, proto.ItemType_ItemTypeShoulder, stats.Stats{stats.Strength: 22, stats.MeleeHit: 18}, kaSetName),
		kaItem(kaChest, proto.ItemType_ItemTypeChest, stats.Stats{stats.Strength: 45}, "", proto.GemColor_GemColorYellow),
		kaItem(kaChestSet, proto.ItemType_ItemTypeChest, stats.Stats{stats.Strength: 40, stats.MeleeHit: 20}, kaSetName),
		kaItem(kaRing, proto.ItemType_ItemTypeFinger, stats.Stats{stats.Strength: 25}, ""),
		kaItem(kaRingHit, proto.ItemType_ItemTypeFinger, stats.Stats{stats.MeleeHit: 30}, ""),
		kaItem(kaRingUnique, proto.ItemType_ItemTypeFinger, stats.Stats{stats.Strength: 34}, ""),
		kaItem(kaTrinketA, proto.ItemType_ItemTypeTrinket, stats.Stats{stats.AttackPower: 20}, ""),
		kaItem(kaTrinketB, proto.ItemType_ItemTypeTrinket, stats.Stats{stats.MeleeHit: 20}, ""),
		kaItem(kaTrinketAP, proto.ItemType_ItemTypeTrinket, stats.Stats{stats.AttackPower: 60}, ""),
		kaItem(kaTrinketAP2, proto.ItemType_ItemTypeTrinket, stats.Stats{stats.AttackPower: 72}, ""),
		kaItem(kaTrinketBad, proto.ItemType_ItemTypeTrinket, stats.Stats{stats.AttackPower: 500}, ""),
		kaWeapon(kaSword, proto.HandType_HandTypeOneHand, proto.WeaponType_WeaponTypeSword, 120, 220, 2.6, stats.Stats{stats.Strength: 12}),
		kaWeapon(kaAxe, proto.HandType_HandTypeOneHand, proto.WeaponType_WeaponTypeAxe, 150, 260, 2.7, stats.Stats{}),
		kaWeapon(kaGreat, proto.HandType_HandTypeTwoHand, proto.WeaponType_WeaponTypeSword, 330, 480, 3.6, stats.Stats{stats.Strength: 30}),
		kaWeapon(kaDagger, proto.HandType_HandTypeOneHand, proto.WeaponType_WeaponTypeDagger, 90, 150, 1.8, stats.Stats{stats.MeleeHit: 12}),
	},
	Gems: []*proto.SimGem{
		{Id: kaRed, Color: proto.GemColor_GemColorRed, Stats: stats.Stats{stats.Strength: 10}.ToFloatArray()},
		{Id: kaYellow, Color: proto.GemColor_GemColorYellow, Stats: stats.Stats{stats.MeleeHit: 10}.ToFloatArray()},
		{Id: kaMetaProc, Color: proto.GemColor_GemColorMeta, Stats: stats.Stats{stats.Strength: 6}.ToFloatArray()},
		{Id: kaMetaPlain, Color: proto.GemColor_GemColorMeta, Stats: stats.Stats{stats.Strength: 14}.ToFloatArray()},
	},
	Enchants: []*proto.SimEnchant{{EffectId: kaBerserk}},
}

var kaOnce sync.Once

// kaRegister adds the made-up data to core, and no-op effects for the items, gem and enchant the fake
// prices, so NeedsSim sees them. Core's effect maps aren't locked, so this runs once, before any sim.
func kaRegister() {
	kaOnce.Do(func() {
		core.AddToDatabase(kaDatabase)
		for _, id := range []int32{kaTrinketA, kaTrinketB, kaTrinketBad, kaMetaProc} {
			core.NewItemEffect(id, func(core.Agent) {})
		}
		core.NewEnchantEffect(kaBerserk, func(core.Agent) {})
	})
}

// kaRequest is a dual-wielding warrior without Titan's Grip, in a pool over the made-up items.
func kaRequest(effort proto.OptimizerEffort) *proto.OptimizeGearRequest {
	kaRegister()
	seed := testEquipment(map[proto.ItemSlot]*proto.ItemSpec{
		proto.ItemSlot_ItemSlotHead:     {Id: kaHeadPlain},
		proto.ItemSlot_ItemSlotShoulder: {Id: kaShoulder},
		proto.ItemSlot_ItemSlotChest:    {Id: kaChest},
		proto.ItemSlot_ItemSlotFinger1:  {Id: kaRing},
		proto.ItemSlot_ItemSlotFinger2:  {Id: kaRingHit},
		proto.ItemSlot_ItemSlotTrinket1: {Id: kaTrinketAP},
		proto.ItemSlot_ItemSlotTrinket2: {Id: kaTrinketAP2},
		proto.ItemSlot_ItemSlotMainHand: {Id: kaSword},
		proto.ItemSlot_ItemSlotOffHand:  {Id: kaDagger},
	})
	target := &proto.Player{
		Name:          "Target",
		Race:          proto.Race_RaceOrc,
		Class:         proto.Class_ClassWarrior,
		TalentsString: rtFuryNoTG,
		Equipment:     seed,
		Spec:          &proto.Player_Warrior{Warrior: &proto.Warrior{Options: &proto.Warrior_Options{}}},
		Rotation:      &proto.APLRotation{Type: proto.APLRotation_TypeAPL},
	}
	weapons := func(slot proto.ItemSlot, ids ...int32) *proto.SlotPool {
		sp := &proto.SlotPool{Slot: slot, ItemIds: ids}
		for _, id := range ids {
			sp.EnchantOptions = append(sp.EnchantOptions, &proto.ItemEnchantOptions{ItemId: id, EnchantIds: []int32{kaBerserk}})
		}
		return sp
	}
	rings := []int32{kaRing, kaRingHit, kaRingUnique}
	trinkets := []int32{kaTrinketA, kaTrinketB, kaTrinketAP, kaTrinketAP2, kaTrinketBad}
	return &proto.OptimizeGearRequest{
		Base: &proto.RaidSimRequest{
			Raid:       &proto.Raid{Parties: []*proto.Party{{Players: []*proto.Player{target}}}},
			Encounter:  &proto.Encounter{Duration: 180, Targets: []*proto.Target{{}}},
			SimOptions: &proto.SimOptions{RandomSeed: 7},
		},
		Settings: &proto.OptimizerSettings{ContentPhase: 1, Effort: effort, RacialMode: proto.OptimizerRacialMode_OptimizerRacialKeepCurrent},
		Pool: &proto.CandidatePool{
			Slots: []*proto.SlotPool{
				{Slot: proto.ItemSlot_ItemSlotHead, ItemIds: []int32{kaHeadMeta, kaHeadSet, kaHeadPlain}},
				{Slot: proto.ItemSlot_ItemSlotShoulder, ItemIds: []int32{kaShoulder, kaShoulder2}},
				{Slot: proto.ItemSlot_ItemSlotChest, ItemIds: []int32{kaChest, kaChestSet}},
				{Slot: proto.ItemSlot_ItemSlotFinger1, ItemIds: rings},
				{Slot: proto.ItemSlot_ItemSlotFinger2, ItemIds: rings},
				{Slot: proto.ItemSlot_ItemSlotTrinket1, ItemIds: trinkets},
				{Slot: proto.ItemSlot_ItemSlotTrinket2, ItemIds: trinkets},
				weapons(proto.ItemSlot_ItemSlotMainHand, kaSword, kaAxe, kaGreat, kaDagger),
				weapons(proto.ItemSlot_ItemSlotOffHand, kaSword, kaAxe, kaDagger),
			},
			GemIds: []int32{kaRed, kaYellow, kaMetaProc, kaMetaPlain},
			MetaConditions: []*proto.MetaGemCondition{
				{GemId: kaMetaProc, Constraints: []*proto.MetaColorConstraint{{Red: 1, MinTotal: 1}}},
			},
			CatalogItems: []*proto.CatalogItem{
				{Id: kaRingUnique, UniqueEquipped: true},
				{Id: kaTrinketA, UniqueEquipped: true},
				{Id: kaTrinketB, UniqueEquipped: true},
				{Id: kaTrinketBad, UniqueEquipped: true},
				{Id: kaShoulder, StatsCount: 2},
			},
			CatalogDate: "2026-09-19",
		},
	}
}

// kaDPS is the fake sim: linear in most stats, a hit cap at 120, weapon damage, and the made-up
// effects, meta, enchant and set bonus. Like the real sim it applies a meta whether or not it's active.
func kaDPS(gear stats.Stats, l Loadout) float64 {
	hit := gear[stats.MeleeHit]
	dps := 3000 + 2*gear[stats.Strength] + gear[stats.AttackPower] + 1.5*gear[stats.MeleeCrit] + 1.2*gear[stats.MeleeHaste] +
		0.8*gear[stats.Expertise] + 3*min(hit, 120) + 0.4*max(hit-120, 0)
	setPieces := 0
	for slot, c := range l.Items {
		if c.ItemID == 0 {
			continue
		}
		item, _ := core.LookupItem(c.ItemID)
		if item.SetName == kaSetName {
			setPieces++
		}
		switch c.ItemID {
		case kaTrinketA:
			dps += 80
		case kaTrinketB:
			dps += 60
		}
		avg := (item.WeaponDamageMin + item.WeaponDamageMax) / 2
		switch proto.ItemSlot(slot) {
		case proto.ItemSlot_ItemSlotMainHand:
			dps += 1.5 * avg
		case proto.ItemSlot_ItemSlotOffHand:
			if l.Items[proto.ItemSlot_ItemSlotMainHand].ItemID != 0 {
				dps += 0.75 * avg
			}
		}
		if c.Enchant == kaBerserk {
			dps += 25
		}
		for _, g := range c.Gems {
			if g == kaMetaProc {
				dps += 40
			}
		}
	}
	if setPieces >= 2 {
		dps += 45
	}
	return dps
}

// knownEvaluator is a fake Evaluator for a known metric function. Like SimEvaluator it caches every
// point, extends a cached point with more iterations, pairs points through noise shared per
// iteration, and fails a whole call when any point fails.
type knownEvaluator struct {
	metrics func(Point) (Metrics, error)
	shared  float64
	own     float64

	mu     sync.Mutex
	noise  [][NumMetrics]float64
	cache  map[Point]*Evaluation
	rngs   map[Point]*rand.Rand
	calls  int
	points int
}

func newKnownEvaluator(metrics func(Point) (Metrics, error)) *knownEvaluator {
	return &knownEvaluator{metrics: metrics, shared: 30, own: 0.3, cache: map[Point]*Evaluation{}, rngs: map[Point]*rand.Rand{}}
}

func (f *knownEvaluator) Evaluate(ctx context.Context, points []Point, iterations int) ([]*Evaluation, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	f.points += len(points)
	means := make([]Metrics, len(points))
	for i, p := range points {
		m, err := f.metrics(p)
		if err != nil {
			return nil, &SimError{Point: p, Message: err.Error()}
		}
		means[i] = m
	}
	shared := rand.New(rand.NewSource(int64(len(f.noise)) + 11))
	for len(f.noise) < iterations {
		var n [NumMetrics]float64
		for m := range n {
			n[m] = shared.NormFloat64()
		}
		f.noise = append(f.noise, n)
	}
	out := make([]*Evaluation, len(points))
	for i, p := range points {
		prev := f.cache[p]
		have := 0
		if prev != nil {
			have = prev.Iterations
		}
		if have >= iterations {
			out[i] = prev
			continue
		}
		rng := f.rngs[p]
		if rng == nil {
			rng = rand.New(rand.NewSource(int64(len(f.rngs)) + 101))
			f.rngs[p] = rng
		}
		var values [NumMetrics][]float64
		for m := MetricDPS; m < MetricPDeath; m++ {
			values[m] = make([]float64, iterations-have)
			for it := range values[m] {
				values[m][it] = means[i][m] + f.shared*f.noise[have+it][m] + f.own*rng.NormFloat64()
			}
		}
		s, err := newShard(values, 0, [NumMetrics]bool{true, true, true, true, true})
		if err != nil {
			return nil, err
		}
		out[i] = prev.extend([]shard{s})
		f.cache[p] = out[i]
	}
	return out, nil
}

var errBadTrinket = errors.New("the bad trinket always crashes")

func kaMetrics(p Point) (Metrics, error) {
	for _, c := range p.Loadout.Items {
		if c.ItemID == kaTrinketBad {
			return Metrics{}, errBadTrinket
		}
	}
	return Metrics{MetricDPS: kaDPS(gearStats(p.Loadout).Add(p.Offset), p.Loadout)}, nil
}

// kaChoices lists every choice the pool offers for a slot: each candidate with every enchant, reforge
// and gem combination, plus the empty slot.
func kaChoices(p *Pool, slot proto.ItemSlot) []ItemChoice {
	out := []ItemChoice{{}}
	for _, cand := range p.Slots[slot] {
		base := []ItemChoice{{ItemID: cand.Item.ID}}
		var next []ItemChoice
		for _, c := range base {
			next = append(next, c)
			for _, e := range cand.Enchants {
				c.Enchant = e.ID
				next = append(next, c)
			}
		}
		base, next = next, nil
		for _, c := range base {
			next = append(next, c)
			for _, r := range cand.Reforges {
				c.ReforgeFrom, c.ReforgeTo = r.From, r.To
				next = append(next, c)
			}
		}
		base = next
		for i, color := range cand.Sockets {
			next = nil
			for _, c := range base {
				next = append(next, c)
				for _, g := range p.Gems {
					if (g.Gem.Color == proto.GemColor_GemColorMeta) == (color == proto.GemColor_GemColorMeta) {
						c.Gems[i] = g.Gem.ID
						next = append(next, c)
					}
				}
			}
			base = next
		}
		out = append(out, base...)
	}
	return out
}

// kaPart is what one slot's choice adds to kaDPS, split so the brute force can add it up fast.
type kaPart struct {
	str, ap, crit, haste, exp, hit float64
	// Effects and weapon damage, the off hand's with a main hand present.
	extra    float64
	setPiece int
}

func kaPartOf(slot proto.ItemSlot, c ItemChoice) kaPart {
	if c.ItemID == 0 {
		return kaPart{}
	}
	item := core.NewItem(c.CoreSpec())
	s := item.TotalStats()
	part := kaPart{str: s[stats.Strength], ap: s[stats.AttackPower], crit: s[stats.MeleeCrit], haste: s[stats.MeleeHaste],
		exp: s[stats.Expertise], hit: s[stats.MeleeHit]}
	// an off hand only deals damage next to a main hand, which the brute force always has
	var with, without Loadout
	if slot == proto.ItemSlot_ItemSlotOffHand {
		with.Items[proto.ItemSlot_ItemSlotMainHand] = ItemChoice{ItemID: kaSword}
		without = with
	}
	with.Items[slot] = c
	part.extra = kaDPS(stats.Stats{}, with) - kaDPS(stats.Stats{}, without)
	if item.SetName == kaSetName {
		part.setPiece = 1
	}
	return part
}

// bruteForce returns the best loadout by kaDPS that passes Check, over every combination of the
// pool's choices: every slot the pool fills has an item, the off hand may be empty, and the bad
// trinket is left out. Pairs the rules forbid outright (a unique item twice, anything next to a
// two-hander) are skipped as they come; Check settles the rest, down the ranking.
func bruteForce(t *testing.T, p *Pool) (Loadout, float64) {
	t.Helper()
	var slots []proto.ItemSlot
	var choices [][]ItemChoice
	var parts [][]kaPart
	for slot := range p.Slots {
		if len(p.Slots[slot]) == 0 {
			continue
		}
		slot := proto.ItemSlot(slot)
		var cs []ItemChoice
		var ps []kaPart
		for _, c := range kaChoices(p, slot) {
			if c.ItemID == kaTrinketBad || c.ItemID == 0 && slot != proto.ItemSlot_ItemSlotOffHand {
				continue
			}
			cs = append(cs, c)
			ps = append(ps, kaPartOf(slot, c))
		}
		slots, choices, parts = append(slots, slot), append(choices, cs), append(parts, ps)
	}

	type scored struct {
		l   Loadout
		dps float64
	}
	var best []scored
	const keep = 256
	var l Loadout
	var walk func(depth int, total kaPart)
	walk = func(depth int, total kaPart) {
		if depth == len(slots) {
			dps := 3000 + 2*total.str + total.ap + 1.5*total.crit + 1.2*total.haste + 0.8*total.exp +
				3*min(total.hit, 120) + 0.4*max(total.hit-120, 0) + total.extra
			if total.setPiece >= 2 {
				dps += 45
			}
			if len(best) == keep && dps <= best[keep-1].dps {
				return
			}
			i, _ := slices.BinarySearchFunc(best, dps, func(s scored, d float64) int {
				switch {
				case s.dps > d:
					return -1
				case s.dps < d:
					return 1
				}
				return 0
			})
			best = slices.Insert(best, i, scored{l, dps})
			if len(best) > keep {
				best = best[:keep]
			}
			return
		}
		slot := slots[depth]
		for i, c := range choices[depth] {
			switch slot {
			case proto.ItemSlot_ItemSlotFinger2, proto.ItemSlot_ItemSlotTrinket2:
				if prev := l.Items[slot-1].ItemID; prev == c.ItemID && itemLimit(p.catalog[c.ItemID]) == 1 {
					continue
				}
			case proto.ItemSlot_ItemSlotOffHand:
				if mh, _ := p.item(proto.ItemSlot_ItemSlotMainHand, l.Items[proto.ItemSlot_ItemSlotMainHand].ItemID); c.ItemID != 0 && p.fillsHands(mh) {
					continue
				}
			}
			l.Items[slot] = c
			pt := parts[depth][i]
			walk(depth+1, kaPart{total.str + pt.str, total.ap + pt.ap, total.crit + pt.crit, total.haste + pt.haste,
				total.exp + pt.exp, total.hit + pt.hit, total.extra + pt.extra, total.setPiece + pt.setPiece})
		}
		l.Items[slot] = ItemChoice{}
	}
	walk(0, kaPart{})
	combos := 1
	for _, cs := range choices {
		combos *= len(cs)
	}
	t.Logf("brute force over %d combinations, best %.1f", combos, best[0].dps)
	for _, s := range best {
		if p.Check(s.l) == nil {
			if exact := kaDPS(gearStats(s.l), s.l); !near(exact, s.dps, 1e-6) {
				t.Fatalf("brute force scored %v at %.3f, kaDPS says %.3f", s.l.Items, s.dps, exact)
			}
			return s.l, s.dps
		}
	}
	t.Fatal("no loadout in the brute force's top passes Check")
	return Loadout{}, 0
}

func TestKnownAnswer(t *testing.T) {
	for _, effort := range []proto.OptimizerEffort{proto.OptimizerEffort_OptimizerEffortQuick, proto.OptimizerEffort_OptimizerEffortNormal} {
		t.Run(effort.String(), func(t *testing.T) {
			r, err := PrepareRequest(kaRequest(effort))
			if err != nil {
				t.Fatal(err)
			}
			pool, err := CompilePool(r)
			if err != nil {
				t.Fatal(err)
			}
			want, wantDPS := bruteForce(t, pool)

			fake := newKnownEvaluator(kaMetrics)
			result := optimize(context.Background(), r, r, fake, nil, time.Now())
			if result.ErrorResult != "" {
				t.Fatal(result.ErrorResult)
			}
			got, err := LoadoutFromProto(result.Best.Equipment, result.Best.RacialTraits)
			if err != nil {
				t.Fatal(err)
			}
			gotDPS := kaDPS(gearStats(got), got)
			if gotDPS < wantDPS-1e-6 {
				t.Errorf("picked %.1f DPS, the brute force's best is %.1f\ngot  %v\nwant %v", gotDPS, wantDPS, got.Items, want.Items)
			}
			if err := pool.Check(got); err != nil {
				t.Errorf("the pick breaks a rule: %v", err)
			}
			seedDPS := kaDPS(gearStats(r.Seed), r.Seed)
			if !result.Improved || !near(result.Best.ScoreDelta, gotDPS-seedDPS, 1) {
				t.Errorf("improved = %v, score delta %.2f ± %.2f; want a %.1f gain", result.Improved, result.Best.ScoreDelta, result.Best.ScoreDeltaSe, gotDPS-seedDPS)
			}
			if !slices.ContainsFunc(result.Warnings, func(w string) bool { return strings.Contains(w, "the bad trinket") }) {
				t.Errorf("no warning about the trinket whose sims fail: %q", result.Warnings)
			}
			checkResultShape(t, result, r)
			budget := EffortBudget(effort)
			t.Logf("%s: %.1f DPS (seed %.1f), %d sims of %d budgeted, %d evaluate calls\npick %v", effort, gotDPS, seedDPS, result.TotalSims,
				budget.Evaluations*budget.Iterations, fake.calls, got.Items)
		})
	}
}

// A seed under a stat floor can't be the pick: the best loadout that meets it replaces the seed, even
// scoring lower.
func TestKnownAnswerFloorBreaksTheSeed(t *testing.T) {
	req := kaRequest(proto.OptimizerEffort_OptimizerEffortQuick)
	r, err := PrepareRequest(req)
	if err != nil {
		t.Fatal(err)
	}
	pool, err := CompilePool(r)
	if err != nil {
		t.Fatal(err)
	}
	sheet, err := pool.finalStats(r.Seed)
	if err != nil {
		t.Fatal(err)
	}
	floor := &proto.StatMinimum{Stat: proto.Stat_StatStrength, MinValue: sheet[stats.Strength] + 40}
	req.Settings.StatMinimums = []*proto.StatMinimum{floor}
	if r, err = PrepareRequest(req); err != nil {
		t.Fatal(err)
	}
	if pool, err = CompilePool(r); err != nil {
		t.Fatal(err)
	}

	result := optimize(context.Background(), r, r, newKnownEvaluator(kaMetrics), nil, time.Now())
	if result.ErrorResult != "" {
		t.Fatal(result.ErrorResult)
	}
	got, err := LoadoutFromProto(result.Best.Equipment, result.Best.RacialTraits)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Improved || pool.Check(got) != nil {
		t.Errorf("improved = %v, the pick's rule check %v; want a pick that meets the floor", result.Improved, pool.Check(got))
	}
	if !hasWarning(result, "the seed gear breaks a rule") {
		t.Errorf("warnings = %q, want one about the seed", result.Warnings)
	}
	t.Logf("strength %.0f, floor %.0f; %.1f DPS, seed %.1f", sheetOf(t, pool, got)[stats.Strength], floor.MinValue,
		kaDPS(gearStats(got), got), kaDPS(gearStats(r.Seed), r.Seed))
}

func sheetOf(t *testing.T, p *Pool, l Loadout) stats.Stats {
	t.Helper()
	sheet, err := p.finalStats(l)
	if err != nil {
		t.Fatal(err)
	}
	return sheet
}

// checkResultShape checks what every finished result carries.
func checkResultShape(t *testing.T, result *proto.OptimizerResult, r *Request) {
	t.Helper()
	if result.Seed == nil || result.Best == nil || len(result.Top) == 0 {
		t.Fatalf("best %v, seed %v, %d top sets", result.Best != nil, result.Seed != nil, len(result.Top))
	}
	seen := map[[NumSlots]int32]bool{}
	// the pick comes first, the rest by score
	sortedFrom := 1
	if result.Improved {
		sortedFrom = 2
	}
	for i, top := range result.Top {
		l, err := LoadoutFromProto(top.Equipment, top.RacialTraits)
		if err != nil {
			t.Fatal(err)
		}
		if key := setKey(l); seen[key] {
			t.Errorf("top set %d repeats an earlier one", i)
		} else {
			seen[key] = true
		}
		if i >= sortedFrom && top.ScoreDelta > result.Top[i-1].ScoreDelta+1e-9 {
			t.Errorf("top set %d scores over the one before it", i)
		}
	}
	if result.Improved && fmt.Sprint(result.Top[0].Equipment) != fmt.Sprint(result.Best.Equipment) {
		t.Error("the first top set isn't the best")
	}
	perSlot := map[proto.ItemSlot]int{}
	for _, alt := range result.Alternatives {
		perSlot[alt.Slot]++
		if alt.Item.GetId() == 0 {
			t.Errorf("alternative for %s has no item", alt.Slot)
		}
	}
	for slot, n := range perSlot {
		if n > runnersUp {
			t.Errorf("%d alternatives for %s, want at most %d", n, slot, runnersUp)
		}
	}
	if len(result.Alternatives) == 0 {
		t.Error("no alternatives")
	}
	if result.TotalSims <= 0 || result.ElapsedSeconds <= 0 || result.SimCommit == "" || result.CatalogDate != r.Pool.CatalogDate {
		t.Errorf("sims %d, elapsed %g, commit %q, catalog date %q", result.TotalSims, result.ElapsedSeconds, result.SimCommit, result.CatalogDate)
	}
	if result.Best.Metrics == nil || result.Best.FinalStats == nil {
		t.Errorf("best has no metrics or sheet: %v", result.Best)
	}
}

// The surrogate's per-choice stats are core's TotalStats, gems, socket bonuses and reforges included.
func TestChoiceStatsMatchCore(t *testing.T) {
	p := rtPool(t, nil)
	s := newSurrogate(p, Loadout{}, nil)
	rng := rand.New(rand.NewSource(5))
	checked := 0
	for slot := range p.Slots {
		for _, c := range kaChoicesSample(p, proto.ItemSlot(slot), rng, 40) {
			item := core.NewItem(c.CoreSpec())
			if got, want := s.choiceStats(proto.ItemSlot(slot), c), item.TotalStats(); got != want {
				t.Errorf("%s %+v: surrogate %v, core %v", proto.ItemSlot(slot), c, got, want)
			}
			checked++
		}
	}
	if checked < 100 {
		t.Errorf("only checked %d choices", checked)
	}
}

// kaChoicesSample is up to n random choices for a slot: random enchant, reforge and gems, sockets
// sometimes left empty.
func kaChoicesSample(p *Pool, slot proto.ItemSlot, rng *rand.Rand, n int) []ItemChoice {
	var out []ItemChoice
	for _, cand := range p.Slots[slot] {
		for k := 0; k < n/max(1, len(p.Slots[slot])); k++ {
			c := ItemChoice{ItemID: cand.Item.ID}
			if len(cand.Enchants) > 0 && rng.Intn(2) == 0 {
				c.Enchant = cand.Enchants[rng.Intn(len(cand.Enchants))].ID
			}
			if len(cand.Reforges) > 0 && rng.Intn(2) == 0 {
				r := cand.Reforges[rng.Intn(len(cand.Reforges))]
				c.ReforgeFrom, c.ReforgeTo = r.From, r.To
			}
			for i := range cand.Sockets {
				if rng.Intn(4) > 0 {
					c.Gems[i] = p.Gems[rng.Intn(len(p.Gems))].Gem.ID
				}
			}
			out = append(out, c)
		}
	}
	return out
}

// The surrogate's curves go straight through every knot and carry the end slopes on past them; a
// curve without knots is its two-line fit.
func TestCurve(t *testing.T) {
	knots := []Knot{{-100, Estimate{Mean: -150}}, {0, Estimate{}}, {50, Estimate{Mean: 60}}, {200, Estimate{Mean: 120}}}
	c := newCurve(&ResponseCurve{Knots: knots})
	for _, tc := range []struct{ x, want float64 }{
		{-200, -300}, {-100, -150}, {-50, -75}, {0, 0}, {25, 30}, {50, 60}, {125, 90}, {200, 120}, {300, 160},
	} {
		if got := c.value(tc.x); !near(got, tc.want, 1e-9) {
			t.Errorf("value(%g) = %g, want %g", tc.x, got, tc.want)
		}
	}
	for _, tc := range []struct{ x, want float64 }{{-150, 1.5}, {-10, 1.5}, {0, 1.5}, {10, 1.2}, {100, 0.4}, {250, 0.4}} {
		if got := c.slope[c.segment(tc.x)]; !near(got, tc.want, 1e-9) {
			t.Errorf("slope at %g = %g, want %g", tc.x, got, tc.want)
		}
	}

	for _, fit := range []*ResponseCurve{
		{SlopeBelow: 2, SlopeAbove: 1},
		{Breakpoint: 40, SlopeBelow: 3, SlopeAbove: 0.5},
		{Breakpoint: -40, SlopeBelow: 3, SlopeAbove: 0.5},
	} {
		c := newCurve(fit)
		lo, hi := slices.Min(c.slope), slices.Max(c.slope)
		for x := -100.0; x <= 100; x += 5 {
			if got, want := c.value(x), fit.Value(x); !near(got, want, 1e-9) {
				t.Errorf("breakpoint %g: value(%g) = %g, the fit says %g", fit.Breakpoint, x, got, want)
			}
		}
		if lo != min(fit.SlopeBelow, fit.SlopeAbove) || hi != max(fit.SlopeBelow, fit.SlopeAbove) {
			t.Errorf("breakpoint %g: slopes %v", fit.Breakpoint, c.slope)
		}
	}
}

// Whatever the rest of the gear, a candidate adds between its lower and upper bound to the score.
func TestCandidateBoundsHold(t *testing.T) {
	r, err := PrepareRequest(kaRequest(proto.OptimizerEffort_OptimizerEffortQuick))
	if err != nil {
		t.Fatal(err)
	}
	p, err := CompilePool(r)
	if err != nil {
		t.Fatal(err)
	}
	resp := Response{
		stats.Strength:    {Stat: stats.Strength, SlopeBelow: 2.1, SlopeAbove: 1.9},
		stats.MeleeHit:    {Stat: stats.MeleeHit, Breakpoint: 40, SlopeBelow: 3, SlopeAbove: 0.4},
		stats.AttackPower: {Stat: stats.AttackPower, SlopeBelow: 1, SlopeAbove: 1},
		stats.MeleeCrit:   {Stat: stats.MeleeCrit, SlopeBelow: 1.5, SlopeAbove: 1.4},
	}
	s := newSurrogate(p, r.Seed, resp)
	s.items[proto.ItemSlot_ItemSlotTrinket1][kaTrinketA] = Estimate{Mean: 80, SE: 1}
	s.items[proto.ItemSlot_ItemSlotTrinket1][kaTrinketB] = Estimate{Mean: 60, SE: 1}
	lo, hi := s.slopeBox()
	gb := s.newGemBounds(lo, hi)
	rng := rand.New(rand.NewSource(9))
	for slot := range p.Slots {
		slot := proto.ItemSlot(slot)
		for _, cand := range p.Slots[slot] {
			lb, ub, ok := s.candBounds(slot, cand, lo, hi, gb)
			if !ok {
				continue
			}
			for k := 0; k < 50; k++ {
				var rest stats.Stats
				rest[stats.Strength] = rng.Float64()*400 - 200
				rest[stats.MeleeHit] = rng.Float64()*200 - 100
				for _, c := range kaChoicesSample(p, slot, rng, 20) {
					if c.ItemID != cand.Item.ID {
						continue
					}
					total := s.seedStats.Add(rest)
					v := s.score(total.Add(s.choiceStats(slot, c)), s.choiceResidual(slot, c), nil) - s.score(total, 0, nil)
					if v > ub+1e-9 {
						t.Errorf("%s %+v adds %.2f, over its upper bound %.2f", slot, c, v, ub)
					}
				}
			}
			// the lower bound is reached by some configuration everywhere
			for k := 0; k < 20; k++ {
				var rest stats.Stats
				rest[stats.MeleeHit] = rng.Float64()*200 - 100
				total := s.seedStats.Add(rest)
				best := math.Inf(-1)
				for _, c := range kaChoicesSample(p, slot, rng, 400) {
					if c.ItemID == cand.Item.ID && !skipForLowerBound(s, c) {
						best = max(best, s.score(total.Add(s.choiceStats(slot, c)), s.choiceResidual(slot, c), nil)-s.score(total, 0, nil))
					}
				}
				if best < lb-1e-9 {
					t.Errorf("%s %d: best sampled configuration adds %.2f, under its lower bound %.2f", slot, cand.Item.ID, best, lb)
				}
			}
		}
	}
}

// A set piece's bounds cover every jump it can make, a 2pc worth less than nothing included.
func TestSetJumpBounds(t *testing.T) {
	set := &itemSet{two: Estimate{Mean: -20, SE: 5}, four: Estimate{Mean: 30, SE: 5}, hasTwo: true, hasFour: true}
	lo, hi := set.jumpBounds()
	for pieces := 0; pieces < 5; pieces++ {
		if jump := set.value(pieces+1) - set.value(pieces); jump < lo || jump > hi {
			t.Errorf("piece %d moves the set's value by %g, outside [%g, %g]", pieces+1, jump, lo, hi)
		}
	}
}

// Missing a floor pulls on every stat in its gradient, so the slope box widens both ways.
func TestSlopeBoxFloors(t *testing.T) {
	s := &surrogate{floorWeight: 10}
	s.curves[stats.Strength] = newCurve(&ResponseCurve{SlopeBelow: 2, SlopeAbove: 2})
	s.curved = []stats.Stat{stats.Strength}
	s.floors = []floorTerm{{stat: stats.Armor, gradient: stats.Stats{stats.Strength: -1, stats.Agility: 2}}}
	lo, hi := s.slopeBox()
	if lo[stats.Strength] > 2-10 || hi[stats.Strength] != 2 {
		t.Errorf("strength slopes [%g, %g], want [-8, 2] or wider below", lo[stats.Strength], hi[stats.Strength])
	}
	if !math.IsInf(hi[stats.Agility], 1) || lo[stats.Agility] != 0 {
		t.Errorf("agility slopes [%g, %g], want [0, +Inf]", lo[stats.Agility], hi[stats.Agility])
	}
}

// With every slot locked there's nothing to change, whatever gems the pool has, so nothing is simmed.
func TestKnownAnswerAllLocked(t *testing.T) {
	req := kaRequest(proto.OptimizerEffort_OptimizerEffortQuick)
	for slot := proto.ItemSlot(0); int(slot) < NumSlots; slot++ {
		req.Settings.LockedSlots = append(req.Settings.LockedSlots, slot)
	}
	r, err := PrepareRequest(req)
	if err != nil {
		t.Fatal(err)
	}
	result := optimize(context.Background(), r, r, newKnownEvaluator(kaMetrics), nil, time.Now())
	if result.ErrorResult != "" || result.Improved || result.TotalSims != 0 || !hasWarning(result, "nothing to change") {
		t.Errorf("result = %v, want the seed, unsimmed, with a warning", result)
	}
}

// skipForLowerBound: the lower bound only counts free gems and no meta, since a meta may not activate
// and a unique or limited gem may not fit.
func skipForLowerBound(s *surrogate, c ItemChoice) bool {
	for _, g := range c.Gems {
		if g == 0 {
			continue
		}
		if gc := s.pool.gems[g]; gc == nil || gc.Gem.Color == proto.GemColor_GemColorMeta || !s.freeGem(gc) {
			return true
		}
	}
	return false
}

// A real Fury P1 run over rings and trinkets alone picks within 2 standard errors of the best
// combination, found by simming them all. The trinkets include two Darkmoon Card: Greatness
// variants, which share one proc and don't stack.
func TestFuryRingTrinketBruteForce(t *testing.T) {
	if raceEnabled {
		t.Skip("simming every combination takes minutes under -race")
	}
	const iterations = 1000
	rings := []int32{43993, 40717, 40474, 40075}
	trinkets := []int32{42987, 44253, 40256, 40431, 39257}
	fingers := [2]proto.ItemSlot{proto.ItemSlot_ItemSlotFinger1, proto.ItemSlot_ItemSlotFinger2}
	trinketSlots := [2]proto.ItemSlot{proto.ItemSlot_ItemSlotTrinket1, proto.ItemSlot_ItemSlotTrinket2}

	req := presetOptimizeRequest(t, "fury_p1")
	// the pool has no gems, so the seed's ring and trinkets go without, like every combination
	equipment := req.Base.Raid.Parties[0].Players[0].Equipment
	for _, slot := range append(fingers[:], trinketSlots[:]...) {
		equipment.Items[slot].Gems = nil
	}
	_, catalog := loadRealData(t)
	req.Pool = &proto.CandidatePool{CatalogDate: catalog.Date}
	for _, slot := range fingers {
		req.Pool.Slots = append(req.Pool.Slots, &proto.SlotPool{Slot: slot, ItemIds: rings})
	}
	for _, slot := range trinketSlots {
		req.Pool.Slots = append(req.Pool.Slots, &proto.SlotPool{Slot: slot, ItemIds: trinkets})
	}
	for _, row := range catalog.Items {
		if slices.Contains(rings, row.Id) || slices.Contains(trinkets, row.Id) {
			// no reforges, so the combinations below are all the search can pick from
			row = goproto.Clone(row).(*proto.CatalogItem)
			row.StatsCount = 0
			req.Pool.CatalogItems = append(req.Pool.CatalogItems, row)
		}
	}
	req.Settings = &proto.OptimizerSettings{ContentPhase: 1, Effort: proto.OptimizerEffort_OptimizerEffortQuick,
		RacialMode: proto.OptimizerRacialMode_OptimizerRacialKeepCurrent}
	for slot := proto.ItemSlot(0); int(slot) < NumSlots; slot++ {
		if !slices.Contains(fingers[:], slot) && !slices.Contains(trinketSlots[:], slot) {
			req.Settings.LockedSlots = append(req.Settings.LockedSlots, slot)
		}
	}

	result := Optimize(context.Background(), req, nil)
	if result.ErrorResult != "" {
		t.Fatal(result.ErrorResult)
	}
	pick, err := LoadoutFromProto(result.Best.Equipment, result.Best.RacialTraits)
	if err != nil {
		t.Fatal(err)
	}

	r, err := PrepareRequest(req)
	if err != nil {
		t.Fatal(err)
	}
	points := []Point{{Loadout: pick}}
	pairs := func(ids []int32, slots [2]proto.ItemSlot, each func(Loadout)) func(Loadout) {
		return func(l Loadout) {
			for i, a := range ids {
				for _, b := range ids[i+1:] {
					l.Items[slots[0]], l.Items[slots[1]] = ItemChoice{ItemID: a}, ItemChoice{ItemID: b}
					each(l)
				}
			}
		}
	}
	pairs(rings, fingers, pairs(trinkets, trinketSlots, func(l Loadout) {
		points = append(points, Point{Loadout: l})
	}))(r.Seed)
	evals, err := NewSimEvaluator(r, MetricDPS).Evaluate(context.Background(), points, iterations)
	if err != nil {
		t.Fatal(err)
	}
	best := 1
	for i := range evals[1:] {
		if evals[i+1].Metrics[MetricDPS].Mean > evals[best].Metrics[MetricDPS].Mean {
			best = i + 1
		}
	}
	d := Delta(evals[0], evals[best], MetricDPS)
	t.Logf("%d combinations; best %v, %.1f DPS; pick %v, %.1f ± %.1f under it; %d sims, improved %v", len(points)-1,
		points[best].Loadout.Items[proto.ItemSlot_ItemSlotFinger1:proto.ItemSlot_ItemSlotTrinket2+1], evals[best].Metrics[MetricDPS].Mean,
		pick.Items[proto.ItemSlot_ItemSlotFinger1:proto.ItemSlot_ItemSlotTrinket2+1], d.Mean, d.SE, result.TotalSims, result.Improved)
	if d.Mean > 2*d.SE {
		t.Errorf("the pick is %.1f ± %.1f DPS under the best combination", d.Mean, d.SE)
	}
}
