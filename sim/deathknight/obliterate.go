package deathknight

import (
	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
)

// TODO: Cleanup obliterate the same way we did for plague strike
var ObliterateActionID = core.ActionID{SpellID: 51425}

func (dk *Deathknight) newObliterateHitSpell(isMH bool) *core.Spell {
	diseaseMulti := dk.dkDiseaseMultiplier(0.125)
	diseaseConsumptionChance := []float64{1.0, 0.67, 0.34, 0.0}[dk.Talents.Annihilation]
	deathConvertChance := float64(dk.Talents.DeathRuneMastery) / 3

	conf := core.SpellConfig{
		ActionID:    ObliterateActionID.WithTag(core.TernaryInt32(isMH, 1, 2)),
		SpellSchool: core.SpellSchoolPhysical,
		ProcMask:    dk.threatOfThassarianProcMask(isMH),
		Flags:       core.SpellFlagMeleeMetrics | core.SpellFlagIncludeTargetBonusDamage,

		RuneCost: core.RuneCostOptions{
			FrostRuneCost:  1,
			UnholyRuneCost: 1,
			RunicPowerGain: 15 + 2.5*float64(dk.Talents.ChillOfTheGrave) + dk.scourgeborneBattlegearRunicPowerBonus(),
			Refundable:     true,
		},
		Cast: core.CastConfig{
			DefaultCast: core.Cast{
				GCD: core.GCDDefault,
			},
			IgnoreHaste: true,
		},

		BonusCritRating: (dk.rimeCritBonus() + dk.subversionCritBonus() + dk.annihilationCritBonus() + dk.scourgeborneBattlegearCritBonus()) * core.CritRatingPerCritChance,
		DamageMultiplier: core.TernaryFloat64(isMH, 1, dk.nervesOfColdSteelBonus()) *
			dk.scourgelordsBattlegearDamageBonus(ScourgelordBonusSpellOB),
		CritMultiplier:   dk.bonusCritMultiplier(dk.Talents.GuileOfGorefiend),
		ThreatMultiplier: 1,

		Direct: core.SpellEffect{Effect: 0, Min: 584, Max: 584, WeaponPct: 0.8},
		Mods: []core.SpellMod{
			{Op: core.SpellModEffect1, Flat: dk.sigilOfAwarenessBonus()},
			{Op: core.SpellModEffect2, Flat: core.TernaryInt32(dk.HasMajorGlyph(proto.DeathknightMajorGlyph_GlyphOfObliterate), 20, 0)},
		},

		ApplyEffects: func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
			// the flat effect comes before the percent one, so the percent scales it too
			baseDamage := normalizedStrikeBase(sim, spell, isMH, spell.Direct.Roll(sim)) * spell.Direct.WeaponPct
			baseDamage *= dk.RoRTSBonus(target) *
				(1.0 + dk.dkCountActiveDiseases(target)*diseaseMulti) *
				dk.mercilessCombatBonus(sim)

			result := spell.CalcDamage(sim, target, baseDamage, dk.threatOfThassarianOutcomeApplier(spell))

			if isMH {
				spell.SpendRefundableCostAndConvertFrostOrUnholyRune(sim, result, deathConvertChance)
				dk.threatOfThassarianProc(sim, result, dk.ObliterateOhHit)

				if sim.RandomFloat("Annihilation") < diseaseConsumptionChance {
					dk.FrostFeverSpell.Dot(target).Deactivate(sim)
					dk.BloodPlagueSpell.Dot(target).Deactivate(sim)
				}

				if sim.RandomFloat("Rime") < dk.rimeHbChanceProc() {
					dk.FreezingFogAura.Activate(sim)
				}
			}

			spell.DealDamage(sim, result)
		},
	}

	if !isMH {
		conf.RuneCost = core.RuneCostOptions{}
		conf.Cast = core.CastConfig{}
		// Threat of Thassarian's off-hand strike
		conf.Direct = core.SpellEffect{Effect: 0, FromSpellID: 66974, Min: 292, Max: 292, WeaponPct: 0.8}
	} else {
		conf.Flags |= core.SpellFlagAPL
	}

	return dk.RegisterSpell(conf)
}

func (dk *Deathknight) registerObliterateSpell() {
	dk.ObliterateMhHit = dk.newObliterateHitSpell(true)
	dk.ObliterateOhHit = dk.newObliterateHitSpell(false)
	dk.Obliterate = dk.ObliterateMhHit
}
