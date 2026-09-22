package shaman

import (
	"math"
	"strconv"
	"time"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
	"github.com/wowsims/wotlk/sim/core/stats"
)

type SpiritWolf struct {
	core.Pet

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

var spiritWolfBaseStats = stats.Stats{
	stats.Stamina:   361,
	stats.Spirit:    109,
	stats.Intellect: 65,
	stats.Armor:     9616,

	stats.Agility:     113,
	stats.Strength:    331,
	stats.AttackPower: -20,

	// Unit::GetUnitCriticalChance gives every non-player unit a flat 5% base; a scripted aura adds the
	// agility scaling below on top of it, the wolves keeping it where the DK's and hunter's summons lost it.
	stats.MeleeCrit: 5 * core.CritRatingPerCritChance,
}

func (shaman *Shaman) NewSpiritWolf(index int) *SpiritWolf {
	spiritWolf := &SpiritWolf{
		Pet:         core.NewPet("Spirit Wolf "+strconv.Itoa(index), &shaman.Character, spiritWolfBaseStats, shaman.makeStatInheritance(), false, false),
		shamanOwner: shaman,
	}

	// mod-spell-tweaks' 425792: immune to direct haste and slows, Bloodlust included; its own melee
	// swing speed tracks the shaman's melee speed instead (Windfury Totem and Improved Icy Talons
	// already reach the wolves through it, so they'd otherwise double up).
	if shaman.Server().SpellTweaks.FeralSpiritHaste {
		spiritWolf.HasteCarrier = true
		spiritWolf.OwnerHasteSource = func() float64 { return shaman.SwingSpeed() }
	}

	spiritWolf.EnableAutoAttacks(spiritWolf, core.AutoAttackOptions{
		MainHand: core.Weapon{
			BaseDamageMin:  246,
			BaseDamageMax:  372,
			SwingSpeed:     1.5,
			CritMultiplier: 2,
		},
		AutoSwingMelee: true,
	})

	spiritWolf.AddStatDependency(stats.Strength, stats.AttackPower, 2)
	spiritWolf.AddStatDependency(stats.Agility, stats.MeleeCrit, core.CritRatingPerCritChance/83.3)
	core.ApplyPetConsumeEffects(&spiritWolf.Character, shaman.Consumes)

	shaman.AddPet(spiritWolf)

	return spiritWolf
}

const PetExpertiseScale = 3.25

func (shaman *Shaman) makeStatInheritance() core.PetStatInheritance {
	return func(ownerStats stats.Stats) stats.Stats {
		ownerHitChance := ownerStats[stats.MeleeHit] / core.MeleeHitRatingPerHitChance
		hitRatingFromOwner := math.Floor(ownerHitChance) * core.MeleeHitRatingPerHitChance

		return stats.Stats{
			stats.Stamina:     ownerStats[stats.Stamina] * 0.3,
			stats.Armor:       ownerStats[stats.Armor] * 0.35,
			stats.AttackPower: ownerStats[stats.AttackPower] * (core.TernaryFloat64(shaman.HasMajorGlyph(proto.ShamanMajorGlyph_GlyphOfFeralSpirit), 0.60, 0.30)),

			stats.MeleeHit:  hitRatingFromOwner,
			stats.Expertise: math.Floor(math.Floor(ownerHitChance)*PetExpertiseScale) * core.ExpertisePerQuarterPercentReduction,
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
	return &spiritWolf.Pet
}
