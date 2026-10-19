package protection

import (
	"testing"
	"time"

	"github.com/wowsims/wotlk/sim/core"
)

// 20178 has four charges: each white swing, a miss too, uses one and queues an extra attack (32746) for the
// paladin's next update. While that one waits or swings, nothing procs it (AuraEffect::CheckEffectProc), so no
// charge goes.
func TestReckoningUsesAChargeAnExtraAttack(t *testing.T) {
	p := newTankFixesPaladin(t, core.APLRotationFromJsonString(`{"type": "TypeAPL", "priorityList": []}`))
	sim, pal := p.sim, p.pal
	sim.Reset()
	sim.PrePull()
	// past the pull, so the auto attacks are on
	for sim.CurrentTime < time.Millisecond && !sim.Step() {
	}
	charges := pal.GetAura("Reckoning Proc")
	extra := pal.GetSpell(core.ActionID{SpellID: 20182})
	if pal.Talents.Reckoning == 0 || charges == nil || extra == nil {
		t.Fatalf("Reckoning %d, its aura %v, its extra attack %v", pal.Talents.Reckoning, charges, extra)
	}
	// no swing of the paladin's own in the way
	pal.AutoAttacks.StopMeleeUntil(sim, sim.CurrentTime+time.Minute, false)
	var extras []time.Duration
	effects := extra.ApplyEffects
	extra.ApplyEffects = func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
		extras = append(extras, sim.CurrentTime)
		effects(sim, target, spell)
	}
	charges.Activate(sim)
	charges.SetStacks(sim, 4)
	swing, target := pal.AutoAttacks.MHAuto(), sim.GetTargetUnit(0)

	for i := 1; i <= 4; i++ {
		pal.OnSpellHitDealt(sim, swing, &core.SpellResult{Target: target, Outcome: core.OutcomeMiss})
		pal.OnSpellHitDealt(sim, swing, &core.SpellResult{Target: target, Outcome: core.OutcomeHit})
		if got := charges.GetStacks(); got != int32(4-i) {
			t.Fatalf("swing %d: %d charges left, want %d", i, got, 4-i)
		}
		for len(extras) < i {
			if sim.Step() || sim.CurrentTime > 30*time.Second {
				t.Fatalf("swing %d: the extra attack never went", i)
			}
		}
		if got := charges.GetStacks(); got != int32(4-i) {
			t.Errorf("swing %d: %d charges left after its extra attack, want %d", i, got, 4-i)
		}
	}
	if charges.IsActive() || len(extras) != 4 {
		t.Errorf("up %v after the fourth charge, %d extra attacks, want 4", charges.IsActive(), len(extras))
	}
}
