package warlock

import (
	"testing"
	"time"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
	googleProto "google.golang.org/protobuf/proto"
)

// Wrath of Air's raid area aura reaches a warlock's Imp, and Firebolt is a magic-class spell, so the
// server cuts its cast to int32(ms * 100 / 105) (Unit::ModSpellCastTime).
func TestImpFireboltUnderWrathOfAir(t *testing.T) {
	castTime := func(wrathOfAir bool) time.Duration {
		options := googleProto.Clone(DefaultAfflictionWarlock.Warlock.Options).(*proto.Warlock_Options)
		options.Summon = proto.Warlock_Options_Imp
		sim := core.NewSim(&proto.RaidSimRequest{
			Raid: core.SinglePlayerRaidProto(&proto.Player{
				Race:          proto.Race_RaceOrc,
				Class:         proto.Class_ClassWarlock,
				Equipment:     core.GetGearSet("../../ui/warlock/gear_sets", "p4_affliction").GearSet,
				Consumes:      &proto.Consumes{},
				Spec:          &proto.Player_Warlock{Warlock: &proto.Warlock{Options: options}},
				Rotation:      core.APLRotationFromJsonString(`{"type": "TypeAPL", "priorityList": []}`),
				TalentsString: "-00",
			}, &proto.PartyBuffs{}, &proto.RaidBuffs{WrathOfAirTotem: wrathOfAir}, &proto.Debuffs{}),
			Encounter:  core.MakeSingleTargetEncounter(0),
			SimOptions: &proto.SimOptions{Iterations: 1, IsTest: true, RandomSeed: 101},
		})
		sim.Reset()
		warlock := sim.Raid.Parties[0].Players[0].(*Warlock)
		if warlock.Talents.DemonicPower != 0 {
			t.Fatalf("Demonic Power %d: the test wants Firebolt's own 2.5 s", warlock.Talents.DemonicPower)
		}
		return warlock.Pet.primaryAbility.CastTime()
	}
	if got := castTime(false); got != 2500*time.Millisecond {
		t.Errorf("Firebolt takes %v without Wrath of Air, want 2.5 s", got)
	}
	if got := castTime(true); got != 2380*time.Millisecond {
		t.Errorf("Firebolt takes %v under Wrath of Air, want 2380 ms", got)
	}
}
