package core

import (
	"testing"
	"time"
)

func TestSingleAuraExclusiveDurationNoOverwrite(t *testing.T) {
	sim := &Simulation{}

	target := Unit{
		Type:        EnemyUnit,
		Index:       0,
		Level:       83,
		auraTracker: newAuraTracker(),
	}
	mangle := MangleAura(&target)
	trauma := MakePermanent(TraumaAura(&target, 2))

	// Trauma in this case should *never* be overwritten
	// as its duration from 'MakePermanent' should make it non overwritable by 1 min duration mangles
	trauma.Activate(sim)

	sim.CurrentTime = 1 * time.Second

	mangle.Activate(sim)

	if !(trauma.IsActive() && !mangle.IsActive()) {
		t.Fatalf("lower duration exclusive aura overwrote previous!")
	}
}

func TestSingleAuraExclusiveDurationOverwrite(t *testing.T) {
	sim := &Simulation{}

	target := Unit{
		Type:        EnemyUnit,
		Index:       0,
		Level:       83,
		auraTracker: newAuraTracker(),
	}
	mangle := MangleAura(&target)
	trauma := TraumaAura(&target, 2)

	trauma.Activate(sim)

	sim.CurrentTime = 1 * time.Second

	mangle.Activate(sim)

	// In this case mangle should overwrite trauma as mangle will give a greater duration

	if !(mangle.IsActive() && !trauma.IsActive()) {
		t.Fatalf("longer duration exclusive aura failed to overwrite")
	}
}

// Unit::IsHighestExclusiveAuraEffect weighs two spells' equal auras by full duration, not what's left: Mangle's
// last 10 s still beat a 15 s aura, which a shorter one never replaces, while a longer one or the same spell
// always does.
func TestSingleAuraExclusiveTieTakesFullDuration(t *testing.T) {
	sim := &Simulation{}
	target := Unit{Type: EnemyUnit, Level: 83, auraTracker: newAuraTracker()}
	mangle := MangleAura(&target)
	short := bleedDamageAura(&target, Aura{Label: "Short", ActionID: ActionID{SpellID: 1}, Duration: 15 * time.Second}, 1.3)
	sameSpell := bleedDamageAura(&target, Aura{Label: "Mangle 10s", ActionID: ActionID{SpellID: 48566}, Duration: 10 * time.Second}, 1.3)

	mangle.Activate(sim)
	sim.CurrentTime = 50 * time.Second
	short.Activate(sim)
	if !mangle.IsActive() || short.IsActive() {
		t.Fatalf("a 15 s aura replaced Mangle with 10 s left")
	}

	sameSpell.Activate(sim)
	if mangle.IsActive() || !sameSpell.IsActive() {
		t.Fatalf("Mangle's own shorter aura didn't replace it")
	}

	sim.CurrentTime = 55 * time.Second
	short.Activate(sim)
	if !short.IsActive() || sameSpell.IsActive() {
		t.Fatalf("a 15 s aura didn't replace a 10 s one")
	}
}
