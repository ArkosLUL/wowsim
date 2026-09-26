package shaman

import (
	"time"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
)

func (shaman *Shaman) registerLavaBurstSpell() {
	actionID := core.ActionID{SpellID: 60043}
	// a SPELLMOD_DAMAGE flat, not an effect mod like Thunderfall Totem's
	dmgBonus := core.TernaryFloat64(shaman.Ranged().ID == VentureCoLightningRod, 121, 0)

	var lvbDotSpell *core.Spell
	var lvbBonusDotDelay *core.DelayedPeriodicApplier
	if shaman.HasSetBonus(ItemSetThrallsRegalia, 4) {
		lvbBonusDotDelay = core.NewDelayedPeriodicApplier(&shaman.Unit)
		lvbDotSpell = shaman.RegisterSpell(core.SpellConfig{
			ActionID:    core.ActionID{SpellID: 71824},
			SpellSchool: core.SpellSchoolFire,
			ProcMask:    core.ProcMaskEmpty,
			Flags:       core.SpellFlagNoOnCastComplete | core.SpellFlagIgnoreModifiers,

			DamageMultiplier: 1,
			ThreatMultiplier: 1,

			Dot: core.DotConfig{
				Aura: core.Aura{
					Label: "LavaBursted",
				},
				TickLength:    time.Second * 2,
				NumberOfTicks: 3,
				// mod-spell-tweaks doesn't give this bonus dot aura 286.
				TicksCanCrit: false,

				OnTick: func(sim *core.Simulation, target *core.Unit, dot *core.Dot) {
					dot.CalcAndDealPeriodicSnapshotDamage(sim, target, dot.OutcomeTick)
				},
			},

			ApplyEffects: func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
				spell.CalcAndDealOutcome(sim, target, spell.OutcomeAlwaysHit)
				spell.Dot(target).ApplyOrReset(sim)
			},
		})
	}

	shaman.LavaBurst = shaman.RegisterSpell(core.SpellConfig{
		ActionID:    actionID,
		SpellSchool: core.SpellSchoolFire,
		ProcMask:    core.ProcMaskSpellDamage,
		Flags:       SpellFlagFocusable | core.SpellFlagAPL,

		ManaCost: core.ManaCostOptions{
			BaseCost:   0.1,
			Multiplier: 1 - 0.02*float64(shaman.Talents.Convection),
		},
		Cast: core.CastConfig{
			DefaultCast: core.Cast{
				CastTime: time.Second*2 - time.Millisecond*100*time.Duration(shaman.Talents.LightningMastery),
				GCD:      core.GCDDefault,
			},
			CD: core.Cooldown{
				Timer:    shaman.NewTimer(),
				Duration: time.Second * 8,
			},
		},

		BonusHitRating:   float64(shaman.Talents.ElementalPrecision) * core.SpellHitRatingPerHitChance,
		DamageMultiplier: 1 + 0.01*float64(shaman.Talents.Concussion) + 0.02*float64(shaman.Talents.CallOfFlame),
		CritMultiplier:   shaman.ElementalCritMultiplier([]float64{0, 0.06, 0.12, 0.24}[shaman.Talents.LavaFlows] + core.TernaryFloat64(shaman.HasSetBonus(ItemSetEarthShatterGarb, 4), 0.1, 0)),
		ThreatMultiplier: shaman.spellThreatMultiplier(),

		MissileSpeed: 24,
		Direct:       core.SpellEffect{Effect: 0, Min: 1192, Max: 1518, SP: 0.571},
		Mods: []core.SpellMod{
			{Op: core.SpellModBonusMultiplier, Flat: 5 * shaman.Talents.Shamanism},
			{Op: core.SpellModBonusMultiplier, Flat: core.TernaryInt32(shaman.HasMajorGlyph(proto.ShamanMajorGlyph_GlyphOfLava), 10, 0)},
			{Op: core.SpellModEffect1, Flat: core.TernaryInt32(shaman.Ranged().ID == ThunderfallTotem, 215, 0)},
		},

		ApplyEffects: func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
			baseDamage := dmgBonus + spell.Direct.Roll(sim) + spell.Direct.SP*spell.SpellPower()
			result := spell.CalcDamage(sim, target, baseDamage, spell.OutcomeMagicHitAndCrit)
			spell.WaitTravelTime(sim, func(sim *core.Simulation) {
				if lvbDotSpell != nil && result.Landed() {
					dot := lvbDotSpell.Dot(target)

					newDamage := result.Damage * 0.1
					outstandingDamage := core.TernaryFloat64(dot.IsActive(), dot.SnapshotBaseDamage*float64(dot.NumberOfTicks-dot.TickCount), 0)
					totalDamage := outstandingDamage + newDamage

					lvbBonusDotDelay.Apply(sim, target, func(sim *core.Simulation) {
						dot.SnapshotBaseDamage = totalDamage / float64(dot.NumberOfTicks)
						dot.SnapshotAttackerMultiplier = 1
						dot.Spell.Cast(sim, target)
					})
				}
				spell.DealDamage(sim, result)
			})
		},
	})
}
