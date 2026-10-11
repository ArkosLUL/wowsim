package dps

import (
	"testing"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
)

// The gargoyle (SummonProperties 209), the army's ghouls (687) and Raise Dead's guardian (829) have
// neither a pet's type nor its category, so no party or raid cast lands on them (Guardian::Guardian,
// Spell.cpp's TARGET_CHECK_RAID). Master of Ghouls' ghoul is a real pet and takes them all.
func TestDeathKnightGuardiansHaveNoPetBar(t *testing.T) {
	for _, tc := range []struct {
		name         string
		talents      string
		spec         *proto.Player_Deathknight
		petGhoul     bool
		withGargoyle bool
	}{
		{"Unholy", UnholyTalents, PlayerOptionsUnholy, true, true},
		{"Frost", FrostTalents, PlayerOptionsFrost, false, false},
	} {
		sim := core.NewSim(&proto.RaidSimRequest{
			Raid: core.SinglePlayerRaidProto(&proto.Player{
				Race:          proto.Race_RaceOrc,
				Class:         proto.Class_ClassDeathknight,
				Equipment:     &proto.EquipmentSpec{},
				Consumes:      &proto.Consumes{},
				TalentsString: tc.talents,
				Spec:          tc.spec,
				Rotation:      core.APLRotationFromJsonString(`{"type": "TypeAPL", "priorityList": []}`),
			}, nil, nil, nil),
			Encounter:  core.MakeSingleTargetEncounter(0),
			SimOptions: &proto.SimOptions{Iterations: 1, IsTest: true, RandomSeed: 101},
		})
		dk := sim.Raid.Parties[0].Players[0].(*DpsDeathknight)

		if dk.Ghoul == nil || dk.Ghoul.IsGuardian() == tc.petGhoul {
			t.Fatalf("%s: the ghoul is a guardian %v, want a pet %v", tc.name, dk.Ghoul != nil && dk.Ghoul.IsGuardian(), tc.petGhoul)
		}
		if got := dk.Ghoul.Uncontrollable; got == tc.petGhoul {
			t.Errorf("%s: the ghoul has no pet bar %v, with a pet ghoul %v", tc.name, got, tc.petGhoul)
		}
		for i, ghoul := range dk.ArmyGhoul {
			if !ghoul.Uncontrollable {
				t.Errorf("%s: army ghoul %d has a pet bar", tc.name, i)
			}
		}
		if (dk.Gargoyle != nil) != tc.withGargoyle {
			t.Fatalf("%s: a gargoyle %v, want %v", tc.name, dk.Gargoyle != nil, tc.withGargoyle)
		}
		if dk.Gargoyle != nil && !dk.Gargoyle.Uncontrollable {
			t.Errorf("%s: the gargoyle has a pet bar", tc.name)
		}
	}
}
