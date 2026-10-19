package dps

import (
	"testing"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
	"github.com/wowsims/wotlk/sim/core/stats"
)

// markOfBloodSim is runeWeaponSim with Mark of Blood, the DK at 40,000 of 50,000 health.
func markOfBloodSim(t *testing.T) (*core.Simulation, *DpsDeathknight, *core.Unit) {
	t.Helper()
	sim, dk := runeWeaponSimWith(t, talentsWith(BloodTalents, 0, 14, '1'))
	if dk.MarkOfBlood == nil {
		t.Fatal("no Mark of Blood")
	}
	metrics := dk.NewHealthMetrics(core.ActionID{OtherID: proto.OtherAction_OtherActionHealingModel})
	dk.AddStatsDynamic(sim, stats.Stats{stats.Health: 50000 - dk.MaxHealth()})
	dk.GainHealth(sim, dk.MaxHealth(), metrics)
	dk.RemoveHealth(sim, 10000)
	return sim, dk, sim.GetTargetUnit(0)
}

// markOfBloodHit is a hit of the target's on the DK, as the marks on it see it.
func markOfBloodHit(t *testing.T, sim *core.Simulation, dk *DpsDeathknight, target *core.Unit, outcome core.HitOutcome) {
	t.Helper()
	target.OnSpellHitDealt(sim, markOfBloodSwing(target), &core.SpellResult{Target: &dk.Unit, Outcome: outcome, Damage: 1})
}

func markOfBloodSwing(target *core.Unit) *core.Spell {
	return target.GetOrRegisterSpell(core.SpellConfig{
		ActionID:    core.ActionID{OtherID: proto.OtherAction_OtherActionAttack},
		SpellSchool: core.SpellSchoolPhysical,
		ProcMask:    core.ProcMaskMeleeMHAuto,
		Flags:       core.SpellFlagNoMetrics,
	})
}

// 61607 heals the unit hit for 4% of its max health, through its healing taken auras only (no damage class, no
// bonus row). Its original caster is the mark's, so a heal on the DK himself is his and makes his threat.
func TestMarkOfBloodHealsTheUnitHit(t *testing.T) {
	sim, dk, target := markOfBloodSim(t)
	dk.MarkOfBlood.SkipCastAndApplyEffects(sim, target)
	mark := target.GetAura("MarkOfBlood-" + dk.Label)
	if !mark.IsActive() || mark.GetStacks() != 20 {
		t.Fatalf("the DK's mark is up %v with %d charges, want 20", mark.IsActive(), mark.GetStacks())
	}

	markOfBloodHit(t, sim, dk, target, core.OutcomeMiss)
	if dk.CurrentHealth() != 40000 || mark.GetStacks() != 20 {
		t.Errorf("a miss healed to %v and left %d charges, want 40000 and 20", dk.CurrentHealth(), mark.GetStacks())
	}
	// spell_proc's SpellTypeMask 1: a hit that deals nothing, a debuff going on, is no damage
	target.OnSpellHitDealt(sim, markOfBloodSwing(target), &core.SpellResult{Target: &dk.Unit, Outcome: core.OutcomeHit})
	if dk.CurrentHealth() != 40000 || mark.GetStacks() != 20 {
		t.Errorf("a hit dealing nothing healed to %v and left %d charges, want 40000 and 20", dk.CurrentHealth(), mark.GetStacks())
	}
	threat := dk.GainThreat(core.ActionID{SpellID: 49005})
	markOfBloodHit(t, sim, dk, target, core.OutcomeHit)
	if dk.CurrentHealth() != 42000 || mark.GetStacks() != 19 {
		t.Errorf("a hit healed to %v and left %d charges, want 42000 and 19", dk.CurrentHealth(), mark.GetStacks())
	}
	heal, bonus, ok := dk.HealThreatOf(61607, 2000)
	if got := dk.GainThreat(core.ActionID{SpellID: 49005}) - threat; !ok || heal == 0 || got != heal+bonus {
		t.Errorf("the heal made %v threat, want %v (%v)", got, heal+bonus, ok)
	}

	// Vampiric Blood's 35%: 2000 x 1.35
	dk.AddHealingTakenPct(35)
	markOfBloodHit(t, sim, dk, target, core.OutcomeHit)
	dk.RemoveHealingTakenPct(35)
	if dk.CurrentHealth() != 44700 {
		t.Errorf("a hit under 35%% healing taken healed to %v, want 44700", dk.CurrentHealth())
	}

	// DONE_PERIODIC: a tick of the target's dot on him
	target.OnPeriodicDamageDealt(sim, markOfBloodSwing(target), &core.SpellResult{Target: &dk.Unit, Outcome: core.OutcomeHit, Damage: 1})
	if dk.CurrentHealth() != 46700 || mark.GetStacks() != 17 {
		t.Errorf("a tick healed to %v and left %d charges, want 46700 and 17", dk.CurrentHealth(), mark.GetStacks())
	}

	// the last charge goes with the mark
	for mark.GetStacks() > 1 {
		markOfBloodHit(t, sim, dk, target, core.OutcomeHit)
	}
	markOfBloodHit(t, sim, dk, target, core.OutcomeHit)
	if mark.IsActive() {
		t.Error("the mark is up after its 20th charge")
	}
}

// spell_dk_dancing_rune_weapon copies Mark of Blood (CheckProc takes it, and 49028 procs on a non-damaging negative
// spell), and Aura::CanStackWith lets the weapon's mark replace the DK's: the same spell from another caster, no
// periodic effect, no ATTR3_DOT_STACKING_RULE. Its heal is the weapon's, so it makes him no threat. The mark is
// single target (ATTR5_LIMIT_N), so Unit::RemoveFromWorld takes it off the target once the weapon's unsummoned.
func TestRuneWeaponCopiesMarkOfBlood(t *testing.T) {
	sim, dk, target := markOfBloodSim(t)
	own, copied := target.GetAura("MarkOfBlood-"+dk.Label), target.GetAura("MarkOfBlood-"+dk.RuneWeapon.Label)
	if copied == nil {
		t.Fatal("the weapon has no mark")
	}

	// no weapon out, no copy
	dk.MarkOfBlood.SkipCastAndApplyEffects(sim, target)
	if !own.IsActive() || copied.IsActive() {
		t.Fatalf("without the weapon: the DK's mark up %v, the weapon's %v; want only his", own.IsActive(), copied.IsActive())
	}
	own.Deactivate(sim)

	if !dk.DancingRuneWeapon.Cast(sim, target) {
		t.Fatal("no Dancing Rune Weapon")
	}
	dk.SetCombatReach(0)
	dk.MarkOfBlood.SkipCastAndApplyEffects(sim, target)
	if own.IsActive() || !copied.IsActive() || copied.GetStacks() != 20 {
		t.Fatalf("with the weapon: the DK's mark up %v, the weapon's %v with %d charges; want the weapon's, 20",
			own.IsActive(), copied.IsActive(), copied.GetStacks())
	}
	// HandleProc leaves his reach at 0.01, so his Death Coil flies the whole 30 yd
	if got := dk.DeathCoilDamage.TravelTime().Milliseconds(); got != 1249 {
		t.Errorf("his Death Coil flies %v ms after the copy, want 1249", got)
	}

	threat := dk.GainThreat(core.ActionID{SpellID: 49005})
	markOfBloodHit(t, sim, dk, target, core.OutcomeHit)
	if dk.CurrentHealth() != 42000 || copied.GetStacks() != 19 {
		t.Errorf("a hit with the weapon out healed to %v and left %d charges, want 42000 and 19", dk.CurrentHealth(), copied.GetStacks())
	}
	if got := dk.GainThreat(core.ActionID{SpellID: 49005}); got != threat {
		t.Errorf("the weapon's heal made the DK %v threat", got-threat)
	}

	dk.RuneWeapon.Disable(sim)
	if copied.IsActive() || own.IsActive() {
		t.Fatalf("with the weapon gone: the weapon's mark up %v, the DK's %v; want neither", copied.IsActive(), own.IsActive())
	}
	markOfBloodHit(t, sim, dk, target, core.OutcomeHit)
	if dk.CurrentHealth() != 42000 {
		t.Errorf("a hit with the weapon gone healed to %v, want 42000", dk.CurrentHealth())
	}
}
