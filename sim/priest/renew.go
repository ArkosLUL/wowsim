package priest

import (
	"math"
	"time"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
)

func (priest *Priest) registerRenewSpell() {
	actionID := core.ActionID{SpellID: 48068}

	if priest.Talents.EmpoweredRenew > 0 {
		priest.EmpoweredRenew = priest.RegisterSpell(core.SpellConfig{
			ActionID:    core.ActionID{SpellID: 63543},
			SpellSchool: core.SpellSchoolHoly,
			ProcMask:    core.ProcMaskSpellHealing,
			Flags:       core.SpellFlagNoOnCastComplete | core.SpellFlagHelpful,

			BonusCritRating: float64(priest.Talents.HolySpecialization) * 1 * core.CritRatingPerCritChance,
			// 63544's own done percents (SpellPctHealingModsDone): the healing done talents and Zabra's 4 piece
			DamageMultiplier: priest.renewHealingDoneMultiplier() *
				core.TernaryFloat64(priest.HasSetBonus(ItemSetZabrasRaiment, 4), 1.1, 1),
			CritMultiplier:   priest.DefaultHealingCritMultiplier(),
			ThreatMultiplier: 1,
			// the server heals as 63544, which has no spell_bonus_data row
			HealingTakenCoefficient: core.DefaultHealingTakenCoefficient(0),

			ApplyEffects: func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
				// spell_pri_renew: 5% a rank of every tick of the hot's amount as the target takes it, so 63544
				// takes the target's terms a second time
				hot := priest.Renew.Hot(target)
				tick := hot.TickHealingTaken(target, math.Trunc(hot.SnapshotBaseDamage*priest.renewDotMultiplier()))
				baseHealing := float64(5 * priest.Talents.EmpoweredRenew * priest.renewTicks() * int32(tick) / 100)
				spell.CalcAndDealHealing(sim, target, baseHealing, spell.OutcomeHealingCrit)
			},
		})
	}

	priest.Renew = priest.RegisterSpell(core.SpellConfig{
		ActionID:    actionID,
		SpellSchool: core.SpellSchoolHoly,
		ProcMask:    core.ProcMaskSpellHealing,
		Flags:       core.SpellFlagHelpful | core.SpellFlagAPL,

		ManaCost: core.ManaCostOptions{
			BaseCost:   0.17,
			Multiplier: 1 - []float64{0, .04, .07, .10}[priest.Talents.MentalAgility],
		},
		Cast: core.CastConfig{
			DefaultCast: core.Cast{
				GCD: core.GCDDefault,
			},
		},

		DamageMultiplier: priest.renewHealingMultiplier(),
		ThreatMultiplier: 1,

		Mods: []core.SpellMod{
			{Op: core.SpellModBonusMultiplier, Pct: 5 * int32(priest.Talents.EmpoweredRenew)},
		},

		Hot: core.DotConfig{
			Aura: core.Aura{
				Label: "Renew",
			},
			NumberOfTicks: priest.renewTicks(),
			TickLength:    time.Second * 3,
			// No periodic-crit aura covers Renew in 3.3.5; HoTs don't crit.
			TicksCanCrit: false,
			Tick:         core.SpellEffect{Effect: 0, Min: 280, Max: 280, SP: 0.376},
			OnSnapshot: func(sim *core.Simulation, target *core.Unit, dot *core.Dot, _ bool) {
				dot.SnapshotBaseDamage = dot.Tick.Roll(sim) + core.PowerBonus(dot.Tick.SP, dot.Spell.HealingPower(target))
				dot.SnapshotAttackerMultiplier = dot.Spell.CasterHealingMultiplier()
			},
			OnTick: func(sim *core.Simulation, target *core.Unit, dot *core.Dot) {
				dot.CalcAndDealPeriodicSnapshotHealing(sim, target, dot.OutcomeTick)
			},
		},

		ApplyEffects: func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
			spell.SpellMetrics[target.UnitIndex].Hits++
			spell.Hot(target).Apply(sim)
			spell.LandOn(sim, target)

			if priest.EmpoweredRenew != nil {
				priest.EmpoweredRenew.Cast(sim, target)
			}
		},
	})
}

func (priest *Priest) renewTicks() int32 {
	return 5 - core.TernaryInt32(priest.HasMajorGlyph(proto.PriestMajorGlyph_GlyphOfRenew), 1, 0)
}

func (priest *Priest) renewHealingMultiplier() float64 {
	return 1 *
		(1 + .02*float64(priest.Talents.SpiritualHealing)) *
		(1 + .01*float64(priest.Talents.BlessedResilience)) *
		(1 + .02*float64(priest.Talents.FocusedPower)) *
		(1 + .01*float64(priest.Talents.TwinDisciplines)) *
		(1 + .05*float64(priest.Talents.ImprovedRenew)) *
		core.TernaryFloat64(priest.HasMajorGlyph(proto.PriestMajorGlyph_GlyphOfRenew), 1.25, 1)
}

// renewDotMultiplier is Renew's SPELLMOD_DOT talents and glyph, the part of renewHealingMultiplier its aura's
// amount carries.
func (priest *Priest) renewDotMultiplier() float64 {
	return 1 *
		(1 + .02*float64(priest.Talents.SpiritualHealing)) *
		(1 + .01*float64(priest.Talents.TwinDisciplines)) *
		(1 + .05*float64(priest.Talents.ImprovedRenew)) *
		core.TernaryFloat64(priest.HasMajorGlyph(proto.PriestMajorGlyph_GlyphOfRenew), 1.25, 1)
}

// renewHealingDoneMultiplier is the rest: the SPELL_AURA_MOD_HEALING_DONE_PERCENT talents, which a heal
// takes after its amount.
func (priest *Priest) renewHealingDoneMultiplier() float64 {
	return 1 *
		(1 + .01*float64(priest.Talents.BlessedResilience)) *
		(1 + .02*float64(priest.Talents.FocusedPower))
}
