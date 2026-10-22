package optimizer

import (
	"github.com/wowsims/wotlk/sim/core/proto"
	"github.com/wowsims/wotlk/sim/statcaps"
)

// requiredDefense is statcaps.RequiredDefense around the seed. It reads the seed's resilience and
// crit reductions, so every result's own sheet is rechecked afterwards rather than trusting the floor
// alone.
func requiredDefense(r *Request) (float64, error) {
	return statcaps.RequiredDefense(seedSheetAt(r))
}

// critImmunityFloor adds D* to the run's stat floors, so the pool, the surrogate's penalty and
// every rule check treat crit immunity like any other floor. It reports the floor it added.
func (r *run) critImmunityFloor() (float64, error) {
	defense, err := requiredDefense(r.r)
	if err != nil {
		return 0, err
	}
	settings := r.r.Settings
	// The run keeps one Defense floor, raised rather than joined by a second: the surrogate's
	// penalty adds a term per floor, so two of them price Defense twice.
	for _, floor := range settings.StatMinimums {
		if floor.Stat == proto.Stat_StatDefense {
			floor.MinValue = max(floor.MinValue, defense)
			return floor.MinValue, nil
		}
	}
	settings.StatMinimums = append(settings.StatMinimums, &proto.StatMinimum{Stat: proto.Stat_StatDefense, MinValue: defense})
	return defense, nil
}
