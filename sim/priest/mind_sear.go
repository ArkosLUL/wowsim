package priest

import (
	"strconv"
	"time"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
)

func (priest *Priest) getMindSearBaseConfig() core.SpellConfig {
	return core.SpellConfig{
		SpellSchool:     core.SpellSchoolShadow,
		ProcMask:        core.ProcMaskProc,
		BonusHitRating:  float64(priest.Talents.ShadowFocus) * 1 * core.SpellHitRatingPerHitChance,
		BonusCritRating: float64(priest.Talents.MindMelt) * 2 * core.CritRatingPerCritChance,
		DamageMultiplier: spellModDamage(
			0.02*float64(priest.Talents.Darkness),
			0.01*float64(priest.Talents.TwinDisciplines),
		),
		ThreatMultiplier: 1 - 0.08*float64(priest.Talents.ShadowAffinity),
		CritMultiplier:   priest.DefaultSpellCritMultiplier(),
	}
}

func (priest *Priest) getMindSearTickSpell(numTicks int32) *core.Spell {
	hasGlyphOfShadow := priest.HasGlyph(int32(proto.PriestMajorGlyph_GlyphOfShadow))

	config := priest.getMindSearBaseConfig()
	config.ActionID = core.ActionID{SpellID: 53022}.WithTag(numTicks)
	config.Direct = core.SpellEffect{Effect: 0, Min: 212, Max: 228, SP: 0.2857}
	config.Mods = []core.SpellMod{
		{Op: core.SpellModBonusMultiplier, Pct: 5 * int32(priest.Talents.Misery)},
	}
	config.ApplyEffects = func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
		damage := spell.Direct.Roll(sim) + spell.Direct.SP*spell.SpellPower()
		result := spell.CalcAndDealDamage(sim, target, damage, spell.OutcomeMagicHitAndCrit)

		if result.Landed() {
			priest.AddShadowWeavingStack(sim)
		}
		if result.DidCrit() && hasGlyphOfShadow {
			priest.ShadowyInsightAura.Activate(sim)
		}
	}
	return priest.GetOrRegisterSpell(config)
}

func (priest *Priest) newMindSearSpell(numTicksIdx int32) *core.Spell {
	numTicks := numTicksIdx
	flags := core.SpellFlagChanneled | core.SpellFlagNoMetrics
	if numTicksIdx == 0 {
		numTicks = 5
		flags |= core.SpellFlagAPL
	}

	mindSearTickSpell := priest.getMindSearTickSpell(numTicksIdx)

	config := priest.getMindSearBaseConfig()
	config.ActionID = core.ActionID{SpellID: 53023}.WithTag(numTicksIdx)
	config.Flags = flags
	config.ManaCost = core.ManaCostOptions{
		BaseCost:   0.28,
		Multiplier: 1 - 0.05*float64(priest.Talents.FocusedMind),
	}
	config.Cast = core.CastConfig{
		DefaultCast: core.Cast{
			GCD: core.GCDDefault,
		},
	}
	config.Dot = core.DotConfig{
		Aura: core.Aura{
			Label: "MindSear-" + strconv.Itoa(int(numTicksIdx)),
		},
		NumberOfTicks:       numTicks,
		TickLength:          time.Second,
		TicksCanCrit:        false,
		AffectedByCastSpeed: true,
		OnTick: func(sim *core.Simulation, target *core.Unit, dot *core.Dot) {
			// Ticks every target in range, the channeled one included: Spell::SelectImplicitAreaTargets
			// has no special case for the caster's own current target.
			for _, aoeTarget := range sim.Encounter.TargetUnits {
				mindSearTickSpell.Cast(sim, aoeTarget)
				mindSearTickSpell.SpellMetrics[aoeTarget.UnitIndex].Casts -= 1
			}
		},
	}
	config.ApplyEffects = func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
		result := spell.CalcAndDealOutcome(sim, target, spell.OutcomeMagicHit)
		if result.Landed() {
			spell.Dot(target).Apply(sim)
			mindSearTickSpell.SpellMetrics[target.UnitIndex].Casts += 1
		}
	}
	config.ExpectedTickDamage = func(sim *core.Simulation, target *core.Unit, spell *core.Spell, _ bool) *core.SpellResult {
		baseDamage := mindSearTickSpell.Direct.Roll(sim) + mindSearTickSpell.Direct.SP*spell.SpellPower()
		return spell.CalcPeriodicDamage(sim, target, baseDamage, spell.OutcomeExpectedMagicCrit)
	}
	return priest.GetOrRegisterSpell(config)
}
