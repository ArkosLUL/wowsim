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

	forceOfNatureAura := druid.RegisterAura(core.Aura{
		Label:    "Force of Nature",
		ActionID: core.ActionID{SpellID: 65861},
		Duration: time.Second * 30,
	})
	druid.ForceOfNature = druid.RegisterSpell(Humanoid|Moonkin, core.SpellConfig{
		ActionID: core.ActionID{SpellID: 65861},
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
	core.Pet
	druidOwner *Druid

	snapshotStat stats.Stats
}

func (druid *Druid) NewTreant() *TreantPet {
	treant := &TreantPet{
		Pet: core.NewPet("Treant", &druid.Character, treantBaseStats, func(ownerStats stats.Stats) stats.Stats {
			return stats.Stats{}
		}, false, true),
		druidOwner: druid,
	}
	treant.AddStatDependency(stats.Strength, stats.AttackPower, 2)
	treant.AddStatDependency(stats.Agility, stats.MeleeCrit, core.CritRatingPerCritChance/83.3)

	treant.EnableAutoAttacks(treant, core.AutoAttackOptions{
		MainHand: core.Weapon{
			BaseDamageMin:  252,
			BaseDamageMax:  357,
			SwingSpeed:     2,
			CritMultiplier: druid.BalanceCritMultiplier(),
		},
		AutoSwingMelee: true,
	})

	treant.Pet.OnPetEnable = treant.enable
	treant.Pet.OnPetDisable = treant.disable

	druid.AddPet(treant)
	return treant
}

func (treant *TreantPet) GetPet() *core.Pet {
	return &treant.Pet
}

// spell_dru_treant_scaling (spell_druid.cpp:389-424): 30% of the owner's Intellect and Stamina,
// attack power at 105% of the owner's spell power, and Brambles adds its own percent to that AP
// conversion rather than a flat treant damage bonus.
func (treant *TreantPet) enable(sim *core.Simulation) {
	spellPower := treant.druidOwner.GetStat(stats.SpellPower)
	attackPower := spellPower * 1.05 * (1 + 0.05*float64(treant.druidOwner.Talents.Brambles))

	treant.snapshotStat = stats.Stats{
		stats.Intellect:   treant.druidOwner.GetStat(stats.Intellect) * 0.3,
		stats.Stamina:     treant.druidOwner.GetStat(stats.Stamina) * 0.3,
		stats.AttackPower: attackPower,
	}
	treant.AddStatsDynamic(sim, treant.snapshotStat)
}

func (treant *TreantPet) disable(sim *core.Simulation) {
	treant.AddStatsDynamic(sim, treant.snapshotStat.Invert())
}

func (treant *TreantPet) Initialize() {
}

func (treant *TreantPet) Reset(_ *core.Simulation) {
}

func (treant *TreantPet) ExecuteCustomRotation(_ *core.Simulation) {
}

// TODO : fix miss/dodge
var treantBaseStats = stats.Stats{
	stats.Strength:  331,
	stats.Agility:   113,
	stats.Stamina:   598,
	stats.Intellect: 281,
	stats.Spirit:    109,
	stats.MeleeCrit: 5 * core.CritRatingPerCritChance,
	stats.MeleeHit:  5 * core.MeleeHitRatingPerHitChance,
	stats.Expertise: 120,
}
