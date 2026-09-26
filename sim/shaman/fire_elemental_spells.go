package shaman

import (
	"time"

	"github.com/wowsims/wotlk/sim/core"
)

func (fireElemental *FireElemental) registerFireBlast() {
	fireElemental.FireBlast = fireElemental.RegisterSpell(core.SpellConfig{
		ActionID:    core.ActionID{SpellID: 13339},
		SpellSchool: core.SpellSchoolFire,
		ProcMask:    core.ProcMaskSpellDamage,

		ManaCost: core.ManaCostOptions{
			FlatCost: 276,
		},
		Cast: core.CastConfig{
			DefaultCast: core.Cast{
				GCD: core.GCDDefault,
			},
			IgnoreHaste: true,
			CD: core.Cooldown{
				Timer:    fireElemental.NewTimer(),
				Duration: time.Second,
			},
		},

		DamageMultiplier: 1,
		CritMultiplier:   fireElemental.DefaultSpellCritMultiplier(),
		ThreatMultiplier: 1,

		// The elemental's AI casts 57984, which has this spell's roll and 0.2 from its own
		// spell_bonus_data row. serverdata only has 13339.
		Direct: core.SpellEffect{Effect: 0, Min: 110, Max: 130, SP: 0.2},

		ApplyEffects: func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
			baseDamage := spell.Direct.Roll(sim) + spell.Direct.SP*spell.SpellPower()
			spell.CalcAndDealDamage(sim, target, baseDamage, spell.OutcomeMagicHitAndCrit)
		},
	})
}

func (fireElemental *FireElemental) registerFireNova() {
	fireElemental.FireNova = fireElemental.RegisterSpell(core.SpellConfig{
		ActionID:    core.ActionID{SpellID: 12470},
		SpellSchool: core.SpellSchoolFire,
		ProcMask:    core.ProcMaskSpellDamage,

		ManaCost: core.ManaCostOptions{
			FlatCost: 207,
		},
		Cast: core.CastConfig{
			DefaultCast: core.Cast{
				GCD:      core.GCDDefault,
				CastTime: time.Second * 2,
			},
			IgnoreHaste: true,
			CD: core.Cooldown{
				Timer:    fireElemental.NewTimer(),
				Duration: time.Second, // TODO estimated from log digging,
			},
		},

		DamageMultiplier: 1,
		CritMultiplier:   fireElemental.DefaultSpellCritMultiplier(),
		ThreatMultiplier: 1,

		Direct: core.SpellEffect{Effect: 0, Min: 148, Max: 170, SP: 0.5},

		// The elemental casts this itself, so the ten-target cap, which only gates a player caster,
		// doesn't reach it (INVESTIGATION, Findings: Attack table).
		ApplyEffects: func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
			for _, aoeTarget := range sim.Encounter.TargetUnits {
				baseDamage := spell.Direct.Roll(sim) + spell.Direct.SP*spell.SpellPower()
				spell.CalcAndDealDamage(sim, aoeTarget, baseDamage, spell.OutcomeMagicHitAndCrit)
			}
		},
	})
}

// The elemental's AI puts Fire Shield (13377) on itself, which casts 13376 every 3 s. The elemental is
// the caster, so there's no ten-target cap.
func (fireElemental *FireElemental) registerFireShieldAura() {
	spell := fireElemental.RegisterSpell(core.SpellConfig{
		ActionID:    core.ActionID{SpellID: 13376},
		SpellSchool: core.SpellSchoolFire,
		ProcMask:    core.ProcMaskEmpty,

		DamageMultiplier: 1,
		CritMultiplier:   fireElemental.DefaultSpellCritMultiplier(),
		ThreatMultiplier: 1,

		Dot: core.DotConfig{
			IsAOE: true,
			Aura: core.Aura{
				Label: "FireShield",
			},
			NumberOfTicks: 40,
			TickLength:    time.Second * 3,
			// 1.2 a level, 95 for the level 80 elemental; 0.015 is 13376's spell_bonus_data row
			Tick: core.SpellEffect{Effect: 0, Min: 95, Max: 95, SP: 0.015},

			OnTick: func(sim *core.Simulation, target *core.Unit, dot *core.Dot) {
				baseDamage := dot.Tick.Roll(sim) + dot.Tick.SP*dot.Spell.SpellPower()
				for _, aoeTarget := range sim.Encounter.TargetUnits {
					dot.Spell.CalcAndDealDamage(sim, aoeTarget, baseDamage, dot.Spell.OutcomeMagicCrit)
				}
			},
		},
	})

	fireElemental.FireShieldAura = fireElemental.RegisterAura(core.Aura{
		Label:    "Fire Shield",
		ActionID: core.ActionID{SpellID: 13377},
		Duration: time.Minute * 2,
		OnGain: func(_ *core.Aura, sim *core.Simulation) {
			spell.AOEDot().Apply(sim)
		},
	})
}
