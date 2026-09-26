package druid

import (
	"time"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
)

func (druid *Druid) registerMangleBearSpell() {
	if !druid.Talents.Mangle {
		return
	}

	mangleAuras := druid.NewEnemyAuraArray(core.MangleAura)
	durReduction := (0.5) * float64(druid.Talents.ImprovedMangle)
	glyphBonus := core.TernaryFloat64(druid.HasMajorGlyph(proto.DruidMajorGlyph_GlyphOfMangle), 1.1, 1.0)

	druid.MangleBear = druid.RegisterSpell(Bear, core.SpellConfig{
		ActionID:    core.ActionID{SpellID: 48564},
		SpellSchool: core.SpellSchoolPhysical,
		ProcMask:    core.ProcMaskMeleeMHSpecial,
		Flags:       core.SpellFlagMeleeMetrics | core.SpellFlagIncludeTargetBonusDamage | core.SpellFlagAPL,

		RageCost: core.RageCostOptions{
			Cost:   20 - float64(druid.Talents.Ferocity),
			Refund: 0.8,
		},
		Cast: core.CastConfig{
			DefaultCast: core.Cast{
				GCD: core.GCDDefault,
			},
			IgnoreHaste: true,
			CD: core.Cooldown{
				Timer:    druid.NewTimer(),
				Duration: time.Duration(float64(time.Second) * (6 - durReduction)),
			},
		},

		DamageMultiplier: glyphBonus,
		CritMultiplier:   druid.MeleeCritMultiplier(Bear),
		ThreatMultiplier: core.TernaryFloat64(druid.HasSetBonus(ItemSetThunderheartHarness, 2), 1.15, 1),

		Direct: core.SpellEffect{Effect: 0, Min: 260, Max: 260, WeaponPct: 1.15},
		Mods:   druid.mangleMods(),

		ApplyEffects: func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
			baseDamage := (spell.Direct.Roll(sim) +
				spell.Unit.MHWeaponDamage(sim, spell.MeleeAttackPower()) +
				spell.BonusWeaponDamage()) * spell.Direct.WeaponPct

			result := spell.CalcAndDealDamage(sim, target, baseDamage, spell.OutcomeMeleeSpecialHitAndCrit)

			if result.Landed() {
				mangleAuras.Get(target).Activate(sim)
			} else {
				spell.IssueRefund(sim)
			}

			if druid.BerserkAura.IsActive() {
				spell.CD.Reset()
			}
		},

		RelatedAuras: []core.AuraArray{mangleAuras},
	})
}

func (druid *Druid) registerMangleCatSpell() {
	if !druid.Talents.Mangle {
		return
	}

	mangleAuras := druid.NewEnemyAuraArray(core.MangleAura)
	glyphBonus := core.TernaryFloat64(druid.HasMajorGlyph(proto.DruidMajorGlyph_GlyphOfMangle), 1.1, 1.0)

	druid.MangleCat = druid.RegisterSpell(Cat, core.SpellConfig{
		ActionID:    core.ActionID{SpellID: 48566},
		SpellSchool: core.SpellSchoolPhysical,
		ProcMask:    core.ProcMaskMeleeMHSpecial,
		Flags:       core.SpellFlagMeleeMetrics | core.SpellFlagIncludeTargetBonusDamage | core.SpellFlagAPL,

		EnergyCost: core.EnergyCostOptions{
			Cost:   45.0 - 2*float64(druid.Talents.ImprovedMangle) - float64(druid.Talents.Ferocity) - core.TernaryFloat64(druid.HasSetBonus(ItemSetThunderheartHarness, 2), 5, 0),
			Refund: 0.8,
		},
		Cast: core.CastConfig{
			DefaultCast: core.Cast{
				GCD: time.Second,
			},
			IgnoreHaste: true,
		},

		DamageMultiplier: glyphBonus,
		CritMultiplier:   druid.MeleeCritMultiplier(Cat),
		ThreatMultiplier: 1,

		Direct: core.SpellEffect{Effect: 0, Min: 283, Max: 283, WeaponPct: 2},
		Mods:   druid.mangleMods(),

		ApplyEffects: func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
			baseDamage := (spell.Direct.Roll(sim) +
				spell.Unit.MHWeaponDamage(sim, spell.MeleeAttackPower()) +
				spell.BonusWeaponDamage()) * spell.Direct.WeaponPct

			result := spell.CalcAndDealDamage(sim, target, baseDamage, spell.OutcomeMeleeSpecialHitAndCrit)

			if result.Landed() {
				druid.AddComboPoints(sim, 1, spell.ComboPointMetrics())
				mangleAuras.Get(target).Activate(sim)
			} else {
				spell.IssueRefund(sim)
			}
		},

		RelatedAuras: []core.AuraArray{mangleAuras},
	})
}

// Savage Fury on both Mangles is a percent on the weapon percent effect (115% or 200%), so the flat
// part rides it too
func (druid *Druid) mangleMods() []core.SpellMod {
	return []core.SpellMod{
		{Op: core.SpellModEffect3, Pct: 10 * druid.Talents.SavageFury},
	}
}

func (druid *Druid) CurrentMangleCatCost() float64 {
	return druid.MangleCat.ApplyCostModifiers(druid.MangleCat.DefaultCast.Cost)
}

func (druid *Druid) IsMangle(spell *core.Spell) bool {
	if druid.MangleBear != nil && druid.MangleBear.IsEqual(spell) {
		return true
	} else if druid.MangleCat != nil && druid.MangleCat.IsEqual(spell) {
		return true
	}
	return false
}
