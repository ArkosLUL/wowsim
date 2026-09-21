package deathknight

import (
	"time"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
)

func (dk *Deathknight) drwCountActiveDiseases(target *core.Unit) float64 {
	count := 0
	if dk.Talents.DancingRuneWeapon {
		if dk.RuneWeapon.FrostFeverSpell.Dot(target).IsActive() {
			count++
		}
		if dk.RuneWeapon.BloodPlagueSpell.Dot(target).IsActive() {
			count++
		}
	}
	return float64(count)
}

func (dk *Deathknight) dkCountActiveDiseases(target *core.Unit) float64 {
	count := 0
	if dk.FrostFeverSpell.Dot(target).IsActive() {
		count++
	}
	if dk.BloodPlagueSpell.Dot(target).IsActive() {
		count++
	}
	if dk.Talents.CryptFever > 0 && dk.EbonPlagueOrCryptFeverAura.Get(target).IsActive() {
		count++
	}
	return float64(count)
}

func (dk *Deathknight) dkCountActiveDiseasesBcb(target *core.Unit) float64 {
	count := 0
	if dk.FrostFeverSpell.Dot(target).IsActive() {
		count++
	}
	if dk.BloodPlagueSpell.Dot(target).IsActive() {
		count++
	}
	if target.HasActiveAuraWithTag("EbonPlaguebringer") {
		count++
	}
	return float64(count)
}

// diseaseMultiplier calculates the bonus based on if you have DarkrunedBattlegear 4p.
//
//	This function is slow so should only be used during initialization.
func (dk *Deathknight) dkDiseaseMultiplier(multiplier float64) float64 {
	if dk.Env.IsFinalized() {
		panic("dont call dk.diseaseMultiplier function during runtime, cache result during initialization")
	}
	if dk.HasSetBonus(ItemSetDarkrunedBattlegear, 4) {
		return multiplier * 1.2
	}
	return multiplier
}

func (dk *Deathknight) registerDiseaseDots() {
	dk.registerFrostFever()
	dk.registerBloodPlague()
}

// Both diseases tick for CalcValue at level 80: -1 + int32(realPointsPerLevel * (80 - max(baseLevel,
// spellLevel))) + 1, with the 0.06325 AP of their spell_bonus_data.
const (
	frostFeverTickBase  = 25 // 0.32 * 80
	bloodPlagueTickBase = 31 // 0.394 * 79
)

// Only an SPELL_AURA_ABILITY_PERIODIC_CRIT aura lets a disease tick crit (AuraEffect::CalcPeriodicCritChance):
// mod-spell-tweaks puts one on Runic Power Mastery for Frost Fever and on Crypt Fever for Blood Plague,
// and the T9 4pc (67118) has one for Blood Plague.
func (dk *Deathknight) frostFeverCanCrit() bool {
	return dk.Talents.RunicPowerMastery > 0
}

func (dk *Deathknight) bloodPlagueCanCrit() bool {
	return dk.Talents.CryptFever > 0 || dk.HasSetBonus(ItemSetThassariansBattlegear, 4)
}

// diseasesAddTicks is mod-spell-tweaks' spell_tweaks_disease_haste: with Epidemic, the owner's melee
// haste shortens the tick interval when the disease goes on, and the duration stays, so it fits more
// ticks. The script reads its caster's auras, so a rune weapon's diseases never get it.
func (dk *Deathknight) diseasesAddTicks() bool {
	return dk.Talents.Epidemic > 0 && dk.Server().SpellTweaks.DiseaseHaste
}

// diseaseCritChance is the crit a disease tick snapshots. Both diseases are melee damage class on the
// server (Frost Fever by mod-spell-tweaks' spell_dbc row), so SpellDoneCritChance hands them the
// owner's melee crit, and the target takes the melee skill penalty off it.
func (dk *Deathknight) diseaseCritChance(dot *core.Dot, target *core.Unit) float64 {
	return dot.Spell.PhysicalCritChance(dk.AttackTables[target.UnitIndex])
}

// refreshDiseaseDuration is Aura::RefreshDuration, which Glyph of Disease's Pestilence does to the
// target's diseases: the duration and tick count start over, and the amount, crit and tick timer stay.
// The duration goes back to the max, Glyph of Scourge Strike's 3 s extensions included.
func refreshDiseaseDuration(sim *core.Simulation, dot *core.Dot, extensions int) {
	dot.Aura.Refresh(sim)
	if extensions > 0 {
		dot.Aura.UpdateExpires(dot.Aura.ExpiresAt() + time.Duration(extensions)*3*time.Second)
	}
	dot.TickCount = 0
}

func (dk *Deathknight) registerFrostFever() {
	dk.FrostFeverDebuffAura = dk.NewEnemyAuraArray(func(target *core.Unit) *core.Aura {
		return core.FrostFeverAura(target, dk.Talents.ImprovedIcyTouch, dk.Talents.Epidemic)
	})

	canCrit := dk.frostFeverCanCrit()

	dk.FrostFeverSpell = dk.RegisterSpell(core.SpellConfig{
		ActionID:    core.ActionID{SpellID: 55095},
		SpellSchool: core.SpellSchoolFrost,
		ProcMask:    core.ProcMaskSpellDamage,
		Flags:       core.SpellFlagDisease,

		DamageMultiplier: core.TernaryFloat64(dk.HasMajorGlyph(proto.DeathknightMajorGlyph_GlyphOfIcyTouch), 1.2, 1.0),
		CritMultiplier:   dk.DefaultMeleeCritMultiplier(),
		ThreatMultiplier: 1,

		Dot: core.DotConfig{
			Aura: core.Aura{
				Label: "FrostFever",
				Tag:   "FrostFever",
				OnGain: func(aura *core.Aura, sim *core.Simulation) {
					if dk.IcyTalonsAura != nil {
						dk.IcyTalonsAura.Activate(sim)
					}
					if dk.Talents.CryptFever > 0 {
						dk.EbonPlagueOrCryptFeverAura.Get(aura.Unit).Activate(sim)
					}
				},
			},
			NumberOfTicks:       5 + dk.Talents.Epidemic,
			TickLength:          time.Second * 3,
			AffectedByCastSpeed: dk.diseasesAddTicks(),
			TickHaste:           core.MeleeHasteAddsTicks,
			TicksCanCrit:        canCrit,
			OnSnapshot: func(sim *core.Simulation, target *core.Unit, dot *core.Dot, isRollover bool) {
				dot.SnapshotBaseDamage = frostFeverTickBase + 0.06325*dk.getImpurityBonus(dot.Spell)

				if !isRollover {
					dot.SnapshotCritChance = dk.diseaseCritChance(dot, target)
					dot.SnapshotAttackerMultiplier = dot.Spell.AttackerDamageMultiplier(dot.Spell.Unit.AttackTables[target.UnitIndex])
					dot.SnapshotAttackerMultiplier *= dk.RoRTSBonus(target)
				}
			},
			OnTick: func(sim *core.Simulation, target *core.Unit, dot *core.Dot) {
				var result *core.SpellResult
				if canCrit {
					result = dot.CalcAndDealPeriodicSnapshotDamage(sim, target, dot.OutcomeSnapshotCrit)
				} else {
					result = dot.CalcAndDealPeriodicSnapshotDamage(sim, target, dot.Spell.OutcomeAlwaysHit)
				}
				dk.doWanderingPlague(sim, dot.Spell, result)
			},
		},

		ApplyEffects: func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
			dot := spell.Dot(target)
			dot.Apply(sim)
			dk.FrostFeverDebuffAura.Get(target).Activate(sim)
		},

		RelatedAuras: []core.AuraArray{dk.FrostFeverDebuffAura},
	})
	dk.FrostFeverExtended = make([]int, dk.Env.GetNumTargets())
}

func (dk *Deathknight) registerBloodPlague() {
	canCrit := dk.bloodPlagueCanCrit()

	// SM can proc off blood plague application
	bloodPlagueApplicationSpell := dk.RegisterSpell(core.SpellConfig{
		ActionID:    core.ActionID{SpellID: 55078}.WithTag(1),
		SpellSchool: core.SpellSchoolShadow,
		ProcMask:    core.ProcMaskProc,
		Flags:       core.SpellFlagNoLogs | core.SpellFlagNoMetrics,
		ApplyEffects: func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
			dk.BloodPlagueSpell.Dot(target).Apply(sim)
		},
	})

	dk.BloodPlagueSpell = dk.RegisterSpell(core.SpellConfig{
		ActionID:    core.ActionID{SpellID: 55078},
		SpellSchool: core.SpellSchoolShadow,
		ProcMask:    core.ProcMaskSpellDamage,
		Flags:       core.SpellFlagDisease,

		DamageMultiplier: 1,
		CritMultiplier:   dk.DefaultMeleeCritMultiplier(),
		ThreatMultiplier: 1,

		Dot: core.DotConfig{
			Aura: core.Aura{
				Label: "BloodPlague",
				Tag:   "BloodPlague",
				OnGain: func(aura *core.Aura, sim *core.Simulation) {
					if dk.Talents.CryptFever > 0 {
						dk.EbonPlagueOrCryptFeverAura.Get(aura.Unit).Activate(sim)
					}
				},
			},
			NumberOfTicks:       5 + dk.Talents.Epidemic,
			TickLength:          time.Second * 3,
			AffectedByCastSpeed: dk.diseasesAddTicks(),
			TickHaste:           core.MeleeHasteAddsTicks,
			TicksCanCrit:        canCrit,

			OnSnapshot: func(sim *core.Simulation, target *core.Unit, dot *core.Dot, isRollover bool) {
				dot.SnapshotBaseDamage = bloodPlagueTickBase + 0.06325*dk.getImpurityBonus(dot.Spell)

				if !isRollover {
					dot.SnapshotCritChance = dk.diseaseCritChance(dot, target)
					dot.SnapshotAttackerMultiplier = dot.Spell.AttackerDamageMultiplier(dot.Spell.Unit.AttackTables[target.UnitIndex])
					dot.SnapshotAttackerMultiplier *= dk.RoRTSBonus(target)
				}
			},
			OnTick: func(sim *core.Simulation, target *core.Unit, dot *core.Dot) {
				var result *core.SpellResult
				if canCrit {
					result = dot.CalcAndDealPeriodicSnapshotDamage(sim, target, dot.OutcomeSnapshotCrit)
				} else {
					result = dot.CalcAndDealPeriodicSnapshotDamage(sim, target, dot.Spell.OutcomeAlwaysHit)
				}
				dk.doWanderingPlague(sim, dot.Spell, result)
			},
		},

		ApplyEffects: func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
			bloodPlagueApplicationSpell.Cast(sim, target)
		},
	})
	dk.BloodPlagueExtended = make([]int, dk.Env.GetNumTargets())
}
func (dk *Deathknight) registerDrwDiseaseDots() {
	dk.registerDrwFrostFever()
	dk.registerDrwBloodPlague()
}

func (dk *Deathknight) registerDrwFrostFever() {
	// CalcPeriodicCritChance goes by the rune weapon's spell mod owner, the DK: his talents, his crit
	canCrit := dk.frostFeverCanCrit()

	dk.RuneWeapon.FrostFeverSpell = dk.RuneWeapon.RegisterSpell(core.SpellConfig{
		ActionID:    core.ActionID{SpellID: 55095},
		SpellSchool: core.SpellSchoolFrost,
		ProcMask:    core.ProcMaskSpellDamage,
		Flags:       core.SpellFlagDisease,

		DamageMultiplier: core.TernaryFloat64(dk.HasMajorGlyph(proto.DeathknightMajorGlyph_GlyphOfIcyTouch), 1.2, 1.0),
		CritMultiplier:   dk.RuneWeapon.DefaultMeleeCritMultiplier(),
		ThreatMultiplier: 1,

		Dot: core.DotConfig{
			Aura: core.Aura{
				Label: "DrwFrostFever",
			},
			NumberOfTicks: 5 + dk.Talents.Epidemic,
			TickLength:    time.Second * 3,
			TicksCanCrit:  canCrit,
			OnSnapshot: func(sim *core.Simulation, target *core.Unit, dot *core.Dot, isRollover bool) {
				dot.SnapshotBaseDamage = frostFeverTickBase + 0.06325*dk.getImpurityBonus(dot.Spell)

				if !isRollover {
					dot.SnapshotCritChance = dk.diseaseCritChance(dot, target)
					dot.SnapshotAttackerMultiplier = dot.Spell.AttackerDamageMultiplier(dot.Spell.Unit.AttackTables[target.UnitIndex])
				}
			},
			OnTick: func(sim *core.Simulation, target *core.Unit, dot *core.Dot) {
				if canCrit {
					dot.CalcAndDealPeriodicSnapshotDamage(sim, target, dot.OutcomeSnapshotCrit)
				} else {
					dot.CalcAndDealPeriodicSnapshotDamage(sim, target, dot.Spell.OutcomeAlwaysHit)
				}
			},
		},

		ApplyEffects: func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
			spell.Dot(target).Apply(sim)
		},
	})
}

func (dk *Deathknight) registerDrwBloodPlague() {
	canCrit := dk.bloodPlagueCanCrit()

	dk.RuneWeapon.BloodPlagueSpell = dk.RuneWeapon.RegisterSpell(core.SpellConfig{
		ActionID:    core.ActionID{SpellID: 55078},
		SpellSchool: core.SpellSchoolShadow,
		ProcMask:    core.ProcMaskSpellDamage,
		Flags:       core.SpellFlagDisease,

		DamageMultiplier: 1,
		CritMultiplier:   dk.RuneWeapon.DefaultMeleeCritMultiplier(),
		ThreatMultiplier: 1,

		Dot: core.DotConfig{
			Aura: core.Aura{
				Label: "DrwBloodPlague",
			},
			NumberOfTicks: 5 + dk.Talents.Epidemic,
			TickLength:    time.Second * 3,
			TicksCanCrit:  canCrit,

			OnSnapshot: func(sim *core.Simulation, target *core.Unit, dot *core.Dot, isRollover bool) {
				dot.SnapshotBaseDamage = bloodPlagueTickBase + 0.06325*dk.getImpurityBonus(dot.Spell)

				if !isRollover {
					dot.SnapshotCritChance = dk.diseaseCritChance(dot, target)
					dot.SnapshotAttackerMultiplier = dot.Spell.AttackerDamageMultiplier(dot.Spell.Unit.AttackTables[target.UnitIndex])
				}
			},
			OnTick: func(sim *core.Simulation, target *core.Unit, dot *core.Dot) {
				var result *core.SpellResult
				if canCrit {
					result = dot.CalcAndDealPeriodicSnapshotDamage(sim, target, dot.OutcomeSnapshotCrit)
				} else {
					result = dot.CalcAndDealPeriodicSnapshotDamage(sim, target, dot.Spell.OutcomeAlwaysHit)
				}
				dk.doWanderingPlague(sim, dot.Spell, result)
			},
		},

		ApplyEffects: func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
			spell.Dot(target).Apply(sim)
		},
	})
}

// spell_dk_wandering_plague_aura puts 50526 on a 1 s cooldown after each proc
const wanderingPlagueCooldown = time.Second

func (dk *Deathknight) doWanderingPlague(sim *core.Simulation, spell *core.Spell, result *core.SpellResult) {
	if dk.Talents.WanderingPlague == 0 {
		return
	}

	if sim.CurrentTime < dk.LastTickTime+wanderingPlagueCooldown {
		return
	}

	attackTable := dk.AttackTables[result.Target.UnitIndex]
	physCritChance := spell.PhysicalCritChance(attackTable)
	if sim.RandomFloat("Wandering Plague Roll") < physCritChance {
		dk.LastTickTime = sim.CurrentTime
		dk.LastDiseaseDamage = result.Damage / dk.WanderingPlague.TargetDamageMultiplier(attackTable, false)
		dk.WanderingPlague.Cast(sim, result.Target)
	}
}
