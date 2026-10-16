package core

import (
	"slices"
	"testing"
	"time"
)

// AuraEffect::HandlePeriodicHealAurasTick multiplies the aura's whole amount by the caster's healing done percents
// at each tick, int32(float(amount) × GetTotalAuraMultiplier), so a percent that comes mid-hot counts from the next
// tick, and the float32 product can land under a whole number that float64 would reach.
func TestAHotTakesTheHealingDonePercentsAfterItsAmount(t *testing.T) {
	var hot *Spell
	var base float64
	var ticks []float64
	sim, a := newTickTestSim(t, 0, 0, func(a *timingTestAgent) {
		hot = a.RegisterSpell(SpellConfig{
			ActionID:         ActionID{SpellID: testSpellNoServerData},
			SpellSchool:      SpellSchoolHoly,
			ProcMask:         ProcMaskSpellHealing,
			Flags:            SpellFlagHelpful,
			DamageMultiplier: 1.2999,
			Hot: DotConfig{
				Aura:          Aura{Label: "hot"},
				NumberOfTicks: 3,
				TickLength:    time.Second,
				TicksCanCrit:  false,
				OnSnapshot: func(_ *Simulation, _ *Unit, dot *Dot, _ bool) {
					dot.SnapshotBaseDamage = base
					dot.SnapshotHotMultiplier(dot.Spell.CasterHealingMultiplier())
				},
				OnTick: func(sim *Simulation, target *Unit, dot *Dot) {
					ticks = append(ticks, dot.CalcSnapshotHealing(sim, target, dot.OutcomeTick).Damage)
				},
			},
		})
		a.AddHealingDonePct(3)
	})
	sim.PrePull()

	// 1000 × 1.2999 is 1299 whole, × 1.03 is 1337: one truncation of 1000 × 1.2999 × 1.03 would give 1338. Avenging
	// Wrath's 20% on top of it from the second tick on: 1299 × 1.236 is 1605.
	base = 1000
	at(sim, ms(10), func(sim *Simulation) { hot.Hot(&a.Unit).Apply(sim) })
	at(sim, ms(1500), func(*Simulation) { a.AddHealingDonePct(20) })
	at(sim, ms(2500), func(*Simulation) { a.RemoveHealingDonePct(20) })
	runUntil(sim, ms(3100))
	if want := []float64{1337, 1605, 1337}; !slices.Equal(ticks, want) {
		t.Errorf("ticks %v, want %v", ticks, want)
	}

	// a base of 173.1 makes the amount 225, and 225 × 1.04f is 233.99998 in float32, so 233
	a.RemoveHealingDonePct(3)
	a.AddHealingDonePct(4)
	base, ticks = 173.1, nil
	at(sim, ms(3200), func(sim *Simulation) { hot.Hot(&a.Unit).Apply(sim) })
	runUntil(sim, ms(4300))
	if len(ticks) != 1 || ticks[0] != 233 {
		t.Errorf("ticks %v of an amount of 225 under 4%%, want 233 (float64 would give 234)", ticks)
	}
}

// Unit::GetTotalAuraMultiplier adds each aura's percent with AddPct in float32: 3% then 4% is 1.0712.
func TestHealingDoneMultiplierIsTheServersFloat32(t *testing.T) {
	unit := &Unit{}
	if got := unit.healingDoneMultiplier(); got != 1 {
		t.Errorf("no aura: %v, want 1", got)
	}
	unit.AddHealingDonePct(3)
	unit.AddHealingDonePct(4)
	unit.AddHealingDonePct(0)
	if got := unit.healingDoneMultiplier(); got != float32(1.0712) {
		t.Errorf("3%% and 4%%: %v, want 1.0712", got)
	}
	unit.RemoveHealingDonePct(3)
	if got := unit.healingDoneMultiplier(); got != float32(1.04) {
		t.Errorf("4%% left: %v, want 1.04", got)
	}
}
