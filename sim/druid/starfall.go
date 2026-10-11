package druid

import (
	"time"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
)

// We register two spells to apply two different dot effects and get two entries in Damage/Detailed results
func (druid *Druid) registerStarfallSpell() {
	if !druid.Talents.Starfall {
		return
	}

	numberOfTicks := core.TernaryInt32(druid.Env.GetNumTargets() > 1, 20, 10)
	tickLength := time.Second

	starfallTickSpell := druid.RegisterSpell(Humanoid|Moonkin, core.SpellConfig{
		ActionID:    core.ActionID{SpellID: 53195},
		SpellSchool: core.SpellSchoolArcane,
		// a star carries SPELL_ATTR3_NOT_A_PROC, so it procs what a cast spell does (Illustration of the Dragon
		// Soul), like Arcane Missiles' missiles; the splash doesn't
		ProcMask: core.ProcMaskSpellDamage | core.ProcMaskNotInSpellbook,
		// Omen of Clarity procs from procs, so each star that lands rolls it
		Flags:            SpellFlagNaturesGrace | SpellFlagOmenTrigger,
		BonusCritRating:  2 * float64(druid.Talents.NaturesMajesty) * core.CritRatingPerCritChance,
		DamageMultiplier: 1 * (1 + core.TernaryFloat64(druid.HasMajorGlyph(proto.DruidMajorGlyph_GlyphOfFocus), 0.1, 0)),
		CritMultiplier:   druid.BalanceCritMultiplier(),
		ThreatMultiplier: 1,
		Direct:           core.SpellEffect{Effect: 0, Min: 563, Max: 653, SP: 0.3},
		ApplyEffects: func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
			baseDamage := spell.Direct.Roll(sim) + core.PowerBonus(spell.Direct.SP, spell.SpellPower())
			spell.CalcAndDealDamage(sim, target, baseDamage, spell.OutcomeMagicHitAndCrit)
		},
	})

	druid.Starfall = druid.RegisterSpell(Humanoid|Moonkin, core.SpellConfig{
		ActionID:    core.ActionID{SpellID: 53201},
		SpellSchool: core.SpellSchoolArcane,
		ProcMask:    core.ProcMaskSpellDamage,
		Flags:       core.SpellFlagAPL | SpellFlagOmenTrigger,
		ManaCost: core.ManaCostOptions{
			BaseCost:   0.35,
			Multiplier: 1 - 0.03*float64(druid.Talents.Moonglow),
		},
		Cast: core.CastConfig{
			DefaultCast: core.Cast{
				GCD: core.GCDDefault,
			},
			CD: core.Cooldown{
				Timer:    druid.NewTimer(),
				Duration: time.Second * (90 - core.TernaryDuration(druid.HasMajorGlyph(proto.DruidMajorGlyph_GlyphOfStarfall), 30, 0)),
			},
		},
		Dot: core.DotConfig{
			Aura: core.Aura{
				Label: "Starfall",
			},
			NumberOfTicks: numberOfTicks,
			TickLength:    tickLength,
			TicksCanCrit:  false,
			OnTick: func(sim *core.Simulation, target *core.Unit, dot *core.Dot) {
				starfallTickSpell.Cast(sim, target)
			},
		},

		ThreatMultiplier: 1,
		// 53201 is an aura on the druid, so it never misses
		ApplyEffects: func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
			spell.CalcAndDealOutcome(sim, target, spell.OutcomeAlwaysHit)
			spell.Dot(target).Apply(sim)
			druid.StarfallSplash.Dot(target).Apply(sim)
		},
	})

	starfallSplashTickSpell := druid.RegisterSpell(Any, core.SpellConfig{
		ActionID:    core.ActionID{SpellID: 53190},
		SpellSchool: core.SpellSchoolArcane,
		ProcMask:    core.ProcMaskSuppressedProc,
		// rolls Omen of Clarity per target hit, like the stars
		Flags:            SpellFlagOmenTrigger,
		BonusCritRating:  2 * float64(druid.Talents.NaturesMajesty) * core.CritRatingPerCritChance,
		DamageMultiplier: 1 * (1 + core.TernaryFloat64(druid.HasMajorGlyph(proto.DruidMajorGlyph_GlyphOfFocus), 0.1, 0)),
		CritMultiplier:   druid.BalanceCritMultiplier(),
		ThreatMultiplier: 1,
		Direct:           core.SpellEffect{Effect: 0, Min: 101, Max: 101, SP: 0.13},
		ApplyEffects: func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
			baseDamage := spell.Direct.Roll(sim) + core.PowerBonus(spell.Direct.SP, spell.SpellPower())
			// the server's script drops the star's own target from the splash, and the cap counts the rest
			baseDamage *= core.AOECapMultiplierFor(len(sim.Encounter.Targets) - 1)
			for _, aoeTarget := range sim.Encounter.TargetUnits {
				if aoeTarget == target {
					continue
				}
				spell.CalcAndDealDamage(sim, aoeTarget, baseDamage, spell.OutcomeMagicHitAndCrit)
			}
		},
	})

	druid.StarfallSplash = druid.RegisterSpell(Any, core.SpellConfig{
		ActionID: core.ActionID{SpellID: 53190},
		Dot: core.DotConfig{
			Aura: core.Aura{
				Label: "StarfallSplash",
			},
			NumberOfTicks: numberOfTicks,
			TickLength:    tickLength,
			TicksCanCrit:  false,
			OnTick: func(sim *core.Simulation, target *core.Unit, dot *core.Dot) {
				starfallSplashTickSpell.Cast(sim, target)
			},
		},
	})
}
