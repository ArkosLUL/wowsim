package optimizer

import (
	"context"
	"math/rand"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
	"github.com/wowsims/wotlk/sim/core/stats"
)

// bonusActive says whether every own socket of l's item in slot holds a gem of a matching color.
func bonusActive(l Loadout, slot proto.ItemSlot) bool {
	item := core.NewItem(l.Items[slot].CoreSpec(), nil)
	if len(item.GemSockets) == 0 || len(item.Gems) < len(item.GemSockets) {
		return false
	}
	for i, color := range item.GemSockets {
		if !core.ColorIntersects(color, item.Gems[i].Color) {
			return false
		}
	}
	return true
}

// Each twin takes one gem out and regems only the slots that held it, with its runners-up: the meta
// still gets its two blue gems and each socket bonus holds where it's worth more than the gem it
// costs.
func TestGemTwinsKeepTheBonusAndMeta(t *testing.T) {
	const head, legs = proto.ItemSlot_ItemSlotHead, proto.ItemSlot_ItemSlotLegs
	p := rtPool(t, nil)
	pick := rtLoadout()
	pick.Items[head].Gems = rtGems(rtMeta2Blue, rtRed, rtBlue)
	pick.Items[legs] = ItemChoice{ItemID: rtLegs, Gems: rtGems(rtRed, rtYellow, rtBlue)}
	if err := p.CheckGear(pick); err != nil {
		t.Fatalf("the fixture's pick breaks a rule: %v", err)
	}
	value := linearValue(stats.Stats{stats.Stamina: 1, stats.Strength: 0.5})

	twins := p.gemTwins(pick, value, nil)
	if len(twins) != 4 {
		t.Fatalf("%d twins, want one for each of red, blue, yellow and the waist's prismatic", len(twins))
	}
	for _, twin := range twins {
		if err := p.CheckGear(twin); err != nil {
			t.Errorf("twin %v breaks a rule: %v", twin.Items, err)
		}
		if twin.Items[head].Gems[0] != rtMeta2Blue {
			t.Errorf("twin %v changed the meta", twin.Items[head].Gems)
		}
		for slot := range twin.Items {
			if twin.Items[slot].ItemID != pick.Items[slot].ItemID {
				t.Errorf("twin %v changed an item", twin.Items)
			}
		}
	}
	// without the blue gem, purple and green keep both socket bonuses and the meta's two blues
	noBlue := twins[1]
	if countGems(noBlue, func(id int32) bool { return id == rtBlue }) != 0 {
		t.Fatalf("the second twin %v still holds the blue gem", noBlue.Items)
	}
	if blue := countGems(noBlue, func(id int32) bool { return gemColors(lookupColor(id))[2] == 1 }); blue < 2 {
		t.Errorf("%d blue-counting gems without the blue one, the meta needs 2", blue)
	}
	if !bonusActive(noBlue, head) || !bonusActive(noBlue, legs) {
		t.Errorf("twin %v lost a socket bonus it could keep", noBlue.Items)
	}
}

// A twin repeats no gem the run bans, and a locked slot keeps its gems.
func TestGemTwinsSkipBannedGemsAndLockedSlots(t *testing.T) {
	const head, legs = proto.ItemSlot_ItemSlotHead, proto.ItemSlot_ItemSlotLegs
	p := rtPool(t, func(req *proto.OptimizeGearRequest) {
		rtTarget(req).Equipment.Items[legs] = &proto.ItemSpec{Id: rtLegs, Gems: []int32{rtRed, rtYellow, rtBlue}}
		req.Settings.LockedSlots = []proto.ItemSlot{legs}
	})
	pick := rtLoadout()
	pick.Items[legs] = ItemChoice{ItemID: rtLegs, Gems: rtGems(rtRed, rtYellow, rtBlue)}
	value := linearValue(stats.Stats{stats.Stamina: 1, stats.Strength: 0.5})
	twins := p.gemTwins(pick, value, map[int32]bool{rtPurple: true})
	if len(twins) == 0 {
		t.Fatal("no twins")
	}
	for _, twin := range twins {
		if twin.Items[legs] != pick.Items[legs] {
			t.Errorf("twin %v regemmed the locked legs", twin.Items[legs])
		}
		if n := countGems(twin, func(id int32) bool { return id == rtPurple }); n != 0 {
			t.Errorf("twin %v holds the banned purple gem", twin.Items[head])
		}
	}
}

// gemCheckRun is a run over the known-answer pool whose pick wears the meta head with a red gem and
// the chest with a yellow one. The surrogate prices Strength at 2 and Hit at 3 a point, so taking
// the red gem out puts a yellow one in the head and taking the yellow one out a red one in the chest.
func gemCheckRun(t *testing.T, r *Request, eval Evaluator) (*run, Loadout) {
	t.Helper()
	pool, err := CompilePool(r)
	if err != nil {
		t.Fatal(err)
	}
	s := newSurrogate(pool, r.Seed, Response{
		stats.Strength: {Stat: stats.Strength, SlopeBelow: 2, SlopeAbove: 2},
		stats.MeleeHit: {Stat: stats.MeleeHit, SlopeBelow: 3, SlopeAbove: 3},
	})
	run := &run{ctx: context.Background(), asked: r, r: r, budget: EffortBudget(proto.OptimizerEffort_OptimizerEffortQuick), pool: pool,
		surrogates: map[proto.Race]*surrogate{r.Seed.RacialTraits: s}}
	run.eval = &countingEvaluator{inner: eval, have: map[Point]int{}}
	obj, err := NewObjective(context.Background(), run.eval, r, run.budget.Iterations)
	if err != nil {
		t.Fatal(err)
	}
	run.obj = obj
	run.seedEval = evaluate(t, run.eval, run.budget.Iterations, Point{Loadout: r.Seed})[0]
	run.seedJ = obj.Score(run.seedEval)

	pick := r.Seed
	pick.Items[proto.ItemSlot_ItemSlotHead] = ItemChoice{ItemID: kaHeadMeta, Gems: rtGems(kaMetaPlain, kaRed)}
	pick.Items[proto.ItemSlot_ItemSlotChest] = ItemChoice{ItemID: kaChest, Gems: rtGems(kaYellow)}
	if err := pool.Check(pick); err != nil {
		t.Fatal(err)
	}
	return run, pick
}

// A twin the sim rates above the pick takes its place, it's named, and its sims count.
func TestGemCheckTakesTheBestMean(t *testing.T) {
	r, err := PrepareRequest(kaRequest(proto.OptimizerEffort_OptimizerEffortQuick))
	if err != nil {
		t.Fatal(err)
	}
	// the sim likes a second yellow gem far more than the slopes do
	fake := newKnownEvaluator(func(p Point) (Metrics, error) {
		m, err := kaMetrics(p)
		m[MetricDPS] += 60 * float64(countGems(p.Loadout, func(id int32) bool { return id == kaYellow }))
		return m, err
	})
	run, pick := gemCheckRun(t, r, fake)
	verified := []*verifiedLoadout{{pick, evaluate(t, run.eval, run.budget.Iterations, Point{Loadout: pick})[0]}}
	before := run.eval.simmed()

	verified, err = run.checkGems(verified)
	if err != nil {
		t.Fatal(err)
	}
	if len(verified) != 3 {
		t.Fatalf("%d verified, want the pick and its two twins", len(verified))
	}
	if got := verified[0].loadout.Items[proto.ItemSlot_ItemSlotHead].Gems; got != rtGems(kaMetaPlain, kaYellow) {
		t.Errorf("head gems %v, want the yellow gem in place of the red one", got)
	}
	// a socket each of Strength and Hit, then the two twins
	if got := run.eval.simmed() - before; got != 4*int64(run.budget.Iterations) {
		t.Errorf("counted %d iterations, want two sockets' and two twins' %d each", got, run.budget.Iterations)
	}
	warnings := run.allWarnings()
	if len(warnings) != 1 || !strings.Contains(warnings[0], "swapping 1 gem 9700101 for 1 gem 9700102") || !strings.Contains(warnings[0], "best of 2") {
		t.Errorf("warnings %q, want one naming the swap", warnings)
	}
}

// With a Hit ceiling at the pick's own Hit, the twin with a second yellow gem goes over it, so it
// can't take the pick's place however well it sims.
func TestGemCheckKeepsTheCeilings(t *testing.T) {
	r, err := PrepareRequest(kaRequest(proto.OptimizerEffort_OptimizerEffortQuick))
	if err != nil {
		t.Fatal(err)
	}
	fake := newKnownEvaluator(func(p Point) (Metrics, error) {
		m, err := kaMetrics(p)
		m[MetricDPS] += 60 * float64(countGems(p.Loadout, func(id int32) bool { return id == kaYellow }))
		return m, err
	})
	run, pick := gemCheckRun(t, r, fake)
	hit := sheetOf(t, run.pool, pick)[stats.MeleeHit]
	r.Settings.StatMaximums = []*proto.StatMaximum{{Stat: proto.Stat_StatMeleeHit, MaxValue: hit}}
	run, pick = gemCheckRun(t, r, fake)

	verified := []*verifiedLoadout{{pick, evaluate(t, run.eval, run.budget.Iterations, Point{Loadout: pick})[0]}}
	verified, err = run.checkGems(verified)
	if err != nil {
		t.Fatal(err)
	}
	if verified[0].loadout != pick || len(run.allWarnings()) != 0 {
		t.Errorf("head %v took the pick's place (%q), want the pick kept under its Hit ceiling of %.0f",
			verified[0].loadout.Items[proto.ItemSlot_ItemSlotHead].Gems, run.allWarnings(), hit)
	}
	for _, v := range verified {
		if got := sheetOf(t, run.pool, v.loadout)[stats.MeleeHit]; got > hit {
			t.Errorf("twin %v has %.0f Hit, over the ceiling of %.0f", v.loadout.Items[proto.ItemSlot_ItemSlotHead].Gems, got, hit)
		}
	}
}

// Without a twin that beats it, the pick stays and nothing's said.
func TestGemCheckKeepsAPickNoTwinBeats(t *testing.T) {
	r, err := PrepareRequest(kaRequest(proto.OptimizerEffort_OptimizerEffortQuick))
	if err != nil {
		t.Fatal(err)
	}
	run, pick := gemCheckRun(t, r, newKnownEvaluator(kaMetrics))
	verified := []*verifiedLoadout{{pick, evaluate(t, run.eval, run.budget.Iterations, Point{Loadout: pick})[0]}}
	verified, err = run.checkGems(verified)
	if err != nil {
		t.Fatal(err)
	}
	if verified[0].loadout != pick || len(run.allWarnings()) != 0 {
		t.Errorf("the pick moved (%v) or a warning came (%q)", verified[0].loadout.Items[proto.ItemSlot_ItemSlotHead], run.allWarnings())
	}
}

// knownLivesEvaluator is a knownEvaluator whose time-to-live fight is a livesEvaluator.
type knownLivesEvaluator struct {
	*knownEvaluator
	lives      func(Point) float64
	livesCalls int
}

func (f *knownLivesEvaluator) timeToLiveEvaluator(*Request) Evaluator {
	return livesEvaluator{lives: f.lives, calls: &f.livesCalls}
}

// A tank's twins that tie the pick on score go to time to live with it, and the longest-lived stands.
func TestGemTwinsTiedOnScoreGoToTimeToLive(t *testing.T) {
	req := kaRequest(proto.OptimizerEffort_OptimizerEffortQuick)
	req.Settings.MetricWeights = &proto.OptimizerMetrics{Tps: 0.3, Dtps: 0.7}
	req.Base.Raid.Parties[0].Players[0].HealingModel = &proto.HealingModel{CadenceSeconds: 2.5, BurstWindow: 6}
	r, err := PrepareRequest(req)
	if err != nil {
		t.Fatal(err)
	}
	var pick Loadout
	fake := &knownLivesEvaluator{knownEvaluator: newKnownEvaluator(func(p Point) (Metrics, error) {
		m := Metrics{MetricTPS: 3000 + p.Offset[stats.AttackPower], MetricDTPS: 1000 - 0.05*p.Offset[stats.Armor]}
		if p.Loadout.Items[proto.ItemSlot_ItemSlotHead].ItemID == kaHeadMeta {
			m[MetricDTPS] -= 100
		}
		// a twin takes a hair more damage than the pick, well inside the noise
		if p.Loadout != pick && p.Loadout.Items[proto.ItemSlot_ItemSlotHead].ItemID == kaHeadMeta {
			m[MetricDTPS] += 0.05
		}
		return m, nil
	})}
	fake.own = 5
	run, picked := gemCheckRun(t, r, fake)
	pick = picked
	verified := []*verifiedLoadout{{pick, evaluate(t, run.eval, run.budget.Iterations, Point{Loadout: pick})[0]}}
	verified, err = run.checkGems(verified)
	if err != nil {
		t.Fatal(err)
	}
	if len(verified) != 3 {
		t.Fatalf("%d verified, want the pick and two twins", len(verified))
	}
	twin := verified[1].loadout
	if verified[0].loadout != pick {
		twin = verified[0].loadout
	}
	fake.lives = func(p Point) float64 {
		if p.Loadout == twin {
			return 50
		}
		return 20
	}
	run.breakTie(verified)
	if fake.livesCalls != 1 || verified[0].loadout != twin {
		t.Errorf("time to live ran %d times and %v went first; want once, the longer-lived twin first", fake.livesCalls, verified[0].loadout.Items[proto.ItemSlot_ItemSlotHead])
	}
	if w := run.allWarnings(); len(w) == 0 || !strings.Contains(w[len(w)-1], "3 loadouts tied") {
		t.Errorf("warnings %q, want the tie of the pick and both twins named", w)
	}
}

// Made-up gear for the stat check: a chest with red and blue sockets whose bonus is Stamina, a bracer
// with a yellow socket, a trinket with two red sockets, a sword with a red one, and Stamina and hit
// gems, clear of the other tests' ids.
const (
	gsChest   = 9700601
	gsWrist   = 9700602
	gsTrinket = 9700603
	gsSword   = 9700604
	gsBlue    = 9700611
	gsPurple  = 9700612
	gsHit     = 9700613
	// kaRed's stats and color under another id
	gsRedTwin = 9700614
	gsCrit    = 9700615
	// 20 Stamina, needs 2 blue gems
	gsMetaSta = 9700621
)

var gsOnce sync.Once

func gsRegister() {
	kaRegister()
	gsOnce.Do(func() {
		core.AddToDatabase(&proto.SimDatabase{
			Items: []*proto.SimItem{
				{Id: gsChest, Type: proto.ItemType_ItemTypeChest, Stats: stats.Stats{stats.Strength: 45}.ToFloatArray(),
					GemSockets: []proto.GemColor{proto.GemColor_GemColorRed, proto.GemColor_GemColorBlue}, SocketBonus: stats.Stats{stats.Stamina: 8}.ToFloatArray()},
				{Id: gsWrist, Type: proto.ItemType_ItemTypeWrist, Stats: stats.Stats{stats.Strength: 30}.ToFloatArray(),
					GemSockets: []proto.GemColor{proto.GemColor_GemColorYellow}, SocketBonus: stats.Stats{stats.Strength: 4}.ToFloatArray()},
				kaItem(gsTrinket, proto.ItemType_ItemTypeTrinket, stats.Stats{stats.Strength: 40}, "", proto.GemColor_GemColorRed, proto.GemColor_GemColorRed),
				{Id: gsSword, Type: proto.ItemType_ItemTypeWeapon, HandType: proto.HandType_HandTypeOneHand, WeaponType: proto.WeaponType_WeaponTypeSword,
					WeaponDamageMin: 120, WeaponDamageMax: 220, WeaponSpeed: 2.6, GemSockets: []proto.GemColor{proto.GemColor_GemColorRed},
					SocketBonus: stats.Stats{stats.Strength: 4}.ToFloatArray()},
			},
			Gems: []*proto.SimGem{
				{Id: gsRedTwin, Color: proto.GemColor_GemColorRed, Stats: stats.Stats{stats.Strength: 10}.ToFloatArray()},
				{Id: gsCrit, Color: proto.GemColor_GemColorYellow, Stats: stats.Stats{stats.MeleeCrit: 10}.ToFloatArray()},
				{Id: gsBlue, Color: proto.GemColor_GemColorBlue, Stats: stats.Stats{stats.Stamina: 15}.ToFloatArray()},
				{Id: gsPurple, Color: proto.GemColor_GemColorPurple, Stats: stats.Stats{stats.Strength: 4, stats.Stamina: 8}.ToFloatArray()},
				{Id: gsHit, Color: proto.GemColor_GemColorYellow, Stats: stats.Stats{stats.MeleeHit: 5, stats.SpellHit: 5}.ToFloatArray()},
				{Id: gsMetaSta, Color: proto.GemColor_GemColorMeta, Stats: stats.Stats{stats.Stamina: 20}.ToFloatArray()},
			},
		})
	})
}

// gsDPS is the known-answer fake with Stamina worth 4 DPS a point, on gear and offsets alike.
func gsDPS(p Point) (Metrics, error) {
	m, err := kaMetrics(p)
	m[MetricDPS] += 4 * (gearStats(p.Loadout)[stats.Stamina] + p.Offset[stats.Stamina])
	return m, err
}

// gsSlopes price Strength and Hit as the fake does, and Stamina at an eighth of its 4 a point.
var gsSlopes = stats.Stats{stats.Strength: 2, stats.MeleeHit: 3, stats.Stamina: 0.5}

// gemStatRun is a run over the known-answer pool plus gsChest, gsWrist and their gems, whose
// surrogate's curves have the given slopes. Its pick is what the gem DP makes of the meta head, the
// chest and the bracer at gsSlopes, or any multiple of them: red gems in the head and chest (the
// chest's Stamina bonus isn't worth a blue gem) and the hit gem in the bracer. gsRedTwin sits in the
// pool after kaRed. hitFloor puts a Hit floor right at the pick's; withMeta adds gsMetaSta to the pool.
func gemStatRun(t *testing.T, hitFloor bool, slopes stats.Stats, withMeta bool) (*run, *surrogate, Loadout) {
	t.Helper()
	gsRegister()
	req := kaRequest(proto.OptimizerEffort_OptimizerEffortQuick)
	for _, sp := range req.Pool.Slots {
		if sp.Slot == proto.ItemSlot_ItemSlotChest {
			sp.ItemIds = append(sp.ItemIds, gsChest)
		}
	}
	req.Pool.Slots = append(req.Pool.Slots, &proto.SlotPool{Slot: proto.ItemSlot_ItemSlotWrist, ItemIds: []int32{gsWrist}})
	req.Pool.GemIds = []int32{kaRed, gsRedTwin, gsHit, gsBlue, gsPurple, kaMetaProc, kaMetaPlain}
	if withMeta {
		req.Pool.GemIds = append(req.Pool.GemIds, gsMetaSta)
		req.Pool.MetaConditions = append(req.Pool.MetaConditions, &proto.MetaGemCondition{GemId: gsMetaSta,
			Constraints: []*proto.MetaColorConstraint{{Blue: 1, MinTotal: 2}}})
	}
	r, err := PrepareRequest(req)
	if err != nil {
		t.Fatal(err)
	}
	pick := r.Seed
	pick.Items[proto.ItemSlot_ItemSlotHead] = ItemChoice{ItemID: kaHeadMeta, Gems: rtGems(kaMetaProc, kaRed)}
	pick.Items[proto.ItemSlot_ItemSlotChest] = ItemChoice{ItemID: gsChest, Gems: rtGems(kaRed, kaRed)}
	pick.Items[proto.ItemSlot_ItemSlotWrist] = ItemChoice{ItemID: gsWrist, Gems: rtGems(gsHit)}
	pool, err := CompilePool(r)
	if err != nil {
		t.Fatal(err)
	}
	if hitFloor {
		sheet, err := pool.finalStats(pick)
		if err != nil {
			t.Fatal(err)
		}
		r.Settings.StatMinimums = []*proto.StatMinimum{{Stat: proto.Stat_StatMeleeHit, MinValue: sheet[stats.MeleeHit]}}
		if pool, err = CompilePool(r); err != nil {
			t.Fatal(err)
		}
	}
	if err := pool.Check(pick); err != nil {
		t.Fatal(err)
	}
	resp := Response{}
	for _, st := range []stats.Stat{stats.Strength, stats.MeleeHit, stats.Stamina} {
		resp[st] = &ResponseCurve{Stat: st, SlopeBelow: slopes[st], SlopeAbove: slopes[st]}
	}
	s := newSurrogate(pool, r.Seed, resp)
	// the meta's +40 DPS, as the effect sims would have priced it
	s.gems[kaMetaProc] = Estimate{Mean: 40}
	if err := s.setFloorsAndCeilings(r.Settings.GetStatMinimums(), nil, []stats.Stat{stats.Strength, stats.MeleeHit, stats.SpellHit, stats.Stamina}); err != nil {
		t.Fatal(err)
	}
	slots := []proto.ItemSlot{proto.ItemSlot_ItemSlotHead, proto.ItemSlot_ItemSlotChest, proto.ItemSlot_ItemSlotWrist}
	if regemmed, _, err := pool.Gem(pick, slots, s.linearAt(pick)); err != nil || regemmed != pick {
		t.Fatalf("the DP gems the fixture's pick %v at its slopes, want %v (%v)", regemmed.Items, pick.Items, err)
	}

	run := &run{ctx: context.Background(), asked: r, r: r, budget: EffortBudget(proto.OptimizerEffort_OptimizerEffortQuick), pool: pool,
		surrogates: map[proto.Race]*surrogate{r.Seed.RacialTraits: s}}
	run.eval = &countingEvaluator{inner: newKnownEvaluator(gsDPS), have: map[Point]int{}}
	obj, err := NewObjective(context.Background(), run.eval, r, run.budget.Iterations)
	if err != nil {
		t.Fatal(err)
	}
	run.obj = obj
	run.seedEval = evaluate(t, run.eval, run.budget.Iterations, Point{Loadout: r.Seed})[0]
	run.seedJ = obj.Score(run.seedEval)
	return run, s, pick
}

// The slopes price Stamina at an eighth of what the sim gives it, so neither the search's gems nor a
// gem banned at those slopes brings any. A socket of it simmed at the pick prices it right: the regem
// fills the sockets with Stamina gems, takes the chest's Stamina bonus with a purple gem in its red
// socket, which keeps the meta's red too, and the sim takes that twin.
func TestGemCheckFindsAnUnderpricedStat(t *testing.T) {
	run, s, pick := gemStatRun(t, false, gsSlopes, false)
	stamina := func(id int32) bool { return run.pool.gem(id).Stats[stats.Stamina] > 0 }
	for _, twin := range run.pool.gemTwins(pick, s.linearAt(pick), nil) {
		if bonusActive(twin, proto.ItemSlot_ItemSlotChest) || countGems(twin, stamina) > 1 {
			t.Fatalf("a gem banned at the slopes already brings the Stamina: %v", twin.Items)
		}
	}
	verified := []*verifiedLoadout{{pick, evaluate(t, run.eval, run.budget.Iterations, Point{Loadout: pick})[0]}}
	verified, err := run.checkGems(verified)
	if err != nil {
		t.Fatal(err)
	}
	got := verified[0].loadout
	head, chest, wrist := got.Items[proto.ItemSlot_ItemSlotHead], got.Items[proto.ItemSlot_ItemSlotChest], got.Items[proto.ItemSlot_ItemSlotWrist]
	if head.Gems != rtGems(kaMetaProc, gsBlue) || chest.Gems != rtGems(gsPurple, gsBlue) || wrist.Gems != rtGems(gsBlue) {
		t.Errorf("head %v, chest %v, wrist %v; want a blue gem in the head and bracer, purple and blue in the chest", head.Gems, chest.Gems, wrist.Gems)
	}
	if !bonusActive(got, proto.ItemSlot_ItemSlotChest) || run.pool.Check(got) != nil {
		t.Errorf("the chest's bonus is off or the pick breaks a rule (%v)", run.pool.Check(got))
	}
	if w := run.allWarnings(); len(w) != 1 || !strings.Contains(w[0], "a socket of 15 Stamina sims +60.0") {
		t.Errorf("warnings %q, want one naming the Stamina socket", w)
	}
}

// Slopes a hundred times what the sim gives Strength and Hit, and a quarter over its Stamina, overprice
// every socket, so no single raise fires; priced at what the sockets sim, the regem finds the Stamina
// all the same.
func TestGemCheckPricesInTheSimsUnits(t *testing.T) {
	run, _, pick := gemStatRun(t, false, stats.Stats{stats.Strength: 200, stats.MeleeHit: 300, stats.Stamina: 5}, false)
	verified := []*verifiedLoadout{{pick, evaluate(t, run.eval, run.budget.Iterations, Point{Loadout: pick})[0]}}
	verified, err := run.checkGems(verified)
	if err != nil {
		t.Fatal(err)
	}
	got := verified[0].loadout
	head, chest, wrist := got.Items[proto.ItemSlot_ItemSlotHead], got.Items[proto.ItemSlot_ItemSlotChest], got.Items[proto.ItemSlot_ItemSlotWrist]
	if head.Gems != rtGems(kaMetaProc, gsBlue) || chest.Gems != rtGems(gsPurple, gsBlue) || wrist.Gems != rtGems(gsBlue) {
		t.Errorf("head %v, chest %v, wrist %v; want a blue gem in the head and bracer, purple and blue in the chest", head.Gems, chest.Gems, wrist.Gems)
	}
	if w := run.allWarnings(); len(w) != 1 || !strings.Contains(w[0], "every gem stat priced at what a socket of it sims at the pick; a socket of 15 Stamina sims +60.0") {
		t.Errorf("warnings %q, want one naming the sims' prices and the Stamina socket", w)
	}
}

// The pick's chest holds a blue gem the DP wouldn't put there at its slopes. Strength's raise lands on
// the DP's own gems at the slopes, so that twin names the slopes, and only Stamina's twin names its
// socket.
func TestPriceTwinsNameTheSlopesForTheirOwnRegem(t *testing.T) {
	run, s, atSlopes := gemStatRun(t, false, stats.Stats{stats.Strength: 1.8, stats.MeleeHit: 3, stats.Stamina: 0.5}, false)
	pick := atSlopes
	pick.Items[proto.ItemSlot_ItemSlotChest].Gems = rtGems(kaRed, gsBlue)
	if err := run.pool.Check(pick); err != nil {
		t.Fatal(err)
	}
	verified := &verifiedLoadout{pick, evaluate(t, run.eval, run.budget.Iterations, Point{Loadout: pick})[0]}
	twins, err := run.priceTwins(verified, s, s.slopesAt(pick))
	if err != nil {
		t.Fatal(err)
	}
	var whys []string
	for _, twin := range twins {
		whys = append(whys, twin.why)
		if twin.loadout == atSlopes && twin.why != "every free socket regemmed at the pick's slopes" {
			t.Errorf("the regem at the slopes says %q", twin.why)
		}
		if strings.Contains(twin.why, "Strength") {
			t.Errorf("a twin names Strength's socket: %q", twin.why)
		}
	}
	if !slices.ContainsFunc(twins, func(twin gemTwin) bool { return twin.loadout == atSlopes }) ||
		!slices.ContainsFunc(whys, func(why string) bool { return strings.HasPrefix(why, "a socket of 15 Stamina") }) {
		t.Errorf("twins say %q; want the regem at the slopes and Stamina's own", whys)
	}
}

// With a Hit floor at the pick's, Stamina can't have the bracer's hit gem: the regem keeps it and
// takes Stamina everywhere else.
func TestGemCheckKeepsTheFloors(t *testing.T) {
	run, s, pick := gemStatRun(t, true, gsSlopes, false)
	v := s.slopesAt(pick)
	v[stats.Stamina] = 4
	twin, ok := run.regemAt(pick, s, v, nil)
	if !ok {
		t.Fatal("no regem keeps every rule")
	}
	head, chest, wrist := twin.Items[proto.ItemSlot_ItemSlotHead], twin.Items[proto.ItemSlot_ItemSlotChest], twin.Items[proto.ItemSlot_ItemSlotWrist]
	if head.Gems != rtGems(kaMetaProc, gsBlue) || chest.Gems != rtGems(gsPurple, gsBlue) || wrist.Gems != rtGems(gsHit) {
		t.Errorf("head %v, chest %v, wrist %v; want Stamina in the head and chest and the hit gem kept", head.Gems, chest.Gems, wrist.Gems)
	}
	if err := run.pool.Check(twin); err != nil {
		t.Error(err)
	}
}

// Strength and hit floors right at the pick's. With a single premium raise allowed, lifting one floor's
// premium drops the other floor, as Bulwark P1's Defense and hit did over every round, so the regem keeps the
// slots whose gems feed a floor and regems the trinket's crit gems alone, to Stamina.
func TestRegemKeepsTheFloorsPremiumsCantSplit(t *testing.T) {
	old := premiumRounds
	premiumRounds = 0
	defer func() { premiumRounds = old }()
	gsRegister()
	const head, chest, wrist, trinket = proto.ItemSlot_ItemSlotHead, proto.ItemSlot_ItemSlotChest, proto.ItemSlot_ItemSlotWrist, proto.ItemSlot_ItemSlotTrinket1
	req := kaRequest(proto.OptimizerEffort_OptimizerEffortQuick)
	for _, sp := range req.Pool.Slots {
		switch sp.Slot {
		case chest:
			sp.ItemIds = append(sp.ItemIds, gsChest)
		case trinket:
			sp.ItemIds = append(sp.ItemIds, gsTrinket)
		}
	}
	req.Pool.Slots = append(req.Pool.Slots, &proto.SlotPool{Slot: wrist, ItemIds: []int32{gsWrist}})
	req.Pool.GemIds = []int32{kaRed, gsHit, gsBlue, gsCrit, kaMetaProc}
	r, err := PrepareRequest(req)
	if err != nil {
		t.Fatal(err)
	}
	pick := r.Seed
	pick.Items[head] = ItemChoice{ItemID: kaHeadMeta, Gems: rtGems(kaMetaProc, kaRed)}
	pick.Items[chest] = ItemChoice{ItemID: gsChest, Gems: rtGems(kaRed, gsHit)}
	pick.Items[wrist] = ItemChoice{ItemID: gsWrist, Gems: rtGems(gsHit)}
	pick.Items[trinket] = ItemChoice{ItemID: gsTrinket, Gems: rtGems(gsCrit, gsCrit)}
	pool, err := CompilePool(r)
	if err != nil {
		t.Fatal(err)
	}
	sheet, err := pool.finalStats(pick)
	if err != nil {
		t.Fatal(err)
	}
	r.Settings.StatMinimums = []*proto.StatMinimum{
		{Stat: proto.Stat_StatStrength, MinValue: sheet[stats.Strength]},
		{Stat: proto.Stat_StatMeleeHit, MinValue: sheet[stats.MeleeHit]},
	}
	if pool, err = CompilePool(r); err != nil {
		t.Fatal(err)
	}
	if err := pool.Check(pick); err != nil {
		t.Fatal(err)
	}
	resp := Response{}
	for st, slope := range map[stats.Stat]float64{stats.Strength: 2, stats.MeleeHit: 3, stats.Stamina: 0.5, stats.MeleeCrit: 1.5} {
		resp[st] = &ResponseCurve{Stat: st, SlopeBelow: slope, SlopeAbove: slope}
	}
	s := newSurrogate(pool, r.Seed, resp)
	s.gems[kaMetaProc] = Estimate{Mean: 40}
	if err := s.setFloorsAndCeilings(r.Settings.GetStatMinimums(), nil, []stats.Stat{stats.Strength, stats.MeleeHit, stats.SpellHit, stats.Stamina, stats.MeleeCrit}); err != nil {
		t.Fatal(err)
	}
	run := &run{ctx: context.Background(), asked: r, r: r, pool: pool}
	v := s.slopesAt(pick)
	v[stats.Stamina] = 4
	twin, ok := run.regemAt(pick, s, v, nil)
	if !ok {
		t.Fatal("no regem keeps every rule")
	}
	for _, slot := range []proto.ItemSlot{head, chest, wrist} {
		if twin.Items[slot] != pick.Items[slot] {
			t.Errorf("%s regemmed to %v, want it kept", slot, twin.Items[slot].Gems)
		}
	}
	if got := twin.Items[trinket].Gems; got != rtGems(gsBlue, gsBlue) {
		t.Errorf("trinket gems %v, want two blue ones", got)
	}
}

// The meta re-solves with the gems: at Stamina's real price the Stamina meta beats the one with the
// effect, and the blue gems the Stamina brings activate it.
func TestRegemPicksTheMeta(t *testing.T) {
	run, s, pick := gemStatRun(t, false, gsSlopes, true)
	if twin, ok := run.regemAt(pick, s, s.slopesAt(pick), nil); !ok || twin != pick {
		t.Fatalf("at the slopes the regem moved the pick to %v (%v)", twin.Items[proto.ItemSlot_ItemSlotHead], ok)
	}
	v := s.slopesAt(pick)
	v[stats.Stamina] = 4
	twin, ok := run.regemAt(pick, s, v, nil)
	if !ok || twin.Items[proto.ItemSlot_ItemSlotHead].Gems != rtGems(gsMetaSta, gsBlue) || run.pool.Check(twin) != nil {
		t.Errorf("head %v (%v, %v), want the Stamina meta with a blue gem", twin.Items[proto.ItemSlot_ItemSlotHead].Gems, ok, run.pool.Check(twin))
	}
}

// A twin that beats the pick by under 2 standard errors stays behind it.
func TestGemCheckKeepsThePickOverATwinInsideTheNoise(t *testing.T) {
	r, err := PrepareRequest(kaRequest(proto.OptimizerEffort_OptimizerEffortQuick))
	if err != nil {
		t.Fatal(err)
	}
	// a second yellow gem's worth 3 DPS more than the slopes have it, inside the noise
	fake := newKnownEvaluator(func(p Point) (Metrics, error) {
		m, err := kaMetrics(p)
		m[MetricDPS] += 3 * float64(countGems(p.Loadout, func(id int32) bool { return id == kaYellow }))
		return m, err
	})
	fake.own = 40
	run, pick := gemCheckRun(t, r, fake)
	verified := []*verifiedLoadout{{pick, evaluate(t, run.eval, run.budget.Iterations, Point{Loadout: pick})[0]}}
	verified, err = run.checkGems(verified)
	if err != nil {
		t.Fatal(err)
	}
	if len(verified) < 2 || verified[0].loadout != pick || len(run.allWarnings()) != 0 {
		t.Fatalf("%d verified, head %v first, warnings %q; want the pick first with its twins after it",
			len(verified), verified[0].loadout.Items[proto.ItemSlot_ItemSlotHead], run.allWarnings())
	}
	twin := slices.IndexFunc(verified, func(v *verifiedLoadout) bool {
		return v.loadout.Items[proto.ItemSlot_ItemSlotHead].Gems == rtGems(kaMetaPlain, kaYellow)
	})
	if twin < 0 {
		t.Fatal("no twin with the second yellow gem")
	}
	if d := run.obj.Delta(verified[0].eval, verified[twin].eval); d.Mean <= 0 || d.Mean > 2*d.SE {
		t.Errorf("the twin scores %+.1f ± %.1f over the pick, want a gain under 2 se", d.Mean, d.SE)
	}
}

// statSockets takes one socket of each stat, from the free gem with the most of it (a jewelcrafter's
// gem only where nothing else has the stat), and sims a hit gem's melee and spell rating as one.
func TestStatSockets(t *testing.T) {
	names := func(p *Pool) []string {
		var out []string
		for _, socket := range p.statSockets(nil) {
			out = append(out, socket.name)
		}
		return out
	}
	run, _, _ := gemStatRun(t, false, gsSlopes, false)
	if got, want := names(run.pool), []string{"10 Strength", "15 Stamina", "5 SpellHit+MeleeHit"}; !slices.Equal(got, want) {
		t.Errorf("sockets %q, want %q", got, want)
	}
	// Strength and Crit come off the plain gems, not the jewelcrafter's; only the unique gem has Agility
	if got, want := names(rtPool(t, nil)), []string{"20 Strength", "10 Agility", "30 Stamina", "20 MeleeCrit"}; !slices.Equal(got, want) {
		t.Errorf("sockets %q, want %q", got, want)
	}
}

// Gems that trade places without moving a socket bonus, or a gem for one with its stats and color,
// sim the same; a swap that turns a bonus off doesn't, nor one onto a weapon under an item swap.
func TestGemmingTellsWhatTheSimTellsApart(t *testing.T) {
	run, _, pick := gemStatRun(t, false, gsSlopes, false)
	p := run.pool
	const head, chest = proto.ItemSlot_ItemSlotHead, proto.ItemSlot_ItemSlotChest
	with := func(headGem int32, chestGems ...int32) Loadout {
		l := pick
		l.Items[head].Gems = rtGems(kaMetaProc, headGem)
		l.Items[chest].Gems = rtGems(chestGems...)
		return l
	}
	base := with(gsPurple, kaRed, gsBlue)
	for _, tc := range []struct {
		name string
		l    Loadout
		same bool
	}{
		{"purple and red trade the head's and chest's red sockets", with(kaRed, gsPurple, gsBlue), true},
		{"a red gem with kaRed's stats", with(gsPurple, gsRedTwin, gsBlue), true},
		{"the chest's gems trade sockets, its bonus off", with(gsPurple, gsBlue, kaRed), false},
	} {
		if got := p.gemmingOf(tc.l) == p.gemmingOf(base); got != tc.same {
			t.Errorf("%s: same gemming %v, want %v", tc.name, got, tc.same)
		}
	}

	// an item swap takes the sword off with its gem, so its gem stops trading with the chest's
	armed := func(swordGem int32, chestGems ...int32) Loadout {
		l := with(gsPurple, chestGems...)
		l.Items[proto.ItemSlot_ItemSlotMainHand] = ItemChoice{ItemID: gsSword, Gems: rtGems(swordGem)}
		return l
	}
	a, b := armed(kaRed, gsPurple, gsBlue), armed(gsPurple, kaRed, gsBlue)
	if p.gemmingOf(a) != p.gemmingOf(b) {
		t.Error("without an item swap, the sword's and chest's red sockets trading red and purple gems sim differently")
	}
	p.weaponSwap = true
	if p.gemmingOf(a) == p.gemmingOf(b) {
		t.Error("under an item swap, the sword's and chest's red sockets trading red and purple gems sim the same")
	}
}

// Banning kaRed hands its sockets to gsRedTwin, which sims the same as the pick, so that twin isn't
// simmed; every twin simmed differs from the pick and the others.
func TestGemCheckSimsEachGemmingOnce(t *testing.T) {
	run, _, pick := gemStatRun(t, false, gsSlopes, false)
	verified := []*verifiedLoadout{{pick, evaluate(t, run.eval, run.budget.Iterations, Point{Loadout: pick})[0]}}
	before := run.eval.simmed()
	verified, err := run.checkGems(verified)
	if err != nil {
		t.Fatal(err)
	}
	seen := map[gemming]bool{}
	for _, v := range verified {
		key := run.pool.gemmingOf(v.loadout)
		if seen[key] || countGems(v.loadout, func(id int32) bool { return id == gsRedTwin }) > 0 {
			t.Errorf("%v sims the same as a loadout simmed before it", v.loadout.Items[proto.ItemSlot_ItemSlotChest])
		}
		seen[key] = true
	}
	sockets := len(run.pool.statSockets(nil))
	if got, want := run.eval.simmed()-before, int64((sockets+len(verified)-1)*run.budget.Iterations); got != want {
		t.Errorf("simmed %d iterations, want %d sockets' and %d twins' %d each", got, sockets, len(verified)-1, run.budget.Iterations)
	}
}

// The neighborhood swaps in a trinket with two red sockets, which the search gems red at its slopes.
// The sim prices a socket of Stamina at 60 to Strength's 20, so the gem check it gets after the move
// fills both with blue gems.
func TestNeighborhoodChecksTheGemsItAdopts(t *testing.T) {
	gsRegister()
	const trinket = proto.ItemSlot_ItemSlotTrinket1
	req := kaRequest(proto.OptimizerEffort_OptimizerEffortQuick)
	for _, sp := range req.Pool.Slots {
		if sp.Slot == trinket {
			sp.ItemIds = []int32{kaTrinketAP, gsTrinket}
		}
	}
	for slot := proto.ItemSlot(0); int(slot) < NumSlots; slot++ {
		if slot != trinket {
			req.Settings.LockedSlots = append(req.Settings.LockedSlots, slot)
		}
	}
	req.Pool.GemIds = []int32{kaRed, gsBlue}
	r, err := PrepareRequest(req)
	if err != nil {
		t.Fatal(err)
	}
	pool, err := CompilePool(r)
	if err != nil {
		t.Fatal(err)
	}
	resp := Response{}
	for st, slope := range map[stats.Stat]float64{stats.Strength: 2, stats.Stamina: 0.5, stats.AttackPower: 1} {
		resp[st] = &ResponseCurve{Stat: st, SlopeBelow: slope, SlopeAbove: slope}
	}
	s := newSurrogate(pool, r.Seed, resp)
	rn := &run{ctx: context.Background(), asked: r, r: r, budget: EffortBudget(proto.OptimizerEffort_OptimizerEffortQuick),
		rng: rand.New(rand.NewSource(1)), pool: pool, best: r.Seed, surrogates: map[proto.Race]*surrogate{r.Seed.RacialTraits: s}}
	rn.eval = &countingEvaluator{inner: newKnownEvaluator(gsDPS), have: map[Point]int{}}
	if rn.obj, err = NewObjective(rn.ctx, rn.eval, r, rn.budget.Iterations); err != nil {
		t.Fatal(err)
	}
	rn.seedEval = evaluate(t, rn.eval, rn.budget.Iterations, Point{Loadout: r.Seed})[0]
	rn.bestEval, rn.seedJ = rn.seedEval, rn.obj.Score(rn.seedEval)
	rn.total = rn.eval.simmed() + 1_000_000

	if _, _, err := rn.neighborhood(s, nil); err != nil {
		t.Fatal(err)
	}
	if got := rn.best.Items[trinket]; got.ItemID != gsTrinket || got.Gems != rtGems(gsBlue, gsBlue) {
		t.Errorf("trinket 1 is %d with gems %v, want %d with two blue gems", got.ItemID, got.Gems, gsTrinket)
	}
}
