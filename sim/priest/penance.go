package priest

import (
	"time"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
)

func (priest *Priest) registerPenanceHealSpell() {
	priest.PenanceHeal = priest.makePenanceSpell(true)
}

func (priest *Priest) RegisterPenanceSpell() {
	priest.Penance = priest.makePenanceSpell(false)
}

func (priest *Priest) makePenanceSpell(isHeal bool) *core.Spell {
	if !priest.Talents.Penance {
		return nil
	}

	var procMask core.ProcMask
	flags := core.SpellFlagChanneled | core.SpellFlagAPL
	// 53007 and its channel make no threat; each tick is a cast of its own
	var tickSpellID int32
	if isHeal {
		flags |= core.SpellFlagHelpful
		procMask = core.ProcMaskSpellHealing
		tickSpellID = 52985
	} else {
		procMask = core.ProcMaskSpellDamage
		tickSpellID = 53000
	}
	var tickThreat *core.ThreatTick

	spell := priest.RegisterSpell(core.SpellConfig{
		ActionID:    core.ActionID{SpellID: 53007},
		SpellSchool: core.SpellSchoolHoly,
		ProcMask:    procMask,
		Flags:       flags,
		// the heal is 52985, with this spell_bonus_data row
		HealingTakenCoefficient: 0.5362,

		ManaCost: core.ManaCostOptions{
			BaseCost: 0.16,
			// Mental Agility's class mask leaves Penance out
			Multiplier: 1 - 0.05*float64(priest.Talents.ImprovedHealing),
		},
		Cast: core.CastConfig{
			DefaultCast: core.Cast{
				GCD: core.GCDDefault,
			},
			CD: core.Cooldown{
				Timer:    priest.NewTimer(),
				Duration: time.Duration(float64(time.Second*12-core.TernaryDuration(priest.HasMajorGlyph(proto.PriestMajorGlyph_GlyphOfPenance), time.Second*2, 0)) * (1 - .1*float64(priest.Talents.Aspiration))),
			},
		},

		// each talent is its own percent mod or done aura on the tick spell, so they all multiply
		DamageMultiplier: core.TernaryFloat64(isHeal,
			(1+.02*float64(priest.Talents.SpiritualHealing))*
				(1+.01*float64(priest.Talents.BlessedResilience))*
				(1+.02*float64(priest.Talents.FocusedPower))*
				(1+.01*float64(priest.Talents.TwinDisciplines)),
			(1+.05*float64(priest.Talents.SearingLight))*(1+.01*float64(priest.Talents.TwinDisciplines))),
		CritMultiplier:   core.TernaryFloat64(isHeal, priest.DefaultHealingCritMultiplier(), priest.DefaultSpellCritMultiplier()),
		ThreatMultiplier: 1,
		ThreatSpellID:    tickSpellID,
		NoThreatBonus:    true,

		Dot: core.Ternary(!isHeal, core.DotConfig{
			Aura: core.Aura{
				Label: "Penance",
			},
			NumberOfTicks:       2,
			TickLength:          time.Second,
			TicksCanCrit:        false,
			AffectedByCastSpeed: true,
			// spell_bonus_data's 0.1905 wins over 53000's own 0.229
			Tick: core.SpellEffect{FromSpellID: tickSpellID, Effect: 0, Min: 375, Max: 375, SP: 0.1905},

			OnTick: func(sim *core.Simulation, target *core.Unit, dot *core.Dot) {
				baseDamage := dot.Tick.Roll(sim) + core.PowerBonus(dot.Tick.SP, dot.Spell.SpellPower())
				result := dot.Spell.CalcAndDealPeriodicDamage(sim, target, baseDamage, dot.Spell.OutcomeMagicHitAndCrit)
				tickThreat.Hit(result)
				tickThreat.Done(sim)
			},
		}, core.DotConfig{}),
		Hot: core.Ternary(isHeal, core.DotConfig{
			Aura: core.Aura{
				Label: "Penance",
			},
			NumberOfTicks:       2,
			TickLength:          time.Second,
			TicksCanCrit:        false,
			AffectedByCastSpeed: true,

			OnTick: func(sim *core.Simulation, target *core.Unit, dot *core.Dot) {
				baseHealing := sim.RollWhole(1484, 1676, "Damage Roll") + core.PowerBonus(0.5362, dot.Spell.HealingPower(target))
				dot.Spell.CalcAndDealPeriodicHealing(sim, target, baseHealing, dot.Spell.OutcomeHealingCrit)
				tickThreat.Done(sim)
			},
		}, core.DotConfig{}),

		ApplyEffects: func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
			if isHeal {
				spell.SpellMetrics[target.UnitIndex].Hits--
				hot := spell.Hot(target)
				hot.Apply(sim)
				// Do immediate tick
				hot.TickOnce(sim)
			} else {
				// 53007 and the channel it starts aren't magic class, so they can't miss and proc nothing a
				// harmful magic spell does (Illustration of the Dragon Soul). Each tick rolls its own hit.
				dot := spell.Dot(target)
				dot.Apply(sim)
				// Do immediate tick
				dot.TickOnce(sim)
			}
		},
	})
	tickThreat = spell.NewThreatTick(tickSpellID)
	return spell
}
