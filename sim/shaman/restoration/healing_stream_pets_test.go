package restoration

import (
	"testing"
	"time"
)

// 58761 is CASTER_AREA_PARTY, so each tick heals the party's pets out at that moment too, one summoned after the drop
// included. TARGET_CHECK_PARTY skips a guardian without a pet bar, as the Fire Elemental is (SummonProperties 61).
func TestHealingStreamTotemHealsThePartysPets(t *testing.T) {
	r := newChainHealRaid()
	sim := r.sim
	caster := r.shamans[0]
	pet := r.shamans[1].FireElemental
	defer func() { pet.Uncontrollable = true }()

	for _, tc := range []struct {
		name                string
		uncontrollable, out bool
		outAfterDrop        bool
		heals               bool
	}{
		{"guardian without a pet bar", true, true, false, false},
		{"pet", false, true, false, true},
		{"pet away", false, false, false, false},
		{"pet out after the drop", false, false, true, true},
	} {
		pet.Uncontrollable = tc.uncontrollable
		sim.Reset()
		sim.PrePull()
		for sim.CurrentTime < 0 && !sim.Step() {
		}
		summon := func() {
			pet.EnableWithTimeout(sim, pet, time.Minute)
			pet.RemoveHealth(sim, 1000)
		}
		if tc.out {
			summon()
		}
		caster.HealingStreamTotem.SkipCastAndApplyEffects(sim, &caster.Unit)
		if tc.outAfterDrop {
			summon()
		}
		before := pet.CurrentHealth()
		caster.HealingStreamTotem.Hot(&caster.Unit).TickOnce(sim)
		if healed := pet.CurrentHealth() > before; healed != tc.heals {
			t.Errorf("%s: healed %v, want %v", tc.name, healed, tc.heals)
		}
		sim.Cleanup()
	}
}
