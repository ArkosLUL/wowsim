package wotlk

import (
	"time"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
	"github.com/wowsims/wotlk/sim/core/stats"
)

// Keep these in alphabetical order.

var ItemSetPurifiedShardOfTheGods = core.NewItemSet(core.ItemSet{
	Name: "Purified Shard of the Gods",
	Bonuses: map[int32]core.ApplyEffect{
		2: func(agent core.Agent) {
			agent.GetCharacter().AddStats(stats.Stats{stats.SpellPower: 222})
			applyShardOfTheGodsHealingProc(agent.GetCharacter())
		},
	},
})

var ItemSetShinyShardOfTheGods = core.NewItemSet(core.ItemSet{
	Name: "Shiny Shard of the Gods",
	Bonuses: map[int32]core.ApplyEffect{
		2: func(agent core.Agent) {
			agent.GetCharacter().AddStats(stats.Stats{stats.SpellPower: 250})
			applyShardOfTheGodsDamageProc(agent.GetCharacter())
		},
	},
})

// Each set's script casts one spell whatever its tier: the Shiny set's bonus 69739 (spell_item_shiny_shard_of_the_scale)
// casts Searing Flames 69729, the Purified set's 69755 (spell_item_purified_shard_of_the_scale) Cauterizing Heal
// 69733, off any magic-class spell, 25% with a 45 s cooldown.
func applyShardOfTheGodsDamageProc(character *core.Character) {
	name := "Searing Flames"
	actionID := core.ActionID{SpellID: 69729}
	tickAmount := 477.0

	dotSpell := character.RegisterSpell(core.SpellConfig{
		ActionID:    actionID,
		SpellSchool: core.SpellSchoolFire,
		ProcMask:    core.ProcMaskSpellDamage,

		DamageMultiplier: 1,
		ThreatMultiplier: 1,

		Dot: core.DotConfig{
			Aura: core.Aura{
				Label: name,
			},
			NumberOfTicks: 6,
			TickLength:    time.Second * 2,
			TicksCanCrit:  false,
			OnSnapshot: func(sim *core.Simulation, target *core.Unit, dot *core.Dot, isRollover bool) {
				dot.SnapshotBaseDamage = tickAmount
				dot.SnapshotAttackerMultiplier = dot.Spell.AttackerDamageMultiplier(dot.Spell.Unit.AttackTables[target.UnitIndex])
			},
			OnTick: func(sim *core.Simulation, target *core.Unit, dot *core.Dot) {
				dot.CalcAndDealPeriodicSnapshotDamage(sim, target, dot.OutcomeTick)
			},
		},
	})

	core.MakeProcTriggerAura(&character.Unit, core.ProcTrigger{
		Name:       name + " Trigger",
		Callback:   core.CallbackOnHealDealt | core.CallbackOnSpellHitDealt,
		ProcMask:   core.ProcMaskSpellHealing | core.ProcMaskSpellDamage,
		Outcome:    core.OutcomeLanded,
		ProcChance: 0.25,
		ICD:        time.Second * 45,
		Handler: func(sim *core.Simulation, _ *core.Spell, result *core.SpellResult) {
			// a heal's hit uses up the proc too: the dot is aimed at whoever was healed and fails
			if result.Target.Type == core.EnemyUnit {
				dotSpell.Dot(result.Target).Apply(sim)
			}
		},
	})
}

func applyShardOfTheGodsHealingProc(character *core.Character) {
	name := "Cauterizing Heal"
	actionID := core.ActionID{SpellID: 69733}
	minHeal := 2269.0
	maxHeal := 2773.0

	spell := character.RegisterSpell(core.SpellConfig{
		ActionID:    actionID,
		SpellSchool: core.SpellSchoolHoly,
		ProcMask:    core.ProcMaskSpellHealing,
		Flags:       core.SpellFlagNoOnCastComplete | core.SpellFlagHelpful,

		DamageMultiplier: 1,
		ThreatMultiplier: 1,
		CritMultiplier:   character.DefaultHealingCritMultiplier(),

		ApplyEffects: func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
			baseHealing := sim.Roll(minHeal, maxHeal)
			spell.CalcAndDealHealing(sim, target, baseHealing, spell.OutcomeHealingCrit)
		},
	})

	core.MakeProcTriggerAura(&character.Unit, core.ProcTrigger{
		Name:       name + " Trigger",
		Callback:   core.CallbackOnHealDealt | core.CallbackOnSpellHitDealt,
		ProcMask:   core.ProcMaskSpellHealing | core.ProcMaskSpellDamage,
		Outcome:    core.OutcomeLanded,
		ProcChance: 0.25,
		ICD:        time.Second * 45,
		Handler: func(sim *core.Simulation, _ *core.Spell, result *core.SpellResult) {
			// a damage spell's hit uses up the proc too: the heal is aimed at the enemy it hit and fails
			if result.Target.Type != core.EnemyUnit {
				spell.Cast(sim, result.Target)
			}
		},
	})
}

// Each bonus is its own MOD_DAMAGE_DONE_VERSUS aura (57943, 29068, 57944), and the server multiplies them.
// DONE_VERSUS is per target, and a weapon hit's flat attack power bonus takes it too.
func makeUndeadSet(setName string) *core.ItemSet {
	versusUndead := func(multiplier float64) core.ApplyEffect {
		return func(agent core.Agent) {
			character := agent.GetCharacter()
			var applied bool
			character.RegisterResetEffect(func(_ *core.Simulation) {
				if applied {
					return
				}
				applied = true
				for _, target := range character.Env.Encounter.TargetUnits {
					if target.MobType == proto.MobType_MobTypeUndead {
						character.AttackTables[target.UnitIndex].DamageDealtMultiplier *= multiplier
					}
				}
			})
		}
	}
	return core.NewItemSet(core.ItemSet{
		Name: setName,
		Bonuses: map[int32]core.ApplyEffect{
			2: versusUndead(1.01),
			3: versusUndead(1.02),
			4: versusUndead(1.03),
		},
	})
}

var ItemSetBlessedBattlegearOfUndeadSlaying = makeUndeadSet("Blessed Battlegear of Undead Slaying")
var ItemSetBlessedRegaliaOfUndeadCleansing = makeUndeadSet("Blessed Regalia of Undead Cleansing")
var ItemSetBlessedGarbOfTheUndeadSlayer = makeUndeadSet("Blessed Garb of the Undead Slayer")
var ItemSetUndeadSlayersBlessedArmor = makeUndeadSet("Undead Slayer's Blessed Armor")
