package wotlk

import (
	"time"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
	"github.com/wowsims/wotlk/sim/core/serverdata"
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
// 69733, off any magic-class spell, 25% with a 50 s cooldown (spell_proc).
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

	registerShardOfTheGodsProc(character, name+" Trigger", func(sim *core.Simulation, target *core.Unit) {
		// a heal's landing uses up the proc too: the dot is aimed at whoever was healed and fails
		if target.Type == core.EnemyUnit {
			dotSpell.Dot(target).Apply(sim)
		}
	})
}

func applyShardOfTheGodsHealingProc(character *core.Character) {
	name := "Cauterizing Heal"
	spell := character.RegisterSpell(itemHealSpell(character, 69733, 2269, 2773))

	registerShardOfTheGodsProc(character, name+" Trigger", func(sim *core.Simulation, target *core.Unit) {
		// a damage spell's landing uses up the proc too: the heal is aimed at the enemy it hit and fails
		if target.Type != core.EnemyUnit {
			spell.Cast(sim, target)
		}
	})
}

// Each bonus rolls once for every target a cast lands on (proc flags DONE_SPELL_MAGIC_DMG_CLASS_POS | _NEG, type
// mask 7): a hit, a heal, or a hot or shield with nothing dealt, never a tick.
func registerShardOfTheGodsProc(character *core.Character, label string, handler func(*core.Simulation, *core.Unit)) {
	icd := core.Cooldown{
		Timer:    character.NewTimer(),
		Duration: time.Second * 50,
	}
	aura := character.RegisterAura(core.Aura{
		Label:    label,
		Duration: core.NeverExpires,
		Icd:      &icd,
		OnReset: func(aura *core.Aura, sim *core.Simulation) {
			aura.Activate(sim)
		},
	})
	character.RegisterOnCastLanded(func(sim *core.Simulation, spell *core.Spell, target *core.Unit) {
		if !aura.IsActive() || !icd.IsReady(sim) || !rollsShardOfTheGods(spell) {
			return
		}
		if sim.UnitRandomFloat(&character.Unit, label) > 0.25 {
			return
		}
		icd.Use(sim)
		handler(sim, target)
	})
}

const attr3SuppressCasterProcs = 0x00010000

// shardLandingSpellIDs maps a sim spell whose own id has no damage class to the magic one it lands as on the
// server. Keep the spell id name: it's how tools/acore/spellids finds the values for gen_serverdata.
var shardLandingSpellIDs = map[int32]int32{
	42846: 42845, // Arcane Missiles: each missile
	47515: 47753, // Divine Aegis: the shield
	48113: 48111, // Prayer of Mending: the buff, its jumps (41635) and heals (33110)
	49067: 49065, // Explosive Trap: the trap casts its effect for the hunter
	53007: 53000, // Penance: each bolt or heal
	63543: 63544, // Empowered Renew: the heal
}

// rollsShardOfTheGods is whether a landed spell can proc either set: magic class, triggered or not (both bonuses
// carry SPELL_ATTR3_CAN_PROC_FROM_PROCS), without SPELL_ATTR3_SUPPRESS_CASTER_PROCS. A spell with no damage class
// raises only the DMG_CLASS_NONE flags (Spell::DoAllEffectOnTarget), so it never rolls.
func rollsShardOfTheGods(spell *core.Spell) bool {
	ss := spell.ServerSpell()
	if id, ok := shardLandingSpellIDs[spell.ActionID.SpellID]; ok && ss != nil {
		ss = serverdata.SpellByID(id)
	}
	if ss == nil {
		// no server data under its action id, like a glyph's heal under the glyph's item
		return spell.ProcMask.Matches(core.ProcMaskSpellDamage | core.ProcMaskSpellHealing)
	}
	return ss.DmgClass == serverdata.DmgClassMagic && ss.Attributes[3]&attr3SuppressCasterProcs == 0
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
