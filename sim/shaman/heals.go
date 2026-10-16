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

func (shaman *Shaman) registerLesserHealingWaveSpell() {
	impShieldChance := 0.2 * float64(shaman.Talents.ImprovedWaterShield)
	impShieldManaGain := 428.0 * (1 + 0.05*float64(shaman.Talents.ImprovedShields))

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
				if impShieldChance > 0 {
					if sim.RandomFloat("imp water shield") > impShieldChance {
						shaman.AddMana(sim, impShieldManaGain, shaman.waterShieldManaMetrics)
					}
				}
				if shaman.Talents.AncestralAwakening > 0 {
					shaman.ancestralHealingAmount = float64(int32(float32(result.Damage) * 30 / 100))

					// TODO: this should actually target the lowest health target in the raid.
					//  does it matter in a sim? We currently only simulate tanks taking damage (multiple tanks could be handled here though.)
					shaman.AncestralAwakening.Cast(sim, target)
				}
			}

			if shaman.tidalWaveProc.IsActive() {
				shaman.tidalWaveProc.RemoveStack(sim)
			}
		},
	})
}

func (shaman *Shaman) registerRiptideSpell() {
	impShieldChance := []float64{0, 0.33, 0.66, 1.0}[shaman.Talents.ImprovedWaterShield]
	impShieldManaGain := 428.0 * (1 + 0.05*float64(shaman.Talents.ImprovedShields))

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
				dot.SnapshotAttackerMultiplier = dot.Spell.CasterHealingMultiplier()
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
				if impShieldChance > 0 {
					if impShieldChance > 0.9999 || sim.RandomFloat("imp water shield") > impShieldChance {
						shaman.AddMana(sim, impShieldManaGain, shaman.waterShieldManaMetrics)
					}
				}
				if shaman.Talents.AncestralAwakening > 0 {
					shaman.ancestralHealingAmount = float64(int32(float32(result.Damage) * 30 / 100))
					// TODO: this should actually target the lowest health target in the raid.
					//  does it matter in a sim? We currently only simulate tanks taking damage (multiple tanks could be handled here though.)
					shaman.AncestralAwakening.Cast(sim, target)
				}
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

	impShieldChance := 0.2 * float64(shaman.Talents.ImprovedWaterShield)
	impShieldManaGain := 428.0 * (1 + 0.05*float64(shaman.Talents.ImprovedShields))

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
				if impShieldChance > 0 {
					if sim.RandomFloat("imp water shield") > impShieldChance {
						shaman.AddMana(sim, impShieldManaGain, shaman.waterShieldManaMetrics)
					}
				}
				if shaman.Talents.AncestralAwakening > 0 {
					shaman.ancestralHealingAmount = float64(int32(float32(result.Damage) * 30 / 100))

					// TODO: this should actually target the lowest health target in the raid.
					//  does it matter in a sim? We currently only simulate tanks taking damage (multiple tanks could be handled here though.)
					shaman.AncestralAwakening.Cast(sim, target)
				}
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
	glyphMultiplier := core.TernaryFloat64(shaman.HasMajorGlyph(proto.ShamanMajorGlyph_GlyphOfEarthShield), 1.2, 1)
	improvedShields := 0.05 * float64(shaman.Talents.ImprovedShields)

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

		BonusCritRating:  float64(shaman.Talents.TidalMastery) * 1 * core.CritRatingPerCritChance,
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
			TicksCanCrit:  false,
			Tick:          core.SpellEffect{Effect: 0, Min: 337, Max: 337, SP: 0.5371},
			OnSnapshot: func(sim *core.Simulation, target *core.Unit, dot *core.Dot, _ bool) {
				base := dot.Tick.Roll(sim)
				amount := (base + core.PowerBonus(dot.Tick.SP, shaman.healingPower(dot.Spell, target))) * glyphMultiplier
				dot.SnapshotBaseDamage = amount + (amount-base)*improvedShields
				dot.SnapshotAttackerMultiplier = dot.Spell.CasterHealingMultiplier()
			},
			OnTick: func(sim *core.Simulation, target *core.Unit, dot *core.Dot) {
				dot.CalcAndDealPeriodicSnapshotHealing(sim, target, dot.OutcomeTick)
			},
		},
		ApplyEffects: func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
			spell.Hot(target).Apply(sim)
			spell.LandOn(sim, target)
		},
	})
}

func (shaman *Shaman) registerChainHealSpell() {
	impShieldChance := 0.1 * float64(shaman.Talents.ImprovedWaterShield)
	impShieldManaGain := 428.0 * (1 + 0.05*float64(shaman.Talents.ImprovedShields))

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
					if impShieldChance > 0 {
						if sim.RandomFloat("imp water shield") > impShieldChance {
							shaman.AddMana(sim, impShieldManaGain, shaman.waterShieldManaMetrics)
						}
					}
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
