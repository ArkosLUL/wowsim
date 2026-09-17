package core

import (
	"math"
	"slices"

	"github.com/wowsims/wotlk/sim/core/proto"
	"github.com/wowsims/wotlk/sim/core/stats"
)

// Server defaults from mod_reforging.conf: Reforging.Percentage and Reforging.ReforgeableStats.
const ReforgePercentage = 0.4

var ReforgeableStatTypes = []int32{6, 13, 14, 31, 32, 36, 37}

// ItemModType -> sim stats. Hit, crit and haste ratings count for both melee and spell, same as the
// item DB stores them.
var reforgeStatTypeToStats = map[int32][]stats.Stat{
	6:  {stats.Spirit},
	13: {stats.Dodge},
	14: {stats.Parry},
	31: {stats.MeleeHit, stats.SpellHit},
	32: {stats.MeleeCrit, stats.SpellCrit},
	36: {stats.MeleeHaste, stats.SpellHaste},
	37: {stats.Expertise},
}

// ReforgeStats returns the stat change a reforge makes on an item with the given base stats, or zero
// stats when the server wouldn't allow it.
func ReforgeStats(base stats.Stats, reforge *proto.ItemReforge) stats.Stats {
	var delta stats.Stats
	if reforge == nil || !ReforgeableStatPair(reforge.FromStatType, reforge.ToStatType) {
		return delta
	}
	fromStats := reforgeStatTypeToStats[reforge.FromStatType]
	toStats := reforgeStatTypeToStats[reforge.ToStatType]

	// server refuses a target stat the item already has
	if maxStatValue(base, toStats) > 0 {
		return delta
	}
	amount := math.Floor(ReforgePercentage * maxStatValue(base, fromStats))
	if amount < 1 {
		return delta
	}

	for _, stat := range fromStats {
		delta[stat] -= amount
	}
	for _, stat := range toStats {
		delta[stat] += amount
	}
	return delta
}

// ReforgeableStatPair says whether the server's reforgeable stat list allows this pair of
// ItemModTypes at all. It's the half of the rule that doesn't need the item; ReforgeStats applies the
// rest.
func ReforgeableStatPair(fromStatType, toStatType int32) bool {
	return fromStatType != toStatType &&
		slices.Contains(ReforgeableStatTypes, fromStatType) && slices.Contains(ReforgeableStatTypes, toStatType)
}

// CanReforge says whether the server would let this reforge sit on an item with these base stats.
// It's the same rule ReforgeStats applies, so callers that only need the yes/no can't drift from it.
func CanReforge(base stats.Stats, reforge *proto.ItemReforge) bool {
	return ReforgeStats(base, reforge) != stats.Stats{}
}

func maxStatValue(s stats.Stats, statList []stats.Stat) float64 {
	value := 0.0
	for _, stat := range statList {
		value = max(value, s[stat])
	}
	return value
}
