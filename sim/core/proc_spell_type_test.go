package core

import (
	"testing"

	"github.com/wowsims/wotlk/sim/core/serverdata"
)

const (
	typeDamage    = serverdata.ProcSpellTypeDamage
	typeHeal      = serverdata.ProcSpellTypeHeal
	typeNoDmgHeal = serverdata.ProcSpellTypeNoDmgHeal
)

// ProcSkillsAndAuras gives an event its spell type from the amounts: a heal, else damage, else, for a spell
// that did neither, NO_DMG_HEAL.
func TestProcEventSpellType(t *testing.T) {
	cases := []struct {
		name    string
		amount  float64
		healing bool
		want    uint32
	}{
		{"damage", 500, false, typeDamage},
		{"heal", 500, true, typeHeal},
		{"hot or debuff landing, nothing dealt", 0, false, typeNoDmgHeal},
		{"heal landing for nothing", 0, true, typeNoDmgHeal},
	}
	for _, c := range cases {
		if got := procEventSpellType(c.amount, c.healing); got != c.want {
			t.Errorf("%s: type %d, want %d", c.name, got, c.want)
		}
	}
}

type procTypeCounts struct{ hit, tick, heal, healTick int }

func newProcTypeSim(t *testing.T, mask uint32) (*Simulation, *timingTestAgent, *Spell, *procTypeCounts) {
	t.Helper()
	var spell *Spell
	counts := &procTypeCounts{}
	sim, a := newTickTestSim(t, 0, 0, func(a *timingTestAgent) {
		spell = a.RegisterSpell(SpellConfig{
			ActionID:    ActionID{SpellID: testSpellNoServerData},
			SpellSchool: SpellSchoolNature,
			ProcMask:    ProcMaskSpellDamage,
		})
		count := func(n *int) ProcHandler {
			return func(*Simulation, *Spell, *SpellResult) { *n++ }
		}
		for label, cfg := range map[string]ProcTrigger{
			"hit":      {Callback: CallbackOnSpellHitDealt, Handler: count(&counts.hit)},
			"tick":     {Callback: CallbackOnPeriodicDamageDealt, AnyPeriodic: true, Handler: count(&counts.tick)},
			"heal":     {Callback: CallbackOnHealDealt, Handler: count(&counts.heal)},
			"healTick": {Callback: CallbackOnPeriodicHealDealt, AnyPeriodic: true, Handler: count(&counts.healTick)},
		} {
			cfg.Name, cfg.SpellTypeMask = "proc type "+label, mask
			MakeProcTriggerAura(&a.Unit, cfg)
		}
	})
	sim.PrePull()
	return sim, a, spell, counts
}

func dealProcTypeEvents(sim *Simulation, a *timingTestAgent, spell *Spell, amount float64) {
	result := &SpellResult{Target: &a.Unit, Outcome: OutcomeHit, Damage: amount}
	a.OnSpellHitDealt(sim, spell, result)
	a.OnPeriodicDamageDealt(sim, spell, result)
	a.OnHealDealt(sim, spell, result)
	a.OnPeriodicHealDealt(sim, spell, result)
}

// CanSpellTriggerProcOnEvent: an entry with a spell type mask only takes events whose type it lists, direct and
// periodic alike, and an entry without one takes everything.
func TestProcTriggerSpellTypeMask(t *testing.T) {
	// damage and heal events, then the same four events dealing nothing (a miss, a debuff or hot landing)
	cases := []struct {
		name    string
		mask    uint32
		dealing procTypeCounts
		nothing procTypeCounts
	}{
		{"no mask", 0, procTypeCounts{1, 1, 1, 1}, procTypeCounts{1, 1, 1, 1}},
		{"damage", typeDamage, procTypeCounts{1, 1, 0, 0}, procTypeCounts{0, 0, 0, 0}},
		{"damage or heal", typeDamage | typeHeal, procTypeCounts{1, 1, 1, 1}, procTypeCounts{0, 0, 0, 0}},
		{"heal", typeHeal, procTypeCounts{0, 0, 1, 1}, procTypeCounts{0, 0, 0, 0}},
		{"nothing dealt", typeNoDmgHeal, procTypeCounts{0, 0, 0, 0}, procTypeCounts{1, 1, 1, 1}},
		{"all", typeDamage | typeHeal | typeNoDmgHeal, procTypeCounts{1, 1, 1, 1}, procTypeCounts{1, 1, 1, 1}},
	}
	for _, c := range cases {
		sim, a, spell, counts := newProcTypeSim(t, c.mask)
		dealProcTypeEvents(sim, a, spell, 500)
		if *counts != c.dealing {
			t.Errorf("%s: after events dealing 500, procs %+v, want %+v", c.name, *counts, c.dealing)
		}
		*counts = procTypeCounts{}
		dealProcTypeEvents(sim, a, spell, 0)
		if *counts != c.nothing {
			t.Errorf("%s: after events dealing nothing, procs %+v, want %+v", c.name, *counts, c.nothing)
		}
	}
}

// A melee swing carries no spell on the server, so its flags aren't spell flags and the type check skips it.
// Auto Shot is a spell, and DONE_RANGED_AUTO_ATTACK is in SPELL_PROC_FLAG_MASK.
func TestProcSpellTypeSkipsWhiteSwings(t *testing.T) {
	swing := &Spell{ProcMask: ProcMaskMeleeMHAuto}
	miss := &SpellResult{Outcome: OutcomeMiss}
	if !procSpellTypeAllows(typeDamage, swing, miss, false) {
		t.Error("a white swing that dealt nothing was refused by a damage only entry")
	}
	autoShot := &Spell{ProcMask: ProcMaskRangedAuto}
	if procSpellTypeAllows(typeDamage, autoShot, miss, false) {
		t.Error("an Auto Shot that dealt nothing was taken by a damage only entry")
	}
	special := &Spell{ProcMask: ProcMaskMeleeMHSpecial}
	if procSpellTypeAllows(typeDamage, special, miss, false) {
		t.Error("a special attack that dealt nothing was taken by a damage only entry")
	}
}

// The generated entries carry their masks: the Ashen Band's is damage only, Pandora's Plea's own is damage and heal.
func TestServerProcCarriesTheSpellTypeMask(t *testing.T) {
	for id, want := range map[int32]uint32{72413: typeDamage, 64742: typeDamage | typeHeal, 15600: typeDamage} {
		if got := ServerProcFor(id).SpellTypeMask; got != want {
			t.Errorf("spell %d: type mask %d, want %d", id, got, want)
		}
	}
	// a cast phase event has every type, so Alacrity of the Elements' damage only mask never filters; a taken
	// entry (Judgement of Wisdom's, phase 0) is typed on the hit
	for id, want := range map[int32]uint32{65005: 0, 64714: typeDamage, 20186: typeDamage} {
		if got := ServerProcFor(id).SpellTypeMask; got != want {
			t.Errorf("spell %d: type mask %d, want %d", id, got, want)
		}
	}
	sp := ServerProcFor(72413)
	spell := &Spell{ProcMask: ProcMaskMeleeMHSpecial}
	if sp.AllowsEvent(spell, &SpellResult{Outcome: OutcomeHit}, false) {
		t.Error("a landing that dealt nothing proc'd a damage only entry")
	}
	if !sp.AllowsEvent(spell, &SpellResult{Outcome: OutcomeHit, Damage: 1}, false) {
		t.Error("a hit that dealt damage did not proc a damage only entry")
	}
}
