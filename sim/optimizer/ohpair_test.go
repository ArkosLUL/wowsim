package optimizer

import (
	"context"
	"math"
	"math/rand"
	"sync"
	"testing"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
	"github.com/wowsims/wotlk/sim/core/stats"
)

// Made-up weapons for the off-hand pairing tests. Under kaDPS a main hand adds 1.5x its average
// damage and an off hand 0.75x, plus 2 per Strength: the great sword is worth 660 alone, the small
// one 460, the sword 320 in the main hand and 170 in the off hand, the axe 240 and 120, the unique
// sword 380 and 200.
const (
	ohpGreat  = 9781001
	ohpGreat2 = 9781002
	ohpSword  = 9781003
	ohpAxe    = 9781004
	ohpUnique = 9781005 // unique-equipped
)

var ohpOnce sync.Once

func ohpRegister() {
	kaRegister()
	ohpOnce.Do(func() {
		core.AddToDatabase(&proto.SimDatabase{Items: []*proto.SimItem{
			kaWeapon(ohpGreat, proto.HandType_HandTypeTwoHand, proto.WeaponType_WeaponTypeSword, 320, 480, 3.6, stats.Stats{stats.Strength: 30}),
			kaWeapon(ohpGreat2, proto.HandType_HandTypeTwoHand, proto.WeaponType_WeaponTypeSword, 220, 340, 3.4, stats.Stats{stats.Strength: 20}),
			kaWeapon(ohpSword, proto.HandType_HandTypeOneHand, proto.WeaponType_WeaponTypeSword, 160, 240, 2.6, stats.Stats{stats.Strength: 10}),
			kaWeapon(ohpAxe, proto.HandType_HandTypeOneHand, proto.WeaponType_WeaponTypeAxe, 120, 200, 2.6, stats.Stats{}),
			kaWeapon(ohpUnique, proto.HandType_HandTypeOneHand, proto.WeaponType_WeaponTypeSword, 200, 280, 2.6, stats.Stats{stats.Strength: 10}),
		}})
	})
}

// ohpRequest is a warrior without Titan's Grip holding mh and oh, with every slot but the hands
// locked, over the given main and off hands.
func ohpRequest(mh, oh int32, mhPool, ohPool []int32) *proto.OptimizeGearRequest {
	ohpRegister()
	seed := testEquipment(map[proto.ItemSlot]*proto.ItemSpec{
		proto.ItemSlot_ItemSlotHead:     {Id: kaHeadPlain},
		proto.ItemSlot_ItemSlotShoulder: {Id: kaShoulder},
		proto.ItemSlot_ItemSlotChest:    {Id: kaChest},
		proto.ItemSlot_ItemSlotFinger1:  {Id: kaRing},
		proto.ItemSlot_ItemSlotFinger2:  {Id: kaRingHit},
		proto.ItemSlot_ItemSlotTrinket1: {Id: kaTrinketAP},
		proto.ItemSlot_ItemSlotTrinket2: {Id: kaTrinketAP2},
		proto.ItemSlot_ItemSlotMainHand: {Id: mh},
		proto.ItemSlot_ItemSlotOffHand:  {Id: oh},
	})
	settings := &proto.OptimizerSettings{ContentPhase: 1, Effort: proto.OptimizerEffort_OptimizerEffortNormal,
		RacialMode: proto.OptimizerRacialMode_OptimizerRacialKeepCurrent, DisableStatCaps: true}
	for slot := proto.ItemSlot(0); int(slot) < NumSlots; slot++ {
		if !isHand(slot) {
			settings.LockedSlots = append(settings.LockedSlots, slot)
		}
	}
	return &proto.OptimizeGearRequest{
		Base: &proto.RaidSimRequest{
			Raid: &proto.Raid{Parties: []*proto.Party{{Players: []*proto.Player{{
				Name:          "Target",
				Race:          proto.Race_RaceOrc,
				Class:         proto.Class_ClassWarrior,
				TalentsString: rtFuryNoTG,
				Equipment:     seed,
				Spec:          &proto.Player_Warrior{Warrior: &proto.Warrior{Options: &proto.Warrior_Options{}}},
				Rotation:      &proto.APLRotation{Type: proto.APLRotation_TypeAPL},
			}}}}},
			Encounter:  &proto.Encounter{Duration: 180, Targets: []*proto.Target{{}}},
			SimOptions: &proto.SimOptions{RandomSeed: 7},
		},
		Settings: settings,
		Pool: &proto.CandidatePool{
			Slots: []*proto.SlotPool{
				{Slot: proto.ItemSlot_ItemSlotMainHand, ItemIds: mhPool},
				{Slot: proto.ItemSlot_ItemSlotOffHand, ItemIds: ohPool},
			},
			CatalogItems: []*proto.CatalogItem{{Id: ohpUnique, UniqueEquipped: true}},
			CatalogDate:  "2026-09-19",
		},
	}
}

// ohpRun is req's run as verify leaves it, the seed its best, with the surrogate measured on kaDPS.
func ohpRun(t *testing.T, req *proto.OptimizeGearRequest) (*run, *surrogate) {
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
		budget: EffortBudget(proto.OptimizerEffort_OptimizerEffortNormal),
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
	resp, _, err := rn.measureCurves(rn.newStatScreen())
	if err != nil {
		t.Fatal(err)
	}
	s := newSurrogate(pool, r.Seed, resp)
	if err := rn.measureEffects(s); err != nil {
		t.Fatal(err)
	}
	return rn, s
}

func ohpMainHands(alts []*proto.OptimizerSlotAlternative) []*proto.OptimizerSlotAlternative {
	var out []*proto.OptimizerSlotAlternative
	for _, a := range alts {
		if a.Slot == proto.ItemSlot_ItemSlotMainHand {
			out = append(out, a)
		}
	}
	return out
}

// With the great sword picked, the swords in both hands (490) beat the small great sword (460),
// which beats the sword alone (320). The pair has to be what's simmed and reported.
func TestAlternativesPairAnOffHandWithATwoHandPick(t *testing.T) {
	req := ohpRequest(ohpGreat, 0, []int32{ohpGreat, ohpGreat2, ohpSword, ohpAxe}, []int32{ohpSword, ohpAxe})
	rn, s := ohpRun(t, req)
	alts, _, err := rn.neighborhood(s, nil)
	if err != nil {
		t.Fatal(err)
	}
	if mh, oh := rn.best.Items[proto.ItemSlot_ItemSlotMainHand].ItemID, rn.best.Items[proto.ItemSlot_ItemSlotOffHand].ItemID; mh != ohpGreat || oh != 0 {
		t.Fatalf("best holds %d and %d, want %d alone", mh, oh, ohpGreat)
	}
	want := []struct {
		mh, oh int32
		delta  float64
	}{
		{ohpSword, ohpSword, -170},
		{ohpGreat2, 0, -200},
		{ohpAxe, ohpSword, -250},
	}
	got := ohpMainHands(alts)
	if len(got) != len(want) {
		t.Fatalf("%d main-hand alternatives, want %d: %v", len(got), len(want), got)
	}
	for i, w := range want {
		a := got[i]
		if a.Item.GetId() != w.mh || a.OffHand.GetId() != w.oh {
			t.Errorf("alternative %d is %d with off hand %d, want %d with %d", i, a.Item.GetId(), a.OffHand.GetId(), w.mh, w.oh)
		}
		if math.Abs(a.ScoreDelta-w.delta) > 5 {
			t.Errorf("alternative %d (%d) scores %+.1f ± %.1f, want about %+.0f", i, a.Item.GetId(), a.ScoreDelta, a.ScoreDeltaSe, w.delta)
		}
	}
	for _, a := range alts {
		if a.Slot != proto.ItemSlot_ItemSlotMainHand && a.OffHand != nil {
			t.Errorf("%s alternative %d carries off hand %d", a.Slot, a.Item.GetId(), a.OffHand.GetId())
		}
	}
}

// With the swords in both hands picked, a main hand moves alone: the axe next to the kept sword
// (410), the small great sword with the off hand emptied (460). Neither reports an off hand.
func TestAlternativesOfADualWieldPickKeepItsOffHand(t *testing.T) {
	req := ohpRequest(ohpSword, ohpSword, []int32{ohpGreat2, ohpSword, ohpAxe}, []int32{ohpSword, ohpAxe})
	rn, s := ohpRun(t, req)

	se := newSearcher(s, rn)
	st := se.newState(rn.best)
	for _, changes := range se.runnersUp(st, proto.ItemSlot_ItemSlotMainHand, alternativesPerSlot, se.all[proto.ItemSlot_ItemSlotMainHand]) {
		for _, ch := range changes[1:] {
			if ch.choice.ItemID != 0 {
				t.Errorf("main hand %d brings off hand %d", changes[0].choice.ItemID, ch.choice.ItemID)
			}
		}
	}

	alts, _, err := rn.neighborhood(s, nil)
	if err != nil {
		t.Fatal(err)
	}
	if mh, oh := rn.best.Items[proto.ItemSlot_ItemSlotMainHand].ItemID, rn.best.Items[proto.ItemSlot_ItemSlotOffHand].ItemID; mh != ohpSword || oh != ohpSword {
		t.Fatalf("best holds %d and %d, want %d in both hands", mh, oh, ohpSword)
	}
	want := map[int32]float64{ohpGreat2: -30, ohpAxe: -80}
	got := ohpMainHands(alts)
	if len(got) != len(want) {
		t.Fatalf("%d main-hand alternatives, want %d: %v", len(got), len(want), got)
	}
	for _, a := range got {
		if a.OffHand != nil {
			t.Errorf("main hand %d reports off hand %d", a.Item.GetId(), a.OffHand.GetId())
		}
		if d, ok := want[a.Item.GetId()]; !ok || math.Abs(a.ScoreDelta-d) > 5 {
			t.Errorf("main hand %d scores %+.1f ± %.1f, want about %+.0f", a.Item.GetId(), a.ScoreDelta, a.ScoreDeltaSe, d)
		}
	}
}

// The unique sword's best partner by score is itself, which the equip rules forbid: it keeps its
// place with the best off hand that fits, the plain sword, while the plain sword's own partner is
// the unique one. The search's forced runners-up take the same moves from the pruned options.
func TestRunnersUpPairTheBestLegalOffHand(t *testing.T) {
	req := ohpRequest(ohpGreat, 0, []int32{ohpGreat, ohpGreat2, ohpSword, ohpUnique}, []int32{ohpSword, ohpUnique})
	rn, s := ohpRun(t, req)
	se := newSearcher(s, rn)
	st := se.newState(rn.best)
	mh, oh := proto.ItemSlot_ItemSlotMainHand, proto.ItemSlot_ItemSlotOffHand

	want := map[int32]int32{ohpUnique: ohpSword, ohpSword: ohpUnique, ohpGreat2: 0}
	got := se.runnersUp(st, mh, forcedRunnersUp, se.opts[mh])
	if len(got) != len(want) {
		t.Fatalf("%d runners-up, want %d: %v", len(got), len(want), got)
	}
	for _, changes := range got {
		l := st.l
		for _, ch := range changes {
			l.Items[ch.slot] = ch.choice
		}
		id := changes[0].choice.ItemID
		if w, ok := want[id]; !ok || l.Items[oh].ItemID != w {
			t.Errorf("main hand %d comes with off hand %d, want %d", id, l.Items[oh].ItemID, w)
		}
		if err := se.check(l); err != nil {
			t.Errorf("main hand %d with off hand %d breaks a rule: %v", id, l.Items[oh].ItemID, err)
		}
	}
}

// With the unique sword the only off hand, its pair with itself (580) is illegal and it falls back to
// playing alone (380): it has to rank there, under the sword beside it (520) and the small great
// sword (460), not first at the pair's 580.
func TestRunnersUpRankAFallbackAtItsLegalScore(t *testing.T) {
	req := ohpRequest(ohpGreat, 0, []int32{ohpGreat, ohpGreat2, ohpSword, ohpUnique}, []int32{ohpUnique})
	rn, s := ohpRun(t, req)
	se := newSearcher(s, rn)
	st := se.newState(rn.best)
	mh, oh := proto.ItemSlot_ItemSlotMainHand, proto.ItemSlot_ItemSlotOffHand

	got := se.runnersUp(st, mh, forcedRunnersUp, se.all[mh])
	want := []struct{ mh, oh int32 }{{ohpSword, ohpUnique}, {ohpGreat2, 0}, {ohpUnique, 0}}
	if len(got) != len(want) {
		t.Fatalf("%d runners-up, want %d: %v", len(got), len(want), got)
	}
	for i, changes := range got {
		l := st.l
		for _, ch := range changes {
			l.Items[ch.slot] = ch.choice
		}
		if w := want[i]; l.Items[mh].ItemID != w.mh || l.Items[oh].ItemID != w.oh {
			t.Errorf("runner-up %d holds %d and %d, want %d and %d", i, l.Items[mh].ItemID, l.Items[oh].ItemID, w.mh, w.oh)
		}
	}
}

// On the round cap the great sword replaces the swords in both hands, and the axe, simmed beside the
// kept sword, has to report that sword: the new best's off hand is empty.
func TestAlternativesReportTheOffHandAnAdoptedTwoHanderDropped(t *testing.T) {
	old := neighborhoodRounds
	neighborhoodRounds = 1
	defer func() { neighborhoodRounds = old }()

	req := ohpRequest(ohpSword, ohpSword, []int32{ohpGreat, ohpSword, ohpAxe}, []int32{ohpSword, ohpAxe})
	rn, s := ohpRun(t, req)
	alts, _, err := rn.neighborhood(s, nil)
	if err != nil {
		t.Fatal(err)
	}
	if mh, oh := rn.best.Items[proto.ItemSlot_ItemSlotMainHand].ItemID, rn.best.Items[proto.ItemSlot_ItemSlotOffHand].ItemID; mh != ohpGreat || oh != 0 {
		t.Fatalf("best holds %d and %d, want %d alone", mh, oh, ohpGreat)
	}
	got := ohpMainHands(alts)
	if len(got) != 1 {
		t.Fatalf("%d main-hand alternatives, want 1: %v", len(got), got)
	}
	if a := got[0]; a.Item.GetId() != ohpAxe || a.OffHand.GetId() != ohpSword || math.Abs(a.ScoreDelta+250) > 5 {
		t.Errorf("main hand %d with off hand %d scores %+.1f ± %.1f, want %d with %d at about -250", a.Item.GetId(), a.OffHand.GetId(), a.ScoreDelta, a.ScoreDeltaSe, ohpAxe, ohpSword)
	}
}
