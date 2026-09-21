package deathknight

import (
	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
	"github.com/wowsims/wotlk/sim/core/stats"
)

type BloodwormPet struct {
	core.Pet

	dkOwner *Deathknight
}

// A bloodworm (28017) has no pet_levelstats row, so InitStatsForLevel's fallback stats and 2 * Str - 20
// AP, with 61017 for hit and no DK pet scaling at all: no stats, haste or armor pen from its owner.
func (dk *Deathknight) NewBloodwormPet(_ int) *BloodwormPet {
	bloodworm := &BloodwormPet{
		Pet: core.NewPet("Bloodworm", &dk.Character, stats.Stats{
			stats.Stamina:     25,
			stats.Agility:     22,
			stats.Strength:    22,
			stats.AttackPower: -20,
			stats.MeleeCrit:   creatureCritChance,
		}, func(stats.Stats) stats.Stats { return stats.Stats{} }, false, true),
		dkOwner: dk,
	}

	bloodworm.EnableAutoAttacks(bloodworm, core.AutoAttackOptions{
		MainHand: core.Weapon{
			// the template's attack time; the damage is set when it's summoned
			SwingSpeed:        2.66,
			CritMultiplier:    2,
			AttackPowerPerDPS: core.DefaultAttackPowerPerDPS,
		},
		AutoSwingMelee: true,
	})

	bloodworm.AddStatDependency(stats.Strength, stats.AttackPower, 2)

	// the orc's Command (65221) only goes to risen ghouls and the gargoyle
	if dk.RacialTraits == proto.Race_RaceOrc {
		bloodworm.PseudoStats.DamageDealtMultiplier /= 1.05
	}

	bloodworm.OnPetEnable = bloodworm.enable

	dk.AddPet(bloodworm)

	return bloodworm
}

func (bloodworm *BloodwormPet) GetPet() *core.Pet {
	return &bloodworm.Pet
}

func (bloodworm *BloodwormPet) Initialize() {

}

func (bloodworm *BloodwormPet) Reset(_ *core.Simulation) {
}

func (bloodworm *BloodwormPet) ExecuteCustomRotation(_ *core.Simulation) {
}

// enable is InitStatsForLevel's NPC_BLOODWORM case: level - 30 -/+ a quarter of it, plus 0.6% of the
// owner's attack power at the summon.
func (bloodworm *BloodwormPet) enable(_ *core.Simulation) {
	const level = core.CharacterLevel
	fromAP := 0.006 * bloodworm.dkOwner.GetStat(stats.AttackPower)
	weapon := bloodworm.AutoAttacks.MH()
	weapon.BaseDamageMin = float64(level-30-level/4) + fromAP
	weapon.BaseDamageMax = float64(level-30+level/4) + fromAP
}
