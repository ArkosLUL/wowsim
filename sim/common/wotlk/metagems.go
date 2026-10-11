package wotlk

import (
	"time"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/stats"
)

// metaGemStat is a meta gem's percent of a stat. The gem's enchant goes on and off as an item swap meets
// or breaks its condition, so then the percent does too.
func metaGemStat(character *core.Character, stat stats.Stat, multiplier float64) {
	if !character.MetaGemSwaps() {
		character.MultiplyStat(stat, multiplier)
		return
	}
	dep := character.NewDynamicMultiplyStat(stat, multiplier)
	character.OnMetaGemSwap(func(sim *core.Simulation, on bool) {
		if on {
			character.EnableDynamicStatDep(sim, dep)
		} else {
			character.DisableDynamicStatDep(sim, dep)
		}
	})
}

// metaGemEffect is a meta gem's effect on the character's pseudo stats: apply puts it on, and takes it off
// again given the inverse factor or the negated bonus.
func metaGemEffect(character *core.Character, apply func(on bool)) {
	if !character.MetaGemSwaps() {
		apply(true)
		return
	}
	character.OnMetaGemSwap(func(_ *core.Simulation, on bool) { apply(on) })
}

// metaGemProc is a meta gem's proc aura, on only while the gem is.
func metaGemProc(character *core.Character, aura *core.Aura) {
	if !character.MetaGemSwaps() {
		return
	}
	aura.OnReset = nil
	character.OnMetaGemSwap(func(sim *core.Simulation, on bool) {
		if on {
			aura.Activate(sim)
		} else {
			aura.Deactivate(sim)
		}
	})
}

func init() {
	// Keep these in order by item ID.

	// TODO: Destructive Skyflare (1% spell reflect)

	core.NewMetaGemEffect(41333, func(agent core.Agent) {
		metaGemStat(agent.GetCharacter(), stats.Intellect, 1.02)
	})

	core.NewMetaGemEffect(41377, func(agent core.Agent) {
		character := agent.GetCharacter()
		metaGemEffect(character, func(on bool) {
			multiplier := core.TernaryFloat64(on, 0.98, 1/0.98)
			character.PseudoStats.SchoolDamageTakenMultiplier[stats.SchoolIndexArcane] *= multiplier
			character.PseudoStats.SchoolDamageTakenMultiplier[stats.SchoolIndexFire] *= multiplier
			character.PseudoStats.SchoolDamageTakenMultiplier[stats.SchoolIndexFrost] *= multiplier
			character.PseudoStats.SchoolDamageTakenMultiplier[stats.SchoolIndexHoly] *= multiplier
			character.PseudoStats.SchoolDamageTakenMultiplier[stats.SchoolIndexNature] *= multiplier
			character.PseudoStats.SchoolDamageTakenMultiplier[stats.SchoolIndexShadow] *= multiplier
		})
	})

	core.NewMetaGemEffect(41380, func(agent core.Agent) {
		character := agent.GetCharacter()
		if !character.MetaGemSwaps() {
			character.ApplyEquipScaling(stats.Armor, 1.02)
			return
		}
		character.OnMetaGemSwap(func(sim *core.Simulation, on bool) {
			if on {
				character.ApplyDynamicEquipScaling(sim, stats.Armor, 1.02)
			} else {
				character.RemoveDynamicEquipScaling(sim, stats.Armor, 1.02)
			}
		})
	})

	core.NewMetaGemEffect(41385, func(agent core.Agent) {
		character := agent.GetCharacter()

		healSpell := character.RegisterSpell(core.SpellConfig{
			ActionID:    core.ActionID{SpellID: 55341},
			SpellSchool: core.SpellSchoolPhysical,
			ProcMask:    core.ProcMaskSpellHealing,
			Flags:       core.SpellFlagNoOnCastComplete | core.SpellFlagHelpful,

			DamageMultiplier: 1,
			CritMultiplier:   character.DefaultHealingCritMultiplier(),
			ThreatMultiplier: 1,

			ApplyEffects: func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
				baseHealing := 0.02 * target.MaxHealth()
				spell.CalcAndDealHealing(sim, target, baseHealing, spell.OutcomeHealingCrit)
			},
		})

		metaGemProc(character, core.MakeProcTriggerAura(&character.Unit, core.ProcTrigger{
			Name:       "Invigorating Earthsiege Diamond",
			ProcMask:   core.ProcMaskMeleeOrRanged,
			Callback:   core.CallbackOnSpellHitDealt | core.CallbackOnPeriodicDamageDealt | core.CallbackOnHealDealt | core.CallbackOnPeriodicHealDealt,
			Outcome:    core.OutcomeCrit,
			ProcChance: 0.5,
			ICD:        time.Second * 45,
			Handler: func(sim *core.Simulation, _ *core.Spell, _ *core.SpellResult) {
				healSpell.Cast(sim, &character.Unit)
			},
		}))
	})

	core.NewMetaGemEffect(41389, func(agent core.Agent) {
		metaGemStat(agent.GetCharacter(), stats.Mana, 1.02)
	})

	core.NewMetaGemEffect(41395, func(agent core.Agent) {
		character := agent.GetCharacter()
		metaGemEffect(character, func(on bool) {
			character.PseudoStats.ThreatMultiplier *= core.TernaryFloat64(on, 0.98, 1/0.98)
		})
	})

	// Eternal Earthsiege
	core.NewMetaGemEffect(41396, func(agent core.Agent) {
		character := agent.GetCharacter()
		metaGemEffect(character, func(on bool) {
			character.PseudoStats.BlockValueMultiplier += core.TernaryFloat64(on, 0.05, -0.05)
		})
	})

	core.NewMetaGemEffect(41400, func(agent core.Agent) {
		character := agent.GetCharacter()
		procAura := character.NewTemporaryStatsAura("Thundering Skyflare Diamond Proc", core.ActionID{SpellID: 55379}, stats.Stats{stats.MeleeHaste: 480}, time.Second*6)

		triggerAura := core.MakeProcTriggerAura(&character.Unit, core.ProcTrigger{
			Name:     "Thundering Skyflare Diamond",
			Callback: core.CallbackOnSpellHitDealt,
			// Mask 68, melee or ranged auto attacks.
			ProcMask: core.ProcMaskWhiteHit,
			Outcome:  core.OutcomeLanded,
			PPM:      1,
			ICD:      time.Second * 40,
			Handler: func(sim *core.Simulation, _ *core.Spell, _ *core.SpellResult) {
				procAura.Activate(sim)
			},
		})
		procAura.Icd = triggerAura.Icd
		metaGemProc(character, triggerAura)
	})

	core.NewMetaGemEffect(41401, func(agent core.Agent) {
		character := agent.GetCharacter()
		manaMetrics := character.NewManaMetrics(core.ActionID{SpellID: 55382}).EnergizeThreat(core.EnergizeBySpell)

		metaGemProc(character, core.MakeProcTriggerAura(&character.Unit, core.ProcTrigger{
			Name:       "Insightful Earthsiege Diamond",
			Callback:   core.CallbackOnCastComplete,
			ProcChance: 0.05,
			ICD:        time.Second * 15,
			Handler: func(sim *core.Simulation, _ *core.Spell, _ *core.SpellResult) {
				character.AddMana(sim, 600, manaMetrics)
			},
			ProcMask: ^core.ProcMaskProc & ^core.ProcMaskWeaponProc,
		}))
	})

	// These are handled in character.go, but create empty effects so they are included in tests.
	core.NewMetaGemEffect(41285, func(_ core.Agent) {}) // Chaotic Skyflare Diamond
	core.NewMetaGemEffect(41376, func(_ core.Agent) {}) // Revitalizing Skyflare Diamond
	core.NewMetaGemEffect(41398, func(_ core.Agent) {}) // Relentless Earthsiege Diamond
}
