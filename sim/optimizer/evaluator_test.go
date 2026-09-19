package optimizer

import (
	"context"
	"errors"
	"math"
	"math/rand"
	"strings"
	"testing"
	"time"

	"github.com/wowsims/wotlk/sim"
	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
	"github.com/wowsims/wotlk/sim/core/stats"
	goproto "google.golang.org/protobuf/proto"
)

func init() {
	sim.RegisterAll()
}

// presetOptimizeRequest puts a preset player alone in a raid with full buffs, against the UI's
// default encounter: one level 83 target for 180 s, give or take 5.
func presetOptimizeRequest(tb testing.TB, name string) *proto.OptimizeGearRequest {
	tb.Helper()
	if !core.WITH_DB {
		tb.Skip("needs the with_db item data")
	}
	var player *proto.Player
	switch name {
	case "fury_p1":
		player = &proto.Player{
			Name:          "Fury",
			Race:          proto.Race_RaceOrc,
			Class:         proto.Class_ClassWarrior,
			Equipment:     core.GetGearSet("../../ui/warrior/gear_sets", "p1_fury").GearSet,
			Rotation:      core.GetAplRotation("../../ui/warrior/apls", "fury").Rotation,
			TalentsString: "302023102331-305053000520310053120500351",
			Glyphs: &proto.Glyphs{
				Major1: int32(proto.WarriorMajorGlyph_GlyphOfWhirlwind),
				Major2: int32(proto.WarriorMajorGlyph_GlyphOfHeroicStrike),
				Major3: int32(proto.WarriorMajorGlyph_GlyphOfRending),
				Minor1: int32(proto.WarriorMinorGlyph_GlyphOfShatteringThrow),
			},
			Spec: &proto.Player_Warrior{Warrior: &proto.Warrior{Options: &proto.Warrior_Options{
				StartingRage:       50,
				UseRecklessness:    true,
				UseShatteringThrow: true,
				Shout:              proto.WarriorShout_WarriorShoutBattle,
			}}},
			Consumes: &proto.Consumes{
				Flask:         proto.Flask_FlaskOfEndlessRage,
				DefaultPotion: proto.Potions_PotionOfSpeed,
				PrepopPotion:  proto.Potions_PotionOfSpeed,
				Food:          proto.Food_FoodFishFeast,
			},
			Buffs: core.FullIndividualBuffs,
		}
	case "arcane_p3":
		player = &proto.Player{
			Name:          "Arcane",
			Race:          proto.Race_RaceTroll,
			Class:         proto.Class_ClassMage,
			Equipment:     core.GetGearSet("../../ui/mage/gear_sets", "p3_arcane_alliance").GearSet,
			Rotation:      core.GetAplRotation("../../ui/mage/apls", "arcane").Rotation,
			TalentsString: "23000513310033015032310250532-03-023303001",
			Glyphs: &proto.Glyphs{
				Major1: int32(proto.MageMajorGlyph_GlyphOfArcaneBlast),
				Major2: int32(proto.MageMajorGlyph_GlyphOfArcaneMissiles),
				Major3: int32(proto.MageMajorGlyph_GlyphOfMoltenArmor),
			},
			Spec: &proto.Player_Mage{Mage: &proto.Mage{Options: &proto.Mage_Options{
				Armor: proto.Mage_Options_MoltenArmor,
			}}},
			Consumes: &proto.Consumes{
				Flask:         proto.Flask_FlaskOfTheFrostWyrm,
				Food:          proto.Food_FoodFirecrackerSalmon,
				DefaultPotion: proto.Potions_PotionOfSpeed,
			},
			Buffs: core.FullIndividualBuffs,
		}
	default:
		tb.Fatalf("no preset %q", name)
	}
	return &proto.OptimizeGearRequest{
		Base: &proto.RaidSimRequest{
			Raid: core.SinglePlayerRaidProto(player, core.FullPartyBuffs, core.FullRaidBuffs, core.FullDebuffs),
			Encounter: &proto.Encounter{
				Duration:             180,
				DurationVariation:    5,
				ExecuteProportion_20: 0.2,
				ExecuteProportion_25: 0.25,
				ExecuteProportion_35: 0.35,
				Targets:              []*proto.Target{core.NewDefaultTarget()},
			},
			SimOptions: &proto.SimOptions{RandomSeed: 101},
		},
		Settings: &proto.OptimizerSettings{ContentPhase: 1},
	}
}

func presetRequest(tb testing.TB, name string) *Request {
	tb.Helper()
	r, err := PrepareRequest(presetOptimizeRequest(tb, name))
	if err != nil {
		tb.Fatal(err)
	}
	return r
}

func evaluate(t *testing.T, e Evaluator, iterations int, points ...Point) []*Evaluation {
	t.Helper()
	evals, err := e.Evaluate(context.Background(), points, iterations)
	if err != nil {
		t.Fatal(err)
	}
	return evals
}

func withOffset(l Loadout, s stats.Stat, v float64) Point {
	p := Point{Loadout: l}
	p.Offset[s] = v
	return p
}

// fakeEvaluator scores points with a known function. Its noise has a part shared by every point at
// the same iteration, which pairing cancels the way the real sim's does, and a small part of its own.
type fakeEvaluator struct {
	metrics func(Point) Metrics
	shared  float64
	own     float64
	calls   int
	points  int
	rng     *rand.Rand
	// noise[i][m] is iteration i's shared noise
	noise [][NumMetrics]float64
}

func newFakeEvaluator(metrics func(Point) Metrics) *fakeEvaluator {
	return &fakeEvaluator{metrics: metrics, shared: 50, own: 0.5, rng: rand.New(rand.NewSource(1))}
}

func (f *fakeEvaluator) Evaluate(ctx context.Context, points []Point, iterations int) ([]*Evaluation, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	f.calls++
	f.points += len(points)
	sharedRng := rand.New(rand.NewSource(int64(len(f.noise)) + 7))
	for len(f.noise) < iterations {
		var n [NumMetrics]float64
		for m := range n {
			n[m] = sharedRng.NormFloat64()
		}
		f.noise = append(f.noise, n)
	}
	out := make([]*Evaluation, len(points))
	for i, p := range points {
		mean := f.metrics(p)
		var values [NumMetrics][]float64
		for m := MetricDPS; m < MetricPDeath; m++ {
			values[m] = make([]float64, iterations)
			for it := range values[m] {
				values[m][it] = mean[m] + f.shared*f.noise[it][m] + f.own*f.rng.NormFloat64()
			}
		}
		deaths := int(math.Round(mean[MetricPDeath] * float64(iterations)))
		s, err := newShard(values, deaths, [NumMetrics]bool{true, true, true, true, true})
		if err != nil {
			return nil, err
		}
		out[i] = (*Evaluation)(nil).extend([]shard{s})
	}
	return out, nil
}

func evaluationOf(t *testing.T, values [NumMetrics][]float64, deaths int, keep [NumMetrics]bool) *Evaluation {
	t.Helper()
	s, err := newShard(values, deaths, keep)
	if err != nil {
		t.Fatal(err)
	}
	return (*Evaluation)(nil).extend([]shard{s})
}

func near(got, want, tolerance float64) bool {
	return math.Abs(got-want) <= tolerance
}

func TestEvaluationStatistics(t *testing.T) {
	values := [NumMetrics][]float64{MetricDPS: {1, 2, 3, 4}, MetricHPS: {0, 0, 0, 0}, MetricTPS: {5, 5, 5, 5}, MetricDTPS: {2, 4, 2, 4}, MetricTMI: {1, 1, 1, 1}}
	first := evaluationOf(t, values, 1, [NumMetrics]bool{MetricDPS: true})

	if first.Iterations != 4 || !near(first.Metrics[MetricDPS].Mean, 2.5, 1e-9) || !near(first.Metrics[MetricDPS].SE, math.Sqrt(5.0/3/4), 1e-9) {
		t.Errorf("DPS = %+v over %d iterations", first.Metrics[MetricDPS], first.Iterations)
	}
	if !near(first.Metrics[MetricPDeath].Mean, 0.25, 1e-9) || !near(first.Metrics[MetricPDeath].SE, math.Sqrt(0.25*0.75/4), 1e-9) {
		t.Errorf("death chance = %+v", first.Metrics[MetricPDeath])
	}
	if first.samples[MetricDPS] == nil || first.samples[MetricTPS] != nil {
		t.Error("kept the wrong metrics")
	}

	more, _ := newShard(values, 0, [NumMetrics]bool{MetricDPS: true})
	both := first.extend([]shard{more})
	if both.Iterations != 8 || len(both.samples[MetricDPS]) != 8 || !near(both.Metrics[MetricPDeath].Mean, 0.125, 1e-9) {
		t.Errorf("extended = %d iterations, %d samples, death chance %v", both.Iterations, len(both.samples[MetricDPS]), both.Metrics[MetricPDeath])
	}
	if first.Iterations != 4 || len(first.samples[MetricDPS]) != 4 {
		t.Error("extend changed the evaluation it started from")
	}
}

func TestDeltaPairsIterations(t *testing.T) {
	rng := rand.New(rand.NewSource(3))
	var a, b [NumMetrics][]float64
	for m := MetricDPS; m < MetricPDeath; m++ {
		a[m], b[m] = make([]float64, 1000), make([]float64, 1000)
		for i := range a[m] {
			noise := 100 * rng.NormFloat64()
			a[m][i] = 1000 + noise
			b[m][i] = 1005 + noise + 0.1*rng.NormFloat64()
		}
	}
	keep := [NumMetrics]bool{MetricDPS: true}
	ea, eb := evaluationOf(t, a, 0, keep), evaluationOf(t, b, 0, keep)

	paired := Delta(ea, eb, MetricDPS)
	unpaired := Delta(ea, eb, MetricTPS)
	if !near(paired.Mean, 5, 0.05) || paired.SE > 0.01 {
		t.Errorf("paired delta = %+v, want 5 with a tiny standard error", paired)
	}
	if !near(unpaired.Mean, 5, 0.05) || unpaired.SE < 3 {
		t.Errorf("unpaired delta = %+v, want 5 with the independent standard error", unpaired)
	}

	// pairs over the first 500 iterations only when one side ran fewer
	short := evaluationOf(t, [NumMetrics][]float64{MetricDPS: a[MetricDPS][:500], MetricHPS: a[MetricHPS][:500], MetricTPS: a[MetricTPS][:500], MetricDTPS: a[MetricDTPS][:500], MetricTMI: a[MetricTMI][:500]}, 0, keep)
	if d := Delta(short, eb, MetricDPS); !near(d.Mean, 5, 0.05) || d.SE > 0.01 {
		t.Errorf("delta against a shorter run = %+v", d)
	}

	score := combinedScore(ea, Metrics{MetricDPS: 2, MetricTPS: 1})
	if !near(score.Mean, 2*ea.Metrics[MetricDPS].Mean+ea.Metrics[MetricTPS].Mean, 1e-6) || score.SE <= 0 {
		t.Errorf("score = %+v", score)
	}
}

func TestSimEvaluatorShardsMatchOneSim(t *testing.T) {
	r := presetRequest(t, "fury_p1")
	sharded := NewSimEvaluator(r, MetricDPS)
	sharded.shardSize = 100
	whole := NewSimEvaluator(r, MetricDPS)
	whole.shardSize = 400

	a := evaluate(t, sharded, 400, Point{Loadout: r.Seed})[0]
	b := evaluate(t, whole, 400, Point{Loadout: r.Seed})[0]
	if a.Iterations != 400 || b.Iterations != 400 {
		t.Fatalf("got %d and %d iterations, want 400", a.Iterations, b.Iterations)
	}
	dpsA, dpsB := a.Metrics[MetricDPS], b.Metrics[MetricDPS]
	if dpsA.Mean < 1000 || math.Abs(dpsA.Mean-dpsB.Mean) > dpsA.SE {
		t.Errorf("sharded DPS %v, unsharded %v: more than 1 se apart", dpsA, dpsB)
	}
	t.Logf("sharded %.2f ± %.2f, unsharded %.2f ± %.2f", dpsA.Mean, dpsA.SE, dpsB.Mean, dpsB.SE)
}

func TestSimEvaluatorLeavesTheRequestAlone(t *testing.T) {
	req := presetOptimizeRequest(t, "fury_p1")
	original := goproto.Clone(req)
	r, err := PrepareRequest(req)
	if err != nil {
		t.Fatal(err)
	}
	base := goproto.Clone(r.Base)

	e := NewSimEvaluator(r, MetricDPS)
	e.shardSize = 50
	changed := r.Seed
	changed.Items[proto.ItemSlot_ItemSlotFinger1] = ItemChoice{}
	changed.RacialTraits = proto.Race_RaceHuman
	evaluate(t, e, 50, Point{Loadout: r.Seed}, withOffset(r.Seed, stats.AttackPower, 100), Point{Loadout: changed})

	if !goproto.Equal(r.Base, base) {
		t.Error("the evaluator changed the prepared request's base")
	}
	if !goproto.Equal(req, original) {
		t.Error("the caller's request changed")
	}
}

func TestSimEvaluatorPairs(t *testing.T) {
	r := presetRequest(t, "fury_p1")
	e := NewSimEvaluator(r, MetricDPS)
	e.shardSize = 100
	evals := evaluate(t, e, 400, Point{Loadout: r.Seed}, withOffset(r.Seed, stats.AttackPower, 100))

	paired := Delta(evals[0], evals[1], MetricDPS)
	a, b := evals[0].Metrics[MetricDPS], evals[1].Metrics[MetricDPS]
	unpairedSE := math.Hypot(a.SE, b.SE)
	if paired.Mean <= 2*paired.SE || paired.SE > unpairedSE/2 {
		t.Errorf("+100 AP = %.2f ± %.2f DPS paired, unpaired se %.2f: want a clear gain, and pairing to at least halve the noise", paired.Mean, paired.SE, unpairedSE)
	}
	t.Logf("+100 AP: %.2f ± %.2f paired, ± %.2f unpaired", paired.Mean, paired.SE, unpairedSE)
}

func TestSimEvaluatorCachesAndExtends(t *testing.T) {
	r := presetRequest(t, "fury_p1")
	e := NewSimEvaluator(r, MetricDPS)
	e.shardSize = 50
	seed := Point{Loadout: r.Seed}

	first := evaluate(t, e, 50, seed, seed)
	if e.SimmedIterations() != 50 || first[0] != first[1] {
		t.Fatalf("simmed %d iterations for a repeated point, want 50 shared", e.SimmedIterations())
	}
	if again := evaluate(t, e, 30, seed)[0]; again != first[0] || e.SimmedIterations() != 50 {
		t.Errorf("a cached point simmed again: %d iterations", e.SimmedIterations())
	}

	longer := evaluate(t, e, 100, seed)[0]
	if longer.Iterations != 100 || e.SimmedIterations() != 100 {
		t.Fatalf("extending to 100 iterations gave %d and simmed %d in all", longer.Iterations, e.SimmedIterations())
	}
	for i, v := range first[0].samples[MetricDPS] {
		if longer.samples[MetricDPS][i] != v {
			t.Fatalf("iteration %d changed when the run was extended", i)
		}
	}
	if first[0].Iterations != 50 {
		t.Error("extending changed an evaluation already handed out")
	}
}

func TestSimEvaluatorRecoversPanics(t *testing.T) {
	r := presetRequest(t, "fury_p1")
	e := NewSimEvaluator(r, MetricDPS)
	e.shardSize = 50
	bad := r.Seed
	bad.Items[proto.ItemSlot_ItemSlotHead] = ItemChoice{ItemID: 9499993}

	_, err := e.Evaluate(context.Background(), []Point{{Loadout: r.Seed}, {Loadout: bad}}, 50)
	var simErr *SimError
	if !errors.As(err, &simErr) || simErr.Point.Loadout != bad || !strings.Contains(simErr.Message, "9499993") {
		t.Fatalf("err = %v, want a SimError for the unknown item", err)
	}

	// failures aren't cached, so this sims it again
	if _, err := e.Evaluate(context.Background(), []Point{{Loadout: bad}}, 50); !errors.As(err, &simErr) {
		t.Errorf("the failed point came back as %v", err)
	}
	if ev := evaluate(t, e, 50, Point{Loadout: r.Seed})[0]; ev.Iterations != 50 {
		t.Errorf("the evaluator broke after a panic: %d iterations", ev.Iterations)
	}
}

func TestSimEvaluatorCancels(t *testing.T) {
	r := presetRequest(t, "fury_p1")
	r.Settings.Workers = 2

	e := NewSimEvaluator(r, MetricDPS)
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := e.Evaluate(cancelled, []Point{{Loadout: r.Seed}}, 100); !errors.Is(err, context.Canceled) || e.SimmedIterations() != 0 {
		t.Errorf("already cancelled: err = %v after %d iterations", err, e.SimmedIterations())
	}

	e.shardSize = 20
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		_, err := e.Evaluate(ctx, []Point{{Loadout: r.Seed}}, 20000)
		done <- err
	}()
	deadline := time.Now().Add(time.Minute)
	for e.SimmedIterations() == 0 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Errorf("err = %v, want context.Canceled", err)
		}
	case <-time.After(time.Minute):
		t.Fatal("Evaluate didn't stop after the cancel")
	}
	if n := e.SimmedIterations(); n >= 20000 {
		t.Errorf("simmed all %d iterations despite the cancel", n)
	}

	// the shards that finished first are kept, and not simmed again
	cached := e.cache[Point{Loadout: r.Seed}]
	if cached == nil {
		return
	}
	if cached.Iterations >= 20000 || cached.Iterations%e.shardSize != 0 {
		t.Fatalf("cached %d iterations after the cancel, want whole shards short of 20000", cached.Iterations)
	}
	simmed := e.SimmedIterations()
	if ev := evaluate(t, e, cached.Iterations, Point{Loadout: r.Seed})[0]; ev != cached || e.SimmedIterations() != simmed {
		t.Errorf("the kept shards simmed again")
	}
}
