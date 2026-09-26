package deathknight

import (
	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
)

// TODO: Cleanup death strike the same way we did for plague strike
var DeathStrikeActionID = core.ActionID{SpellID: 49924}

func (dk *Deathknight) deathStrikeEffect() (core.SpellEffect, []core.SpellMod) {
	return core.SpellEffect{Effect: 0, Min: 297, Max: 297, WeaponPct: 0.75},
		[]core.SpellMod{{Op: core.SpellModEffect1, Flat: dk.sigilOfAwarenessBonus()}}
}

func (dk *Deathknight) newDeathStrikeSpell(isMH bool) *core.Spell {
	hasGlyph := dk.HasMajorGlyph(proto.DeathknightMajorGlyph_GlyphOfDeathStrike)
	deathConvertChance := float64(dk.Talents.DeathRuneMastery) / 3

	var healthMetrics *core.ResourceMetrics
	if isMH {
		healthMetrics = dk.NewHealthMetrics(DeathStrikeActionID)
	}

	conf := core.SpellConfig{
		ActionID:    DeathStrikeActionID.WithTag(core.TernaryInt32(isMH, 1, 2)),
		SpellSchool: core.SpellSchoolPhysical,
		ProcMask:    dk.threatOfThassarianProcMask(isMH),
		Flags:       core.SpellFlagMeleeMetrics | core.SpellFlagIncludeTargetBonusDamage,

		RuneCost: core.RuneCostOptions{
			FrostRuneCost:  1,
			UnholyRuneCost: 1,
			RunicPowerGain: 15 + 2.5*float64(dk.Talents.Dirge),
			Refundable:     true,
		},
		Cast: core.CastConfig{
			DefaultCast: core.Cast{
				GCD: core.GCDDefault,
			},
			IgnoreHaste: true,
		},

		BonusCritRating: (dk.annihilationCritBonus() + dk.improvedDeathStrikeCritBonus()) * core.CritRatingPerCritChance,
		DamageMultiplier: core.TernaryFloat64(isMH, 1, dk.nervesOfColdSteelBonus()) *
			dk.improvedDeathStrikeDamageBonus(),
		CritMultiplier:   dk.bonusCritMultiplier(dk.Talents.MightOfMograine),
		ThreatMultiplier: 1,

		ApplyEffects: func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
			baseDamage := normalizedStrikeBase(sim, spell, isMH, spell.Direct.Roll(sim)) * spell.Direct.WeaponPct
			baseDamage *= dk.RoRTSBonus(target)
			if hasGlyph {
				baseDamage *= 1 + 0.01*min(dk.CurrentRunicPower(), 25)
			}

			result := spell.CalcDamage(sim, target, baseDamage, dk.threatOfThassarianOutcomeApplier(spell))

			if isMH {
				spell.SpendRefundableCostAndConvertFrostOrUnholyRune(sim, result, deathConvertChance)

				if result.Landed() {
					healingAmount := 0.05 * dk.dkCountActiveDiseases(target) * dk.MaxHealth() * (1.0 + 0.5*float64(dk.Talents.ImprovedDeathStrike))
					dk.GainHealth(sim, healingAmount*dk.PseudoStats.HealingTakenMultiplier, healthMetrics)
					dk.DeathStrikeHeals = append(dk.DeathStrikeHeals, healingAmount)
				}

				dk.threatOfThassarianProc(sim, result, dk.DeathStrikeOhHit)
			}

			spell.DealDamage(sim, result)
		},
	}

	conf.Direct, conf.Mods = dk.deathStrikeEffect()
	if !isMH {
		conf.RuneCost = core.RuneCostOptions{}
		conf.Cast = core.CastConfig{}
		// Threat of Thassarian's off-hand strike
		conf.Direct = core.SpellEffect{Effect: 0, FromSpellID: 66953, Min: 148, Max: 148, WeaponPct: 0.75}
	} else {
		conf.Flags |= core.SpellFlagAPL
	}

	return dk.RegisterSpell(conf)
}

func (dk *Deathknight) registerDeathStrikeSpell() {
	dk.DeathStrikeOhHit = dk.newDeathStrikeSpell(false)
	dk.DeathStrikeMhHit = dk.newDeathStrikeSpell(true)
	dk.DeathStrike = dk.DeathStrikeMhHit
}

func (dk *Deathknight) registerDrwDeathStrikeSpell() {
	hasGlyph := dk.HasMajorGlyph(proto.DeathknightMajorGlyph_GlyphOfDeathStrike)
	direct, mods := dk.deathStrikeEffect()

	dk.RuneWeapon.DeathStrike = dk.RuneWeapon.RegisterSpell(core.SpellConfig{
		ActionID:    DeathStrikeActionID.WithTag(1),
		SpellSchool: core.SpellSchoolPhysical,
		ProcMask:    core.ProcMaskMeleeMHSpecial,
		Flags:       core.SpellFlagMeleeMetrics | core.SpellFlagIncludeTargetBonusDamage,

		BonusCritRating:  (dk.annihilationCritBonus() + dk.improvedDeathStrikeCritBonus()) * core.CritRatingPerCritChance,
		DamageMultiplier: dk.improvedDeathStrikeDamageBonus(),
		CritMultiplier:   dk.bonusCritMultiplier(dk.Talents.MightOfMograine),
		ThreatMultiplier: 1,

		Direct: direct,
		Mods:   mods,

		ApplyEffects: func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
			baseDamage := (spell.Direct.Roll(sim) + dk.DrwWeaponDamage(sim, spell)) * spell.Direct.WeaponPct

			if hasGlyph {
				baseDamage *= 1 + 0.01*min(dk.CurrentRunicPower(), 25)
			}
			spell.CalcAndDealDamage(sim, target, baseDamage, spell.OutcomeMeleeSpecialHitAndCrit)
		},
	})
}
