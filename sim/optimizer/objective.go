package optimizer

import (
	"context"
	"errors"
	"fmt"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
	"github.com/wowsims/wotlk/sim/core/stats"
)

// Objective is the score J the search maximizes: the sum over metrics of r[m] * M[m] / w[m]. w[m] is
// how much one point of the metric's reference stat moves it at the seed, which puts every term in
// reference-stat points, so the weights r[m] trade like for like. w is negative for DTPS, TMI and
// death chance (more Armor lowers them), so lowering those raises J.
type Objective struct {
	// r[m] as run: the settings' weights, or DPS alone when they're all 0.
	Weights Metrics
	// What each w[m] is measured against: the spec's EP reference stat for DPS, HPS and TPS, Armor
	// for the rest.
	ReferenceStats [NumMetrics]stats.Stat
	// w[m] per point of its reference stat, measured at the seed and frozen there.
	Normalizers [NumMetrics]Estimate
	// Weighted metrics J had to leave out, and why.
	Warnings []string

	coef Metrics
}

// Lower is better for these, so their normalizers must come out negative.
var lowerIsBetter = [NumMetrics]bool{MetricDTPS: true, MetricTMI: true, MetricPDeath: true}

// objectiveWeights is where raid mode (J = raid DPS) plugs in.
func objectiveWeights(settings *proto.OptimizerSettings) (Metrics, error) {
	var weights Metrics
	switch settings.GetObjective() {
	case proto.OptimizerObjective_OptimizerObjectiveOwnMetrics:
		weights = MetricsFromProto(settings.GetMetricWeights())
	case proto.OptimizerObjective_OptimizerObjectiveRaidDps:
		return weights, errors.New("the raid DPS objective isn't implemented yet")
	default:
		return weights, fmt.Errorf("unknown objective %v", settings.GetObjective())
	}
	for m, r := range weights {
		if r < 0 {
			return weights, fmt.Errorf("metric weight %d is negative (%g); weights are importance, not signs", m, r)
		}
	}
	if weights == (Metrics{}) {
		weights[MetricDPS] = 1
	}
	return weights, nil
}

// WeightedMetrics lists the metrics J will use, for NewSimEvaluator to keep samples of.
func WeightedMetrics(settings *proto.OptimizerSettings) ([]Metric, error) {
	weights, err := objectiveWeights(settings)
	if err != nil {
		return nil, err
	}
	var out []Metric
	for m, r := range weights {
		if r > 0 {
			out = append(out, Metric(m))
		}
	}
	return out, nil
}

// referenceStat is the spec's EP reference stat, as epReferenceStat in each ui/<spec>/sim.ts.
func referenceStat(player *proto.Player) stats.Stat {
	switch player.GetSpec().(type) {
	case *proto.Player_Hunter:
		return stats.RangedAttackPower
	case *proto.Player_Warrior, *proto.Player_ProtectionWarrior, *proto.Player_Rogue,
		*proto.Player_FeralDruid, *proto.Player_FeralTankDruid, *proto.Player_RetributionPaladin,
		*proto.Player_EnhancementShaman, *proto.Player_Deathknight, *proto.Player_TankDeathknight:
		return stats.AttackPower
	default:
		return stats.SpellPower
	}
}

// normalizerOffset is how far each side of the seed the normalizer sims move the reference stat.
// Armor moves DTPS far less per point than AP or SP move DPS, so it gets more.
func normalizerOffset(s stats.Stat) float64 {
	if s == stats.Armor {
		return 1000
	}
	return 100
}

// NewObjective measures the normalizers: paired sims of the seed with each reference stat moved
// down and up. A weighted metric whose normalizer doesn't clear 2 standard errors with the right
// sign is left out, with a warning; if that leaves nothing, it's an error.
func NewObjective(ctx context.Context, eval Evaluator, r *Request, iterations int) (*Objective, error) {
	weights, err := objectiveWeights(r.Settings)
	if err != nil {
		return nil, err
	}
	o := &Objective{Weights: weights}
	ref := referenceStat(r.Target())
	for m := range o.ReferenceStats {
		o.ReferenceStats[m] = ref
		if lowerIsBetter[m] {
			o.ReferenceStats[m] = core.DTPSReferenceStat
		}
	}

	var points []Point
	pointIndex := map[stats.Stat]int{}
	for m, w := range weights {
		s := o.ReferenceStats[m]
		if _, ok := pointIndex[s]; w == 0 || ok {
			continue
		}
		pointIndex[s] = len(points)
		var down, up stats.Stats
		down[s], up[s] = -normalizerOffset(s), normalizerOffset(s)
		points = append(points, Point{Loadout: r.Seed, Offset: down}, Point{Loadout: r.Seed, Offset: up})
	}
	evals, err := eval.Evaluate(ctx, points, iterations)
	if err != nil {
		return nil, fmt.Errorf("normalizer sims: %w", err)
	}

	for m, w := range weights {
		if w == 0 {
			continue
		}
		s := o.ReferenceStats[m]
		i := pointIndex[s]
		d := Delta(evals[i], evals[i+1], Metric(m))
		span := 2 * normalizerOffset(s)
		norm := Estimate{Mean: d.Mean / span, SE: d.SE / span}
		o.Normalizers[m] = norm
		signed := norm.Mean
		if lowerIsBetter[m] {
			signed = -signed
		}
		if signed <= 2*norm.SE {
			o.Warnings = append(o.Warnings, fmt.Sprintf("left %s out of the objective: %g %s doesn't move it measurably at the seed (%g ± %g per point)",
				metricName(Metric(m)), normalizerOffset(s), s.StatName(), norm.Mean, norm.SE))
			continue
		}
		o.coef[m] = w / norm.Mean
	}
	if o.coef == (Metrics{}) {
		return nil, fmt.Errorf("no weighted metric responds to its reference stat at the seed: %v", o.Warnings)
	}
	return o, nil
}

// Score is J for one evaluation.
func (o *Objective) Score(e *Evaluation) Estimate {
	return combinedScore(e, o.coef)
}

// Delta is J(b) - J(a), paired.
func (o *Objective) Delta(a, b *Evaluation) Estimate {
	return combinedDelta(a, b, o.coef)
}

func metricName(m Metric) string {
	switch m {
	case MetricDPS:
		return "DPS"
	case MetricHPS:
		return "HPS"
	case MetricTPS:
		return "TPS"
	case MetricDTPS:
		return "DTPS"
	case MetricTMI:
		return "TMI"
	case MetricPDeath:
		return "death chance"
	}
	return fmt.Sprintf("metric %d", m)
}
