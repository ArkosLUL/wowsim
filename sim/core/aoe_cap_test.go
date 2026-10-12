package core

import "testing"

// Volley's channel: its ticks are casts of 58433
const testSpellVolley = 58434

// Spell::DoAllEffectOnLaunchTarget cuts a player's area hit to damage × 10 / units in whole numbers, after
// the done percent: 1001 × 1.5 is 1501, and 15010 / 11 is 1364. Cut first, it would be 1365.
func TestAoECapCutsTheWholeDamageAfterThePercents(t *testing.T) {
	var plain, tickCasts *Spell
	sim, a := newTickTestSim(t, 0, 0, func(a *timingTestAgent) {
		config := SpellConfig{
			ActionID:         ActionID{SpellID: testSpellNoServerData},
			SpellSchool:      SpellSchoolArcane,
			ProcMask:         ProcMaskSpellDamage,
			Flags:            SpellFlagIgnoreResists,
			DamageMultiplier: 1.5,
			CritMultiplier:   1.5,
		}
		plain = a.RegisterSpell(config)
		config.ActionID = ActionID{SpellID: testSpellVolley}
		tickCasts = a.RegisterSpell(config)
	})
	target := a.CurrentTarget
	if !tickCasts.ticksAsCasts() || plain.ticksAsCasts() {
		t.Fatalf("ticks as casts: Volley %v, a spell without server data %v", tickCasts.ticksAsCasts(), plain.ticksAsCasts())
	}

	for _, tc := range []struct {
		name     string
		spell    *Spell
		units    int
		periodic bool
		want     float64
	}{
		{"11 units", plain, 11, false, 1364},
		{"10 units", plain, 10, false, 1501},
		{"a dot's tick", plain, 11, true, 1501},
		{"a tick the server casts", tickCasts, 11, true, 1364},
	} {
		tc.spell.AoECapUnits = tc.units
		var result *SpellResult
		if tc.periodic {
			result = tc.spell.CalcPeriodicDamage(sim, target, 1001, tc.spell.OutcomeAlwaysHit)
		} else {
			result = tc.spell.CalcDamage(sim, target, 1001, tc.spell.OutcomeAlwaysHit)
		}
		if result.Damage != tc.want {
			t.Errorf("%s: %v damage, want %v", tc.name, result.Damage, tc.want)
		}
		tc.spell.DisposeResult(result)
	}
}
