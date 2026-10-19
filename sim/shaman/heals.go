package shaman

import (
	"time"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
)

func (shaman *Shaman) registerAncestralHealingSpell() {
	shaman.AncestralAwakening = shaman.RegisterSpell(core.SpellConfig{
		ActionID:         core.ActionID{SpellID: 52752},
		SpellSchool:      core.SpellSchoolNature,
		ProcMask:         core.ProcMaskSpellHealing,
		Flags:            core.SpellFlagHelpful | core.SpellFlagAPL | core.SpellFlagIgnoreAttackerModifiers,
		DamageMultiplier: 1, // 52752 has SPELL_ATTR3_IGNORE_CASTER_MODIFIERS, so no Purification
		CritMultiplier:   1,
		ThreatMultiplier: 1 - (float64(shaman.Talents.HealingGrace) * 0.05),
		// 52752 has no spell_bonus_data row
		HealingTakenCoefficient: core.DefaultHealingTakenCoefficient(0),
		ApplyEffects: func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
			spell.CalcAndDealHealing(sim, target, shaman.ancestralHealingAmount, spell.OutcomeHealing)
		},
	})
}

// procAncestralAwakening is spell_sha_ancestral_awakening: a crit heal casts 52759 for the talent's percent of
// it, and spell_sha_ancestral_awakening_proc heals the living raid member lowest in health percent (52759's area
// is 40 yd round the shaman, the whole raid in the sim). Its stable sort leaves a tie in the grid search's order,
// raid order here.
func (shaman *Shaman) procAncestralAwakening(sim *core.Simulation, crit *core.SpellResult) {
	if shaman.Talents.AncestralAwakening == 0 {
		return
	}
	if shaman.raidPets == nil {
		shaman.raidPets = make(map[*core.Unit]*core.Pet)
		for _, party := range shaman.Env.Raid.Parties {
			for _, agent := range party.Pets {
				shaman.raidPets[&agent.GetPet().Unit] = agent.GetPet()
			}
		}
	}
	var target *core.Unit
	var lowest float32
	for _, unit := range shaman.Env.Raid.AllUnits {
		// a pet that's away isn't in the world, and SpellInfo::CheckTarget keeps the dead out
		if !unit.IsEnabled() || (unit.HasHealthBar() && unit.CurrentHealth() <= 0) {
			continue
		}
		// TARGET_CHECK_RAID skips a guardian without a pet bar
		if pet := shaman.raidPets[unit]; pet != nil && pet.Uncontrollable {
			continue
		}
		if pct := healthPctOrder(unit); target == nil || pct < lowest {
			target, lowest = unit, pct
		}
	}
	if target == nil {
		return
	}
	shaman.ancestralHealingAmount = float64(int32(float32(crit.Damage) * float32(10*shaman.Talents.AncestralAwakening) / 100))
	shaman.AncestralAwakening.Cast(sim, target)
}

// healthPctOrder is Acore::HealthPctOrderPred's float ratio of whole health, full without a health bar.
func healthPctOrder(unit *core.Unit) float32 {
	if !unit.HasHealthBar() {
		return 1
	}
	cur, most := uint32(unit.CurrentHealth()), uint32(unit.MaxHealth())
	// health can sit above max after max health drops, where the server caps it
	return float32(min(cur, most)) / float32(most)
}

// impWaterShieldChance is Improved Water Shield's chance on a heal's crit: the talent's proc chance, then
// spell_sha_imp_water_shield's pct of it (60 for Lesser Healing Wave, 30 for Chain Heal, 100 otherwise).
func (shaman *Shaman) impWaterShieldChance(pct int32) float64 {
	return float64([]int32{0, 33, 66, 100}[shaman.Talents.ImprovedWaterShield]*pct) / 10000
}

// procImpWaterShield rolls Improved Water Shield: its proc casts Water Shield's mana, without using an orb, and
// only with Water Shield up.
func (shaman *Shaman) procImpWaterShield(sim *core.Simulation, chance float64) {
	if chance == 0 || shaman.SelfBuffs.Shield != proto.ShamanShield_WaterShield {
		return
	}
	if chance >= 1 || sim.RandomFloat("imp water shield") < chance {
		shaman.AddMana(sim, 428*(1+0.05*float64(shaman.Talents.ImprovedShields)), shaman.waterShieldManaMetrics)
	}
}

func (shaman *Shaman) registerLesserHealingWaveSpell() {
	impShieldChance := shaman.impWaterShieldChance(60)

	hasGlyph := shaman.HasMajorGlyph(proto.ShamanMajorGlyph_GlyphOfLesserHealingWave)

	// The Totems of the Third Wind add healing power (class script 3736), which the coefficient scales
	relicHealingPower := 0 +
		core.TernaryFloat64(shaman.Ranged().ID == 42598, 320, 0) +
		core.TernaryFloat64(shaman.Ranged().ID == 42597, 267, 0) +
		core.TernaryFloat64(shaman.Ranged().ID == 42596, 236, 0) +
		core.TernaryFloat64(shaman.Ranged().ID == 42595, 204, 0)

	shaman.LesserHealingWave = shaman.RegisterSpell(core.SpellConfig{
		ActionID:    core.ActionID{SpellID: 49276},
		SpellSchool: core.SpellSchoolNature,
		ProcMask:    core.ProcMaskSpellHealing,
		Flags:       core.SpellFlagHelpful | core.SpellFlagAPL,

		ManaCost: core.ManaCostOptions{
			BaseCost:   0.15,
			Multiplier: 1 - .01*float64(shaman.Talents.TidalFocus),
		},
		Cast: core.CastConfig{
			DefaultCast: core.Cast{
				GCD:      core.GCDDefault,
				CastTime: time.Millisecond * 1500,
			},
		},

		BonusCritRating: float64(shaman.Talents.TidalMastery) * 1 * core.CritRatingPerCritChance,
		DamageMultiplier: 1 *
			(1 + .02*float64(shaman.Talents.Purification)),
		CritMultiplier:   shaman.DefaultHealingCritMultiplier(),
		ThreatMultiplier: 1 - (float64(shaman.Talents.HealingGrace) * 0.05),

		Direct: core.SpellEffect{Effect: 0, Min: 1624, Max: 1852, SP: 0.8057},
		Mods:   []core.SpellMod{{Op: core.SpellModBonusMultiplier, Flat: 2 * shaman.Talents.TidalWaves}},

		ApplyEffects: func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
			healPower := shaman.healingPower(spell, target)
			baseHealing := spell.Direct.Roll(sim) + core.PowerBonus(spell.Direct.SP, (healPower+relicHealingPower))
			if hasGlyph {
				if shaman.EarthShield.Hot(target).IsActive() {
					baseHealing *= 1.2
				}
			}
			result := spell.CalcAndDealHealing(sim, target, baseHealing, spell.OutcomeHealingCrit)

			if result.Outcome.Matches(core.OutcomeCrit) {
				shaman.procImpWaterShield(sim, impShieldChance)
				shaman.procAncestralAwakening(sim, result)
			}

			if shaman.tidalWaveProc.IsActive() {
				shaman.tidalWaveProc.RemoveStack(sim)
			}
		},
	})
}

func (shaman *Shaman) registerRiptideSpell() {
	impShieldChance := shaman.impWaterShieldChance(100)

	shaman.Riptide = shaman.RegisterSpell(core.SpellConfig{
		ActionID:    core.ActionID{SpellID: 61301},
		SpellSchool: core.SpellSchoolNature,
		ProcMask:    core.ProcMaskSpellHealing,
		Flags:       core.SpellFlagHelpful | core.SpellFlagAPL,

		ManaCost: core.ManaCostOptions{
			BaseCost: 0.18,
			Multiplier: 1 - .01*float64(shaman.Talents.TidalFocus) -
				0.02*float64(shaman.Talents.MentalQuickness),
		},
		Cast: core.CastConfig{
			DefaultCast: core.Cast{
				GCD: core.GCDDefault,
			},
			CD: core.Cooldown{
				Timer:    shaman.NewTimer(),
				Duration: time.Second * 6,
			},
		},

		BonusCritRating: float64(shaman.Talents.TidalMastery) * 1 * core.CritRatingPerCritChance,
		DamageMultiplier: 1 *
			(1 + .02*float64(shaman.Talents.Purification)),
		CritMultiplier:   shaman.DefaultHealingCritMultiplier(),
		ThreatMultiplier: 1 - (float64(shaman.Talents.HealingGrace) * 0.05),

		Direct: core.SpellEffect{Effect: 0, Min: 1604, Max: 1736, SP: 0.402},
		Hot: core.DotConfig{
			Aura: core.Aura{
				Label: "Riptide",
			},
			NumberOfTicks: 5,
			TickLength:    time.Second * 3,
			TicksCanCrit:  false,
			Tick:          core.SpellEffect{Effect: 1, Min: 334, Max: 334, SP: 0.188},
			OnSnapshot: func(sim *core.Simulation, target *core.Unit, dot *core.Dot, _ bool) {
				dot.SnapshotBaseDamage = dot.Tick.Roll(sim) + core.PowerBonus(dot.Tick.SP, shaman.healingPower(dot.Spell, target))
				// Purification is a healing done percent, which each tick takes from the shaman
				dot.SnapshotHotMultiplier(dot.Spell.CasterHealingMultiplier() / (1 + .02*float64(shaman.Talents.Purification)))
			},
			OnTick: func(sim *core.Simulation, target *core.Unit, dot *core.Dot) {
				dot.CalcAndDealPeriodicSnapshotHealing(sim, target, dot.OutcomeTick)
			},
		},

		ApplyEffects: func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
			healPower := shaman.healingPower(spell, target)
			baseHealing := spell.Direct.Roll(sim) + core.PowerBonus(spell.Direct.SP, healPower)
			result := spell.CalcAndDealHealing(sim, target, baseHealing, spell.OutcomeHealingCrit)
			spell.Hot(target).Apply(sim)

			if result.Outcome.Matches(core.OutcomeCrit) {
				shaman.procImpWaterShield(sim, impShieldChance)
				shaman.procAncestralAwakening(sim, result)
			}
			if shaman.Talents.TidalWaves > 0 {
				shaman.tidalWaveProc.Activate(sim)
				shaman.tidalWaveProc.SetStacks(sim, 2)
			}
		},
	})
}

func (shaman *Shaman) registerHealingWaveSpell() {
	// TODO: finish this
	// ActionID:    core.ActionID{SpellID: 49273},

	// -79 mana totem: 39728

	impShieldChance := shaman.impWaterShieldChance(100)

	shaman.HealingWave = shaman.RegisterSpell(core.SpellConfig{
		ActionID:    core.ActionID{SpellID: 49273},
		SpellSchool: core.SpellSchoolNature,
		ProcMask:    core.ProcMaskSpellHealing,
		Flags:       core.SpellFlagHelpful | core.SpellFlagAPL,

		ManaCost: core.ManaCostOptions{
			BaseCost:   0.25,
			Multiplier: 1 - .01*float64(shaman.Talents.TidalFocus),
		},
		Cast: core.CastConfig{
			DefaultCast: core.Cast{
				GCD:      core.GCDDefault,
				CastTime: time.Millisecond * time.Duration(3000-100*shaman.Talents.ImprovedHealingWave),
			},
		},

		BonusCritRating: float64(shaman.Talents.TidalMastery) * 1 * core.CritRatingPerCritChance,
		DamageMultiplier: 1 *
			(1 + .02*float64(shaman.Talents.Purification)),
		CritMultiplier:   shaman.DefaultHealingCritMultiplier(),
		ThreatMultiplier: 1 - (float64(shaman.Talents.HealingGrace) * 0.05),

		Direct: core.SpellEffect{Effect: 0, Min: 3034, Max: 3466, SP: 1.611},
		Mods:   []core.SpellMod{{Op: core.SpellModBonusMultiplier, Flat: 4 * shaman.Talents.TidalWaves}},

		ApplyEffects: func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
			healPower := shaman.healingPower(spell, target)
			baseHealing := spell.Direct.Roll(sim) + core.PowerBonus(spell.Direct.SP, healPower)
			result := spell.CalcAndDealHealing(sim, target, baseHealing, spell.OutcomeHealingCrit)

			if result.Outcome.Matches(core.OutcomeCrit) {
				shaman.procImpWaterShield(sim, impShieldChance)
				shaman.procAncestralAwakening(sim, result)
			}

			if shaman.tidalWaveProc.IsActive() {
				shaman.tidalWaveProc.RemoveStack(sim)
			}
		},
	})
}

func (shaman *Shaman) registerEarthShieldSpell() {
	actionID := core.ActionID{SpellID: 49284}

	// spell_sha_earth_shield adds the glyph's 20% to the whole charge, then Improved Shields' percent
	// once more to what the bonuses added over the base value
	glyphBonus := core.TernaryFloat64(shaman.HasMajorGlyph(proto.ShamanMajorGlyph_GlyphOfEarthShield), 0.2, 0)
	improvedShields := 0.05 * float64(shaman.Talents.ImprovedShields)
	// 49284's healing bonus puts Purification in the charge before the glyph, in float32 like
	// GetTotalAuraMultiplier, and 379 takes no caster mods of its own
	purification := 1 + float32(2*shaman.Talents.Purification)/100

	icd := core.Cooldown{
		Timer:    shaman.NewTimer(),
		Duration: time.Millisecond * 3500,
	}

	shaman.EarthShield = shaman.RegisterSpell(core.SpellConfig{
		ActionID:    actionID,
		SpellSchool: core.SpellSchoolNature,
		ProcMask:    core.ProcMaskEmpty,
		Flags:       core.SpellFlagHelpful | core.SpellFlagAPL,

		ManaCost: core.ManaCostOptions{
			BaseCost: 0.15,
			Multiplier: 1 -
				.01*float64(shaman.Talents.TidalFocus) -
				0.02*float64(shaman.Talents.MentalQuickness),
		},
		Cast: core.CastConfig{
			DefaultCast: core.Cast{
				GCD: core.GCDDefault,
			},
		},

		// the charge's 379 crits off the shaman (Spell::DoAllEffectOnLaunchTarget's original caster). Only Tidal
		// Mastery's ranks 4 and 5 list it in their class mask
		BonusCritRating:  core.TernaryFloat64(shaman.Talents.TidalMastery >= 4, float64(shaman.Talents.TidalMastery), 0) * core.CritRatingPerCritChance,
		CritMultiplier:   shaman.DefaultHealingCritMultiplier(),
		DamageMultiplier: 1,
		ThreatMultiplier: 1,
		// the heal is 379, which has no spell_bonus_data row
		HealingTakenCoefficient: core.DefaultHealingTakenCoefficient(0),
		Mods: []core.SpellMod{
			{Op: core.SpellModEffect1, Pct: 5 * shaman.Talents.ImprovedShields},
			{Op: core.SpellModEffect1, Pct: 5 * shaman.Talents.ImprovedEarthShield},
		},
		Hot: core.DotConfig{
			Aura: core.Aura{
				Label:    "Earth Shield",
				ActionID: core.ActionID{SpellID: 379},
				OnSpellHitTaken: func(aura *core.Aura, sim *core.Simulation, spell *core.Spell, result *core.SpellResult) {
					if !result.Landed() {
						return
					}
					if !icd.IsReady(sim) {
						return
					}
					icd.Use(sim)
					shaman.EarthShield.Hot(result.Target).ManualTick(sim)
					// the charge's heal is 379, a cast of its own on the server and not a tick
					shaman.EarthShield.LandOn(sim, result.Target)
				},
				OnExpire: func(aura *core.Aura, sim *core.Simulation) {
				},
			},
			NumberOfTicks: 6 + shaman.Talents.ImprovedEarthShield,
			TickLength:    time.Minute*10 + 1, // tick length longer than expire time.
			AuraDuration:  time.Minute * 10,   // charges are ticks here; the server's aura stays 10 min
			TicksCanCrit:  true,
			Tick:          core.SpellEffect{Effect: 0, Min: 337, Max: 337, SP: 0.5371},
			OnSnapshot: func(sim *core.Simulation, target *core.Unit, dot *core.Dot, _ bool) {
				base := dot.Tick.Roll(sim)
				bonus := core.PowerBonus(dot.Tick.SP, shaman.healingPower(dot.Spell, target))
				// SpellHealingBonusDone's whole heal, then each percent truncates on its own
				charge := float64(int32(float32(float32(base+bonus) * purification)))
				charge += core.ApplyPct(charge, glyphBonus)
				charge += core.ApplyPct(charge-base, improvedShields)
				dot.SnapshotBaseDamage = charge
				dot.SnapshotAttackerMultiplier = dot.Spell.CasterHealingMultiplier()
			},
			OnTick: func(sim *core.Simulation, target *core.Unit, dot *core.Dot) {
				dot.CalcAndDealPeriodicSnapshotHealing(sim, target, dot.Spell.OutcomeHealingCrit)
			},
		},
		ApplyEffects: func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
			spell.Hot(target).Apply(sim)
			spell.LandOn(sim, target)
		},
	})
}

func (shaman *Shaman) registerChainHealSpell() {
	impShieldChance := shaman.impWaterShieldChance(30)

	hasGlyph := shaman.HasMajorGlyph(proto.ShamanMajorGlyph_GlyphOfChainHeal)

	numHits := min(core.TernaryInt32(hasGlyph, 4, 3), int32(len(shaman.Env.Raid.AllUnits)))
	bounceTargets := make([]*core.Unit, 0, numHits)

	bonusHeal := 0 +
		core.TernaryFloat64(shaman.Ranged().ID == 28523, 87, 0) +
		core.TernaryFloat64(shaman.Ranged().ID == 38368, 102, 0) +
		// Steamcaller's Totem gives 243 on the server, where Classic gave 257 (effects_review.csv)
		core.TernaryFloat64(shaman.Ranged().ID == 45114, 243, 0)

	manaDiscount := 0 +
		core.TernaryFloat64(shaman.Ranged().ID == 40709, 78, 0)

	shaman.ChainHeal = shaman.RegisterSpell(core.SpellConfig{
		ActionID:    core.ActionID{SpellID: 55459},
		SpellSchool: core.SpellSchoolNature,
		ProcMask:    core.ProcMaskSpellHealing,
		Flags:       core.SpellFlagHelpful | core.SpellFlagAPL,

		ManaCost: core.ManaCostOptions{
			FlatCost:   0.19*shaman.BaseMana - manaDiscount,
			Multiplier: 1 - .01*float64(shaman.Talents.TidalFocus),
		},
		Cast: core.CastConfig{
			DefaultCast: core.Cast{
				GCD:      core.GCDDefault,
				CastTime: time.Millisecond * 2500,
			},
		},
		BonusCritRating: float64(shaman.Talents.TidalMastery) * 1 * core.CritRatingPerCritChance,
		DamageMultiplier: spellModDamage(
			.02*float64(shaman.Talents.Purification),
			0.1*float64(shaman.Talents.ImprovedChainHeal),
		),
		CritMultiplier:   shaman.DefaultHealingCritMultiplier(),
		ThreatMultiplier: 1 - (float64(shaman.Talents.HealingGrace) * 0.05),

		Direct: core.SpellEffect{Effect: 0, Min: 1055, Max: 1205, SP: 1.3428},

		ApplyEffects: func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
			bounceCoeff := 1.0
			dmgReductionPerBounce := 0.6
			curTarget := target
			// all jumps are picked before the first heal lands
			bounceTargets = chainHealJumps(sim, target, bounceTargets[:0], int(numHits-1))
			for hitIndex := 0; hitIndex <= len(bounceTargets); hitIndex++ {
				healPower := shaman.healingPower(spell, target)
				baseHealing := spell.Direct.Roll(sim) + core.PowerBonus(spell.Direct.SP, healPower) + bonusHeal
				baseHealing *= bounceCoeff

				riptide := shaman.Riptide.Hot(curTarget)
				if riptide.IsActive() {
					riptide.Deactivate(sim)
					baseHealing *= 1.25
				}

				result := spell.CalcAndDealHealing(sim, curTarget, baseHealing, spell.OutcomeHealingCrit)
				if result.Outcome.Matches(core.OutcomeCrit) {
					shaman.procImpWaterShield(sim, impShieldChance)
				}
				if shaman.Talents.TidalWaves > 0 {
					shaman.tidalWaveProc.Activate(sim)
					shaman.tidalWaveProc.SetStacks(sim, 2)
				}

				bounceCoeff *= dmgReductionPerBounce
				if hitIndex < len(bounceTargets) {
					curTarget = bounceTargets[hitIndex]
				}
			}
		},
	})
}
