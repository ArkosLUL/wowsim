package optimizer

import (
	"context"
	"fmt"
	"math"
	"math/rand"
	"slices"
	"sync"
	"testing"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
	"github.com/wowsims/wotlk/sim/core/stats"
)

// Made-up gear for the racial-traits-under-the-caps tests, next to the known-answer suite's.
const (
	crBigSword  = 9720001 // one-handed sword that outdamages every axe
	crMace      = 9720002
	crFist      = 9720003
	crGreatAxe  = 9720004
	crGreatMace = 9720005
	crPolearm   = 9720006
	crShield    = 9720007
	crFrill     = 9720008 // held in the off hand
	crExpRing   = 9720009 // expertise and nothing else
	crSpellRing = 9720010 // spell hit and nothing else
)

var crOnce sync.Once

func crRegister() {
	kaRegister()
	crOnce.Do(func() {
		offHand := func(id int32, wt proto.WeaponType) *proto.SimItem {
			return &proto.SimItem{Id: id, Type: proto.ItemType_ItemTypeWeapon, HandType: proto.HandType_HandTypeOffHand, WeaponType: wt}
		}
		core.AddToDatabase(&proto.SimDatabase{Items: []*proto.SimItem{
			kaWeapon(crBigSword, proto.HandType_HandTypeOneHand, proto.WeaponType_WeaponTypeSword, 300, 400, 2.6, stats.Stats{}),
			kaWeapon(crMace, proto.HandType_HandTypeOneHand, proto.WeaponType_WeaponTypeMace, 140, 250, 2.6, stats.Stats{}),
			kaWeapon(crFist, proto.HandType_HandTypeOneHand, proto.WeaponType_WeaponTypeFist, 140, 250, 2.6, stats.Stats{}),
			kaWeapon(crGreatAxe, proto.HandType_HandTypeTwoHand, proto.WeaponType_WeaponTypeAxe, 330, 480, 3.6, stats.Stats{}),
			kaWeapon(crGreatMace, proto.HandType_HandTypeTwoHand, proto.WeaponType_WeaponTypeMace, 330, 480, 3.6, stats.Stats{}),
			kaWeapon(crPolearm, proto.HandType_HandTypeTwoHand, proto.WeaponType_WeaponTypePolearm, 330, 480, 3.6, stats.Stats{}),
			offHand(crShield, proto.WeaponType_WeaponTypeShield),
			offHand(crFrill, proto.WeaponType_WeaponTypeOffHand),
			kaItem(crExpRing, proto.ItemType_ItemTypeFinger, stats.Stats{stats.Expertise: 45}, ""),
			kaItem(crSpellRing, proto.ItemType_ItemTypeFinger, stats.Stats{stats.SpellHit: 30}, ""),
		}})
	})
}

// crRequest is kaRequest's warrior wearing race's traits with mh and oh, its pool widened by the
// made-up weapons and both rings.
func crRequest(race proto.Race, mh, oh int32) *proto.OptimizeGearRequest {
	crRegister()
	req := kaRequest(proto.OptimizerEffort_OptimizerEffortQuick)
	target := req.Base.Raid.Parties[0].Players[0]
	target.Race = race
	target.Equipment.Items[proto.ItemSlot_ItemSlotMainHand] = &proto.ItemSpec{Id: mh}
	target.Equipment.Items[proto.ItemSlot_ItemSlotOffHand] = &proto.ItemSpec{Id: oh}
	for _, sp := range req.Pool.Slots {
		switch sp.Slot {
		case proto.ItemSlot_ItemSlotFinger1, proto.ItemSlot_ItemSlotFinger2:
			sp.ItemIds = append(sp.ItemIds, crExpRing, crSpellRing)
		case proto.ItemSlot_ItemSlotMainHand:
			sp.ItemIds = append(sp.ItemIds, crBigSword, crMace, crFist, crGreatAxe)
		case proto.ItemSlot_ItemSlotOffHand:
			sp.ItemIds = append(sp.ItemIds, crBigSword, crMace, crFist, crShield, crFrill)
		}
	}
	return req
}

func racesWithWeaponSpecialization(t *testing.T) (specs []proto.Race, none proto.Race) {
	t.Helper()
	for _, race := range allRaces {
		if _, ok := core.RacialWeaponSpecialization(race); ok {
			specs = append(specs, race)
		} else if none == proto.Race_RaceUnknown {
			none = race
		}
	}
	if len(specs) == 0 || none == proto.Race_RaceUnknown {
		t.Fatalf("want races with and without a weapon specialization, got %v and %v", specs, none)
	}
	return specs, none
}

func sheetExpertise(t *testing.T, r *Request, l Loadout) float64 {
	t.Helper()
	sheet, err := playerSheet(r.Base, r.TargetIndex, l, stats.Stats{})
	if err != nil {
		t.Fatal(err)
	}
	return sheet.FinalStats[stats.Expertise]
}

// Whatever the hands hold, the expertise the surrogate counts for a race's weapon specialization is
// exactly what the character sheet gains over a race without one.
func TestRacialExpertiseMatchesTheSheet(t *testing.T) {
	r, err := PrepareRequest(crRequest(proto.Race_RaceOrc, kaAxe, kaAxe))
	if err != nil {
		t.Fatal(err)
	}
	pool, err := CompilePool(r)
	if err != nil {
		t.Fatal(err)
	}
	base := newSurrogate(pool, r.Seed, nil)
	if err := base.setFloorsAndCeilings([]*proto.StatMinimum{{Stat: proto.Stat_StatExpertise, MinValue: 1}}, nil, nil); err != nil {
		t.Fatal(err)
	}
	specs, none := racesWithWeaponSpecialization(t)

	// two-handers, one-handers of every covered type and one no race covers, shield, frill, nothing
	mainHands := []int32{0, crGreatAxe, crGreatMace, kaGreat, crPolearm, kaAxe, crMace, kaSword, crFist, kaDagger}
	offHands := []int32{0, kaAxe, crMace, kaSword, crFist, kaDagger, crShield, crFrill}
	name := func(id int32) string {
		if id == 0 {
			return "nothing"
		}
		item, _ := core.LookupItem(id)
		return fmt.Sprintf("%v %v", item.HandType, item.WeaponType)
	}
	for _, race := range specs {
		s, err := base.forRace(race)
		if err != nil {
			t.Fatal(err)
		}
		covered := 0
		for _, mh := range mainHands {
			for _, oh := range offHands {
				// core moves a lone off-hand weapon to the main hand, so the pool never offers one
				// (checkPlacement)
				if mh == 0 && oh != 0 && oh != crShield && oh != crFrill {
					continue
				}
				l := r.Seed
				l.Items[proto.ItemSlot_ItemSlotMainHand] = ItemChoice{ItemID: mh}
				l.Items[proto.ItemSlot_ItemSlotOffHand] = ItemChoice{ItemID: oh}
				other := l
				l.RacialTraits, other.RacialTraits = race, none
				want := sheetExpertise(t, r, l) - sheetExpertise(t, r, other)
				if got := s.racialExpertise(&l); math.Abs(got-want) > 1e-9 {
					t.Errorf("%v, main hand %s, off hand %s: the surrogate counts %.3f expertise, the sheet gains %.3f", race, name(mh), name(oh), got, want)
				}
				if want > 0 {
					covered++
				}
			}
		}
		if covered == 0 {
			t.Errorf("%v: no pair of hands puts its weapon specialization on the sheet", race)
		}
	}
}

// An Orc seed with nothing in the main hand and an axe in the off hand reads the axe's expertise on
// its sheet, since core equips it in the main hand. Putting the axe there for real changes nothing on
// the sheet, so it mustn't move the surrogate's reading either.
func TestRacialExpertiseOfALoneOffHandSeed(t *testing.T) {
	r, err := PrepareRequest(crRequest(proto.Race_RaceOrc, 0, kaAxe))
	if err != nil {
		t.Fatal(err)
	}
	pool, err := CompilePool(r)
	if err != nil {
		t.Fatal(err)
	}
	s := newSurrogate(pool, r.Seed, nil)
	if err := s.setFloorsAndCeilings([]*proto.StatMinimum{{Stat: proto.Stat_StatExpertise, MinValue: 1}}, nil, nil); err != nil {
		t.Fatal(err)
	}
	mh, oh := proto.ItemSlot_ItemSlotMainHand, proto.ItemSlot_ItemSlotOffHand
	moved := r.Seed
	moved.Items[mh], moved.Items[oh] = moved.Items[oh], ItemChoice{}
	if pool.Check(moved) != nil {
		t.Fatalf("the axe in the main hand should keep every rule: %v", pool.Check(moved))
	}
	total := pool.gearStats(moved)
	want := sheetExpertise(t, r, moved)
	if got := s.floors[0].at(&total, &s.seedStats, s.racialDelta(&moved)); math.Abs(got-want) > 1e-9 {
		t.Errorf("with the axe in the main hand the surrogate reads %.3f expertise, the sheet %.3f", got, want)
	}
}

// crSurrogate is crRequest's Orc dual-wielding axes, priced by made-up curves, with an expertise
// floor 60 over the seed's sheet so the racial expertise always moves the penalty.
func crSurrogate(t *testing.T) (*run, *surrogate) {
	t.Helper()
	r, err := PrepareRequest(crRequest(proto.Race_RaceOrc, kaAxe, kaAxe))
	if err != nil {
		t.Fatal(err)
	}
	floor := sheetExpertise(t, r, r.Seed) + 60
	r.Settings.StatMinimums = []*proto.StatMinimum{{Stat: proto.Stat_StatExpertise, MinValue: floor}}
	pool, err := CompilePool(r)
	if err != nil {
		t.Fatal(err)
	}
	resp := Response{
		stats.Strength:    {Stat: stats.Strength, SlopeBelow: 2, SlopeAbove: 2},
		stats.Expertise:   {Stat: stats.Expertise, Breakpoint: 20, SlopeBelow: 0.8, SlopeAbove: 0.3},
		stats.MeleeHit:    {Stat: stats.MeleeHit, Breakpoint: 40, SlopeBelow: 3, SlopeAbove: 0.4},
		stats.AttackPower: {Stat: stats.AttackPower, SlopeBelow: 1, SlopeAbove: 1},
	}
	s := newSurrogate(pool, r.Seed, resp)
	if err := s.setFloorsAndCeilings(r.Settings.StatMinimums, nil, statsOf(resp)); err != nil {
		t.Fatal(err)
	}
	if s.weaponSpec == nil {
		t.Fatal("an Orc with an expertise floor should price its weapon specialization")
	}
	// weapons need a residual or the search leaves them alone
	for slot := range pool.Slots {
		for i, cand := range pool.Slots[slot] {
			if cand.NeedsSim {
				s.items[familySlot(proto.ItemSlot(slot))][cand.Item.ID] = Estimate{Mean: float64(10 * i), SE: 1}
			}
		}
	}
	rn := &run{ctx: context.Background(), asked: r, r: r, pool: pool, rng: rand.New(rand.NewSource(3))}
	return rn, s
}

// The racial term moves with the weapons both hands hold, and a move's trial score, the state it
// leaves and a whole-loadout rescore all agree on it.
func TestRacialTermKeepsTheSearchConsistent(t *testing.T) {
	rn, s := crSurrogate(t)
	se := newSearcher(s, rn)
	mh, oh := proto.ItemSlot_ItemSlotMainHand, proto.ItemSlot_ItemSlotOffHand

	// a sword next to an axe takes the seed's 5 expertise off the sheet, and the floor prices all of it
	st := se.newState(s.seed)
	sword := ItemChoice{ItemID: crBigSword}
	swapped := s.seed
	swapped.Items[mh] = sword
	lost := 5 * core.ExpertisePerQuarterPercentReduction
	if got := s.racialDelta(&swapped); math.Abs(got+lost) > 1e-9 {
		t.Fatalf("a sword next to an axe leaves racial %.3f, want %.3f", got, -lost)
	}
	plain := *s
	plain.weaponSpec = nil
	if got, want := se.trial(st, change{mh, sword}), plain.value(swapped)-s.floorWeight*lost; math.Abs(got-want) > 1e-6*math.Abs(want) {
		t.Errorf("the swap scores %.3f, want %.3f: the gear alone less the floor's weight on the lost expertise", got, want)
	}

	rng := rand.New(rand.NewSource(5))
	var slots []proto.ItemSlot
	for slot := range se.all {
		if len(se.all[slot]) > 0 && !s.pool.Locked[slot] {
			slots = append(slots, proto.ItemSlot(slot))
		}
	}
	for step := 0; step < 400; step++ {
		slot := slots[rng.Intn(len(slots))]
		if step%2 == 0 {
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
		for _, ch := range changes {
			se.set(st, ch.slot, ch.choice)
		}
		if got, want := st.value, s.value(st.l); math.Abs(got-want) > 1e-6*max(1, math.Abs(want)) {
			t.Fatalf("step %d: the state holds %.6f, a rescore %.6f", step, got, want)
		}
		if st.racial != s.racialDelta(&st.l) {
			t.Fatalf("step %d: the state's racial term is %.3f, its loadout's %.3f", step, st.racial, s.racialDelta(&st.l))
		}
	}
}

// Pruning stays safe with the racial term: whatever the other hand holds and wherever the rest of the
// gear puts the floor, a weapon adds no more than its upper bound, and the bare weapon no less than
// its lower bound.
func TestRacialBoundsHold(t *testing.T) {
	rn, s := crSurrogate(t)
	se := newSearcher(s, rn)
	mh, oh := proto.ItemSlot_ItemSlotMainHand, proto.ItemSlot_ItemSlotOffHand
	rng := rand.New(rand.NewSource(9))
	widened := 0
	for _, slot := range []proto.ItemSlot{mh, oh} {
		other := pairedHand(slot)
		others := []ItemChoice{{}}
		for _, cand := range s.pool.Slots[other] {
			others = append(others, ItemChoice{ItemID: cand.Item.ID})
		}
		for _, o := range se.all[slot] {
			if rlb, rub := s.racialBounds(slot, &o.cand.Item); rlb != 0 || rub != 0 {
				widened++
			}
			var choices []ItemChoice
			for _, c := range kaChoices(s.pool, slot) {
				if c.ItemID == o.cand.Item.ID {
					choices = append(choices, c)
				}
			}
			for _, oc := range others {
				for k := 0; k < 10; k++ {
					l := s.seed
					l.Items[other] = oc
					l.Items[slot] = ItemChoice{}
					var rest stats.Stats
					rest[stats.Expertise] = rng.Float64()*160 - 80
					rest[stats.Strength] = rng.Float64()*200 - 100
					total := s.seedStats.Add(rest)
					empty := s.score(total, 0, nil, s.racialDelta(&l))
					for i, c := range choices {
						l.Items[slot] = c
						v := s.score(total.Add(s.choiceStats(slot, c)), s.choiceResidual(slot, c), nil, s.racialDelta(&l)) - empty
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
		t.Error("no weapon's bounds moved with the racial term")
	}
}

func pairedHand(slot proto.ItemSlot) proto.ItemSlot {
	if slot == proto.ItemSlot_ItemSlotMainHand {
		return proto.ItemSlot_ItemSlotOffHand
	}
	return proto.ItemSlot_ItemSlotMainHand
}

// raceSearchRun takes req's run through the stages before the search on the known-answer fake, with
// a floor on st at the seed's own sheet reading, and runs searchRaces over finalists. It returns the
// surrogate the search started from too.
func raceSearchRun(t *testing.T, req *proto.OptimizeGearRequest, st stats.Stat, finalists []proto.Race) (*run, *surrogate, []Loadout) {
	t.Helper()
	r, err := PrepareRequest(req)
	if err != nil {
		t.Fatal(err)
	}
	seed, err := playerSheet(r.Base, r.TargetIndex, r.Seed, stats.Stats{})
	if err != nil {
		t.Fatal(err)
	}
	r.Settings.StatMinimums = []*proto.StatMinimum{{Stat: proto.Stat(st), MinValue: seed.FinalStats[st]}}
	pool, err := CompilePool(r)
	if err != nil {
		t.Fatal(err)
	}
	rn := &run{
		ctx:    context.Background(),
		asked:  r,
		r:      r,
		budget: EffortBudget(proto.OptimizerEffort_OptimizerEffortQuick),
		rng:    rand.New(rand.NewSource(1)),
		pool:   pool,
		best:   r.Seed,
		ref:    r.Equipped,
	}
	rn.total = int64(rn.budget.Evaluations) * int64(rn.budget.Iterations)
	rn.screen = roundShards(rn.budget.Iterations / 4)
	rn.eval = &countingEvaluator{inner: newKnownEvaluator(kaMetrics), have: map[Point]int{}}
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
	if err := s.setFloorsAndCeilings(r.Settings.StatMinimums, nil, rangeStats); err != nil {
		t.Fatal(err)
	}
	if err := rn.measureEffects(s); err != nil {
		t.Fatal(err)
	}
	rn.finalists = finalists
	candidates, err := rn.searchRaces(s)
	if err != nil {
		t.Fatal(err)
	}
	return rn, s, candidates
}

// checkEveryRace wants candidates from every finalist, each meeting the run's floors under its own
// traits, and the seed's gear short of them under some other race's.
func checkEveryRace(t *testing.T, rn *run, candidates []Loadout, finalists []proto.Race) {
	t.Helper()
	byRace := map[proto.Race]int{}
	for _, l := range candidates {
		byRace[l.RacialTraits]++
		if err := rn.pool.Check(l); err != nil {
			t.Errorf("a %v candidate breaks a rule under its own traits: %v", l.RacialTraits, err)
		}
	}
	shortSomewhere := false
	for _, race := range finalists {
		if byRace[race] == 0 {
			t.Errorf("no candidates for %v (%v)", race, byRace)
		}
		seed := rn.r.Seed
		seed.RacialTraits = race
		shortSomewhere = shortSomewhere || rn.pool.Check(seed) != nil
	}
	if !shortSomewhere {
		t.Error("the seed's gear meets the floor under every finalist, so the floor tests nothing")
	}
	t.Logf("candidates by race: %v", byRace)
}

// An Orc dual-wielding axes sits right at an expertise floor its axe specialization pays for. A
// sword outdamages both axes but loses that expertise, so every race (the Orc too, once it drops an
// axe) has to make it up from gear, and every candidate the search hands verify does.
func TestEveryRaceMeetsTheExpertiseFloorUnderItsOwnTraits(t *testing.T) {
	finalists := []proto.Race{proto.Race_RaceOrc, proto.Race_RaceTroll, proto.Race_RaceHuman}
	rn, _, candidates := raceSearchRun(t, crRequest(proto.Race_RaceOrc, kaAxe, kaAxe), stats.Expertise, finalists)
	checkEveryRace(t, rn, candidates, finalists)

	swords := 0
	for _, l := range candidates {
		if l.Items[proto.ItemSlot_ItemSlotMainHand].ItemID == crBigSword {
			swords++
		}
	}
	if swords == 0 {
		t.Error("no candidate takes the better sword")
	}
	if rn.surrogates[proto.Race_RaceTroll].weaponSpec != nil {
		t.Error("a Troll has no weapon specialization to price")
	}
}

// The races search at once, so searching again from the same surrogate and random seed has to come
// back with the same candidates in the same order.
func TestRaceSearchesDontDependOnScheduling(t *testing.T) {
	finalists := []proto.Race{proto.Race_RaceOrc, proto.Race_RaceTroll, proto.Race_RaceHuman}
	rn, s, _ := raceSearchRun(t, crRequest(proto.Race_RaceOrc, kaAxe, kaAxe), stats.Expertise, finalists)
	search := func() []Loadout {
		rn.rng = rand.New(rand.NewSource(7))
		candidates, err := rn.searchRaces(s)
		if err != nil {
			t.Fatal(err)
		}
		return candidates
	}
	first := search()
	for i := 0; i < 5; i++ {
		if again := search(); !slices.Equal(first, again) {
			t.Fatalf("search %d found other candidates", i+2)
		}
	}
}

// A Draenei sits right at a spell hit floor its own Heroic Presence helps pay for. Every other
// race's candidates make that hit up from gear; the Draenei's don't have to.
func TestOtherRacesMakeUpHeroicPresenceFromGear(t *testing.T) {
	finalists := []proto.Race{proto.Race_RaceDraenei, proto.Race_RaceTroll, proto.Race_RaceHuman}
	rn, _, candidates := raceSearchRun(t, crRequest(proto.Race_RaceDraenei, kaSword, kaDagger), stats.SpellHit, finalists)
	checkEveryRace(t, rn, candidates, finalists)

	presence := core.SpellHitRatingPerHitChance
	for _, l := range candidates {
		gear := rn.pool.gearStats(l)[stats.SpellHit]
		if l.RacialTraits != proto.Race_RaceDraenei && gear < presence {
			t.Errorf("a %v candidate wears %.1f spell hit, under the %.1f Heroic Presence gave the seed", l.RacialTraits, gear, presence)
		}
	}
	seedReading := func(race proto.Race) float64 {
		floors := rn.surrogates[race].floors
		if len(floors) != 1 || floors[0].stat != stats.SpellHit {
			t.Fatalf("%v: want one spell hit floor, got %+v", race, floors)
		}
		return floors[0].seed
	}
	if gap := seedReading(proto.Race_RaceDraenei) - seedReading(proto.Race_RaceTroll); math.Abs(gap-presence) > 1e-6 {
		t.Errorf("the Draenei's surrogate reads the seed %.3f spell hit over the Troll's, want Heroic Presence's %.3f", gap, presence)
	}
}
