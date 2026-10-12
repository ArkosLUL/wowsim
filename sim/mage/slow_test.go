package mage

import (
	"math"
	"testing"
	"time"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
)

// ArcaneTalents with the point in Slow.
const slowTalents = "23000513310033015032310251532-03-023303001"

// Slow snares an add for Torment the Weak and sits on one target at a time. The snare-immune boss takes only
// its other two effects, the cast and ranged slows, and gives Torment the Weak nothing.
func TestSlowSnaresWhatTakesSnares(t *testing.T) {
	encounter := core.MakeSingleTargetEncounter(0)
	encounter.Targets = append(encounter.Targets, &proto.Target{Level: 80, MobType: proto.MobType_MobTypeHumanoid})
	sim := core.NewSim(&proto.RaidSimRequest{
		Raid: core.SinglePlayerRaidProto(&proto.Player{
			Race:          proto.Race_RaceTroll,
			Class:         proto.Class_ClassMage,
			Equipment:     core.GetGearSet("../../ui/mage/gear_sets", "p3_arcane_alliance").GearSet,
			Consumes:      &proto.Consumes{},
			Spec:          PlayerOptionsArcane,
			Rotation:      core.APLRotationFromJsonString(`{"type": "TypeAPL", "priorityList": []}`),
			TalentsString: slowTalents,
		}, &proto.PartyBuffs{}, &proto.RaidBuffs{}, &proto.Debuffs{}),
		Encounter:  encounter,
		SimOptions: &proto.SimOptions{Iterations: 1, IsTest: true, RandomSeed: 101},
	})
	sim.Reset()
	mage := sim.Raid.Parties[0].Players[0].(*Mage)
	if mage.Slow == nil || mage.Talents.TormentTheWeak != 3 {
		t.Fatalf("Slow registered: %v, Torment the Weak points: %d", mage.Slow != nil, mage.Talents.TormentTheWeak)
	}
	boss, add := sim.GetTargetUnit(0), sim.GetTargetUnit(1)
	advance := func(by time.Duration) {
		for until := sim.CurrentTime + by; sim.CurrentTime < until && !sim.Step(); {
		}
	}
	slowed := func(target *core.Unit) bool { return target.HasActiveAura("Slow by " + mage.Label) }
	wantTorment := func(when string, target *core.Unit, want float64) {
		t.Helper()
		if got := mage.tormentTheWeak(target); math.Abs(got-want) > 1e-9 {
			t.Errorf("%s: Torment the Weak x%v, want x%v", when, got, want)
		}
	}
	// a resist happens, so cast until it lands. The idle rotation parks the GCD, so each forced cast frees it
	slow := func(target *core.Unit) {
		t.Helper()
		for try := 0; try < 10; try++ {
			advance(2 * time.Second)
			mage.GCD.Reset()
			if !mage.Slow.Cast(sim, target) {
				t.Fatalf("Slow didn't cast at %s", target.Label)
			}
			if slowed(target) {
				return
			}
		}
		t.Fatalf("ten Slows never landed on %s", target.Label)
	}

	bossCasts, addCasts := boss.CastSpeed, add.CastSpeed
	slow(boss)
	wantTorment("the boss under Slow", boss, 1)
	if got := boss.CastSpeed / bossCasts; math.Abs(got-1.3) > 1e-9 {
		t.Errorf("the boss's casts take x%v under Slow, want x1.3", got)
	}

	slow(add)
	wantTorment("the add under Slow", add, 1.12)
	if got := add.CastSpeed / addCasts; math.Abs(got-1.3) > 1e-9 {
		t.Errorf("the add's casts take x%v under Slow, want x1.3", got)
	}
	if slowed(boss) {
		t.Error("Slow stayed on the boss after it went on the add")
	}
	if got := boss.CastSpeed / bossCasts; math.Abs(got-1) > 1e-9 {
		t.Errorf("the boss's casts take x%v once Slow left it, want x1", got)
	}

	// Clearcasting's class mask leaves Slow out: it pays in full and the buff stays for the next spell
	mage.ClearcastingAura.Activate(sim)
	advance(2 * time.Second)
	mana := mage.CurrentMana()
	mage.GCD.Reset()
	if !mage.Slow.Cast(sim, add) {
		t.Fatal("Slow didn't cast under Clearcasting")
	}
	if paid := mana - mage.CurrentMana(); paid < 0.1*mage.BaseMana {
		t.Errorf("Slow paid %v mana under Clearcasting, want its whole cost", paid)
	}
	if !mage.ClearcastingAura.IsActive() {
		t.Error("Slow used up Clearcasting")
	}

	advance(16 * time.Second)
	wantTorment("the add once Slow ran out", add, 1)
	if got := add.CastSpeed / addCasts; math.Abs(got-1) > 1e-9 {
		t.Errorf("the add's casts take x%v once Slow ran out, want x1", got)
	}
}
