package core

import "testing"

// Dot ticks under AnyPeriodic: a real tick procs whatever its mask; a tick the server casts as another
// spell only procs an aura that can proc from triggered spells, by that spell's damage class.
func TestAnyPeriodicTicks(t *testing.T) {
	unit := &Unit{Type: PlayerUnit}
	swp := serverDataSpell(t, unit, 48125, ProcMaskSpellDamage)
	serpentSting := serverDataSpell(t, unit, 49001, ProcMaskEmpty)
	deathAndDecay := serverDataSpell(t, unit, 49938, ProcMaskEmpty)
	explosiveShot := serverDataSpell(t, unit, 60053, ProcMaskRangedSpecial)

	for _, tc := range []struct {
		name  string
		proc  ProcTrigger
		procs map[*Spell]bool
	}{
		{"Illustration", ProcTrigger{ProcMask: ProcMaskSpellDamage | ProcMaskSpellHealing},
			map[*Spell]bool{swp: true, serpentSting: true, deathAndDecay: false, explosiveShot: false}},
		{"Dying Curse", ProcTrigger{ProcMask: ProcMaskSpellOrProc, CanProcFromProcs: true},
			map[*Spell]bool{swp: true, serpentSting: true, deathAndDecay: true, explosiveShot: false}},
		{"Greatness", ProcTrigger{ProcMask: ProcMaskDirect | ProcMaskProc, CanProcFromProcs: true},
			map[*Spell]bool{swp: true, serpentSting: true, deathAndDecay: true, explosiveShot: true}},
	} {
		for spell, want := range tc.procs {
			procced := false
			config := tc.proc
			config.Callback = CallbackOnPeriodicDamageDealt
			config.AnyPeriodic = true
			config.Handler = func(*Simulation, *Spell, *SpellResult) { procced = true }
			aura := &Aura{}
			ApplyProcTriggerCallback(unit, aura, config)
			aura.OnPeriodicDamageDealt(aura, nil, spell, &SpellResult{Outcome: OutcomeHit, Damage: 100})
			if procced != want {
				t.Errorf("%s on a %d tick: procced %v, want %v", tc.name, spell.ActionID.SpellID, procced, want)
			}
		}
	}
}
