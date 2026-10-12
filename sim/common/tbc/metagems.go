package tbc

import (
	"time"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/stats"
)

// Equip auras of the meta gems below, whose proc entries hold their chance, PPM and ICD.
const (
	insightfulEarthstormSpellID = 27521
	thunderingSkyfireSpellID    = 39958
)

// metaGemAura registers a meta gem's proc aura, which is on only while the gem is: always without an item
// swap that turns the gem on or off, else through the swap.
func metaGemAura(character *core.Character, aura core.Aura) {
	registered := character.RegisterAura(aura)
	if !character.MetaGemSwaps() {
		return
	}
	registered.OnReset = nil
	character.OnMetaGemSwap(func(sim *core.Simulation, on bool) {
		if on {
			registered.Activate(sim)
		} else {
			registered.Deactivate(sim)
		}
	})
}

// metaGemEffect applies a meta gem's pseudo stat bonus, and takes it off again while an item swap breaks
// the gem's condition. apply(true) puts it on and apply(false) takes it off.
func metaGemEffect(character *core.Character, apply func(on bool)) {
	if !character.MetaGemSwaps() {
		apply(true)
		return
	}
	character.OnMetaGemSwap(func(_ *core.Simulation, on bool) { apply(on) })
}

func init() {
	core.AddEffectsToTest = false
	// Keep these in order by item ID.

	core.NewMetaGemEffect(25893, func(agent core.Agent) {
		character := agent.GetCharacter()
		procAura := character.NewTemporaryStatsAura("Mystic Focus Proc", core.ActionID{ItemID: 25893}, stats.Stats{stats.SpellHaste: 320}, time.Second*4)

		icd := core.Cooldown{
			Timer:    character.NewTimer(),
			Duration: time.Second * 35,
		}

		metaGemAura(character, core.Aura{
			Label:    "Mystical Skyfire Diamond",
			Duration: core.NeverExpires,
			OnReset: func(aura *core.Aura, sim *core.Simulation) {
				aura.Activate(sim)
			},
			OnCastComplete: func(aura *core.Aura, sim *core.Simulation, spell *core.Spell) {
				if !icd.IsReady(sim) || sim.RandomFloat("Mystical Skyfire Diamond") > 0.15 {
					return
				}
				icd.Use(sim)
				procAura.Activate(sim)
			},
		})
	})

	core.NewMetaGemEffect(25899, func(agent core.Agent) {
		character := agent.GetCharacter()
		metaGemEffect(character, func(on bool) {
			character.PseudoStats.BonusDamage += core.TernaryFloat64(on, 3, -3)
		})
	})

	insightful := core.ServerProcFor(insightfulEarthstormSpellID)
	core.NewMetaGemEffect(25901, func(agent core.Agent) {
		character := agent.GetCharacter()
		icd := core.Cooldown{
			Timer:    character.NewTimer(),
			Duration: insightful.ICD,
		}
		manaMetrics := character.NewManaMetrics(core.ActionID{ItemID: 25901}).EnergizeThreat(core.EnergizeBySpell)

		metaGemAura(character, core.Aura{
			Label:    "Insightful Earthstorm Diamond",
			Duration: core.NeverExpires,
			OnReset: func(aura *core.Aura, sim *core.Simulation) {
				aura.Activate(sim)
			},
			OnCastComplete: func(aura *core.Aura, sim *core.Simulation, spell *core.Spell) {
				if !icd.IsReady(sim) || sim.RandomFloat("Insightful Earthstorm Diamond") > insightful.Chance {
					return
				}
				icd.Use(sim)
				character.AddMana(sim, 300, manaMetrics)
			},
		})
	})

	thundering := core.ServerProcFor(thunderingSkyfireSpellID)
	core.NewMetaGemEffect(32410, func(agent core.Agent) {
		character := agent.GetCharacter()
		procAura := character.NewTemporaryStatsAura("Thundering Skyfire Diamond Proc", core.ActionID{ItemID: 32410}, stats.Stats{stats.MeleeHaste: 240}, time.Second*6)

		icd := core.Cooldown{
			Timer:    character.NewTimer(),
			Duration: thundering.ICD,
		}
		ppmm := character.AutoAttacks.NewPPMManager(thundering.PPM, core.ProcMaskWhiteHit) // Mask 68, melee or ranged auto attacks.

		metaGemAura(character, core.Aura{
			Label:    "Thundering Skyfire Diamond",
			Duration: core.NeverExpires,
			OnReset: func(aura *core.Aura, sim *core.Simulation) {
				aura.Activate(sim)
			},
			OnSpellHitDealt: func(aura *core.Aura, sim *core.Simulation, spell *core.Spell, result *core.SpellResult) {
				if !result.Landed() {
					return
				}

				if !icd.IsReady(sim) {
					return
				}

				if ppmm.Proc(sim, spell.ProcMask, "Thundering Skyfire Diamond") {
					icd.Use(sim)
					procAura.Activate(sim)
				}
			},
		})
	})

	// Eternal Earthstorm
	core.NewMetaGemEffect(35501, func(agent core.Agent) {
		character := agent.GetCharacter()
		metaGemEffect(character, func(on bool) {
			character.PseudoStats.BlockValueMultiplier += core.TernaryFloat64(on, 0.05, -0.05)
		})
	})

	core.NewMetaGemEffect(35503, func(agent core.Agent) {
		character := agent.GetCharacter()
		if !character.MetaGemSwaps() {
			character.MultiplyStat(stats.Intellect, 1.02)
			return
		}
		dep := character.NewDynamicMultiplyStat(stats.Intellect, 1.02)
		character.OnMetaGemSwap(func(sim *core.Simulation, on bool) {
			if on {
				character.EnableDynamicStatDep(sim, dep)
			} else {
				character.DisableDynamicStatDep(sim, dep)
			}
		})
	})

	// These are handled in character.go, but create empty effects, so they are included in tests.
	core.NewItemEffect(34220, func(_ core.Agent) {}) // Chaotic Skyfire Diamond
	core.NewItemEffect(32409, func(_ core.Agent) {}) // Relentless Earthstorm Diamond

	core.AddEffectsToTest = true
}
