package warrior

import (
	"time"

	"github.com/wowsims/wotlk/sim/core"
)

func (warrior *Warrior) applyDeepWounds() {
	if warrior.Talents.DeepWounds == 0 {
		return
	}

	warrior.DeepWounds = warrior.RegisterSpell(core.SpellConfig{
		ActionID:    core.ActionID{SpellID: 12867},
		SpellSchool: core.SpellSchoolPhysical,
		ProcMask:    core.ProcMaskEmpty,
		Flags:       core.SpellFlagNoOnCastComplete | core.SpellFlagIgnoreModifiers,

		DamageMultiplier: 1,
		ThreatMultiplier: 1,

		Dot: core.DotConfig{
			Aura: core.Aura{
				Label: "DeepWounds",
			},
			NumberOfTicks: 6,
			TickLength:    time.Second * 1,
			// No SPELL_AURA_ABILITY_PERIODIC_CRIT aura covers Deep Wounds, so its ticks never crit.
			TicksCanCrit: false,

			OnTick: func(sim *core.Simulation, target *core.Unit, dot *core.Dot) {
				dot.SnapshotAttackerMultiplier = target.PseudoStats.PeriodicPhysicalDamageTakenMultiplier
				dot.CalcAndDealPeriodicSnapshotDamage(sim, target, dot.OutcomeTick)
			},
		},

		ApplyEffects: func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
			spell.Dot(target).ApplyOrReset(sim)
			spell.CalcAndDealOutcome(sim, target, spell.OutcomeAlwaysHit)
		},
	})

	warrior.RegisterAura(core.Aura{
		Label:    "Deep Wounds Talent",
		Duration: core.NeverExpires,
		OnReset: func(aura *core.Aura, sim *core.Simulation) {
			aura.Activate(sim)
		},
		OnSpellHitDealt: func(aura *core.Aura, sim *core.Simulation, spell *core.Spell, result *core.SpellResult) {
			if spell.ProcMask.Matches(core.ProcMaskEmpty) || !spell.SpellSchool.Matches(core.SpellSchoolPhysical) {
				return
			}
			if result.Outcome.Matches(core.OutcomeCrit) {
				warrior.procDeepWounds(sim, result.Target, spell.IsOH())
			}
		},
	})
}

// deepWoundsMunchDelay is Unit::CastDelayedSpellWithPeriodicAmount (MunchingBlizzlike.Enabled, live
// on): every Deep Wounds refresh queues on the warrior's own event list until its next 400 ms boundary
// (EventProcessor::CalculateQueueTime), so up to 400 ms out, and the old dot keeps ticking at its old
// amount meanwhile. The sim has no phase for that clock and always takes the full 400 ms. Overlapping
// crits within the window each read the outstanding damage as it stood at their own proc.
const deepWoundsMunchDelay = time.Millisecond * 400

func (warrior *Warrior) procDeepWounds(sim *core.Simulation, target *core.Unit, isOh bool) {
	dot := warrior.DeepWounds.Dot(target)

	outstandingDamage := core.TernaryFloat64(dot.IsActive(), dot.SnapshotBaseDamage*float64(dot.NumberOfTicks-dot.TickCount), 0)

	attackTable := warrior.AttackTables[target.UnitIndex]
	var awd float64
	if isOh {
		adm := warrior.AutoAttacks.OHAuto().AttackerDamageMultiplier(attackTable)
		tdm := warrior.AutoAttacks.OHAuto().TargetDamageMultiplier(attackTable, false)
		awd = ((warrior.AutoAttacks.OH().CalculateAverageWeaponDamage(dot.Spell.MeleeAttackPower()) * 0.5) + dot.Spell.BonusWeaponDamage()) * adm * tdm
	} else { // MH, Ranged (e.g. Thunder Clap)
		adm := warrior.AutoAttacks.MHAuto().AttackerDamageMultiplier(attackTable)
		tdm := warrior.AutoAttacks.MHAuto().TargetDamageMultiplier(attackTable, false)
		awd = (warrior.AutoAttacks.MH().CalculateAverageWeaponDamage(dot.Spell.MeleeAttackPower()) + dot.Spell.BonusWeaponDamage()) * adm * tdm
	}
	newDamage := awd * 0.16 * float64(warrior.Talents.DeepWounds)
	totalDamage := outstandingDamage + newDamage

	sim.AddPendingAction(&core.PendingAction{
		NextActionAt: sim.NextServerTick(sim.CurrentTime + deepWoundsMunchDelay),
		OnAction: func(sim *core.Simulation) {
			dot.SnapshotBaseDamage = totalDamage / float64(dot.NumberOfTicks)
			dot.SnapshotAttackerMultiplier = 1
			warrior.DeepWounds.Cast(sim, target)
		},
	})
}
