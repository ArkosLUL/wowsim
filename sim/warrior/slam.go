package warrior

import (
	"time"

	"github.com/wowsims/wotlk/sim/core"
)

// slamCastTime is Improved Slam's talent reduction on the base 1.5 s cast.
func (warrior *Warrior) slamCastTime() time.Duration {
	return time.Millisecond*1500 - time.Millisecond*500*time.Duration(warrior.Talents.ImprovedSlam)
}

// registerSlamSpell splits the cast (47475) from its damage (spell_warr_slam::HandleDummy triggers
// 50783 on a landed hit): 47475 is SPELL_ATTR0_NO_ACTIVE_DEFENSE, so it can only miss, and 50783 is
// SPELL_ATTR3_ALWAYS_HIT (the table roll already happened on the cast), so it only rolls crit and the
// separate yellow block chance.
func (warrior *Warrior) registerSlamSpell() {
	slamDamage := warrior.RegisterSpell(core.SpellConfig{
		ActionID:    core.ActionID{SpellID: 50783},
		SpellSchool: core.SpellSchoolPhysical,
		ProcMask:    core.ProcMaskMeleeMHSpecial,
		Flags:       core.SpellFlagMeleeMetrics | core.SpellFlagIncludeTargetBonusDamage | core.SpellFlagNoOnCastComplete,

		BonusCritRating:  core.TernaryFloat64(warrior.HasSetBonus(ItemSetWrynnsBattlegear, 4), 5, 0) * core.CritRatingPerCritChance,
		DamageMultiplier: 1 + 0.02*float64(warrior.Talents.UnendingFury) + core.TernaryFloat64(warrior.HasSetBonus(ItemSetDreadnaughtBattlegear, 2), 0.1, 0),
		CritMultiplier:   warrior.critMultiplier(mh),
		ThreatMultiplier: 1,
		FlatThreatBonus:  140,

		ApplyEffects: func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
			baseDamage := 250 +
				spell.Unit.MHWeaponDamage(sim, spell.MeleeAttackPower()) +
				spell.BonusWeaponDamage()

			spell.CalcAndDealDamage(sim, target, baseDamage, spell.OutcomeMeleeSpecialCritOnly)
		},
	})

	warrior.Slam = warrior.RegisterSpell(core.SpellConfig{
		ActionID:    core.ActionID{SpellID: 47475},
		SpellSchool: core.SpellSchoolPhysical,
		ProcMask:    core.ProcMaskEmpty,
		Flags:       core.SpellFlagAPL,

		RageCost: core.RageCostOptions{
			Cost:   15 - float64(warrior.Talents.FocusedRage),
			Refund: 0.8,
		},
		Cast: core.CastConfig{
			DefaultCast: core.Cast{
				GCD:      core.GCDDefault,
				CastTime: warrior.slamCastTime(),
			},
			IgnoreHaste: true,
		},

		ApplyEffects: func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
			result := spell.CalcAndDealOutcome(sim, target, spell.OutcomeMeleeSpecialHit)
			if result.Landed() {
				slamDamage.Cast(sim, target)
			} else {
				spell.IssueRefund(sim)
			}
		},
	})
}

func (warrior *Warrior) ShouldInstantSlam(sim *core.Simulation) bool {
	return warrior.CurrentRage() >= warrior.Slam.DefaultCast.Cost && warrior.Slam.IsReady(sim) && warrior.isBloodsurgeActive() &&
		sim.CurrentTime > (warrior.lastBloodsurgeProc+warrior.reactionTime) && warrior.GCD.IsReady(sim)
}

func (warrior *Warrior) ShouldSlam(sim *core.Simulation) bool {
	return warrior.CurrentRage() >= warrior.Slam.DefaultCast.Cost && warrior.Slam.IsReady(sim) && warrior.Talents.ImprovedSlam > 0
}
