package core

import "github.com/wowsims/wotlk/sim/core/proto"

// RunRaidSimShards lets the raid tests in core_test pick the shard count.
func RunRaidSimShards(rsr *proto.RaidSimRequest, progress chan *proto.ProgressMetrics, shards int) *proto.RaidSimResult {
	return runRaidSimShards(rsr, progress, shards)
}
