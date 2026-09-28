package warlock

import (
	"time"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
)

func (warlock *Warlock) registerConflagrateSpell() {
	if !warlock.Talents.Conflagrate {
		return
	}

	hasGlyphOfConflag := warlock.HasMajorGlyph(proto.WarlockMajorGlyph_GlyphOfConflagrate)
	// The hit is a share of the consumed Immolate's aura amount, which already carries Immolate's own
	// periodic mods (AuraEffect::CalculateAmount), and it skips Conflagrate's own done mods
	// (apply_direct_bonus off), so Firestone's and the T8 2pc's Conflagrate mods never reach it.
	immolatePeriodicMods := spellModDamage(
		0.03*float64(warlock.Talents.Emberstorm),
		0.1*float64(warlock.Talents.ImprovedImmolate),
		core.TernaryFloat64(warlock.HasSetBonus(ItemSetDeathbringerGarb, 2), 0.1, 0),
		core.TernaryFloat64(warlock.HasSetBonus(ItemSetGuldansRegalia, 4), 0.1, 0),
		0.03*float64(warlock.Talents.Aftermath),
		core.TernaryFloat64(warlock.HasMajorGlyph(proto.WarlockMajorGlyph_GlyphOfImmolate), 0.1, 0),
		warlock.GrandSpellstoneBonus(),
	)
	// the dot's own amount then takes Conflagrate's SPELLMOD_DOT, and only Emberstorm's mask covers it
	dotOnlyMultiplier := spellModDamage(0.03 * float64(warlock.Talents.Emberstorm))
	// Spell::EffectSchoolDMG scripts Conflagrate from the consumed Immolate's five ticks: the hit adds
	// effect 1's value as a percent of them, and each dot tick deals effect 2's 40/3 = 13 (integer) percent.
	immolateTicks := func(target *core.Unit) float64 {
		return 5 * warlock.Immolate.Dot(target).SnapshotBaseDamage
	}
	warlock.Conflagrate = warlock.RegisterSpell(core.SpellConfig{
		ActionID:    core.ActionID{SpellID: 17962},
		SpellSchool: core.SpellSchoolFire,
		ProcMask:    core.ProcMaskSpellDamage,
		Flags:       core.SpellFlagAPL,

		ManaCost: core.ManaCostOptions{
			BaseCost:   0.16,
			Multiplier: 1 - []float64{0, .04, .07, .10}[warlock.Talents.Cataclysm],
		},
		Cast: core.CastConfig{
			DefaultCast: core.Cast{
				GCD: core.GCDDefault,
			},
			CD: core.Cooldown{
				Timer:    warlock.NewTimer(),
				Duration: time.Second * 10,
			},
		},
		ExtraCastCondition: func(sim *core.Simulation, target *core.Unit) bool {
			return warlock.Immolate.Dot(target).IsActive()
		},

		BonusCritRating: 0 +
			core.TernaryFloat64(warlock.Talents.Devastation, 5*core.CritRatingPerCritChance, 0) +
			5*float64(warlock.Talents.FireAndBrimstone)*core.CritRatingPerCritChance,
		DamageMultiplier: immolatePeriodicMods,
		CritMultiplier:   warlock.SpellCritMultiplier(1, float64(warlock.Talents.Ruin)/5),
		ThreatMultiplier: 1 - 0.1*float64(warlock.Talents.DestructiveReach),

		Direct: core.SpellEffect{Effect: 0, Min: 1, Max: 1},

		Dot: core.DotConfig{
			Aura: core.Aura{
				Label: "Conflagrate",
			},
			NumberOfTicks: 3,
			TickLength:    time.Second * 2,
			// only Improved Immolate rank 3 (17834) has the aura 286 covering Conflagrate
			TicksCanCrit: warlock.Talents.ImprovedImmolate == 3,

			Tick: core.SpellEffect{Effect: 1, Min: 60, Max: 60},

			OnSnapshot: func(sim *core.Simulation, target *core.Unit, dot *core.Dot, isRollover bool) {
				dot.SnapshotBaseDamage = 0.13 * immolateTicks(target)
				attackTable := dot.Spell.Unit.AttackTables[target.UnitIndex]
				dot.SnapshotCritChance = dot.Spell.SpellCritChance(target)

				dot.Spell.DamageMultiplier *= dotOnlyMultiplier
				dot.SnapshotAttackerMultiplier = dot.Spell.AttackerDamageMultiplier(attackTable)
				dot.Spell.DamageMultiplier /= dotOnlyMultiplier
			},
			OnTick: func(sim *core.Simulation, target *core.Unit, dot *core.Dot) {
				dot.CalcAndDealPeriodicSnapshotDamage(sim, target, dot.OutcomeSnapshotCrit)
			},
		},

		ApplyEffects: func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
			baseDamage := spell.Direct.Roll(sim) + spell.Dot(target).Tick.Roll(sim)/100*immolateTicks(target)
			result := spell.CalcAndDealDamage(sim, target, baseDamage, spell.OutcomeMagicHitAndCrit)
			if !result.Landed() {
				return
			}

			spell.Dot(target).Apply(sim)

			if !hasGlyphOfConflag {
				warlock.Immolate.Dot(target).Deactivate(sim)
				//warlock.ShadowflameDot.Deactivate(sim)
			}
		},
	})
}
