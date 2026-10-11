package mage

import (
	"time"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
	"github.com/wowsims/wotlk/sim/core/stats"
)

// The numbers in this file are VERY rough approximations based on logs.

func (mage *Mage) registerSummonWaterElementalCD() {
	if !mage.Talents.SummonWaterElemental {
		return
	}

	if mage.HasMajorGlyph(proto.MageMajorGlyph_GlyphOfEternalWater) {
		// Makes pet permanent, so doesn't use the CD.
		return
	}

	summonDuration := time.Second*45 + time.Second*5*time.Duration(mage.Talents.EnduringWinter)
	mage.SummonWaterElemental = mage.RegisterSpell(core.SpellConfig{
		ActionID: core.ActionID{SpellID: 31687},
		Flags:    core.SpellFlagAPL,

		ManaCost: core.ManaCostOptions{
			BaseCost: 0.16,
		},
		Cast: core.CastConfig{
			DefaultCast: core.Cast{
				GCD: core.GCDDefault,
			},
			CD: core.Cooldown{
				Timer: mage.NewTimer(),
				Duration: time.Duration(float64(time.Minute*3)*(1-0.1*float64(mage.Talents.ColdAsIce))) -
					core.TernaryDuration(mage.HasMajorGlyph(proto.MageMajorGlyph_GlyphOfWaterElemental), time.Second*30, 0),
			},
		},
		ExtraCastCondition: func(sim *core.Simulation, target *core.Unit) bool {
			return !mage.waterElemental.IsEnabled()
		},

		ApplyEffects: func(sim *core.Simulation, _ *core.Unit, _ *core.Spell) {
			mage.waterElemental.EnableWithTimeout(sim, mage.waterElemental, summonDuration)
		},
	})

	mage.AddMajorCooldown(core.MajorCooldown{
		Spell:    mage.SummonWaterElemental,
		Priority: core.CooldownPriorityDrums + 1000, // Always prefer to cast before drums or lust so the ele gets their benefits.
		Type:     core.CooldownTypeDPS,
	})
}

type WaterElemental struct {
	*core.Pet

	// Water Ele almost never just stands still and spams like we want, it sometimes
	// does its own thing. This controls how much it does that.
	disobeyChance float64

	Waterbolt *core.Spell
}

func (mage *Mage) NewWaterElemental(disobeyChance float64) *WaterElemental {
	// Glyph of Eternal Water summons a real pet: its scaling ticks, and only a guardian is immune to it
	permanent := mage.HasMajorGlyph(proto.MageMajorGlyph_GlyphOfEternalWater)
	waterElemental := &WaterElemental{
		Pet:           core.NewPet("Water Elemental", &mage.Character, waterElementalBaseStats, waterElementalStatInheritance, permanent, true),
		disobeyChance: disobeyChance,
	}
	if permanent {
		waterElemental.SummonedAsPet = true
		waterElemental.ScalingPeriod = 2 * time.Second
	} else {
		waterElemental.BuffImmunity = core.PetImmuneToScaling
	}
	waterElemental.EnableCreatureManaBar(2908, waterElementalBaseStats[stats.Intellect], 4.95, true)
	waterElemental.AddStatDependency(stats.Strength, stats.AttackPower, 2)

	mage.AddPet(waterElemental)

	return waterElemental
}

func (we *WaterElemental) GetPet() *core.Pet {
	return we.Pet
}

func (we *WaterElemental) Initialize() {
	we.registerWaterboltSpell()
}

func (we *WaterElemental) Reset(_ *core.Simulation) {
}

func (we *WaterElemental) ExecuteCustomRotation(sim *core.Simulation) {
	spell := we.Waterbolt

	if sim.RandomFloat("Water Elemental Disobey") < we.disobeyChance {
		// Water ele has decided not to cooperate, so just wait for the cast time
		// instead of casting.
		we.WaitUntil(sim, sim.CurrentTime+spell.DefaultCast.CastTime)
		return
	}

	spell.Cast(sim, we.CurrentTarget)
}

// pet_levelstats for 510 at 80, the same as the glyph's 37994, and 2 * Str - 20 attack power
var waterElementalBaseStats = core.PetCreateStats(203, 159, 118, 369, 240, 7782).Add(stats.Stats{
	stats.AttackPower: -20,
	// spell_mage_pet_scaling never sets an amount for a pet's crit, so it keeps the creature default
	// (Unit::GetUnitCriticalChance): a flat 5%, nothing from agility.
	stats.SpellCrit: 5 * core.CritRatingPerCritChance,
})

// spell_mage_pet_scaling::CalculateStatAmount/CalculateSPAmount (spell_mage.cpp:266-296): 30% of the
// owner's stamina and intellect, 33% of the owner's frost spell power (the sim's one spell power
// stat, in practice), 35% of his armor.
var waterElementalStatInheritance = func(ownerStats stats.Stats) stats.Stats {
	return stats.Stats{
		stats.Stamina:    core.PetScalingPct(ownerStats[stats.Stamina], 30),
		stats.Intellect:  core.PetScalingPct(ownerStats[stats.Intellect], 30),
		stats.SpellPower: core.PetScalingPct(ownerStats[stats.SpellPower], 33),
		stats.Armor:      core.PetScalingPct(ownerStats[stats.Armor], 35),
	}
}

func (we *WaterElemental) registerWaterboltSpell() {
	we.Waterbolt = we.RegisterSpell(core.SpellConfig{
		ActionID:     core.ActionID{SpellID: 31707},
		SpellSchool:  core.SpellSchoolFrost,
		ProcMask:     core.ProcMaskSpellDamage,
		MissileSpeed: 16,

		ManaCost: core.ManaCostOptions{
			BaseCost: 0.01,
		},
		Cast: core.CastConfig{
			DefaultCast: core.Cast{
				GCD:      core.GCDDefault,
				CastTime: time.Millisecond * 2500,
			},
		},

		DamageMultiplier: 1,
		CritMultiplier:   we.DefaultSpellCritMultiplier(),
		ThreatMultiplier: 1,

		Direct: core.SpellEffect{Effect: 0, Min: 601, Max: 673, SP: 0.83},

		ApplyEffects: func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
			baseDamage := spell.Direct.Roll(sim) + core.PowerBonus(spell.Direct.SP, spell.SpellPower())
			result := spell.CalcDamage(sim, target, baseDamage, spell.OutcomeMagicHitAndCrit)
			spell.DealDamageAfterTravel(sim, result)
		},
	})
}
