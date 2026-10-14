package core

import (
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
