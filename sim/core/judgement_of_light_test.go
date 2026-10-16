package core

import (
	"testing"

	"github.com/wowsims/wotlk/sim/core/proto"
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

// The heals off two judged targets go on one row of the attacker's.
func TestJudgementOfLightHealsOnOneRow(t *testing.T) {
	var strike *Spell
	timingTestSetup = func(a *timingTestAgent) {
		strike = a.RegisterSpell(SpellConfig{ActionID: ActionID{SpellID: testSpellNoServerData}, SpellSchool: SpellSchoolFire,
			ProcMask: ProcMaskSpellDamage, Flags: SpellFlagNoServerData})
	}
	defer func() { timingTestSetup = nil }()
	sim := NewSim(&proto.RaidSimRequest{
		SimOptions: &proto.SimOptions{RandomSeed: 101, IsTest: true},
		Raid: &proto.Raid{Parties: []*proto.Party{{
			Players: []*proto.Player{{Name: "Attacker", Class: proto.Class_ClassShaman, Consumes: &proto.Consumes{},
				Buffs: &proto.IndividualBuffs{}, Spec: &proto.Player_EnhancementShaman{}, Equipment: &proto.EquipmentSpec{}}},
			Buffs: &proto.PartyBuffs{},
		}}},
		Encounter: &proto.Encounter{
			Targets:  []*proto.Target{{Name: "a", Level: 83}, {Name: "b", Level: 83}},
			Duration: 60,
		},
	})
	sim.reset()
	unit := &sim.Raid.Parties[0].Players[0].(*timingTestAgent).Unit
	unit.RemoveHealth(sim, unit.MaxHealth()/2)

	for _, target := range sim.Encounter.TargetUnits {
		jol := JudgementOfLightAura(target)
		jol.Activate(sim)
		healed := false
		for i := 0; i < 200 && !healed; i++ {
			before := unit.CurrentHealth()
			jol.OnSpellHitTaken(jol, sim, strike, &SpellResult{Target: target, Outcome: OutcomeHit, Damage: 100})
			healed = unit.CurrentHealth() > before
		}
		if !healed {
			t.Fatalf("200 hits on %s never healed", target.Label)
		}
	}
	rows := 0
	for _, rm := range unit.Metrics.resources {
		if rm.ActionID.SpellID == 20267 {
			rows++
		}
	}
	if rows != 1 {
		t.Errorf("%d Judgement of Light rows, want 1", rows)
	}
}
