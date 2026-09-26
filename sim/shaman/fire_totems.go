package shaman

import (
	"time"

	"github.com/wowsims/wotlk/sim/core"
)

// The bolt the totem fires is its own spell (58702): the summon (58704) is binary on the server, the
// bolt isn't, so they can't share one spell object. The dot stays on the summon, the id an APL names;
// only its ticks roll under 58702.
//
// No proc mask: the totem is the caster, so the procs go on it, not on the shaman. Only crit reads
// the owner (Spell::DoAllEffectOnLaunchTarget's IsTotem branch).
func (shaman *Shaman) registerSearingTotemAttackSpell() *core.Spell {
	return shaman.RegisterSpell(core.SpellConfig{
		ActionID:     core.ActionID{SpellID: 58702},
		SpellSchool:  core.SpellSchoolFire,
		ProcMask:     core.ProcMaskEmpty,
		Flags:        core.SpellFlagNoOnCastComplete,
		MissileSpeed: 19,

		BonusHitRating:   float64(shaman.Talents.ElementalPrecision) * core.SpellHitRatingPerHitChance,
		DamageMultiplier: 1 + float64(shaman.Talents.CallOfFlame)*0.05,
		CritMultiplier:   shaman.ElementalCritMultiplier(0),
	})
}

func (shaman *Shaman) registerSearingTotemSpell() {
	attack := shaman.registerSearingTotemAttackSpell()

	shaman.SearingTotem = shaman.RegisterSpell(core.SpellConfig{
		ActionID:    core.ActionID{SpellID: 58704},
		SpellSchool: core.SpellSchoolPhysical,
		ProcMask:    core.ProcMaskEmpty,
		Flags:       SpellFlagTotem | core.SpellFlagAPL,

		ManaCost: core.ManaCostOptions{
			BaseCost: 0.07,
			Multiplier: 1 -
				0.05*float64(shaman.Talents.TotemicFocus) -
				0.02*float64(shaman.Talents.MentalQuickness),
		},
		Cast: core.CastConfig{
			DefaultCast: core.Cast{
				GCD: time.Second,
			},
		},

		Dot: core.DotConfig{
			Spell: attack,
			Aura: core.Aura{
				Label: "SearingTotem",
			},
			// stand-in cadence: the totem recasts its 2.2 s bolt without waiting for the missile to land
			NumberOfTicks: 24,
			TickLength:    time.Second * 60 / 24,
			Tick:          core.SpellEffect{Effect: 0, FromSpellID: 58702, Min: 90, Max: 120, SP: 0.1667},
			OnTick: func(sim *core.Simulation, target *core.Unit, dot *core.Dot) {
				baseDamage := dot.Tick.Roll(sim) + dot.Tick.SP*dot.Spell.SpellPower()
				result := dot.Spell.CalcDamage(sim, target, baseDamage, dot.Spell.OutcomeMagicHitAndCrit)
				dot.Spell.WaitTravelTime(sim, func(sim *core.Simulation) {
					dot.Spell.DealDamage(sim, result)
				})
			},
		},

		ApplyEffects: func(sim *core.Simulation, _ *core.Unit, spell *core.Spell) {
			shaman.MagmaTotem.AOEDot().Cancel(sim)
			shaman.FireElemental.Disable(sim)
			spell.Dot(sim.GetTargetUnit(0)).Apply(sim)
			// +1 needed because of rounding issues with totem tick time.
			shaman.TotemExpirations[FireTotem] = sim.CurrentTime + time.Second*60 + 1
		},
	})
}

// Same split as Searing Totem: the pulse is 58735 and isn't binary, the summon (58734) is, and the dot
// stays on the summon for the APL. The totem casts the pulse itself, and the ten-target cap only gates
// a player caster, so it doesn't reach it (INVESTIGATION, Findings: Attack table). No proc mask for
// the same reason Searing Totem's bolt has none.
func (shaman *Shaman) registerMagmaTotemAttackSpell() *core.Spell {
	return shaman.RegisterSpell(core.SpellConfig{
		ActionID:    core.ActionID{SpellID: 58735},
		SpellSchool: core.SpellSchoolFire,
		ProcMask:    core.ProcMaskEmpty,
		Flags:       core.SpellFlagNoOnCastComplete,

		BonusHitRating:   float64(shaman.Talents.ElementalPrecision) * core.SpellHitRatingPerHitChance,
		DamageMultiplier: 1 + float64(shaman.Talents.CallOfFlame)*0.05,
		CritMultiplier:   shaman.ElementalCritMultiplier(0),
	})
}

func (shaman *Shaman) registerMagmaTotemSpell() {
	attack := shaman.registerMagmaTotemAttackSpell()

	shaman.MagmaTotem = shaman.RegisterSpell(core.SpellConfig{
		ActionID:    core.ActionID{SpellID: 58734},
		SpellSchool: core.SpellSchoolPhysical,
		ProcMask:    core.ProcMaskEmpty,
		Flags:       SpellFlagTotem | core.SpellFlagAPL,

		ManaCost: core.ManaCostOptions{
			BaseCost: 0.27,
			Multiplier: 1 -
				0.05*float64(shaman.Talents.TotemicFocus) -
				0.02*float64(shaman.Talents.MentalQuickness),
		},
		Cast: core.CastConfig{
			DefaultCast: core.Cast{
				GCD: time.Second,
			},
		},

		Dot: core.DotConfig{
			Spell: attack,
			IsAOE: true,
			Aura: core.Aura{
				Label: "MagmaTotem",
			},
			NumberOfTicks: 10,
			TickLength:    time.Second * 2,
			Tick:          core.SpellEffect{Effect: 0, FromSpellID: 58735, Min: 371, Max: 371, SP: 0.1},

			OnTick: func(sim *core.Simulation, target *core.Unit, dot *core.Dot) {
				baseDamage := dot.Tick.Roll(sim) + dot.Tick.SP*dot.Spell.SpellPower()
				for _, aoeTarget := range sim.Encounter.TargetUnits {
					dot.Spell.CalcAndDealDamage(sim, aoeTarget, baseDamage, dot.Spell.OutcomeMagicHitAndCrit)
				}
			},
		},

		ApplyEffects: func(sim *core.Simulation, _ *core.Unit, spell *core.Spell) {
			shaman.SearingTotem.Dot(shaman.CurrentTarget).Cancel(sim)
			shaman.FireElemental.Disable(sim)
			spell.AOEDot().Apply(sim)
			// +1 needed because of rounding issues with totem tick time.
			shaman.TotemExpirations[FireTotem] = sim.CurrentTime + time.Second*20 + 1
		},
	})
}
