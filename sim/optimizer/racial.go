package optimizer

import (
	"errors"
	"slices"

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
// races. The finalists get the real spend: every one of their sets goes into verification, which sims
// them paired, so the sim picks the race, not the screen.
func (r *run) screenRacials() ([]proto.Race, error) {
	points := make([]Point, len(allRaces))
	for i, race := range allRaces {
		l := r.r.Seed
		l.RacialTraits = race
		points[i] = Point{Loadout: l}
	}
	evals, err := r.evaluate(points, r.screen)
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

// withRacialTraits pairs every gear set the search found with every finalist race. A set's variants
// stay together, so trimming the list to verification's budget drops the weakest gear rather than
// one race's whole tail.
func withRacialTraits(candidates []Loadout, races []proto.Race) []Loadout {
	if len(races) == 0 {
		return candidates
	}
	out := make([]Loadout, 0, len(candidates)*len(races))
	for _, l := range candidates {
		for _, race := range races {
			l.RacialTraits = race
			out = append(out, l)
		}
	}
	return out
}
