package shaman

import (
	"time"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/stats"
)

// Variables that control the Fire Elemental.
const (
	// 7.5 CPM
	maxFireBlastCasts = 15
	maxFireNovaCasts  = 15
)

type FireElemental struct {
	*core.Pet

	FireBlast *core.Spell
	FireNova  *core.Spell

	FireShieldAura *core.Aura

	shamanOwner *Shaman
}

func (shaman *Shaman) NewFireElemental(bonusSpellPower float64) *FireElemental {
	fireElemental := &FireElemental{
		Pet:         core.NewPet("Greater Fire Elemental", &shaman.Character, fireElementalPetBaseStats, fireElementalStatInheritance, false, true),
		shamanOwner: shaman,
	}
	fireElemental.BuffImmunity = core.PetImmuneToScaling
	// 32982 summons it with SummonProperties 61, a plain guardian's (Guardian::Guardian)
	fireElemental.Uncontrollable = true
	// pet_levelstats' 2280 mana, and Guardian::UpdateMaxPower's default 15 a point of intellect
	fireElemental.EnableCreatureManaBar(2280, fireElementalPetBaseStats[stats.Intellect], 15, false)
	// InitStatsForLevel's NPC_FIRE_ELEMENTAL case: 3.5 a level -/+ the level, in the template's
	// fire school at its 2 s attack time
	fireElemental.EnableAutoAttacks(fireElemental, core.AutoAttackOptions{
		MainHand: core.Weapon{
			BaseDamageMin:  200,
			BaseDamageMax:  360,
			SwingSpeed:     2,
			CritMultiplier: 2,
			SpellSchool:    core.SpellSchoolFire,
		},
		AutoSwingMelee: true,
	})
	fireElemental.AddStatDependency(stats.Strength, stats.AttackPower, 2)

	// the spell power the totem options add at the summon scales as the owner's own does
	if bonusSpellPower > 0 {
		fireElemental.AddStat(stats.SpellPower, core.PetScalingPct(bonusSpellPower, 100))
		fireElemental.AddStat(stats.AttackPower, core.PetScalingPct(bonusSpellPower, 300))
	}

	fireElemental.OnPetEnable = fireElemental.enable
	fireElemental.OnPetDisable = fireElemental.disable

	shaman.AddPet(fireElemental)

	return fireElemental
}

func (fireElemental *FireElemental) enable(sim *core.Simulation) {
	fireElemental.FireShieldAura.Activate(sim)
}

func (fireElemental *FireElemental) disable(sim *core.Simulation) {
	fireElemental.FireShieldAura.Deactivate(sim)
}

func (fireElemental *FireElemental) GetPet() *core.Pet {
	return fireElemental.Pet
}

func (fireElemental *FireElemental) Initialize() {

	fireElemental.registerFireBlast()
	fireElemental.registerFireNova()
	fireElemental.registerFireShieldAura()
}

func (fireElemental *FireElemental) Reset(_ *core.Simulation) {

}

func (fireElemental *FireElemental) ExecuteCustomRotation(sim *core.Simulation) {
	/*
		TODO this is a little dirty, can probably clean this up, the rotation might go through some more overhauls,
		the random AI is hard to emulate.
	*/
	target := fireElemental.CurrentTarget
	fireBlastCasts := fireElemental.FireBlast.SpellMetrics[0].Casts
	fireNovaCasts := fireElemental.FireNova.SpellMetrics[0].Casts

	if fireBlastCasts == maxFireBlastCasts && fireNovaCasts == maxFireNovaCasts {
		return
	}

	if fireElemental.FireNova.DefaultCast.Cost > fireElemental.CurrentMana() {
		return
	}

	random := sim.RandomFloat("Fire Elemental Pet Spell")

	//Melee the other 30%
	if random >= .65 {
		if !fireElemental.TryCast(sim, target, fireElemental.FireNova, maxFireNovaCasts) {
			fireElemental.TryCast(sim, target, fireElemental.FireBlast, maxFireBlastCasts)
		}
	} else if random >= .35 {
		if !fireElemental.TryCast(sim, target, fireElemental.FireBlast, maxFireBlastCasts) {
			fireElemental.TryCast(sim, target, fireElemental.FireNova, maxFireNovaCasts)
		}
	}

	if !fireElemental.GCD.IsReady(sim) {
		return
	}

	fireElemental.WaitUntil(sim, sim.CurrentTime+time.Second)
}

func (fireElemental *FireElemental) TryCast(sim *core.Simulation, target *core.Unit, spell *core.Spell, maxCastCount int32) bool {
	if maxCastCount == spell.SpellMetrics[0].Casts {
		return false
	}

	return spell.Cast(sim, target)
}

// pet_levelstats for 15438 at 80, and 2 * Str - 20 attack power
var fireElementalPetBaseStats = core.PetCreateStats(283, 160, 125, 405, 460, 1100).Add(stats.Stats{
	// Guardian::UpdateMaxHealth: 8000 plus 10 a point of stamina over 125. The universal stamina
	// dependency (10 a point, -180) adds the rest
	stats.Health:      8000 - 10*125 + 180,
	stats.AttackPower: -20,

	// Unit::GetUnitCriticalChance and m_baseSpellCritChance give every non-player unit a flat 5%; the fire
	// elemental isn't one of the pets a scripted aura scales further by a stat.
	stats.MeleeCrit: 5 * core.CritRatingPerCritChance,
	stats.SpellCrit: 5 * core.CritRatingPerCritChance,
})

// fireElementalStatInheritance is spell_sha_fire_elemental_scaling on 35665 and 35666: 30% of the
// owner's intellect, 35% of his armor, and 300% and 100% of his fire spell power as attack and spell
// power. No stamina: 35665's first effect is a dummy. Hit and expertise come from core's scaling
// aura, not from here.
func fireElementalStatInheritance(ownerStats stats.Stats) stats.Stats {
	return stats.Stats{
		stats.Intellect:   core.PetScalingPct(ownerStats[stats.Intellect], 30),
		stats.Armor:       core.PetScalingPct(ownerStats[stats.Armor], 35),
		stats.AttackPower: core.PetScalingPct(ownerStats[stats.SpellPower], 300),
		stats.SpellPower:  core.PetScalingPct(ownerStats[stats.SpellPower], 100),
	}
}
