package dps

import (
	"math"
	"testing"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
)

func tundraStalkerSim(t *testing.T, talents string, debuffs *proto.Debuffs) (*core.Simulation, *DpsDeathknight) {
	t.Helper()
	player := parityPlayer(talents, "p3_frost", noRotation)
	sim := core.NewSim(&proto.RaidSimRequest{
		Raid:       core.SinglePlayerRaidProto(player, &proto.PartyBuffs{}, &proto.RaidBuffs{}, debuffs),
		Encounter:  core.MakeSingleTargetEncounter(0),
		SimOptions: &proto.SimOptions{Iterations: 1, IsTest: true, RandomSeed: 1},
	})
	sim.Reset()
	sim.PrePull()
	for sim.CurrentTime < 0 && !sim.Step() {
	}
	return sim, sim.Raid.Parties[0].Players[0].(*DpsDeathknight)
}

// Tundra Stalker is aura 112 (7277) with a class mask per rank: flags[1] of ranks 2 and 3 (0x2C030126, 0x2C130126)
// lacks Blood Plague's 0x02000000, so only ranks 1, 4 and 5 raise its ticks.
func TestTundraStalkerRanksOnBloodPlague(t *testing.T) {
	for rank := 1; rank <= 5; rank++ {
		sim, dk := tundraStalkerSim(t, talentsWith(FrostTalents, 1, 27, byte('0'+rank)), nil)
		if int(dk.Talents.TundraStalker) != rank {
			t.Fatalf("Tundra Stalker %d, want %d", dk.Talents.TundraStalker, rank)
		}
		target := sim.GetTargetUnit(0)
		dk.FrostFeverSpell.Dot(target).Apply(sim)
		dot := dk.BloodPlagueSpell.Dot(target)
		dot.Apply(sim)

		want := 1 + 0.03*float64(rank)
		if rank == 2 || rank == 3 {
			want = 1
		}
		got := dot.SnapshotAttackerMultiplier / dk.BloodPlagueSpell.AttackerDamageMultiplier(dk.AttackTables[target.UnitIndex])
		if math.Abs(got-want) > 1e-9 {
			t.Errorf("rank %d: Blood Plague takes %v from Tundra Stalker, want %v", rank, got, want)
		}
	}
}

// The script asks the target for anyone's Frost Fever (HasAura(55095)): the raid's Frost Fever debuff, another
// knight's, counts. The knight's own debuff part, up without his disease, doesn't.
func TestTundraStalkerCountsTheRaidsFrostFever(t *testing.T) {
	sim, dk := tundraStalkerSim(t, FrostTalents, &proto.Debuffs{FrostFever: proto.TristateEffect_TristateEffectRegular})
	target := sim.GetTargetUnit(0)
	if dk.FrostFeverSpell.Dot(target).IsActive() {
		t.Fatal("the knight's own Frost Fever is up")
	}
	if got := dk.RoRTSBonus(target); math.Abs(got-1.15) > 1e-9 {
		t.Errorf("bonus %v under the raid's Frost Fever, want 1.15", got)
	}

	sim, dk = tundraStalkerSim(t, FrostTalents, nil)
	target = sim.GetTargetUnit(0)
	if got := dk.RoRTSBonus(target); got != 1 {
		t.Errorf("bonus %v with no Frost Fever, want 1", got)
	}
	dk.FrostFeverSpell.Cast(sim, target)
	if got := dk.RoRTSBonus(target); math.Abs(got-1.15) > 1e-9 {
		t.Errorf("bonus %v under his own Frost Fever, want 1.15", got)
	}
	dk.FrostFeverSpell.Dot(target).Deactivate(sim)
	if got := dk.RoRTSBonus(target); got != 1 {
		t.Errorf("bonus %v once his Frost Fever is gone, want 1", got)
	}
}
