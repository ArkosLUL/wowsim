package optimizer

import (
	"context"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/wowsims/wotlk/sim/core/proto"
	"github.com/wowsims/wotlk/sim/core/stats"
	goproto "google.golang.org/protobuf/proto"
)

func TestObjectiveWeights(t *testing.T) {
	for _, tc := range []struct {
		name     string
		settings *proto.OptimizerSettings
		want     Metrics
		err      string
	}{
		{"default is DPS", &proto.OptimizerSettings{}, Metrics{MetricDPS: 1}, ""},
		{"nil settings", nil, Metrics{MetricDPS: 1}, ""},
		{"tank blend", &proto.OptimizerSettings{MetricWeights: &proto.OptimizerMetrics{Tps: 0.3, Tmi: 0.7}}, Metrics{MetricTPS: 0.3, MetricTMI: 0.7}, ""},
		{"negative", &proto.OptimizerSettings{MetricWeights: &proto.OptimizerMetrics{Dtps: -1}}, Metrics{}, "negative"},
		{"raid mode", &proto.OptimizerSettings{Objective: proto.OptimizerObjective_OptimizerObjectiveRaidDps}, Metrics{MetricDPS: 1}, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := objectiveWeights(tc.settings)
			if tc.err != "" {
				if err == nil || !strings.Contains(err.Error(), tc.err) {
					t.Errorf("err = %v, want one about %q", err, tc.err)
				}
				return
			}
			if err != nil || got != tc.want {
				t.Errorf("weights = %v, %v; want %v", got, err, tc.want)
			}
		})
	}

	metrics, err := WeightedMetrics(&proto.OptimizerSettings{MetricWeights: &proto.OptimizerMetrics{Tps: 0.3, Tmi: 0.7}})
	if err != nil || len(metrics) != 2 || metrics[0] != MetricTPS || metrics[1] != MetricTMI {
		t.Errorf("WeightedMetrics = %v, %v", metrics, err)
	}
}

func TestReferenceStat(t *testing.T) {
	for _, tc := range []struct {
		player *proto.Player
		want   stats.Stat
	}{
		{&proto.Player{Spec: &proto.Player_Warrior{}}, stats.AttackPower},
		{&proto.Player{Spec: &proto.Player_Hunter{}}, stats.RangedAttackPower},
		{&proto.Player{Spec: &proto.Player_Mage{}}, stats.SpellPower},
		{&proto.Player{Spec: &proto.Player_ProtectionPaladin{}}, stats.SpellPower},
		{&proto.Player{Spec: &proto.Player_FeralTankDruid{}}, stats.AttackPower},
	} {
		if got := referenceStat(tc.player); got != tc.want {
			t.Errorf("%T: reference stat %s, want %s", tc.player.Spec, got.StatName(), tc.want.StatName())
		}
	}
}

// tankRequest is testRequest's warrior with a warrior spec, so its reference stat is AP.
func tankRequest(t *testing.T, weights *proto.OptimizerMetrics) *Request {
	t.Helper()
	req := testRequest()
	req.Base.Raid.Parties[1].Players[1].Spec = &proto.Player_Warrior{Warrior: &proto.Warrior{}}
	req.Settings.MetricWeights = weights
	r, err := PrepareRequest(req)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestNewObjectiveNormalizes(t *testing.T) {
	r := tankRequest(t, &proto.OptimizerMetrics{Tps: 1, Dtps: 1, Tmi: 1, PDeath: 1})
	fake := newFakeEvaluator(func(p Point) Metrics {
		return Metrics{
			MetricDPS:  5000 + 2*p.Offset[stats.AttackPower],
			MetricTPS:  3000 + p.Offset[stats.AttackPower],
			MetricDTPS: 1000 - 0.05*p.Offset[stats.Armor],
			MetricTMI:  50 - 0.001*p.Offset[stats.Armor],
		}
	})
	fake.shared, fake.own = 0.01, 0.0001

	o, err := NewObjective(context.Background(), fake, r, 1000)
	if err != nil {
		t.Fatal(err)
	}
	if fake.calls != 1 || fake.points != 4 {
		t.Errorf("simmed %d points in %d calls, want AP and Armor down and up in one call", fake.points, fake.calls)
	}
	if o.ReferenceStats[MetricTPS] != stats.AttackPower || o.ReferenceStats[MetricTMI] != stats.Armor {
		t.Errorf("reference stats = %v", o.ReferenceStats)
	}
	for m, want := range map[Metric]float64{MetricTPS: 1, MetricDTPS: -0.05, MetricTMI: -0.001} {
		if got := o.Normalizers[m]; !near(got.Mean, want, 0.02*math.Abs(want)) {
			t.Errorf("%s normalizer = %+v, want %g", metricName(m), got, want)
		}
	}
	if len(o.Warnings) != 1 || !strings.Contains(o.Warnings[0], "death chance") {
		t.Errorf("warnings = %q, want death chance left out", o.Warnings)
	}

	evals := evaluate(t, fake, 1000, Point{Loadout: r.Seed}, withOffset(r.Seed, stats.Armor, 100))
	if score := o.Score(evals[0]); !near(score.Mean, 3000-1000/0.05-50/0.001, 200) {
		t.Errorf("score = %+v, want 3000 TPS minus the DTPS and TMI terms", score)
	}
	// +100 armor is worth 100 armor-points of DTPS and 100 of TMI, and no TPS
	if d := o.Delta(evals[0], evals[1]); !near(d.Mean, 200, 5) || d.SE > 5 {
		t.Errorf("+100 armor = %+v, want 200 paired", d)
	}
}

func TestNewObjectiveNeedsAResponse(t *testing.T) {
	r := tankRequest(t, nil)
	flat := newFakeEvaluator(func(Point) Metrics { return Metrics{MetricDPS: 5000} })
	flat.own = 0
	if _, err := NewObjective(context.Background(), flat, r, 200); err == nil || !strings.Contains(err.Error(), "no weighted metric") {
		t.Errorf("err = %v, want one saying nothing responds", err)
	}

	r.Settings.Objective = proto.OptimizerObjective_OptimizerObjectiveRaidDps
	if _, err := NewObjective(context.Background(), flat, r, 200); err == nil {
		t.Error("raid mode didn't fail")
	}
}

func TestNewObjectiveFury(t *testing.T) {
	r := presetRequest(t, "fury_p1")
	e := NewSimEvaluator(r, MetricDPS)
	e.shardSize = 100
	o, err := NewObjective(context.Background(), e, r, 400)
	if err != nil {
		t.Fatal(err)
	}
	w := o.Normalizers[MetricDPS]
	if o.ReferenceStats[MetricDPS] != stats.AttackPower || w.Mean <= 2*w.SE {
		t.Fatalf("DPS per AP = %+v against %s", w, o.ReferenceStats[MetricDPS].StatName())
	}
	seed := evaluate(t, e, 400, Point{Loadout: r.Seed})[0]
	score := o.Score(seed)
	if !near(score.Mean, seed.Metrics[MetricDPS].Mean/w.Mean, 1e-6*score.Mean) {
		t.Errorf("J = %v, want DPS / (DPS per AP) = %v", score.Mean, seed.Metrics[MetricDPS].Mean/w.Mean)
	}
	t.Logf("%.3f ± %.3f DPS per AP; J = %.1f ± %.1f AP", w.Mean, w.SE, score.Mean, score.SE)
}

// A slot the trimmer emptied for lack of a pool candidate (a weapon, most sharply) inflates the
// trimmed seed's own DPS-per-AP: the objective measures its normalizers against the gear as
// equipped instead, so an emptied weapon slot doesn't size the score's points on gear the player
// doesn't have (BIS-seed's DK case, about 12%).
func TestNewObjectiveNormalizesAgainstEquippedNotTrimmedSeed(t *testing.T) {
	req := presetOptimizeRequest(t, "fury_p1")
	player := req.Base.Raid.Parties[0].Players[0]
	req.Equipped = goproto.Clone(player.Equipment).(*proto.EquipmentSpec)
	// as the trimmer leaves it when the pool has no weapon for this phase
	player.Equipment.Items[proto.ItemSlot_ItemSlotMainHand] = &proto.ItemSpec{}
	player.Equipment.Items[proto.ItemSlot_ItemSlotOffHand] = &proto.ItemSpec{}

	r, err := PrepareRequest(req)
	if err != nil {
		t.Fatal(err)
	}
	if r.Seed == r.Equipped {
		t.Fatal("the fixture needs the trimmed seed to differ from the gear as equipped")
	}

	const iterations = 4000
	e := NewSimEvaluator(r, MetricDPS)
	e.shardSize = 100
	o, err := NewObjective(context.Background(), e, r, iterations)
	if err != nil {
		t.Fatal(err)
	}
	got := o.Normalizers[MetricDPS]

	// what measuring against the weapon-less trimmed seed, the old behavior, would have found
	seedEval := NewSimEvaluator(r, MetricDPS)
	seedEval.shardSize = 100
	evals := evaluate(t, seedEval, iterations, withOffset(r.Seed, stats.AttackPower, -100), withOffset(r.Seed, stats.AttackPower, 100))
	d := Delta(evals[0], evals[1], MetricDPS)
	seedNorm := d.Mean / 200

	if near(got.Mean, seedNorm, 0.03*math.Abs(seedNorm)) {
		t.Errorf("normalizer %.4f is within 3%% of the weapon-less seed's %.4f, want it measured against the equipped weapon instead", got.Mean, seedNorm)
	}
	t.Logf("equipped-based normalizer %.4f vs the weapon-less trimmed seed's %.4f (%+.1f%%)", got.Mean, seedNorm, 100*(got.Mean-seedNorm)/seedNorm)
}

// The survival/threat slider puts both sides into J: taking less damage raises it, and so does
// landing more threat.
func TestTankSliderObjective(t *testing.T) {
	slider, err := objectiveWeights(&proto.OptimizerSettings{TankSurvival: 0.7})
	want := Metrics{MetricTPS: 0.3, MetricHealthMargin: 0.35, MetricDTPS: 0.7 / 6, MetricTMI: 0.7 / 6, MetricPDeath: 0.7 / 6}
	if err != nil {
		t.Fatal(err)
	}
	for m := range slider {
		if !near(slider[m], want[m], 1e-9) {
			t.Fatalf("slider weights = %v, want %v", slider, want)
		}
	}
	// the UI sends both, and its own weights win
	both := &proto.OptimizerSettings{TankSurvival: 0.7, MetricWeights: &proto.OptimizerMetrics{Dps: 1}}
	if got, err := objectiveWeights(both); err != nil || got != (Metrics{MetricDPS: 1}) {
		t.Errorf("weights alongside the slider = %v, %v", got, err)
	}
	if _, err := objectiveWeights(&proto.OptimizerSettings{TankSurvival: 1.5}); err == nil {
		t.Error("a slider past 1 didn't fail")
	}

	const (
		quieterID = 90001
		angrierID = 90002
	)
	r := tankRequest(t, nil)
	r.Settings.TankSurvival = 0.7
	fake := newFakeEvaluator(func(p Point) Metrics {
		m := Metrics{
			MetricTPS:  3000 + p.Offset[stats.AttackPower],
			MetricDTPS: 1000 - 0.05*p.Offset[stats.Armor],
			MetricTMI:  50 - 0.001*p.Offset[stats.Armor],
		}
		switch p.Loadout.Items[proto.ItemSlot_ItemSlotHead].ItemID {
		case quieterID:
			m[MetricDTPS] -= 50
		case angrierID:
			m[MetricTPS] += 50
		}
		return m
	})
	fake.shared, fake.own = 0.01, 0.0001

	o, err := NewObjective(context.Background(), fake, r, 1000)
	if err != nil {
		t.Fatal(err)
	}

	wearing := func(id int32) Point {
		l := r.Seed
		l.Items[proto.ItemSlot_ItemSlotHead].ItemID = id
		return Point{Loadout: l}
	}
	evals := evaluate(t, fake, 1000, Point{Loadout: r.Seed}, wearing(quieterID), wearing(angrierID))
	if d := o.Delta(evals[0], evals[1]); d.Mean <= 2*d.SE {
		t.Errorf("50 less DTPS moved J by %+v; it should raise it", d)
	}
	if d := o.Delta(evals[0], evals[2]); d.Mean <= 2*d.SE {
		t.Errorf("50 more TPS moved J by %+v; it should raise it", d)
	}
}

// The health margin is normalized on Armor like the other survival metrics, but higher is better, so
// its normalizer and coefficient come out positive and more margin raises J.
func TestHealthMarginNormalizesOnArmor(t *testing.T) {
	r := tankRequest(t, &proto.OptimizerMetrics{Tps: 0.3, HealthMargin: 0.7})
	fake := newFakeEvaluator(func(p Point) Metrics {
		return Metrics{
			MetricTPS:          3000 + p.Offset[stats.AttackPower],
			MetricHealthMargin: 20000 + 2*p.Offset[stats.Armor] + p.Offset[stats.Health],
		}
	})
	fake.shared, fake.own = 0.01, 0.0001
	o, err := NewObjective(context.Background(), fake, r, 1000)
	if err != nil {
		t.Fatal(err)
	}
	if o.ReferenceStats[MetricHealthMargin] != stats.Armor || !near(o.Normalizers[MetricHealthMargin].Mean, 2, 0.01) || o.coef[MetricHealthMargin] <= 0 {
		t.Errorf("margin against %s: %+v per point, coefficient %v; want Armor, 2 and positive",
			o.ReferenceStats[MetricHealthMargin].StatName(), o.Normalizers[MetricHealthMargin], o.coef[MetricHealthMargin])
	}
	if len(o.Warnings) != 0 {
		t.Errorf("warnings %q", o.Warnings)
	}
	// 1000 more health is 1000 more margin: 500 Armor's worth, at the margin's 0.7
	evals := evaluate(t, fake, 1000, Point{Loadout: r.Seed}, withOffset(r.Seed, stats.Health, 1000))
	if d := o.Delta(evals[0], evals[1]); !near(d.Mean, 0.7*500, 1) {
		t.Errorf("1000 more margin moved J %+v, want %v", d, 0.7*500)
	}
}

// A target with no healing model reports no margin at all, and the warning says why.
func TestHealthMarginLeftOutWithoutAHealingModel(t *testing.T) {
	r := tankRequest(t, &proto.OptimizerMetrics{Tps: 0.3, HealthMargin: 0.7})
	fake := newFakeEvaluator(func(p Point) Metrics { return Metrics{MetricTPS: 3000 + p.Offset[stats.AttackPower]} })
	fake.shared, fake.own = 0, 0
	o, err := NewObjective(context.Background(), fake, r, 1000)
	if err != nil {
		t.Fatal(err)
	}
	if o.coef[MetricHealthMargin] != 0 || len(o.Warnings) != 1 || !strings.Contains(o.Warnings[0], "no healing model") {
		t.Errorf("coefficient %v, warnings %q; want the margin left out for want of a healing model", o.coef[MetricHealthMargin], o.Warnings)
	}
}

func TestDeathCoefficientNeedsAClearSignal(t *testing.T) {
	measure := func(mean, se, down, up, armor float64) deathMeasure {
		return deathMeasure{Norm: Estimate{Mean: mean, SE: se}, Down: down, Up: up, Dies: (down + up) / 2, Armor: armor}
	}
	const weight = 0.2
	for _, tc := range []struct {
		name string
		dm   deathMeasure
		coef float64
		left string
	}{
		{"dies in every iteration", measure(0, 0, 1, 1, 27000), 0, "dies in 100%"},
		{"dies in none", measure(0, 0, 0, 0, 27000), 0, "dies in 0%"},
		{"under 2 se", measure(-2e-7, 1.5e-7, 0.002, 0.001, 27000), 0, "doesn't move that measurably"},
		{"Halion's worn gear at 5.7 se", measure(-2.1e-6, 3.7e-7, 0.9998, 0.9912, 18000), 0, "only 5.7 standard errors"},
		{"a clear signal", measure(-9e-5, 2e-6, 0.4, 0.04, 27000), weight / -9e-5, ""},
		{"just clearing the bar", measure(-1e-5, 1e-6, 0.4, 0.04, 27000), weight / -1e-5, ""},
		// 20,000 points of J for a certain death, against 18,000 Armor to give up
		{"capped at the Armor worn", measure(-1e-5, 5e-7, 0.5, 0.3, 18000), -18000, ""},
	} {
		coef, left := deathCoefficient(weight, tc.dm)
		if !near(coef, tc.coef, 1e-6*math.Abs(tc.coef)) || !strings.Contains(left, tc.left) || (tc.left == "") != (left == "") {
			t.Errorf("%s: coefficient %g and %q, want %g and %q", tc.name, coef, left, tc.coef, tc.left)
		}
	}
}

// Where the gear as equipped always dies, the first search runs without death chance, and the pick
// decides whether a second one counts it: only a pick that dies in between and responds to Armor does.
func TestDeathChanceComesBackAtTheFirstPick(t *testing.T) {
	for _, tc := range []struct {
		name   string
		pick   func(armor float64) float64
		second bool
		stays  string
	}{
		{"a pick that responds to Armor", func(armor float64) float64 { return 0.5 - 1.25e-4*armor }, true, ""},
		{"a pick that dies every time", func(float64) float64 { return 1 }, false, "dies in 100%"},
		{"a pick that never dies", func(float64) float64 { return 0 }, false, "dies in 0%"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := kaRequest(proto.OptimizerEffort_OptimizerEffortQuick)
			req.Settings.MetricWeights = &proto.OptimizerMetrics{Dps: 1, PDeath: 1}
			r, err := PrepareRequest(req)
			if err != nil {
				t.Fatal(err)
			}
			fake := newFakeEvaluator(func(p Point) Metrics {
				dps, _ := kaMetrics(p)
				if p.Loadout == r.Equipped {
					dps[MetricPDeath] = 1
				} else {
					dps[MetricPDeath] = tc.pick(p.Offset[stats.Armor])
				}
				return dps
			})
			result := optimize(context.Background(), r, r, fake, nil, time.Now())
			if result.ErrorResult != "" {
				t.Fatal(result.ErrorResult)
			}
			if !result.Improved {
				t.Fatal("the known-answer pool should beat the seed")
			}
			var counted, kept bool
			for _, w := range result.Warnings {
				counted = counted || strings.Contains(w, "a second search counted death chance")
				kept = kept || strings.Contains(w, "death chance stays out") && strings.Contains(w, tc.stays)
			}
			if counted != tc.second || kept == tc.second || tc.second && !strings.Contains(result.Warnings[0], "left death chance out") {
				t.Errorf("warnings %q, want a second search: %v", result.Warnings, tc.second)
			}
		})
	}
}
