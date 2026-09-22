package optimizer

import (
	"context"
	"math"
	"strings"
	"testing"

	"github.com/wowsims/wotlk/sim/core/proto"
	"github.com/wowsims/wotlk/sim/core/stats"
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

// The survival/threat slider puts both sides into J: taking less damage raises it, and so does
// landing more threat.
func TestTankSliderObjective(t *testing.T) {
	slider, err := objectiveWeights(&proto.OptimizerSettings{TankSurvival: 0.7})
	want := Metrics{MetricTPS: 0.3, MetricDTPS: 0.35, MetricTMI: 0.35}
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
