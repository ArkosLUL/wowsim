package druid

import (
	"time"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/stats"
)

func (druid *Druid) registerSavageDefensePassive() {
	if !druid.InForm(Bear) {
		return
	}

	// 62606's spell_proc row spends its one charge on any landed melee or ranged hit of any school,
	// so a hit the physical absorb skips still uses the shield up.
	druid.SavageDefenseAura = druid.RegisterAura(core.Aura{
		Label:    "Savage Defense",
		ActionID: core.ActionID{SpellID: 62606},
		Duration: 10 * time.Second,
		OnSpellHitTaken: func(aura *core.Aura, sim *core.Simulation, spell *core.Spell, result *core.SpellResult) {
			if result.Landed() && spell.ProcMask.Matches(core.ProcMaskMeleeOrRanged) {
				aura.Deactivate(sim)
			}
		},
	})

	// spell_dru_savage_defense::Absorb zeroes the shield after one absorb, so CalcAbsorbResist
	// removes it right there.
	druid.AddDynamicDamageTakenModifier(func(sim *core.Simulation, spell *core.Spell, result *core.SpellResult) {
		if druid.SavageDefenseAura.IsActive() && (result.Damage > 0) && spell.SpellSchool.Matches(core.SpellSchoolPhysical) {
			result.Damage = max(0, result.Damage-0.25*druid.GetStat(stats.AttackPower))
			druid.SavageDefenseAura.Deactivate(sim)
		}
	})

	// 62600's spell_proc row: crits only, from auto attacks, melee-class spells and periodic damage.
	// Magic class hits like Faerie Fire (Feral) don't proc it.
	core.MakePermanent(druid.RegisterAura(core.Aura{
		Label: "Savage Defense Trigger",
		OnSpellHitDealt: func(_ *core.Aura, sim *core.Simulation, spell *core.Spell, result *core.SpellResult) {
			if result.DidCrit() && spell.ProcMask.Matches(core.ProcMaskMelee) {
				druid.SavageDefenseAura.Activate(sim)
			}
		},
		OnPeriodicDamageDealt: func(_ *core.Aura, sim *core.Simulation, _ *core.Spell, result *core.SpellResult) {
			if result.DidCrit() {
				druid.SavageDefenseAura.Activate(sim)
			}
		},
	}))
}
