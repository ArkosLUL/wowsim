package priest

import (
	"math"
	"time"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
)

func (priest *Priest) registerCircleOfHealingSpell() {
	if !priest.Talents.CircleOfHealing {
		return
	}

	numTargets := 5 + core.TernaryInt32(priest.HasMajorGlyph(proto.PriestMajorGlyph_GlyphOfCircleOfHealing), 1, 0)
	units := priest.Env.Raid.GetFirstNPlayersOrPets(math.MaxInt32)

	priest.CircleOfHealing = priest.RegisterSpell(core.SpellConfig{
		ActionID:    core.ActionID{SpellID: 48089},
		SpellSchool: core.SpellSchoolHoly,
		ProcMask:    core.ProcMaskSpellHealing,
		Flags:       core.SpellFlagHelpful | core.SpellFlagAPL,

		ManaCost: core.ManaCostOptions{
			BaseCost:   0.21,
			Multiplier: 1 - []float64{0, .04, .07, .10}[priest.Talents.MentalAgility],
		},
		Cast: core.CastConfig{
			DefaultCast: core.Cast{
				GCD: core.GCDDefault,
			},
			CD: core.Cooldown{
				Timer:    priest.NewTimer(),
				Duration: time.Second * 6,
			},
		},

		BonusCritRating: float64(priest.Talents.HolySpecialization) * 1 * core.CritRatingPerCritChance,
		DamageMultiplier: 1 *
			(1 + .02*float64(priest.Talents.SpiritualHealing)) *
			(1 + .01*float64(priest.Talents.BlessedResilience)) *
			(1 + .02*float64(priest.Talents.FocusedPower)) *
			(1 + .02*float64(priest.Talents.DivineProvidence)) *
			core.TernaryFloat64(priest.HasSetBonus(ItemSetCrimsonAcolytesRaiment, 4), 1.1, 1),
		CritMultiplier:   priest.DefaultHealingCritMultiplier(),
		ThreatMultiplier: 1,

		Direct: core.SpellEffect{Effect: 0, Min: 958, Max: 1058, SP: 0.402},

		ApplyEffects: func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
			healFromSP := core.PowerBonus(spell.Direct.SP, spell.HealingPower(target))
			healed := int32(0)
			for _, aoeTarget := range units {
				// a pet that's away isn't in the world to be healed
				if healed == numTargets {
					break
				}
				if !aoeTarget.IsEnabled() {
					continue
				}
				healed++
				baseHealing := spell.Direct.Roll(sim) + healFromSP
				spell.CalcAndDealHealing(sim, aoeTarget, baseHealing, spell.OutcomeHealingCrit)
			}
		},
	})
}
