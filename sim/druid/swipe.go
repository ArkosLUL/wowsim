package druid

import (
	"time"

	"github.com/wowsims/wotlk/sim/core"
)

func (druid *Druid) registerSwipeBearSpell() {
	idolBonus := int32(0)
	if druid.Ranged().ID == IdolOfBrutality {
		idolBonus = 10
	} else if druid.Ranged().ID == IdolOfPerspicaciousAttacks {
		idolBonus = 24
	}

	lbdm := core.TernaryFloat64(druid.HasSetBonus(ItemSetLasherweaveBattlegear, 2), 1.2, 1.0)
	thdm := core.TernaryFloat64(druid.HasSetBonus(ItemSetThunderheartHarness, 4), 1.15, 1.0)
	fidm := 1.0 + 0.1*float64(druid.Talents.FeralInstinct)

	druid.SwipeBear = druid.RegisterSpell(Bear, core.SpellConfig{
		ActionID:    core.ActionID{SpellID: 48562},
		SpellSchool: core.SpellSchoolPhysical,
		ProcMask:    core.ProcMaskMeleeMHSpecial,
		Flags:       core.SpellFlagMeleeMetrics | core.SpellFlagIncludeTargetBonusDamage | core.SpellFlagAPL,

		RageCost: core.RageCostOptions{
			Cost: 20 - float64(druid.Talents.Ferocity),
		},
		Cast: core.CastConfig{
			DefaultCast: core.Cast{
				GCD: core.GCDDefault,
			},
			IgnoreHaste: true,
		},

		DamageMultiplier: lbdm * thdm * fidm,
		CritMultiplier:   druid.MeleeCritMultiplier(Bear),
		ThreatMultiplier: 1,

		Direct: core.SpellEffect{Effect: 0, Min: 108, Max: 108, AP: 0.063},
		Mods: []core.SpellMod{
			{Op: core.SpellModEffect1, Flat: idolBonus},
		},

		ApplyEffects: func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
			baseDamage := spell.Direct.Roll(sim) + spell.Direct.AP*spell.MeleeAttackPower()
			baseDamage *= sim.Encounter.AOECapMultiplier()
			for _, aoeTarget := range sim.Encounter.TargetUnits {
				spell.CalcAndDealDamage(sim, aoeTarget, baseDamage, spell.OutcomeMeleeSpecialHitAndCrit)
			}
		},
	})
}

func (druid *Druid) registerSwipeCatSpell() {
	fidm := 1.0 + 0.1*float64(druid.Talents.FeralInstinct)

	druid.SwipeCat = druid.RegisterSpell(Cat, core.SpellConfig{
		ActionID:    core.ActionID{SpellID: 62078},
		SpellSchool: core.SpellSchoolPhysical,
		ProcMask:    core.ProcMaskMeleeMHSpecial,
		Flags:       core.SpellFlagMeleeMetrics | core.SpellFlagIncludeTargetBonusDamage | core.SpellFlagAPL,

		EnergyCost: core.EnergyCostOptions{
			Cost: 50 - float64(druid.Talents.Ferocity),
		},
		Cast: core.CastConfig{
			DefaultCast: core.Cast{
				GCD: time.Second,
			},
			IgnoreHaste: true,
		},

		DamageMultiplier: fidm,
		CritMultiplier:   druid.MeleeCritMultiplier(Cat),
		ThreatMultiplier: 1,

		Direct: core.SpellEffect{Effect: 0, WeaponPct: 2.5},

		ApplyEffects: func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
			baseDamage := spell.Unit.MHNormalizedWeaponDamage(sim, spell.MeleeAttackPower()) * spell.Direct.WeaponPct
			baseDamage *= sim.Encounter.AOECapMultiplier()
			for _, aoeTarget := range sim.Encounter.TargetUnits {
				spell.CalcAndDealDamage(sim, aoeTarget, baseDamage, spell.OutcomeMeleeSpecialHitAndCrit)
			}
		},
	})
}

func (druid *Druid) CurrentSwipeCatCost() float64 {
	return druid.SwipeCat.ApplyCostModifiers(druid.SwipeCat.DefaultCast.Cost)
}

func (druid *Druid) IsSwipeSpell(spell *core.Spell) bool {
	return druid.SwipeBear.IsEqual(spell) || druid.SwipeCat.IsEqual(spell)
}
