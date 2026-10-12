package core

import "testing"

// Unit::SpellHealingBonusTaken adds the target's flat healing taken times the heal's coefficient as an
// int32 of its own, after the caster's percent: five stacks of Talisman of Troll Divinity put 233 on a
// Prayer of Mending whatever the priest's talents multiply it by.
func TestFlatHealingTakenJoinsAfterTheCastersPercent(t *testing.T) {
	var withCoefficient, without *Spell
	sim, a := newTickTestSim(t, 0, 0, func(a *timingTestAgent) {
		config := SpellConfig{
			ActionID:         ActionID{SpellID: testSpellNoServerData},
			SpellSchool:      SpellSchoolHoly,
			ProcMask:         ProcMaskSpellHealing,
			Flags:            SpellFlagHelpful,
			DamageMultiplier: 1.5,
			CritMultiplier:   1.5,
		}
		without = a.RegisterSpell(config)
		config.ActionID.Tag = 1
		config.HealingTakenCoefficient = 0.8057
		withCoefficient = a.RegisterSpell(config)
	})
	target := &a.Unit
	target.PseudoStats.BonusHealingTaken = 290
	target.PseudoStats.HealingTakenMultiplier = 1.1

	if got, want := withCoefficient.HealingPower(target), withCoefficient.SpellPower(); got != want {
		t.Errorf("healing power %v with healing taken on the target, want the caster's %v", got, want)
	}
	// 1000 × 1.5 = 1500, + int32(290 × 0.8057) = 233, × 1.1 = 1906.3
	if got := withCoefficient.CalcHealing(sim, target, 1000, withCoefficient.OutcomeHealing).Damage; got != 1906 {
		t.Errorf("healed %v, want 1906", got)
	}
	// a heal the server gives no coefficient takes the percent alone
	if got := without.CalcHealing(sim, target, 1000, without.OutcomeHealing).Damage; got != 1650 {
		t.Errorf("healed %v without a coefficient, want 1650", got)
	}
}

// A heal with no spell_bonus_data row gets the default coefficient times 1.88: 0.8057 for an instant.
func TestDefaultHealingTakenCoefficient(t *testing.T) {
	if got := PowerBonus(DefaultHealingTakenCoefficient(0), 290); got != 233 {
		t.Errorf("290 healing taken on an instant heal adds %v, want 233", got)
	}
	// a 6 s hot with two ticks
	if got := PowerBonus(DefaultHealingTakenCoefficient(6.0/15/2), 290); got != 109 {
		t.Errorf("290 healing taken on a tick of a 6 s hot adds %v, want 109", got)
	}
}
