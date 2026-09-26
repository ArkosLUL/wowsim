package deathknight

import (
	"time"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
)

func (dk *Deathknight) registerDeathAndDecaySpell() {
	hasGlyph := dk.HasMajorGlyph(proto.DeathknightMajorGlyph_GlyphOfDeathAndDecay)

	dk.DeathAndDecay = dk.RegisterSpell(core.SpellConfig{
		ActionID:    core.ActionID{SpellID: 49938},
		Flags:       core.SpellFlagAPL,
		SpellSchool: core.SpellSchoolShadow,
		ProcMask:    core.ProcMaskEmpty, // D&D doesn't seem to proc things in game.

		RuneCost: core.RuneCostOptions{
			BloodRuneCost:  1,
			FrostRuneCost:  1,
			UnholyRuneCost: 1,
			RunicPowerGain: 15,
		},
		Cast: core.CastConfig{
			DefaultCast: core.Cast{
				GCD: core.GCDDefault,
			},
			CD: core.Cooldown{
				Timer:    dk.NewTimer(),
				Duration: time.Second*30 - time.Second*5*time.Duration(dk.Talents.Morbidity),
			},
		},

		// spell_dk_death_and_decay adds the glyph's 20% to each tick's damage, on top of its spell mod
		DamageMultiplier: core.TernaryFloat64(hasGlyph, 1.2, 1),
		ThreatMultiplier: 1.9,
		CritMultiplier:   dk.DefaultMeleeCritMultiplier(),

		// each tick casts 52212 with the aura's amount as its base points (spell_dk_death_and_decay_aura),
		// adding 52212's AP
		Direct: core.SpellEffect{Effect: 0, FromSpellID: 52212, AP: 0.04805},

		Mods: []core.SpellMod{
			{Op: core.SpellModEffect1, Pct: core.TernaryInt32(hasGlyph, 20, 0)},
			{Op: core.SpellModAllEffects, Pct: dk.scourgelordsPlateDeathAndDecayPct()},
		},

		Dot: core.DotConfig{
			IsAOE: true,
			Aura: core.Aura{
				Label: "Death and Decay",
			},
			NumberOfTicks: 10,
			TickLength:    time.Second * 1,
			// each tick is 52212, a spell of its own that spell_dk_death_and_decay casts, so it crits
			// like one
			TicksCanCrit: true,
			Tick:         core.SpellEffect{Effect: 0, Min: 62, Max: 62},
			OnSnapshot: func(sim *core.Simulation, _ *core.Unit, dot *core.Dot, _ bool) {
				dot.SnapshotBaseDamage = dot.Tick.Roll(sim) + dot.Spell.Direct.AP*dk.getImpurityBonus(dot.Spell)
			},
			OnTick: func(sim *core.Simulation, target *core.Unit, dot *core.Dot) {
				for _, aoeTarget := range sim.Encounter.TargetUnits {
					// DnD recalculates attack multipliers + crit dynamically on every tick so this is here on purpose
					dot.SnapshotAttackerMultiplier = dot.Spell.AttackerDamageMultiplier(dot.Spell.Unit.AttackTables[aoeTarget.UnitIndex]) * dk.RoRTSBonus(aoeTarget)
					dot.SnapshotCritChance = dot.Spell.SpellCritChance(aoeTarget)
					dot.CalcAndDealPeriodicSnapshotDamage(sim, aoeTarget, dot.OutcomeMagicHitAndSnapshotCrit)
				}
			},
		},

		ApplyEffects: func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
			dot := spell.AOEDot()
			dot.Apply(sim)
			dot.TickOnce(sim)
		},
	})
}
