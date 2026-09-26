package mage

import (
	"time"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
)

func (mage *Mage) registerFireballSpell() {
	hasGlyph := mage.HasMajorGlyph(proto.MageMajorGlyph_GlyphOfFireball)

	mage.Fireball = mage.RegisterSpell(core.SpellConfig{
		ActionID:     core.ActionID{SpellID: 42833},
		SpellSchool:  core.SpellSchoolFire,
		ProcMask:     core.ProcMaskSpellDamage,
		Flags:        SpellFlagMage | BarrageSpells | HotStreakSpells | core.SpellFlagAPL,
		MissileSpeed: 24,

		ManaCost: core.ManaCostOptions{
			BaseCost: 0.19,
		},
		Cast: core.CastConfig{
			DefaultCast: core.Cast{
				GCD: core.GCDDefault,
				CastTime: time.Millisecond*3500 -
					time.Millisecond*100*time.Duration(mage.Talents.ImprovedFireball) -
					core.TernaryDuration(hasGlyph, time.Millisecond*150, 0),
			},
		},

		BonusCritRating: 0 +
			2*float64(mage.Talents.CriticalMass)*core.CritRatingPerCritChance +
			float64(mage.Talents.ImprovedScorch)*core.CritRatingPerCritChance +
			core.TernaryFloat64(mage.HasSetBonus(ItemSetKhadgarsRegalia, 4), 5*core.CritRatingPerCritChance, 0),
		DamageMultiplier: 1 *
			(1 + .04*float64(mage.Talents.TormentTheWeak)),
		DamageMultiplierAdditive: spellModDamage(
			.02*float64(mage.Talents.SpellImpact),
			.02*float64(mage.Talents.FirePower),
			core.TernaryFloat64(mage.HasSetBonus(ItemSetTempestRegalia, 4), .05, 0),
		),
		CritMultiplier:   mage.SpellCritMultiplier(1, mage.bonusCritDamage),
		ThreatMultiplier: 1 - 0.1*float64(mage.Talents.BurningSoul),

		Direct: core.SpellEffect{Effect: 0, Min: 898, Max: 1142, SP: 1},
		Mods: []core.SpellMod{
			{Op: core.SpellModBonusMultiplier, Flat: 5 * mage.Talents.EmpoweredFire},
		},

		Dot: core.DotConfig{
			Aura: core.Aura{
				Label: "Fireball",
			},
			NumberOfTicks: 4,
			TickLength:    time.Second * 2,
			// No SPELL_AURA_ABILITY_PERIODIC_CRIT aura covers this dot, so its ticks never crit.
			TicksCanCrit: false,

			Tick: core.SpellEffect{Effect: 1, Min: 29, Max: 29},

			OnSnapshot: func(sim *core.Simulation, target *core.Unit, dot *core.Dot, _ bool) {
				dot.SnapshotBaseDamage = dot.Tick.Roll(sim)
				dot.SnapshotAttackerMultiplier = dot.Spell.AttackerDamageMultiplier(dot.Spell.Unit.AttackTables[target.UnitIndex])
			},
			OnTick: func(sim *core.Simulation, target *core.Unit, dot *core.Dot) {
				dot.CalcAndDealPeriodicSnapshotDamage(sim, target, dot.OutcomeTick)
			},
		},

		ApplyEffects: func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
			baseDamage := spell.Direct.Roll(sim) + spell.Direct.SP*spell.SpellPower()
			result := spell.CalcDamage(sim, target, baseDamage, spell.OutcomeMagicHitAndCrit)
			spell.WaitTravelTime(sim, func(sim *core.Simulation) {
				if result.Landed() && !hasGlyph {
					spell.Dot(target).Apply(sim)
				}
				spell.DealDamage(sim, result)
			})
		},
	})
}
