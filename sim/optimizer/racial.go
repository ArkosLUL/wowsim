package optimizer

import (
	"context"
	"errors"
	"math/rand"
	"slices"
	"time"

	"github.com/wowsims/wotlk/sim/core/proto"
)

// allRaces are the racial traits a character can wear, whatever it was born as: the server's
// mod-racial-trait-swap decouples traits from the base race, and core.NewCharacter takes
// Player.racial_traits over Player.race.
var allRaces = []proto.Race{
	proto.Race_RaceBloodElf,
	proto.Race_RaceDraenei,
	proto.Race_RaceDwarf,
	proto.Race_RaceGnome,
	proto.Race_RaceHuman,
	proto.Race_RaceNightElf,
	proto.Race_RaceOrc,
	proto.Race_RaceTauren,
	proto.Race_RaceTroll,
	proto.Race_RaceUndead,
}

// racialFinalists is how many races go on to the search after the screen; races tied with the last
// of them join too. maxRacialFinalists caps that: several races give a spec nothing at all, so they
// tie exactly, and every extra finalist takes verification sims away from the gear search.
const (
	racialFinalists    = 3
	maxRacialFinalists = 6
)

// screenRacials scores the seed gear under every race's traits at screening iterations and returns
// the finalists, best first. It fills in the run's racial_screen for the result.
//
// The screen runs on the seed rather than on each race's own best gear: the surrogate prices gear
// stats, and racials add flat stats and effects on top, so the gear ranking barely moves between
// races. The finalists get the real spend: each gets a gear search of its own (searchRaces), and
// every set it finds goes into verification, which sims them paired, so the sim picks the race, not
// the screen.
func (r *run) screenRacials() ([]proto.Race, error) {
	evals, err := r.evaluate(racialScreenPoints(r.r.Seed), r.screen)
	if err != nil {
		return nil, err
	}

	type scored struct {
		race proto.Race
		eval *Evaluation
		j    Estimate
	}
	var ranked []scored
	for i, race := range allRaces {
		if evals[i] == nil {
			r.warn("left %s traits out of the racial screen: their sim failed", race)
			continue
		}
		ranked = append(ranked, scored{race, evals[i], r.obj.Score(evals[i])})
	}
	if len(ranked) == 0 {
		return nil, errors.New("every race's screening sim failed")
	}
	slices.SortStableFunc(ranked, func(a, b scored) int {
		switch {
		case a.j.Mean > b.j.Mean:
			return -1
		case a.j.Mean < b.j.Mean:
			return 1
		}
		return 0
	})

	last := ranked[min(racialFinalists, len(ranked))-1]
	var finalists []proto.Race
	tied := 0
	for _, s := range ranked {
		d := r.obj.Delta(last.eval, s.eval)
		finalist := len(finalists) < racialFinalists || d.Mean+2*d.SE >= 0
		if finalist && len(finalists) >= maxRacialFinalists {
			finalist = false
			tied++
		}
		if finalist {
			finalists = append(finalists, s.race)
		}
		r.racialScreen = append(r.racialScreen, &proto.OptimizerRacialScreen{
			RacialTraits: s.race,
			Score:        s.j.Mean,
			ScoreSe:      s.j.SE,
			Finalist:     finalist,
		})
		r.trace("racial screen: %s scores %.1f ± %.1f, finalist %v", s.race, s.j.Mean, s.j.SE, finalist)
	}
	if tied > 0 {
		r.warn("%d more races tied with the last finalist, so the search only ran the best %d", tied, maxRacialFinalists)
	}
	return finalists, nil
}

// compareRaces holds r's target's gear exactly as given and scores only its racial traits, in
// paired sims through eval (the caller's raid evaluator). A switch needs the same 2-standard-error
// bar as any other pick here; nothing else about the request (its pool, warm starts, metric weights)
// is read. The raid batch's Heroic Presence passes are the caller, so it only weighs the races
// comparedRaces lists.
func compareRaces(ctx context.Context, r *Request, eval Evaluator, start time.Time) *proto.OptimizerResult {
	budget := EffortBudget(r.Settings.GetEffort())

	races := comparedRaces(r.Seed.RacialTraits)
	const current = 0
	points := make([]Point, len(races))
	for i, race := range races {
		l := r.Seed
		l.RacialTraits = race
		points[i] = Point{Loadout: l}
	}

	evals, err := eval.Evaluate(ctx, points, budget.Iterations)
	if err != nil {
		if ctx.Err() != nil {
			return cancelledBeforeStart(r, start)
		}
		return errorResult(err)
	}

	best, bestDelta := current, Estimate{}
	screen := make([]*proto.OptimizerRacialScreen, len(races))
	for i, race := range races {
		d := Delta(evals[current], evals[i], MetricDPS)
		screen[i] = &proto.OptimizerRacialScreen{RacialTraits: race, Score: evals[i].Metrics[MetricDPS].Mean, ScoreSe: evals[i].Metrics[MetricDPS].SE}
		if i != current && d.Mean > 2*d.SE && d.Mean > bestDelta.Mean {
			best, bestDelta = i, d
		}
	}
	screen[best].Finalist = true

	seedLoadout, bestLoadout := points[current].Loadout, points[best].Loadout
	improved := best != current
	bestResult := &proto.OptimizerLoadoutResult{Equipment: bestLoadout.Equipment(), RacialTraits: bestLoadout.RacialTraits}
	if improved {
		bestResult.RaidDpsDelta, bestResult.RaidDpsDeltaSe = bestDelta.Mean, bestDelta.SE
	}
	var totalSims int32
	for _, e := range evals {
		if e != nil {
			totalSims += int32(e.Iterations)
		}
	}
	return &proto.OptimizerResult{
		Best:            bestResult,
		Seed:            &proto.OptimizerLoadoutResult{Equipment: seedLoadout.Equipment(), RacialTraits: seedLoadout.RacialTraits},
		RacialScreen:    screen,
		Improved:        improved,
		Settings:        r.Settings,
		TargetRaidIndex: int32(r.TargetIndex),
		SimCommit:       SimCommit,
		CatalogDate:     r.Pool.GetCatalogDate(),
		TotalSims:       totalSims,
		ElapsedSeconds:  time.Since(start).Seconds(),
	}
}

// comparedRaces is current first, then what compareRaces weighs it against. Heroic Presence is the
// only racial that reaches anyone but the raider, so the raid only ever asks whether to wear it: a
// Draenei weighs every other race, anyone else just Draenei. Their own best race was stage 1's call.
func comparedRaces(current proto.Race) []proto.Race {
	if current != proto.Race_RaceDraenei {
		return []proto.Race{current, proto.Race_RaceDraenei}
	}
	races := []proto.Race{current}
	for _, race := range allRaces {
		if race != current {
			races = append(races, race)
		}
	}
	return races
}

// racialScreenPoints are the seed gear under every race's traits, in allRaces' order.
func racialScreenPoints(seed Loadout) []Point {
	points := make([]Point, len(allRaces))
	for i, race := range allRaces {
		l := seed
		l.RacialTraits = race
		points[i] = Point{Loadout: l}
	}
	return points
}

// searchRaces runs the gear search once per race the run pairs gear with: every finalist, or just the
// seed's traits when there's no screen. Each race's search prices the floors and ceilings around the
// seed worn with its own traits (forRace), so gear that leans on another race's hit or expertise
// never reaches verify under this one. The curves and residuals are base's, measured once at the seed.
// It records each race's surrogate in r.surrogates, base's own race included, and returns the
// candidates by rank across races (byRank).
func (r *run) searchRaces(base *surrogate) ([]Loadout, error) {
	races := r.finalists
	if len(races) == 0 {
		races = []proto.Race{base.seed.RacialTraits}
	}
	searchers := make([]*searcher, len(races))
	errs := make([]error, len(races))
	runParallel(len(races), len(races), func(i int) {
		s := base
		if races[i] != base.seed.RacialTraits {
			if s, errs[i] = base.forRace(races[i]); errs[i] != nil {
				return
			}
		}
		searchers[i] = newSearcher(s, r)
	})
	if err := firstError(errs); err != nil {
		return nil, err
	}
	r.surrogates = map[proto.Race]*surrogate{base.seed.RacialTraits: base}
	for i, se := range searchers {
		r.surrogates[races[i]] = se.s
		// drawn up front in race order, so concurrent searches can't change each other's numbers
		if len(races) > 1 {
			se.rng = rand.New(rand.NewSource(r.rng.Int63()))
		}
		if traceHook != nil {
			for slot := range se.opts {
				r.trace("%s, slot %s: %d options of %d priced, %d in the pool", races[i], proto.ItemSlot(slot), len(se.opts[slot]), len(se.all[slot]), len(r.pool.Slots[slot]))
			}
		}
	}
	found := make([][]Loadout, len(races))
	runParallel(len(races), len(races), func(i int) {
		found[i], errs[i] = searchers[i].search()
	})
	if err := firstError(errs); err != nil {
		return nil, err
	}
	return byRank(found), nil
}

func firstError(errs []error) error {
	for _, err := range errs {
		if err != nil {
			return err
		}
	}
	return nil
}

// byRank is every race's best candidate, then every race's second best, and so on, so trimming the
// list to verification's budget drops the weakest gear rather than one race's whole tail.
func byRank(lists [][]Loadout) []Loadout {
	var out []Loadout
	for rank := 0; ; rank++ {
		more := false
		for _, list := range lists {
			if rank < len(list) {
				out = append(out, list[rank])
				more = true
			}
		}
		if !more {
			return out
		}
	}
}
