package optimizer

import (
	"context"
	"strings"
	"testing"

	"github.com/wowsims/wotlk/sim/core/stats"
)

// dpsObjective is J = DPS, so fake curves read straight off the fake's formula.
func dpsObjective() *Objective {
	return &Objective{Weights: Metrics{MetricDPS: 1}, coef: Metrics{MetricDPS: 1}}
}

func TestMeasureResponseFindsTheCap(t *testing.T) {
	const hitCap = 120.0
	fake := newFakeEvaluator(func(p Point) Metrics {
		hit, str := p.Offset[stats.MeleeHit], p.Offset[stats.Strength]
		return Metrics{MetricDPS: 5000 + 3*min(hit, hitCap) + 0.5*max(hit-hitCap, 0) + 2*str - 0.001*str*str}
	})
	resp, err := MeasureResponse(context.Background(), fake, dpsObjective(), Loadout{}, map[stats.Stat]ResponseRange{
		stats.MeleeHit:  {Min: -100, Max: 300},
		stats.Strength:  {Min: -100, Max: 100},
		stats.Expertise: {},
	}, 1000)
	if err != nil {
		t.Fatal(err)
	}

	hit := resp[stats.MeleeHit]
	if hit == nil || !near(hit.Breakpoint, hitCap, 2) || !near(hit.SlopeBelow, 3, 0.02) || !near(hit.SlopeAbove, 0.5, 0.02) {
		t.Fatalf("hit curve = %+v, want a break at %g from 3 to 0.5", hit, hitCap)
	}
	if fake.calls != 1+bisectionSteps || len(hit.Knots) != 5+bisectionSteps {
		t.Errorf("%d evaluate calls and %d knots, want the first batch plus %d bisection steps", fake.calls, len(hit.Knots), bisectionSteps)
	}
	str := resp[stats.Strength]
	if str == nil || str.Breakpoint != 0 || !near(str.SlopeBelow, 2.1, 0.01) || !near(str.SlopeAbove, 1.9, 0.01) {
		t.Errorf("strength curve = %+v, want slopes 2.1 below the seed and 1.9 above", str)
	}
	if _, ok := resp[stats.Expertise]; ok {
		t.Error("an empty range got a curve")
	}

	if v := resp.Value(stats.Stats{}); v != 0 {
		t.Errorf("value at the seed = %g", v)
	}
	want := 3*hitCap + 0.5*80 + 1.9*50
	if v := resp.Value(stats.Stats{stats.MeleeHit: 200, stats.Strength: 50, stats.Agility: 1000}); !near(v, want, 2) {
		t.Errorf("value = %g, want %g", v, want)
	}
}

// A cap inside an outermost segment leaves one side without a clean slope to start from. Real
// case: Fury P1's expertise caps 36 rating under the seed.
func TestMeasureResponseCapInAnOuterSegment(t *testing.T) {
	fake := newFakeEvaluator(func(p Point) Metrics {
		exp, hit := p.Offset[stats.Expertise], p.Offset[stats.SpellHit]
		return Metrics{MetricDPS: 5000 + 2*min(exp+40, 0) + 3*min(hit, 45)}
	})
	resp, err := MeasureResponse(context.Background(), fake, dpsObjective(), Loadout{}, map[stats.Stat]ResponseRange{
		stats.Expertise: {Min: -60, Max: 60},
		stats.SpellHit:  {Min: -60, Max: 60},
	}, 1000)
	if err != nil {
		t.Fatal(err)
	}
	if exp := resp[stats.Expertise]; !near(exp.Breakpoint, -40, 1) || !near(exp.SlopeBelow, 2, 0.02) || !near(exp.SlopeAbove, 0, 0.02) {
		t.Errorf("expertise curve = %+v, want a break at -40 from 2 to 0", exp)
	}
	if hit := resp[stats.SpellHit]; !near(hit.Breakpoint, 45, 1) || !near(hit.SlopeBelow, 3, 0.02) || !near(hit.SlopeAbove, 0, 0.02) {
		t.Errorf("spell hit curve = %+v, want a break at 45 from 3 to 0", hit)
	}
}

func TestFindBreakpointNeedsFourKnots(t *testing.T) {
	// slope 3 up to +5, then 0.5
	kink := []Knot{{Offset: -10, Delta: Estimate{Mean: -30}}, {Offset: 0}, {Offset: 10, Delta: Estimate{Mean: 17.5}}}
	if b := findBreakpoint(kink); b != nil {
		t.Errorf("3 knots gave a bracket %+v; neither side has a clean segment", b)
	}
	kink = append(kink, Knot{Offset: 20, Delta: Estimate{Mean: 22.5}})
	if b := findBreakpoint(kink); b == nil || b.lo.Offset != 0 || b.hi.Offset != 10 || !near(b.estimate(), 5, 1e-9) {
		t.Errorf("4 knots gave %+v, want the break at 5, bracketed by 0 and 10", b)
	}
}

func TestMeasureResponseWithoutACap(t *testing.T) {
	fake := newFakeEvaluator(func(p Point) Metrics {
		return Metrics{MetricDPS: 5000 + 1.5*p.Offset[stats.Expertise]}
	})
	resp, err := MeasureResponse(context.Background(), fake, dpsObjective(), Loadout{}, map[stats.Stat]ResponseRange{
		stats.Expertise: {Min: -50, Max: 50},
	}, 1000)
	if err != nil {
		t.Fatal(err)
	}
	exp := resp[stats.Expertise]
	if fake.calls != 1 || exp.Breakpoint != 0 || exp.SlopeBelow != exp.SlopeAbove || !near(exp.SlopeBelow, 1.5, 0.01) {
		t.Errorf("curve = %+v after %d calls, want one straight line and no bisection", exp, fake.calls)
	}
}

func TestMeasureResponseOneSidedRange(t *testing.T) {
	fake := newFakeEvaluator(func(p Point) Metrics {
		return Metrics{MetricDPS: 5000 + 4*p.Offset[stats.SpellPower]}
	})
	resp, err := MeasureResponse(context.Background(), fake, dpsObjective(), Loadout{}, map[stats.Stat]ResponseRange{
		stats.SpellPower: {Min: 0, Max: 80},
	}, 500)
	if err != nil {
		t.Fatal(err)
	}
	sp := resp[stats.SpellPower]
	if !near(sp.SlopeBelow, 4, 0.01) || !near(sp.SlopeAbove, 4, 0.01) || !near(sp.Value(-10), -40, 0.1) {
		t.Errorf("curve = %+v, want 4 on both sides", sp)
	}

	if _, err := MeasureResponse(context.Background(), fake, dpsObjective(), Loadout{}, map[stats.Stat]ResponseRange{
		stats.SpellPower: {Min: 10, Max: -10},
	}, 500); err == nil || !strings.Contains(err.Error(), "backwards") {
		t.Errorf("err = %v, want one about a backwards range", err)
	}
}

func TestResponseCurveValue(t *testing.T) {
	for _, tc := range []struct {
		bp, x, want float64
	}{
		{100, 50, 100},
		{100, 150, 200 + 25},
		{100, -20, -40},
		{-100, -150, -100 - 50},
		{-100, 40, 20},
		{-100, -40, 20 * -1},
	} {
		c := &ResponseCurve{Breakpoint: tc.bp, SlopeBelow: 2, SlopeAbove: 0.5}
		if got := c.Value(tc.x); !near(got, tc.want, 1e-9) {
			t.Errorf("break at %g: value(%g) = %g, want %g", tc.bp, tc.x, got, tc.want)
		}
		if got := c.Value(0); got != 0 {
			t.Errorf("break at %g: value at the seed = %g", tc.bp, got)
		}
	}
}
