package paladin

import (
	"github.com/wowsims/wotlk/sim/core"
)

// registerSealJudgementSpell is 54158, which spell_pal_judgement casts beside the judgement's debuff under a seal
// with no judgement of its own in effect 2: Seal of Wisdom and Seal of Light.
func (paladin *Paladin) registerSealJudgementSpell() {
	paladin.SealJudgement = paladin.RegisterSpell(core.SpellConfig{
		ActionID:    core.ActionID{SpellID: 54158},
		SpellSchool: core.SpellSchoolHoly,
		// main hand only: a seal's PPM reads this hand's speed
		ProcMask: core.ProcMaskMeleeMHSpecial,
		Flags:    core.SpellFlagMeleeMetrics | SpellFlagSecondaryJudgement,

		BonusCritRating: (6 * float64(paladin.Talents.Fanaticism) * core.CritRatingPerCritChance) +
			(core.TernaryFloat64(paladin.HasSetBonus(ItemSetTuralyonsBattlegear, 4), 5, 0) * core.CritRatingPerCritChance),

		DamageMultiplier: spellModDamage(paladin.getItemSetLightswornBattlegearBonus4(), paladin.getTalentJudgementsOfThePureBonus(), paladin.getMajorGlyphOfJudgementBonus(), paladin.getTalentTheArtOfWarBonus()),
		CritMultiplier:   paladin.MeleeCritMultiplier(),
		ThreatMultiplier: 1,

		Direct: core.SpellEffect{Effect: 0, Min: 1, Max: 1, SP: 0.25, AP: 0.16},

		ApplyEffects: func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
			baseDamage := spell.Direct.Roll(sim) +
				core.PowerBonus(spell.Direct.AP, spell.MeleeAttackPower()) +
				core.PowerBonus(spell.Direct.SP, spell.SpellPower())
			// it lands where the judgement did, only rolling for crit
			spell.CalcAndDealDamage(sim, target, baseDamage, spell.OutcomeMeleeSpecialCritOnly)
		},
	})
}

// registerSealAura is a seal of 30 minutes whose judgement is 54158 and whose melee hits roll onProc at its
// spell_proc PPM. spell_pal_seal_of_light keeps it off hits on the paladin himself, which the sim never deals.
func (paladin *Paladin) registerSealAura(label string, spellID int32, onProc func(sim *core.Simulation)) *core.Aura {
	ppm := core.ServerProcFor(spellID).PPM
	return paladin.RegisterAura(core.Aura{
		Label:    label,
		Tag:      "Seal",
		ActionID: core.ActionID{SpellID: spellID},
		Duration: SealDuration,

		OnSpellHitDealt: func(aura *core.Aura, sim *core.Simulation, spell *core.Spell, result *core.SpellResult) {
			if !result.Landed() {
				return
			}
			if spell.Flags.Matches(SpellFlagPrimaryJudgement) {
				paladin.SealJudgement.Cast(sim, result.Target)
				return
			}
			// 54158 is melee class with SPELL_ATTR3_NOT_A_PROC, so it rolls too
			if paladin.sealCanProcOn(spell) && sim.RandomFloat(label) < paladin.AuraPPMProcChance(ppm, spell) {
				onProc(sim)
			}
		},
	})
}

// registerSealSpell is the cast that puts a seal up in place of the current one.
func (paladin *Paladin) registerSealSpell(aura *core.Aura) *core.Spell {
	return paladin.RegisterSpell(core.SpellConfig{
		ActionID:    aura.ActionID,
		SpellSchool: core.SpellSchoolHoly,
		ProcMask:    core.ProcMaskEmpty,
		Flags:       core.SpellFlagAPL,

		ManaCost: core.ManaCostOptions{
			BaseCost:   0.14,
			Multiplier: 1 - 0.02*float64(paladin.Talents.Benediction),
		},
		Cast: core.CastConfig{
			DefaultCast: core.Cast{
				GCD: core.GCDDefault,
			},
		},

		ThreatMultiplier: 1,
		ApplyEffects: func(sim *core.Simulation, _ *core.Unit, spell *core.Spell) {
			spell.LandOn(sim, &paladin.Unit)
			if paladin.CurrentSeal != nil {
				paladin.CurrentSeal.Deactivate(sim)
			}
			paladin.CurrentSeal = aura
			paladin.CurrentSeal.Activate(sim)
		},
	})
}

// registerSealOfWisdomSpellAndAura is 20166, whose 20168 gives 4% of the paladin's max mana (SPELL_EFFECT_ENERGIZE_PCT).
func (paladin *Paladin) registerSealOfWisdomSpellAndAura() {
	manaMetrics := paladin.NewManaMetrics(core.ActionID{SpellID: 20168}).EnergizeThreat(core.EnergizeBySpell)

	paladin.SealOfWisdomAura = paladin.registerSealAura("Seal of Wisdom", 20166, func(sim *core.Simulation) {
		paladin.AddMana(sim, float64(uint32(float32(uint32(paladin.MaxMana()))*4/100)), manaMetrics)
	})
	paladin.SealOfWisdom = paladin.registerSealSpell(paladin.SealOfWisdomAura)
}

// registerSealOfLightSpellAndAura is 20165, whose 20167 heals the paladin for 15% of his spell power and attack
// power. It can't crit (SPELL_ATTR2_CANT_CRIT).
func (paladin *Paladin) registerSealOfLightSpellAndAura() {
	heal := paladin.RegisterSpell(core.SpellConfig{
		ActionID:    core.ActionID{SpellID: 20167},
		SpellSchool: core.SpellSchoolHoly,
		ProcMask:    core.ProcMaskSpellHealing,
		Flags:       core.SpellFlagHelpful | core.SpellFlagNoOnCastComplete,

		DamageMultiplier: paladin.healingDoneMultiplier(),
		CritMultiplier:   1,
		ThreatMultiplier: 1,

		Direct: core.SpellEffect{Effect: 0, Min: 0, Max: 0, SP: 0.15, AP: 0.15},

		ApplyEffects: func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
			baseHealing := core.PowerBonus(spell.Direct.AP, spell.MeleeAttackPower()) +
				core.PowerBonus(spell.Direct.SP, spell.HealingPower(target))
			spell.CalcAndDealHealing(sim, target, baseHealing*paladin.healingDoneFactor(false), spell.OutcomeHealing)
		},
	})

	paladin.SealOfLightAura = paladin.registerSealAura("Seal of Light", 20165, func(sim *core.Simulation) {
		heal.Cast(sim, &paladin.Unit)
	})
	paladin.SealOfLight = paladin.registerSealSpell(paladin.SealOfLightAura)
}
