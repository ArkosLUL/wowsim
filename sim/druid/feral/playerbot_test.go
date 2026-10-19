package feral

import (
	"testing"
	"time"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
)

// The default APL's pre-pull Berserk goes; the rotation action's own Berserk in the fight stays.
func TestPlayerbotFeralLosesPrepullBerserk(t *testing.T) {
	for _, playerbot := range []bool{false, true} {
		res := core.RunRaidSim(&proto.RaidSimRequest{
			Raid: core.SinglePlayerRaidProto(&proto.Player{
				Race:            proto.Race_RaceTauren,
				Class:           proto.Class_ClassDruid,
				Equipment:       core.GetGearSet("../../../ui/feral_druid/gear_sets", "p1").GearSet,
				Consumes:        FullConsumes,
				Spec:            PlayerOptionsMonoCat,
				Buffs:           core.FullIndividualBuffs,
				Rotation:        core.GetAplRotation("../../../ui/feral_druid/apls", "default").Rotation,
				Glyphs:          StandardGlyphs,
				TalentsString:   StandardTalents,
				InFrontOfTarget: true,
				Playerbot:       playerbot,
			}, core.FullPartyBuffs, core.FullRaidBuffs, core.FullDebuffs),
			Encounter:  core.MakeSingleTargetEncounter(0),
			SimOptions: &proto.SimOptions{Iterations: 1, IsTest: true, Debug: true, RandomSeed: 101},
		})
		if res.ErrorResult != "" {
			t.Fatal(res.ErrorResult)
		}
		berserks := core.CastTimes(res.Logs, core.ActionID{SpellID: 50334})
		if len(berserks) == 0 || (berserks[0] == -time.Second) == playerbot {
			t.Errorf("playerbot %v: Berserk at %v", playerbot, berserks)
		}
	}
}
