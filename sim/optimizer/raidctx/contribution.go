package raidctx

import "github.com/wowsims/wotlk/sim/core/proto"

// RaidDPS is the raid's total damage per second for one sim, per iteration: every player's own DPS,
// added up by core itself (Raid.GetMetrics), so it needs SaveAllValues set the way a single player's
// DPS does.
//
// It's what a full-raid evaluator scores a DPS raider's gear against for raid contribution. A paired
// sim of two of the target's loadouts replays every other raider's random numbers identically (see
// SimEvaluator.request), so only the target's own damage and whatever its gear changes for the rest
// of the raid - the Demonic Pact spell power it gives a warlock, the Heroic Presence hit its party
// gets - move a paired delta of this value. Everyone else's output cancels out.
func RaidDPS(metrics *proto.RaidMetrics) []float64 {
	return metrics.GetDps().GetAllValues()
}
