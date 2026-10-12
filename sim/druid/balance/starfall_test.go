package balance

import (
	"fmt"
	"math"
	"testing"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
)

// omenFight is 12 s of a naked druid casting only castID, whose damage lands as hitID.
type omenFight struct {
	casts, castMisses int32
	hits, hitMisses   int32
	// Clearcasting procs, refreshes too
	procs float64
}

func runOmenFight(t *testing.T, castID, hitID int32) omenFight {
	t.Helper()
	encounter := core.MakeSingleTargetEncounter(0)
	encounter.Duration = 12
	return runOmenFightOn(t, encounter, castID, hitID)
}

func runOmenFightOn(t *testing.T, encounter *proto.Encounter, castID, hitID int32) omenFight {
	t.Helper()
	const iterations = 400
	res := core.RunRaidSim(&proto.RaidSimRequest{
		Raid: core.SinglePlayerRaidProto(&proto.Player{
			Race:     proto.Race_RaceTauren,
			Class:    proto.Class_ClassDruid,
			Spec:     PlayerOptionsAdaptive,
			Consumes: &proto.Consumes{},
			Rotation: core.APLRotationFromJsonString(fmt.Sprintf(
				`{"type": "TypeAPL", "priorityList": [{"action": {"castSpell": {"spellId": {"spellId": %d}}}}]}`, castID)),
			TalentsString: StandardTalents,
			// no gear, so no hit rating: a spell that rolls hit misses about one cast in eight
			Equipment: &proto.EquipmentSpec{},
		}, nil, nil, nil),
		Encounter:  encounter,
		SimOptions: &proto.SimOptions{Iterations: iterations, IsTest: true, RandomSeed: 101},
	})
	if res.ErrorResult != "" {
		t.Fatal(res.ErrorResult)
	}
	player := res.RaidMetrics.Parties[0].Players[0]
	var fight omenFight
	for _, action := range player.Actions {
		for _, target := range action.Targets {
			switch action.Id.GetSpellId() {
			case castID:
				fight.casts += target.Casts
				fight.castMisses += target.Misses
			case hitID:
				fight.hits += target.Casts
				fight.hitMisses += target.Misses
			}
		}
	}
	for _, aura := range player.Auras {
		if aura.Id.GetSpellId() == 16870 {
			fight.procs = aura.ProcsAvg * iterations
		}
	}
	return fight
}

// checkOmenRolls holds the Clearcasting procs to 8.75% (3.5 PPM over the 1.5 s floor) of the casts and the
// hits that landed, within five standard deviations.
func checkOmenRolls(t *testing.T, fight omenFight) {
	t.Helper()
	rolls := float64(fight.casts + fight.hits - fight.hitMisses)
	want := 0.0875 * rolls
	if tolerance := 5 * math.Sqrt(rolls*0.0875*0.9125); math.Abs(fight.procs-want) > tolerance {
		t.Errorf("%.0f Clearcasting procs from %d casts and %d landed hits, want %.0f give or take %.0f", fight.procs,
			fight.casts, fight.hits-fight.hitMisses, want, tolerance)
	}
	t.Logf("%d casts, %d hits landed, %.0f Clearcasting procs (%.4f a roll)", fight.casts, fight.hits-fight.hitMisses,
		fight.procs, fight.procs/rolls)
}

// Starfall's 53201 is an aura on the druid, so it never misses. Omen of Clarity rolls on the cast and on every
// star that lands: a live Starfall-only fight gave Clearcasting at 6 of 82 star hits and 1 of 7 casts.
func TestStarfallNeverMissesAndItsStarsRollOmen(t *testing.T) {
	fight := runOmenFight(t, 53201, 53195)
	if fight.casts == 0 || fight.castMisses != 0 {
		t.Errorf("%d Starfalls, %d missed", fight.casts, fight.castMisses)
	}
	if fight.hits < 8*fight.casts || fight.hitMisses == 0 {
		t.Fatalf("%d stars from %d Starfalls, %d missed", fight.hits, fight.casts, fight.hitMisses)
	}
	checkOmenRolls(t, fight)
}

// Hurricane's cast puts its aura on the druid, and that hit rolls Omen of Clarity like each tick that lands: a
// live Hurricane-only fight gave Clearcasting at 3 of 30 casts and 29 of 288 ticks.
func TestHurricaneCastAndTicksRollOmen(t *testing.T) {
	fight := runOmenFight(t, 48467, 48466)
	// the second channel is cut short by the fight's end
	if fight.casts == 0 || fight.hits < 5*fight.casts || fight.hitMisses == 0 {
		t.Fatalf("%d Hurricanes, %d ticks, %d missed", fight.casts, fight.hits, fight.hitMisses)
	}
	checkOmenRolls(t, fight)
}

// The server's dummy spell casts two stars a tick on random units in sight, 20 over the 10 s.
func TestStarfallCastsTwoStarsATickOnManyTargets(t *testing.T) {
	encounter := core.MakeSingleTargetEncounter(0)
	encounter.Duration = 12
	encounter.Targets = append(encounter.Targets, core.NewDefaultTarget(), core.NewDefaultTarget())
	fight := runOmenFightOn(t, encounter, 53201, 53195)
	if fight.casts == 0 || fight.hits != 20*fight.casts {
		t.Errorf("%d stars from %d Starfalls, want 20 each", fight.hits, fight.casts)
	}
}
