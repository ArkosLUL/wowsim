package hunter

import (
	"fmt"
	"time"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
)

func (hunter *Hunter) registerExplosiveShotSpell(timer *core.Timer) {
	if !hunter.Talents.ExplosiveShot {
		return
	}

	hunter.ExplosiveShotR4 = hunter.makeExplosiveShotSpell(timer, false)
	hunter.ExplosiveShotR3 = hunter.makeExplosiveShotSpell(timer, true)
}

func (hunter *Hunter) makeExplosiveShotSpell(timer *core.Timer, downrank bool) *core.Spell {
	actionID := core.ActionID{SpellID: 60053}
	tick := core.SpellEffect{Effect: 0, Min: 386, Max: 464}
	if downrank {
		actionID = core.ActionID{SpellID: 60052}
		tick = core.SpellEffect{Effect: 0, Min: 325, Max: 391}
	}

	return hunter.RegisterSpell(core.SpellConfig{
		ActionID:     actionID,
		SpellSchool:  core.SpellSchoolFire,
		ProcMask:     core.ProcMaskRangedSpecial,
		Flags:        core.SpellFlagMeleeMetrics | core.SpellFlagAPL,
		MissileSpeed: 40,

		ManaCost: core.ManaCostOptions{
			BaseCost:   0.07,
			Multiplier: 1 - 0.03*float64(hunter.Talents.Efficiency),
		},
		Cast: core.CastConfig{
			DefaultCast: core.Cast{
				GCD: core.GCDDefault,
			},
			IgnoreHaste: true,
			CD: core.Cooldown{
				Timer:    timer,
				Duration: time.Second * 6,
			},
		},

		BonusCritRating: 0 +
			2*core.CritRatingPerCritChance*float64(hunter.Talents.SurvivalInstincts) +
			core.TernaryFloat64(hunter.HasMajorGlyph(proto.HunterMajorGlyph_GlyphOfExplosiveShot), 4*core.CritRatingPerCritChance, 0),
		DamageMultiplierAdditive: 1 +
			.02*float64(hunter.Talents.TNT),
		DamageMultiplier: 1,
		CritMultiplier:   hunter.critMultiplier(true, false),
		ThreatMultiplier: 1,

		// each tick casts 53352 with the rolled amount in place of its own 10
		// (AuraEffect::HandlePeriodicDummyAuraTick), adding 53352's ranged AP
		Direct: core.SpellEffect{Effect: 0, FromSpellID: 53352, Min: 10, Max: 10, AP: 0.16},

		Dot: core.DotConfig{
			Aura: core.Aura{
				Label: fmt.Sprintf("ExplosiveShot-%d", actionID.SpellID),
			},
			NumberOfTicks: 2,
			TickLength:    time.Second * 1,
			// 53352 is a hit that crits like any other but never misses (SPELL_ATTR3_ALWAYS_HIT)
			TicksCanCrit: true,
			Tick:         tick,
			OnSnapshot: func(sim *core.Simulation, target *core.Unit, dot *core.Dot, isRollover bool) {
				dot.SnapshotBaseDamage = dot.Tick.Roll(sim) + dot.Spell.Direct.AP*dot.Spell.RangedAttackPowerSpellBonus(target)
				attackTable := dot.Spell.Unit.AttackTables[target.UnitIndex]
				dot.SnapshotCritChance = dot.Spell.PhysicalCritChance(attackTable)
				dot.SnapshotAttackerMultiplier = dot.Spell.AttackerDamageMultiplier(attackTable)
			},
			OnTick: func(sim *core.Simulation, target *core.Unit, dot *core.Dot) {
				dot.CalcAndDealPeriodicSnapshotDamage(sim, target, dot.OutcomeSnapshotCrit)
			},
		},

		ApplyEffects: func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
			result := spell.CalcOutcome(sim, target, spell.OutcomeRangedHit)
			spell.WaitTravelTime(sim, func(sim *core.Simulation) {
				spell.DealOutcome(sim, result)
				if result.Landed() {
					spell.SpellMetrics[target.UnitIndex].Hits--
					dot := spell.Dot(target)
					dot.Apply(sim)
					dot.TickOnce(sim)
				}
			})
		},
	})
}
