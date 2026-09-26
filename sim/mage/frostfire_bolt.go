package mage

import (
	"time"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
)

func (mage *Mage) registerFrostfireBoltSpell() {
	glyphOfFrostfireBonus := core.TernaryFloat64(mage.HasMajorGlyph(proto.MageMajorGlyph_GlyphOfFrostfire), .02, 0)

	mage.FrostfireBolt = mage.RegisterSpell(core.SpellConfig{
		ActionID:     core.ActionID{SpellID: 47610},
		SpellSchool:  core.SpellSchoolFire | core.SpellSchoolFrost,
		ProcMask:     core.ProcMaskSpellDamage,
		Flags:        SpellFlagMage | BarrageSpells | HotStreakSpells | core.SpellFlagAPL,
		MissileSpeed: 28,

		ManaCost: core.ManaCostOptions{
			BaseCost: 0.14,
		},
		Cast: core.CastConfig{
			DefaultCast: core.Cast{
				GCD:      core.GCDDefault,
				CastTime: time.Second * 3,
			},
		},

		// FFB double-dips the bonus from Precision, so add it again here.
		BonusHitRating: float64(mage.Talents.Precision) * core.SpellHitRatingPerHitChance,
		BonusCritRating: 0 +
			core.TernaryFloat64(mage.HasSetBonus(ItemSetKhadgarsRegalia, 4), 5*core.CritRatingPerCritChance, 0) +
			core.TernaryFloat64(mage.HasMajorGlyph(proto.MageMajorGlyph_GlyphOfFrostfire), 2*core.CritRatingPerCritChance, 0) +
			2*float64(mage.Talents.CriticalMass)*core.CritRatingPerCritChance +
			1*float64(mage.Talents.ImprovedScorch)*core.CritRatingPerCritChance,
		DamageMultiplier: 1 *
			// Need to re-apply these frost talents because FFB only inherits the fire multipliers from core.
			(1 + .02*float64(mage.Talents.PiercingIce)) *
			(1 + .01*float64(mage.Talents.ArcticWinds)) *
			(1 + .04*float64(mage.Talents.TormentTheWeak)),
		DamageMultiplierAdditive: spellModDamage(
			.02*float64(mage.Talents.FirePower),
			.01*float64(mage.Talents.ChilledToTheBone),
			glyphOfFrostfireBonus,
		),
		CritMultiplier:   mage.SpellCritMultiplier(1, mage.bonusCritDamage+float64(mage.Talents.IceShards)/3),
		ThreatMultiplier: 1 - 0.1*float64(mage.Talents.BurningSoul) - .04*float64(mage.Talents.FrostChanneling),

		Direct: core.SpellEffect{Effect: 1, Min: 722, Max: 838, SP: 0.857},
		Mods: []core.SpellMod{
			{Op: core.SpellModBonusMultiplier, Flat: 5 * mage.Talents.EmpoweredFire},
		},

		Dot: core.DotConfig{
			Aura: core.Aura{
				Label: "FrostfireBolt",
			},
			NumberOfTicks: 3,
			TickLength:    time.Second * 3,
			// No SPELL_AURA_ABILITY_PERIODIC_CRIT aura covers this dot, so its ticks never crit.
			TicksCanCrit: false,

			Tick: core.SpellEffect{Effect: 2, Min: 30, Max: 30},

			OnSnapshot: func(sim *core.Simulation, target *core.Unit, dot *core.Dot, _ bool) {
				dot.SnapshotBaseDamage = dot.Tick.Roll(sim)
				// Glyph of Frostfire boosts the direct hit only, so pull its factor back out here.
				dot.Spell.DamageMultiplierAdditive /= 1 + glyphOfFrostfireBonus
				dot.SnapshotAttackerMultiplier = dot.Spell.AttackerDamageMultiplier(dot.Spell.Unit.AttackTables[target.UnitIndex])
				dot.Spell.DamageMultiplierAdditive *= 1 + glyphOfFrostfireBonus
			},
			OnTick: func(sim *core.Simulation, target *core.Unit, dot *core.Dot) {
				dot.CalcAndDealPeriodicSnapshotDamage(sim, target, dot.OutcomeTick)
			},
		},

		ApplyEffects: func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
			baseDamage := spell.Direct.Roll(sim) + spell.Direct.SP*spell.SpellPower()

			// FFB also double-dips the bonus from debuff crit modifiers:
			//  1) Totem of Wrath / Heart of the Crusader / Master Poisoner
			//  2) Shadow Mastery / Improved Scorch / Winter's Chill
			// Luckily, each of those effects has its own dedicated pseudostat, so we
			// can implement this by modifying the crit of this spell before the calc.
			doubleDipBonus := target.PseudoStats.BonusCritRatingTaken + target.PseudoStats.BonusSpellCritRatingTaken
			spell.BonusCritRating += doubleDipBonus
			result := spell.CalcDamage(sim, target, baseDamage, spell.OutcomeMagicHitAndCrit)
			spell.BonusCritRating -= doubleDipBonus

			spell.WaitTravelTime(sim, func(sim *core.Simulation) {
				if result.Landed() {
					spell.Dot(target).Apply(sim)
				}
				spell.DealDamage(sim, result)
			})
		},
	})
}
