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
		shaman.AddMana(sim, shaman.waterShieldOrbMana, shaman.waterShieldManaMetrics)
	}
}

func (shaman *Shaman) registerLesserHealingWaveSpell() {
	impShieldChance := shaman.impWaterShieldChance(60)

	hasGlyph := shaman.HasMajorGlyph(proto.ShamanMajorGlyph_GlyphOfLesserHealingWave)

	// The Totems of the Third Wind add healing power (class script 3736), which the coefficient scales
	relicHealingPower := 0 +
		core.TernaryFloat64(shaman.Ranged().ID == 51501, 459, 0) +
		core.TernaryFloat64(shaman.Ranged().ID == 42599, 404, 0) +
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
			if hasGlyph && shaman.EarthShield != nil && shaman.EarthShield.Hot(target).IsActive() {
				baseHealing *= 1.2
			}
			result := spell.CalcAndDealHealing(sim, target, baseHealing, spell.OutcomeHealingCrit)

			if result.Outcome.Matches(core.OutcomeCrit) {
				shaman.procImpWaterShield(sim, impShieldChance)
				shaman.procAncestralAwakening(sim, result)
			}

			if shaman.tidalWaveProc != nil && shaman.tidalWaveProc.IsActive() {
				shaman.tidalWaveProc.RemoveStack(sim)
			}
		},
	})
}

func (shaman *Shaman) registerRiptideSpell() {
	if !shaman.Talents.Riptide {
		return
	}
	impShieldChance := shaman.impWaterShieldChance(100)

	// the glyph adds 6 s to the 15 s hot, 2 more ticks
	extraTicks := core.TernaryInt32(shaman.HasMajorGlyph(proto.ShamanMajorGlyph_GlyphOfRiptide), 2, 0)
	// T8's 2 piece takes 1 s off the cooldown
	cooldown := time.Second * time.Duration(core.TernaryInt32(shaman.HasSetBonus(ItemSetWorldbreakerRegalia, 2), 5, 6))
	// T9's 2 piece is 20% more healing, direct and periodic, a spell mod that multiplies with Purification
	setHealing := core.TernaryFloat64(shaman.HasSetBonus(ItemSetThrallsGarb, 2), .2, 0)

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
				Duration: cooldown,
			},
		},

		BonusCritRating:  float64(shaman.Talents.TidalMastery) * 1 * core.CritRatingPerCritChance,
		DamageMultiplier: spellModDamage(.02*float64(shaman.Talents.Purification), setHealing),
		CritMultiplier:   shaman.DefaultHealingCritMultiplier(),
		ThreatMultiplier: 1 - (float64(shaman.Talents.HealingGrace) * 0.05),

		Direct: core.SpellEffect{Effect: 0, Min: 1604, Max: 1736, SP: 0.402},
		Hot: core.DotConfig{
			Aura: core.Aura{
				Label: "Riptide",
			},
			NumberOfTicks: 5 + extraTicks,
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
	impShieldChance := shaman.impWaterShieldChance(100)

	// Totem of Misery takes 79 off the base cost
	manaDiscount := core.TernaryFloat64(shaman.Ranged().ID == 39728, 79, 0)
	// Healing Way (+8, 16 or 25%) and T7's 4 piece (+5%) are damage spell mods
	healingWay := []float64{0, .08, .16, .25}[shaman.Talents.HealingWay]
	setHealing := core.TernaryFloat64(shaman.HasSetBonus(ItemSetEarthshatterRegalia, 4), .05, 0)
	glyphHeal := shaman.HasMajorGlyph(proto.ShamanMajorGlyph_GlyphOfHealingWave)
	if glyphHeal {
		shaman.registerGlyphOfHealingWaveHeal()
	}

	shaman.HealingWave = shaman.RegisterSpell(core.SpellConfig{
		ActionID:    core.ActionID{SpellID: 49273},
		SpellSchool: core.SpellSchoolNature,
		ProcMask:    core.ProcMaskSpellHealing,
		Flags:       core.SpellFlagHelpful | core.SpellFlagAPL,

		ManaCost: core.ManaCostOptions{
			FlatCost:   0.25*shaman.BaseMana - manaDiscount,
			Multiplier: 1 - .01*float64(shaman.Talents.TidalFocus),
		},
		Cast: core.CastConfig{
			DefaultCast: core.Cast{
				GCD:      core.GCDDefault,
				CastTime: time.Millisecond * time.Duration(3000-100*shaman.Talents.ImprovedHealingWave),
			},
		},

		BonusCritRating:  float64(shaman.Talents.TidalMastery) * 1 * core.CritRatingPerCritChance,
		DamageMultiplier: spellModDamage(.02*float64(shaman.Talents.Purification), healingWay, setHealing),
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
			if glyphHeal && target != &shaman.Unit {
				shaman.healFromGlyphOfHealingWave(sim, result.Damage)
			}

			if shaman.tidalWaveProc != nil && shaman.tidalWaveProc.IsActive() {
				shaman.tidalWaveProc.RemoveStack(sim)
			}
		},
	})
}

// earthShieldHits is what one Earth Shield target takes: the dummies' hits, or without incoming damage a stand-in
// at the page's rate. 379's 3.5 s cooldown (on the target) lets one hit through at a time.
type earthShieldHits struct {
	standIn *core.HitTakenStandIn
	readyAt time.Duration
}

func (shaman *Shaman) earthShieldHitsOn(target *core.Unit) *earthShieldHits {
	if hits := shaman.earthShieldTargets[target]; hits != nil {
		return hits
	}
	if shaman.earthShieldTargets == nil {
		shaman.earthShieldTargets = make(map[*core.Unit]*earthShieldHits)
	}
	hits := &earthShieldHits{}
	hits.standIn = core.NewHitTakenStandIn(float64(shaman.SelfBuffs.EarthShieldPPM), func(sim *core.Simulation) {
		shaman.earthShieldHit(sim, target, hits)
	})
	shaman.earthShieldTargets[target] = hits
	return hits
}

// earthShieldHit spends one charge on a hit the target took.
func (shaman *Shaman) earthShieldHit(sim *core.Simulation, target *core.Unit, hits *earthShieldHits) {
	if sim.CurrentTime < hits.readyAt {
		return
	}
	hot := shaman.EarthShield.Hot(target)
	if !hot.IsActive() {
		return
	}
	hits.readyAt = sim.CurrentTime + earthShieldCooldown
	hot.ManualTick(sim)
	// the charge's heal is 379, a cast of its own on the server and not a tick
	shaman.EarthShield.LandOn(sim, target)
}

const earthShieldCooldown = time.Millisecond * 3500

func (shaman *Shaman) registerEarthShieldSpell() {
	if !shaman.Talents.EarthShield {
		return
	}
	actionID := core.ActionID{SpellID: 49284}

	// spell_sha_earth_shield adds the glyph's 20% to the whole charge, then Improved Shields' percent
	// once more to what the bonuses added over the base value
	glyphBonus := core.TernaryFloat64(shaman.HasMajorGlyph(proto.ShamanMajorGlyph_GlyphOfEarthShield), 0.2, 0)
	improvedShields := 0.05 * float64(shaman.Talents.ImprovedShields)
	// 49284's healing bonus puts Purification in the charge before the glyph, in float32 like
	// GetTotalAuraMultiplier, and 379 takes no caster mods of its own
	purification := 1 + float32(2*shaman.Talents.Purification)/100

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
				OnGain: func(aura *core.Aura, sim *core.Simulation) {
					hits := shaman.earthShieldHitsOn(aura.Unit)
					if !shaman.Env.Raid.HasIncomingDamage() {
						hits.standIn.Start(sim)
					}
				},
				OnSpellHitTaken: func(aura *core.Aura, sim *core.Simulation, _ *core.Spell, result *core.SpellResult) {
					if result.Damage > 0 && shaman.Env.Raid.HasIncomingDamage() {
						shaman.earthShieldHit(sim, aura.Unit, shaman.earthShieldHitsOn(aura.Unit))
					}
				},
				OnExpire: func(aura *core.Aura, sim *core.Simulation) {
					shaman.earthShieldHitsOn(aura.Unit).standIn.Stop(sim)
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
