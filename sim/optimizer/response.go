package optimizer

import (
	"context"
	"fmt"
	"math"
	"slices"

	"github.com/wowsims/wotlk/sim/core/stats"
)

// capProneStats get five offsets and a bisected breakpoint instead of one offset each side of the
// seed. ArP is here though the sheet knows its cap: Blood Gorged adds ArP to some spells only, so the
// real breakpoint moves per spell and only sims find it.
var capProneStats = map[stats.Stat]bool{
	stats.MeleeHit:         true,
	stats.SpellHit:         true,
	stats.Expertise:        true,
	stats.ArmorPenetration: true,
	stats.Defense:          true,
}

const (
	capProneOffsets = 5
	// Each step halves the bracket around a breakpoint: five take it to 1/64 of the range.
	bisectionSteps = 5
	// Standard errors a slope change needs to count as a breakpoint rather than noise.
	breakpointZ = 3
)

// ResponseRange is the offsets from the seed a curve has to cover, usually what the pool can reach.
type ResponseRange struct {
	Min, Max float64
}

// Knot is one simmed offset and J's paired change from the seed there.
type Knot struct {
	Offset float64
	Delta  Estimate
}

// ResponseCurve is J's change from the seed as one stat moves by an offset: two lines through the
// seed that meet at Breakpoint, carried on past the measured range. Stats that aren't cap-prone
// break at the seed, so their two slopes just pick up some curvature.
type ResponseCurve struct {
	Stat       stats.Stat
	Breakpoint float64
	SlopeBelow float64
	SlopeAbove float64
	// Every simmed offset, sorted, the seed's included.
	Knots []Knot
}

func (c *ResponseCurve) Value(offset float64) float64 {
	return c.SlopeBelow*hingeBelow(offset, c.Breakpoint) + c.SlopeAbove*hingeAbove(offset, c.Breakpoint)
}

// The hinge basis: both are 0 at the seed and meet at bp.
func hingeBelow(x, bp float64) float64 { return min(x, bp) - min(0, bp) }
func hingeAbove(x, bp float64) float64 { return max(x, bp) - max(0, bp) }

// Response is one curve per measured stat. Adding them up assumes stats act separately; the
// residuals catch what doesn't.
type Response map[stats.Stat]*ResponseCurve

// Value predicts J's change for gear stats moved by offset from the seed. Stats without a curve
// count as 0.
func (r Response) Value(offset stats.Stats) float64 {
	total := 0.0
	// stat order, so the float sum comes out the same every run
	for s := stats.Stat(0); s < stats.Len; s++ {
		if c := r[s]; c != nil && offset[s] != 0 {
			total += c.Value(offset[s])
		}
	}
	return total
}

// MeasureResponse measures a curve per stat over its range, with paired sims of the seed plus
// offsets. Every stat's sims go to the evaluator together, so its workers stay busy.
func MeasureResponse(ctx context.Context, eval Evaluator, obj *Objective, seed Loadout, ranges map[stats.Stat]ResponseRange, iterations int) (Response, error) {
	var order []stats.Stat
	for s, rng := range ranges {
		if rng.Min > rng.Max {
			return nil, fmt.Errorf("%s range runs backwards, %g to %g", s.StatName(), rng.Min, rng.Max)
		}
		if rng.Min != 0 || rng.Max != 0 {
			order = append(order, s)
		}
	}
	slices.Sort(order)

	type owner struct {
		stat   stats.Stat
		offset float64
	}
	points := []Point{{Loadout: seed}}
	var owners []owner
	for _, s := range order {
		for _, off := range initialOffsets(s, ranges[s]) {
			points = append(points, offsetPoint(seed, s, off))
			owners = append(owners, owner{s, off})
		}
	}
	evals, err := eval.Evaluate(ctx, points, iterations)
	if err != nil {
		return nil, fmt.Errorf("response sims: %w", err)
	}
	base := evals[0]
	knots := make(map[stats.Stat][]Knot, len(order))
	for _, s := range order {
		knots[s] = []Knot{{Offset: 0}}
	}
	for i, o := range owners {
		knots[o.stat] = append(knots[o.stat], Knot{Offset: o.offset, Delta: obj.Delta(base, evals[i+1])})
	}
	for _, s := range order {
		sortKnots(knots[s])
	}

	brackets := make(map[stats.Stat]*bracket)
	for _, s := range order {
		if !capProneStats[s] {
			continue
		}
		if b := findBreakpoint(knots[s]); b != nil {
			brackets[s] = b
		}
	}
	for step := 0; step < bisectionSteps && len(brackets) > 0; step++ {
		var stepStats []stats.Stat
		var stepPoints []Point
		for _, s := range order {
			if b := brackets[s]; b != nil {
				stepStats = append(stepStats, s)
				stepPoints = append(stepPoints, offsetPoint(seed, s, b.mid()))
			}
		}
		evals, err := eval.Evaluate(ctx, stepPoints, iterations)
		if err != nil {
			return nil, fmt.Errorf("breakpoint sims: %w", err)
		}
		for i, s := range stepStats {
			k := Knot{Offset: brackets[s].mid(), Delta: obj.Delta(base, evals[i])}
			knots[s] = append(knots[s], k)
			sortKnots(knots[s])
			brackets[s].narrow(k)
		}
	}

	resp := make(Response, len(order))
	for _, s := range order {
		c := &ResponseCurve{Stat: s, Knots: knots[s]}
		switch b := brackets[s]; {
		case b != nil:
			c.Breakpoint = b.estimate()
			c.SlopeBelow, c.SlopeAbove = fitHinge(knots[s], c.Breakpoint)
		case capProneStats[s]:
			// no breakpoint in range, so one line fits all five offsets best
			slope := fitLine(knots[s])
			c.SlopeBelow, c.SlopeAbove = slope, slope
		default:
			c.SlopeBelow, c.SlopeAbove = fitHinge(knots[s], 0)
		}
		resp[s] = c
	}
	return resp, nil
}

// initialOffsets never includes 0: the seed is simmed once for every stat.
func initialOffsets(s stats.Stat, rng ResponseRange) []float64 {
	var offsets []float64
	if capProneStats[s] && rng.Max > rng.Min {
		step := (rng.Max - rng.Min) / (capProneOffsets - 1)
		for i := 0; i < capProneOffsets; i++ {
			offsets = append(offsets, rng.Min+float64(i)*step)
		}
	} else {
		offsets = []float64{rng.Min, rng.Max}
	}
	out := offsets[:0]
	for _, off := range offsets {
		if off != 0 && !slices.Contains(out, off) {
			out = append(out, off)
		}
	}
	return out
}

func offsetPoint(seed Loadout, s stats.Stat, offset float64) Point {
	p := Point{Loadout: seed}
	p.Offset[s] = offset
	return p
}

func sortKnots(knots []Knot) {
	slices.SortFunc(knots, func(a, b Knot) int {
		switch {
		case a.Offset < b.Offset:
			return -1
		case a.Offset > b.Offset:
			return 1
		}
		return 0
	})
}

// bracket holds a breakpoint between lo and hi, with a line of slope left through lo and one of
// slope right through hi. A side only gets a slope from a segment wholly on that side of the
// breakpoint: a segment across it would put the knot in the middle on both lines.
type bracket struct {
	lo, hi            Knot
	left, right       float64
	hasLeft, hasRight bool
}

func (b *bracket) mid() float64 {
	return (b.lo.Offset + b.hi.Offset) / 2
}

func (b *bracket) leftLine(x float64) float64 {
	return b.lo.Delta.Mean + b.left*(x-b.lo.Offset)
}

func (b *bracket) rightLine(x float64) float64 {
	return b.hi.Delta.Mean + b.right*(x-b.hi.Offset)
}

// narrow keeps the half the breakpoint is in: a knot on the left line means it's above that knot.
// With one line only, a knot off it by more than the noise is on the other side. findBreakpoint
// always leaves at least one.
func (b *bracket) narrow(k Knot) {
	y := k.Delta.Mean
	var onLeft bool
	switch {
	case b.hasLeft && b.hasRight:
		onLeft = math.Abs(y-b.leftLine(k.Offset)) <= math.Abs(y-b.rightLine(k.Offset))
	case b.hasRight:
		onLeft = math.Abs(y-b.rightLine(k.Offset)) > lineTolerance(k, b.hi)
	default:
		onLeft = math.Abs(y-b.leftLine(k.Offset)) <= lineTolerance(k, b.lo)
	}
	// lo to k, or k to hi, is now a clean segment
	if onLeft {
		if !b.hasLeft {
			b.left, b.hasLeft = segmentSlope(b.lo, k), true
		}
		b.lo = k
	} else {
		if !b.hasRight {
			b.right, b.hasRight = segmentSlope(k, b.hi), true
		}
		b.hi = k
	}
}

// lineTolerance is how far off a line a knot can sit and still count as on it: two standard errors,
// with a floor for noiseless deltas.
func lineTolerance(k, anchor Knot) float64 {
	return max(2*math.Hypot(k.Delta.SE, anchor.Delta.SE), 1e-9*(1+math.Abs(k.Delta.Mean)))
}

func segmentSlope(a, b Knot) float64 {
	return (b.Delta.Mean - a.Delta.Mean) / (b.Offset - a.Offset)
}

// estimate is where the two lines cross, or the middle when they don't cross inside the bracket.
func (b *bracket) estimate() float64 {
	if b.hasLeft && b.hasRight && b.left != b.right {
		x := (b.hi.Delta.Mean - b.lo.Delta.Mean + b.left*b.lo.Offset - b.right*b.hi.Offset) / (b.left - b.right)
		if x >= b.lo.Offset && x <= b.hi.Offset {
			return x
		}
	}
	return b.mid()
}

// findBreakpoint brackets the knot where the slope changes the most, or returns nil when no change
// clears breakpointZ standard errors.
func findBreakpoint(knots []Knot) *bracket {
	n := len(knots)
	// with 4 knots or more, at least one side of any bracket has a clean segment for narrow
	if n < 4 {
		return nil
	}
	slopes := make([]Estimate, n-1)
	for i := range slopes {
		dx := knots[i+1].Offset - knots[i].Offset
		// as if independent, which overstates the noise: both knots share the seed's
		se := math.Hypot(knots[i].Delta.SE, knots[i+1].Delta.SE)
		slopes[i] = Estimate{Mean: (knots[i+1].Delta.Mean - knots[i].Delta.Mean) / dx, SE: se / dx}
	}
	best, bestZ := -1, 0.0
	for j := 1; j < n-1; j++ {
		change := math.Abs(slopes[j].Mean - slopes[j-1].Mean)
		scale := max(math.Abs(slopes[j].Mean), math.Abs(slopes[j-1].Mean))
		// the floor keeps float noise on a straight noiseless line from reading as a break
		noise := max(math.Hypot(slopes[j].SE, slopes[j-1].SE), 1e-9*scale)
		if change == 0 || noise == 0 {
			continue
		}
		if z := change / noise; z > bestZ {
			best, bestZ = j, z
		}
	}
	if best < 0 || bestZ < breakpointZ {
		return nil
	}
	b := &bracket{lo: knots[best-1], hi: knots[best+1]}
	if best >= 2 {
		b.left, b.hasLeft = slopes[best-2].Mean, true
	}
	if best+1 < len(slopes) {
		b.right, b.hasRight = slopes[best+1].Mean, true
	}
	b.narrow(knots[best])
	return b
}

// fitHinge least-squares fits the slopes of two lines through the seed that meet at bp. With no
// knots on one side, both get the single best slope.
func fitHinge(knots []Knot, bp float64) (below, above float64) {
	var s11, s12, s22, s1y, s2y float64
	for _, k := range knots {
		f1, f2 := hingeBelow(k.Offset, bp), hingeAbove(k.Offset, bp)
		s11 += f1 * f1
		s12 += f1 * f2
		s22 += f2 * f2
		s1y += f1 * k.Delta.Mean
		s2y += f2 * k.Delta.Mean
	}
	det := s11*s22 - s12*s12
	if s11 == 0 || s22 == 0 || det <= 1e-9*s11*s22 {
		slope := fitLine(knots)
		return slope, slope
	}
	return (s22*s1y - s12*s2y) / det, (s11*s2y - s12*s1y) / det
}

// fitLine least-squares fits one line through the seed.
func fitLine(knots []Knot) float64 {
	var sxx, sxy float64
	for _, k := range knots {
		sxx += k.Offset * k.Offset
		sxy += k.Offset * k.Delta.Mean
	}
	if sxx == 0 {
		return 0
	}
	return sxy / sxx
}
