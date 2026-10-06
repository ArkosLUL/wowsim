package druid

import (
	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/stats"
	"math"
)

type DruidForm uint8

const (
	Humanoid DruidForm = 1 << iota
	Bear
	Cat
	Moonkin
	Tree
	Any = Humanoid | Bear | Cat | Moonkin | Tree
)

// Converts from 0.009327 to 0.0085
const AnimalSpiritRegenSuppression = 0.911337

func (form DruidForm) Matches(other DruidForm) bool {
	return (form & other) != 0
}

func (druid *Druid) GetForm() DruidForm {
	return druid.form
}

func (druid *Druid) InForm(form DruidForm) bool {
	return druid.form.Matches(form)
}

func (druid *Druid) ClearForm(sim *core.Simulation) {
	if druid.InForm(Cat) {
		druid.CatFormAura.Deactivate(sim)
	} else if druid.InForm(Bear) {
		druid.BearFormAura.Deactivate(sim)
	} else if druid.InForm(Moonkin) {
		panic("cant clear moonkin form")
	}
	druid.form = Humanoid
	druid.SetCurrentPowerBar(core.ManaBar)
}

func (druid *Druid) GetCatWeapon() core.Weapon {
	return core.Weapon{
		BaseDamageMin:        druid.formWeaponDamage(0.85, 1.0),
		BaseDamageMax:        druid.formWeaponDamage(1.25, 1.0),
		SwingSpeed:           1.0,
		NormalizedSwingSpeed: 1.0,
		CritMultiplier:       druid.whiteCritMultiplier(Cat),
		AttackPowerPerDPS:    core.DefaultAttackPowerPerDPS,
	}
}

func (druid *Druid) GetBearWeapon() core.Weapon {
	return core.Weapon{
		BaseDamageMin:        druid.formWeaponDamage(0.85, 2.5),
		BaseDamageMax:        druid.formWeaponDamage(1.25, 2.5),
		SwingSpeed:           2.5,
		NormalizedSwingSpeed: 2.5,
		CritMultiplier:       druid.whiteCritMultiplier(Bear),
		AttackPowerPerDPS:    core.DefaultAttackPowerPerDPS,
	}
}

// formWeaponDamage is a form's weapon damage end: min(level, 60) x pct x the form's attack speed
// (Player::CalculateMinMaxDamage, in float32).
func (druid *Druid) formWeaponDamage(pct float32, speed float32) float64 {
	return float64(float32(min(druid.Level, 60)) * pct * speed)
}

// weaponFeralAP is the attack power the server's druids get from a weapon's dps in cat, bear and moonkin form:
// int32(dps x 14) - 767, at least 0, with getDPS in float32 (ItemTemplate::getFeralBonus).
func (druid *Druid) weaponFeralAP(weapon *core.Item) float64 {
	delayMs := float32(math.Round(weapon.SwingSpeed * 1000))
	dps := float32(weapon.WeaponDamageMin+weapon.WeaponDamageMax) * 500 / delayMs
	return float64(max(0, int32((float32(druid.PseudoStats.BonusMHDps)+dps)*14)-767))
}

// Bonus stats for both cat and bear.
func (druid *Druid) GetFormShiftStats() stats.Stats {
	s := stats.Stats{
		stats.AttackPower: float64(druid.Talents.PredatoryStrikes) * 0.5 * float64(druid.Level),
		stats.MeleeCrit:   float64(druid.Talents.SharpenedClaws) * 2 * core.CritRatingPerCritChance,
		// ranged AP is 0 in both forms, not agility - 10
		stats.RangedAttackPower: 10,
	}

	if weapon := druid.GetMHWeapon(); weapon != nil {
		fap := druid.weaponFeralAP(weapon)
		s[stats.AttackPower] += fap
		// Predatory Strikes' 7/14/20% reads the weapon's template only: its feral AP and attack power, never its
		// enchant (Player::UpdateAttackPowerAndDamage)
		predatoryStrikesPct := []float64{0, 0.07, 0.14, 0.20}[druid.Talents.PredatoryStrikes]
		s[stats.AttackPower] += (fap + weapon.Stats[stats.AttackPower]) * predatoryStrikesPct
	}

	return s
}

func (druid *Druid) registerCatFormSpell() {
	actionID := core.ActionID{SpellID: 768}

	srm := druid.getSavageRoarMultiplier()

	statBonus := druid.GetFormShiftStats().Add(stats.Stats{
		stats.AttackPower: float64(druid.Level) * 2,
		stats.MeleeCrit:   2 * float64(druid.Talents.MasterShapeshifter) * core.CritRatingPerCritChance,
	})

	agiApDep := druid.NewDynamicStatDependency(stats.Agility, stats.AttackPower, 1)
	agiRapDep := druid.NewDynamicStatDependency(stats.Agility, stats.RangedAttackPower, -1)

	var hotwDep *stats.StatDependency
	if druid.Talents.HeartOfTheWild > 0 {
		hotwDep = druid.NewDynamicMultiplyStat(stats.AttackPower, 1.0+0.02*float64(druid.Talents.HeartOfTheWild))
	}

	clawWeapon := druid.GetCatWeapon()

	druid.CatFormAura = druid.RegisterAura(core.Aura{
		Label:      "Cat Form",
		ActionID:   actionID,
		Duration:   core.NeverExpires,
		BuildPhase: core.Ternary(druid.StartingForm.Matches(Cat), core.CharacterBuildPhaseBase, core.CharacterBuildPhaseNone),
		OnGain: func(aura *core.Aura, sim *core.Simulation) {
			if !druid.Env.MeasuringStats && druid.form != Humanoid {
				druid.ClearForm(sim)
			}
			druid.form = Cat
			druid.SetCurrentPowerBar(core.EnergyBar)

			druid.AutoAttacks.SetMH(clawWeapon)

			druid.PseudoStats.ThreatMultiplier *= 0.71
			druid.PseudoStats.SpiritRegenMultiplier *= AnimalSpiritRegenSuppression
			druid.PseudoStats.BaseDodge += 0.02 * float64(druid.Talents.FeralSwiftness)

			druid.AddStatsDynamic(sim, statBonus)
			druid.EnableDynamicStatDep(sim, agiApDep)
			druid.EnableDynamicStatDep(sim, agiRapDep)
			if hotwDep != nil {
				druid.EnableDynamicStatDep(sim, hotwDep)
			}

			if !druid.Env.MeasuringStats {
				druid.AutoAttacks.SetReplaceMHSwing(nil)
				druid.AutoAttacks.EnableAutoSwing(sim)
				druid.manageCooldownsEnabled()
				druid.UpdateManaRegenRates()

				// These buffs stay up, but corresponding changes don't
				if druid.SavageRoarAura.IsActive() {
					druid.PseudoStats.SchoolDamageDealtMultiplier[stats.SchoolIndexPhysical] *= srm
				}

				if druid.PredatoryInstinctsAura != nil {
					druid.PredatoryInstinctsAura.Activate(sim)
				}
			}
		},
		OnExpire: func(aura *core.Aura, sim *core.Simulation) {
			druid.form = Humanoid

			druid.AutoAttacks.SetMH(druid.WeaponFromMainHand(druid.MeleeCritMultiplier(Humanoid)))

			druid.PseudoStats.ThreatMultiplier /= 0.71
			druid.PseudoStats.SpiritRegenMultiplier /= AnimalSpiritRegenSuppression
			druid.PseudoStats.BaseDodge -= 0.02 * float64(druid.Talents.FeralSwiftness)

			druid.AddStatsDynamic(sim, statBonus.Invert())
			druid.DisableDynamicStatDep(sim, agiApDep)
			druid.DisableDynamicStatDep(sim, agiRapDep)
			if hotwDep != nil {
				druid.DisableDynamicStatDep(sim, hotwDep)
			}

			if !druid.Env.MeasuringStats {
				druid.AutoAttacks.SetReplaceMHSwing(nil)
				druid.AutoAttacks.EnableAutoSwing(sim)
				druid.manageCooldownsEnabled()
				druid.UpdateManaRegenRates()

				druid.TigersFuryAura.Deactivate(sim)

				// These buffs stay up, but corresponding changes don't
				if druid.SavageRoarAura.IsActive() {
					druid.PseudoStats.SchoolDamageDealtMultiplier[stats.SchoolIndexPhysical] /= srm
				}

				if druid.PredatoryInstinctsAura != nil {
					druid.PredatoryInstinctsAura.Deactivate(sim)
				}
			}
		},
	})

	energyMetrics := druid.NewEnergyMetrics(actionID)

	druid.CatForm = druid.RegisterSpell(Any, core.SpellConfig{
		ActionID: actionID,
		Flags:    core.SpellFlagNoOnCastComplete | core.SpellFlagAPL,

		ManaCost: core.ManaCostOptions{
			BaseCost:   0.35,
			Multiplier: 1 - 0.2*float64(druid.Talents.KingOfTheJungle) - 0.1*float64(druid.Talents.NaturalShapeshifter),
		},
		Cast: core.CastConfig{
			DefaultCast: core.Cast{
				GCD: core.GCDDefault,
			},
			IgnoreHaste: true,
		},

		ApplyEffects: func(sim *core.Simulation, _ *core.Unit, spell *core.Spell) {
			maxShiftEnergy := float64(20 * druid.Talents.Furor)

			energyDelta := maxShiftEnergy - druid.CurrentEnergy()

			if energyDelta < 0 {
				druid.SpendEnergy(sim, -energyDelta, energyMetrics)
			}
			druid.CatFormAura.Activate(sim)
		},
	})
}

func (druid *Druid) registerBearFormSpell() {
	actionID := core.ActionID{SpellID: 9634}
	healthMetrics := druid.NewHealthMetrics(actionID)

	statBonus := druid.GetFormShiftStats().Add(stats.Stats{
		stats.AttackPower: 3 * float64(druid.Level),
	})

	stamDep := druid.NewDynamicMultiplyStat(stats.Stamina, 1.25)
	agiRapDep := druid.NewDynamicStatDependency(stats.Agility, stats.RangedAttackPower, -1)

	var potpDep *stats.StatDependency
	if druid.Talents.ProtectorOfThePack > 0 {
		potpDep = druid.NewDynamicMultiplyStat(stats.AttackPower, 1.0+0.02*float64(druid.Talents.ProtectorOfThePack))
	}

	var hotwDep *stats.StatDependency
	if druid.Talents.HeartOfTheWild > 0 {
		hotwDep = druid.NewDynamicMultiplyStat(stats.Stamina, 1.0+0.02*float64(druid.Talents.HeartOfTheWild))
	}

	potpdtm := 1 - 0.04*float64(druid.Talents.ProtectorOfThePack)
	// Bear Form (Passive2) 21178's +45% and Tank Class Passive Threat 57339's +43%
	const bearThreat = 1.45 * 1.43

	clawWeapon := druid.GetBearWeapon()

	druid.BearFormAura = druid.RegisterAura(core.Aura{
		Label:      "Bear Form",
		ActionID:   actionID,
		Duration:   core.NeverExpires,
		BuildPhase: core.Ternary(druid.StartingForm.Matches(Bear), core.CharacterBuildPhaseBase, core.CharacterBuildPhaseNone),
		OnGain: func(aura *core.Aura, sim *core.Simulation) {
			if !druid.Env.MeasuringStats && druid.form != Humanoid {
				druid.ClearForm(sim)
			}
			druid.form = Bear
			druid.SetCurrentPowerBar(core.RageBar)

			druid.AutoAttacks.SetMH(clawWeapon)

			druid.PseudoStats.ThreatMultiplier *= bearThreat
			druid.PseudoStats.SchoolDamageDealtMultiplier[stats.SchoolIndexPhysical] *= 1.0 + 0.02*float64(druid.Talents.MasterShapeshifter)
			druid.PseudoStats.DamageTakenMultiplier *= potpdtm
			druid.PseudoStats.SpiritRegenMultiplier *= AnimalSpiritRegenSuppression
			druid.PseudoStats.BaseDodge += 0.02 * float64(druid.Talents.FeralSwiftness+druid.Talents.NaturalReaction)

			druid.AddStatsDynamic(sim, statBonus)
			druid.ApplyDynamicEquipScaling(sim, stats.Armor, druid.BearArmorMultiplier())
			if potpDep != nil {
				druid.EnableDynamicStatDep(sim, potpDep)
			}

			// Preserve fraction of max health when shifting
			healthFrac := druid.CurrentHealth() / druid.MaxHealth()
			druid.EnableDynamicStatDep(sim, agiRapDep)
			druid.EnableDynamicStatDep(sim, stamDep)
			if hotwDep != nil {
				druid.EnableDynamicStatDep(sim, hotwDep)
			}
			druid.GainHealth(sim, healthFrac*druid.MaxHealth()-druid.CurrentHealth(), healthMetrics)

			if !druid.Env.MeasuringStats {
				druid.AutoAttacks.SetReplaceMHSwing(druid.ReplaceBearMHFunc)
				druid.AutoAttacks.EnableAutoSwing(sim)

				druid.manageCooldownsEnabled()
				druid.UpdateManaRegenRates()
			}
		},
		OnExpire: func(aura *core.Aura, sim *core.Simulation) {
			druid.form = Humanoid
			druid.AutoAttacks.SetMH(druid.WeaponFromMainHand(druid.MeleeCritMultiplier(Humanoid)))

			druid.PseudoStats.ThreatMultiplier /= bearThreat
			druid.PseudoStats.SchoolDamageDealtMultiplier[stats.SchoolIndexPhysical] /= 1.0 + 0.02*float64(druid.Talents.MasterShapeshifter)
			druid.PseudoStats.DamageTakenMultiplier /= potpdtm
			druid.PseudoStats.SpiritRegenMultiplier /= AnimalSpiritRegenSuppression
			druid.PseudoStats.BaseDodge -= 0.02 * float64(druid.Talents.FeralSwiftness+druid.Talents.NaturalReaction)

			druid.AddStatsDynamic(sim, statBonus.Invert())
			druid.RemoveDynamicEquipScaling(sim, stats.Armor, druid.BearArmorMultiplier())
			if potpDep != nil {
				druid.DisableDynamicStatDep(sim, potpDep)
			}

			healthFrac := druid.CurrentHealth() / druid.MaxHealth()
			druid.DisableDynamicStatDep(sim, agiRapDep)
			druid.DisableDynamicStatDep(sim, stamDep)
			if hotwDep != nil {
				druid.DisableDynamicStatDep(sim, hotwDep)
			}
			druid.RemoveHealth(sim, druid.CurrentHealth()-healthFrac*druid.MaxHealth())

			if !druid.Env.MeasuringStats {
				druid.AutoAttacks.SetReplaceMHSwing(nil)
				druid.AutoAttacks.EnableAutoSwing(sim)

				druid.manageCooldownsEnabled()
				druid.UpdateManaRegenRates()
				druid.EnrageAura.Deactivate(sim)
				druid.MaulQueueAura.Deactivate(sim)
			}
		},
	})

	rageMetrics := druid.NewRageMetrics(actionID)

	furorProcChance := []float64{0, 0.2, 0.4, 0.6, 0.8, 1}[druid.Talents.Furor]

	druid.BearForm = druid.RegisterSpell(Any, core.SpellConfig{
		ActionID: actionID,
		Flags:    core.SpellFlagNoOnCastComplete | core.SpellFlagAPL,

		ManaCost: core.ManaCostOptions{
			BaseCost:   0.35,
			Multiplier: 1 - 0.2*float64(druid.Talents.KingOfTheJungle) - 0.1*float64(druid.Talents.NaturalShapeshifter),
		},
		Cast: core.CastConfig{
			DefaultCast: core.Cast{
				GCD: core.GCDDefault,
			},
			IgnoreHaste: true,
		},

		ApplyEffects: func(sim *core.Simulation, _ *core.Unit, spell *core.Spell) {
			rageDelta := 0 - druid.CurrentRage()
			if sim.Proc(furorProcChance, "Furor") {
				rageDelta += 10
			}
			if rageDelta > 0 {
				druid.AddRage(sim, rageDelta, rageMetrics)
			} else if rageDelta < 0 {
				druid.SpendRage(sim, -rageDelta, rageMetrics)
			}
			druid.BearFormAura.Activate(sim)
		},
	})
}

func (druid *Druid) manageCooldownsEnabled() {
	// Disable cooldowns not usable in form and/or delay others
	if druid.StartingForm.Matches(Cat | Bear) {
		for _, mcd := range druid.disabledMCDs {
			mcd.Enable()
		}
		druid.disabledMCDs = nil

		if druid.InForm(Humanoid) {
			// Disable cooldown that incurs a gcd, so we dont get stuck out of form when we dont need to (Greater Drums)
			for _, mcd := range druid.GetMajorCooldowns() {
				if mcd.Spell.DefaultCast.GCD > 0 {
					mcd.Disable()
					druid.disabledMCDs = append(druid.disabledMCDs, mcd)
				}
			}
		}
	}
}

func (druid *Druid) applyMoonkinForm() {
	if !druid.InForm(Moonkin) || !druid.Talents.MoonkinForm {
		return
	}

	druid.MultiplyStat(stats.Intellect, 1+(0.02*float64(druid.Talents.Furor)))
	// the form's passive (24905) adds 370% armor from items, and the server gives it the weapon's feral attack power
	druid.ApplyEquipScaling(stats.Armor, 4.7)
	if weapon := druid.GetMHWeapon(); weapon != nil {
		druid.AddStat(stats.AttackPower, druid.weaponFeralAP(weapon))
	}
	druid.PseudoStats.DamageDealtMultiplier *= 1 + (float64(druid.Talents.MasterShapeshifter) * 0.02)
	if druid.Talents.ImprovedMoonkinForm > 0 {
		druid.AddStatDependency(stats.Spirit, stats.SpellPower, 0.1*float64(druid.Talents.ImprovedMoonkinForm))
	}

	manaMetrics := druid.NewManaMetrics(core.ActionID{SpellID: 24858})
	druid.RegisterAura(core.Aura{
		Label:    "Moonkin Form",
		Duration: core.NeverExpires,
		OnReset: func(aura *core.Aura, sim *core.Simulation) {
			aura.Activate(sim)
		},
		OnSpellHitDealt: func(aura *core.Aura, sim *core.Simulation, spell *core.Spell, result *core.SpellResult) {
			if result.DidCrit() {
				if druid.Moonfire.IsEqual(spell) || druid.Starfire.IsEqual(spell) || druid.Wrath.IsEqual(spell) {
					druid.AddMana(sim, 0.02*druid.MaxMana(), manaMetrics)
				}
			}
		},
	})
}
