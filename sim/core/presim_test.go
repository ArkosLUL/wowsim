package core_test

import (
	"testing"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
	googleProto "google.golang.org/protobuf/proto"
)

// A raider in group 8 has raid index 35, and a healing model without HPS runs a presim for them.
func TestPresimForARaiderInGroup8(t *testing.T) {
	rsr := shardTestRaid(t, 20)
	var rogue *proto.Player
	for _, party := range rsr.Raid.Parties {
		for _, player := range party.Players {
			if player.Name == "Combat" {
				rogue = player
			}
		}
	}
	raider := googleProto.Clone(rogue).(*proto.Player)
	raider.Name = "Group8"
	raider.HealingModel = &proto.HealingModel{CadenceSeconds: 2}
	for len(rsr.Raid.Parties) < 8 {
		rsr.Raid.Parties = append(rsr.Raid.Parties, &proto.Party{})
	}
	rsr.Raid.Parties[7].Players = []*proto.Player{raider}
	rsr.Raid.NumActiveParties = 8

	result := core.RunRaidSim(rsr)
	if result.ErrorResult != "" {
		t.Fatal(result.ErrorResult)
	}
	if dps := result.RaidMetrics.Parties[7].Players[0].GetDps().GetAvg(); dps <= 0 {
		t.Errorf("the group 8 raider did %v DPS, want some", dps)
	}
}
