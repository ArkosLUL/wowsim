package optimizer

import (
	"context"
	"math"
	"math/rand"
	"os"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
	"github.com/wowsims/wotlk/sim/core/stats"
	goproto "google.golang.org/protobuf/proto"
)

// Made-up one-handers, two of each type and alike otherwise, so a pair of them only differs in which
// talents cover it.
var wtTwins = map[proto.WeaponType][2]int32{
	proto.WeaponType_WeaponTypeSword: {9730001, 9730002},
	proto.WeaponType_WeaponTypeAxe:   {9730003, 9730004},
	proto.WeaponType_WeaponTypeMace:  {9730005, 9730006},
	proto.WeaponType_WeaponTypeFist:  {9730007, 9730008},
}

var wtOnce sync.Once

func wtRegister() {
	wtOnce.Do(func() {
		var items []*proto.SimItem
		for wt, ids := range wtTwins {
			for _, id := range ids {
				items = append(items, kaWeapon(id, proto.HandType_HandTypeOneHand, wt, 300, 450, 2.6, stats.Stats{}))
			}
		}
		core.AddToDatabase(&proto.SimDatabase{Items: items})
	})
}

// withTalent sets the talent at pos (from 1) in tree (from 0) to rank, padding the tree with zeros.
func withTalent(talents string, tree, pos int, rank byte) string {
	trees := strings.Split(talents, "-")
	for len(trees) <= tree {
		trees = append(trees, "")
	}
	t := []byte(trees[tree])
	for len(t) < pos {
		t = append(t, '0')
	}
	t[pos-1] = '0' + rank
	trees[tree] = string(t)
	return strings.Join(trees, "-")
}

// The slow suite's combat rogue and fury warrior, and where their weapon talents sit.
const (
	wtRogueTalents   = "00532000523-0252051050035010223100501251"
	wtWarriorTalents = "302023102331-305052000520310053120500351"
	wtCQC            = 9  // combat
	wtRogueMace      = 14 // combat
	wtHackAndSlash   = 16 // combat
	wtPoleaxe        = 13 // arms
	wtWarriorMace    = 15 // arms
	wtSwordSpec      = 16 // arms
)

func talentNames(talents []weaponTalent) []string {
	var out []string
	for _, t := range talents {
		out = append(out, t.name)
	}
	return out
}

func TestWeaponTalentsOf(t *testing.T) {
	rogue := func(talents string) *proto.Player {
		return &proto.Player{Class: proto.Class_ClassRogue, TalentsString: talents}
	}
	warrior := func(talents string) *proto.Player {
		return &proto.Player{Class: proto.Class_ClassWarrior, TalentsString: talents}
	}
	allRogue := withTalent(withTalent(wtRogueTalents, 1, wtRogueMace, 5), 1, wtHackAndSlash, 5)
	allWarrior := withTalent(withTalent(withTalent(wtWarriorTalents, 0, wtPoleaxe, 5), 0, wtWarriorMace, 5), 0, wtSwordSpec, 5)
	for _, c := range []struct {
		player *proto.Player
		want   []string
	}{
		{rogue(wtRogueTalents), []string{"Close Quarters Combat"}},
		{rogue(allRogue), []string{"Close Quarters Combat", "Hack and Slash", "Mace Specialization"}},
		{warrior(wtWarriorTalents), nil},
		// the sim applies warrior Mace Specialization whatever the weapons
		{warrior(allWarrior), []string{"Sword Specialization", "Poleaxe Specialization"}},
		{&proto.Player{Class: proto.Class_ClassPaladin, TalentsString: "050501-05-05232051203331302133231331"}, nil},
	} {
		got, err := weaponTalentsOf(c.player)
		if err != nil {
			t.Fatal(err)
		}
		if names := talentNames(got); !slices.Equal(names, c.want) {
			t.Errorf("%v %q: weapon talents %v, want %v", c.player.Class, c.player.TalentsString, names, c.want)
		}
	}
	if _, err := weaponTalentsOf(rogue(strings.Repeat("0", 90))); err == nil {
		t.Error("a talent string past the trees should be an error")
	}
}

// wtFakeRun takes req through the stages before the search on a fake sim.
func wtFakeRun(t *testing.T, req *proto.OptimizeGearRequest, metrics func(Point) (Metrics, error)) (*run, *surrogate) {
	t.Helper()
	r, err := PrepareRequest(req)
	if err != nil {
		t.Fatal(err)
	}
	pool, err := CompilePool(r)
	if err != nil {
		t.Fatal(err)
	}
	rn := &run{
		ctx:    context.Background(),
		asked:  r,
		r:      r,
		budget: EffortBudget(r.Settings.GetEffort()),
		rng:    rand.New(rand.NewSource(1)),
		pool:   pool,
		best:   r.Seed,
		ref:    r.Equipped,
	}
	rn.total = int64(rn.budget.Evaluations) * int64(rn.budget.Iterations)
	rn.screen = roundShards(rn.budget.Iterations / 4)
	rn.eval = &countingEvaluator{inner: newKnownEvaluator(metrics), have: map[Point]int{}}
	if rn.obj, err = NewObjective(rn.ctx, rn.eval, r, rn.budget.Iterations); err != nil {
		t.Fatal(err)
	}
	evals, err := rn.evaluate([]Point{{Loadout: r.Seed}}, rn.budget.Iterations)
	if err != nil {
		t.Fatal(err)
	}
	rn.seedEval, rn.bestEval = evals[0], evals[0]
	rn.seedJ = rn.obj.Score(rn.seedEval)
	resp, rangeStats, err := rn.measureCurves(rn.newStatScreen())
	if err != nil {
		t.Fatal(err)
	}
	s := newSurrogate(pool, r.Seed, resp)
	if err := s.setFloorsAndCeilings(nil, nil, rangeStats); err != nil {
		t.Fatal(err)
	}
	if err := rn.measureEffects(s); err != nil {
		t.Fatal(err)
	}
	return rn, s
}

// On a fake sim where a sword in each hand is worth bonus more than the two swords apart, a warrior
// with Sword Specialization gets that bonus as the term, and every pair of hands scores what the fake
// says, the off-hand sword next to an axe included. Without the talent there's nothing to price.
func TestWeaponTalentTermPricesThePair(t *testing.T) {
	mh, oh := proto.ItemSlot_ItemSlotMainHand, proto.ItemSlot_ItemSlotOffHand
	run := func(talents string, bonus float64) (*run, *surrogate) {
		req := kaRequest(proto.OptimizerEffort_OptimizerEffortQuick)
		req.Base.Raid.Parties[0].Players[0].TalentsString = talents
		return wtFakeRun(t, req, func(p Point) (Metrics, error) {
			m, err := kaMetrics(p)
			if err == nil && p.Loadout.Items[mh].ItemID == kaSword && p.Loadout.Items[oh].ItemID == kaSword {
				m[MetricDPS] += bonus
			}
			return m, err
		})
	}

	const bonus = 150.0
	rn, s := run(withTalent(rtFuryNoTG, 0, wtSwordSpec, 5), bonus)
	if len(s.weaponTerms) != 1 || s.weaponTerms[0].talent.name != "Sword Specialization" {
		t.Fatalf("weapon terms %+v, want Sword Specialization's alone", s.weaponTerms)
	}
	want := bonus * rn.obj.coef[MetricDPS]
	if got := s.weaponTerms[0].both; math.Abs(got.Mean-want) > 0.01*want+2*got.SE {
		t.Errorf("the term is %.2f ± %.2f, want %.2f", got.Mean, got.SE, want)
	}
	if s.seedWeapon != 0 {
		t.Errorf("the seed's sword and dagger read %.2f, want nothing", s.seedWeapon)
	}

	weapons := []int32{kaSword, kaAxe, kaDagger}
	var points []Point
	for _, m := range weapons {
		for _, o := range weapons {
			l := rn.r.Seed
			l.Items[mh], l.Items[oh] = ItemChoice{ItemID: m}, ItemChoice{ItemID: o}
			points = append(points, Point{Loadout: l})
		}
	}
	great := rn.r.Seed
	great.Items[mh], great.Items[oh] = ItemChoice{ItemID: kaGreat}, ItemChoice{}
	points = append(points, Point{Loadout: great})
	evals, err := rn.evaluate(points, rn.budget.Iterations)
	if err != nil {
		t.Fatal(err)
	}
	for i, p := range points {
		truth := rn.obj.Delta(rn.seedEval, evals[i]).Mean
		pred := s.value(p.Loadout) - s.value(rn.r.Seed)
		if math.Abs(pred-truth) > 2 {
			t.Errorf("main hand %d, off hand %d: the surrogate says %+.2f, the fake %+.2f", p.Loadout.Items[mh].ItemID, p.Loadout.Items[oh].ItemID, pred, truth)
		}
	}

	if _, s := run(rtFuryNoTG, bonus); len(s.weaponTerms) != 0 {
		t.Errorf("a warrior without weapon talents got terms %+v", s.weaponTerms)
	}
	if _, s := run(withTalent(rtFuryNoTG, 0, wtSwordSpec, 5), 0); len(s.weaponTerms) != 0 {
		t.Errorf("a pair worth nothing more than its hands got terms %+v", s.weaponTerms)
	}
}

// wtSurrogate is crSurrogate with made-up weapon terms, one per sign and one more for maces.
func wtSurrogate(t *testing.T) (*run, *surrogate) {
	t.Helper()
	rn, s := crSurrogate(t)
	s.weaponTerms = []weaponTerm{
		{weaponTalent{"swords", []proto.WeaponType{proto.WeaponType_WeaponTypeSword}}, Estimate{Mean: -60, SE: 4}},
		{weaponTalent{"axes", []proto.WeaponType{proto.WeaponType_WeaponTypeAxe, proto.WeaponType_WeaponTypePolearm}}, Estimate{Mean: 45, SE: 4}},
		{weaponTalent{"maces", []proto.WeaponType{proto.WeaponType_WeaponTypeMace}}, Estimate{Mean: 30, SE: 4}},
	}
	mh, oh := proto.ItemSlot_ItemSlotMainHand, proto.ItemSlot_ItemSlotOffHand
	s.seedWeapon = s.weaponPairOf(s.seed.Items[mh].ItemID, s.seed.Items[oh].ItemID)
	if s.seedWeapon != 45 {
		t.Fatalf("the seed's two axes read %.1f, want the axes' 45", s.seedWeapon)
	}
	return rn, s
}

// The weapon terms move with both hands, and a move's trial score, the state it leaves and a
// whole-loadout rescore agree on them.
func TestWeaponTermKeepsTheSearchConsistent(t *testing.T) {
	rn, s := wtSurrogate(t)
	se := newSearcher(s, rn)
	mh, oh := proto.ItemSlot_ItemSlotMainHand, proto.ItemSlot_ItemSlotOffHand
	st := se.newState(s.seed)

	// a sword next to an axe loses the axes' 45 and gains nothing
	swapped := s.seed
	swapped.Items[mh] = ItemChoice{ItemID: crBigSword}
	plain := *s
	plain.weaponTerms, plain.seedWeapon = nil, 0
	if got, want := se.trial(st, change{mh, swapped.Items[mh]}), plain.value(swapped)-45; math.Abs(got-want) > 1e-6*math.Abs(want) {
		t.Errorf("the swap scores %.3f, want %.3f: the gear alone less the axes' term", got, want)
	}

	rng := rand.New(rand.NewSource(11))
	var slots []proto.ItemSlot
	for slot := range se.all {
		if len(se.all[slot]) > 0 && !s.pool.Locked[slot] {
			slots = append(slots, proto.ItemSlot(slot))
		}
	}
	moved := 0
	for step := 0; step < 400; step++ {
		slot := slots[rng.Intn(len(slots))]
		if step%3 > 0 {
			slot = []proto.ItemSlot{mh, oh}[rng.Intn(2)]
		}
		var c ItemChoice
		if opts := se.all[slot]; rng.Intn(6) > 0 {
			c = se.configure(st, slot, opts[rng.Intn(len(opts))].cand)
		}
		changes := []change{{slot, c}}
		if slot == mh && c.ItemID != 0 {
			if item, _ := s.pool.item(mh, c.ItemID); s.pool.fillsHands(item) {
				changes = append(changes, change{oh, ItemChoice{}})
			}
		}
		next := st.l
		for _, ch := range changes {
			next.Items[ch.slot] = ch.choice
		}
		if got, want := se.trial(st, changes...), s.value(next); math.Abs(got-want) > 1e-6*max(1, math.Abs(want)) {
			t.Fatalf("step %d: trial scores %.6f, a rescore %.6f", step, got, want)
		}
		before := st.weapon
		for _, ch := range changes {
			se.set(st, ch.slot, ch.choice)
		}
		if got, want := st.value, s.value(st.l); math.Abs(got-want) > 1e-6*max(1, math.Abs(want)) {
			t.Fatalf("step %d: the state holds %.6f, a rescore %.6f", step, got, want)
		}
		if st.weapon != s.weaponDelta(&st.l) {
			t.Fatalf("step %d: the state's weapon term is %.3f, its loadout's %.3f", step, st.weapon, s.weaponDelta(&st.l))
		}
		if st.weapon != before {
			moved++
		}
	}
	if moved == 0 {
		t.Error("no step moved the weapon term")
	}
}

// Pruning stays safe with the weapon terms: whatever the other hand holds and wherever the rest of the
// gear puts the floor, a weapon adds no more than its upper bound, and the bare weapon no less than
// its lower bound.
func TestWeaponTermBoundsHold(t *testing.T) {
	rn, s := wtSurrogate(t)
	se := newSearcher(s, rn)
	mh, oh := proto.ItemSlot_ItemSlotMainHand, proto.ItemSlot_ItemSlotOffHand
	rng := rand.New(rand.NewSource(13))
	widened := 0
	for _, slot := range []proto.ItemSlot{mh, oh} {
		other := pairedHand(slot)
		others := []ItemChoice{{}}
		for _, cand := range s.pool.Slots[other] {
			others = append(others, ItemChoice{ItemID: cand.Item.ID})
		}
		for _, o := range se.all[slot] {
			if lb, ub := s.weaponBounds(slot, &o.cand.Item); lb != 0 || ub != 0 {
				widened++
			}
			var choices []ItemChoice
			for _, c := range kaChoices(s.pool, slot) {
				if c.ItemID == o.cand.Item.ID {
					choices = append(choices, c)
				}
			}
			for _, oc := range others {
				// nothing goes next to a main hand that fills both hands
				mhItem := &o.cand.Item
				if slot == oh {
					mhItem = s.handItem(mh, oc.ItemID)
				}
				if oc.ItemID != 0 && s.pool.fillsHands(mhItem) {
					continue
				}
				for k := 0; k < 10; k++ {
					l := s.seed
					l.Items[other] = oc
					l.Items[slot] = ItemChoice{}
					var rest stats.Stats
					rest[stats.Expertise] = rng.Float64()*160 - 80
					rest[stats.Strength] = rng.Float64()*200 - 100
					total := s.seedStats.Add(rest)
					empty := s.score(total, s.weaponDelta(&l), nil, s.racialDelta(&l))
					for i, c := range choices {
						l.Items[slot] = c
						v := s.score(total.Add(s.choiceStats(slot, c)), s.choiceResidual(slot, c)+s.weaponDelta(&l), nil, s.racialDelta(&l)) - empty
						if v > o.ub+1e-9 {
							t.Errorf("%s %+v next to %d adds %.2f, over its upper bound %.2f", slot, c, oc.ItemID, v, o.ub)
						}
						if i == 0 && v < o.lb-1e-9 {
							t.Errorf("%s %d bare, next to %d, adds %.2f, under its lower bound %.2f", slot, c.ItemID, oc.ItemID, v, o.lb)
						}
					}
				}
			}
		}
	}
	if widened == 0 {
		t.Error("no weapon's bounds moved with the weapon terms")
	}
}

// bestItem's and bestPartner's moves in the hands score at most their bounds with the weapon terms in
// play, from random gear, under a floor that's missed or met.
func TestWeaponTermMoveBoundsHold(t *testing.T) {
	rn, s := wtSurrogate(t)
	se := newSearcher(s, rn)
	mh, oh := proto.ItemSlot_ItemSlotMainHand, proto.ItemSlot_ItemSlotOffHand
	rng := rand.New(rand.NewSource(17))
	floor := s.floors[0].min
	defer func() { s.floors[0].min = floor }()
	checked := 0
	for k := 0; k < 40; k++ {
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
		for _, slot := range []proto.ItemSlot{mh, oh} {
			if slot == oh && st.l.Items[mh].ItemID != 0 && s.pool.fillsHands(s.handItem(mh, st.l.Items[mh].ItemID)) {
				continue
			}
			bounds := se.moveBounds(st, slot)
			for _, o := range se.opts[slot] {
				ub := bounds.of(o)
				var extra []change
				if slot == mh && s.pool.fillsHands(&o.cand.Item) && st.l.Items[oh].ItemID != 0 {
					extra = []change{{oh, ItemChoice{}}}
				}
				c := se.configure(st, slot, o.cand, extra...)
				changes := append([]change{{slot, c}}, extra...)
				if slot != mh || len(extra) > 0 || st.l.Items[oh].ItemID != 0 || s.pool.fillsHands(&o.cand.Item) {
					checked++
					if v := se.trial(st, changes...); v > ub {
						t.Errorf("%s %+v scores %.3f, over its bound %.3f", slot, c, v, ub)
					}
					continue
				}
				bare, gaps := se.moveBase(st, &se.partnerGaps, changes...)
				for _, p := range se.opts[oh] {
					pub := se.moveBound(bare, gaps, se.racialWith(&o.cand.Item, &p.cand.Item), p)
					pc := se.configure(st, oh, p.cand, changes...)
					checked++
					v := se.trial(st, append(changes, change{oh, pc})...)
					if v > ub || v > pub {
						t.Errorf("%+v with %+v scores %.3f, over the main hand's bound %.3f or the off hand's %.3f", c, pc, v, ub, pub)
					}
				}
			}
		}
	}
	if checked == 0 {
		t.Fatal("no move checked")
	}
}

// wtPlayer is the slow suite's combat rogue or fury warrior as a Night Elf, which has no weapon
// specialization, with talents.
func wtPlayer(tb testing.TB, class proto.Class, talents string) *proto.Player {
	var player *proto.Player
	switch class {
	case proto.Class_ClassRogue:
		player = &proto.Player{
			Name:      "Rogue",
			Class:     proto.Class_ClassRogue,
			Equipment: core.GetGearSet("../../ui/rogue/gear_sets", "p3_combat").GearSet,
			Rotation:  core.GetAplRotation("../../ui/rogue/apls", "combat_expose").Rotation,
			Glyphs: &proto.Glyphs{
				Major1: int32(proto.RogueMajorGlyph_GlyphOfKillingSpree),
				Major2: int32(proto.RogueMajorGlyph_GlyphOfTricksOfTheTrade),
				Major3: int32(proto.RogueMajorGlyph_GlyphOfRupture),
			},
			Spec: &proto.Player_Rogue{Rogue: &proto.Rogue{Options: &proto.Rogue_Options{
				MhImbue: proto.Rogue_Options_DeadlyPoison,
				OhImbue: proto.Rogue_Options_InstantPoison,
			}}},
			Consumes: &proto.Consumes{Flask: proto.Flask_FlaskOfEndlessRage, DefaultPotion: proto.Potions_PotionOfSpeed},
			Buffs:    core.FullIndividualBuffs,
		}
	default:
		player = presetOptimizeRequest(tb, "fury_p1").Base.Raid.Parties[0].Players[0]
	}
	player.Race, player.TalentsString = proto.Race_RaceNightElf, talents
	return player
}

// wtRequest puts player alone with full buffs against the default encounter, holding the uncovered
// twins, with every slot but the hands locked and the pool offering the uncovered and the covered
// twins in each hand.
func wtRequest(tb testing.TB, player *proto.Player, uncovered, covered proto.WeaponType) *proto.OptimizeGearRequest {
	wtRegister()
	req := presetOptimizeRequest(tb, "fury_p1")
	mh, oh := proto.ItemSlot_ItemSlotMainHand, proto.ItemSlot_ItemSlotOffHand
	player.Equipment = goproto.Clone(player.Equipment).(*proto.EquipmentSpec)
	player.Equipment.Items[mh] = &proto.ItemSpec{Id: wtTwins[uncovered][0]}
	player.Equipment.Items[oh] = &proto.ItemSpec{Id: wtTwins[uncovered][1]}
	req.Base.Raid = core.SinglePlayerRaidProto(player, core.FullPartyBuffs, core.FullRaidBuffs, core.FullDebuffs)
	req.Settings = &proto.OptimizerSettings{ContentPhase: 1, Effort: proto.OptimizerEffort_OptimizerEffortNormal,
		RacialMode: proto.OptimizerRacialMode_OptimizerRacialKeepCurrent, DisableStatCaps: true}
	for slot := proto.ItemSlot(0); int(slot) < NumSlots; slot++ {
		if slot != mh && slot != oh {
			req.Settings.LockedSlots = append(req.Settings.LockedSlots, slot)
		}
	}
	req.Pool = &proto.CandidatePool{CatalogDate: "2026-09-19", Slots: []*proto.SlotPool{
		{Slot: mh, ItemIds: []int32{wtTwins[uncovered][0], wtTwins[covered][0]}},
		{Slot: oh, ItemIds: []int32{wtTwins[uncovered][1], wtTwins[covered][1]}},
	}}
	return req
}

// wtMeasure prices req's hands as a run does before its search, on real sims at iterations. The
// twins carry the same stats, so there are no curves to measure.
func wtMeasure(t *testing.T, req *proto.OptimizeGearRequest, iterations int) (*run, *surrogate) {
	t.Helper()
	r, err := PrepareRequest(req)
	if err != nil {
		t.Fatal(err)
	}
	pool, err := CompilePool(r)
	if err != nil {
		t.Fatal(err)
	}
	keep, err := WeightedMetrics(r.Settings)
	if err != nil {
		t.Fatal(err)
	}
	rn := &run{
		ctx:    context.Background(),
		asked:  r,
		r:      r,
		budget: Budget{Iterations: iterations, Evaluations: 200},
		rng:    rand.New(rand.NewSource(1)),
		pool:   pool,
		best:   r.Seed,
		ref:    r.Equipped,
	}
	rn.total = int64(rn.budget.Evaluations) * int64(rn.budget.Iterations)
	rn.screen = roundShards(iterations / 4)
	rn.eval = &countingEvaluator{inner: NewSimEvaluator(r, keep...), have: map[Point]int{}}
	if rn.obj, err = NewObjective(rn.ctx, rn.eval, r, iterations); err != nil {
		t.Fatal(err)
	}
	evals, err := rn.evaluate([]Point{{Loadout: r.Seed}}, iterations)
	if err != nil {
		t.Fatal(err)
	}
	rn.seedEval, rn.bestEval = evals[0], evals[0]
	rn.seedJ = rn.obj.Score(rn.seedEval)
	s := newSurrogate(pool, r.Seed, nil)
	if err := rn.measureEffects(s); err != nil {
		t.Fatal(err)
	}
	return rn, s
}

// Per weapon talent, the surrogate's value of each pair of hands (both, one or neither covered)
// against a paired sim of that pair on other random numbers, within 2 standard errors. Rogue Mace
// Specialization counts once whichever hands hold maces, Poleaxe Specialization's crit damage too,
// and two swords share Sword Specialization's cooldown, so their residuals alone overcount a covered
// pair: the term has to make that up.
func TestWeaponTalentPairsMatchTheSim(t *testing.T) {
	if raceEnabled {
		t.Skip("real sims of every pair take minutes under -race")
	}
	if !core.WITH_DB {
		t.Skip("needs the with_db item data")
	}
	const iterations, checkIterations = 2000, 4000
	sword, axe, mace, fist := proto.WeaponType_WeaponTypeSword, proto.WeaponType_WeaponTypeAxe, proto.WeaponType_WeaponTypeMace, proto.WeaponType_WeaponTypeFist
	rogueOnly := func(pos int) string {
		return withTalent(withTalent(wtRogueTalents, 1, wtCQC, 0), 1, pos, 5)
	}
	cases := []struct {
		name               string
		class              proto.Class
		talents            string
		uncovered, covered proto.WeaponType
		// the talent's pair is worth measurably less than its hands apart
		priced bool
	}{
		{"rogue Close Quarters Combat", proto.Class_ClassRogue, wtRogueTalents, sword, fist, false},
		{"rogue Hack and Slash", proto.Class_ClassRogue, rogueOnly(wtHackAndSlash), fist, sword, false},
		{"rogue Mace Specialization", proto.Class_ClassRogue, rogueOnly(wtRogueMace), fist, mace, true},
		{"warrior Sword Specialization", proto.Class_ClassWarrior, withTalent(wtWarriorTalents, 0, wtSwordSpec, 5), fist, sword, true},
		{"warrior Poleaxe Specialization", proto.Class_ClassWarrior, withTalent(wtWarriorTalents, 0, wtPoleaxe, 5), fist, axe, true},
	}
	mh, oh := proto.ItemSlot_ItemSlotMainHand, proto.ItemSlot_ItemSlotOffHand
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			req := wtRequest(t, wtPlayer(t, c.class, c.talents), c.uncovered, c.covered)
			rn, s := wtMeasure(t, req, iterations)
			var term *weaponTerm
			if len(s.weaponTerms) > 0 {
				term = &s.weaponTerms[0]
				t.Logf("term %+.1f ± %.1f", term.both.Mean, term.both.SE)
			} else {
				t.Log("no term: the pair is worth what its hands are apart")
			}
			if c.priced && (term == nil || term.both.Mean >= 0) {
				t.Fatalf("want a negative term, got %+v", term)
			}

			check := goproto.Clone(req).(*proto.OptimizeGearRequest)
			check.Base.SimOptions.RandomSeed = 9001
			rc, err := PrepareRequest(check)
			if err != nil {
				t.Fatal(err)
			}
			keep, _ := WeightedMetrics(rc.Settings)
			mhs := [2]int32{wtTwins[c.uncovered][0], wtTwins[c.covered][0]}
			ohs := [2]int32{wtTwins[c.uncovered][1], wtTwins[c.covered][1]}
			var points []Point
			for a := range mhs {
				for b := range ohs {
					l := rn.r.Seed
					l.Items[mh], l.Items[oh] = ItemChoice{ItemID: mhs[a]}, ItemChoice{ItemID: ohs[b]}
					points = append(points, Point{Loadout: l})
				}
			}
			evals, err := NewSimEvaluator(rc, keep...).Evaluate(context.Background(), points, checkIterations)
			if err != nil {
				t.Fatal(err)
			}
			for i, p := range points[1:] {
				a, b := (i+1)/2, (i+1)%2
				sim := rn.obj.Delta(evals[0], evals[i+1])
				pred := s.value(p.Loadout) - s.value(rn.r.Seed)
				se := math.Hypot(s.items[mh][mhs[a]].SE, math.Hypot(s.items[oh][ohs[b]].SE, s.items[oh][ohs[0]].SE))
				separable := pred
				if a == 1 && b == 1 && term != nil {
					se, separable = math.Hypot(se, term.both.SE), pred-term.both.Mean
				}
				tol := 2 * math.Hypot(se, sim.SE)
				t.Logf("main hand covered %v, off hand covered %v: surrogate %+.1f (without the term %+.1f), sim %+.1f ± %.1f, 2 se %.1f",
					a == 1, b == 1, pred, separable, sim.Mean, sim.SE, tol)
				if math.Abs(pred-sim.Mean) > tol {
					t.Errorf("main hand covered %v, off hand covered %v: the surrogate says %+.1f, the sim %+.1f ± %.1f", a == 1, b == 1, pred, sim.Mean, sim.SE)
				}
				if c.priced && a == 1 && b == 1 && math.Abs(separable-sim.Mean) <= tol {
					t.Errorf("both hands covered: the residuals alone say %+.1f, as close to the sim's %+.1f as the term", separable, sim.Mean)
				}
			}
		})
	}
}

// The sim gives a warrior Mace Specialization whatever the hands hold, which is why it's no weapon
// talent here: with fists it adds as much as with maces, and maces add nothing over fists.
func TestWarriorMaceSpecializationIgnoresTheWeapons(t *testing.T) {
	if !core.WITH_DB {
		t.Skip("needs the with_db item data")
	}
	const iterations = 1000
	mh, oh := proto.ItemSlot_ItemSlotMainHand, proto.ItemSlot_ItemSlotOffHand
	fist, mace := proto.WeaponType_WeaponTypeFist, proto.WeaponType_WeaponTypeMace
	evaluate := func(rank byte, wt proto.WeaponType) *Evaluation {
		req := wtRequest(t, wtPlayer(t, proto.Class_ClassWarrior, withTalent(wtWarriorTalents, 0, wtWarriorMace, rank)), fist, mace)
		r, err := PrepareRequest(req)
		if err != nil {
			t.Fatal(err)
		}
		l := r.Seed
		l.Items[mh], l.Items[oh] = ItemChoice{ItemID: wtTwins[wt][0]}, ItemChoice{ItemID: wtTwins[wt][1]}
		evals, err := NewSimEvaluator(r, MetricDPS).Evaluate(context.Background(), []Point{{Loadout: l}}, iterations)
		if err != nil {
			t.Fatal(err)
		}
		return evals[0]
	}
	fists, maces, untalented := evaluate(5, fist), evaluate(5, mace), evaluate(0, fist)
	if d := Delta(fists, maces, MetricDPS); math.Abs(d.Mean) > 2*d.SE {
		t.Errorf("with Mace Specialization, maces deal %+.1f ± %.1f DPS over fists", d.Mean, d.SE)
	}
	if d := Delta(untalented, fists, MetricDPS); d.Mean <= 2*d.SE {
		t.Errorf("Mace Specialization adds %+.1f ± %.1f DPS with fists, want a gain", d.Mean, d.SE)
	}
}

// assasinSeed is Assasin's phase 2 gear in the batch export at 0ccf8aa60 (the roster of 2026-10-06),
// Remorse in the main hand.
func assasinSeed() *proto.EquipmentSpec {
	reforge := func(from, to int32) *proto.ItemReforge { return &proto.ItemReforge{FromStatType: from, ToStatType: to} }
	return &proto.EquipmentSpec{Items: []*proto.ItemSpec{
		{Id: 46125, Enchant: 3817, Gems: []int32{41398, 45879}, Reforge: reforge(32, 37)},
		{Id: 44664, Gems: []int32{36766}, Reforge: reforge(32, 31)},
		{Id: 45265, Enchant: 3835, Gems: []int32{45879}},
		{Id: 45224, Enchant: 3730},
		{Id: 46123, Enchant: 3832, Gems: []int32{45879, 45879}},
		{Id: 45108, Enchant: 3756, Gems: []int32{45879}},
		{Id: 46124, Enchant: 3603, Gems: []int32{45879, 45879}},
		{Id: 45491, Enchant: 3601, Gems: []int32{45879, 45879}, Reforge: reforge(36, 31)},
		{Id: 46126, Enchant: 3328, Gems: []int32{36766, 40052}, Reforge: reforge(36, 31)},
		{Id: 45564, Enchant: 3826, Gems: []int32{42702, 36766}, Reforge: reforge(31, 36)},
		{Id: 45503, Enchant: 3839, Reforge: reforge(36, 31)},
		{Id: 45157, Enchant: 3839},
		{Id: 40684, Reforge: reforge(32, 31)},
		{Id: 45522},
		{Id: 45142, Enchant: 3789},
		{Id: 45484, Enchant: 3789, Gems: []int32{45879}},
		{Id: 45570, Enchant: 3843, Reforge: reforge(31, 37)},
	}}
}

// assasinRequest is the roster's Assasin (a Night Elf wearing Orc traits, Combat with Close Quarters
// Combat 5) alone with full buffs, as slowRequest builds its cases, over a phase 2 pool at Normal.
// The batch simmed him with the buffs his raid gives instead.
func assasinRequest(tb testing.TB) *proto.OptimizeGearRequest {
	player := &proto.Player{
		Name:          "Assasin",
		Race:          proto.Race_RaceNightElf,
		RacialTraits:  proto.Race_RaceOrc,
		Class:         proto.Class_ClassRogue,
		Equipment:     assasinSeed(),
		Rotation:      core.GetAplRotation("../../ui/rogue/apls", "combat_expose").Rotation,
		TalentsString: "00532000514-0252051050035010223100501251",
		Glyphs: &proto.Glyphs{
			Major1: int32(proto.RogueMajorGlyph_GlyphOfExposeArmor),
			Major2: int32(proto.RogueMajorGlyph_GlyphOfKillingSpree),
			Major3: int32(proto.RogueMajorGlyph_GlyphOfRupture),
			Minor1: int32(proto.RogueMinorGlyph_GlyphOfBlurredSpeed),
			Minor2: int32(proto.RogueMinorGlyph_GlyphOfVanish),
			Minor3: int32(proto.RogueMinorGlyph_GlyphOfSafeFall),
		},
		Spec: &proto.Player_Rogue{Rogue: &proto.Rogue{Options: &proto.Rogue_Options{
			MhImbue: proto.Rogue_Options_DeadlyPoison,
			OhImbue: proto.Rogue_Options_InstantPoison,
		}}},
		Consumes: &proto.Consumes{
			Flask:         proto.Flask_FlaskOfEndlessRage,
			Food:          proto.Food_FoodHeartyRhino,
			DefaultPotion: proto.Potions_PotionOfSpeed,
		},
		Buffs: core.FullIndividualBuffs,
		Professions: []proto.Profession{proto.Profession_Alchemy, proto.Profession_Blacksmithing, proto.Profession_Enchanting,
			proto.Profession_Engineering, proto.Profession_Inscription, proto.Profession_Jewelcrafting, proto.Profession_Leatherworking,
			proto.Profession_Tailoring, proto.Profession_Mining, proto.Profession_Herbalism, proto.Profession_Skinning},
	}
	req := presetOptimizeRequest(tb, "fury_p1")
	req.Base.Raid = core.SinglePlayerRaidProto(player, core.FullPartyBuffs, core.FullRaidBuffs, core.FullDebuffs)
	req.Settings = &proto.OptimizerSettings{ContentPhase: 2, Effort: proto.OptimizerEffort_OptimizerEffortNormal,
		RacialMode: proto.OptimizerRacialMode_OptimizerRacialKeepCurrent, DisableStatCaps: true}
	req.Pool = realisticPool(tb, player, 2, keepUnlessOutclassed(tb, player))
	req.Equipped = goproto.Clone(player.Equipment).(*proto.EquipmentSpec)
	return req
}

// Assasin's three main hands next to Bladetwister, scored by the surrogate against their sims, and
// the sims against the batch export's. A measurement, not a check: it runs the curves and residuals
// of a Normal run, so it only runs with OPTIMIZER_ASSASIN set:
//
//	tools/acore/dock.sh exec env OPTIMIZER_ASSASIN=1 go test --tags=with_db -count=1 -timeout 30m -run TestAssasinMainHands -v ./sim/optimizer/
func TestAssasinMainHands(t *testing.T) {
	if os.Getenv("OPTIMIZER_ASSASIN") == "" {
		t.Skip("set OPTIMIZER_ASSASIN to run it")
	}
	traceHook = t.Logf
	defer func() { traceHook = nil }()
	req := assasinRequest(t)
	r, err := PrepareRequest(req)
	if err != nil {
		t.Fatal(err)
	}
	pool, err := CompilePool(r)
	if err != nil {
		t.Fatal(err)
	}
	if err := pool.Check(r.Seed); err != nil {
		t.Logf("the seed breaks a rule here: %v", err)
	}
	keep, _ := WeightedMetrics(r.Settings)
	rn := &run{
		ctx:    context.Background(),
		asked:  r,
		r:      r,
		budget: EffortBudget(r.Settings.GetEffort()),
		rng:    rand.New(rand.NewSource(1)),
		pool:   pool,
		best:   r.Seed,
		ref:    r.Equipped,
	}
	rn.total = int64(rn.budget.Evaluations) * int64(rn.budget.Iterations)
	rn.screen = roundShards(rn.budget.Iterations / 4)
	rn.eval = &countingEvaluator{inner: NewSimEvaluator(r, keep...), have: map[Point]int{}}
	if rn.obj, err = NewObjective(rn.ctx, rn.eval, r, rn.budget.Iterations); err != nil {
		t.Fatal(err)
	}
	evals, err := rn.evaluate([]Point{{Loadout: r.Seed}}, rn.budget.Iterations)
	if err != nil {
		t.Fatal(err)
	}
	rn.seedEval, rn.bestEval = evals[0], evals[0]
	rn.seedJ = rn.obj.Score(rn.seedEval)
	resp, rangeStats, err := rn.measureCurves(rn.newStatScreen())
	if err != nil {
		t.Fatal(err)
	}
	s := newSurrogate(pool, r.Seed, resp)
	if err := s.setFloorsAndCeilings(nil, nil, rangeStats); err != nil {
		t.Fatal(err)
	}
	if err := rn.measureEffects(s); err != nil {
		t.Fatal(err)
	}

	mh := proto.ItemSlot_ItemSlotMainHand
	hands := []struct {
		name   string
		id     int32
		export float64
	}{{"Remorse", 45142, 8969.7}, {"Golem-Shard Sticker", 45246, 8898.4}, {"Fang of Oblivion", 45607, 8335.4}}
	check := goproto.Clone(req).(*proto.OptimizeGearRequest)
	check.Base.SimOptions.RandomSeed = 9001
	rc, err := PrepareRequest(check)
	if err != nil {
		t.Fatal(err)
	}
	var points []Point
	for _, h := range hands {
		l := r.Seed
		l.Items[mh] = ItemChoice{ItemID: h.id, Enchant: 3789}
		points = append(points, Point{Loadout: l})
	}
	checks, err := NewSimEvaluator(rc, keep...).Evaluate(context.Background(), points, assasinCheckIterations)
	if err != nil {
		t.Fatal(err)
	}
	coef := rn.obj.coef[MetricDPS]
	for i, h := range hands {
		dps := checks[i].Metrics[MetricDPS]
		res, priced := s.items[mh][h.id]
		pred := s.value(points[i].Loadout) - s.value(points[0].Loadout)
		sim := rn.obj.Delta(checks[0], checks[i])
		t.Logf("%s: %.1f ± %.1f DPS here, %.1f in the export (%+.1f%%); against Remorse the surrogate says %+.1f J (%+.1f DPS), the sim %+.1f ± %.1f J; residual %+.1f ± %.1f, priced %v",
			h.name, dps.Mean, dps.SE, h.export, 100*(dps.Mean-h.export)/h.export, pred, pred/coef, sim.Mean, sim.SE, res.Mean, res.SE, priced && !s.unpriced[mh][h.id])
	}
	for _, term := range s.weaponTerms {
		t.Logf("weapon term %s: %+.1f ± %.1f", term.talent.name, term.both.Mean, term.both.SE)
	}
}

// assasinCheckIterations is how long TestAssasinMainHands sims each main hand on other random
// numbers, as the slow suite's check does.
const assasinCheckIterations = 10000
