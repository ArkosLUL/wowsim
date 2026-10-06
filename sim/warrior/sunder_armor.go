package warrior

import (
	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
)

// TODO: GlyphOfSunderArmor will require refactoring this a bit

func (warrior *Warrior) newSunderArmorSpell(isDevastateEffect bool) *core.Spell {
	warrior.SunderArmorAuras = warrior.NewEnemyAuraArray(core.SunderArmorAura)

	config := core.SpellConfig{
		ActionID:    core.ActionID{SpellID: 47467},
		SpellSchool: core.SpellSchoolPhysical,
		ProcMask:    core.ProcMaskMeleeMHSpecial,
		Flags:       core.SpellFlagMeleeMetrics,

		RageCost: core.RageCostOptions{
			Cost:   15 - float64(warrior.Talents.FocusedRage) - float64(warrior.Talents.Puncture),
			Refund: 0.8,
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

		ThreatMultiplier: 1,

		RelatedAuras: []core.AuraArray{warrior.SunderArmorAuras},
	}

	extraStack := isDevastateEffect && warrior.HasMajorGlyph(proto.WarriorMajorGlyph_GlyphOfDevastate)
	if isDevastateEffect {
		config.RageCost = core.RageCostOptions{}
		config.Cast.DefaultCast.GCD = 0
		config.ExtraCastCondition = nil

		// Spell::EffectWeaponDmg casts 58567, whose only threat is its level 10 bonus
		config.ThreatSpellID = 58567
	} else {
		config.Flags |= core.SpellFlagAPL
		// warriors know 7386 (character_spell), whose spell_threat row gives the bonus; 47467 is a rank
		// the server's warriors don't learn
		config.ThreatSpellID = 7386
	}

	config.ApplyEffects = func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
		var result *core.SpellResult
		if isDevastateEffect {
			result = spell.CalcOutcome(sim, target, spell.OutcomeAlwaysHit)
		} else {
			result = spell.CalcOutcome(sim, target, spell.OutcomeMeleeSpecialHit)
		}

		if result.Landed() {
			aura := warrior.SunderArmorAuras.Get(target)
			aura.Activate(sim)
			if aura.IsActive() {
				aura.AddStack(sim)
				if extraStack {
					aura.AddStack(sim)
				}
			}
			if !isDevastateEffect {
				// 7386 applies the debuff by casting 58567, which adds its own bonus
				warrior.SunderArmorDevastate.LandThreatBonus(sim, target)
			}
		} else {
			spell.IssueRefund(sim)
		}

		spell.DealOutcome(sim, result)
	}
	return warrior.RegisterSpell(config)
}

func (warrior *Warrior) CanApplySunderAura(target *core.Unit) bool {
	return warrior.SunderArmorAuras.Get(target).IsActive() || !warrior.SunderArmorAuras.Get(target).ExclusiveEffects[0].Category.AnyActive()
}
