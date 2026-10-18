package restoration

import (
	"testing"
)

// The totem's 58761 searches its party through SpellInfo::CheckTarget, which keeps the dead out, so a dead
// member's tick heals nothing.
func TestHealingStreamTotemSkipsTheDead(t *testing.T) {
	r := newChainHealRaid()
	sim := r.sim
	caster := r.shamans[0]
	dead, hurt := &r.shamans[1].Unit, &r.shamans[2].Unit

	sim.Reset()
	sim.PrePull()
	for sim.CurrentTime < 0 && !sim.Step() {
	}
	dead.RemoveHealth(sim, dead.MaxHealth())
	hurt.RemoveHealth(sim, 1000)
	before := hurt.CurrentHealth()
	for _, unit := range []*RestorationShaman{r.shamans[1], r.shamans[2]} {
		hot := caster.HealingStreamTotem.Hot(&unit.Unit)
		hot.Activate(sim)
		hot.TickOnce(sim)
	}
	if dead.CurrentHealth() != 0 {
		t.Errorf("the dead shaman is at %v health", dead.CurrentHealth())
	}
	if hurt.CurrentHealth() <= before {
		t.Errorf("the hurt shaman is still at %v health", hurt.CurrentHealth())
	}
	sim.Cleanup()
}
