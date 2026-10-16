package paladin

import (
	"time"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
	"github.com/wowsims/wotlk/sim/core/stats"
)

// LayOnHandsActionID is rank 5, always on the paladin himself.
var LayOnHandsActionID = core.ActionID{SpellID: 48788}

// registerLayOnHandsSpell is spell_pal_lay_on_hands cast on himself. The heal is his whole max health through
// SpellHealingBonusTaken only, so no done percent, and it can't crit. On himself it puts up Forbearance and the
// Avenging Wrath marker, the same lockouts Divine Protection checks and puts up.
func (paladin *Paladin) registerLayOnHandsSpell() {
	manaMetrics := paladin.NewManaMetrics(LayOnHandsActionID).EnergizeThreat(core.EnergizeBySpell)
	// Glyph of Divinity is +100% on effect 2, the energize
	mana := 1950 * core.TernaryFloat64(paladin.HasMajorGlyph(proto.PaladinMajorGlyph_GlyphOfDivinity), 2, 1)

	// Divine Protection's and Avenging Wrath's shared cooldown stands in for the 30 s Avenging Wrath marker
	marker := paladin.GetMutualLockoutDPAW()

	cd := time.Minute*20 -
		core.TernaryDuration(paladin.HasMinorGlyph(proto.PaladinMinorGlyph_GlyphOfLayOnHands), time.Minute*5, 0) -
		time.Minute*2*time.Duration(paladin.Talents.ImprovedLayOnHands)

	// Improved Lay on Hands' proc: 20233 or 20236 on the healed unit
	var physicalCut *core.Aura
	if rank := paladin.Talents.ImprovedLayOnHands; rank > 0 {
		multiplier := []float64{1, 0.9, 0.8}[rank]
		physicalCut = paladin.RegisterAura(core.Aura{
			Label:    "Improved Lay on Hands",
			ActionID: core.ActionID{SpellID: []int32{0, 20233, 20236}[rank]},
			Duration: time.Second * 15,
			OnGain: func(aura *core.Aura, sim *core.Simulation) {
				aura.Unit.PseudoStats.SchoolDamageTakenMultiplier[stats.SchoolIndexPhysical] *= multiplier
			},
			OnExpire: func(aura *core.Aura, sim *core.Simulation) {
				aura.Unit.PseudoStats.SchoolDamageTakenMultiplier[stats.SchoolIndexPhysical] /= multiplier
			},
		})
	}

	paladin.RegisterSpell(core.SpellConfig{
		ActionID:    LayOnHandsActionID,
		SpellSchool: core.SpellSchoolHoly,
		ProcMask:    core.ProcMaskSpellHealing,
		Flags:       core.SpellFlagAPL | core.SpellFlagHelpful | core.SpellFlagIgnoreAttackerModifiers,

		Cast: core.CastConfig{
			DefaultCast: core.Cast{
				GCD: core.GCDDefault,
			},
			CD: core.Cooldown{
				Timer:    paladin.NewTimer(),
				Duration: cd,
			},
		},
		ExtraCastCondition: func(sim *core.Simulation, target *core.Unit) bool {
			return !paladin.ForbearanceAura.IsActive() && marker.IsReady(sim)
		},

		DamageMultiplier: 1,
		CritMultiplier:   1,
		ThreatMultiplier: 1,

		ApplyEffects: func(sim *core.Simulation, _ *core.Unit, spell *core.Spell) {
			spell.CalcAndDealHealing(sim, &paladin.Unit, paladin.CountPctFromMaxHealth(100), spell.OutcomeHealing)
			paladin.AddMana(sim, mana, manaMetrics)
			if physicalCut != nil {
				physicalCut.Activate(sim)
			}
			paladin.ForbearanceAura.Activate(sim)
			marker.Set(sim.CurrentTime + 30*time.Second)
		},
	})
}
