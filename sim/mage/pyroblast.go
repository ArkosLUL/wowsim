package mage

import (
	"time"

	"github.com/wowsims/wotlk/sim/core"
)

func (mage *Mage) registerPyroblastSpell() {
	if !mage.Talents.Pyroblast {
		return
	}

	actionID := core.ActionID{SpellID: 42891}
	var pyroblastDot *core.Spell

	// mod-spell-tweaks puts a periodic crit effect on Empowered Fire in spell_dbc, so no switch turns it off
	ticksCanCrit := mage.Talents.EmpoweredFire > 0
	onTick := func(sim *core.Simulation, target *core.Unit, dot *core.Dot) {
		dot.CalcAndDealPeriodicSnapshotDamage(sim, target, dot.OutcomeTick)
	}
	if ticksCanCrit {
		onTick = func(sim *core.Simulation, target *core.Unit, dot *core.Dot) {
			// Unit::_UpdateSpells walks the target's auras in spell id order, so an Ignite tick due on this
			// update lands first and a crit here rolls over what's left after it
			if mage.Ignite != nil {
				if ignite := mage.Ignite.Dot(target); ignite.IsActive() && ignite.NextTickAt() == sim.CurrentTime && ignite.MaxTicksRemaining() > 0 {
					ignite.ManualTick(sim)
				}
			}
			dot.CalcAndDealPeriodicSnapshotDamage(sim, target, dot.OutcomeSnapshotCrit)
		}
	}

	pyroblastConfig := core.SpellConfig{
		ActionID:     actionID,
		SpellSchool:  core.SpellSchoolFire,
		ProcMask:     core.ProcMaskSpellDamage,
		Flags:        SpellFlagMage | core.SpellFlagAPL,
		MissileSpeed: 24,

		ManaCost: core.ManaCostOptions{
			BaseCost: 0.22,
		},
		Cast: core.CastConfig{
			DefaultCast: core.Cast{
				GCD:      core.GCDDefault,
				CastTime: time.Second * 5,
			},
			ModifyCast: func(sim *core.Simulation, spell *core.Spell, cast *core.Cast) {
				if mage.HotStreakAura.IsActive() {
					cast.CastTime = 0
					if !mage.t84pcKeepsProc(sim) {
						mage.HotStreakAura.Deactivate(sim)
						mage.pushingTheLimit(sim)
					}
				}
			},
		},

		BonusCritRating: 0 +
			2*float64(mage.Talents.CriticalMass)*core.CritRatingPerCritChance +
			2*float64(mage.Talents.WorldInFlames)*core.CritRatingPerCritChance,
		DamageMultiplierAdditive: 1 +
			.02*float64(mage.Talents.FirePower),
		CritMultiplier:   mage.SpellCritMultiplier(1, mage.bonusCritDamage),
		ThreatMultiplier: 1,

		Direct: core.SpellEffect{Effect: 0, Min: 1210, Max: 1530, SP: 1.15},
		Mods: []core.SpellMod{
			{Op: core.SpellModBonusMultiplier, Flat: 5 * mage.Talents.EmpoweredFire},
		},

		ApplyEffects: func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
			baseDamage := (spell.Direct.Roll(sim) + core.PowerBonus(spell.Direct.SP*spell.SpellPower())) * mage.tormentTheWeak(target)
			result := spell.CalcDamage(sim, target, baseDamage, spell.OutcomeMagicHitAndCrit)
			spell.WaitTravelTime(sim, func(sim *core.Simulation) {
				if result.Landed() {
					pyroblastDot.SpellMetrics[target.UnitIndex].Casts++
					spell.Dot(target).Apply(sim)
				}
				spell.DealDamage(sim, result)
			})
		},
	}

	// the ticks' damage goes under its own tag, with the direct hit's crit and damage talents. It's never
	// cast and has no dot of its own, so a rotation can't name it
	dotSpellConfig := pyroblastConfig
	dotSpellConfig.ActionID = actionID.WithTag(1)
	dotSpellConfig.Flags = SpellFlagMage
	pyroblastDot = mage.RegisterSpell(dotSpellConfig)

	pyroblastConfig.Dot = core.DotConfig{
		Spell: pyroblastDot,
		Aura: core.Aura{
			Label:    "Pyroblast",
			ActionID: actionID,
		},
		NumberOfTicks:       4,
		TickLength:          time.Second * 3,
		AffectedByCastSpeed: mage.pyroblastDotAddsTicks(),
		TickHaste:           core.SpellHasteAddsTicks,
		TicksCanCrit:        ticksCanCrit,

		Tick: core.SpellEffect{Effect: 1, Min: 113, Max: 113, SP: 0.05},

		OnSnapshot: func(sim *core.Simulation, target *core.Unit, dot *core.Dot, _ bool) {
			dot.SnapshotBaseDamage = dot.Tick.Roll(sim) + core.PowerBonus(dot.Tick.SP*dot.Spell.SpellPower())
			dot.SnapshotCritChance = dot.Spell.SpellCritChance(target)
			dot.SnapshotAttackerMultiplier = dot.Spell.AttackerDamageMultiplier(dot.Spell.Unit.AttackTables[target.UnitIndex]) *
				mage.tormentTheWeak(target)
		},
		OnTick: onTick,
	}
	mage.Pyroblast = mage.RegisterSpell(pyroblastConfig)
}

// pyroblastDotAddsTicks is spell_tweaks_pyroblast_dot_haste: with Empowered Fire, the cast speed the
// Pyroblast lands with shortens its DoT's tick interval and the 12 s stay put.
func (mage *Mage) pyroblastDotAddsTicks() bool {
	return mage.Talents.EmpoweredFire > 0 && mage.Server().SpellTweaks.PyroblastEmpoweredFire
}
