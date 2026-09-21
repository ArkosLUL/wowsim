package hunter

import (
	"time"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/stats"
)

var ItemSetGronnstalker = core.NewItemSet(core.ItemSet{
	Name: "Gronnstalker's Armor",
	Bonuses: map[int32]core.ApplyEffect{
		2: func(agent core.Agent) {
			// Handled in rotation.go
		},
		4: func(agent core.Agent) {
			// Handled in steady_shot.go
		},
	},
})

func init() {
	// spell_gen_black_bow_of_the_betrayer (spell_generic.cpp) has no class check, so this fires for
	// any wielder; energizing a unit with no mana bar is just a no-op, same as the server.
	core.NewItemEffect(32336, func(agent core.Agent) {
		character := agent.GetCharacter()
		const manaGain = 8.0
		manaMetrics := character.NewManaMetrics(core.ActionID{SpellID: 46939})

		character.RegisterAura(core.Aura{
			Label:    "Black Bow of the Betrayer",
			Duration: core.NeverExpires,
			OnReset: func(aura *core.Aura, sim *core.Simulation) {
				aura.Activate(sim)
			},
			OnSpellHitDealt: func(aura *core.Aura, sim *core.Simulation, spell *core.Spell, result *core.SpellResult) {
				if !result.Landed() || !spell.ProcMask.Matches(core.ProcMaskRanged) {
					return
				}
				character.AddMana(sim, manaGain, manaMetrics)
			},
		})
	})

	core.NewItemEffect(32487, func(agent core.Agent) {
		hunter := agent.(HunterAgent).GetHunter()

		procAura := hunter.NewTemporaryStatsAura("Ashtongue Talisman Proc", core.ActionID{ItemID: 32487}, stats.Stats{stats.AttackPower: 275, stats.RangedAttackPower: 275}, time.Second*8)
		const procChance = 0.15

		hunter.RegisterAura(core.Aura{
			Label:    "Ashtongue Talisman",
			Duration: core.NeverExpires,
			OnReset: func(aura *core.Aura, sim *core.Simulation) {
				aura.Activate(sim)
			},
			OnSpellHitDealt: func(aura *core.Aura, sim *core.Simulation, spell *core.Spell, result *core.SpellResult) {
				if spell != hunter.SteadyShot {
					return
				}
				if sim.RandomFloat("Ashtongue Talisman of Swiftness") > procChance {
					return
				}
				procAura.Activate(sim)
			},
		})
	})

}
