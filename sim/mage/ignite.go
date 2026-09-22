package mage

import (
	"math"
	"time"

	"github.com/wowsims/wotlk/sim/core"
)

// If two spells proc Ignite at almost exactly the same time, the latter
// overwrites the former.
const IgniteTicks = 2

func (mage *Mage) applyIgnite() {
	if mage.Talents.Ignite == 0 {
		return
	}

	igniteDelay := core.NewDelayedPeriodicApplier(&mage.Unit)

	mage.RegisterAura(core.Aura{
		Label:    "Ignite Talent",
		Duration: core.NeverExpires,
		OnReset: func(aura *core.Aura, sim *core.Simulation) {
			aura.Activate(sim)
		},
		OnSpellHitDealt: func(aura *core.Aura, sim *core.Simulation, spell *core.Spell, result *core.SpellResult) {
			if !spell.ProcMask.Matches(core.ProcMaskSpellDamage) {
				return
			}
			if spell.SpellSchool.Matches(core.SpellSchoolFire) && result.DidCrit() {
				// An instant cast (Living Bomb's direct hit, or Pyroblast/Frostfire Bolt made
				// instant by Hot Streak/Brain Freeze) is processed with the session, ahead of this
				// update's own clock advance; a hardcast landing in _UpdateSpells is processed
				// after it.
				mage.procIgnite(sim, result, igniteDelay, result.FromInstantCast())
			}
		},
		OnPeriodicDamageDealt: func(aura *core.Aura, sim *core.Simulation, spell *core.Spell, result *core.SpellResult) {
			if !spell.ProcMask.Matches(core.ProcMaskSpellDamage) {
				return
			}
			if mage.LivingBomb != nil && result.DidCrit() {
				// A dot tick is processed on the target's own update, not the caster's.
				mage.procIgnite(sim, result, igniteDelay, false)
			}
		},
	})

	mage.Ignite = mage.RegisterSpell(core.SpellConfig{
		ActionID:    core.ActionID{SpellID: 12654},
		SpellSchool: core.SpellSchoolFire,
		ProcMask:    core.ProcMaskProc,
		Flags:       SpellFlagMage | core.SpellFlagIgnoreModifiers,

		DamageMultiplier: 1,
		ThreatMultiplier: 1 - 0.1*float64(mage.Talents.BurningSoul),

		Dot: core.DotConfig{
			Aura: core.Aura{
				Label: "Ignite",
			},
			NumberOfTicks: IgniteTicks,
			TickLength:    time.Second * 2,
			// No SPELL_AURA_ABILITY_PERIODIC_CRIT aura covers Ignite, so its ticks never crit.
			TicksCanCrit: false,
			OnTick: func(sim *core.Simulation, target *core.Unit, dot *core.Dot) {
				dot.CalcAndDealPeriodicSnapshotDamage(sim, target, dot.OutcomeTick)
			},
		},

		ApplyEffects: func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
			spell.SpellMetrics[target.UnitIndex].Hits++
			spell.Dot(target).ApplyOrReset(sim)
		},
	})
}

// Mirrors spell_mage_ignite::HandleProc and Unit::CastDelayedSpellWithPeriodicAmount: both round
// down to whole damage, once for the new crit's share, again for the blend with the dot's leftover.
func (mage *Mage) procIgnite(sim *core.Simulation, result *core.SpellResult, delay *core.DelayedPeriodicApplier, instantCast bool) {
	target := result.Target
	dot := mage.Ignite.Dot(target)

	pctPerRank := float64(8 * mage.Talents.Ignite)
	newPerTick := math.Trunc(math.Trunc(result.Damage*pctPerRank/100) / IgniteTicks)

	// Read the dot's outstanding tick value before the delay: the server reads it at the proc.
	oldPerTick := core.TernaryFloat64(dot.IsActive(), dot.SnapshotBaseDamage, 0)
	remainingTicks := math.Max(0, core.TernaryFloat64(dot.IsActive(), float64(dot.NumberOfTicks-dot.TickCount), 0))
	addAmount := newPerTick + math.Trunc(oldPerTick*remainingTicks/IgniteTicks)

	onApply := func(sim *core.Simulation) {
		dot.SnapshotAttackerMultiplier = 1
		dot.SnapshotBaseDamage = addAmount
		mage.Ignite.Cast(sim, target)
	}
	if instantCast {
		delay.ApplyFromInstantCast(sim, target, onApply)
	} else {
		delay.Apply(sim, target, onApply)
	}
}

func (mage *Mage) applyEmpoweredFire() {
	if mage.Talents.EmpoweredFire == 0 {
		return
	}

	procChance := []float64{0, .33, .67, 1}[mage.Talents.EmpoweredFire]
	manaMetrics := mage.NewManaMetrics(core.ActionID{SpellID: 67545})

	mage.RegisterAura(core.Aura{
		Label:    "Empowered Fire",
		Duration: core.NeverExpires,
		OnReset: func(aura *core.Aura, sim *core.Simulation) {
			aura.Activate(sim)
		},
		OnPeriodicDamageDealt: func(aura *core.Aura, sim *core.Simulation, spell *core.Spell, result *core.SpellResult) {
			if spell == mage.Ignite && (procChance == 1 || sim.Proc(procChance, "Empowered Fire")) {
				mage.AddMana(sim, mage.Unit.BaseMana*0.02, manaMetrics)
			}
		},
	})
}
