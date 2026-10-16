package optimizer

import (
	"context"
	"strings"
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
	if got := run.eval.simmed() - before; got != 2*int64(run.budget.Iterations) {
		t.Errorf("counted %d iterations, want the two twins' %d each", got, run.budget.Iterations)
	}
	warnings := run.allWarnings()
	if len(warnings) != 1 || !strings.Contains(warnings[0], "swapping 1 gem 9700101 for 1 gem 9700102") || !strings.Contains(warnings[0], "best of 2") {
		t.Errorf("warnings %q, want one naming the swap", warnings)
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
