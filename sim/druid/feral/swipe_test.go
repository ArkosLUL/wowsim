package feral

import (
	"math"
	"testing"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
)

// Swipe (Cat) pays the miss cost, irand(0, cost / 4), when its unit target doesn't take the hit
// (Spell::TakePower), and the full cost when only the other target does.
func TestSwipeCatPaysTheMissCostOnItsTarget(t *testing.T) {
	encounter := core.MakeSingleTargetEncounter(0)
	encounter.Targets = append(encounter.Targets, encounter.Targets[0])
	sim := core.NewSim(&proto.RaidSimRequest{
		Raid: core.SinglePlayerRaidProto(&proto.Player{
			Race:  proto.Race_RaceTauren,
			Class: proto.Class_ClassDruid,
			// naked, so misses and dodges come often
			Equipment:     &proto.EquipmentSpec{},
			Spec:          PlayerOptionsMonoCat,
			Glyphs:        StandardGlyphs,
			TalentsString: StandardTalents,
			Rotation:      core.APLRotationFromJsonString(`{"type": "TypeAPL", "priorityList": []}`),
		}, &proto.PartyBuffs{}, &proto.RaidBuffs{}, &proto.Debuffs{}),
		Encounter:  encounter,
		SimOptions: &proto.SimOptions{Iterations: 1, IsTest: true, RandomSeed: 101},
	})
	sim.Reset()
	cat := sim.Raid.Parties[0].Players[0].(*FeralDruid)
	main, other := sim.GetTargetUnit(0), sim.GetTargetUnit(1)

	landed := map[*core.Unit]bool{}
	cat.RegisterAura(core.Aura{
		Label:    "swipe result test",
		Duration: core.NeverExpires,
		OnSpellHitDealt: func(_ *core.Aura, _ *core.Simulation, spell *core.Spell, result *core.SpellResult) {
			if cat.SwipeCat.IsEqual(spell) {
				landed[result.Target] = result.Landed()
			}
		},
	}).Activate(sim)

	cost := cat.CurrentSwipeCatCost()
	missCap := math.Floor(cost / 4)
	misses, otherMisses := 0, 0
	for range 400 {
		cat.AddEnergy(sim, 100-cat.CurrentEnergy(), cat.EnergyRefundMetrics)
		if cat.ClearcastingAura != nil {
			cat.ClearcastingAura.Deactivate(sim)
		}
		cat.GCD.Reset()
		clear(landed)
		if !cat.SwipeCat.Cast(sim, main) {
			t.Fatal("Swipe (Cat) didn't cast")
		}
		taken := 100 - cat.CurrentEnergy()
		switch {
		case landed[main]:
			if !landed[other] {
				otherMisses++
			}
			if math.Abs(taken-cost) > 1e-9 {
				t.Fatalf("Swipe (Cat) landed on its target and took %v energy, want %v", taken, cost)
			}
		default:
			misses++
			if taken < 0 || taken > missCap+1e-9 {
				t.Fatalf("Swipe (Cat) missed its target and took %v energy, want 0 to %v", taken, missCap)
			}
		}
	}
	if misses < 20 || otherMisses < 20 {
		t.Fatalf("%d misses on the target and %d on the other one in 400 casts, too few to tell", misses, otherMisses)
	}
}
