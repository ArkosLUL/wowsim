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
	deathConvertChance := dk.deathRuneMasteryChance()

	var healthMetrics *core.ResourceMetrics
	if isMH {
		healthMetrics = dk.HealThreat(dk.NewHealthMetrics(DeathStrikeActionID), 45470)
	}

	conf := core.SpellConfig{
		ActionID:    DeathStrikeActionID.WithTag(core.TernaryInt32(isMH, 1, 2)),
		SpellSchool: core.SpellSchoolPhysical,
		ProcMask:    dk.threatOfThassarianProcMask(isMH),
		Flags:       core.SpellFlagMeleeMetrics | core.SpellFlagIncludeTargetBonusDamage,

		RuneCost: core.RuneCostOptions{
			FrostRuneCost:  1,
			UnholyRuneCost: 1,
			RunicPowerGain: 15 + 2.5*float64(dk.Talents.Dirge) + dk.scourgeborneBattlegearRunicPowerBonus(),
			Refundable:     true,
		},
		Cast: core.CastConfig{
			DefaultCast: core.Cast{
				GCD: core.GCDDefault,
			},
			IgnoreHaste: true,
		},

		BonusCritRating:    (dk.annihilationCritBonus() + dk.viciousStrikesCritChanceBonus() + dk.improvedDeathStrikeCritBonus() + dk.scourgeborneBattlegearCritBonus()) * core.CritRatingPerCritChance,
		TotalPctMultiplier: core.TernaryFloat64(isMH, 1, dk.nervesOfColdSteelBonus()),
		DamageMultiplier: core.TernaryFloat64(isMH, 1, dk.nervesOfColdSteelBonus()) *
			dk.improvedDeathStrikeDamageBonus(),
		CritMultiplier:   dk.bonusCritMultiplier(dk.Talents.MightOfMograine),
		ThreatMultiplier: 1,

		ApplyEffects: func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
			baseDamage := core.ApplyPct(normalizedStrikeBase(sim, spell, isMH, spell.Direct.Roll(sim)), spell.Direct.WeaponPct)
			if hasGlyph {
				baseDamage = core.ApplyPct(baseDamage, 1+0.01*min(dk.CurrentRunicPower(), 25))
			}
			baseDamage *= dk.RoRTSBonus(target)
			// the class scripts are DoneTotalMod, which scales the flat creature-type bonus too
			baseDamage += spell.WeaponBonusForDonePct(target, dk.RoRTSBonus(target))

			result := spell.CalcDamage(sim, target, baseDamage, dk.threatOfThassarianOutcomeApplier(spell))

			if isMH {
				spell.SpendRefundableCostAndConvertFrostOrUnholyRune(sim, result, deathConvertChance)

				if result.Landed() {
					// spell_dk_death_strike: 5% of max health a disease, then Improved Death Strike's effect 2 adds 25% a rank
					healingAmount := dk.dkCountActiveDiseases(target) * dk.CountPctFromMaxHealth(5)
					healingAmount += calculatePct(healingAmount, int64(25*dk.Talents.ImprovedDeathStrike))
					dk.gainHealingTakenOnly(sim, healingAmount, healthMetrics)
					dk.DeathStrikeHeals = append(dk.DeathStrikeHeals, healingAmount)
				}

				dk.threatOfThassarianProc(sim, result, dk.DeathStrikeOhHit)
			} else if result.Landed() {
				// Death Rune Mastery procs from procs, so the off hand rolls for the main hand's runes too
				dk.RollFrostUnholyDeathRunes(sim, deathConvertChance)
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
		conf.ThreatSpellID = 66953
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

// registerDrwDeathStrikeSpell is the weapon's Death Strike, with no Glyph of Death Strike: Spell::EffectWeaponDmg
// asks the caster, the weapon, for its aura.
func (dk *Deathknight) registerDrwDeathStrikeSpell() {
	direct, mods := dk.deathStrikeEffect()

	dk.RuneWeapon.DeathStrike = dk.RuneWeapon.RegisterSpell(core.SpellConfig{
		ActionID:    DeathStrikeActionID.WithTag(1),
		SpellSchool: core.SpellSchoolPhysical,
		ProcMask:    core.ProcMaskMeleeMHSpecial,
		Flags:       core.SpellFlagMeleeMetrics | core.SpellFlagIncludeTargetBonusDamage,

		BonusCritRating:  (dk.annihilationCritBonus() + dk.viciousStrikesCritChanceBonus() + dk.improvedDeathStrikeCritBonus() + dk.scourgeborneBattlegearCritBonus()) * core.CritRatingPerCritChance,
		DamageMultiplier: dk.improvedDeathStrikeDamageBonus(),
		CritMultiplier:   dk.drwBonusCritMultiplier(dk.Talents.MightOfMograine),
		ThreatMultiplier: 1,

		Direct: direct,
		Mods:   mods,

		ApplyEffects: func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
			baseDamage := core.ApplyPct(spell.Direct.Roll(sim)+dk.DrwWeaponDamage(sim, true), spell.Direct.WeaponPct)
			// Rage of Rivendare and Tundra Stalker scale the flat bonus too
			baseDamage = (baseDamage + dk.drwVersusAttackPowerBonus(spell, true)) * dk.RoRTSBonus(target)
			spell.CalcAndDealDamage(sim, target, baseDamage, spell.OutcomeMeleeSpecialHitAndCrit)
		},
	})
}
