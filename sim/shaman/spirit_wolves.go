package shaman

import (
	"strconv"
	"time"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
	"github.com/wowsims/wotlk/sim/core/stats"
)

type SpiritWolf struct {
	*core.Pet

	shamanOwner *Shaman
}

type SpiritWolves struct {
	SpiritWolf1 *SpiritWolf
	SpiritWolf2 *SpiritWolf
}

func (SpiritWolves *SpiritWolves) EnableWithTimeout(sim *core.Simulation) {
	SpiritWolves.SpiritWolf1.EnableWithTimeout(sim, SpiritWolves.SpiritWolf1, time.Second*45)
	SpiritWolves.SpiritWolf2.EnableWithTimeout(sim, SpiritWolves.SpiritWolf2, time.Second*45)
}

func (SpiritWolves *SpiritWolves) CancelGCDTimer(sim *core.Simulation) {
	SpiritWolves.SpiritWolf1.CancelGCDTimer(sim)
	SpiritWolves.SpiritWolf2.CancelGCDTimer(sim)
}

// No pet_levelstats row for 29264, so InitStatsForLevel's fallback stats, and a creature's 5% crit, nothing
// from agility (Unit::GetUnitCriticalChance)
var spiritWolfBaseStats = core.GuardianCreateStats.Add(stats.Stats{
	stats.AttackPower: -20,
	stats.MeleeCrit:   5 * core.CritRatingPerCritChance,
})

func (shaman *Shaman) NewSpiritWolf(index int) *SpiritWolf {
	spiritWolf := &SpiritWolf{
		Pet:         core.NewPet("Spirit Wolf "+strconv.Itoa(index), &shaman.Character, spiritWolfBaseStats, shaman.makeStatInheritance(), false, true),
		shamanOwner: shaman,
	}
	// spell_sha_feral_spirit_scaling ticks every second, guardian or not
	spiritWolf.ScalingPeriod = time.Second
	spiritWolf.BuffImmunity = core.PetImmuneToScaling

	// mod-spell-tweaks' 425792: immune to direct haste and slows, Bloodlust included; its own melee
	// swing speed tracks the shaman's melee speed instead (Windfury Totem and Improved Icy Talons
	// already reach the wolves through it, so they'd otherwise double up).
	if shaman.Server().SpellTweaks.FeralSpiritHaste {
		spiritWolf.HasteCarrier = true
		spiritWolf.OwnerHasteSource = func() float64 { return shaman.SwingSpeed() }
		spiritWolf.OwnerHasteCoversMeleeHasteAuras = true
	}

	// InitStatsForLevel's NPC_FERAL_SPIRIT case
	spiritWolf.EnableAutoAttacks(spiritWolf, core.AutoAttackOptions{
		MainHand: core.Weapon{
			BaseDamageMin:  240,
			BaseDamageMax:  400,
			SwingSpeed:     1.5,
			CritMultiplier: 2,
		},
		AutoSwingMelee: true,
	})

	spiritWolf.AddStatDependency(stats.Strength, stats.AttackPower, 2)

	shaman.AddPet(spiritWolf)

	return spiritWolf
}

// makeStatInheritance is spell_sha_feral_spirit_scaling: 30% of the owner's stamina, and 30% of his
// attack power as attack power and as spell damage, 60% with Glyph of Feral Spirit. 35676's armor
// has no script, so it stays at its 0.
func (shaman *Shaman) makeStatInheritance() core.PetStatInheritance {
	apPct := core.TernaryFloat64(shaman.HasMajorGlyph(proto.ShamanMajorGlyph_GlyphOfFeralSpirit), 60, 30)
	return func(ownerStats stats.Stats) stats.Stats {
		return stats.Stats{
			stats.Stamina:     core.PetScalingPct(ownerStats[stats.Stamina], 30),
			stats.AttackPower: core.PetScalingPct(ownerStats[stats.AttackPower], apPct),
			stats.SpellPower:  core.PetScalingPct(ownerStats[stats.AttackPower], apPct),
		}
	}
}

func (spiritWolf *SpiritWolf) Initialize() {
	// Nothing
}

func (spiritWolf *SpiritWolf) ExecuteCustomRotation(_ *core.Simulation) {
}

func (spiritWolf *SpiritWolf) Reset(sim *core.Simulation) {
	spiritWolf.Disable(sim)
	if sim.Log != nil {
		spiritWolf.Log(sim, "Base Stats: %s", spiritWolfBaseStats)
		inheritedStats := spiritWolf.shamanOwner.makeStatInheritance()(spiritWolf.shamanOwner.GetStats())
		spiritWolf.Log(sim, "Inherited Stats: %s", inheritedStats)
		spiritWolf.Log(sim, "Total Stats: %s", spiritWolf.GetStats())
	}
}

func (spiritWolf *SpiritWolf) GetPet() *core.Pet {
	return spiritWolf.Pet
}
