package rogue

import (
	"math"
	"time"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
)

const RuptureEnergyCost = 25.0
const RuptureSpellID = 48672

// Glyph of Backstab (56800): a landed Backstab adds 2 s to the rogue's Rupture on the target, 6 s at most.
const (
	glyphOfBackstabStep = 2 * time.Second
	glyphOfBackstabCap  = 6 * time.Second
)

// ruptureAddsTicks is mod-spell-tweaks' spell_tweaks_rupture_haste: with Weapon Expertise, the
// rogue's melee haste shortens Rupture's tick interval while its duration stays fixed, so it fits
// more ticks.
func (rogue *Rogue) ruptureAddsTicks() bool {
	return rogue.Talents.WeaponExpertise > 0 && rogue.Server().SpellTweaks.RuptureWeaponExpertise
}

func (rogue *Rogue) registerRupture() {
	glyphTicks := core.TernaryInt32(rogue.HasMajorGlyph(proto.RogueMajorGlyph_GlyphOfRupture), 2, 0)
	rogue.ruptureBackstabExtension = make([]time.Duration, len(rogue.Env.AllUnits))

	rogue.Rupture = rogue.RegisterSpell(core.SpellConfig{
		ActionID:     core.ActionID{SpellID: RuptureSpellID},
		SpellSchool:  core.SpellSchoolPhysical,
		ProcMask:     core.ProcMaskMeleeMHSpecial,
		Flags:        core.SpellFlagMeleeMetrics | rogue.finisherFlags() | core.SpellFlagAPL,
		MetricSplits: 6,

		EnergyCost: core.EnergyCostOptions{
			Cost:              RuptureEnergyCost,
			ProcRefund:        0.4 * float64(rogue.Talents.QuickRecovery),
			ProcRefundMetrics: rogue.QuickRecoveryMetrics,
		},
		Cast: core.CastConfig{
			DefaultCast: core.Cast{
				GCD: time.Second,
			},
			IgnoreHaste: true,
			ModifyCast: func(sim *core.Simulation, spell *core.Spell, cast *core.Cast) {
				spell.SetMetricsSplit(spell.Unit.ComboPoints())
			},
		},
		ExtraCastCondition: func(sim *core.Simulation, target *core.Unit) bool {
			return rogue.ComboPoints() > 0
		},

		DamageMultiplier: spellModDamage(
			0.15*float64(rogue.Talents.BloodSpatter),
			0.02*float64(rogue.Talents.FindWeakness),
			core.TernaryFloat64(rogue.HasSetBonus(Tier7, 2), 0.1, 0),
			core.TernaryFloat64(rogue.HasSetBonus(Tier8, 4), 0.2, 0),
			0.1*float64(rogue.Talents.SerratedBlades),
		),
		CritMultiplier:   rogue.MeleeCritMultiplier(false),
		ThreatMultiplier: 1,

		Dot: core.DotConfig{
			Aura: core.Aura{
				Label: "Rupture",
				Tag:   RogueBleedTag,
			},
			NumberOfTicks:       0, // Set dynamically
			TickLength:          time.Second * 2,
			AffectedByCastSpeed: rogue.ruptureAddsTicks(),
			TickHaste:           core.MeleeHasteAddsTicks,
			// ticks can crit with no talent at all (AuraEffect::CalcPeriodicCritChance's
			// SPELLFAMILY_ROGUE case, family flag 0x100000), unlike every other rogue dot
			TicksCanCrit: true,
			Tick:         core.SpellEffect{Effect: 0, Min: 127, Max: 127},

			OnSnapshot: func(sim *core.Simulation, target *core.Unit, dot *core.Dot, _ bool) {
				comboPoints := rogue.ComboPoints()
				attackTable := dot.Spell.Unit.AttackTables[target.UnitIndex]
				// spell_rog_rupture adds its attack power part after SpellDamageBonusDone, so none of the
				// caster's damage modifiers reach that part; Shadowstep's SPELLMOD_EFFECT1 (shadowstep.go)
				// scales the base points below, truncated by CalcValue, before that split
				tick := math.Floor((dot.Tick.Roll(sim) + 18*float64(comboPoints)) * core.TernaryFloat64(rogue.ShadowstepAura.IsActive(), 1.2, 1))
				dot.SnapshotBaseDamage = math.Floor(tick*dot.Spell.AttackerDamageMultiplier(attackTable)) + rogue.ruptureAPDamage(comboPoints)
				dot.SnapshotAttackerMultiplier = 1
				dot.SnapshotCritChance = dot.Spell.PhysicalCritChance(attackTable)
			},
			OnTick: func(sim *core.Simulation, target *core.Unit, dot *core.Dot) {
				dot.CalcAndDealPeriodicSnapshotDamage(sim, target, dot.OutcomeSnapshotCrit)
			},
		},

		ApplyEffects: func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
			rogue.BreakStealth(sim)
			result := spell.CalcOutcome(sim, target, spell.OutcomeMeleeSpecialHit)
			if result.Landed() {
				numberOfTicks := 3 + rogue.ComboPoints() + glyphTicks
				dot := spell.Dot(target)
				dot.Spell = spell
				dot.NumberOfTicks = numberOfTicks
				rogue.ruptureBackstabExtension[target.UnitIndex] = 0
				dot.Apply(sim)
				rogue.ApplyFinisher(sim, spell)
			} else {
				spell.IssueRefund(sim)
			}
			spell.DealOutcome(sim, result)
		},
	})
}

// extendRupture is Glyph of Backstab's proc on a landed Backstab.
func (rogue *Rogue) extendRupture(sim *core.Simulation, target *core.Unit) {
	dot := rogue.Rupture.Dot(target)
	if !dot.IsActive() {
		return
	}
	if rogue.Server().SpellTweaks.GlyphOfBackstabTicks {
		// mod-spell-tweaks raises the max duration too, so the added seconds tick, at the interval Rupture
		// went on with. A recast starts the 6 s over.
		extension := &rogue.ruptureBackstabExtension[target.UnitIndex]
		if *extension+glyphOfBackstabStep > glyphOfBackstabCap {
			return
		}
		*extension += glyphOfBackstabStep
		// NumberOfTicks counts 2 s intervals, the glyph's step
		dot.NumberOfTicks++
		dot.Duration += glyphOfBackstabStep
		dot.AddDuration(sim, glyphOfBackstabStep)
		return
	}
	if dot.ExpiresAt() >= dot.StartedAt()+dot.Duration+5*time.Second {
		// core's script lets Rupture end up to 5 s past its apply time plus max duration: three Backstabs
		return
	}
	// it only adds to the time left: the max duration stays, so the dot is out of ticks by then
	// (AuraEffect::GetTotalTicks) and the added time deals nothing
	dot.Aura.AddDuration(sim, glyphOfBackstabStep)
}

// ruptureAPDamage is spell_rog_rupture's share of attack power in a tick, in whole points.
func (rogue *Rogue) ruptureAPDamage(comboPoints int32) float64 {
	apPerTick := [...]float64{0, 0.015, 0.024, 0.03, 0.03428571, 0.0375}[comboPoints]
	return math.Floor(apPerTick * rogue.Rupture.MeleeAttackPower())
}

func (rogue *Rogue) RuptureTicks(comboPoints int32) int32 {
	return 3 + comboPoints + core.TernaryInt32(rogue.HasMajorGlyph(proto.RogueMajorGlyph_GlyphOfRupture), 2, 0)
}

func (rogue *Rogue) RuptureDuration(comboPoints int32) time.Duration {
	return time.Duration(rogue.RuptureTicks(comboPoints)) * time.Second * 2
}
