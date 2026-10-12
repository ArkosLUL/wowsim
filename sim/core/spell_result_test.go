package core

import (
	"testing"
	"time"
)

// A result carries the cast it was made for, not whatever CurCast reads when it lands: a missile
// stays in the air long enough for the next cast of the same spell to overwrite CurCast, which is
// how Ignite used to misread a Brain Freeze Frostfire Bolt as a hardcast.
func TestSpellResultKeepsTheCastItWasMadeFor(t *testing.T) {
	spell := &Spell{}
	target := &Unit{}

	spell.CurCast.CastTime = 0
	instant := spell.NewResult(target)

	spell.CurCast.CastTime = 2 * time.Second
	hardcast := spell.NewResult(target)

	if !instant.FromInstantCast() {
		t.Error("the instant cast's result reads as a hardcast")
	}
	if hardcast.FromInstantCast() {
		t.Error("the hardcast's result reads as instant")
	}
}

// Only an instant hit the player casts itself is handled with its packet. A missile's hit, and a cast made
// inside a unit's update (Living Bomb's explosion as the aura ends on the target), come once the caster's
// clock has moved, like a hardcast's landing.
func TestOnlyAnInstantHitOffThePlayersOwnCastIsDealtWithItsCast(t *testing.T) {
	sim := &Simulation{}
	target := &Unit{}

	instant := &Spell{}
	if result := instant.NewResult(target); !result.DealtWithItsCast(sim, instant) {
		t.Error("an instant hit with no missile isn't dealt with its cast")
	}

	missile := &Spell{MissileSpeed: 24}
	if result := missile.NewResult(target); result.DealtWithItsCast(sim, missile) {
		t.Error("an instant missile's hit is dealt with its cast: it lands in the caster's event pass")
	}

	hardcast := &Spell{}
	hardcast.CurCast.CastTime = 2 * time.Second
	if result := hardcast.NewResult(target); result.DealtWithItsCast(sim, hardcast) {
		t.Error("a hardcast's hit is dealt with its cast")
	}

	sim.updating = target
	if result := instant.NewResult(target); result.DealtWithItsCast(sim, instant) {
		t.Error("an instant cast made inside the target's update is dealt with its cast")
	}
}
