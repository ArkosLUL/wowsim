package shaman

import (
	"time"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
	"github.com/wowsims/wotlk/sim/core/stats"
)

var ItemSetEarthshatterRegalia = core.NewItemSet(core.ItemSet{
	Name: "Earthshatter Regalia",
	Bonuses: map[int32]core.ApplyEffect{
		// Water Shield is 10% stronger: its mana per orb (waterShieldOrbMana) and its 100 mana per 5 s
		2: core.ClassEffect(func(agent ShamanAgent) {
			shaman := agent.GetShaman()
			if shaman.SelfBuffs.Shield == proto.ShamanShield_WaterShield {
				shaman.AddStat(stats.MP5, 10)
			}
		}),
		4: func(agent core.Agent) {
			// 5% more Healing Wave and Chain Heal, in heals.go and chain_heal.go
		},
	},
})

var ItemSetWorldbreakerRegalia = core.NewItemSet(core.ItemSet{
	Name: "Worldbreaker Regalia",
	Bonuses: map[int32]core.ApplyEffect{
		2: func(agent core.Agent) {
			// Riptide's cooldown, in heals.go
		},
		4: func(agent core.Agent) {
			// Chain Heal's cast time, in chain_heal.go
		},
	},
})

var ItemSetThrallsGarb = core.NewItemSet(core.ItemSet{
	Name:            "Thrall's Garb",
	AlternativeName: "Nobundo's Garb",
	Bonuses: map[int32]core.ApplyEffect{
		2: func(agent core.Agent) {
			// Riptide's healing, in heals.go
		},
		4: func(agent core.Agent) {
			// Chain Heal's crit chance, in chain_heal.go
		},
	},
})

var ItemSetFrostWitchGarb = core.NewItemSet(core.ItemSet{
	Name: "Frost Witch's Garb",
	Bonuses: map[int32]core.ApplyEffect{
		// Riptide gives the next cast 20% haste (70806), for 10 s or until a cast uses it up
		2: core.ClassEffect(func(agent ShamanAgent) {
			shaman := agent.GetShaman()
			fresh := false
			rapidCurrents := shaman.RegisterAura(core.Aura{
				Label:    "Rapid Currents",
				ActionID: core.ActionID{SpellID: 70806},
				Duration: time.Second * 10,
				OnGain: func(aura *core.Aura, sim *core.Simulation) {
					shaman.MultiplyCastSpeed(1.2)
				},
				OnExpire: func(aura *core.Aura, sim *core.Simulation) {
					shaman.MultiplyCastSpeed(1 / 1.2)
					fresh = false
				},
				OnCastComplete: func(aura *core.Aura, sim *core.Simulation, spell *core.Spell) {
					// the Riptide that gave it completes after its own heal: that cast doesn't use it
					if fresh {
						fresh = false
						return
					}
					aura.Deactivate(sim)
				},
			})
			shaman.RegisterAura(core.Aura{
				Label:    "Shaman T10 Restoration 2P Bonus",
				Duration: core.NeverExpires,
				OnReset: func(aura *core.Aura, sim *core.Simulation) {
					aura.Activate(sim)
					fresh = false
				},
				OnHealDealt: func(aura *core.Aura, sim *core.Simulation, spell *core.Spell, result *core.SpellResult) {
					if spell == shaman.Riptide {
						rapidCurrents.Activate(sim)
						fresh = true
					}
				},
			})
		}),
		// a Chain Heal crit heals its target again for 25% over 9 s (70809)
		4: core.ClassEffect(func(agent ShamanAgent) {
			shaman := agent.GetShaman()
			shaman.RegisterAura(core.Aura{
				Label:    "Shaman T10 Restoration 4P Bonus",
				Duration: core.NeverExpires,
				OnReset: func(aura *core.Aura, sim *core.Simulation) {
					aura.Activate(sim)
				},
				OnHealDealt: func(aura *core.Aura, sim *core.Simulation, spell *core.Spell, result *core.SpellResult) {
					if spell == shaman.ChainHeal && result.Outcome.Matches(core.OutcomeCrit) {
						shaman.chainHealHeals(sim, result.Target, result.Damage)
					}
				},
			})
		}),
	},
})

func init() {
	// Totem of Calming Tides: a Chain Heal cast has a chance to give 234 spell power for 15 s (67389 triggers 67388),
	// 8 s apart. It's a cast phase proc, so the cast that rolls it already has the spell power
	core.NewItemEffect(47665, core.ClassEffect(func(agent ShamanAgent) {
		shaman := agent.GetShaman()
		procAura := shaman.NewTemporaryStatsAura("Spiritual Trance", core.ActionID{SpellID: 67388}, stats.Stats{stats.SpellPower: 234}, time.Second*15)
		icd := core.Cooldown{
			Timer:    shaman.NewTimer(),
			Duration: time.Second * 8,
		}
		procAura.Icd = &icd
		shaman.onChainHealCast = func(sim *core.Simulation) {
			if icd.IsReady(sim) && sim.RandomFloat("Totem of Calming Tides") < 0.7 {
				icd.Use(sim)
				procAura.Activate(sim)
			}
		}
	}))

	// Totem of the Surging Sea: each Riptide gives 85 spell power for 15 s, up to 3 times (71217 triggers 71220)
	core.NewItemEffect(50464, core.ClassEffect(func(agent ShamanAgent) {
		shaman := agent.GetShaman()
		procAura := shaman.RegisterAura(core.Aura{
			Label:     "Energized",
			ActionID:  core.ActionID{SpellID: 71220},
			Duration:  time.Second * 15,
			MaxStacks: 3,
			OnStacksChange: func(aura *core.Aura, sim *core.Simulation, oldStacks int32, newStacks int32) {
				shaman.AddStatDynamic(sim, stats.SpellPower, 85*float64(newStacks-oldStacks))
			},
		})
		shaman.RegisterAura(core.Aura{
			Label:    "Totem of the Surging Sea",
			Duration: core.NeverExpires,
			OnReset: func(aura *core.Aura, sim *core.Simulation) {
				aura.Activate(sim)
			},
			OnHealDealt: func(aura *core.Aura, sim *core.Simulation, spell *core.Spell, result *core.SpellResult) {
				if spell == shaman.Riptide {
					procAura.Activate(sim)
					procAura.AddStack(sim)
				}
			},
		})
	}))
}
