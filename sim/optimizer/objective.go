package optimizer

import (
	"context"
	"fmt"
	"slices"

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
	// The Warnings entry that left death chance out, or "". A tank run searches again once it knows
	// whether the first pick brings the term back, so execute holds that entry back.
	DeathWarning string

	coef Metrics
}

// deathMinSE is how many standard errors the death normalizer has to clear before J counts death
// chance: the coefficient is a weight over that normalizer, so a barely measurable one lets death
// chance swamp the score.
const deathMinSE = 10.0

// deathMeasure is the death chance a pair of sims with the reference stat moved down and up came to.
type deathMeasure struct {
	// Per point of Armor, as measured: negative when more Armor means fewer deaths.
	Norm     Estimate
	Down, Up float64
	// Chance of death at the loadout itself.
	Dies float64
	// Armor on the sheet the pair was taken around; 0 where it couldn't be read.
	Armor float64
}

// measureDeath is NewObjective's death normalizer for the loadout at: paired sims of it with the Armor
// offsets the objective uses.
func measureDeath(ctx context.Context, eval Evaluator, r *Request, at Loadout, iterations int) (deathMeasure, error) {
	var down, up stats.Stats
	down[stats.Armor], up[stats.Armor] = -normalizerOffset(stats.Armor), normalizerOffset(stats.Armor)
	evals, err := eval.Evaluate(ctx, []Point{{Loadout: at, Offset: down}, {Loadout: at, Offset: up}}, iterations)
	if err != nil {
		return deathMeasure{}, err
	}
	d := Delta(evals[0], evals[1], MetricPDeath)
	span := 2 * normalizerOffset(stats.Armor)
	dm := deathMeasure{
		Norm: Estimate{Mean: d.Mean / span, SE: d.SE / span},
		Down: evals[0].Metrics[MetricPDeath].Mean,
		Up:   evals[1].Metrics[MetricPDeath].Mean,
	}
	dm.Dies = (dm.Down + dm.Up) / 2
	if sheet, err := playerSheet(r.Base, r.TargetIndex, at, stats.Stats{}); err == nil {
		dm.Armor = sheet.FinalStats[stats.Armor]
	}
	return dm, nil
}

// deathCoefficient is the death term's coefficient for a weight, or 0 and why it stays out. The
// normalizer has to clear deathMinSE with the right sign. The coefficient is the weight over a
// per-point slope, so a small slope makes it huge: death chance is capped at what dying could cost in
// J's points, the Armor the gear wears, however little a point of Armor moves it.
func deathCoefficient(weight float64, dm deathMeasure) (coef float64, left string) {
	norm := dm.Norm
	signed := -norm.Mean
	switch {
	case dm.Down == dm.Up && (dm.Down == 0 || dm.Down == 1):
		// no offset or iteration count measures this one: one hit outsizes the health bar, or none comes close
		return 0, fmt.Sprintf("dies in %.0f%% of this fight's iterations, %g Armor either way, so nothing there measures it",
			dm.Dies*100, normalizerOffset(stats.Armor))
	case signed <= 2*norm.SE:
		return 0, fmt.Sprintf("dies in %.0f%% of this fight's iterations, and %g Armor doesn't move that measurably (%g ± %g per point)",
			dm.Dies*100, normalizerOffset(stats.Armor), norm.Mean, norm.SE)
	case signed < deathMinSE*norm.SE:
		return 0, fmt.Sprintf("dies in %.0f%% of this fight's iterations, and %g Armor moves that only %.1f standard errors (%g ± %g per point), short of the %g it takes to price it",
			dm.Dies*100, normalizerOffset(stats.Armor), signed/norm.SE, norm.Mean, norm.SE, deathMinSE)
	}
	coef = weight / norm.Mean
	if dm.Armor > 0 && -coef > dm.Armor {
		coef = -dm.Armor
	}
	return coef, ""
}

// judgeDeath sets death chance's coefficient from dm, or leaves it out with a warning.
func (o *Objective) judgeDeath(dm deathMeasure) {
	o.Normalizers[MetricPDeath] = dm.Norm
	coef, left := deathCoefficient(o.Weights[MetricPDeath], dm)
	o.coef[MetricPDeath] = coef
	if left != "" {
		o.DeathWarning = "left death chance out of the objective: the gear as equipped " + left
		o.Warnings = append(o.Warnings, o.DeathWarning)
	}
}

// countDeathAt prices death chance from the normalizer at dm, taken at the first search's pick, in
// place of the gear as equipped's, which failed the bar and dropped the term.
func (o *Objective) countDeathAt(dm deathMeasure) {
	o.Warnings = slices.DeleteFunc(o.Warnings, func(w string) bool { return w == o.DeathWarning })
	o.DeathWarning = ""
	o.judgeDeath(dm)
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
		// NewRaidEvaluator reports the raid's total DPS as MetricDPS, so weighing it alone makes J the
		// raid's DPS (scaled by its normalizer, like every other objective).
		weights[MetricDPS] = 1
	default:
		return weights, fmt.Errorf("unknown objective %v", settings.GetObjective())
	}
	for m, r := range weights {
		if r < 0 {
			return weights, fmt.Errorf("metric weight %d is negative (%g); weights are importance, not signs", m, r)
		}
	}
	survival := settings.GetTankSurvival()
	if survival < 0 || survival > 1 {
		return weights, fmt.Errorf("tank_survival is %g; it runs from 0 (all threat) to 1 (all survival)", survival)
	}
	if weights == (Metrics{}) {
		// a slider at 0 reads as unset, so an all-threat run has to send metric_weights
		if survival > 0 {
			weights = TankWeights(survival)
		} else {
			weights[MetricDPS] = 1
		}
	}
	return weights, nil
}

// TankWeights turns the survival/threat slider into metric weights. Survival splits evenly between
// the damage the tank takes, how spiky it is and how often it dies, threat goes to TPS; the normalizers
// then put all of them in the same units, so the slider trades one against the other.
//
// The UI sends metric_weights as well, and those win: this is what a request that only carries the
// slider - the CLI, an export replayed by hand - runs.
func TankWeights(survival float64) Metrics {
	survival = min(1, max(0, survival))
	var weights Metrics
	weights[MetricDTPS] = survival / 3
	weights[MetricTMI] = survival / 3
	weights[MetricPDeath] = survival / 3
	weights[MetricTPS] = 1 - survival
	return weights
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
		return 2000
	}
	return 100
}

// NewObjective measures the normalizers: paired sims of the gear as equipped with each reference
// stat moved down and up, so a slot the trimmer emptied (a weapon, most sharply: about 12% on
// BIS-seed's DK case) doesn't size the objective's points against gear the player doesn't have. A
// weighted metric whose normalizer doesn't clear 2 standard errors with the right sign is left out,
// with a warning; if that leaves nothing, it's an error.
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

	normalizerPoints := func(base Loadout) ([]Point, map[stats.Stat]int) {
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
			points = append(points, Point{Loadout: base, Offset: down}, Point{Loadout: base, Offset: up})
		}
		return points, pointIndex
	}

	base := r.Equipped
	points, pointIndex := normalizerPoints(base)
	evals, err := eval.Evaluate(ctx, points, iterations)
	if err != nil && base != r.Seed {
		o.Warnings = append(o.Warnings, fmt.Sprintf("measured the objective's normalizers against the trimmed seed instead of the gear as equipped, which failed to sim: %v", err))
		base = r.Seed
		points, pointIndex = normalizerPoints(base)
		evals, err = eval.Evaluate(ctx, points, iterations)
	}
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
		if Metric(m) == MetricPDeath {
			o.judgeDeath(deathMeasure{Norm: norm, Down: evals[i].Metrics[m].Mean, Up: evals[i+1].Metrics[m].Mean, Dies: (evals[i].Metrics[m].Mean + evals[i+1].Metrics[m].Mean) / 2, Armor: o.armorOf(r, base)})
			continue
		}
		signed := norm.Mean
		if lowerIsBetter[m] {
			signed = -signed
		}
		if signed <= 2*norm.SE {
			o.Warnings = append(o.Warnings, fmt.Sprintf("left %s out of the objective: %g %s doesn't move it measurably there (%g ± %g per point)",
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

// armorOf is the Armor on the sheet of the loadout, 0 where it can't be read.
func (o *Objective) armorOf(r *Request, l Loadout) (armor float64) {
	defer func() {
		// a request the sheet can't build, a test's stand-in
		if recover() != nil {
			armor = 0
		}
	}()
	sheet, err := playerSheet(r.Base, r.TargetIndex, l, stats.Stats{})
	if err != nil {
		return 0
	}
	return sheet.FinalStats[stats.Armor]
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
