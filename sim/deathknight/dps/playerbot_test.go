package dps

import (
	"slices"
	"testing"
	"time"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
)

// A bot keeps its presence and Horn of Winter from before the pull, never Army of the Dead. TestFrost's
// player, built by hand: simval's goldchar check reads any test calling FrostSuite as a golden suite.
func TestPlayerbotFrostLosesPrepullCooldowns(t *testing.T) {
	for _, playerbot := range []bool{false, true} {
		res := core.RunRaidSim(&proto.RaidSimRequest{
			Raid: core.SinglePlayerRaidProto(&proto.Player{
				Race:          proto.Race_RaceOrc,
				Class:         proto.Class_ClassDeathknight,
				Equipment:     core.GetGearSet("../../../"+gearSetsDir, "p3_frost").GearSet,
				TalentsString: FrostTalents,
				Glyphs:        FrostDefaultGlyphs,
				Consumes:      FullConsumes,
				Spec:          PlayerOptionsFrost,
				Rotation:      suiteRotation("../../..", "frost_bl_pesti").Rotation,
				Buffs:         core.FullIndividualBuffs,
				Playerbot:     playerbot,
			}, core.FullPartyBuffs, core.FullRaidBuffs, core.FullDebuffs),
			Encounter:  core.MakeSingleTargetEncounter(0),
			SimOptions: &proto.SimOptions{Iterations: 1, IsTest: true, Debug: true, RandomSeed: 101},
		})
		if res.ErrorResult != "" {
			t.Fatal(res.ErrorResult)
		}
		beforePull := func(id int32) int {
			times := core.CastTimes(res.Logs, core.ActionID{SpellID: id})
			return len(slices.DeleteFunc(times, func(at time.Duration) bool { return at >= 0 }))
		}

		if army := beforePull(42650); (army > 0) == playerbot {
			t.Errorf("playerbot %v: %d Army of the Dead casts before the pull", playerbot, army)
		}
		if horn := beforePull(57623); horn == 0 {
			t.Errorf("playerbot %v: no Horn of Winter before the pull", playerbot)
		}
		if blood := beforePull(48266); blood == 0 {
			t.Errorf("playerbot %v: no Blood Presence before the pull", playerbot)
		}
	}
}
