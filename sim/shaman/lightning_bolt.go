package shaman

import (
	"time"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
)

func (shaman *Shaman) registerLightningBoltSpell() {
	shaman.LightningBolt = shaman.RegisterSpell(shaman.newLightningBoltSpellConfig(false))
	shaman.LightningBoltLO = shaman.RegisterSpell(shaman.newLightningBoltSpellConfig(true))
}

func (shaman *Shaman) newLightningBoltSpellConfig(isLightningOverload bool) core.SpellConfig {
	spellConfig := shaman.newElectricSpellConfig(
		core.ActionID{SpellID: 49238},
		0.1*core.TernaryFloat64(shaman.HasSetBonus(ItemSetEarthShatterGarb, 2), 0.95, 1),
		time.Millisecond*2500,
		isLightningOverload)
	spellConfig.MissileSpeed = 20
	spellConfig.Direct = core.SpellEffect{Effect: 0, Min: 719, Max: 819, SP: 0.714}
	spellConfig.Mods = []core.SpellMod{{Op: core.SpellModBonusMultiplier, Flat: 4 * shaman.Talents.Shamanism}}

	if shaman.HasMajorGlyph(proto.ShamanMajorGlyph_GlyphOfLightningBolt) {
		spellConfig.DamageMultiplier += 0.04
	}

	if shaman.HasSetBonus(ItemSetSkyshatterRegalia, 4) {
		spellConfig.DamageMultiplier += 0.05
	}

	var lbDotSpell *core.Spell
	var electrifiedDelay *core.DelayedPeriodicApplier
	if !isLightningOverload && shaman.HasSetBonus(ItemSetWorldbreakerGarb, 4) {
		electrifiedDelay = core.NewDelayedPeriodicApplier(&shaman.Unit)
		lbDotSpell = shaman.RegisterSpell(core.SpellConfig{
			ActionID:         core.ActionID{SpellID: 64930},
			SpellSchool:      core.SpellSchoolNature,
			ProcMask:         core.ProcMaskEmpty,
			Flags:            core.SpellFlagNoOnCastComplete | core.SpellFlagIgnoreModifiers,
			DamageMultiplier: 1,
			ThreatMultiplier: 1,

			Dot: core.DotConfig{
				Aura: core.Aura{
					Label: "Electrified",
				},
				TickLength:    time.Second * 2,
				NumberOfTicks: 2,
				// mod-spell-tweaks doesn't give Electrified aura 286.
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

	relicSpellPower := shaman.electricSpellRelicSpellPower()

	canLO := !isLightningOverload && shaman.Talents.LightningOverload > 0
	lightningOverloadChance := float64(shaman.Talents.LightningOverload) * 0.11
	spellConfig.ApplyEffects = func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
		baseDamage := spell.Direct.Roll(sim) + spell.Direct.SP*(spell.SpellPower()+relicSpellPower)
		result := spell.CalcDamage(sim, target, baseDamage, spell.OutcomeMagicHitAndCrit)

		// Electrified and Lightning Overload proc off the hit, so they wait for the bolt too
		spell.WaitTravelTime(sim, func(sim *core.Simulation) {
			if !isLightningOverload && lbDotSpell != nil && result.DidCrit() {
				lbDot := lbDotSpell.Dot(target)

				newDamage := result.Damage * 0.08
				outstandingDamage := core.TernaryFloat64(lbDot.IsActive(), lbDot.SnapshotBaseDamage*float64(lbDot.NumberOfTicks-lbDot.TickCount), 0)
				totalDamage := outstandingDamage + newDamage

				electrifiedDelay.Apply(sim, target, func(sim *core.Simulation) {
					lbDot.SnapshotBaseDamage = totalDamage / float64(lbDot.NumberOfTicks)
					lbDot.SnapshotAttackerMultiplier = 1
					lbDotSpell.Cast(sim, target)
				})
			}

			if canLO && result.Landed() && sim.RandomFloat("LB Lightning Overload") < lightningOverloadChance {
				shaman.LightningBoltLO.Cast(sim, target)
			}

			spell.DealDamage(sim, result)
		})
	}

	return spellConfig
}
