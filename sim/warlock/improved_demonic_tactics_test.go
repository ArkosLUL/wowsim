package warlock

import (
	"math"
	"testing"
	"time"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
	"github.com/wowsims/wotlk/sim/core/stats"
)

// Improved Demonic Tactics (54349) hands the demon three tenths of the warlock's sheet crit, spell for
// spell and melee for melee, each in whole percent of a whole percent, and only reworks it every 10 s
// (spell_warl_improved_demonic_tactics).
func TestImprovedDemonicTacticsTicksEveryTenSeconds(t *testing.T) {
	sim := core.NewSim(&proto.RaidSimRequest{
		Raid: core.SinglePlayerRaidProto(&proto.Player{
			Race:          proto.Race_RaceOrc,
			Class:         proto.Class_ClassWarlock,
			Equipment:     core.GetGearSet("../../ui/warlock/gear_sets", "p4_demo").GearSet,
			Consumes:      &proto.Consumes{},
			TalentsString: ImprovedDemonicTacticsTalents,
			Spec:          DefaultDemonologyWarlock,
			Rotation:      core.APLRotationFromJsonString(`{"type": "TypeAPL", "priorityList": []}`),
		}, nil, nil, nil),
		Encounter:  core.MakeSingleTargetEncounter(0),
		SimOptions: &proto.SimOptions{Iterations: 1, IsTest: true, RandomSeed: 101},
	})
	sim.Reset()
	warlock := sim.Raid.Parties[0].Players[0].(*Warlock)
	pet := warlock.Pet
	if warlock.Talents.ImprovedDemonicTactics != 3 || pet == nil {
		t.Fatalf("Improved Demonic Tactics %d with a demon %v: the test needs 3 points and one out", warlock.Talents.ImprovedDemonicTactics, pet != nil)
	}

	const perPct = core.CritRatingPerCritChance
	share := func(ownerRating float64) float64 { return math.Trunc(math.Trunc(ownerRating/perPct)*30/100) * perPct }
	// a creature's 5% and Demonic Tactics' 2% a point
	own := (5 + 2*float64(warlock.Talents.DemonicTactics)) * perPct
	check := func(when string, spell, melee float64) {
		t.Helper()
		if got, want := pet.GetStat(stats.SpellCrit), own+share(spell); math.Abs(got-want) > 1e-6 {
			t.Errorf("%s: the demon's spell crit rating %v, want %v", when, got, want)
		}
		if got, want := pet.GetStat(stats.MeleeCrit), own+share(melee); math.Abs(got-want) > 1e-6 {
			t.Errorf("%s: the demon's melee crit rating %v, want %v", when, got, want)
		}
	}

	spell, melee := warlock.GetStat(stats.SpellCrit), warlock.GetStat(stats.MeleeCrit)
	if math.Trunc(spell/perPct) == math.Trunc(melee/perPct) || share(spell) == 0 {
		t.Fatalf("the warlock crits %v%% with spells and %v%% in melee: the test needs two shares", spell/perPct, melee/perPct)
	}
	check("at the pull", spell, melee)

	// 10% more of both lands within 10 s, at the aura's next tick
	sim.PrePull()
	stepTo := func(until time.Duration) {
		for sim.CurrentTime < until && !sim.Step() {
		}
	}
	stepTo(time.Second)
	gained := sim.CurrentTime
	warlock.AddStatsDynamic(sim, stats.Stats{stats.SpellCrit: 10 * perPct, stats.MeleeCrit: 10 * perPct})
	check("as the warlock gains crit", spell, melee)

	before := pet.GetStat(stats.SpellCrit)
	for sim.CurrentTime < gained+11*time.Second && pet.GetStat(stats.SpellCrit) == before && !sim.Step() {
	}
	if waited := sim.CurrentTime - gained; waited <= 0 || waited > 10*time.Second+10*time.Millisecond {
		t.Errorf("the demon's crit moved %v after the warlock's, want within the aura's 10 s", waited)
	}
	check("after the aura's tick", spell+10*perPct, melee+10*perPct)

	// a demon that comes back gets what the aura holds
	pet.Disable(sim)
	if got := pet.GetStat(stats.SpellCrit); math.Abs(got-own) > 1e-6 {
		t.Errorf("a demon that left keeps %v spell crit rating, want its own %v", got, own)
	}
	pet.Enable(sim, pet)
	if got, want := pet.GetStat(stats.SpellCrit), own+share(spell+10*perPct); math.Abs(got-want) > 1e-6 {
		t.Errorf("a demon that came back has %v spell crit rating, want %v", got, want)
	}
}

// Guardian::UpdateMaxHealth gives each demon its own health a point of stamina over the create value, on
// pet_levelstats' health.
func TestDemonHealth(t *testing.T) {
	for _, tc := range []struct {
		summon                proto.Warlock_Options_Summon
		create, sta, perPoint float64
	}{
		{proto.Warlock_Options_Imp, 3867, 118, 8.4},
		{proto.Warlock_Options_Felguard, 5428, 328, 11},
		{proto.Warlock_Options_Succubus, 4567, 328, 9.1},
		{proto.Warlock_Options_Felhunter, 4787, 328, 9.5},
	} {
		sim := core.NewSim(&proto.RaidSimRequest{
			Raid: core.SinglePlayerRaidProto(&proto.Player{
				Race:      proto.Race_RaceOrc,
				Class:     proto.Class_ClassWarlock,
				Equipment: core.GetGearSet("../../ui/warlock/gear_sets", "p4_demo").GearSet,
				Consumes:  &proto.Consumes{},
				Spec: &proto.Player_Warlock{Warlock: &proto.Warlock{
					Options: &proto.Warlock_Options{Armor: proto.Warlock_Options_FelArmor, Summon: tc.summon},
				}},
				Rotation: core.APLRotationFromJsonString(`{"type": "TypeAPL", "priorityList": []}`),
			}, nil, nil, nil),
			Encounter:  core.MakeSingleTargetEncounter(0),
			SimOptions: &proto.SimOptions{Iterations: 1, IsTest: true, RandomSeed: 101},
		})
		sim.Reset()
		pet := sim.Raid.Parties[0].Players[0].(*Warlock).Pet
		// no Blood Pact, whose health comes on top
		if pact := pet.GetAuraByID(core.ActionID{SpellID: 47982}); pact.IsActive() {
			pact.Deactivate(sim)
		}
		stamina := pet.GetStat(stats.Stamina)
		if stamina <= tc.sta {
			t.Fatalf("%s: %v stamina, no more than its create %v", tc.summon, stamina, tc.sta)
		}
		if got, want := pet.MaxHealth(), tc.create+(stamina-tc.sta)*tc.perPoint; math.Abs(got-want) > 1e-6 {
			t.Errorf("%s at %v stamina: %v health, want %v", tc.summon, stamina, got, want)
		}
	}
}
