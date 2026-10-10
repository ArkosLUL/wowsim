package hunter

import (
	"time"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
)

func (hunter *Hunter) registerSerpentStingSpell() {
	canCrit := hunter.HasSetBonus(ItemSetWindrunnersPursuit, 2)

	hunter.SerpentSting = hunter.RegisterSpell(core.SpellConfig{
		ActionID:     core.ActionID{SpellID: 49001},
		SpellSchool:  core.SpellSchoolNature,
		ProcMask:     core.ProcMaskEmpty,
		Flags:        core.SpellFlagAPL,
		MissileSpeed: 40,

		ManaCost: core.ManaCostOptions{
			BaseCost:   0.09,
			Multiplier: 1 - 0.03*float64(hunter.Talents.Efficiency),
		},
		Cast: core.CastConfig{
			DefaultCast: core.Cast{
				GCD: core.GCDDefault,
			},
			IgnoreHaste: true, // Hunter GCD is locked at 1.5s
		},

		// Need to specially apply LethalShots here, because this spell uses an empty proc mask
		BonusCritRating: 1 * core.CritRatingPerCritChance * float64(hunter.Talents.LethalShots),

		DamageMultiplierAdditive: spellModDamage(
			0.1*float64(hunter.Talents.ImprovedStings),
			core.TernaryFloat64(hunter.HasSetBonus(ItemSetScourgestalkerBattlegear, 2), .1, 0),
		),
		DamageMultiplier: 1,
		CritMultiplier:   hunter.critMultiplier(true, false),
		ThreatMultiplier: 1,

		Dot: core.DotConfig{
			Aura: core.Aura{
				Label: "SerpentSting",
				Tag:   "SerpentSting",
			},
			NumberOfTicks: 5 + core.TernaryInt32(hunter.HasMajorGlyph(proto.HunterMajorGlyph_GlyphOfSerpentSting), 2, 0),
			TickLength:    time.Second * 3,
			// the T9 2pc (67150) is an SPELL_AURA_ABILITY_PERIODIC_CRIT on Serpent Sting
			TicksCanCrit: canCrit,

			Tick: core.SpellEffect{Effect: 0, Min: 242, Max: 242, AP: 0.04},

			OnSnapshot: func(sim *core.Simulation, target *core.Unit, dot *core.Dot, isRollover bool) {
				dot.SnapshotBaseDamage = dot.Tick.Roll(sim) + core.PowerBonus(dot.Tick.AP*dot.Spell.RangedAttackPowerSpellBonus(target))
				if !isRollover {
					attackTable := dot.Spell.Unit.AttackTables[target.UnitIndex]
					dot.SnapshotCritChance = dot.Spell.PhysicalCritChance(attackTable)
					dot.SnapshotAttackerMultiplier = dot.Spell.AttackerDamageMultiplier(attackTable)
				}
			},
			OnTick: func(sim *core.Simulation, target *core.Unit, dot *core.Dot) {
				if canCrit {
					dot.CalcAndDealPeriodicSnapshotDamage(sim, target, dot.OutcomeSnapshotCrit)
				} else {
					dot.CalcAndDealPeriodicSnapshotDamage(sim, target, dot.OutcomeTick)
				}
			},
		},

		ApplyEffects: func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
			result := spell.CalcOutcome(sim, target, spell.OutcomeRangedHit)
			spell.WaitTravelTime(sim, func(sim *core.Simulation) {
				if result.Landed() {
					spell.SpellMetrics[target.UnitIndex].Hits--
					spell.Dot(target).Apply(sim)
				}
				spell.DealOutcome(sim, result)
			})
		},
	})
}
