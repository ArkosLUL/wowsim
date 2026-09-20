package core

import (
	"fmt"

	"github.com/wowsims/wotlk/sim/core/proto"
	"github.com/wowsims/wotlk/sim/core/stats"
)

// PlayerSheet is one raider's character sheet plus what the encounter's bosses can do to them.
type PlayerSheet struct {
	// Final stats with gear, buffs and talents, as ComputeStats reports them to the UI.
	FinalStats stats.Stats
	// The worst chance any enemy has to crit them with a melee swing; 0 means crit immune.
	MeleeCritTakenChance float64
	// False when nothing in the encounter swings at them, which leaves the crit chance unknown
	// rather than zero. Targets only swing when a tank is assigned to them.
	MeleeAttacker bool
}

// ComputePlayerSheet builds the raid and encounter the way ComputeStats does, then reads one
// player's sheet out of it. The crit chance comes from the enemy's own auto attack against that
// player, so it carries defense, resilience and every crit-taken reduction the sim models, rather
// than assuming a fixed 540 defense.
func ComputePlayerSheet(raidProto *proto.Raid, encounterProto *proto.Encounter, raidIndex int) (sheet PlayerSheet, err error) {
	defer func() {
		if e := recover(); e != nil {
			err = fmt.Errorf("computing raid index %d's sheet: %v", raidIndex, e)
		}
	}()
	if encounterProto == nil {
		encounterProto = &proto.Encounter{}
	}
	env, raidStats, _ := NewEnvironment(raidProto, encounterProto, true)

	parties := raidStats.GetParties()
	party, slot := raidIndex/5, raidIndex%5
	if raidIndex < 0 || party >= len(parties) || slot >= len(parties[party].GetPlayers()) {
		return sheet, fmt.Errorf("no stats for raid index %d", raidIndex)
	}
	sheet.FinalStats = stats.FromFloatArray(parties[party].GetPlayers()[slot].GetFinalStats().GetStats())

	var defender *Unit
	for _, unit := range env.Raid.AllPlayerUnits {
		if unit.Index == int32(raidIndex) {
			defender = unit
		}
	}
	if defender == nil {
		return sheet, fmt.Errorf("raid index %d is an empty slot", raidIndex)
	}
	for _, target := range env.Encounter.Targets {
		auto := target.AutoAttacks.MHAuto()
		// CurrentTarget is the tank the encounter points this one at; everyone else is never swung at
		if auto == nil || target.CurrentTarget != defender || int(defender.UnitIndex) >= len(target.AttackTables) {
			continue
		}
		chance := auto.enemyCritChance(target.AttackTables[defender.UnitIndex])
		if !sheet.MeleeAttacker || chance > sheet.MeleeCritTakenChance {
			sheet.MeleeCritTakenChance = chance
		}
		sheet.MeleeAttacker = true
	}
	return sheet, nil
}
