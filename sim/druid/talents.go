package druid

import (
	"math"
	"slices"
	"time"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
	"github.com/wowsims/wotlk/sim/core/serverdata"
	"github.com/wowsims/wotlk/sim/core/stats"
)

func (druid *Druid) ThickHideMultiplier() float64 {
	thickHideMulti := 1.0

	if druid.Talents.ThickHide > 0 {
		thickHideMulti += 0.04 + 0.03*float64(druid.Talents.ThickHide-1)
	}

	return thickHideMulti
}

func (druid *Druid) BearArmorMultiplier() float64 {
	sotfMulti := 1.0 + 0.33/3.0*float64(druid.Talents.SurvivalOfTheFittest)
	return 4.7 * sotfMulti
}

func (druid *Druid) ApplyTalents() {
	druid.AddStat(stats.SpellHit, float64(druid.Talents.BalanceOfPower)*2*core.SpellHitRatingPerHitChance)
	druid.AddStat(stats.SpellCrit, float64(druid.Talents.NaturalPerfection)*1*core.CritRatingPerCritChance)
	druid.PseudoStats.CastSpeedMultiplier *= 1 + (float64(druid.Talents.CelestialFocus) * 0.01)
	druid.PseudoStats.DamageDealtMultiplier *= 1 + (float64(druid.Talents.EarthAndMoon) * 0.02)
	druid.PseudoStats.SpiritRegenRateCasting = float64(druid.Talents.Intensity) * (0.5 / 3)
	druid.PseudoStats.SchoolDamageDealtMultiplier[stats.SchoolIndexPhysical] *= 1 + 0.02*float64(druid.Talents.Naturalist)
	druid.ApplyEquipScaling(stats.Armor, druid.ThickHideMultiplier())

	if druid.Talents.LunarGuidance > 0 {
		bonus := 0.04 * float64(druid.Talents.LunarGuidance)
		druid.AddStatDependency(stats.Intellect, stats.SpellPower, bonus)
	}

	if druid.Talents.Dreamstate > 0 {
		bonus := 0.04 * float64(druid.Talents.Dreamstate)
		druid.AddStatDependency(stats.Intellect, stats.MP5, bonus)
	}

	if druid.Talents.HeartOfTheWild > 0 {
		bonus := 0.04 * float64(druid.Talents.HeartOfTheWild)
		druid.MultiplyStat(stats.Intellect, 1.0+bonus)
	}

	if druid.Talents.ImprovedFaerieFire > 0 && druid.CurrentTarget.HasAuraWithTag(core.FaerieFireAuraTag) {
		druid.AddStat(stats.SpellCrit, float64(druid.Talents.ImprovedFaerieFire)*1*core.CritRatingPerCritChance)
	}

	if druid.Talents.SurvivalOfTheFittest > 0 {
		bonus := 0.02 * float64(druid.Talents.SurvivalOfTheFittest)
		druid.MultiplyStat(stats.Stamina, 1.0+bonus)
		druid.MultiplyStat(stats.Strength, 1.0+bonus)
		druid.MultiplyStat(stats.Agility, 1.0+bonus)
		druid.MultiplyStat(stats.Intellect, 1.0+bonus)
		druid.MultiplyStat(stats.Spirit, 1.0+bonus)
		druid.PseudoStats.ReducedCritTakenChance += 0.02 * float64(druid.Talents.SurvivalOfTheFittest)
	}

	if druid.Talents.ImprovedMarkOfTheWild > 0 {
		bonus := 0.01 * float64(druid.Talents.ImprovedMarkOfTheWild)
		druid.MultiplyStat(stats.Stamina, 1.0+bonus)
		druid.MultiplyStat(stats.Strength, 1.0+bonus)
		druid.MultiplyStat(stats.Agility, 1.0+bonus)
		druid.MultiplyStat(stats.Intellect, 1.0+bonus)
		druid.MultiplyStat(stats.Spirit, 1.0+bonus)
	}

	if druid.Talents.PrimalPrecision > 0 {
		druid.AddStat(stats.Expertise, 5.0*float64(druid.Talents.PrimalPrecision)*core.ExpertisePerQuarterPercentReduction)
	}

	if druid.Talents.LivingSpirit > 0 {
		bonus := 0.05 * float64(druid.Talents.LivingSpirit)
		druid.MultiplyStat(stats.Spirit, 1.0+bonus)
	}

	druid.setupNaturesGrace()
	druid.registerNaturesSwiftnessCD()
	druid.applyEarthAndMoon()
	druid.applyMoonkinForm()
	druid.applyPrimalFury()
	druid.applyOmenOfClarity()
	druid.applyEclipse()
	druid.applyImprovedLotp()
	druid.applyPredatoryInstincts()
	druid.applyNaturalReaction()
	druid.applyOwlkinFrenzy()
	druid.applyInfectedWounds()
}

func (druid *Druid) setupNaturesGrace() {
	if druid.Talents.NaturesGrace == 0 {
		return
	}

	druid.NaturesGraceProcAura = druid.RegisterAura(core.Aura{
		Label:    "Natures Grace Proc",
		ActionID: core.ActionID{SpellID: 16886},
		Duration: time.Second * 3,
		OnGain: func(aura *core.Aura, sim *core.Simulation) {
			druid.MultiplyCastSpeed(1.2)
		},
		OnExpire: func(aura *core.Aura, sim *core.Simulation) {
			druid.MultiplyCastSpeed(1 / 1.2)
		},
	})

	druid.RegisterAura(core.Aura{
		Label:    "Natures Grace",
		Duration: core.NeverExpires,
		OnReset: func(aura *core.Aura, sim *core.Simulation) {
			aura.Activate(sim)
		},
		OnSpellHitDealt: func(aura *core.Aura, sim *core.Simulation, spell *core.Spell, result *core.SpellResult) {
			if !result.Outcome.Matches(core.OutcomeCrit) || !spell.Flags.Matches(SpellFlagNaturesGrace) {
				return
			}
			// the cast phase: a missile had it at its launch, another spell the druid casts has it here. A
			// triggered spell has none (Starfall's stars)
			if spell.MissileSpeed == 0 && spell.Flags.Matches(core.SpellFlagAPL) {
				druid.rollNaturesGrace(sim)
			}
			druid.rollNaturesGrace(sim)
		},
	})
}

// rollNaturesGrace is one phase's roll for a crit. spell_proc -16880 takes the cast phase and the hit phase,
// each rolling the talent's chance: live, all 171 crit Wraths of a fight gave 16886 at the launch and again at
// the hit.
func (druid *Druid) rollNaturesGrace(sim *core.Simulation) {
	if druid.NaturesGraceProcAura == nil {
		return
	}
	if sim.Proc([]float64{0, .33, .66, 1}[druid.Talents.NaturesGrace], "Natures Grace") {
		druid.NaturesGraceProcAura.Activate(sim)
	}
}

func (druid *Druid) registerNaturesSwiftnessCD() {
	if !druid.Talents.NaturesSwiftness {
		return
	}
	actionID := core.ActionID{SpellID: 17116}

	var nsAura *core.Aura
	// every spell with a cast time the server's aura covers, heals and Rebirth included
	var instant []*core.Spell
	nsSpell := druid.RegisterSpell(Humanoid|Moonkin|Tree, core.SpellConfig{
		ActionID: actionID,
		Flags:    core.SpellFlagNoOnCastComplete,
		Cast: core.CastConfig{
			CD: core.Cooldown{
				Timer:    druid.NewTimer(),
				Duration: time.Minute * 3,
			},
		},
		ApplyEffects: func(sim *core.Simulation, _ *core.Unit, _ *core.Spell) {
			nsAura.Activate(sim)
		},
	})

	nsAura = druid.RegisterAura(core.Aura{
		Label:    "Natures Swiftness",
		ActionID: actionID,
		Duration: core.NeverExpires,
		OnGain: func(aura *core.Aura, sim *core.Simulation) {
			instant = druid.NaturesSwiftnessSpells(actionID.SpellID)
			for _, spell := range instant {
				spell.CastTimeMultiplier -= 1
			}
		},
		OnExpire: func(aura *core.Aura, sim *core.Simulation) {
			for _, spell := range instant {
				spell.CastTimeMultiplier += 1
			}
		},
		OnCastComplete: func(aura *core.Aura, sim *core.Simulation, spell *core.Spell) {
			if !slices.Contains(instant, spell) {
				return
			}

			// Remove the buff and put skill on CD
			aura.Deactivate(sim)
			nsSpell.CD.Use(sim)
			druid.UpdateMajorCooldowns()
		},
	})

	druid.AddMajorCooldown(core.MajorCooldown{
		Spell: nsSpell.Spell,
		Type:  core.CooldownTypeDPS,
		ShouldActivate: func(sim *core.Simulation, character *core.Character) bool {
			// Don't use NS unless we're casting a full-length wrath.
			return !character.HasTemporarySpellCastSpeedIncrease()
		},
	})
}

func (druid *Druid) applyEarthAndMoon() {
	if druid.Talents.EarthAndMoon == 0 {
		return
	}

	eamAuras := druid.NewEnemyAuraArray(func(target *core.Unit) *core.Aura {
		return core.EarthAndMoonAura(target, druid.Talents.EarthAndMoon)
	})
	druid.Env.RegisterPreFinalizeEffect(func() {
		if druid.Starfire != nil {
			druid.Starfire.RelatedAuras = append(druid.Starfire.RelatedAuras, eamAuras)
		}
		if druid.Wrath != nil {
			druid.Wrath.RelatedAuras = append(druid.Wrath.RelatedAuras, eamAuras)
		}
	})

	earthAndMoonSpell := druid.RegisterSpell(Any, core.SpellConfig{
		ActionID: core.ActionID{SpellID: 60432},
		ProcMask: core.ProcMaskSuppressedProc,
		Flags:    core.SpellFlagNoLogs,
		ApplyEffects: func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
			eamAuras.Get(target).Activate(sim)
		},
	})

	druid.RegisterAura(core.Aura{
		Label:    "Earth And Moon Talent",
		Duration: core.NeverExpires,
		OnReset: func(aura *core.Aura, sim *core.Simulation) {
			aura.Activate(sim)
		},
		OnSpellHitDealt: func(aura *core.Aura, sim *core.Simulation, spell *core.Spell, result *core.SpellResult) {
			if result.Landed() && (druid.Starfire.IsEqual(spell) || druid.Wrath.IsEqual(spell)) {
				earthAndMoonSpell.Cast(sim, result.Target)
			}
		},
	})
}

func (druid *Druid) applyPrimalFury() {
	if druid.Talents.PrimalFury == 0 {
		return
	}

	procChance := []float64{0, 0.5, 1}[druid.Talents.PrimalFury]
	actionID := core.ActionID{SpellID: 37117}
	rageMetrics := druid.NewRageMetrics(actionID).EnergizeThreat(core.EnergizeBySpell)
	cpMetrics := druid.NewComboPointMetrics(actionID)

	druid.RegisterAura(core.Aura{
		Label:    "Primal Fury",
		Duration: core.NeverExpires,
		OnReset: func(aura *core.Aura, sim *core.Simulation) {
			aura.Activate(sim)
		},
		OnSpellHitDealt: func(aura *core.Aura, sim *core.Simulation, spell *core.Spell, result *core.SpellResult) {
			if druid.InForm(Bear) {
				if result.Outcome.Matches(core.OutcomeCrit) {
					if sim.Proc(procChance, "Primal Fury") {
						druid.AddRage(sim, 5, rageMetrics)
					}
				}
			} else if druid.InForm(Cat) {
				if druid.IsMangle(spell) || druid.Shred.IsEqual(spell) || druid.Rake.IsEqual(spell) {
					if result.Outcome.Matches(core.OutcomeCrit) {
						if sim.Proc(procChance, "Primal Fury") {
							druid.AddComboPoints(sim, 1, cpMetrics)
						}
					}
				}
			}
		},
	})
}

// Modifies the Bleed aura to apply the bonus.
func (druid *Druid) applyRendAndTear(aura core.Aura) core.Aura {
	if druid.FerociousBite == nil || druid.Talents.RendAndTear == 0 || druid.AssumeBleedActive {
		return aura
	}

	bonusCrit := 5.0 * float64(druid.Talents.RendAndTear) * core.CritRatingPerCritChance

	aura.ApplyOnGain(func(aura *core.Aura, sim *core.Simulation) {
		if druid.BleedsActive == 0 {
			druid.FerociousBite.BonusCritRating += bonusCrit
		}
		druid.BleedsActive++
	})
	aura.ApplyOnExpire(func(aura *core.Aura, sim *core.Simulation) {
		druid.BleedsActive--
		if druid.BleedsActive == 0 {
			druid.FerociousBite.BonusCritRating -= bonusCrit
		}
	})

	return aura
}

func (druid *Druid) applyOmenOfClarity() {
	// Feral 2p needs clearcasting aura
	if !druid.Talents.OmenOfClarity && !druid.HasSetBonus(ItemSetNightsongBattlegear, 2) {
		return
	}

	// T10-2P
	var lasherweave2P *core.Aura
	if druid.HasSetBonus(ItemSetLasherweaveRegalia, 2) {
		lasherweave2P = druid.RegisterAura(core.Aura{
			Label:    "T10-2P proc",
			ActionID: core.ActionID{SpellID: 70718},
			Duration: time.Second * 6,
			OnGain: func(aura *core.Aura, sim *core.Simulation) {
				druid.PseudoStats.SchoolDamageDealtMultiplier[stats.SchoolIndexArcane] *= 1.15
				druid.PseudoStats.SchoolDamageDealtMultiplier[stats.SchoolIndexNature] *= 1.15
			},
			OnExpire: func(aura *core.Aura, sim *core.Simulation) {
				druid.PseudoStats.SchoolDamageDealtMultiplier[stats.SchoolIndexArcane] /= 1.15
				druid.PseudoStats.SchoolDamageDealtMultiplier[stats.SchoolIndexNature] /= 1.15
			},
		})
	}

	var affectedSpells []*DruidSpell
	druid.ClearcastingAura = druid.RegisterAura(core.Aura{
		Label:    "Clearcasting",
		ActionID: core.ActionID{SpellID: 16870},
		Duration: time.Second * 15,
		OnInit: func(aura *core.Aura, sim *core.Simulation) {
			affectedSpells = core.FilterSlice([]*DruidSpell{
				// Balance
				druid.ForceOfNature,
				druid.Hurricane,
				druid.InsectSwarm,
				druid.Moonfire,
				// TODO druid.Starfall, not sure how the proc chance is affected.
				druid.Starfire,
				druid.Typhoon,
				druid.Wrath,

				// Feral
				druid.DemoralizingRoar,
				druid.FerociousBite,
				druid.Lacerate,
				druid.MangleBear,
				druid.MangleCat,
				druid.Maul,
				druid.Rake,
				druid.Rip,
				druid.Shred,
				druid.SwipeBear,
				druid.SwipeCat,
			}, func(spell *DruidSpell) bool { return spell != nil })
		},
		OnGain: func(aura *core.Aura, sim *core.Simulation) {
			for _, spell := range affectedSpells {
				spell.CostMultiplier -= 1
			}
		},
		OnExpire: func(aura *core.Aura, sim *core.Simulation) {
			for _, spell := range affectedSpells {
				spell.CostMultiplier += 1
			}
		},
		OnCastComplete: func(aura *core.Aura, sim *core.Simulation, spell *core.Spell) {
			if aura.RemainingDuration(sim) == aura.Duration {
				// OnCastComplete is called after OnSpellHitDealt / etc, so don't deactivate
				// if it was just activated.
				return
			}

			for _, as := range affectedSpells {
				if as.IsEqual(spell) {
					aura.Deactivate(sim)
					break
				}
			}
		},
	})

	if !druid.Talents.OmenOfClarity {
		return
	}

	druid.ProcOoc = func(sim *core.Simulation) {
		druid.ClearcastingAura.Activate(sim)
		if lasherweave2P != nil {
			lasherweave2P.Activate(sim)
		}
	}

	// Omen of Clarity's own spell_proc row (16864): 3.5 PPM, no fixed chance, rolled the same way as
	// any other proc-trigger aura (Aura::CalcProcChance / GetPPMProcChance).
	ppm := core.ServerProcFor(16864).PPM

	rollOmenOfClarity := func(sim *core.Simulation, spell *core.Spell) {
		chanceToProc := druid.AuraPPMProcChance(ppm, spell)
		if druid.GiftOfTheWild.IsEqual(spell) {
			// This cast stands in for the real spell's ~30 raid-wide targets, each rolling its
			// own chance (sim/druid/fake_gotw.go).
			chanceToProc = 1 - math.Pow(1-chanceToProc, 30)
		}
		if sim.RandomFloat("Clearcasting") < chanceToProc {
			druid.ProcOoc(sim)
		}
	}

	druid.RegisterAura(core.Aura{
		Label:    "Omen of Clarity",
		Duration: core.NeverExpires,
		OnReset: func(aura *core.Aura, sim *core.Simulation) {
			aura.Activate(sim)
		},
		OnSpellHitDealt: func(aura *core.Aura, sim *core.Simulation, spell *core.Spell, result *core.SpellResult) {
			if !result.Landed() {
				return
			}

			// spell_tweaks_omen_faerie_fire (SpellTweaks_classes.cpp) casts Clearcasting itself, no
			// roll, off Faerie Fire (Feral) landing on a plain NPC (not a player, pet or charm). Its
			// Bear Form hit lands in the same instant, so one Clearcasting covers both.
			faerieFireFeral := druid.FaerieFire.IsEqual(spell) && druid.InForm(Cat|Bear)
			if (faerieFireFeral || spell.ActionID.SameAction(FaerieFireFeralDamageActionID)) &&
				result.Target.Type == core.EnemyUnit && druid.Server().SpellTweaks.OmenClarityFaerieFire {
				if faerieFireFeral {
					druid.ClearcastingAura.Activate(sim)
				}
				// Omen's own roll still runs, and only it gives the Balance T10 2-piece proc
				// (spell_dru_omen_of_clarity::HandleProc)
				if lasherweave2P != nil && sim.RandomFloat("Clearcasting") < druid.AuraPPMProcChance(ppm, spell) {
					lasherweave2P.Activate(sim)
				}
				return
			}

			if !spell.ProcMask.Matches(core.ProcMaskMeleeWhiteHit) && !spell.Flags.Matches(SpellFlagOmenTrigger) &&
				!rollsOmenOffFamily(spell) {
				return
			}

			rollOmenOfClarity(sim, spell)
		},
	})
}

// SPELL_ATTR0_CU_NO_INITIAL_THREAT and SPELL_ATTR0_CU_DIRECT_DAMAGE, SPELL_EFFECT_ENERGIZE and
// SPELL_EFFECT_CREATE_ITEM
const (
	attrCuNoInitialThreat = 0x00000010
	attrCuDirectDamage    = 0x00000100
	effectCreateItem      = 24
	effectEnergize        = 30
)

var druidFamily = serverdata.ClassFamily(proto.Class_ClassDruid)

// rollsOmenOffFamily is whether a spell from outside the druid's family rolls Omen of Clarity when it lands,
// which an item's damage does: 16864 takes any magic-class spell, procs included (CAN_PROC_FROM_PROCS), and
// spell_dru_omen_of_clarity::CheckProc lets one through that deals direct damage or heals and gives no power.
// The chance is the PPM over the spell's own cast time entry, 2.2 s for Extract of Necromantic Power's bolt.
func rollsOmenOffFamily(spell *core.Spell) bool {
	ss := spell.ServerSpell()
	if ss == nil || ss.Family == druidFamily || ss.DmgClass != serverdata.DmgClassMagic ||
		ss.Flags&serverdata.FlagPassive != 0 || ss.AttributesCu&(attrCuDirectDamage|attrCuNoInitialThreat) == 0 {
		return false
	}
	for _, e := range ss.Effects {
		if e.Effect == effectEnergize || e.Effect == effectCreateItem {
			return false
		}
	}
	return true
}

func (druid *Druid) applyEclipse() {
	druid.SolarICD = core.Cooldown{Timer: druid.NewTimer(), Duration: 0}
	druid.LunarICD = core.Cooldown{Timer: druid.NewTimer(), Duration: 0}
	if druid.Talents.Eclipse == 0 {
		return
	}

	eclipseDuration := time.Millisecond * 15000

	// spell_dru_eclipse rolls the rank's proc chance (33, 66, 100) on a Wrath or Starfire crit (spell_proc -48516
	// HitMask 2), Wrath's at 60% of it, and the proc system rolls the same chance again
	rankChance := []float64{0, 0.33, 0.66, 1}[druid.Talents.Eclipse]
	eclipseActive := func() bool {
		return druid.SolarEclipseProcAura.IsActive() || druid.LunarEclipseProcAura.IsActive()
	}

	// Solar
	solarProcChance := rankChance * rankChance
	solarProcMultiplier := 1.4 + core.TernaryFloat64(druid.HasSetBonus(ItemSetNightsongGarb, 2), 0.07, 0)
	druid.SolarICD.Duration = time.Millisecond * 30000
	druid.SolarEclipseProcAura = druid.RegisterAura(core.Aura{
		Icd:      &druid.SolarICD,
		Label:    "Solar Eclipse proc",
		Duration: eclipseDuration,
		ActionID: core.ActionID{SpellID: 48517},
		OnGain: func(aura *core.Aura, sim *core.Simulation) {
			druid.Wrath.DamageMultiplier *= solarProcMultiplier
		},
		OnExpire: func(aura *core.Aura, sim *core.Simulation) {
			druid.Wrath.DamageMultiplier /= solarProcMultiplier
		},
	})

	druid.RegisterAura(core.Aura{
		Label:    "Eclipse (Solar)",
		Duration: core.NeverExpires,
		OnReset: func(aura *core.Aura, sim *core.Simulation) {
			aura.Activate(sim)
		},
		OnSpellHitDealt: func(aura *core.Aura, sim *core.Simulation, spell *core.Spell, result *core.SpellResult) {
			if !result.DidCrit() || !druid.Starfire.IsEqual(spell) {
				return
			}
			if !druid.SolarICD.Timer.IsReady(sim) || eclipseActive() {
				return
			}
			if sim.RandomFloat("Eclipse (Solar)") < solarProcChance {
				druid.SolarICD.Use(sim)
				druid.SolarEclipseProcAura.Activate(sim)
			}
		},
	})

	// Lunar
	lunarProcChance := rankChance * 0.6 * rankChance
	lunarBonusCrit := (40 + core.TernaryFloat64(druid.HasSetBonus(ItemSetNightsongGarb, 2), 7, 0)) * core.CritRatingPerCritChance
	druid.LunarICD.Duration = time.Millisecond * 30000
	druid.LunarEclipseProcAura = druid.RegisterAura(core.Aura{
		Icd:      &druid.LunarICD,
		Label:    "Lunar Eclipse proc",
		Duration: eclipseDuration,
		ActionID: core.ActionID{SpellID: 48518},
		OnGain: func(aura *core.Aura, sim *core.Simulation) {
			druid.Starfire.BonusCritRating += lunarBonusCrit
		},
		OnExpire: func(aura *core.Aura, sim *core.Simulation) {
			druid.Starfire.BonusCritRating -= lunarBonusCrit
		},
	})
	druid.RegisterAura(core.Aura{
		Label:    "Eclipse (Lunar)",
		Duration: core.NeverExpires,
		OnReset: func(aura *core.Aura, sim *core.Simulation) {
			aura.Activate(sim)
		},
		OnSpellHitDealt: func(aura *core.Aura, sim *core.Simulation, spell *core.Spell, result *core.SpellResult) {
			if !result.DidCrit() || !druid.Wrath.IsEqual(spell) {
				return
			}
			if !druid.LunarICD.Timer.IsReady(sim) || eclipseActive() {
				return
			}
			if sim.RandomFloat("Eclipse (Lunar)") < lunarProcChance {
				druid.LunarICD.Use(sim)
				druid.LunarEclipseProcAura.Activate(sim)
			}
		},
	})
}

func (druid *Druid) applyOwlkinFrenzy() {
	if druid.Talents.OwlkinFrenzy == 0 {
		return
	}

	druid.OwlkinFrenzyAura = druid.RegisterAura(core.Aura{
		Label:    "Owlkin Frenzy proc",
		ActionID: core.ActionID{SpellID: 48393},
		Duration: time.Second * 10,
		OnGain: func(aura *core.Aura, sim *core.Simulation) {
			druid.PseudoStats.DamageDealtMultiplier *= 1.1
		},
		OnExpire: func(aura *core.Aura, sim *core.Simulation) {
			druid.PseudoStats.DamageDealtMultiplier /= 1.1
		},
	})
}

func (druid *Druid) applyImprovedLotp() {
	if druid.Talents.ImprovedLeaderOfThePack == 0 {
		return
	}

	actionID := core.ActionID{SpellID: 34300}
	manaMetrics := druid.NewManaMetrics(actionID)
	healthMetrics := druid.NewHealthMetrics(actionID)
	manaRestore := float64(druid.Talents.ImprovedLeaderOfThePack) * 0.04
	healthRestore := 0.5 * manaRestore

	icd := core.Cooldown{
		Timer:    druid.NewTimer(),
		Duration: time.Second * 6,
	}

	druid.RegisterAura(core.Aura{
		Icd:      &icd,
		Label:    "Improved Leader of the Pack",
		Duration: core.NeverExpires,
		OnReset: func(aura *core.Aura, sim *core.Simulation) {
			aura.Activate(sim)
		},
		OnSpellHitDealt: func(aura *core.Aura, sim *core.Simulation, spell *core.Spell, result *core.SpellResult) {
			if !result.Landed() {
				return
			}
			if !spell.ProcMask.Matches(core.ProcMaskMeleeOrRanged) || !result.Outcome.Matches(core.OutcomeCrit) {
				return
			}
			if !icd.IsReady(sim) {
				return
			}
			icd.Use(sim)
			druid.AddMana(sim, druid.MaxMana()*manaRestore, manaMetrics)
			druid.GainHealth(sim, druid.MaxHealth()*healthRestore, healthMetrics)
		},
	})
}

func (druid *Druid) applyPredatoryInstincts() {
	if druid.Talents.PredatoryInstincts == 0 {
		return
	}

	onGainMod := druid.MeleeCritMultiplier(Cat)
	onExpireMod := druid.MeleeCritMultiplier(Humanoid)

	druid.PredatoryInstinctsAura = druid.RegisterAura(core.Aura{
		Label:    "Predatory Instincts",
		Duration: core.NeverExpires,
		OnGain: func(aura *core.Aura, sim *core.Simulation) {
			druid.Lacerate.CritMultiplier = onGainMod
			druid.Rip.CritMultiplier = onGainMod
			druid.Rake.CritMultiplier = onGainMod
		},
		OnExpire: func(aura *core.Aura, sim *core.Simulation) {
			druid.Lacerate.CritMultiplier = onExpireMod
			druid.Rip.CritMultiplier = onExpireMod
			druid.Rake.CritMultiplier = onExpireMod
		},
	})
}

func (druid *Druid) applyNaturalReaction() {
	if druid.Talents.NaturalReaction == 0 {
		return
	}

	actionID := core.ActionID{SpellID: 59072}
	rageMetrics := druid.NewRageMetrics(actionID).EnergizeThreat(core.EnergizeBySpell)
	rageAdded := float64(druid.Talents.NaturalReaction)

	core.MakeProcTriggerAura(&druid.Unit, core.ProcTrigger{
		Name:     "Natural Reaction Trigger",
		Callback: core.CallbackOnSpellHitTaken,
		ProcMask: core.ProcMaskMelee,
		Handler: func(sim *core.Simulation, _ *core.Spell, result *core.SpellResult) {
			if druid.InForm(Bear) && result.Outcome.Matches(core.OutcomeDodge) {
				druid.AddRage(sim, rageAdded, rageMetrics)
			}
		},
	})
}

func (druid *Druid) applyInfectedWounds() {
	if druid.Talents.InfectedWounds == 0 {
		return
	}

	iwAuras := druid.NewEnemyAuraArray(func(target *core.Unit) *core.Aura {
		return core.InfectedWoundsAura(target, druid.Talents.InfectedWounds)
	})
	druid.Env.RegisterPreFinalizeEffect(func() {
		if druid.Shred != nil {
			druid.Shred.RelatedAuras = append(druid.Shred.RelatedAuras, iwAuras)
		}
		if druid.MangleCat != nil {
			druid.MangleCat.RelatedAuras = append(druid.MangleCat.RelatedAuras, iwAuras)
		}
		if druid.MangleBear != nil {
			druid.MangleBear.RelatedAuras = append(druid.MangleBear.RelatedAuras, iwAuras)
		}
		if druid.Maul != nil {
			druid.Maul.RelatedAuras = append(druid.Maul.RelatedAuras, iwAuras)
		}
	})

	druid.RegisterAura(core.Aura{
		Label:    "Infected Wounds Talent",
		Duration: core.NeverExpires,
		OnReset: func(aura *core.Aura, sim *core.Simulation) {
			aura.Activate(sim)
		},
		OnSpellHitDealt: func(aura *core.Aura, sim *core.Simulation, spell *core.Spell, result *core.SpellResult) {
			if result.Landed() && (druid.Shred.IsEqual(spell) || druid.Maul.IsEqual(spell) || druid.MangleCat.IsEqual(spell) || druid.MangleBear.IsEqual(spell)) {
				iwAuras.Get(result.Target).Activate(sim)
			}
		},
	})
}
