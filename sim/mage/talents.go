package mage

import (
	"math"
	"slices"
	"time"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
	"github.com/wowsims/wotlk/sim/core/serverdata"
	"github.com/wowsims/wotlk/sim/core/stats"
)

// spellModDamage stacks percent damage bonuses from talents, glyphs and set pieces the server's way:
// Player::ApplySpellMod multiplies SPELLMOD_DAMAGE and SPELLMOD_DOT percentages, it doesn't add them.
func spellModDamage(bonuses ...float64) float64 {
	multiplier := 1.0
	for _, bonus := range bonuses {
		multiplier *= 1 + bonus
	}
	return multiplier
}

func (mage *Mage) ApplyTalents() {
	mage.t10Aura = mage.BloodmagesRegalia2pcAura()

	mage.applyArcaneConcentration()
	mage.applyFocusMagic()
	mage.applyIgnite()
	mage.applyEmpoweredFire()
	mage.applyMasterOfElements()
	mage.applyArcaneCritRefund()
	mage.applyWintersChill()
	mage.applyMoltenFury()
	mage.applyMissileBarrage()
	mage.applyHotStreak()
	mage.applyFingersOfFrost()
	mage.applyBrainFreeze()
	mage.applyFireStarter()

	mage.registerArcanePowerCD()
	mage.registerPresenceOfMindCD()
	mage.registerCombustionCD()
	mage.registerIcyVeinsCD()
	mage.registerColdSnapCD()
	mage.registerSummonWaterElementalCD()

	mage.PseudoStats.SpiritRegenRateCasting += float64(mage.Talents.ArcaneMeditation) / 6

	if mage.Talents.StudentOfTheMind > 0 {
		mage.MultiplyStat(stats.Spirit, 1.0+[]float64{0, .04, .07, .10}[mage.Talents.StudentOfTheMind])
	}

	if mage.Talents.ArcaneMind > 0 {
		mage.MultiplyStat(stats.Intellect, 1.0+0.03*float64(mage.Talents.ArcaneMind))
	}

	if mage.Talents.MindMastery > 0 {
		mage.AddStatDependency(stats.Intellect, stats.SpellPower, 0.03*float64(mage.Talents.MindMastery))
	}

	mage.AddStat(stats.SpellCrit, float64(mage.Talents.ArcaneInstability)*1*core.CritRatingPerCritChance)
	mage.PseudoStats.DamageDealtMultiplier *= 1 + .01*float64(mage.Talents.ArcaneInstability)
	mage.PseudoStats.DamageDealtMultiplier *= 1 + .01*float64(mage.Talents.PlayingWithFire)
	mage.PseudoStats.CastSpeedMultiplier *= 1 + .02*float64(mage.Talents.NetherwindPresence)

	mage.AddStat(stats.SpellCrit, float64(mage.Talents.Pyromaniac)*core.CritRatingPerCritChance)
	mage.PseudoStats.SpiritRegenRateCasting += float64(mage.Talents.Pyromaniac) / 6

	mage.AddStat(stats.SpellHit, float64(mage.Talents.Precision)*core.SpellHitRatingPerHitChance)
	// all school auras (UNIT_FIELD_POWER_COST_MULTIPLIER): Precision on every magic school, Frost Channeling on the
	// three of its mask (fire, frost, arcane), Arcane Focus on arcane
	mage.AddSchoolCostMultiplier(core.SpellSchoolMagic, -.01*float64(mage.Talents.Precision))
	mage.AddSchoolCostMultiplier(core.SpellSchoolArcane, -.01*float64(mage.Talents.ArcaneFocus))

	mage.PseudoStats.SchoolDamageDealtMultiplier[stats.SchoolIndexFrost] *= 1 + .02*float64(mage.Talents.PiercingIce)
	mage.PseudoStats.SchoolDamageDealtMultiplier[stats.SchoolIndexFrost] *= 1 + .01*float64(mage.Talents.ArcticWinds)
	mage.PseudoStats.ReducedPhysicalHitTakenChance += .01 * float64(mage.Talents.ArcticWinds)
	mage.AddSchoolCostMultiplier(core.SpellSchoolArcane|core.SpellSchoolFire|core.SpellSchoolFrost,
		-[]float64{0, .04, .07, .10}[mage.Talents.FrostChanneling])

	// threat auras, one school each, so Frostfire Bolt gets Burning Soul times Frost Channeling
	for _, t := range []struct {
		school core.SpellSchool
		cut    float64
	}{
		{core.SpellSchoolArcane, []float64{0, .2, .4}[mage.Talents.ArcaneSubtlety]},
		{core.SpellSchoolFire, []float64{0, .1, .2}[mage.Talents.BurningSoul]},
		{core.SpellSchoolFrost, []float64{0, .04, .07, .10}[mage.Talents.FrostChanneling]},
	} {
		if t.cut > 0 {
			mage.AddThreatSchoolMultiplier(t.school, 1-t.cut)
		}
	}

	// 0.5 resistance per level per rank (realPointsPerLevel of 29441 and 29444)
	magicAbsorptionBonus := 0.5 * float64(mage.Talents.MagicAbsorption) * float64(mage.Level)
	mage.AddStat(stats.ArcaneResistance, magicAbsorptionBonus)
	mage.AddStat(stats.FireResistance, magicAbsorptionBonus)
	mage.AddStat(stats.FrostResistance, magicAbsorptionBonus)
	mage.AddStat(stats.NatureResistance, magicAbsorptionBonus)
	mage.AddStat(stats.ShadowResistance, magicAbsorptionBonus)
}

func (mage *Mage) applyHotStreak() {
	if mage.Talents.HotStreak == 0 {
		return
	}

	procChance := float64(mage.Talents.HotStreak) / 3

	// Pyroblast uses it up (pyroblast.go)
	mage.HotStreakAura = mage.RegisterAura(core.Aura{
		Label:    "HotStreak",
		ActionID: core.ActionID{SpellID: 44448},
		Duration: time.Second * 10,
	})

	mage.hotStreakCritAura = mage.RegisterAura(core.Aura{
		Label:     "Hot Streak Proc Aura",
		ActionID:  core.ActionID{SpellID: 44448, Tag: 1},
		MaxStacks: 2,
		Duration:  time.Hour,
	})

	mage.RegisterAura(core.Aura{
		Label:    "Hot Streak Trigger",
		Duration: core.NeverExpires,
		OnReset: func(aura *core.Aura, sim *core.Simulation) {
			aura.Activate(sim)
		},
		OnSpellHitDealt: func(aura *core.Aura, sim *core.Simulation, spell *core.Spell, result *core.SpellResult) {
			if !spell.Flags.Matches(HotStreakSpells) {
				return
			}

			if !result.DidCrit() {
				mage.hotStreakCritAura.SetStacks(sim, 0)
				mage.hotStreakCritAura.Deactivate(sim)
				return
			}

			if mage.hotStreakCritAura.GetStacks() == 1 {
				if procChance == 1 || sim.Proc(procChance, "Hot Streak") {
					mage.hotStreakCritAura.SetStacks(sim, 0)
					mage.hotStreakCritAura.Deactivate(sim)

					mage.HotStreakAura.Activate(sim)
				}
			} else {
				mage.hotStreakCritAura.Activate(sim)
				mage.hotStreakCritAura.AddStack(sim)
			}
		},
	})

}

func (mage *Mage) applyArcaneConcentration() {
	if mage.Talents.ArcaneConcentration == 0 {
		return
	}

	bonusCrit := float64(mage.Talents.ArcanePotency) * 15 * core.CritRatingPerCritChance

	// The result that caused the proc. Used to check we don't deactivate from the same proc.
	var proccedAt time.Duration
	var proccedSpell *core.Spell

	if mage.Talents.ArcanePotency > 0 {
		mage.ArcanePotencyAura = mage.RegisterAura(core.Aura{
			Label:    "Arcane Potency",
			ActionID: core.ActionID{SpellID: 31572},
			Duration: time.Second * 15,
			OnGain: func(aura *core.Aura, sim *core.Simulation) {
				aura.Unit.AddStatDynamic(sim, stats.SpellCrit, bonusCrit)
			},
			OnExpire: func(aura *core.Aura, sim *core.Simulation) {
				aura.Unit.AddStatDynamic(sim, stats.SpellCrit, -bonusCrit)
			},
			OnCastComplete: func(aura *core.Aura, sim *core.Simulation, spell *core.Spell) {
				if !spell.Flags.Matches(SpellFlagMage) {
					return
				}
				if proccedAt == sim.CurrentTime && proccedSpell == spell {
					// Means this is another hit from the same cast that procced CC.
					return
				}
				aura.Deactivate(sim)
			},
		})
	}

	mage.ClearcastingAura = mage.RegisterAura(core.Aura{
		Label:    "Clearcasting",
		ActionID: core.ActionID{SpellID: 12536},
		Duration: time.Second * 15,
		OnGain: func(aura *core.Aura, sim *core.Simulation) {
			aura.Unit.PseudoStats.CostMultiplier -= 1
		},
		OnExpire: func(aura *core.Aura, sim *core.Simulation) {
			aura.Unit.PseudoStats.CostMultiplier += 1
		},
		OnCastComplete: func(aura *core.Aura, sim *core.Simulation, spell *core.Spell) {
			if !spell.Flags.Matches(SpellFlagMage) {
				return
			}
			if spell.DefaultCast.Cost == 0 {
				return
			}
			if spell == mage.ArcaneMissiles && mage.MissileBarrageAura.IsActive() {
				return
			}
			if proccedAt == sim.CurrentTime && proccedSpell == spell {
				// Means this is another hit from the same cast that procced CC.
				return
			}
			aura.Deactivate(sim)
		},
	})

	mage.RegisterAura(core.Aura{
		Label:    "Arcane Concentration",
		Duration: core.NeverExpires,
		OnReset: func(aura *core.Aura, sim *core.Simulation) {
			aura.Activate(sim)
		},
		OnSpellHitDealt: func(aura *core.Aura, sim *core.Simulation, spell *core.Spell, result *core.SpellResult) {
			if !spell.Flags.Matches(SpellFlagMage) || spell == mage.ArcaneMissiles {
				return
			}

			if !result.Landed() {
				return
			}

			procChance := 0.02 * float64(mage.Talents.ArcaneConcentration)

			// Arcane Missile ticks can proc CC, just at a low rate of about 1.5% with 5/5 Arcane Concentration
			if spell == mage.ArcaneMissilesTickSpell {
				procChance *= 0.15
			}

			if sim.RandomFloat("Arcane Concentration") > procChance {
				return
			}

			proccedAt = sim.CurrentTime
			proccedSpell = spell
			mage.ClearcastingAura.Activate(sim)
			if mage.ArcanePotencyAura != nil {
				mage.ArcanePotencyAura.Activate(sim)
			}
		},
	})
}

func (mage *Mage) applyMissileBarrage() {
	if mage.Talents.MissileBarrage == 0 {
		return
	}

	procChance := float64(mage.Talents.MissileBarrage) * .04
	// Arcane Missiles uses it up (arcane_missiles.go)
	mage.MissileBarrageAura = mage.RegisterAura(core.Aura{
		Label:    "Missile Barrage Proc",
		ActionID: core.ActionID{SpellID: 44401},
		Duration: time.Second * 15,
		OnGain: func(aura *core.Aura, sim *core.Simulation) {
			mage.ArcaneMissiles.CostMultiplier -= 100
			mage.ArcaneMissiles.CastTimeMultiplier /= 2
		},
		OnExpire: func(aura *core.Aura, sim *core.Simulation) {
			mage.ArcaneMissiles.CostMultiplier += 100
			mage.ArcaneMissiles.CastTimeMultiplier *= 2
			// spell_mage_missile_barrage_proc: timing out mid-channel still gives T10's 2 pieces when the channel ends
			if mage.missiles.channeling && !mage.missiles.barrageUsed && mage.t10Aura != nil {
				mage.missiles.t10AtEnd = true
			}
		},
	})

	mage.RegisterAura(core.Aura{
		Label:    "Missile Barrage Talent",
		Duration: core.NeverExpires,
		OnReset: func(aura *core.Aura, sim *core.Simulation) {
			aura.Activate(sim)
		},
		OnCastComplete: func(aura *core.Aura, sim *core.Simulation, spell *core.Spell) {
			if !spell.Flags.Matches(BarrageSpells) {
				return
			}

			roll := sim.RandomFloat("Missile Barrage")
			updChance := core.TernaryFloat64(spell.ActionID == mage.ArcaneBlast.ActionID, 2*procChance, procChance)

			if roll < updChance {
				mage.MissileBarrageAura.Activate(sim)
			}
		},
	})
}

func (mage *Mage) registerPresenceOfMindCD() {
	if !mage.Talents.PresenceOfMind {
		return
	}

	cooldown := 120.0
	if mage.Talents.ArcaneFlows > 0 {
		cooldown *= 1 - (.15 * float64(mage.Talents.ArcaneFlows))
	}

	actionID := core.ActionID{SpellID: 12043}

	var spellToUse *core.Spell
	mage.Env.RegisterPostFinalizeEffect(func() {
		if mage.Pyroblast != nil {
			spellToUse = mage.Pyroblast
		} else if mage.PrimaryTalentTree == 1 {
			spellToUse = mage.Fireball
		} else if mage.PrimaryTalentTree == 2 {
			spellToUse = mage.Frostbolt
		} else {
			spellToUse = mage.ArcaneBlast
		}
	})

	spell := mage.RegisterSpell(core.SpellConfig{
		ActionID: actionID,
		Flags:    core.SpellFlagNoOnCastComplete,
		Cast: core.CastConfig{
			CD: core.Cooldown{
				Timer:    mage.NewTimer(),
				Duration: time.Duration(cooldown) * time.Second,
			},
		},
		ExtraCastCondition: func(sim *core.Simulation, target *core.Unit) bool {
			if !mage.GCD.IsReady(sim) {
				return false
			}
			if mage.ArcanePowerAura.IsActive() {
				return false
			}

			return mage.CurrentMana() >= spellToUse.ApplyCostModifiers(spellToUse.DefaultCast.Cost)
		},
		SpellSchool:      core.SpellSchoolPhysical,
		ProcMask:         core.ProcMaskEmpty,
		ThreatMultiplier: 1,
		ApplyEffects: func(sim *core.Simulation, _ *core.Unit, _ *core.Spell) {
			if mage.ArcanePotencyAura != nil {
				mage.ArcanePotencyAura.Activate(sim)
			}

			normalCastTime := spellToUse.DefaultCast.CastTime
			spellToUse.DefaultCast.CastTime = 0
			spellToUse.Cast(sim, mage.CurrentTarget)
			spellToUse.DefaultCast.CastTime = normalCastTime
		},
	})

	mage.AddMajorCooldown(core.MajorCooldown{
		Spell: spell,
		Type:  core.CooldownTypeDPS,
	})
}

func (mage *Mage) registerArcanePowerCD() {
	if !mage.Talents.ArcanePower {
		return
	}
	actionID := core.ActionID{SpellID: 12042}

	var affectedSpells []*core.Spell
	mage.OnSpellRegistered(func(spell *core.Spell) {
		if spell.Flags.Matches(SpellFlagMage) {
			affectedSpells = append(affectedSpells, spell)
		}
	})

	mage.ArcanePowerAura = mage.RegisterAura(core.Aura{
		Label:    "Arcane Power",
		ActionID: actionID,
		Duration: core.TernaryDuration(mage.HasMajorGlyph(proto.MageMajorGlyph_GlyphOfArcanePower), time.Second*18, time.Second*15),
		// Arcane Power is SPELLMOD_DAMAGE (Player::ApplySpellMod), which multiplies onto whatever a
		// spell's own talents and glyphs already put in DamageMultiplierAdditive.
		OnGain: func(aura *core.Aura, sim *core.Simulation) {
			for _, spell := range affectedSpells {
				spell.DamageMultiplierAdditive *= 1.2
				spell.CostMultiplier += 0.2
			}
		},
		OnExpire: func(aura *core.Aura, sim *core.Simulation) {
			for _, spell := range affectedSpells {
				spell.DamageMultiplierAdditive /= 1.2
				spell.CostMultiplier -= 0.2
			}
		},
	})
	core.RegisterPercentDamageModifierEffect(mage.ArcanePowerAura, 1.2)
	core.RegisterPowerInfusionArcanePowerEffect(mage.ArcanePowerAura)

	spell := mage.RegisterSpell(core.SpellConfig{
		ActionID: actionID,
		Flags:    core.SpellFlagNoOnCastComplete,
		Cast: core.CastConfig{
			CD: core.Cooldown{
				Timer:    mage.NewTimer(),
				Duration: time.Second * time.Duration(120*(1-(.15*float64(mage.Talents.ArcaneFlows)))),
			},
		},
		SpellSchool:      core.SpellSchoolArcane,
		ProcMask:         core.ProcMaskEmpty,
		ThreatMultiplier: 1,
		ApplyEffects: func(sim *core.Simulation, _ *core.Unit, _ *core.Spell) {
			mage.ArcanePowerAura.Activate(sim)
		},
		ExtraCastCondition: func(sim *core.Simulation, target *core.Unit) bool {
			return !mage.ArcanePotencyAura.IsActive()
		},
	})

	mage.AddMajorCooldown(core.MajorCooldown{
		Spell: spell,
		Type:  core.CooldownTypeDPS,
	})
}

// TormentTheWeakBonus is tormentTheWeak for tests outside the package.
func (mage *Mage) TormentTheWeakBonus(target *core.Unit) float64 { return mage.tormentTheWeak(target) }

// tormentTheWeak is Torment the Weak's bonus against target, which the server gives only while the target is
// snared or its attacks are slowed (Unit::SpellPctDamageModsDone), whoever did it. Raid bosses are immune to
// snares, so on them only the attack speed debuffs count.
func (mage *Mage) tormentTheWeak(target *core.Unit) float64 {
	if mage.Talents.TormentTheWeak == 0 {
		return 1
	}
	if !mage.attackSpeedSlows.Get(target).AnyActive() && !target.HasActiveAuraWithTag(core.SlowAttackAuraTag) &&
		(target.ImmuneToMechanic(core.MechanicSnare) || !target.HasActiveAuraWithTag(core.SnareAuraTag)) {
		return 1
	}
	return 1 + .04*float64(mage.Talents.TormentTheWeak)
}

// registerChills gives Torment the Weak the mage's own snares, on the targets that take snares.
func (mage *Mage) registerChills() {
	if mage.Talents.TormentTheWeak == 0 {
		return
	}
	mage.chills = mage.NewEnemyAuraArray(func(target *core.Unit) *core.Aura {
		if target.ImmuneToMechanic(core.MechanicSnare) {
			return nil
		}
		return target.GetOrRegisterAura(core.Aura{
			Label:    "Chilled by " + mage.Label,
			Tag:      core.SnareAuraTag,
			Duration: time.Second,
		})
	})
}

// chill is a landed Frostbolt's, Frostfire Bolt's or Chilled's snare, or a Mirror Image's Frostbolt's, which
// casts with the mage's spell mods: Permafrost adds a second a point to each (11175's duration mod).
func (mage *Mage) chill(sim *core.Simulation, target *core.Unit, duration time.Duration) {
	mage.snare(sim, target, duration+time.Second*time.Duration(mage.Talents.Permafrost))
}

// snare is a snare outside Permafrost's class mask (Blast Wave). The longest one left counts.
func (mage *Mage) snare(sim *core.Simulation, target *core.Unit, duration time.Duration) {
	if mage.chills == nil {
		return
	}
	aura := mage.chills.Get(target)
	if aura == nil {
		return
	}
	if aura.IsActive() && aura.RemainingDuration(sim) >= duration {
		return
	}
	aura.Duration = duration
	aura.Activate(sim)
}

// t84pcKeepsProc is Kirin Tor Garb's 4 pieces keeping the Missile Barrage, Hot Streak or Fireball! a cast would
// use up (64869's roll in spell_mage.cpp).
func (mage *Mage) t84pcKeepsProc(sim *core.Simulation) bool {
	return mage.hasT84pc && sim.RandomFloat("MageT84PC") < T84PcProcChance
}

// pushingTheLimit is T10's 2 pieces on a Hot Streak, Missile Barrage or Fireball! used up, never one timing out
// (Aura::HandleAuraSpecificMods).
func (mage *Mage) pushingTheLimit(sim *core.Simulation) {
	if mage.t10Aura != nil {
		mage.t10Aura.Activate(sim)
	}
}

func (mage *Mage) applyMasterOfElements() {
	if mage.Talents.MasterOfElements > 0 {
		// spell_mage_master_of_elements: 10% a point of the base cost, whatever the cast paid
		refundPct := 10 * float64(mage.Talents.MasterOfElements)
		manaMetrics := mage.NewManaMetrics(core.ActionID{SpellID: 29076}).EnergizeThreat(core.EnergizeBySpell)
		// spell_proc -29074's 8 ms cooldown: one refund for all the targets a spell crits at once
		icd := core.Cooldown{Timer: mage.NewTimer(), Duration: time.Millisecond * 8}

		mage.RegisterAura(core.Aura{
			Label:    "Master of Elements",
			Duration: core.NeverExpires,
			OnReset: func(aura *core.Aura, sim *core.Simulation) {
				aura.Activate(sim)
			},
			OnSpellHitDealt: func(aura *core.Aura, sim *core.Simulation, spell *core.Spell, result *core.SpellResult) {
				// spell_proc -29074: crits of fire and frost mage spells
				if !result.DidCrit() || !spell.Flags.Matches(SpellFlagMage) ||
					!spell.SpellSchool.Matches(core.SpellSchoolFire|core.SpellSchoolFrost) || !icd.IsReady(sim) {
					return
				}
				icd.Use(sim)
				cost := spell.DefaultCast.Cost
				switch {
				case spell.ActionID.SpellID == 55362:
					// the explosion costs nothing, so the script reads the bomb's cost
					cost = mage.LivingBomb.DefaultCast.Cost
				case spell == mage.DeepFreeze:
					// its damage is 71757's, which costs nothing
					cost = 0
				}
				if refund := math.Floor(cost * refundPct / 100); refund > 0 {
					mage.AddMana(sim, refund, manaMetrics)
				}
			},
		})
	}

	if mage.Talents.Burnout > 0 {
		// spell_mage_burnout burns 1% a point of the crit spell's cost, outside the combat log
		burnPct := float64(mage.Talents.Burnout)
		burnMetrics := mage.NewManaMetrics(core.ActionID{SpellID: 44472})
		// spell_proc -44449's 8 ms cooldown, as for Master of Elements
		icd := core.Cooldown{Timer: mage.NewTimer(), Duration: time.Millisecond * 8}

		mage.RegisterAura(core.Aura{
			Label:    "Burnout",
			Duration: core.NeverExpires,
			OnReset: func(aura *core.Aura, sim *core.Simulation) {
				aura.Activate(sim)
			},
			OnSpellHitDealt: func(aura *core.Aura, sim *core.Simulation, spell *core.Spell, result *core.SpellResult) {
				// Deep Freeze is outside spell_proc -44449's class mask
				if !result.DidCrit() || !spell.Flags.Matches(SpellFlagMage) || spell == mage.DeepFreeze || !icd.IsReady(sim) {
					return
				}
				icd.Use(sim)
				// SpellInfo::CalcPowerCost at the hit, by which a Clearcasting or Firestarter cast has used up its
				// free cast
				cost := spell.CurCast.Cost
				if cost <= 0 {
					cost = spell.StaticCost()
				}
				if burn := min(math.Floor(cost*burnPct/100), mage.CurrentMana()); burn > 0 {
					mage.SpendMana(sim, burn, burnMetrics)
				}
			},
		})
	}
}

// applyArcaneCritRefund is spell_tweaks_arcane_crit_refund: an arcane crit gives back 15% a point of
// Spell Power of what its cast paid, as an energize under the talent's id.
func (mage *Mage) applyArcaneCritRefund() {
	if mage.Talents.SpellPower == 0 || !mage.Server().SpellTweaks.ArcaneCritRefund {
		return
	}
	refundPct := 15 * float64(mage.Talents.SpellPower)
	talentID := []int32{35578, 35581}[mage.Talents.SpellPower-1]
	manaMetrics := mage.NewManaMetrics(core.ActionID{SpellID: talentID}).EnergizeThreat(core.EnergizeBySpell)
	// spell_proc -35578's 8 ms cooldown: one refund for all the targets an Arcane Explosion crits
	icd := core.Cooldown{Timer: mage.NewTimer(), Duration: time.Millisecond * 8}

	mage.RegisterAura(core.Aura{
		Label:    "Arcane Crit Refund",
		Duration: core.NeverExpires,
		OnReset: func(aura *core.Aura, sim *core.Simulation) {
			aura.Activate(sim)
		},
		OnSpellHitDealt: func(aura *core.Aura, sim *core.Simulation, spell *core.Spell, result *core.SpellResult) {
			if !result.DidCrit() || !spell.Flags.Matches(SpellFlagMage) ||
				!spell.SpellSchool.Matches(core.SpellSchoolArcane) || !icd.IsReady(sim) {
				return
			}
			icd.Use(sim)
			// what the cast paid, so nothing under Clearcasting. Arcane Barrage's cooldown outlasts its
			// flight, so no later cast has written over it by the hit
			cost := spell.CurCast.Cost
			if spell == mage.ArcaneMissilesTickSpell {
				// a missile lands after the channel, so it takes a share of the channel's base cost, free or
				// not. SpellInfo::GetMaxTicks doesn't count a periodic trigger and gives its default 6, not 5
				cost = math.Floor(mage.ArcaneMissiles.DefaultCast.Cost / 6)
			}
			if refund := math.Floor(cost * refundPct / 100); refund > 0 {
				mage.AddMana(sim, refund, manaMetrics)
			}
		},
	})
}

func (mage *Mage) registerCombustionCD() {
	if !mage.Talents.Combustion {
		return
	}
	actionID := core.ActionID{SpellID: 11129}
	cd := core.Cooldown{
		Timer:    mage.NewTimer(),
		Duration: time.Minute * 2,
	}

	// the factor in the state the mage was built in and with the other state of its meta gem
	combCritMult := func(bonus float64) (float64, float64) {
		with, without := mage.SpellCritMultiplier(1, bonus+.5), mage.SpellCritMultiplier(1, bonus)
		return with / without, mage.MetaCritOther(with) / mage.MetaCritOther(without)
	}
	fireCombCritMult, fireCombCritOther := combCritMult(mage.bonusCritDamage)
	frostfireCombCritMult, frostfireCombCritOther := combCritMult(mage.bonusCritDamage + float64(mage.Talents.IceShards)/3)

	var fireSpells []*core.Spell
	mage.OnSpellRegistered(func(spell *core.Spell) {
		if spell.SpellSchool.Matches(core.SpellSchoolFire) {
			fireSpells = append(fireSpells, spell)
		}
	})

	numCrits := 0
	const critPerStack = 10 * core.CritRatingPerCritChance

	// Mirror Image's Fire Blast takes both Combustion mods through the mage (Unit::GetSpellModOwner), but
	// its hits neither add stacks nor count toward the three crits
	imageCombCritMult := func() float64 {
		return mage.mirrorImage.fireblastCritMultiplier(.5) / mage.mirrorImage.fireblastCritMultiplier(0)
	}

	mage.CombustionAura = mage.RegisterAura(core.Aura{
		Label:     "Combustion",
		ActionID:  actionID,
		Duration:  core.NeverExpires,
		MaxStacks: 20,
		OnGain: func(aura *core.Aura, sim *core.Simulation) {
			numCrits = 0
			for _, spell := range fireSpells {
				if spell == mage.FrostfireBolt {
					mage.ScaleCrit(spell, frostfireCombCritMult, frostfireCombCritOther, true)
				} else {
					mage.ScaleCrit(spell, fireCombCritMult, fireCombCritOther, true)
				}
			}
			mage.mirrorImage.Fireblast.CritMultiplier *= imageCombCritMult()
		},
		OnExpire: func(aura *core.Aura, sim *core.Simulation) {
			cd.Use(sim)
			mage.UpdateMajorCooldowns()
			for _, spell := range fireSpells {
				if spell == mage.FrostfireBolt {
					mage.ScaleCrit(spell, frostfireCombCritMult, frostfireCombCritOther, false)
				} else {
					mage.ScaleCrit(spell, fireCombCritMult, fireCombCritOther, false)
				}
			}
			mage.mirrorImage.Fireblast.CritMultiplier /= imageCombCritMult()
		},
		OnStacksChange: func(aura *core.Aura, sim *core.Simulation, oldStacks int32, newStacks int32) {
			bonusCrit := critPerStack * float64(newStacks-oldStacks)
			for _, spell := range fireSpells {
				spell.BonusCritRating += bonusCrit
			}
			mage.mirrorImage.Fireblast.BonusCritRating += bonusCrit
		},
		OnSpellHitDealt: func(aura *core.Aura, sim *core.Simulation, spell *core.Spell, result *core.SpellResult) {
			if !spell.SpellSchool.Matches(core.SpellSchoolFire) || !spell.Flags.Matches(SpellFlagMage) {
				return
			}
			if spell == mage.Ignite || spell == mage.LivingBomb { //LB dot action should be ignored
				return
			}
			if !result.Landed() {
				return
			}
			if numCrits >= 3 {
				return
			}

			// TODO: This wont work properly with flamestrike
			aura.AddStack(sim)

			if result.DidCrit() {
				numCrits++
				if numCrits == 3 {
					aura.Deactivate(sim)
				}
			}
		},
	})

	spell := mage.RegisterSpell(core.SpellConfig{
		ActionID: actionID,
		Flags:    core.SpellFlagNoOnCastComplete,
		Cast: core.CastConfig{
			CD: cd,
		},
		ExtraCastCondition: func(sim *core.Simulation, target *core.Unit) bool {
			return !mage.CombustionAura.IsActive()
		},
		SpellSchool:      core.SpellSchoolFire,
		ProcMask:         core.ProcMaskEmpty,
		ThreatMultiplier: 1,
		ApplyEffects: func(sim *core.Simulation, _ *core.Unit, _ *core.Spell) {
			mage.CombustionAura.Activate(sim)
			mage.CombustionAura.AddStack(sim)
		},
	})

	mage.AddMajorCooldown(core.MajorCooldown{
		Spell: spell,
		Type:  core.CooldownTypeDPS,
	})
}

func (mage *Mage) registerIcyVeinsCD() {
	if !mage.Talents.IcyVeins {
		return
	}

	actionID := core.ActionID{SpellID: 12472}

	icyVeinsAura := mage.RegisterAura(core.Aura{
		Label:    "Icy Veins",
		ActionID: actionID,
		Duration: time.Second * 20,
		OnGain: func(aura *core.Aura, sim *core.Simulation) {
			aura.Unit.MultiplyCastSpeed(1.2)
		},
		OnExpire: func(aura *core.Aura, sim *core.Simulation) {
			aura.Unit.MultiplyCastSpeed(1 / 1.2)
		},
	})

	mage.IcyVeins = mage.RegisterSpell(core.SpellConfig{
		ActionID: actionID,
		Flags:    core.SpellFlagNoOnCastComplete,

		ManaCost: core.ManaCostOptions{
			BaseCost: 0.03,
		},
		Cast: core.CastConfig{
			CD: core.Cooldown{
				Timer:    mage.NewTimer(),
				Duration: time.Second * time.Duration(180*[]float64{1, .93, .86, .80}[mage.Talents.IceFloes]),
			},
		},
		ExtraCastCondition: func(sim *core.Simulation, target *core.Unit) bool {
			// Need to check for icy veins already active in case Cold Snap is used right after.
			return !icyVeinsAura.IsActive()
		},
		SpellSchool:      core.SpellSchoolFrost,
		ProcMask:         core.ProcMaskEmpty,
		ThreatMultiplier: 1,
		ApplyEffects: func(sim *core.Simulation, _ *core.Unit, _ *core.Spell) {
			icyVeinsAura.Activate(sim)
		},
	})

	mage.AddMajorCooldown(core.MajorCooldown{
		Spell: mage.IcyVeins,
		Type:  core.CooldownTypeDPS,
	})
}

func (mage *Mage) registerColdSnapCD() {
	if !mage.Talents.ColdSnap {
		return
	}

	cooldown := time.Duration(float64(time.Minute*8) * (1.0 - float64(mage.Talents.ColdAsIce)*0.1))
	actionID := core.ActionID{SpellID: 11958}

	spell := mage.RegisterSpell(core.SpellConfig{
		ActionID: actionID,
		Flags:    core.SpellFlagNoOnCastComplete,

		Cast: core.CastConfig{
			CD: core.Cooldown{
				Timer:    mage.NewTimer(),
				Duration: cooldown,
			},
		},
		ExtraCastCondition: func(sim *core.Simulation, target *core.Unit) bool {
			// Don't use if there are no cooldowns to reset.
			return (mage.IcyVeins != nil && !mage.IcyVeins.IsReady(sim)) ||
				(mage.SummonWaterElemental != nil && !mage.SummonWaterElemental.IsReady(sim))
		},
		SpellSchool:      core.SpellSchoolFrost,
		ProcMask:         core.ProcMaskEmpty,
		ThreatMultiplier: 1,
		ApplyEffects: func(sim *core.Simulation, _ *core.Unit, _ *core.Spell) {
			if mage.IcyVeins != nil {
				mage.IcyVeins.CD.Reset()
			}
			if mage.SummonWaterElemental != nil {
				mage.SummonWaterElemental.CD.Reset()
			}
		},
	})

	mage.AddMajorCooldown(core.MajorCooldown{
		Spell: spell,
		Type:  core.CooldownTypeDPS,
		ShouldActivate: func(sim *core.Simulation, character *core.Character) bool {
			// Ideally wait for both water ele and icy veins so we can reset both.
			if mage.IcyVeins != nil && mage.IcyVeins.IsReady(sim) {
				return false
			}
			if mage.SummonWaterElemental != nil && mage.SummonWaterElemental.IsReady(sim) {
				return false
			}

			return true
		},
	})
}

func (mage *Mage) applyMoltenFury() {
	if mage.Talents.MoltenFury == 0 {
		return
	}

	multiplier := 1.0 + 0.06*float64(mage.Talents.MoltenFury)

	mage.RegisterResetEffect(func(sim *core.Simulation) {
		sim.RegisterExecutePhaseCallback(func(sim *core.Simulation, isExecute int32) {
			if isExecute == 35 {
				mage.PseudoStats.DamageDealtMultiplier *= multiplier
				// For some reason Molten Fury doesn't apply to living bomb DoT, so cancel it out.
				if mage.LivingBomb != nil {
					mage.LivingBomb.DamageMultiplier /= multiplier
				}
			}
		})
	})
}

func (mage *Mage) applyFingersOfFrost() {
	if mage.Talents.FingersOfFrost == 0 {
		return
	}

	bonusCrit := []float64{0, 17, 34, 50}[mage.Talents.Shatter] * core.CritRatingPerCritChance
	iceLanceMultiplier := core.TernaryFloat64(mage.HasMajorGlyph(proto.MageMajorGlyph_GlyphOfIceLance), 4, 3)

	var shatterSpells, castChargeSpells, hitChargeSpells []*core.Spell

	mage.FingersOfFrostAura = mage.RegisterAura(core.Aura{
		Label:     "Fingers of Frost Proc",
		ActionID:  core.ActionID{SpellID: 44545},
		Duration:  time.Second * 15,
		MaxStacks: 2,
		OnInit: func(aura *core.Aura, sim *core.Simulation) {
			for _, spell := range mage.Spellbook {
				if inMageMask(spell, fofShatterMask) {
					shatterSpells = append(shatterSpells, spell)
				}
				if !inMageMask(spell, fofChargeMask) || spell.ServerSpell().SchoolMask&fofChargeSchools == 0 {
					continue
				}
				// spell_mage_fingers_of_frost: a cast pays at its cast phase, a channel never, and a triggered
				// spell (missile, tick, explosion) skips the cast phase and pays per landed hit
				switch {
				case !spell.Flags.Matches(core.SpellFlagAPL):
					hitChargeSpells = append(hitChargeSpells, spell)
				case !spell.Flags.Matches(core.SpellFlagChanneled):
					castChargeSpells = append(castChargeSpells, spell)
				}
			}
		},
		OnGain: func(aura *core.Aura, sim *core.Simulation) {
			for _, spell := range shatterSpells {
				spell.BonusCritRating += bonusCrit
			}
			mage.IceLance.DamageMultiplier *= iceLanceMultiplier
		},
		OnExpire: func(aura *core.Aura, sim *core.Simulation) {
			for _, spell := range shatterSpells {
				spell.BonusCritRating -= bonusCrit
			}
			mage.IceLance.DamageMultiplier /= iceLanceMultiplier
		},
		// a tick's proc from Improved Blizzard comes after this (blizzard.go)
		OnSpellHitDealt: func(aura *core.Aura, sim *core.Simulation, spell *core.Spell, result *core.SpellResult) {
			if result.Landed() && slices.Contains(hitChargeSpells, spell) {
				aura.RemoveStack(sim)
			}
		},
	})

	procChance := []float64{0, .07, .15}[mage.Talents.FingersOfFrost]
	mage.procFingersOfFrost = func(sim *core.Simulation) {
		if sim.RandomFloat("Fingers of Frost") < procChance {
			mage.FingersOfFrostAura.Activate(sim)
			mage.FingersOfFrostAura.SetStacks(sim, 2)
		}
	}
	mage.RegisterAura(core.Aura{
		Label:    "Fingers of Frost Talent",
		Duration: core.NeverExpires,
		OnReset: func(aura *core.Aura, sim *core.Simulation) {
			aura.Activate(sim)
		},
		// The charge goes before the roll, as the proc system takes charges before it triggers anything, so a
		// cast that procs keeps both. spell_proc 44545 rolls at the cast, hit or miss (Cone of Cold too, which
		// the sim lacks); Improved Blizzard rolls its own on each landed tick (blizzard.go).
		OnCastComplete: func(aura *core.Aura, sim *core.Simulation, spell *core.Spell) {
			if mage.FingersOfFrostAura.IsActive() && slices.Contains(castChargeSpells, spell) {
				mage.FingersOfFrostAura.RemoveStack(sim)
			}
			if spell == mage.Frostbolt || spell == mage.FrostfireBolt {
				mage.procFingersOfFrost(sim)
			}
		},
	})
}

// Shatter's crit needs a frozen target, which Fingers of Frost's 44544 fakes for the spells of its class mask,
// and goes only to the spells of Shatter's own (Unit::SpellTakenCritChance): both masks (11170, 44544) at once.
var fofShatterMask = [3]uint32{551686775, 1151040, 32}

// spell_proc 74396: a cast of these, fire, frost or arcane (the server's school mask 84), uses up a charge
var fofChargeMask = [3]uint32{685904631, 1151048, 0}

const fofChargeSchools = 84

// inMageMask is SpellInfo::IsAffected for a mage aura's class mask, for a harmful magic spell
// (SPELL_DAMAGE_CLASS_MAGIC, PROC_FLAG_DONE_SPELL_MAGIC_DMG_CLASS_NEG).
func inMageMask(spell *core.Spell, mask [3]uint32) bool {
	sd := spell.ServerSpell()
	return sd != nil && sd.Family == mageFamily && sd.DmgClass == serverdata.DmgClassMagic &&
		sd.Flags&serverdata.FlagPositive == 0 &&
		(sd.FamilyFlags[0]&mask[0] != 0 || sd.FamilyFlags[1]&mask[1] != 0 || sd.FamilyFlags[2]&mask[2] != 0)
}

const mageFamily = 3

func (mage *Mage) applyBrainFreeze() {
	if mage.Talents.BrainFreeze == 0 {
		return
	}

	mage.BrainFreezeAura = mage.RegisterAura(core.Aura{
		Label:    "Brain Freeze Proc",
		ActionID: core.ActionID{SpellID: 44549},
		Duration: time.Second * 15,
		OnGain: func(aura *core.Aura, sim *core.Simulation) {
			mage.Fireball.CostMultiplier -= 100
			mage.Fireball.CastTimeMultiplier -= 1
			mage.FrostfireBolt.CostMultiplier -= 100
			mage.FrostfireBolt.CastTimeMultiplier -= 1
		},
		OnExpire: func(aura *core.Aura, sim *core.Simulation) {
			mage.Fireball.CostMultiplier += 100
			mage.Fireball.CastTimeMultiplier += 1
			mage.FrostfireBolt.CostMultiplier += 100
			mage.FrostfireBolt.CastTimeMultiplier += 1
		},
		// spell_proc 57761 uses it up at the cast, not the landing. T10 2pc only on that, never when it times out
		OnCastComplete: func(aura *core.Aura, sim *core.Simulation, spell *core.Spell) {
			if (spell == mage.FrostfireBolt || spell == mage.Fireball) && !mage.t84pcKeepsProc(sim) {
				aura.Deactivate(sim)
				mage.pushingTheLimit(sim)
			}
		},
	})

	procChance := .05 * float64(mage.Talents.BrainFreeze)
	var procSpells []*core.Spell
	mage.RegisterAura(core.Aura{
		Label:    "Brain Freeze Talent",
		Duration: core.NeverExpires,
		OnInit: func(aura *core.Aura, sim *core.Simulation) {
			for _, spell := range mage.Spellbook {
				// Blizzard's own cast puts its aura on the mage, so it's no harmful hit
				if inMageMask(spell, brainFreezeMask) && !spell.Flags.Matches(core.SpellFlagChanneled) {
					procSpells = append(procSpells, spell)
				}
			}
		},
		OnReset: func(aura *core.Aura, sim *core.Simulation) {
			aura.Activate(sim)
		},
		OnSpellHitDealt: func(aura *core.Aura, sim *core.Simulation, spell *core.Spell, result *core.SpellResult) {
			if result.Landed() && slices.Contains(procSpells, spell) && sim.RandomFloat("Brain Freeze") < procChance {
				mage.BrainFreezeAura.Activate(sim)
			}
		},
	})
}

// spell_proc -44546: a landed Frostbolt, Frostfire Bolt or Blizzard tick, on each target it hits (Frost Nova and
// Cone of Cold too, which the sim lacks). Chilled is outside the mask, so Improved Blizzard adds nothing.
var brainFreezeMask = [3]uint32{736, 4096, 0}

func (mage *Mage) applyWintersChill() {
	if mage.Talents.WintersChill == 0 {
		return
	}

	procChance := []float64{0, 0.33, 0.66, 1}[mage.Talents.WintersChill]

	wcAuras := mage.NewEnemyAuraArray(func(target *core.Unit) *core.Aura {
		return core.WintersChillAura(target, 0)
	})
	mage.Env.RegisterPreFinalizeEffect(func() {
		for _, spell := range mage.GetSpellsMatchingSchool(core.SpellSchoolFrost) {
			spell.RelatedAuras = append(spell.RelatedAuras, wcAuras)
		}
	})

	mage.RegisterAura(core.Aura{
		Label:    "Winters Chill Talent",
		Duration: core.NeverExpires,
		OnReset: func(aura *core.Aura, sim *core.Simulation) {
			aura.Activate(sim)
		},
		OnSpellHitDealt: func(aura *core.Aura, sim *core.Simulation, spell *core.Spell, result *core.SpellResult) {
			if !result.Landed() || !spell.SpellSchool.Matches(core.SpellSchoolFrost) {
				return
			}

			if sim.Proc(procChance, "Winters Chill") {
				aura := wcAuras.Get(result.Target)
				aura.Activate(sim)
				if aura.IsActive() {
					aura.AddStack(sim)
				}
			}
		},
	})
}

func (mage *Mage) applyFireStarter() {
	if mage.Talents.Firestarter == 0 {
		return
	}

	firestarterAura := mage.RegisterAura(core.Aura{
		Label:    "Firestarter",
		ActionID: core.ActionID{SpellID: 54741},
		Duration: 10 * time.Second,
		OnGain: func(aura *core.Aura, sim *core.Simulation) {
			mage.Flamestrike.CostMultiplier -= 100
			mage.Flamestrike.CastTimeMultiplier -= 1
			mage.FlamestrikeRank8.CostMultiplier -= 100
			mage.FlamestrikeRank8.CastTimeMultiplier -= 1
		},
		OnExpire: func(aura *core.Aura, sim *core.Simulation) {
			mage.Flamestrike.CostMultiplier += 100
			mage.Flamestrike.CastTimeMultiplier += 1
			mage.FlamestrikeRank8.CostMultiplier += 100
			mage.FlamestrikeRank8.CastTimeMultiplier += 1
		},
		OnCastComplete: func(aura *core.Aura, sim *core.Simulation, spell *core.Spell) {
			if spell == mage.Flamestrike || spell == mage.FlamestrikeRank8 {
				aura.Deactivate(sim)
			}
		},
	})

	mage.RegisterAura(core.Aura{
		Label:    "Firestarter talent",
		Duration: core.NeverExpires,
		OnReset: func(aura *core.Aura, sim *core.Simulation) {
			aura.Activate(sim)
		},
		OnSpellHitDealt: func(aura *core.Aura, sim *core.Simulation, spell *core.Spell, result *core.SpellResult) {
			if !result.Landed() {
				return
			}

			if spell == mage.BlastWave || spell == mage.DragonsBreath {
				firestarterAura.Activate(sim)
			}
		},
	})
}
