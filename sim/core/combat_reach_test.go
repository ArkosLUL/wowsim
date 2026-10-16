package core

import (
	"math"
	"testing"
	"time"
)

// Bloodlust's 30% SPELL_AURA_MOD_SCALE grows a player's combat reach from 1.5 to 1.95, and a missile's delay
// takes the caster's reach off the distance. Live at 18 yd, Pyroblast and Fireball land from about 668 ms under
// it and 687 without.
func TestBloodlustShortensAMissilesFlight(t *testing.T) {
	sim, a := newTickTestSim(t, 0, 0, nil)
	a.DistanceFromTarget = 18
	spell := &Spell{Unit: &a.Unit, MissileSpeed: 24}
	if got := spell.TravelTime(); got != 687*time.Millisecond {
		t.Errorf("without Bloodlust %v, want 687ms", got)
	}
	bloodlust := BloodlustAura(&a.Character, -1)
	bloodlust.Activate(sim)
	if got := spell.TravelTime(); got != 668*time.Millisecond {
		t.Errorf("under Bloodlust %v, want 668ms", got)
	}
	bloodlust.Deactivate(sim)
	if got := spell.TravelTime(); got != 687*time.Millisecond {
		t.Errorf("after Bloodlust %v, want 687ms", got)
	}
}

// A script's UNIT_FIELD_COMBATREACH holds until the unit's scale changes: the Dancing Rune Weapon's 0.01 on the DK
// lasts until Bloodlust comes or goes, which puts him back on his scale's (Player::SetObjectScale).
func TestScriptReachHoldsUntilTheScaleChanges(t *testing.T) {
	sim, a := newTickTestSim(t, 0, 0, nil)
	a.DistanceFromTarget = 18
	spell := &Spell{Unit: &a.Unit, MissileSpeed: 24}
	a.SetCombatReach(0.01)
	if got := spell.TravelTime(); got != 749*time.Millisecond {
		t.Errorf("at 0.01 yd of reach %v, want 749ms", got)
	}
	bloodlust := BloodlustAura(&a.Character, -1)
	bloodlust.Activate(sim)
	if got := spell.TravelTime(); got != 668*time.Millisecond {
		t.Errorf("Bloodlust after the script %v, want 668ms", got)
	}
	a.SetCombatReach(0.01)
	bloodlust.Deactivate(sim)
	if got := spell.TravelTime(); got != 687*time.Millisecond {
		t.Errorf("Bloodlust gone after the script %v, want 687ms", got)
	}
}

// Bloodlust reaches a haste carrier with its scale alone: the carrier is immune to its two haste auras, effect by
// effect. A guardian without a pet bar gets none of it, and a guardian's reach grows from its model's.
func TestBloodlustScalesAHasteCarrier(t *testing.T) {
	for _, tc := range []struct {
		name           string
		uncontrollable bool
		modelReach     float64
		want           float64
	}{
		{"haste carrier", false, 0, 1.95},
		{"haste carrier with a model's reach of 1", false, 1, 1.3},
		{"haste carrier without a pet bar", true, 0, 1.5},
	} {
		sim, pet := newSummonTestSim(t)
		pet.HasteCarrier = true
		pet.Uncontrollable = tc.uncontrollable
		pet.BaseCombatReach = tc.modelReach
		runUntil(sim, ms(1000))
		pet.Enable(sim, pet)
		melee, cast := pet.PseudoStats.MeleeSpeedMultiplier, pet.PseudoStats.CastSpeedMultiplier

		BloodlustAura(pet.Owner, -1).Activate(sim)
		if got := pet.combatReach(); math.Abs(got-tc.want) > 1e-9 {
			t.Errorf("%s: reach %v under its owner's Bloodlust, want %v", tc.name, got, tc.want)
		}
		if pet.PseudoStats.MeleeSpeedMultiplier != melee || pet.PseudoStats.CastSpeedMultiplier != cast {
			t.Errorf("%s: Bloodlust's haste reached the carrier", tc.name)
		}
		runUntil(sim, ms(1000)+BloodlustDuration+time.Second)
		base := tc.modelReach
		if base == 0 {
			base = defaultCombatReach
		}
		if got := pet.combatReach(); math.Abs(got-base) > 1e-9 {
			t.Errorf("%s: reach %v once Bloodlust's gone, want %v", tc.name, got, base)
		}
	}
}

// A pet that follows its owner stands its reach plus PET_FOLLOW_DIST's 2 yd off his left side.
func TestFollowSpotDistance(t *testing.T) {
	_, pet := newSummonTestSim(t)
	pet.Owner.DistanceFromTarget = 30
	if got, want := pet.FollowSpotDistance(), math.Hypot(30, 3.5); math.Abs(got-want) > 1e-9 {
		t.Errorf("a pet follows %v yd from the target, want %v", got, want)
	}
	pet.BaseCombatReach = 0.85
	if got, want := pet.FollowSpotDistance(), math.Hypot(30, 2.85); math.Abs(got-want) > 1e-9 {
		t.Errorf("a guardian with model 525's reach follows %v yd from the target, want %v", got, want)
	}
}
