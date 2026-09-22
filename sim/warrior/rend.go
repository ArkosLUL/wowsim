package warrior

import (
	"time"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
)

// TODO (maybe) https://github.com/magey/wotlk-warrior/issues/23 - Rend is not benefitting from Two-Handed Weapon Specialization
func (warrior *Warrior) RegisterRendSpell() {
	dotDuration := time.Second * 15
	dotTicks := int32(5)
	if warrior.HasMajorGlyph(proto.WarriorMajorGlyph_GlyphOfRending) {
		dotDuration += time.Second * 6
		dotTicks += 2
	}

	// Trauma's crit is a static override on the talent itself (SpellTweaks_classes.cpp's
	// spell_tweaks_rend_haste comment), so it always applies once talented; only the haste add-ticks
	// half reads the RendTrauma config switch.
	canCrit := warrior.Talents.Trauma > 0
	addsTicks := canCrit && warrior.Server().SpellTweaks.RendTrauma

	warrior.Rend = warrior.RegisterSpell(core.SpellConfig{
		ActionID:    core.ActionID{SpellID: 47465},
		SpellSchool: core.SpellSchoolPhysical,
		ProcMask:    core.ProcMaskMeleeMHSpecial,
		Flags:       core.SpellFlagNoOnCastComplete | core.SpellFlagAPL,

		RageCost: core.RageCostOptions{
			Cost:   10 - float64(warrior.Talents.FocusedRage),
			Refund: 0.8,
		},
		Cast: core.CastConfig{
			DefaultCast: core.Cast{
				GCD: core.GCDDefault,
			},
			IgnoreHaste: true,
		},

		ExtraCastCondition: func(sim *core.Simulation, target *core.Unit) bool {
			return warrior.StanceMatches(BattleStance|DefensiveStance)
		},

		DamageMultiplier: 1 + 0.1*float64(warrior.Talents.ImprovedRend),
		CritMultiplier:   warrior.critMultiplier(mh),
		ThreatMultiplier: 1,

		Dot: core.DotConfig{
			Aura: core.Aura{
				Label: "Rend",
				Tag:   "Rend",
			},
			NumberOfTicks:       dotTicks,
			TickLength:          time.Second * 3,
			AffectedByCastSpeed: addsTicks,
			TickHaste:           core.MeleeHasteAddsTicks,
			TicksCanCrit:        canCrit,
			OnSnapshot: func(sim *core.Simulation, target *core.Unit, dot *core.Dot, _ bool) {
				dot.SnapshotBaseDamage = (380 + warrior.AutoAttacks.MH().CalculateAverageWeaponDamage(dot.Spell.MeleeAttackPower())) / 5
				// 135% damage multiplier is applied at the beginning of the fight and removed when target is at 75% health
				if sim.GetRemainingDurationPercent() > 0.75 {
					dot.SnapshotBaseDamage *= 1.35
				}
				dot.SnapshotAttackerMultiplier = dot.Spell.AttackerDamageMultiplier(dot.Spell.Unit.AttackTables[target.UnitIndex])
				if canCrit {
					dot.SnapshotCritChance = dot.Spell.PhysicalCritChance(dot.Spell.Unit.AttackTables[target.UnitIndex])
				}
			},
			OnTick: func(sim *core.Simulation, target *core.Unit, dot *core.Dot) {
				if canCrit {
					dot.CalcAndDealPeriodicSnapshotDamage(sim, target, dot.OutcomeSnapshotCrit)
				} else {
					dot.CalcAndDealPeriodicSnapshotDamage(sim, target, dot.OutcomeTick)
				}
			},
		},

		ApplyEffects: func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
			result := spell.CalcOutcome(sim, target, spell.OutcomeMeleeSpecialHit)
			if result.Landed() {
				spell.Dot(target).Apply(sim)
				warrior.RendValidUntil = sim.CurrentTime + dotDuration
			} else {
				spell.IssueRefund(sim)
			}

			spell.DealOutcome(sim, result)
		},
	})
}
