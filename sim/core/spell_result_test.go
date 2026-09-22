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
