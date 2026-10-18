package deathknight

import (
	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
)

var PestilenceActionID = core.ActionID{SpellID: 50842}

func (dk *Deathknight) registerPestilenceSpell() {
	hasGlyphOfDisease := dk.HasMajorGlyph(proto.DeathknightMajorGlyph_GlyphOfDisease)
	deathConvertChance := dk.bloodDeathRuneChance()

	dk.Pestilence = dk.RegisterSpell(core.SpellConfig{
		ActionID:    core.ActionID{SpellID: 50842},
		Flags:       core.SpellFlagAPL,
		SpellSchool: core.SpellSchoolShadow,
		ProcMask:    core.ProcMaskSpellDamage,

		RuneCost: core.RuneCostOptions{
			BloodRuneCost:  1,
			RunicPowerGain: 10,
			Refundable:     true,
		},
		Cast: core.CastConfig{
			DefaultCast: core.Cast{
				GCD: core.GCDDefault,
			},
		},

		DamageMultiplier: 0,
		ThreatMultiplier: 0,

		ApplyEffects: func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
			for _, aoeTarget := range sim.Encounter.TargetUnits {
				// Zero damage spell with a Hit mechanic, thanks blizz!
				result := spell.CalcAndDealDamage(sim, aoeTarget, 0, spell.OutcomeMagicHit)

				if aoeTarget == target {
					spell.SpendRefundableCostAndConvertBloodRune(sim, result, deathConvertChance)
				}
				if result.Landed() {
					refreshCFEP := false
					// Main target
					if aoeTarget == target {
						if hasGlyphOfDisease {
							// spell_dk_pestilence: only the duration starts over
							if dk.FrostFeverSpell.Dot(aoeTarget).IsActive() {
								refreshDiseaseDuration(sim, dk.FrostFeverSpell.Dot(aoeTarget), dk.FrostFeverExtended[aoeTarget.Index])
								if dk.Talents.IcyTalons > 0 {
									dk.IcyTalonsAura.Activate(sim)
								}
								dk.FrostFeverDebuffAura.Get(aoeTarget).Activate(sim)

								if dk.Talents.CryptFever > 0 {
									refreshCFEP = true
								}
							}

							if dk.BloodPlagueSpell.Dot(aoeTarget).IsActive() {
								refreshDiseaseDuration(sim, dk.BloodPlagueSpell.Dot(aoeTarget), dk.BloodPlagueExtended[aoeTarget.Index])

								if dk.Talents.CryptFever > 0 {
									refreshCFEP = true
								}
							}
						}
					} else {
						// Apply diseases on every other target
						if dk.FrostFeverSpell.Dot(target).IsActive() {
							dk.FrostFeverExtended[aoeTarget.Index] = 0
							dk.FrostFeverSpell.Dot(aoeTarget).Apply(sim)

							if dk.Talents.CryptFever > 0 {
								refreshCFEP = true
							}
						}
						if dk.BloodPlagueSpell.Dot(target).IsActive() {
							dk.BloodPlagueExtended[aoeTarget.Index] = 0
							dk.BloodPlagueSpell.Dot(aoeTarget).Apply(sim)

							if dk.Talents.CryptFever > 0 {
								refreshCFEP = true
							}
						}
					}

					if refreshCFEP {
						dk.EbonPlagueOrCryptFeverAura.Get(aoeTarget).Activate(sim)
					}
				}
			}
		},
	})
}
