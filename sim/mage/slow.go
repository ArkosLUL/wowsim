package mage

import (
	"time"

	"github.com/wowsims/wotlk/sim/core"
)

// Slow is outside every class mask that SpellFlagMage stands for: Clearcasting and Arcane Power leave its cost
// alone, and Arcane Concentration only takes damage (spell_proc -11213).
func (mage *Mage) registerSlowSpell() {
	if !mage.Talents.Slow {
		return
	}

	// Three effects for 15 s: a snare, 60% off the ranged attack speed and 30% off the cast speed. Only the
	// snare has a mechanic, so a snare-immune target takes the other two and gives Torment the Weak nothing.
	// The cast time debuffs don't stack, the strongest one counts (spell_group 1096)
	slows := mage.NewEnemyAuraArray(func(target *core.Unit) *core.Aura {
		tag := core.SnareAuraTag
		if target.ImmuneToMechanic(core.MechanicSnare) {
			tag = ""
		}
		aura := target.GetOrRegisterAura(core.Aura{
			Label:    "Slow by " + mage.Label,
			ActionID: core.ActionID{SpellID: 31589},
			Tag:      tag,
			Duration: time.Second * 15,
		})
		aura.NewExclusiveEffect("CastTimeDebuff", false, core.ExclusiveEffect{
			Priority: 1.3,
			OnGain: func(ee *core.ExclusiveEffect, sim *core.Simulation) {
				ee.Aura.Unit.MultiplyCastSpeed(1 / 1.3)
				ee.Aura.Unit.MultiplyRangedSpeed(sim, 1/1.6)
			},
			OnExpire: func(ee *core.ExclusiveEffect, sim *core.Simulation) {
				ee.Aura.Unit.MultiplyCastSpeed(1.3)
				ee.Aura.Unit.MultiplyRangedSpeed(sim, 1.6)
			},
		})
		return aura
	})

	mage.Slow = mage.RegisterSpell(core.SpellConfig{
		ActionID:    core.ActionID{SpellID: 31589},
		SpellSchool: core.SpellSchoolArcane,
		ProcMask:    core.ProcMaskSpellDamage,
		Flags:       core.SpellFlagAPL,

		ManaCost: core.ManaCostOptions{
			BaseCost: 0.12,
		},
		Cast: core.CastConfig{
			DefaultCast: core.Cast{
				GCD: core.GCDDefault,
			},
		},

		BonusHitRating:   float64(mage.Talents.ArcaneFocus) * core.SpellHitRatingPerHitChance,
		ThreatMultiplier: 1,

		ApplyEffects: func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
			result := spell.CalcOutcome(sim, target, spell.OutcomeMagicHit)
			if result.Landed() {
				// one target at a time (SPELL_ATTR5_LIMIT_N)
				for _, aura := range slows {
					if aura != nil && aura.Unit != target {
						aura.Deactivate(sim)
					}
				}
				slows.Get(target).Activate(sim)
			}
			spell.DealOutcome(sim, result)
		},

		RelatedAuras: []core.AuraArray{slows},
	})
}
