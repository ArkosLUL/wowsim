package core

import (
	"testing"

	"github.com/wowsims/wotlk/sim/core/stats"
)

// spell_pal_judgement_of_light_heal: a damaging hit on the judged target heals the attacker for 2% of his max
// health, through his healing taken percents. Like Judgement of Wisdom's entry, a triggered cast, a miss and a
// hit for 0 never proc.
func TestJudgementOfLightHeal(t *testing.T) {
	var strike, proc *Spell
	sim, a := newTickTestSim(t, 0, 0, func(a *timingTestAgent) {
		strike = a.RegisterSpell(SpellConfig{ActionID: ActionID{SpellID: testSpellNoServerData}, SpellSchool: SpellSchoolFire,
			ProcMask: ProcMaskSpellDamage, Flags: SpellFlagNoServerData})
		proc = a.RegisterSpell(SpellConfig{ActionID: ActionID{SpellID: testSpellNoServerData, Tag: 1}, SpellSchool: SpellSchoolPhysical,
			ProcMask: ProcMaskProc, Flags: SpellFlagNoServerData})
	})
	unit, target := &a.Unit, sim.Encounter.TargetUnits[0]
	jol := JudgementOfLightAura(target)
	metrics := unit.NewHealthMetrics(ActionID{OtherID: 1})

	// procs returns how many of 200 hits healed under a takenPct healing taken aura, failing on any heal other
	// than want
	procs := func(spell *Spell, outcome HitOutcome, damage, want float64, takenPct int32) int {
		t.Helper()
		sim.Cleanup()
		sim.reset()
		unit.AddStatsDynamic(sim, stats.Stats{stats.Health: 50000 - unit.MaxHealth()})
		unit.GainHealth(sim, unit.MaxHealth(), metrics)
		unit.RemoveHealth(sim, 40000)
		unit.AddHealingTakenPct(takenPct)
		jol.Activate(sim)
		healed := 0
		for range 200 {
			before := unit.CurrentHealth()
			jol.OnSpellHitTaken(jol, sim, spell, &SpellResult{Target: target, Outcome: outcome, Damage: damage})
			if gain := unit.CurrentHealth() - before; gain != 0 {
				if gain != want {
					t.Fatalf("healed %v, want %v", gain, want)
				}
				healed++
				unit.RemoveHealth(sim, gain)
			}
		}
		return healed
	}

	if procs(strike, OutcomeHit, 100, 1000, 0) == 0 {
		t.Errorf("200 hits never healed")
	}
	if procs(strike, OutcomeHit, 100, 1200, 20) == 0 {
		t.Errorf("200 hits under a 20%% healing taken aura never healed")
	}
	if procs(proc, OutcomeHit, 100, 1000, 0) != 0 {
		t.Errorf("a triggered hit healed")
	}
	if procs(strike, OutcomeMiss, 0, 1000, 0) != 0 {
		t.Errorf("a miss healed")
	}
	if procs(strike, OutcomeHit, 0, 1000, 0) != 0 {
		t.Errorf("a hit for 0 healed")
	}
}
