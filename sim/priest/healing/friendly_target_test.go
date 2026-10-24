package healing

import "testing"

// Shadow Word: Pain at another healer in the raid: the server refuses it, and a heal there still goes through.
func TestNoShadowWordPainAtAFriend(t *testing.T) {
	r := newHealRaid(t, 2)
	priest, friend := r.priests[0], &r.priests[1].Unit

	if priest.ShadowWordPain.CanCast(r.sim, friend) {
		t.Error("Shadow Word: Pain can be cast at a friend")
	}
	if priest.ShadowWordPain.Cast(r.sim, friend) {
		t.Error("Shadow Word: Pain cast at a friend")
	}
	if !priest.FlashHeal.CanCast(r.sim, friend) {
		t.Error("Flash Heal can't be cast at a friend")
	}
	if !priest.ShadowWordPain.CanCast(r.sim, r.sim.Encounter.TargetUnits[0]) {
		t.Error("Shadow Word: Pain can't be cast at the boss")
	}
}
