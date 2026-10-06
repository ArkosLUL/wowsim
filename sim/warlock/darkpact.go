package warlock

import (
	"math"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
	"github.com/wowsims/wotlk/sim/core/stats"
)

func (warlock *Warlock) registerDarkPactSpell() {
	if !warlock.Talents.DarkPact {
		return
	}

	actionID := core.ActionID{SpellID: 59092}
	baseRestore := 1200.0
	// SPELL_EFFECT_POWER_DRAIN energizes the caster with what it drains
	manaMetrics := warlock.NewManaMetrics(actionID).EnergizeThreat(core.EnergizeBySpell)
	var petManaMetrics *core.ResourceMetrics
	if warlock.Pet != nil {
		petManaMetrics = warlock.Pet.NewManaMetrics(actionID)
	}
	hasGlyph := warlock.HasMajorGlyph(proto.WarlockMajorGlyph_GlyphOfLifeTap)

	warlock.DarkPact = warlock.RegisterSpell(core.SpellConfig{
		ActionID:    actionID,
		SpellSchool: core.SpellSchoolShadow,
		ProcMask:    core.ProcMaskEmpty,
		Flags:       core.SpellFlagAPL,

		Cast: core.CastConfig{
			DefaultCast: core.Cast{
				GCD: core.GCDDefault,
			},
		},

		ExtraCastCondition: func(sim *core.Simulation, target *core.Unit) bool {
			return warlock.Pet != nil
		},

		ThreatMultiplier: 1 - 0.1*float64(warlock.Talents.ImprovedDrainSoul),

		ApplyEffects: func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
			// Glyph activates and applies SP before coeff calculations are done.
			if hasGlyph {
				warlock.GlyphOfLifeTapAura.Activate(sim)
			}

			maxDrain := baseRestore + 0.96*warlock.GetStat(stats.SpellPower)
			actualDrain := math.Min(maxDrain, warlock.Pet.CurrentMana())

			warlock.Pet.SpendMana(sim, actualDrain, petManaMetrics)
			warlock.AddMana(sim, actualDrain, manaMetrics)
		},
	})
}
