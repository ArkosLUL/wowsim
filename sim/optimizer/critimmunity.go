package optimizer

import (
	"errors"
	"fmt"
	"math"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
	"github.com/wowsims/wotlk/sim/core/stats"
	goproto "google.golang.org/protobuf/proto"
)

// playerSheet is the target's character sheet in l, with offset added to its bonus stats, plus the
// chance the encounter's boss crits it.
func playerSheet(base *proto.RaidSimRequest, targetIndex int, l Loadout, offset stats.Stats) (core.PlayerSheet, error) {
	rsr := goproto.Clone(base).(*proto.RaidSimRequest)
	player := rsr.Raid.Parties[targetIndex/5].Players[targetIndex%5]
	player.Equipment = l.Equipment()
	player.RacialTraits = l.RacialTraits
	if offset != (stats.Stats{}) {
		if player.BonusStats == nil {
			player.BonusStats = &proto.UnitStats{}
		}
		player.BonusStats.Stats = stats.FromFloatArray(player.BonusStats.Stats).Add(offset).ToFloatArray()
	}
	return core.ComputePlayerSheet(rsr.Raid, rsr.Encounter, targetIndex)
}

// maxDefenseBonus caps the bisection. 2000 rating is 406 defense skill, and 140 over the level cap's
// 400 is all the crit-taken formula asks of a tank with no defense at all.
const maxDefenseBonus = 2000

// requiredDefense is D*: the sheet Defense rating at which the encounter's boss stops critting the
// target. It bisects bonus Defense around the seed, since defense skill comes in whole points, and
// reads the resulting sheet rather than assuming a rating point is a sheet point.
//
// Everything else the crit formula reads (resilience, talent crit reductions, the boss's own crit)
// comes off the seed, so gear that moves those moves the real answer too. Hence every result's own
// sheet is rechecked afterwards rather than trusting the floor alone.
func requiredDefense(r *Request) (float64, error) {
	sheetAt := func(bonus float64) (core.PlayerSheet, error) {
		var offset stats.Stats
		offset[stats.Defense] = bonus
		return playerSheet(r.Base, r.TargetIndex, r.Seed, offset)
	}
	seed, err := sheetAt(0)
	if err != nil {
		return 0, err
	}
	if !seed.MeleeAttacker {
		return 0, errors.New("nothing in the encounter swings at the target, so there's no melee crit to be immune to")
	}
	immune := func(bonus int) (bool, error) {
		sheet, err := sheetAt(float64(bonus))
		return sheet.MeleeAttacker && sheet.MeleeCritTakenChance == 0, err
	}
	switch ok, err := immune(maxDefenseBonus); {
	case err != nil:
		return 0, err
	case !ok:
		return 0, fmt.Errorf("even %d more Defense rating leaves the boss a %.2f%% crit chance", maxDefenseBonus, seed.MeleeCritTakenChance*100)
	}
	// down as well as up: a seed that is already immune says nothing about how much of its defense
	// the search may trade away
	lo, hi := -int(math.Ceil(seed.FinalStats[stats.Defense])), maxDefenseBonus
	for lo < hi {
		// lo + (hi-lo)/2, not (lo+hi)/2: lo can be negative, and Go truncates toward zero
		mid := lo + (hi-lo)/2
		ok, err := immune(mid)
		if err != nil {
			return 0, err
		}
		if ok {
			hi = mid
		} else {
			lo = mid + 1
		}
	}
	final, err := sheetAt(float64(lo))
	if err != nil {
		return 0, err
	}
	return final.FinalStats[stats.Defense], nil
}

// critImmunityFloor adds D* to the run's stat floors, so the pool, the surrogate's penalty and
// every rule check treat crit immunity like any other floor. It reports the floor it added.
func (r *run) critImmunityFloor() (float64, error) {
	defense, err := requiredDefense(r.r)
	if err != nil {
		return 0, err
	}
	settings := r.r.Settings
	for _, floor := range settings.StatMinimums {
		if floor.Stat == proto.Stat_StatDefense && floor.MinValue >= defense {
			return floor.MinValue, nil
		}
	}
	settings.StatMinimums = append(settings.StatMinimums, &proto.StatMinimum{Stat: proto.Stat_StatDefense, MinValue: defense})
	return defense, nil
}
