package dps

import (
	"math"
	"testing"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
	"github.com/wowsims/wotlk/sim/core/serverdata"
	"github.com/wowsims/wotlk/sim/core/stats"
	"github.com/wowsims/wotlk/sim/warrior"
)

// armsTalentsWith is ArmsTalents with the Arms talent at index (0-based) set to points.
func armsTalentsWith(t *testing.T, index int, points byte) string {
	t.Helper()
	talents := []byte(ArmsTalents)
	if talents[index] < '1' {
		t.Fatalf("ArmsTalents has no points at %d", index)
	}
	talents[index] = '0' + points
	return string(talents)
}

const (
	armsEndlessRage  = 27
	armsWreckingCrew = 29
)

func idleArmsWarrior(t *testing.T, talents string) (*core.Simulation, *DpsWarrior) {
	t.Helper()
	sim := core.NewSim(&proto.RaidSimRequest{
		Raid: core.SinglePlayerRaidProto(&proto.Player{
			Race:          proto.Race_RaceOrc,
			Class:         proto.Class_ClassWarrior,
			TalentsString: talents,
			Equipment:     core.GetGearSet("../../../ui/warrior/gear_sets", "p1_arms").GearSet,
			Consumes:      &proto.Consumes{},
			Spec:          PlayerOptionsArms,
			Rotation:      core.APLRotationFromJsonString(`{"type": "TypeAPL", "priorityList": []}`),
		}, &proto.PartyBuffs{}, &proto.RaidBuffs{}, &proto.Debuffs{}),
		Encounter:  core.MakeSingleTargetEncounter(0),
		SimOptions: &proto.SimOptions{Iterations: 1, IsTest: true, RandomSeed: 101},
	})
	sim.Reset()
	return sim, sim.Raid.Parties[0].Players[0].(*DpsWarrior)
}

// Endless Rage's aura adds to the rating's percent, both under one cap.
func TestEndlessRageArmorPenetration(t *testing.T) {
	_, with := idleArmsWarrior(t, ArmsTalents)
	_, without := idleArmsWarrior(t, armsTalentsWith(t, armsEndlessRage, 0))
	if !with.Talents.EndlessRage || without.Talents.EndlessRage {
		t.Fatalf("Endless Rage: %v and %v", with.Talents.EndlessRage, without.Talents.EndlessRage)
	}
	if got := with.PseudoStats.BonusArmorPenPct - without.PseudoStats.BonusArmorPenPct; got != warrior.EndlessRageArmorPenPct {
		t.Errorf("Endless Rage adds %v%% armor penetration, want %v%%", got, warrior.EndlessRageArmorPenPct)
	}

	perPct := without.PseudoStats.ArmorPenRatingPerPercent
	for _, ratingPct := range []float64{0, 30, 90} {
		rating := ratingPct * perPct
		want := math.Min(ratingPct+without.PseudoStats.BonusArmorPenPct+warrior.EndlessRageArmorPenPct, 100) / 100
		if got := with.ArmorPenetrationPercentage(rating); math.Abs(got-want) > 1e-9 {
			t.Errorf("%v%% from rating: %v of the cap ignored, want %v", ratingPct, got, want)
		}
	}
}

// Wrecking Crew's Enrage: physical damage only, by its rank's percent. serverdata only carries rank 1 (57518).
func TestWreckingCrewEnrage(t *testing.T) {
	if s := serverdata.SpellByID(57518); s == nil || float64(s.Effects[0].BasePoints+1) != warrior.WreckingCrewEnragePct[1] {
		t.Errorf("rank 1 is %v%%, but the server's 57518 isn't", warrior.WreckingCrewEnragePct[1])
	}
	for rank := byte(1); rank <= 5; rank++ {
		sim, war := idleArmsWarrior(t, armsTalentsWith(t, armsWreckingCrew, rank))
		enrage := war.GetAuraByID(core.ActionID{SpellID: 57518})
		if enrage == nil {
			t.Fatalf("rank %d: no Enrage", rank)
		}
		physical := war.PseudoStats.SchoolDamageDealtMultiplier[stats.SchoolIndexPhysical]
		fire := war.PseudoStats.SchoolDamageDealtMultiplier[stats.SchoolIndexFire]
		enrage.Activate(sim)
		want := 1 + warrior.WreckingCrewEnragePct[rank]/100
		if got := war.PseudoStats.SchoolDamageDealtMultiplier[stats.SchoolIndexPhysical] / physical; math.Abs(got-want) > 1e-9 {
			t.Errorf("rank %d: physical damage x%v, want x%v", rank, got, want)
		}
		if got := war.PseudoStats.SchoolDamageDealtMultiplier[stats.SchoolIndexFire]; got != fire {
			t.Errorf("rank %d: fire damage x%v, was x%v", rank, got, fire)
		}
		enrage.Deactivate(sim)
		if got := war.PseudoStats.SchoolDamageDealtMultiplier[stats.SchoolIndexPhysical]; math.Abs(got-physical) > 1e-12 {
			t.Errorf("rank %d: physical damage x%v after the Enrage, was x%v", rank, got, physical)
		}
	}
}
