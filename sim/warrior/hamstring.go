package warrior

import (
	"time"

	"github.com/wowsims/wotlk/sim/core"
)

// Hamstring deals nothing: it's a 15 s snare, which a mage's Torment the Weak reads. The snare is its only
// effect, so a snare-immune target is immune to the whole spell (Creature::IsImmunedToSpell).
func (warrior *Warrior) registerHamstringSpell() {
	snares := warrior.NewEnemyAuraArray(func(target *core.Unit) *core.Aura {
		if target.ImmuneToMechanic(core.MechanicSnare) {
			return nil
		}
		return target.GetOrRegisterAura(core.Aura{
			Label:    "Hamstring by " + warrior.Label,
			ActionID: core.ActionID{SpellID: 1715},
			Tag:      core.SnareAuraTag,
			Duration: time.Second * 15,
		})
	})

	warrior.Hamstring = warrior.RegisterSpell(core.SpellConfig{
		ActionID:    core.ActionID{SpellID: 1715},
		SpellSchool: core.SpellSchoolPhysical,
		ProcMask:    core.ProcMaskMeleeMHSpecial,
		Flags:       core.SpellFlagMeleeMetrics | core.SpellFlagAPL,

		RageCost: core.RageCostOptions{
			Cost: 10 - float64(warrior.Talents.FocusedRage),
		},
		Cast: core.CastConfig{
			DefaultCast: core.Cast{
				GCD: core.GCDDefault,
			},
			IgnoreHaste: true,
		},
		ExtraCastCondition: func(sim *core.Simulation, target *core.Unit) bool {
			return warrior.StanceMatches(BattleStance | BerserkerStance)
		},

		ThreatMultiplier: 1,

		ApplyEffects: func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
			snare := snares.Get(target)
			if snare == nil {
				spell.IssueRefund(sim)
				spell.CalcAndDealOutcome(sim, target, spell.OutcomeAlwaysMiss)
				return
			}

			result := spell.CalcOutcome(sim, target, spell.OutcomeMeleeSpecialHit)
			if result.Landed() {
				snare.Activate(sim)
			} else if !result.Outcome.Matches(core.OutcomeBlock) {
				// a full block costs the whole 10 rage (Spell::TakePower)
				spell.IssueRefund(sim)
			}
			spell.DealOutcome(sim, result)
		},
	})
}
