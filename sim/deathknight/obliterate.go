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
	deathConvertChance := dk.deathRuneMasteryChance()

	conf := core.SpellConfig{
		ActionID:    ObliterateActionID.WithTag(core.TernaryInt32(isMH, 1, 2)),
		SpellSchool: core.SpellSchoolPhysical,
		ProcMask:    dk.threatOfThassarianProcMask(isMH),
		Flags:       core.SpellFlagMeleeMetrics | core.SpellFlagIncludeTargetBonusDamage,

		RuneCost: core.RuneCostOptions{
			FrostRuneCost:  1,
			UnholyRuneCost: 1,
			// Chill of the Grave's and Dirge's class masks both cover Obliterate
			RunicPowerGain: 15 + 2.5*float64(dk.Talents.ChillOfTheGrave+dk.Talents.Dirge) + dk.scourgeborneBattlegearRunicPowerBonus(),
			Refundable:     true,
		},
		Cast: core.CastConfig{
			DefaultCast: core.Cast{
				GCD: core.GCDDefault,
			},
			IgnoreHaste: true,
		},

		BonusCritRating:    (dk.rimeCritBonus() + dk.subversionCritBonus() + dk.annihilationCritBonus() + dk.scourgeborneBattlegearCritBonus()) * core.CritRatingPerCritChance,
		TotalPctMultiplier: core.TernaryFloat64(isMH, 1, dk.nervesOfColdSteelBonus()),
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
			baseDamage := core.ApplyPct(normalizedStrikeBase(sim, spell, isMH, spell.Direct.Roll(sim)), spell.Direct.WeaponPct)
			bonus := dk.RoRTSBonus(target)
			baseDamage = core.ApplyPct(baseDamage, 1.0+dk.dkCountActiveDiseases(target)*diseaseMulti) *
				bonus *
				dk.mercilessCombatBonus(sim)
			// the class scripts are DoneTotalMod, which scales the flat creature-type bonus too
			baseDamage += spell.WeaponBonusForDonePct(target, bonus*dk.mercilessCombatBonus(sim))

			// the off hand's strike goes after the main hand's has taken the diseases, and rolls Annihilation itself
			result := spell.CalcDamage(sim, target, baseDamage, dk.obliterateOutcome(dk.threatOfThassarianOutcomeApplier(spell), bonus,
				func(sim *core.Simulation, target *core.Unit) {
					if sim.RandomFloat("Annihilation") < diseaseConsumptionChance {
						dk.takeDiseases(sim, target)
					}
				}))

			if isMH {
				spell.SpendRefundableCostAndConvertFrostOrUnholyRune(sim, result, deathConvertChance)

				// spell_proc's default hit mask: only a landed hit procs it
				if result.Landed() && sim.RandomFloat("Rime") < dk.rimeHbChanceProc() {
					dk.FreezingFogAura.Activate(sim)
				}
				dk.threatOfThassarianProc(sim, result, dk.ObliterateOhHit)
			} else if result.Landed() {
				// Death Rune Mastery procs from procs, so the off hand rolls for the main hand's runes too
				dk.RollFrostUnholyDeathRunes(sim, deathConvertChance)
			}

			spell.DealDamage(sim, result)
		},
	}

	if !isMH {
		conf.RuneCost = core.RuneCostOptions{}
		conf.Cast = core.CastConfig{}
		// Threat of Thassarian's off-hand strike
		conf.Direct = core.SpellEffect{Effect: 0, FromSpellID: 66974, Min: 292, Max: 292, WeaponPct: 0.8}
		conf.ThreatSpellID = 66974
	} else {
		conf.Flags |= core.SpellFlagAPL
	}

	return dk.RegisterSpell(conf)
}

// obliterateOutcome is outcome plus the launch of Spell::EffectWeaponDmg's Obliterate branch, which only a hit that
// isn't missed, dodged or parried gets: consume takes the diseases off before MeleeDamageBonusDone reads Rage of
// Rivendare and Tundra Stalker, so the damage, built with bonus, drops to the one left after.
func (dk *Deathknight) obliterateOutcome(outcome core.OutcomeApplier, bonus float64, consume func(*core.Simulation, *core.Unit)) core.OutcomeApplier {
	return func(sim *core.Simulation, result *core.SpellResult, attackTable *core.AttackTable) {
		outcome(sim, result, attackTable)
		if result.Outcome.Matches(core.OutcomeMiss | core.OutcomeDodge | core.OutcomeParry) {
			return
		}
		consume(sim, result.Target)
		after := dk.RoRTSBonus(result.Target)
		if after == bonus {
			return
		}
		// a partial block took the whole block value off after the bonus, or all of a smaller hit
		switch scale := after / bonus; {
		case !result.Outcome.Matches(core.OutcomeBlock):
			result.Damage *= scale
		case result.Damage > 0:
			blockValue := result.Target.BlockValue()
			result.Damage = max(0, (result.Damage+blockValue)*scale-blockValue)
		}
	}
}

// takeDiseases is GetDiseasesByCaster's consume: every disease of the knight's, Crypt Fever or Ebon Plague too.
func (dk *Deathknight) takeDiseases(sim *core.Simulation, target *core.Unit) {
	dk.FrostFeverSpell.Dot(target).Deactivate(sim)
	// the attack speed slow is the same 55095 aura, so it goes too, unless it's the raid's debuff or another
	// knight's Frost Fever is still up
	if slow := dk.FrostFeverDebuffAura.Get(target); slow.Duration != core.NeverExpires && !target.HasActiveAuraWithTag("FrostFever") {
		slow.Deactivate(sim)
	}
	dk.BloodPlagueSpell.Dot(target).Deactivate(sim)
	if dk.Talents.CryptFever > 0 {
		dk.EbonPlagueOrCryptFeverAura.Get(target).Deactivate(sim)
	}
}

func (dk *Deathknight) registerObliterateSpell() {
	dk.ObliterateMhHit = dk.newObliterateHitSpell(true)
	dk.ObliterateOhHit = dk.newObliterateHitSpell(false)
	dk.Obliterate = dk.ObliterateMhHit
}
