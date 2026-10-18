package priest

import (
	"cmp"
	"math"
	"slices"
	"time"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
)

func (priest *Priest) registerCircleOfHealingSpell() {
	if !priest.Talents.CircleOfHealing {
		return
	}

	numTargets := 5 + core.TernaryInt(priest.HasMajorGlyph(proto.PriestMajorGlyph_GlyphOfCircleOfHealing), 1, 0)
	units := priest.Env.Raid.GetFirstNPlayersOrPets(math.MaxInt32)
	targets := make([]*core.Unit, 0, len(units))

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
			targets = circleOfHealingTargets(targets[:0], units, numTargets)
			for _, aoeTarget := range targets {
				baseHealing := spell.Direct.Roll(sim) + healFromSP
				spell.CalcAndDealHealing(sim, aoeTarget, baseHealing, spell.OutcomeHealingCrit)
			}
		},
	})
}

// circleOfHealingTargets is spell_pri_circle_of_healing's pick: past n living raid members and pets, the n with
// the lowest health percent. std::list::sort is stable, so a tie keeps the search order, raid order here. No
// positions in the sim, so everyone's in range.
func circleOfHealingTargets(targets []*core.Unit, units []*core.Unit, n int) []*core.Unit {
	for _, unit := range units {
		// a pet that's away isn't in the world to be healed, and SpellInfo::CheckTarget keeps the dead out
		if !unit.IsEnabled() || (unit.HasHealthBar() && unit.CurrentHealth() <= 0) {
			continue
		}
		targets = append(targets, unit)
	}
	if len(targets) > n {
		slices.SortStableFunc(targets, func(a, b *core.Unit) int {
			return cmp.Compare(healthPctOrder(a), healthPctOrder(b))
		})
		targets = targets[:n]
	}
	return targets
}

// healthPctOrder is Acore::HealthPctOrderPred's float ratio of whole health, full for a unit with no health bar.
func healthPctOrder(unit *core.Unit) float32 {
	if !unit.HasHealthBar() {
		return 1
	}
	cur, most := uint32(unit.CurrentHealth()), uint32(unit.MaxHealth())
	// health can sit above max after max health drops, where the server caps it
	return float32(min(cur, most)) / float32(most)
}
