package dps

import (
	"testing"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
	"github.com/wowsims/wotlk/sim/core/stats"
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

// Guardian::UpdateMaxHealth gives a Bloodworm 1 a point of stamina over its create value, on the
// 437 that level 80's 5342 at the template's 0.0819 comes to.
func TestBloodwormHealthTakesOneAPointOfStamina(t *testing.T) {
	blood := parityEnv(t, parityPlayer(BloodTalents, "p3_blood", noRotation), nil)
	worm := blood.Bloodworm[0].Pet
	if got, want := worm.MaxHealth(), 437.0; got != want {
		t.Errorf("a Bloodworm has %v health, want %v", got, want)
	}
	if got := worm.ApplyStatDependencies(stats.Stats{stats.Stamina: 100})[stats.Health]; got != 100 {
		t.Errorf("100 stamina gives a Bloodworm %v health, want 100", got)
	}
}
