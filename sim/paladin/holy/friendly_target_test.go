package holy

import "testing"

// A healer's current target is a friendly dummy. The server refuses Exorcism there, while the area spells
// ignore their target and a heal goes through.
func TestNoDamageCastsAtAFriend(t *testing.T) {
	sim, paladin := healSetup{dummies: 2}.build(t)
	startFight(sim)
	dummy := targetDummies(sim)[0]

	if paladin.Exorcism.CanCast(sim, dummy) {
		t.Error("Exorcism can be cast at a friendly dummy")
	}
	if paladin.Exorcism.Cast(sim, dummy) {
		t.Error("Exorcism cast at a friendly dummy")
	}
	if !paladin.Exorcism.CanCast(sim, sim.Encounter.TargetUnits[0]) {
		t.Error("Exorcism can't be cast at the boss")
	}
	if !paladin.HolyWrath.CanCast(sim, dummy) || !paladin.Consecration.CanCast(sim, dummy) {
		t.Error("Holy Wrath or Consecration can't be cast with a friendly dummy as the current target")
	}
}
