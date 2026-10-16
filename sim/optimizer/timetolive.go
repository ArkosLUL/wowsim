package optimizer

import (
	"errors"
	"fmt"
	"math"
	"slices"
	"strings"

	"github.com/wowsims/wotlk/sim/core/proto"
	goproto "google.golang.org/protobuf/proto"
)

// timeToLiveSeconds is how long the time-to-live fight runs, and so what a loadout that lives through
// it scores.
const timeToLiveSeconds = 60

// timeToLiveRequest is r's fight with the healing model off, from the pull to timeToLiveSeconds: how
// long the tank lasts on its own, cooldowns as its rotation casts them. The burst window and
// Inspiration stay, and core still tracks deaths because a healing model is there. Base is cloned:
// prefetches and calibrate read r's while this one is in use.
func timeToLiveRequest(r *Request) *Request {
	out := *r
	out.Base = goproto.Clone(r.Base).(*proto.RaidSimRequest)
	target := out.Target()
	healing := &proto.HealingModel{}
	if model := target.GetHealingModel(); model != nil {
		healing = goproto.Clone(model).(*proto.HealingModel)
	}
	healing.Hps, healing.CadenceSeconds, healing.CadenceVariation = 0, 0, 0
	target.HealingModel = healing
	if out.Base.Encounter == nil {
		out.Base.Encounter = &proto.Encounter{}
	}
	out.Base.Encounter.Duration = timeToLiveSeconds
	out.Base.Encounter.DurationVariation = 0
	out.Base.Encounter.UseHealth = false
	return &out
}

// timeToLiveSource is an Evaluator that can also sim the time-to-live fight.
type timeToLiveSource interface {
	timeToLiveEvaluator(r *Request) Evaluator
}

func (e *SimEvaluator) timeToLiveEvaluator(r *Request) Evaluator {
	return NewSimEvaluator(timeToLiveRequest(r))
}

// tankRun is a run whose target tanks with a healing model and is scored on surviving.
func (r *run) tankRun() bool {
	if r.obj == nil || r.r.Target().GetHealingModel() == nil {
		return false
	}
	for m, w := range r.obj.Weights {
		if armorReferenced[m] && w > 0 {
			return true
		}
	}
	return false
}

// timeToLive sims each loadout in the time-to-live fight at the run's iterations, paired with each
// other, and returns how long each lives.
func (r *run) timeToLive(loadouts []Loadout) ([]Estimate, error) {
	if r.lives == nil {
		src, ok := r.eval.inner.(timeToLiveSource)
		if !ok {
			return nil, errors.New("this run's evaluator can't sim the time-to-live fight")
		}
		r.lives = &countingEvaluator{inner: src.timeToLiveEvaluator(r.r), have: map[Point]int{}}
	}
	evals, err := r.lives.Evaluate(r.ctx, pointsOf(loadouts), r.budget.Iterations)
	if err != nil {
		return nil, err
	}
	out := make([]Estimate, len(evals))
	for i, e := range evals {
		if e.TimeToLive == (Estimate{}) {
			return nil, errors.New("the time-to-live fight tracked no deaths for the target")
		}
		out[i] = e.TimeToLive
	}
	return out, nil
}

// livesSimmed counts the time-to-live fight's iterations.
func (r *run) livesSimmed() int64 {
	if r.lives == nil {
		return 0
	}
	return r.lives.simmed()
}

// simmed is everything the run simmed.
func (r *run) simmed() int64 {
	return r.eval.simmed() + r.livesSimmed()
}

// breakTie hands a tank run's tie on score to time to live: the verified loadouts within 2 standard
// errors of the best that also beat the seed go to the time-to-live fight, and the longest-lived of
// them goes first. Trading a little Stamina for a little avoidance moves the score less than its
// noise; lasting longer unhealed tells such loadouts apart. Only verified[0] moves.
func (r *run) breakTie(verified []*verifiedLoadout) {
	if len(verified) < 2 || !r.tankRun() || !r.beatsSeed(verified[0].eval) {
		return
	}
	best := verified[0]
	tied := []*verifiedLoadout{best}
	for _, v := range verified[1:] {
		if d := r.obj.Delta(best.eval, v.eval); d.Mean+2*d.SE >= 0 && r.beatsSeed(v.eval) {
			tied = append(tied, v)
		}
	}
	if len(tied) < 2 {
		return
	}
	loadouts := make([]Loadout, len(tied))
	for i, v := range tied {
		loadouts[i] = v.loadout
	}
	lives, err := r.timeToLive(loadouts)
	if err != nil {
		if r.ctx.Err() == nil {
			r.warn("%d loadouts tied on score and time to live couldn't pick between them, so the best score stands: %v", len(tied), err)
		}
		return
	}
	longest := 0
	for i, life := range lives {
		if life.Mean > lives[longest].Mean {
			longest = i
		}
	}
	parts := make([]string, len(tied))
	for i, v := range tied {
		d := r.obj.Delta(r.seedEval, v.eval)
		parts[i] = fmt.Sprintf("%.1f ± %.1f s at score %+.1f ± %.1f", lives[i].Mean, lives[i].SE, d.Mean, d.SE)
		if i == longest {
			parts[i] += " (the pick)"
		}
	}
	r.warn("%d loadouts tied on score, within 2 standard errors of the best, so the one that lives longest with no healing from the pull (out of %d s) stands: %s",
		len(tied), timeToLiveSeconds, strings.Join(parts, ", "))
	r.trace("time to live: %v", lives)
	if longest > 0 {
		i := slices.Index(verified, tied[longest])
		winner := verified[i]
		copy(verified[1:i+1], verified[:i])
		verified[0] = winner
	}
}

// checkNaN fails a loadout whose metrics or score came out NaN: one NaN in a seed or reference eval
// turns every score paired with it into junk (a hunter with no ranged weapon sims NaN DPS).
func (r *run) checkNaN(what string, l Loadout, e *Evaluation) error {
	if e == nil {
		return nil
	}
	for m, est := range e.Metrics {
		if math.IsNaN(est.Mean) {
			return fmt.Errorf("%s%s sims %s as NaN, so nothing can be scored against it", what, emptySlots(l), metricName(Metric(m)))
		}
	}
	if r.obj != nil && math.IsNaN(r.obj.Score(e).Mean) {
		return fmt.Errorf("%s%s scores NaN, so nothing can be scored against it", what, emptySlots(l))
	}
	return nil
}

// emptySlots names l's empty slots, for an error about it.
func emptySlots(l Loadout) string {
	var empty []string
	for slot, c := range l.Items {
		if c.ItemID == 0 {
			empty = append(empty, slotLabel(proto.ItemSlot(slot)))
		}
	}
	if len(empty) == 0 {
		return ""
	}
	return fmt.Sprintf(" (empty: %s)", strings.Join(empty, ", "))
}
