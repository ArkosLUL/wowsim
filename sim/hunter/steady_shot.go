package hunter

import (
	"time"

	"github.com/wowsims/wotlk/sim/core"
)

func (hunter *Hunter) registerSteadyShotSpell() {
	impSSProcChance := 0.05 * float64(hunter.Talents.ImprovedSteadyShot)
	if hunter.Talents.ImprovedSteadyShot > 0 {
		hunter.ImprovedSteadyShotAura = hunter.RegisterAura(core.Aura{
			Label:    "Improved Steady Shot",
			ActionID: core.ActionID{SpellID: 53220},
			Duration: time.Second * 12,
			OnGain: func(aura *core.Aura, sim *core.Simulation) {
				hunter.ArcaneShot.DamageMultiplierAdditive += .15
				hunter.ArcaneShot.CostMultiplier -= 0.2
				if hunter.AimedShot != nil {
					hunter.AimedShot.DamageMultiplierAdditive += .15
					hunter.AimedShot.CostMultiplier -= 0.2
				}
				if hunter.ChimeraShot != nil {
					hunter.ChimeraShot.DamageMultiplierAdditive += .15
					hunter.ChimeraShot.CostMultiplier -= 0.2
				}
			},
			OnExpire: func(aura *core.Aura, sim *core.Simulation) {
				hunter.ArcaneShot.DamageMultiplierAdditive -= .15
				hunter.ArcaneShot.CostMultiplier += 0.2
				if hunter.AimedShot != nil {
					hunter.AimedShot.DamageMultiplierAdditive -= .15
					hunter.AimedShot.CostMultiplier += 0.2
				}
				if hunter.ChimeraShot != nil {
					hunter.ChimeraShot.DamageMultiplierAdditive -= .15
					hunter.ChimeraShot.CostMultiplier += 0.2
				}
			},
			OnSpellHitDealt: func(aura *core.Aura, sim *core.Simulation, spell *core.Spell, result *core.SpellResult) {
				if spell == hunter.AimedShot || spell == hunter.ArcaneShot || spell == hunter.ChimeraShot {
					aura.Deactivate(sim)
				}
			},
		})
	}

	hunter.SteadyShot = hunter.RegisterSpell(core.SpellConfig{
		ActionID:    core.ActionID{SpellID: 49052},
		SpellSchool: core.SpellSchoolPhysical,
		ProcMask:    core.ProcMaskRangedSpecial,
		Flags:       core.SpellFlagMeleeMetrics | core.SpellFlagIncludeTargetBonusDamage | core.SpellFlagAPL,

		ManaCost: core.ManaCostOptions{
			BaseCost: 0.05,
			Multiplier: 1 -
				0.03*float64(hunter.Talents.Efficiency) -
				0.05*float64(hunter.Talents.MasterMarksman),
		},
		Cast: core.CastConfig{
			// 1.5 s plus the ranged slot's 500 ms, scaled by ranged attack speed (Unit::ModSpellCastTime)
			DefaultCast: core.Cast{
				GCD:      core.GCDDefault,
				CastTime: time.Second * 2,
			},
		},

		BonusCritRating: 0 +
			2*core.CritRatingPerCritChance*float64(hunter.Talents.SurvivalInstincts),
		DamageMultiplierAdditive: 1 +
			.03*float64(hunter.Talents.FerociousInspiration) +
			core.TernaryFloat64(hunter.HasSetBonus(ItemSetGronnstalker, 4), .1, 0),
		DamageMultiplier: 1 *
			hunter.markedForDeathMultiplier(),
		CritMultiplier:   hunter.critMultiplier(true, true),
		ThreatMultiplier: 1,

		ApplyEffects: func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
			// Spell::EffectSchoolDMG: a plain roll of the weapon's damage and ammo at the weapon's own
			// speed, neither normalized, plus spell_bonus_data's 0.1 ranged AP
			baseDamage := 0.1*spell.RangedAttackPower(target) +
				hunter.AutoAttacks.Ranged().BaseDamage(sim) +
				hunter.AmmoDamageBonus +
				252

			result := spell.CalcDamage(sim, target, baseDamage, spell.OutcomeRangedHitAndCrit)
			if result.Landed() && impSSProcChance > 0 && sim.RandomFloat("Imp Steady Shot") < impSSProcChance {
				hunter.ImprovedSteadyShotAura.Activate(sim)
			}
			spell.DealDamage(sim, result)
		},
	})
}
