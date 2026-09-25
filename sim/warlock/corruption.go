package warlock

import (
	"time"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
)

func (warlock *Warlock) registerCorruptionSpell() {
	canCrit := warlock.Talents.Pandemic

	warlock.Corruption = warlock.RegisterSpell(core.SpellConfig{
		ActionID:    core.ActionID{SpellID: 47813},
		SpellSchool: core.SpellSchoolShadow,
		ProcMask:    core.ProcMaskSpellDamage,
		Flags:       core.SpellFlagHauntSE | core.SpellFlagAPL,

		ManaCost: core.ManaCostOptions{
			BaseCost:   0.14,
			Multiplier: 1 - 0.02*float64(warlock.Talents.Suppression),
		},
		Cast: core.CastConfig{
			DefaultCast: core.Cast{
				GCD: core.GCDDefault,
			},
		},

		BonusCritRating: 0 +
			3*float64(warlock.Talents.Malediction)*core.CritRatingPerCritChance +
			core.TernaryFloat64(warlock.HasSetBonus(ItemSetDarkCovensRegalia, 2), 5*core.CritRatingPerCritChance, 0),
		DamageMultiplier: spellModDamage(
			warlock.GrandSpellstoneBonus(),
			0.03*float64(warlock.Talents.ShadowMastery),
			0.01*float64(warlock.Talents.Contagion),
			0.02*float64(warlock.Talents.ImprovedCorruption),
			core.TernaryFloat64(warlock.Talents.SiphonLife, 0.05, 0),
			core.TernaryFloat64(warlock.HasSetBonus(ItemSetGuldansRegalia, 4), 0.1, 0),
		),
		CritMultiplier:   warlock.SpellCritMultiplier(1, core.TernaryFloat64(warlock.Talents.Pandemic, 1, 0)),
		ThreatMultiplier: 1 - 0.1*float64(warlock.Talents.ImprovedDrainSoul),

		Mods: []core.SpellMod{
			{Op: core.SpellModBonusMultiplier, Flat: 2 * warlock.Talents.EmpoweredCorruption},
			{Op: core.SpellModBonusMultiplier, Flat: warlock.Talents.EverlastingAffliction},
		},

		Dot: core.DotConfig{
			Aura: core.Aura{
				Label: "Corruption",
			},
			NumberOfTicks:       6,
			TickLength:          time.Second * 3,
			AffectedByCastSpeed: warlock.HasMajorGlyph(proto.WarlockMajorGlyph_GlyphOfQuickDecay),
			TicksCanCrit:        canCrit,

			Tick: core.SpellEffect{Effect: 0, Min: 180, Max: 180, SP: 0.2},

			OnSnapshot: func(sim *core.Simulation, target *core.Unit, dot *core.Dot, isRollover bool) {
				dot.SnapshotBaseDamage = dot.Tick.Roll(sim) + dot.Tick.SP*dot.Spell.SpellPower()
				if !isRollover {
					attackTable := dot.Spell.Unit.AttackTables[target.UnitIndex]
					dot.SnapshotCritChance = dot.Spell.SpellCritChance(target)
					dot.SnapshotAttackerMultiplier = dot.Spell.AttackerDamageMultiplier(attackTable)
				}
			},
			OnTick: func(sim *core.Simulation, target *core.Unit, dot *core.Dot) {
				if canCrit {
					dot.CalcAndDealPeriodicSnapshotDamage(sim, target, dot.OutcomeSnapshotCrit)
				} else {
					dot.CalcAndDealPeriodicSnapshotDamage(sim, target, dot.OutcomeTickCounted)
				}
			},
		},

		ApplyEffects: func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
			result := spell.CalcOutcome(sim, target, spell.OutcomeMagicHit)
			if result.Landed() {
				spell.SpellMetrics[target.UnitIndex].Hits--
				spell.Dot(target).Apply(sim)
			}
		},
		ExpectedTickDamage: func(sim *core.Simulation, target *core.Unit, spell *core.Spell, useSnapshot bool) *core.SpellResult {
			if useSnapshot {
				dot := spell.Dot(target)
				return dot.CalcSnapshotDamage(sim, target, dot.OutcomeExpectedMagicSnapshotCrit)
			} else {
				tick := &spell.Dot(target).Tick
				baseDmg := tick.Average() + tick.SP*spell.SpellPower()
				return spell.CalcPeriodicDamage(sim, target, baseDmg, spell.OutcomeExpectedMagicCrit)
			}
		},
	})
}
