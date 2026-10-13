package warrior

import (
	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
)

func (warrior *Warrior) registerDevastateSpell() {
	if !warrior.Talents.Devastate {
		return
	}

	hasGlyph := warrior.HasMajorGlyph(proto.WarriorMajorGlyph_GlyphOfDevastate)
	sunderStacks := core.TernaryInt32(hasGlyph, 2, 1)

	overallMulti := core.TernaryFloat64(warrior.HasSetBonus(ItemSetWrynnsPlate, 2), 1.05, 1.00)

	warrior.Devastate = warrior.RegisterSpell(core.SpellConfig{
		ActionID:    core.ActionID{SpellID: 47498},
		SpellSchool: core.SpellSchoolPhysical,
		ProcMask:    core.ProcMaskMeleeMHSpecial,
		Flags:       core.SpellFlagMeleeMetrics | core.SpellFlagAPL,

		RageCost: core.RageCostOptions{
			Cost: 15 - float64(warrior.Talents.FocusedRage) - float64(warrior.Talents.Puncture),
		},
		Cast: core.CastConfig{
			DefaultCast: core.Cast{
				GCD: core.GCDDefault,
			},
			IgnoreHaste: true,
		},
		ExtraCastCondition: func(sim *core.Simulation, target *core.Unit) bool {
			return warrior.CanApplySunderAura(target)
		},

		BonusCritRating: 5*core.CritRatingPerCritChance*float64(warrior.Talents.SwordAndBoard) +
			core.TernaryFloat64(warrior.HasSetBonus(ItemSetSiegebreakerPlate, 2), 10*core.CritRatingPerCritChance, 0),

		DamageMultiplier: overallMulti,
		CritMultiplier:   warrior.critMultiplier(),
		ThreatMultiplier: 1,

		// the percent effect comes before the flat one, so it leaves the flat alone
		Direct: core.SpellEffect{Effect: 2, Min: 242, Max: 242, WeaponPct: 1.2},

		ApplyEffects: func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
			// Spell::EffectWeaponDmg applies this Devastate's Sunder Armor first, then deals the flat once
			// per stack
			stacks := min(warrior.SunderArmorAuras.Get(target).GetStacks()+sunderStacks, 5)

			baseDamage := core.ApplyPct(spell.MHNormalizedStrikeDamage(sim, spell.MeleeAttackPower()), spell.Direct.WeaponPct) +
				spell.StrikeFixed(spell.Direct.Roll(sim), core.MainHand) + spell.StrikeFixed(spell.Direct.Roll(sim)*float64(stacks-1), core.MainHand)

			result := spell.CalcDamage(sim, target, baseDamage, spell.OutcomeMeleeSpecialHitAndCrit)
			spell.DealDamage(sim, result)

			if result.Landed() {
				if warrior.CanApplySunderAura(target) {
					warrior.SunderArmorDevastate.Cast(sim, target)
				} else {
					// 58567 is cast and lands either way, so it adds its bonus
					warrior.SunderArmorDevastate.LandThreatBonus(sim, target)
				}
			} else {
				spell.IssueRefund(sim)
			}
		},

		RelatedAuras: []core.AuraArray{warrior.SunderArmorAuras},
	})
}
