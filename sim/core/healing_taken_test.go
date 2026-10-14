package core

import (
	"testing"

	"github.com/wowsims/wotlk/sim/core/serverdata"
	"github.com/wowsims/wotlk/sim/core/stats"
)

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

// Unit::SpellHealingBonusTaken runs the target's own SPELLMOD_BONUS_MULTIPLIER mods on the coefficient, so a
// priest's Empowered Healing raises the term on its own Greater Heal and no other target's.
func TestHealTakenCoefficientTakesTheTargetsMods(t *testing.T) {
	heal := &serverdata.Spell{ID: testSpellID, DmgClass: serverdata.DmgClassMagic,
		Effects: [3]serverdata.Effect{{Effect: effectHeal}}}
	newSpell := func(unit *Unit, sp float64) *Spell {
		spell := &Spell{Unit: unit, ActionID: ActionID{SpellID: testSpellID}, serverSpell: heal,
			Direct: SpellEffect{SP: sp}, unmoddedSP: [3]float64{1.611}}
		unit.Spellbook = append(unit.Spellbook, spell)
		return spell
	}
	caster, sameTalents, bare := &Unit{}, &Unit{}, &Unit{}
	cast := newSpell(caster, 1.711)
	newSpell(sameTalents, 1.811)

	for _, tc := range []struct {
		name   string
		target *Unit
		want   float64
	}{
		{"the caster's own mods on a self heal", caster, 1.711},
		{"the target's mods", sameTalents, 1.811},
		{"none on a target without the spell", bare, 1.611},
	} {
		if got := cast.healTakenCoefficient(tc.target); got != tc.want {
			t.Errorf("%s: %v, want %v", tc.name, got, tc.want)
		}
	}
}

// A pet's GetSpellModOwner is its owner, so a heal on the pet takes the owner's SPELLMOD_BONUS_MULTIPLIER
// mods: the caster's own when it heals its pet.
func TestHealTakenCoefficientOnAPetIsItsOwners(t *testing.T) {
	var pet *timingTestPet
	timingTestConstruct = func(a *timingTestAgent) {
		pet = &timingTestPet{Pet: NewPet("Pet", &a.Character, stats.Stats{}, func(stats.Stats) stats.Stats { return stats.Stats{} }, true, false)}
		a.AddPet(pet)
	}
	_, a := newTickTestSim(t, 0, 0, nil)
	timingTestConstruct = nil

	heal := &serverdata.Spell{ID: testSpellID, DmgClass: serverdata.DmgClassMagic,
		Effects: [3]serverdata.Effect{{Effect: effectHeal}}}
	newSpell := func(unit *Unit, sp float64) *Spell {
		spell := &Spell{Unit: unit, ActionID: ActionID{SpellID: testSpellID}, serverSpell: heal,
			Direct: SpellEffect{SP: sp}, unmoddedSP: [3]float64{1.611}}
		unit.Spellbook = append(unit.Spellbook, spell)
		return spell
	}
	fromOther := newSpell(&Unit{}, 1.711)
	if got := fromOther.healTakenCoefficient(&pet.Unit); got != 1.611 {
		t.Errorf("another caster's heal on a pet whose owner lacks the spell: %v, want 1.611", got)
	}
	fromOwner := newSpell(&a.Unit, 1.811)
	if got := fromOwner.healTakenCoefficient(&pet.Unit); got != 1.811 {
		t.Errorf("the owner's heal on its pet: %v, want its own 1.811", got)
	}
	if got := fromOther.healTakenCoefficient(&pet.Unit); got != 1.811 {
		t.Errorf("another caster's heal on the pet: %v, want the owner's 1.811", got)
	}
}

// A heal with SPELL_ATTR0_CU_NO_POSITIVE_TAKEN_BONUS (Ancestral Awakening) loses the flat term and the taken
// percent once the percent is over 1, and keeps both under it. It ignores the caster's modifiers too, so
// Strength of Wrynn's stand-in, a healing done percent, never reaches it.
func TestNoPositiveHealingTakenOverOne(t *testing.T) {
	var spell *Spell
	sim, a := newTickTestSim(t, 0, 0, func(a *timingTestAgent) {
		spell = a.RegisterSpell(SpellConfig{
			ActionID:                ActionID{SpellID: testSpellNoServerData},
			SpellSchool:             SpellSchoolNature,
			ProcMask:                ProcMaskSpellHealing,
			Flags:                   SpellFlagHelpful | SpellFlagIgnoreAttackerModifiers,
			DamageMultiplier:        1.5,
			CritMultiplier:          1.5,
			HealingTakenCoefficient: 0.8057,
		})
	})
	spell.serverSpell = &serverdata.Spell{ID: testSpellNoServerData, DmgClass: serverdata.DmgClassMagic,
		AttributesCu: attrCuNoPositiveTakenBonus, Effects: [3]serverdata.Effect{{Effect: effectHeal}}}
	target := &a.Unit
	target.PseudoStats.BonusHealingTaken = 290
	heal := func() float64 { return spell.CalcHealing(sim, target, 1000, spell.OutcomeHealing).Damage }

	target.AddHealingTakenPct(-10)
	// 1000 + 233, × 0.9 = 1109.7
	if got := heal(); got != 1109 {
		t.Errorf("under a -10%% aura healed %v, want 1559", got)
	}
	// 1.2 × 0.9 = 1.08, over 1
	target.AddHealingTakenPct(20)
	if got := heal(); got != 1000 {
		t.Errorf("under -10%% and +20%% auras healed %v, want 1000", got)
	}
	target.PseudoStats.HealingTakenMultiplier *= 1.3
	if got := heal(); got != 1000 {
		t.Errorf("with Strength of Wrynn's stand-in healed %v, want 1000", got)
	}
	// Earth Shield's 49284 has the attribute, and its heal is another spell
	spell.serverSpell.Effects[0] = serverdata.Effect{Effect: effectApplyAura, Aura: 4}
	// 1000 + 233, × 1.08 = 1331.6, without the stand-in
	if got := heal(); got != 1331 {
		t.Errorf("a spell with the attribute that doesn't heal healed %v, want 1331", got)
	}
}

// Unit::GetCastingTimeForBonus gives a creature that isn't a pet 3500 ms for an instant spell, so an instant heal
// without a spell_bonus_data row takes 290 healing taken on a guardian at 1.88: live, Empowered Renew's 63544 on a
// Shadowfiend gained 545, not 233. A real pet keeps the instant 1.5 of 3.5, and a hot keeps its share.
func TestInstantHealTakenCoefficientOnAGuardian(t *testing.T) {
	var guardian, pet, glyphed *timingTestPet
	timingTestConstruct = func(a *timingTestAgent) {
		noStats := func(stats.Stats) stats.Stats { return stats.Stats{} }
		guardian = &timingTestPet{Pet: NewPet("Guardian", &a.Character, stats.Stats{}, noStats, false, true)}
		pet = &timingTestPet{Pet: NewPet("Pet", &a.Character, stats.Stats{}, noStats, true, false)}
		pet.SummonedAsPet = true
		// the glyphed Water Elemental is built as a guardian and summoned as a pet
		glyphed = &timingTestPet{Pet: NewPet("Glyphed", &a.Character, stats.Stats{}, noStats, true, true)}
		glyphed.SummonedAsPet = true
		a.AddPet(guardian)
		a.AddPet(pet)
		a.AddPet(glyphed)
	}
	_, a := newTickTestSim(t, 0, 0, nil)
	timingTestConstruct = nil

	instant := &Spell{Unit: &a.Unit, HealingTakenCoefficient: DefaultHealingTakenCoefficient(0)}
	if got := PowerBonus(instant.healTakenCoefficient(&guardian.Unit), 290); got != 545 {
		t.Errorf("an instant heal on a guardian adds %v of 290, want 545", got)
	}
	if got := PowerBonus(instant.healTakenCoefficient(&pet.Unit), 290); got != 233 {
		t.Errorf("an instant heal on a pet adds %v of 290, want 233", got)
	}
	if got := PowerBonus(instant.healTakenCoefficient(&glyphed.Unit), 290); got != 233 {
		t.Errorf("an instant heal on a pet built as a guardian adds %v of 290, want 233", got)
	}
	if got := PowerBonus(instant.healTakenCoefficient(&a.Unit), 290); got != 233 {
		t.Errorf("an instant heal on a player adds %v of 290, want 233", got)
	}
	hot := &Spell{Unit: &a.Unit, HealingTakenCoefficient: DefaultHealingTakenCoefficient(6.0 / 15 / 2)}
	if got := hot.healTakenCoefficient(&guardian.Unit); got != DefaultHealingTakenCoefficient(6.0/15/2) {
		t.Errorf("a hot's tick on a guardian takes %v, want its own share's %v", got, DefaultHealingTakenCoefficient(6.0/15/2))
	}
}
