package core

import (
	"testing"

	"github.com/wowsims/wotlk/sim/core/proto"
	googleProto "google.golang.org/protobuf/proto"
)

// A target's mechanic immunities come from its creature_immunities mask. Left unset, a world boss gets the
// usual raid boss row and anything else none.
func TestTargetMechanicImmunities(t *testing.T) {
	off := false
	for _, tc := range []struct {
		name    string
		options *proto.Target
		immune  []Mechanic
		takes   []Mechanic
	}{
		{"raid boss, unset", &proto.Target{Level: 83}, []Mechanic{MechanicDisarm, MechanicRoot, MechanicSnare, MechanicStun}, nil},
		{"level 83 without the boss flag, unset", &proto.Target{Level: 83, WorldBoss: &off}, nil,
			[]Mechanic{MechanicDisarm, MechanicRoot, MechanicSnare, MechanicStun}},
		{"level 80 add, unset", &proto.Target{Level: 80}, nil, []Mechanic{MechanicDisarm, MechanicRoot, MechanicSnare, MechanicStun}},
		{"raid boss, no row", &proto.Target{Level: 83, MechanicImmuneMask: googleProto.Uint64(0)}, nil,
			[]Mechanic{MechanicDisarm, MechanicRoot, MechanicSnare, MechanicStun}},
		// creature_immunities -355, Gormok the Impaler on normal: the boss row without disarm
		{"raid boss, row -355", &proto.Target{Level: 83, MechanicImmuneMask: googleProto.Uint64(0x4D967EF6)},
			[]Mechanic{MechanicRoot, MechanicSnare, MechanicStun}, []Mechanic{MechanicDisarm}},
		{"level 80 add, snare immune", &proto.Target{Level: 80, MechanicImmuneMask: googleProto.Uint64(1 << MechanicSnare)},
			[]Mechanic{MechanicSnare}, []Mechanic{MechanicDisarm, MechanicRoot, MechanicStun}},
	} {
		target := NewTarget(tc.options, 0)
		for _, mechanic := range tc.immune {
			if !target.ImmuneToMechanic(mechanic) {
				t.Errorf("%s: takes mechanic %d, want immune", tc.name, mechanic)
			}
		}
		for _, mechanic := range tc.takes {
			if target.ImmuneToMechanic(mechanic) {
				t.Errorf("%s: immune to mechanic %d, want not", tc.name, mechanic)
			}
		}
	}
	// attack slows (8) and bleeds (15) land on a raid boss
	if RaidBossMechanicImmunities&(1<<8|1<<15) != 0 {
		t.Errorf("the raid boss row %#x holds attack slows or bleeds", RaidBossMechanicImmunities)
	}
}
