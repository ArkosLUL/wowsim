package druid

import (
	"time"

	"github.com/wowsims/wotlk/sim/core"
)

// FaerieFireFeralDamageActionID is the id Faerie Fire (Feral)'s Bear Form hit triggers for its
// damage (Spell::PrepareTriggersExecutedOnHit and DoTriggersOnSpellHit, Spell.cpp: only Bear and
// Dire Bear form set m_preCastSpell to it). 16857 itself only applies the armor and dodge/parry
// debuffs and stays binary; 60089 carries the AP coefficient, crits, and resists partially.
var FaerieFireFeralDamageActionID = core.ActionID{SpellID: 60089}

func (druid *Druid) registerFaerieFireSpell() {
	actionID := core.ActionID{SpellID: 770}
	manaCostOptions := core.ManaCostOptions{
		BaseCost: 0.08,
	}
	gcd := core.GCDDefault
	ignoreHaste := false
	cd := core.Cooldown{}
	flatThreatBonus := 66. * 2.
	flags := SpellFlagOmenTrigger
	formMask := Humanoid | Moonkin
	inFeralForm := druid.InForm(Cat | Bear)

	if inFeralForm {
		actionID = core.ActionID{SpellID: 16857}
		manaCostOptions = core.ManaCostOptions{}
		gcd = time.Second
		ignoreHaste = true
		// Its Omen of Clarity interaction is handled explicitly below (spell_tweaks_omen_faerie_fire),
		// so it should not also fall into the generic OmenTrigger roll.
		flags = core.SpellFlagNone
		formMask = Cat | Bear
		cd = core.Cooldown{
			Timer:    druid.NewTimer(),
			Duration: time.Second * 6,
		}
		flatThreatBonus = 632.
	}
	flags |= core.SpellFlagAPL

	druid.FaerieFireAuras = druid.NewEnemyAuraArray(func(target *core.Unit) *core.Aura {
		return core.FaerieFireAura(target, druid.Talents.ImprovedFaerieFire)
	})

	var feralDamage *DruidSpell
	if inFeralForm {
		feralDamage = druid.RegisterSpell(Any, core.SpellConfig{
			ActionID:    FaerieFireFeralDamageActionID,
			SpellSchool: core.SpellSchoolNature,
			ProcMask:    core.ProcMaskSpellDamage,
			Flags:       core.SpellFlagNoOnCastComplete,

			DamageMultiplier: 1,
			ThreatMultiplier: 1,
			CritMultiplier:   druid.BalanceCritMultiplier(),

			ApplyEffects: func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
				baseDamage := 1 + 0.15*spell.MeleeAttackPower()
				spell.CalcAndDealDamage(sim, target, baseDamage, spell.OutcomeMagicHitAndCrit)
			},
		})
	}

	druid.FaerieFire = druid.RegisterSpell(formMask, core.SpellConfig{
		ActionID:    actionID,
		SpellSchool: core.SpellSchoolNature,
		ProcMask:    core.ProcMaskSpellDamage,
		Flags:       flags,

		ManaCost: manaCostOptions,
		Cast: core.CastConfig{
			DefaultCast: core.Cast{
				GCD: gcd,
			},
			IgnoreHaste: ignoreHaste,
			CD:          cd,
		},

		ThreatMultiplier: 1,
		FlatThreatBonus:  flatThreatBonus,
		DamageMultiplier: 1,
		CritMultiplier:   druid.BalanceCritMultiplier(),

		ApplyEffects: func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
			result := spell.CalcAndDealDamage(sim, target, 0, spell.OutcomeMagicHit)
			if !result.Landed() {
				return
			}
			druid.FaerieFireAuras.Get(target).Activate(sim)
			if druid.InForm(Bear) {
				feralDamage.Cast(sim, target)
			}
		},

		RelatedAuras: []core.AuraArray{druid.FaerieFireAuras},
	})
}

func (druid *Druid) ShouldFaerieFire(sim *core.Simulation, target *core.Unit) bool {
	if druid.FaerieFire == nil {
		return false
	}

	if !druid.FaerieFire.IsReady(sim) {
		return false
	}

	return druid.FaerieFireAuras.Get(target).ShouldRefreshExclusiveEffects(sim, time.Second*3)
}
