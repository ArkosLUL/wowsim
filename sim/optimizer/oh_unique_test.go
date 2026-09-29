package optimizer

import (
	"context"
	"math/rand"
	"sync"
	"testing"
	"time"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
	"github.com/wowsims/wotlk/sim/core/stats"
)

// Made-up weapons for the off-hand-unique known-answer tests: a unique-equipped one they should
// move out of the main hand, a plain upgrade that outranks it there, and a weak filler it should
// still beat in the off hand. The TG pair repeats the shape for a two-hander under Titan's Grip.
const (
	kohUnique = 9710001 // unique-equipped one-hander
	kohBetter = 9710002 // outranks it for the main hand
	kohWeak   = 9710003 // a plain off-hand filler it should outrank there

	kohUniqueTG = 9710011 // unique-equipped two-hander
	kohBetterTG = 9710012 // outranks it for the main hand under Titan's Grip
)

var kohDatabase = &proto.SimDatabase{
	Items: []*proto.SimItem{
		kaWeapon(kohUnique, proto.HandType_HandTypeOneHand, proto.WeaponType_WeaponTypeSword, 200, 300, 2.4, stats.Stats{stats.Strength: 20}),
		kaWeapon(kohBetter, proto.HandType_HandTypeOneHand, proto.WeaponType_WeaponTypeSword, 260, 380, 2.2, stats.Stats{stats.Strength: 30}),
		kaWeapon(kohWeak, proto.HandType_HandTypeOneHand, proto.WeaponType_WeaponTypeSword, 80, 120, 2.6, stats.Stats{stats.Strength: 5}),
		kaWeapon(kohUniqueTG, proto.HandType_HandTypeTwoHand, proto.WeaponType_WeaponTypeSword, 400, 600, 3.4, stats.Stats{stats.Strength: 40}),
		kaWeapon(kohBetterTG, proto.HandType_HandTypeTwoHand, proto.WeaponType_WeaponTypeSword, 520, 760, 3.2, stats.Stats{stats.Strength: 55}),
	},
}

var kohOnce sync.Once

// kohRegister adds the off-hand-unique fixture's weapons to core, on top of the known-answer
// suite's own armor, rings and trinkets, which kohRequest and kohRequestTG both reuse.
func kohRegister() {
	kaRegister()
	kohOnce.Do(func() { core.AddToDatabase(kohDatabase) })
}

// kohArmor is the non-weapon gear kohRequest and kohRequestTG share: the known-answer suite's head,
// shoulder, chest, rings and trinkets, unchanged, so only the weapon slots are under test.
func kohArmor(mh, oh int32) (*proto.EquipmentSpec, []*proto.SlotPool) {
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
	rings := []int32{kaRing, kaRingHit}
	trinkets := []int32{kaTrinketAP, kaTrinketAP2}
	slots := []*proto.SlotPool{
		{Slot: proto.ItemSlot_ItemSlotHead, ItemIds: []int32{kaHeadPlain}},
		{Slot: proto.ItemSlot_ItemSlotShoulder, ItemIds: []int32{kaShoulder}},
		{Slot: proto.ItemSlot_ItemSlotChest, ItemIds: []int32{kaChest}},
		{Slot: proto.ItemSlot_ItemSlotFinger1, ItemIds: rings},
		{Slot: proto.ItemSlot_ItemSlotFinger2, ItemIds: rings},
		{Slot: proto.ItemSlot_ItemSlotTrinket1, ItemIds: trinkets},
		{Slot: proto.ItemSlot_ItemSlotTrinket2, ItemIds: trinkets},
	}
	return seed, slots
}

// kohRequest is a dual-wielding Fury Warrior without Titan's Grip, over a pool of kohUnique,
// kohBetter and kohWeak: kohBetter only fits the main hand, so the off hand's real choice is
// between kohUnique and kohWeak. mh and oh seed the weapon slots.
func kohRequest(effort proto.OptimizerEffort, mh, oh int32) *proto.OptimizeGearRequest {
	kohRegister()
	seed, slots := kohArmor(mh, oh)
	target := &proto.Player{
		Name:          "Target",
		Race:          proto.Race_RaceOrc,
		Class:         proto.Class_ClassWarrior,
		TalentsString: rtFuryNoTG,
		Equipment:     seed,
		Spec:          &proto.Player_Warrior{Warrior: &proto.Warrior{Options: &proto.Warrior_Options{}}},
		Rotation:      &proto.APLRotation{Type: proto.APLRotation_TypeAPL},
	}
	slots = append(slots,
		&proto.SlotPool{Slot: proto.ItemSlot_ItemSlotMainHand, ItemIds: []int32{kohUnique, kohBetter, kohWeak}},
		&proto.SlotPool{Slot: proto.ItemSlot_ItemSlotOffHand, ItemIds: []int32{kohUnique, kohWeak}},
	)
	return &proto.OptimizeGearRequest{
		Base: &proto.RaidSimRequest{
			Raid:       &proto.Raid{Parties: []*proto.Party{{Players: []*proto.Player{target}}}},
			Encounter:  &proto.Encounter{Duration: 180, Targets: []*proto.Target{{}}},
			SimOptions: &proto.SimOptions{RandomSeed: 7},
		},
		Settings: &proto.OptimizerSettings{ContentPhase: 1, Effort: effort, RacialMode: proto.OptimizerRacialMode_OptimizerRacialKeepCurrent},
		Pool: &proto.CandidatePool{
			Slots:        slots,
			CatalogItems: []*proto.CatalogItem{{Id: kohUnique, UniqueEquipped: true}},
			CatalogDate:  "2026-09-19",
		},
	}
}

// kohRequestTG is kohRequest's Titan's Grip case: a two-handed kohUniqueTG and kohBetterTG, both
// legal in either hand under Titan's Grip.
func kohRequestTG(effort proto.OptimizerEffort, mh, oh int32) *proto.OptimizeGearRequest {
	kohRegister()
	seed, slots := kohArmor(mh, oh)
	target := &proto.Player{
		Name:          "Target",
		Race:          proto.Race_RaceOrc,
		Class:         proto.Class_ClassWarrior,
		TalentsString: rtFuryTG,
		Equipment:     seed,
		Spec:          &proto.Player_Warrior{Warrior: &proto.Warrior{Options: &proto.Warrior_Options{}}},
		Rotation:      &proto.APLRotation{Type: proto.APLRotation_TypeAPL},
	}
	slots = append(slots,
		&proto.SlotPool{Slot: proto.ItemSlot_ItemSlotMainHand, ItemIds: []int32{kohUniqueTG, kohBetterTG}},
		&proto.SlotPool{Slot: proto.ItemSlot_ItemSlotOffHand, ItemIds: []int32{kohUniqueTG}},
	)
	return &proto.OptimizeGearRequest{
		Base: &proto.RaidSimRequest{
			Raid:       &proto.Raid{Parties: []*proto.Party{{Players: []*proto.Player{target}}}},
			Encounter:  &proto.Encounter{Duration: 180, Targets: []*proto.Target{{}}},
			SimOptions: &proto.SimOptions{RandomSeed: 7},
		},
		Settings: &proto.OptimizerSettings{ContentPhase: 1, Effort: effort, RacialMode: proto.OptimizerRacialMode_OptimizerRacialKeepCurrent},
		Pool: &proto.CandidatePool{
			Slots:        slots,
			CatalogItems: []*proto.CatalogItem{{Id: kohUniqueTG, UniqueEquipped: true}},
			CatalogDate:  "2026-09-19",
		},
	}
}

// A live batch moved an Assassination rogue's unique main hand to a faster dagger and put a weaker
// one in the off hand, with the unique item in neither slot's alternatives (sim d2b022d5b). kohUnique
// stands in for it here: outranked in the main hand by kohBetter, it should end up in the off hand,
// ahead of kohWeak.
func TestKnownAnswerOffHandUniqueMainHand(t *testing.T) {
	req := kohRequest(proto.OptimizerEffort_OptimizerEffortQuick, kohUnique, kohWeak)
	r, err := PrepareRequest(req)
	if err != nil {
		t.Fatal(err)
	}
	pool, err := CompilePool(r)
	if err != nil {
		t.Fatal(err)
	}
	want, wantDPS := bruteForce(t, pool)

	result := optimize(context.Background(), r, r, newKnownEvaluator(kaMetrics), nil, time.Now())
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
	mh, oh := got.Items[proto.ItemSlot_ItemSlotMainHand].ItemID, got.Items[proto.ItemSlot_ItemSlotOffHand].ItemID
	if oh != kohUnique {
		t.Errorf("off hand picked %d, want %d (outranked in the main hand by %d, still better than %d in the off hand)", oh, kohUnique, kohBetter, kohWeak)
	}
	if mh != kohBetter {
		t.Errorf("main hand picked %d, want %d", mh, kohBetter)
	}
}

// The same case under Titan's Grip, with a unique two-hander (kohUniqueTG, standing in for one like
// Shadowmourne) taking the path a one-hander does: outranked in the main hand, it should still land
// in the off hand rather than being dropped from the run.
func TestKnownAnswerOffHandUniqueTwoHandUnderTitansGrip(t *testing.T) {
	req := kohRequestTG(proto.OptimizerEffort_OptimizerEffortQuick, kohUniqueTG, 0)
	r, err := PrepareRequest(req)
	if err != nil {
		t.Fatal(err)
	}
	pool, err := CompilePool(r)
	if err != nil {
		t.Fatal(err)
	}
	want, wantDPS := bruteForce(t, pool)

	result := optimize(context.Background(), r, r, newKnownEvaluator(kaMetrics), nil, time.Now())
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
	mh, oh := got.Items[proto.ItemSlot_ItemSlotMainHand].ItemID, got.Items[proto.ItemSlot_ItemSlotOffHand].ItemID
	if oh != kohUniqueTG {
		t.Errorf("off hand picked %d, want %d", oh, kohUniqueTG)
	}
	if mh != kohBetterTG {
		t.Errorf("main hand picked %d, want %d", mh, kohBetterTG)
	}
}

// Pins the residual pricing behind both known-answer tests: the seed's unique weapon has to price
// for the opposite hand, and the mirror direction (seeded in the off hand, pricing for the main
// hand) needs the same check, since it goes through a different code path in effectFamilies.
func TestOffHandPricesTheSeedsUniqueWeaponForTheOtherHand(t *testing.T) {
	cases := []struct {
		name           string
		seedMH, seedOH int32
		priceFor       proto.ItemSlot
	}{
		{"seeded in the main hand", kohUnique, kohWeak, proto.ItemSlot_ItemSlotOffHand},
		{"seeded in the off hand (mirror)", kohWeak, kohUnique, proto.ItemSlot_ItemSlotMainHand},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			req := kohRequest(proto.OptimizerEffort_OptimizerEffortQuick, c.seedMH, c.seedOH)
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

			resp, _, err := rn.measureCurves(rn.newStatScreen())
			if err != nil {
				t.Fatal(err)
			}
			s := newSurrogate(pool, r.Seed, resp)
			if err := rn.measureEffects(s); err != nil {
				t.Fatal(err)
			}

			if s.unpriced[c.priceFor][kohUnique] {
				t.Fatalf("item %d never priced for %s: candBounds drops it from the whole run", kohUnique, c.priceFor)
			}
			est, measured := s.items[c.priceFor][kohUnique]
			if !measured {
				t.Fatalf("item %d has no residual recorded for %s", kohUnique, c.priceFor)
			}
			if est.Mean <= 0 {
				t.Errorf("item %d priced at %+.1f for %s, want a real gain over an empty slot", kohUnique, est.Mean, c.priceFor)
			}
		})
	}
}
