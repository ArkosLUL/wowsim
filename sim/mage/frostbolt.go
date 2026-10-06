package mage

import (
	"time"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
)

func (mage *Mage) registerFrostboltSpell() {
	// spell_proc -44557: a landed Frostbolt at its rank's 33, 66 or 100%, at most once every 6 s
	replProcChance := []float64{0, .33, .66, 1}[mage.Talents.EnduringWinter]
	var replSrc core.ReplenishmentSource
	var replICD core.Cooldown
	if replProcChance > 0 {
		replSrc = mage.Env.Raid.NewReplenishmentSource(core.ActionID{SpellID: 44561})
		replICD = core.Cooldown{
			Timer:    mage.NewTimer(),
			Duration: time.Second * 6,
		}
	}

	// Glyph of Frostbolt cuts the slow's duration to 0 as it goes on (Aura::HandleAuraSpecificMods)
	chills := !mage.HasMajorGlyph(proto.MageMajorGlyph_GlyphOfFrostbolt)

	mage.Frostbolt = mage.RegisterSpell(core.SpellConfig{
		ActionID:     core.ActionID{SpellID: 42842},
		SpellSchool:  core.SpellSchoolFrost,
		ProcMask:     core.ProcMaskSpellDamage,
		Flags:        SpellFlagMage | BarrageSpells | core.SpellFlagAPL,
		MissileSpeed: 28,

		ManaCost: core.ManaCostOptions{
			BaseCost: 0.11,
		},
		Cast: core.CastConfig{
			DefaultCast: core.Cast{
				GCD:      core.GCDDefault,
				CastTime: time.Second*3 - time.Millisecond*100*time.Duration(mage.Talents.ImprovedFrostbolt+mage.Talents.EmpoweredFrostbolt),
			},
		},

		BonusCritRating: 0 +
			float64(mage.Talents.WintersChill)*core.CritRatingPerCritChance +
			core.TernaryFloat64(mage.HasSetBonus(ItemSetKhadgarsRegalia, 4), 5*core.CritRatingPerCritChance, 0),
		DamageMultiplierAdditive: spellModDamage(
			.01*float64(mage.Talents.ChilledToTheBone),
			core.TernaryFloat64(mage.HasMajorGlyph(proto.MageMajorGlyph_GlyphOfFrostbolt), .05, 0),
			core.TernaryFloat64(mage.HasSetBonus(ItemSetTempestRegalia, 4), .05, 0),
		),
		CritMultiplier:   mage.SpellCritMultiplier(1, mage.bonusCritDamage+float64(mage.Talents.IceShards)/3),
		ThreatMultiplier: 1,

		Direct: core.SpellEffect{Effect: 1, Min: 803, Max: 865, SP: 0.857},
		Mods: []core.SpellMod{
			{Op: core.SpellModBonusMultiplier, Flat: 5 * mage.Talents.EmpoweredFrostbolt},
		},

		ApplyEffects: func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
			baseDamage := (spell.Direct.Roll(sim) + spell.Direct.SP*spell.SpellPower()) * mage.tormentTheWeak(target)
			result := spell.CalcDamage(sim, target, baseDamage, spell.OutcomeMagicHitAndCrit)

			spell.WaitTravelTime(sim, func(sim *core.Simulation) {
				spell.DealDamage(sim, result)
				if chills && result.Landed() {
					mage.chill(sim, target, time.Second*9)
				}
				if replProcChance > 0 && result.Landed() && replICD.IsReady(sim) &&
					(replProcChance == 1 || sim.RandomFloat("Enduring Winter") < replProcChance) {
					replICD.Use(sim)
					mage.Env.Raid.ProcReplenishment(sim, replSrc)
				}
			})
		},
	})
}
