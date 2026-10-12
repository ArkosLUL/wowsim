package druid

import (
	"time"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/stats"
)

func (druid *Druid) registerForceOfNatureCD() {
	if !druid.Talents.ForceOfNature {
		return
	}

	// the talent's own 33831, not 65861 (same cost, cooldown and flags, another creature): only 33831 is a druid
	// spell, so only it spends Clearcasting
	forceOfNatureAura := druid.RegisterAura(core.Aura{
		Label:    "Force of Nature",
		ActionID: core.ActionID{SpellID: 33831},
		Duration: time.Second * 30,
	})
	druid.ForceOfNature = druid.RegisterSpell(Humanoid|Moonkin, core.SpellConfig{
		ActionID: core.ActionID{SpellID: 33831},
		Flags:    core.SpellFlagAPL,
		ManaCost: core.ManaCostOptions{
			BaseCost:   0.12,
			Multiplier: 1,
		},
		Cast: core.CastConfig{
			DefaultCast: core.Cast{
				GCD: core.GCDDefault,
			},
			CD: core.Cooldown{
				Timer:    druid.NewTimer(),
				Duration: time.Minute * 3,
			},
		},

		ApplyEffects: func(sim *core.Simulation, _ *core.Unit, _ *core.Spell) {
			druid.Treant1.EnableWithTimeout(sim, druid.Treant1, time.Second*30)
			druid.Treant2.EnableWithTimeout(sim, druid.Treant2, time.Second*30)
			druid.Treant3.EnableWithTimeout(sim, druid.Treant3, time.Second*30)

			forceOfNatureAura.Activate(sim)

			// Animation delay, courtesy of our DK friends
			pa := core.PendingAction{
				NextActionAt: sim.CurrentTime + time.Second*1,
				Priority:     core.ActionPriorityAuto,
				OnAction: func(s *core.Simulation) {
				},
			}
			sim.AddPendingAction(&pa)
		},
	})
}

type TreantPet struct {
	*core.Pet
	druidOwner *Druid
}

func (druid *Druid) NewTreant() *TreantPet {
	treant := &TreantPet{
		Pet:        core.NewPet("Treant", &druid.Character, treantBaseStats, druid.treantStatInheritance(), false, true),
		druidOwner: druid,
	}
	treant.BuffImmunity = core.PetImmuneToScaling
	treant.FullManaPool = true
	treant.AddStatDependency(stats.Strength, stats.AttackPower, 2)

	// InitStatsForLevel's NPC_TREANT case, and 1964's template attack time. A white crit is the plain
	// x2 (Unit::CalculateMeleeDamage): the owner's crit damage auras never reach a creature's swing.
	treant.EnableAutoAttacks(treant, core.AutoAttackOptions{
		MainHand: core.Weapon{
			BaseDamageMin:  120,
			BaseDamageMax:  280,
			SwingSpeed:     1.574,
			CritMultiplier: 2,
		},
		AutoSwingMelee: true,
	})

	druid.AddPet(treant)
	return treant
}

func (treant *TreantPet) GetPet() *core.Pet {
	return treant.Pet
}

// treantStatInheritance is spell_dru_treant_scaling: 30% of the owner's stamina and intellect, 35% of
// his armor, 105% of his nature spell power as attack power, Brambles adding 5% a rank to that, and
// 15% as spell power.
func (druid *Druid) treantStatInheritance() core.PetStatInheritance {
	brambles := float64(5 * druid.Talents.Brambles)
	return func(ownerStats stats.Stats) stats.Stats {
		attackPower := core.PetScalingPct(ownerStats[stats.SpellPower], 105)
		return stats.Stats{
			stats.Stamina:     core.PetScalingPct(ownerStats[stats.Stamina], 30),
			stats.Intellect:   core.PetScalingPct(ownerStats[stats.Intellect], 30),
			stats.Armor:       core.PetScalingPct(ownerStats[stats.Armor], 35),
			stats.AttackPower: attackPower + core.PetScalingPct(attackPower, brambles),
			stats.SpellPower:  core.PetScalingPct(ownerStats[stats.SpellPower], 15),
		}
	}
}

func (treant *TreantPet) Initialize() {
}

func (treant *TreantPet) Reset(_ *core.Simulation) {
}

func (treant *TreantPet) ExecuteCustomRotation(_ *core.Simulation) {
}

// No pet_levelstats row for 1964, so InitStatsForLevel's fallback stats, and a creature's 5% crit, nothing
// from agility (Unit::GetUnitCriticalChance). Its only hit and expertise are 61017's share of the owner's.
var treantBaseStats = core.GuardianCreateStats.Add(stats.Stats{
	stats.AttackPower: -20,
	stats.MeleeCrit:   5 * core.CritRatingPerCritChance,
})
