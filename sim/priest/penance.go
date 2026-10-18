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
		// each tick is a 53000 or 52985 the channel casts at its target, a missile at 40 yd a second
		MissileSpeed: 40,

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
				result := dot.Spell.CalcPeriodicDamage(sim, target, baseDamage, dot.Spell.OutcomeMagicHitAndCrit)
				dot.Spell.WaitTravelTime(sim, func(sim *core.Simulation) {
					dot.Spell.DealPeriodicDamage(sim, result)
					tickThreat.Hit(result)
					tickThreat.Done(sim)
				})
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
				// EffectHeal works the heal out at the launch, crit included, and it lands after the flight
				result := dot.Spell.CalcHealing(sim, target, baseHealing, dot.Spell.OutcomeHealingCrit)
				dot.Spell.WaitTravelTimeTo(sim, target, func(sim *core.Simulation) {
					dot.Spell.DealHealing(sim, result)
					tickThreat.Done(sim)
				})
			},
		}, core.DotConfig{}),

		ApplyEffects: func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
			if isHeal {
				spell.SpellMetrics[target.UnitIndex].Hits--
			}
			// 53007 is a channel with no duration, so its cast waits for its SpellEvent on the press's tick,
			// and the channel its script casts (53003, or 52988 at a friend) for its own on the tick after.
			// The first tick goes off as the channel starts (SPELL_ATTR5_EXTRA_INITIAL_PERIOD).
			channel := func(sim *core.Simulation) {
				if isHeal {
					hot := spell.Hot(target)
					hot.Apply(sim)
					hot.TickOnce(sim)
					return
				}
				// 53007 and the channel it starts aren't magic class, so they can't miss and proc nothing a
				// harmful magic spell does (Illustration of the Dragon Soul). Each tick rolls its own hit.
				dot := spell.Dot(target)
				dot.Apply(sim)
				dot.TickOnce(sim)
			}
			if at := penanceChannelStart(sim); at > sim.CurrentTime {
				sim.QueueSpellEvent(&priest.Unit, at, channel)
			} else {
				channel(sim)
			}
		},
	})
	tickThreat = spell.NewThreatTick(tickSpellID)
	return spell
}

// penanceChannelStart is the world tick after the one a press now is handled on, or now with exact timing.
func penanceChannelStart(sim *core.Simulation) time.Duration {
	pressed := sim.NextWorldTick(sim.CurrentTime)
	if next := sim.NextWorldTick(pressed + 1); next != pressed+1 {
		return next
	}
	return sim.CurrentTime
}
