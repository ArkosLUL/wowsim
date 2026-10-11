package wotlk

import (
	"time"

	"github.com/wowsims/wotlk/sim/core"
)

type ProcDamageEffect struct {
	ID int32
	// the damage spell the item's aura triggers
	SpellID int32
	Trigger core.ProcTrigger

	School core.SpellSchool
	MinDmg float64
	MaxDmg float64
	// yards per second, for a spell that flies
	MissileSpeed float64
}

func newProcDamageEffect(config ProcDamageEffect) {
	core.NewItemEffect(config.ID, func(agent core.Agent) {
		character := agent.GetCharacter()

		damageSpell := character.RegisterSpell(itemDamageSpell(character, config.SpellID, config.School,
			config.MinDmg, config.MaxDmg, config.MissileSpeed))

		triggerConfig := config.Trigger
		triggerConfig.Handler = func(sim *core.Simulation, _ *core.Spell, _ *core.SpellResult) {
			damageSpell.Cast(sim, character.CurrentTarget)
		}
		core.MakeProcTriggerAura(&character.Unit, triggerConfig)
	})
}

// itemDamageSpell is an item's triggered damage under its server spell, so class auras can read its damage
// class and cast time (Omen of Clarity rolls on any magic-class hit).
func itemDamageSpell(character *core.Character, spellID int32, school core.SpellSchool, minDmg, maxDmg, missileSpeed float64) core.SpellConfig {
	return core.SpellConfig{
		ActionID:     core.ActionID{SpellID: spellID},
		SpellSchool:  school,
		ProcMask:     core.ProcMaskEmpty,
		MissileSpeed: missileSpeed,

		DamageMultiplier: 1,
		CritMultiplier:   character.DefaultSpellCritMultiplier(),
		ThreatMultiplier: 1,

		Direct: core.SpellEffect{Effect: 0, Min: minDmg, Max: maxDmg},

		ApplyEffects: func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
			if spell.MissileSpeed == 0 {
				spell.CalcAndDealDamage(sim, target, spell.Direct.Roll(sim), spell.OutcomeMagicHitAndCrit)
				return
			}
			spell.DealDamageAfterTravel(sim, spell.CalcDamage(sim, target, spell.Direct.Roll(sim), spell.OutcomeMagicHitAndCrit))
		},
	}
}

func init() {
	core.AddEffectsToTest = false

	newProcDamageEffect(ProcDamageEffect{
		ID:      37064,
		SpellID: 60307,
		Trigger: core.ProcTrigger{
			Name:       "Vestige of Haldor",
			Callback:   core.CallbackOnSpellHitDealt,
			ProcMask:   core.ProcMaskMeleeOrRanged,
			Outcome:    core.OutcomeLanded,
			ProcChance: 0.15,
			ICD:        time.Second * 45,
			ActionID:   core.ActionID{ItemID: 37064},
		},
		School: core.SpellSchoolFire,
		MinDmg: 1024,
		MaxDmg: 1536,
	})

	newProcDamageEffect(ProcDamageEffect{
		ID:      37264,
		SpellID: 60483,
		Trigger: core.ProcTrigger{
			Name:       "Pendulum of Telluric Currents",
			Callback:   core.CallbackOnSpellHitDealt,
			ProcMask:   core.ProcMaskSpellOrProc,
			Outcome:    core.OutcomeLanded,
			ProcChance: 0.15,
			ICD:        time.Second * 45,
			ActionID:   core.ActionID{ItemID: 37264},
		},
		School: core.SpellSchoolShadow,
		MinDmg: 1168,
		MaxDmg: 1752,
	})

	newProcDamageEffect(ProcDamageEffect{
		ID:      39889,
		SpellID: 55748,
		Trigger: core.ProcTrigger{
			Name:       "Horn of Agent Fury",
			Callback:   core.CallbackOnSpellHitDealt,
			ProcMask:   core.ProcMaskMeleeOrRanged,
			Outcome:    core.OutcomeLanded,
			ProcChance: 0.15,
			ICD:        time.Second * 45,
			ActionID:   core.ActionID{ItemID: 39889},
		},
		School: core.SpellSchoolHoly,
		MinDmg: 1024,
		MaxDmg: 1536,
	})

	core.AddEffectsToTest = true

	newProcDamageEffect(ProcDamageEffect{
		ID:      40371,
		SpellID: 60443,
		Trigger: core.ProcTrigger{
			Name:       "Bandit's Insignia",
			Callback:   core.CallbackOnSpellHitDealt,
			ProcMask:   core.ProcMaskMeleeOrRanged,
			Outcome:    core.OutcomeLanded,
			ProcChance: 0.15,
			ICD:        time.Second * 45,
			ActionID:   core.ActionID{ItemID: 40371},
		},
		School: core.SpellSchoolArcane,
		MinDmg: 1504,
		MaxDmg: 2256,
	})

	newProcDamageEffect(ProcDamageEffect{
		ID:      40373,
		SpellID: 60488,
		Trigger: core.ProcTrigger{
			Name:       "Extract of Necromantic Power",
			Callback:   core.CallbackOnPeriodicDamageDealt,
			Harmful:    true,
			ProcChance: 0.10,
			ICD:        time.Second * 15,
			ActionID:   core.ActionID{ItemID: 40373},
		},
		School:       core.SpellSchoolShadow,
		MinDmg:       788,
		MaxDmg:       1312,
		MissileSpeed: 20,
	})

	newProcDamageEffect(ProcDamageEffect{
		ID:      42990,
		SpellID: 60203,
		Trigger: core.ProcTrigger{
			Name:       "DMC Death",
			Callback:   core.CallbackOnSpellHitDealt | core.CallbackOnPeriodicDamageDealt,
			Harmful:    true,
			ProcChance: 0.15,
			ICD:        time.Second * 45,
			ActionID:   core.ActionID{ItemID: 42990},
		},
		School: core.SpellSchoolShadow,
		MinDmg: 1750,
		MaxDmg: 2250,
	})
}
