package hunter

import (
	"math"
	"time"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/stats"
)

type PetAbilityType int

const (
	Unknown PetAbilityType = iota
	AcidSpit
	Bite
	Claw
	DemoralizingScreech
	FireBreath
	FuriousHowl
	FroststormBreath
	Gore
	LavaBreath
	LightningBreath
	MonstrousBite
	NetherShock
	Pin
	PoisonSpit
	Rake
	Ravage
	SavageRend
	ScorpidPoison
	Smack
	Snatch
	SonicBlast
	SpiritStrike
	SporeCloud
	Stampede
	Sting
	Swipe
	TendonRip
	VenomWebSpray
)

// These IDs are needed for certain talents.
const BiteSpellID = 52474
const ClawSpellID = 52472
const SmackSpellID = 52476

func (hp *HunterPet) NewPetAbility(abilityType PetAbilityType, isPrimary bool) *core.Spell {
	switch abilityType {
	case AcidSpit:
		return hp.newAcidSpit()
	case Bite:
		return hp.newBite()
	case Claw:
		return hp.newClaw()
	case DemoralizingScreech:
		return hp.newDemoralizingScreech()
	case FireBreath:
		return hp.newFireBreath()
	case FroststormBreath:
		return hp.newFroststormBreath()
	case FuriousHowl:
		return hp.newFuriousHowl()
	case Gore:
		return hp.newGore()
	case LavaBreath:
		return hp.newLavaBreath()
	case LightningBreath:
		return hp.newLightningBreath()
	case MonstrousBite:
		return hp.newMonstrousBite()
	case NetherShock:
		return hp.newNetherShock()
	case Pin:
		return hp.newPin()
	case PoisonSpit:
		return hp.newPoisonSpit()
	case Rake:
		return hp.newRake()
	case Ravage:
		return hp.newRavage()
	case SavageRend:
		return hp.newSavageRend()
	case ScorpidPoison:
		return hp.newScorpidPoison()
	case Smack:
		return hp.newSmack()
	case Snatch:
		return hp.newSnatch()
	case SonicBlast:
		return hp.newSonicBlast()
	case SpiritStrike:
		return hp.newSpiritStrike()
	case SporeCloud:
		return hp.newSporeCloud()
	case Stampede:
		return hp.newStampede()
	case Sting:
		return hp.newSting()
	case Swipe:
		return hp.newSwipe()
	case TendonRip:
		return hp.newTendonRip()
	case VenomWebSpray:
		return hp.newVenomWebSpray()
	case Unknown:
		return nil
	default:
		panic("Invalid pet ability type")
	}
}

func (hp *HunterPet) newFocusDump(pat PetAbilityType, spellID int32) *core.Spell {
	return hp.RegisterSpell(core.SpellConfig{
		ActionID:    core.ActionID{SpellID: spellID},
		SpellSchool: core.SpellSchoolPhysical,
		ProcMask:    core.ProcMaskMeleeMHSpecial,
		Flags:       core.SpellFlagMeleeMetrics | core.SpellFlagIncludeTargetBonusDamage,

		FocusCost: core.FocusCostOptions{
			Cost: 25,
		},
		Cast: core.CastConfig{
			DefaultCast: core.Cast{
				GCD: core.GCDDefault,
			},
			IgnoreHaste: true,
		},

		DamageMultiplier: 1 * hp.hunterOwner.markedForDeathMultiplier(),
		CritMultiplier:   2,
		ThreatMultiplier: 1,

		// SP unused: the pet's spell damage only reaches the magic schools
		Direct: core.SpellEffect{Effect: 0, Min: 118, Max: 168, SP: 0.119658, AP: 0.07},

		ApplyEffects: func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
			baseDamage := spell.Direct.Roll(sim) + spell.Direct.AP*spell.MeleeAttackPower()
			baseDamage *= hp.killCommandMult()
			spell.CalcAndDealDamage(sim, target, baseDamage, spell.OutcomeMeleeSpecialHitAndCrit)
		},
	})
}

func (hp *HunterPet) newBite() *core.Spell {
	return hp.newFocusDump(Bite, BiteSpellID)
}
func (hp *HunterPet) newClaw() *core.Spell {
	return hp.newFocusDump(Claw, ClawSpellID)
}
func (hp *HunterPet) newSmack() *core.Spell {
	return hp.newFocusDump(Smack, SmackSpellID)
}

type PetSpecialAbilityConfig struct {
	Type    PetAbilityType
	Cost    float64
	SpellID int32
	School  core.SpellSchool
	// magic damage class: the magic hit table, and a crit adds 50% (Unit::SpellCriticalDamageBonus)
	Magic bool
	// SPELL_ATTR2_CANT_CRIT, or magic class with a physical school, which Unit::SpellDoneCritChance
	// gives no crit chance
	CantCrit bool
	GCD      time.Duration
	CD       time.Duration
	// yards per second, 0 for a melee ability
	MissileSpeed float64
	// SP only counts for a magic school: the pet's spell damage reaches no other
	Damage core.SpellEffect

	Dot core.DotConfig

	OnSpellHitDealt func(*core.Simulation, *core.Spell, *core.SpellResult)
}

func (hp *HunterPet) newSpecialAbility(config PetSpecialAbilityConfig) *core.Spell {
	var flags core.SpellFlag
	var procMask core.ProcMask
	if config.School == core.SpellSchoolPhysical {
		flags = core.SpellFlagMeleeMetrics | core.SpellFlagIncludeTargetBonusDamage
		procMask = core.ProcMaskSpellDamage
	} else {
		procMask = core.ProcMaskMeleeMHSpecial
	}

	critMultiplier := 2.0
	if config.Magic {
		critMultiplier = hp.DefaultSpellCritMultiplier()
	}
	magicSchool := config.School.Matches(core.SpellSchoolMagic)

	onSpellHitDealt := config.OnSpellHitDealt
	land := func(sim *core.Simulation, spell *core.Spell, result *core.SpellResult) {
		spell.DealDamage(sim, result)
		if onSpellHitDealt != nil {
			onSpellHitDealt(sim, spell, result)
		}
	}
	applyEffects := func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
		baseDamage := spell.Direct.Roll(sim) + spell.Direct.AP*spell.MeleeAttackPower()
		if magicSchool {
			baseDamage += spell.Direct.SP * spell.SpellPower()
		}
		baseDamage *= hp.killCommandMult()
		outcome := spell.OutcomeMeleeSpecialHitAndCrit
		switch {
		case config.Magic && config.CantCrit:
			outcome = spell.OutcomeMagicHit
		case config.Magic:
			outcome = spell.OutcomeMagicHitAndCrit
		case config.CantCrit:
			outcome = spell.OutcomeMeleeSpecialHit
		}
		result := spell.CalcDamage(sim, target, baseDamage, outcome)
		if spell.MissileSpeed == 0 {
			land(sim, spell, result)
			return
		}
		spell.WaitTravelTime(sim, func(sim *core.Simulation) {
			land(sim, spell, result)
		})
	}

	return hp.RegisterSpell(core.SpellConfig{
		ActionID:     core.ActionID{SpellID: config.SpellID},
		SpellSchool:  config.School,
		ProcMask:     procMask,
		Flags:        flags,
		MissileSpeed: config.MissileSpeed,

		DamageMultiplier: 1 * hp.hunterOwner.markedForDeathMultiplier(),
		CritMultiplier:   critMultiplier,
		ThreatMultiplier: 1,

		FocusCost: core.FocusCostOptions{
			Cost: config.Cost,
		},
		Cast: core.CastConfig{
			DefaultCast: core.Cast{
				GCD: config.GCD,
			},
			IgnoreHaste: true,
			CD: core.Cooldown{
				Timer:    hp.NewTimer(),
				Duration: hp.hunterOwner.applyLongevity(config.CD),
			},
		},
		Direct:       config.Damage,
		Dot:          config.Dot,
		ApplyEffects: applyEffects,
	})
}

func (hp *HunterPet) newAcidSpit() *core.Spell {
	acidSpitAuras := hp.NewEnemyAuraArray(core.AcidSpitAura)
	return hp.newSpecialAbility(PetSpecialAbilityConfig{
		Type:         AcidSpit,
		Cost:         20,
		GCD:          core.GCDDefault,
		CD:           time.Second * 10,
		SpellID:      55754,
		School:       core.SpellSchoolNature,
		Magic:        true,
		MissileSpeed: 24,
		Damage:       core.SpellEffect{Effect: 0, Min: 124, Max: 176, SP: 0.333},
		OnSpellHitDealt: func(sim *core.Simulation, spell *core.Spell, result *core.SpellResult) {
			if result.Landed() {
				aura := acidSpitAuras.Get(result.Target)
				aura.Activate(sim)
				if aura.IsActive() {
					aura.AddStack(sim)
				}
			}
		},
	})
}

func (hp *HunterPet) newDemoralizingScreech() *core.Spell {
	debuffs := hp.NewEnemyAuraArray(core.DemoralizingScreechAura)

	return hp.newSpecialAbility(PetSpecialAbilityConfig{
		Type:     DemoralizingScreech,
		Cost:     20,
		GCD:      core.GCDDefault,
		CD:       time.Second * 10,
		SpellID:  55487,
		School:   core.SpellSchoolPhysical,
		Magic:    true,
		CantCrit: true,
		Damage:   core.SpellEffect{Effect: 0, Min: 85, Max: 129, SP: 0.119658, AP: 0.07},
		OnSpellHitDealt: func(sim *core.Simulation, spell *core.Spell, result *core.SpellResult) {
			if result.Landed() {
				for _, aoeTarget := range sim.Encounter.TargetUnits {
					debuffs.Get(aoeTarget).Activate(sim)
				}
			}
		},
	})
}

func (hp *HunterPet) newFireBreath() *core.Spell {
	return hp.newSpecialAbility(PetSpecialAbilityConfig{
		Type:     FireBreath,
		Cost:     20,
		GCD:      core.GCDDefault,
		CD:       time.Second * 10,
		SpellID:  55485,
		School:   core.SpellSchoolFire,
		Magic:    true,
		CantCrit: true,
		Damage:   core.SpellEffect{Effect: 0, Min: 43, Max: 57, SP: 0.333},

		Dot: core.DotConfig{
			Aura: core.Aura{
				Label: "Fire Breath",
			},
			NumberOfTicks: 2,
			TickLength:    time.Second * 1,
			Tick:          core.SpellEffect{Effect: 1, Min: 22, Max: 28, SP: 0.167},
			OnSnapshot: func(sim *core.Simulation, target *core.Unit, dot *core.Dot, isRollover bool) {
				dot.SnapshotBaseDamage = (dot.Tick.Roll(sim) + dot.Tick.SP*dot.Spell.SpellPower()) * hp.killCommandMult()
				dot.SnapshotAttackerMultiplier = dot.Spell.AttackerDamageMultiplier(dot.Spell.Unit.AttackTables[target.UnitIndex])
			},
			OnTick: func(sim *core.Simulation, target *core.Unit, dot *core.Dot) {
				dot.CalcAndDealPeriodicSnapshotDamage(sim, target, dot.OutcomeTick)
			},
		},
		OnSpellHitDealt: func(sim *core.Simulation, spell *core.Spell, result *core.SpellResult) {
			if result.Landed() {
				spell.Dot(result.Target).Apply(sim)
			}
		},
	})
}

func (hp *HunterPet) newFroststormBreath() *core.Spell {
	return hp.newSpecialAbility(PetSpecialAbilityConfig{
		Type:    FroststormBreath,
		Cost:    20,
		GCD:     0,
		CD:      time.Second * 10,
		SpellID: 55492,
		School:  core.SpellSchoolFrost | core.SpellSchoolNature,
		Magic:   true,
		Damage:  core.SpellEffect{Effect: 0, Min: 128, Max: 172, SP: 0.333},
	})
}

func (hp *HunterPet) newFuriousHowl() *core.Spell {
	actionID := core.ActionID{SpellID: 64495}

	petAura := hp.NewTemporaryStatsAura("FuriousHowl", actionID, stats.Stats{stats.AttackPower: 320, stats.RangedAttackPower: 320}, time.Second*20)
	ownerAura := hp.hunterOwner.NewTemporaryStatsAura("FuriousHowl", actionID, stats.Stats{stats.AttackPower: 320, stats.RangedAttackPower: 320}, time.Second*20)

	howlSpell := hp.RegisterSpell(core.SpellConfig{
		ActionID: actionID,

		FocusCost: core.FocusCostOptions{
			Cost: 20,
		},
		Cast: core.CastConfig{
			CD: core.Cooldown{
				Timer:    hp.NewTimer(),
				Duration: hp.hunterOwner.applyLongevity(time.Second * 40),
			},
		},
		ExtraCastCondition: func(sim *core.Simulation, target *core.Unit) bool {
			return hp.IsEnabled()
		},
		ApplyEffects: func(sim *core.Simulation, _ *core.Unit, _ *core.Spell) {
			petAura.Activate(sim)
			ownerAura.Activate(sim)
		},
	})

	hp.hunterOwner.RegisterSpell(core.SpellConfig{
		ActionID: actionID,
		Flags:    core.SpellFlagAPL | core.SpellFlagMCD,
		ExtraCastCondition: func(sim *core.Simulation, target *core.Unit) bool {
			return howlSpell.CanCast(sim, target)
		},
		ApplyEffects: func(sim *core.Simulation, target *core.Unit, _ *core.Spell) {
			howlSpell.Cast(sim, target)
		},
	})

	hp.hunterOwner.AddMajorCooldown(core.MajorCooldown{
		Spell: howlSpell,
		Type:  core.CooldownTypeDPS,
	})

	return nil
}

func (hp *HunterPet) newGore() *core.Spell {
	return hp.newSpecialAbility(PetSpecialAbilityConfig{
		Type:    Gore,
		Cost:    20,
		GCD:     core.GCDDefault,
		CD:      time.Second * 10,
		SpellID: 35295,
		School:  core.SpellSchoolPhysical,
		Damage:  core.SpellEffect{Effect: 0, Min: 122, Max: 164, SP: 0.119658, AP: 0.07},
	})
}

func (hp *HunterPet) newLavaBreath() *core.Spell {
	return hp.newSpecialAbility(PetSpecialAbilityConfig{
		Type:    LavaBreath,
		Cost:    20,
		GCD:     core.GCDDefault,
		CD:      time.Second * 10,
		SpellID: 58611,
		School:  core.SpellSchoolFire,
		Magic:   true,
		Damage:  core.SpellEffect{Effect: 0, Min: 128, Max: 172, SP: 0.333},
	})
}

func (hp *HunterPet) newLightningBreath() *core.Spell {
	return hp.newSpecialAbility(PetSpecialAbilityConfig{
		Type:    LightningBreath,
		Cost:    20,
		GCD:     core.GCDDefault,
		CD:      time.Second * 10,
		SpellID: 25012,
		School:  core.SpellSchoolNature,
		Magic:   true,
		Damage:  core.SpellEffect{Effect: 0, Min: 80, Max: 120, SP: 0.333},
	})
}

func (hp *HunterPet) newMonstrousBite() *core.Spell {
	procAura := hp.RegisterAura(core.Aura{
		Label:     "Monstrous Bite",
		ActionID:  core.ActionID{SpellID: 55499},
		Duration:  time.Second * 12,
		MaxStacks: 3,
		OnStacksChange: func(aura *core.Aura, sim *core.Simulation, oldStacks int32, newStacks int32) {
			aura.Unit.PseudoStats.DamageDealtMultiplier /= math.Pow(1.03, float64(oldStacks))
			aura.Unit.PseudoStats.DamageDealtMultiplier *= math.Pow(1.03, float64(newStacks))
		},
	})

	return hp.newSpecialAbility(PetSpecialAbilityConfig{
		Type:    MonstrousBite,
		Cost:    20,
		GCD:     core.GCDDefault,
		CD:      time.Second * 10,
		SpellID: 55499,
		School:  core.SpellSchoolPhysical,
		Damage:  core.SpellEffect{Effect: 0, Min: 91, Max: 123, SP: 0.119658, AP: 0.07},
		OnSpellHitDealt: func(sim *core.Simulation, spell *core.Spell, result *core.SpellResult) {
			if result.Landed() {
				procAura.Activate(sim)
				procAura.AddStack(sim)
			}
		},
	})
}

func (hp *HunterPet) newNetherShock() *core.Spell {
	return hp.newSpecialAbility(PetSpecialAbilityConfig{
		Type:    NetherShock,
		Cost:    20,
		CD:      time.Second * 40,
		SpellID: 53589,
		School:  core.SpellSchoolShadow,
		Magic:   true,
		Damage:  core.SpellEffect{Effect: 1, Min: 64, Max: 86, SP: 0.333},
	})
}

func (hp *HunterPet) newPin() *core.Spell {
	return hp.RegisterSpell(core.SpellConfig{
		ActionID:    core.ActionID{SpellID: 53548},
		SpellSchool: core.SpellSchoolPhysical,
		ProcMask:    core.ProcMaskEmpty,

		Cast: core.CastConfig{
			CD: core.Cooldown{
				Timer:    hp.NewTimer(),
				Duration: hp.hunterOwner.applyLongevity(time.Second * 40),
			},
		},

		DamageMultiplier: 1 * hp.hunterOwner.markedForDeathMultiplier(),
		ThreatMultiplier: 1,

		Dot: core.DotConfig{
			Aura: core.Aura{
				Label: "Pin",
			},
			NumberOfTicks: 4,
			TickLength:    time.Second * 1,
			Tick:          core.SpellEffect{Effect: 1, Min: 28, Max: 36},
			OnSnapshot: func(sim *core.Simulation, target *core.Unit, dot *core.Dot, isRollover bool) {
				dot.SnapshotBaseDamage = dot.Tick.Roll(sim)
				dot.SnapshotBaseDamage *= hp.killCommandMult()
				dot.SnapshotAttackerMultiplier = dot.Spell.AttackerDamageMultiplier(dot.Spell.Unit.AttackTables[target.UnitIndex])
			},
			OnTick: func(sim *core.Simulation, target *core.Unit, dot *core.Dot) {
				dot.CalcAndDealPeriodicSnapshotDamage(sim, target, dot.OutcomeTick)
			},
		},

		ApplyEffects: func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
			result := spell.CalcAndDealOutcome(sim, target, spell.OutcomeMeleeSpecialHit)
			if result.Landed() {
				spell.Dot(result.Target).Apply(sim)
			}
		},
	})
}

func (hp *HunterPet) newPoisonSpit() *core.Spell {
	return hp.RegisterSpell(core.SpellConfig{
		ActionID:     core.ActionID{SpellID: 55557},
		SpellSchool:  core.SpellSchoolNature,
		ProcMask:     core.ProcMaskEmpty,
		MissileSpeed: 40,

		FocusCost: core.FocusCostOptions{
			Cost: 20,
		},
		Cast: core.CastConfig{
			DefaultCast: core.Cast{
				GCD: core.GCDDefault,
			},
			IgnoreHaste: true,
			CD: core.Cooldown{
				Timer:    hp.NewTimer(),
				Duration: hp.hunterOwner.applyLongevity(time.Second * 10),
			},
		},

		DamageMultiplier: 1 * hp.hunterOwner.markedForDeathMultiplier(),
		ThreatMultiplier: 1,

		Dot: core.DotConfig{
			Aura: core.Aura{
				Label: "PoisonSpit",
			},
			NumberOfTicks: 4,
			TickLength:    time.Second * 2,
			Tick:          core.SpellEffect{Effect: 0, Min: 26, Max: 34, SP: 0.067},
			OnSnapshot: func(sim *core.Simulation, target *core.Unit, dot *core.Dot, isRollover bool) {
				dot.SnapshotBaseDamage = dot.Tick.Roll(sim) + dot.Tick.SP*dot.Spell.SpellPower()
				dot.SnapshotBaseDamage *= hp.killCommandMult()
				dot.SnapshotAttackerMultiplier = dot.Spell.AttackerDamageMultiplier(dot.Spell.Unit.AttackTables[target.UnitIndex])
			},
			OnTick: func(sim *core.Simulation, target *core.Unit, dot *core.Dot) {
				dot.CalcAndDealPeriodicSnapshotDamage(sim, target, dot.OutcomeTick)
			},
		},

		ApplyEffects: func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
			result := spell.CalcOutcome(sim, target, spell.OutcomeMagicHit)
			spell.WaitTravelTime(sim, func(sim *core.Simulation) {
				spell.DealOutcome(sim, result)
				if result.Landed() {
					spell.Dot(result.Target).Apply(sim)
				}
			})
		},
	})
}

func (hp *HunterPet) newRake() *core.Spell {
	return hp.newSpecialAbility(PetSpecialAbilityConfig{
		Type:    Rake,
		Cost:    20,
		GCD:     core.GCDDefault,
		CD:      time.Second * 10,
		SpellID: 59886,
		School:  core.SpellSchoolPhysical,
		Damage:  core.SpellEffect{Effect: 0, Min: 47, Max: 67, SP: 0.0299, AP: 0.0175},
		Dot: core.DotConfig{
			Aura: core.Aura{
				Label: "Rake",
			},
			NumberOfTicks: 3,
			TickLength:    time.Second * 3,
			Tick:          core.SpellEffect{Effect: 1, Min: 19, Max: 25, SP: 0.0299, AP: 0.0175},
			OnSnapshot: func(sim *core.Simulation, target *core.Unit, dot *core.Dot, isRollover bool) {
				dot.SnapshotBaseDamage = dot.Tick.Roll(sim) + dot.Tick.AP*dot.Spell.MeleeAttackPower()
				dot.SnapshotBaseDamage *= hp.killCommandMult()
				dot.SnapshotAttackerMultiplier = dot.Spell.AttackerDamageMultiplier(dot.Spell.Unit.AttackTables[target.UnitIndex])
			},
			OnTick: func(sim *core.Simulation, target *core.Unit, dot *core.Dot) {
				dot.CalcAndDealPeriodicSnapshotDamage(sim, target, dot.OutcomeTick)
			},
		},
		OnSpellHitDealt: func(sim *core.Simulation, spell *core.Spell, result *core.SpellResult) {
			if result.Landed() {
				spell.Dot(result.Target).Apply(sim)
			}
		},
	})
}

func (hp *HunterPet) newRavage() *core.Spell {
	return hp.newSpecialAbility(PetSpecialAbilityConfig{
		Type:    Ravage,
		Cost:    0,
		CD:      time.Second * 40,
		SpellID: 53562,
		School:  core.SpellSchoolPhysical,
		Damage:  core.SpellEffect{Effect: 0, Min: 106, Max: 150, SP: 0.119658, AP: 0.07},
	})
}

func (hp *HunterPet) newSavageRend() *core.Spell {
	actionID := core.ActionID{SpellID: 53582}

	procAura := hp.RegisterAura(core.Aura{
		Label:    "Savage Rend",
		ActionID: actionID,
		Duration: time.Second * 30,
		OnGain: func(aura *core.Aura, sim *core.Simulation) {
			aura.Unit.PseudoStats.DamageDealtMultiplier *= 1.1
		},
		OnExpire: func(aura *core.Aura, sim *core.Simulation) {
			aura.Unit.PseudoStats.DamageDealtMultiplier /= 1.1
		},
	})

	srSpell := hp.RegisterSpell(core.SpellConfig{
		ActionID:    actionID,
		SpellSchool: core.SpellSchoolPhysical,
		ProcMask:    core.ProcMaskSpellDamage,
		Flags:       core.SpellFlagMeleeMetrics | core.SpellFlagIncludeTargetBonusDamage | core.SpellFlagApplyArmorReduction,

		FocusCost: core.FocusCostOptions{
			Cost: 20,
		},
		Cast: core.CastConfig{
			CD: core.Cooldown{
				Timer:    hp.NewTimer(),
				Duration: hp.hunterOwner.applyLongevity(time.Second * 60),
			},
		},
		ExtraCastCondition: func(sim *core.Simulation, target *core.Unit) bool {
			return hp.IsEnabled()
		},

		DamageMultiplier: 1 * hp.hunterOwner.markedForDeathMultiplier(),
		CritMultiplier:   2,
		ThreatMultiplier: 1,

		Dot: core.DotConfig{
			Aura: core.Aura{
				Label: "SavageRend",
			},
			NumberOfTicks: 3,
			TickLength:    time.Second * 5,
			Tick:          core.SpellEffect{Effect: 1, Min: 21, Max: 27},
			OnSnapshot: func(sim *core.Simulation, target *core.Unit, dot *core.Dot, isRollover bool) {
				dot.SnapshotBaseDamage = dot.Tick.Roll(sim)
				dot.SnapshotBaseDamage *= hp.killCommandMult()
				dot.SnapshotAttackerMultiplier = dot.Spell.AttackerDamageMultiplier(dot.Spell.Unit.AttackTables[target.UnitIndex])
			},
			OnTick: func(sim *core.Simulation, target *core.Unit, dot *core.Dot) {
				dot.CalcAndDealPeriodicSnapshotDamage(sim, target, dot.OutcomeTick)
			},
		},

		// SP unused: the pet's spell damage only reaches the magic schools
		Direct: core.SpellEffect{Effect: 0, Min: 59, Max: 83, SP: 0.119658, AP: 0.07},

		ApplyEffects: func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
			baseDamage := spell.Direct.Roll(sim) + spell.Direct.AP*spell.MeleeAttackPower()
			baseDamage *= hp.killCommandMult()
			result := spell.CalcAndDealDamage(sim, target, baseDamage, spell.OutcomeMeleeSpecialHitAndCrit)

			if result.Landed() {
				spell.Dot(target).Apply(sim)
				if result.DidCrit() {
					procAura.Activate(sim)
				}
			}
		},
	})

	hp.hunterOwner.AddMajorCooldown(core.MajorCooldown{
		Spell: srSpell,
		Type:  core.CooldownTypeDPS,
	})

	return nil
}

func (hp *HunterPet) newScorpidPoison() *core.Spell {
	return hp.RegisterSpell(core.SpellConfig{
		ActionID:    core.ActionID{SpellID: 55728},
		SpellSchool: core.SpellSchoolNature,
		ProcMask:    core.ProcMaskEmpty,

		FocusCost: core.FocusCostOptions{
			Cost: 20,
		},
		Cast: core.CastConfig{
			DefaultCast: core.Cast{
				GCD: core.GCDDefault,
			},
			IgnoreHaste: true,
			CD: core.Cooldown{
				Timer:    hp.NewTimer(),
				Duration: hp.hunterOwner.applyLongevity(time.Second * 10),
			},
		},

		DamageMultiplier: 1 * hp.hunterOwner.markedForDeathMultiplier(),
		ThreatMultiplier: 1,

		Dot: core.DotConfig{
			Aura: core.Aura{
				Label: "ScorpidPoison",
			},
			NumberOfTicks: 5,
			TickLength:    time.Second * 2,
			Tick:          core.SpellEffect{Effect: 1, Min: 20, Max: 26, SP: 0.067},
			OnSnapshot: func(sim *core.Simulation, target *core.Unit, dot *core.Dot, isRollover bool) {
				dot.SnapshotBaseDamage = dot.Tick.Roll(sim) + dot.Tick.SP*dot.Spell.SpellPower()
				dot.SnapshotBaseDamage *= hp.killCommandMult()
				dot.SnapshotAttackerMultiplier = dot.Spell.AttackerDamageMultiplier(dot.Spell.Unit.AttackTables[target.UnitIndex])
			},
			OnTick: func(sim *core.Simulation, target *core.Unit, dot *core.Dot) {
				dot.CalcAndDealPeriodicSnapshotDamage(sim, target, dot.OutcomeTick)
			},
		},

		ApplyEffects: func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
			result := spell.CalcAndDealOutcome(sim, target, spell.OutcomeMeleeSpecialHit)
			if result.Landed() {
				spell.Dot(target).Apply(sim)
			}
		},
	})
}

func (hp *HunterPet) newSnatch() *core.Spell {
	return hp.newSpecialAbility(PetSpecialAbilityConfig{
		Type:    Snatch,
		Cost:    20,
		CD:      time.Second * 60,
		SpellID: 53543,
		School:  core.SpellSchoolPhysical,
		Damage:  core.SpellEffect{Effect: 0, Min: 89, Max: 125, SP: 0.119658, AP: 0.07},
	})
}

func (hp *HunterPet) newSonicBlast() *core.Spell {
	return hp.newSpecialAbility(PetSpecialAbilityConfig{
		Type:    SonicBlast,
		Cost:    80,
		CD:      time.Second * 60,
		SpellID: 53568,
		School:  core.SpellSchoolNature,
		Magic:   true,
		Damage:  core.SpellEffect{Effect: 0, Min: 62, Max: 88, SP: 0.333},
	})
}

func (hp *HunterPet) newSpiritStrike() *core.Spell {
	return hp.newSpecialAbility(PetSpecialAbilityConfig{
		Type:    SpiritStrike,
		Cost:    20,
		GCD:     0,
		CD:      time.Second * 10,
		SpellID: 61198,
		School:  core.SpellSchoolArcane,
		Magic:   true,
		Damage:  core.SpellEffect{Effect: 1, Min: 49, Max: 65, SP: 0.333},

		Dot: core.DotConfig{
			Aura: core.Aura{
				Label: "SpiritStrike",
			},
			NumberOfTicks: 1,
			TickLength:    time.Second * 6,
			Tick:          core.SpellEffect{Effect: 0, Min: 49, Max: 65, SP: 0.333},
			OnSnapshot: func(sim *core.Simulation, target *core.Unit, dot *core.Dot, isRollover bool) {
				dot.SnapshotBaseDamage = dot.Tick.Roll(sim) + dot.Tick.SP*dot.Spell.SpellPower()
				dot.SnapshotBaseDamage *= hp.killCommandMult()
				dot.SnapshotAttackerMultiplier = dot.Spell.AttackerDamageMultiplier(dot.Spell.Unit.AttackTables[target.UnitIndex])
			},
			OnTick: func(sim *core.Simulation, target *core.Unit, dot *core.Dot) {
				dot.CalcAndDealPeriodicSnapshotDamage(sim, target, dot.OutcomeTick)
			},
		},
		OnSpellHitDealt: func(sim *core.Simulation, spell *core.Spell, result *core.SpellResult) {
			if result.Landed() {
				spell.Dot(result.Target).Apply(sim)
			}
		},
	})
}

func (hp *HunterPet) newSporeCloud() *core.Spell {
	debuffs := hp.NewEnemyAuraArray(core.SporeCloudAura)
	return hp.RegisterSpell(core.SpellConfig{
		ActionID:    core.ActionID{SpellID: 53598},
		SpellSchool: core.SpellSchoolNature,
		ProcMask:    core.ProcMaskSpellDamage,

		FocusCost: core.FocusCostOptions{
			Cost: 20,
		},
		Cast: core.CastConfig{
			DefaultCast: core.Cast{
				GCD: core.GCDDefault,
			},
			IgnoreHaste: true,
			CD: core.Cooldown{
				Timer:    hp.NewTimer(),
				Duration: hp.hunterOwner.applyLongevity(time.Second * 10),
			},
		},

		DamageMultiplier: 1 * hp.hunterOwner.markedForDeathMultiplier(),
		ThreatMultiplier: 1,

		Dot: core.DotConfig{
			IsAOE: true,
			Aura: core.Aura{
				Label: "SporeCloud",
			},
			NumberOfTicks: 3,
			TickLength:    time.Second * 3,
			Tick:          core.SpellEffect{Effect: 0, Min: 22, Max: 28, SP: 0.333},
			// an AOE dot sits on the pet, so snapshot against its target
			OnSnapshot: func(sim *core.Simulation, _ *core.Unit, dot *core.Dot, isRollover bool) {
				dot.SnapshotBaseDamage = dot.Tick.Roll(sim) + dot.Tick.SP*dot.Spell.SpellPower()
				dot.SnapshotBaseDamage *= hp.killCommandMult()
				dot.SnapshotAttackerMultiplier = dot.Spell.AttackerDamageMultiplier(dot.Spell.Unit.AttackTables[hp.CurrentTarget.UnitIndex])
			},
			OnTick: func(sim *core.Simulation, _ *core.Unit, dot *core.Dot) {
				for _, aoeTarget := range sim.Encounter.TargetUnits {
					dot.CalcAndDealPeriodicSnapshotDamage(sim, aoeTarget, dot.OutcomeTick)
				}
			},
		},

		ApplyEffects: func(sim *core.Simulation, _ *core.Unit, spell *core.Spell) {
			spell.AOEDot().Apply(sim)
			for _, target := range spell.Unit.Env.Encounter.TargetUnits {
				debuffs.Get(target).Activate(sim)
			}
		},
	})
}

func (hp *HunterPet) newStampede() *core.Spell {
	debuffs := hp.NewEnemyAuraArray(core.StampedeAura)
	return hp.newSpecialAbility(PetSpecialAbilityConfig{
		Type:    Stampede,
		Cost:    0,
		CD:      time.Second * 60,
		SpellID: 57393,
		School:  core.SpellSchoolPhysical,
		Damage:  core.SpellEffect{Effect: 0, Min: 182, Max: 246, SP: 0.333},
		OnSpellHitDealt: func(sim *core.Simulation, spell *core.Spell, result *core.SpellResult) {
			if result.Landed() {
				debuffs.Get(result.Target).Activate(sim)
			}
		},
	})
}

func (hp *HunterPet) newSting() *core.Spell {
	debuffs := hp.NewEnemyAuraArray(core.StingAura)
	return hp.newSpecialAbility(PetSpecialAbilityConfig{
		Type:    Sting,
		Cost:    20,
		GCD:     core.GCDDefault,
		CD:      time.Second * 6,
		SpellID: 56631,
		School:  core.SpellSchoolNature,
		Damage:  core.SpellEffect{Effect: 0, Min: 64, Max: 86, SP: 0.333},
		OnSpellHitDealt: func(sim *core.Simulation, spell *core.Spell, result *core.SpellResult) {
			if result.Landed() {
				debuffs.Get(result.Target).Activate(sim)
			}
		},
	})
}

func (hp *HunterPet) newSwipe() *core.Spell {
	// TODO: This is frontal cone, but might be more realistic as single-target
	// since pets are hard to control.
	return hp.newSpecialAbility(PetSpecialAbilityConfig{
		Type:    Swipe,
		Cost:    20,
		GCD:     core.GCDDefault,
		CD:      time.Second * 5,
		SpellID: 53533,
		School:  core.SpellSchoolPhysical,
		Damage:  core.SpellEffect{Effect: 0, Min: 90, Max: 126, SP: 0.119658, AP: 0.07},
	})
}

func (hp *HunterPet) newTendonRip() *core.Spell {
	return hp.newSpecialAbility(PetSpecialAbilityConfig{
		Type:    TendonRip,
		Cost:    20,
		CD:      time.Second * 20,
		SpellID: 53575,
		School:  core.SpellSchoolPhysical,
		Damage:  core.SpellEffect{Effect: 1, Min: 49, Max: 69, SP: 0.333},
	})
}

func (hp *HunterPet) newVenomWebSpray() *core.Spell {
	return hp.RegisterSpell(core.SpellConfig{
		ActionID:    core.ActionID{SpellID: 55509},
		SpellSchool: core.SpellSchoolNature,
		ProcMask:    core.ProcMaskEmpty,

		Cast: core.CastConfig{
			CD: core.Cooldown{
				Timer:    hp.NewTimer(),
				Duration: hp.hunterOwner.applyLongevity(time.Second * 40),
			},
		},

		DamageMultiplier: 1 * hp.hunterOwner.markedForDeathMultiplier(),
		ThreatMultiplier: 1,

		Dot: core.DotConfig{
			Aura: core.Aura{
				Label: "VenomWebSpray",
			},
			NumberOfTicks: 4,
			TickLength:    time.Second * 1,
			Tick:          core.SpellEffect{Effect: 1, Min: 46, Max: 68, SP: 0.333},
			OnSnapshot: func(sim *core.Simulation, target *core.Unit, dot *core.Dot, isRollover bool) {
				dot.SnapshotBaseDamage = dot.Tick.Roll(sim) + dot.Tick.SP*dot.Spell.SpellPower()
				dot.SnapshotBaseDamage *= hp.killCommandMult()
				dot.SnapshotAttackerMultiplier = dot.Spell.AttackerDamageMultiplier(dot.Spell.Unit.AttackTables[target.UnitIndex])
			},
			OnTick: func(sim *core.Simulation, target *core.Unit, dot *core.Dot) {
				dot.CalcAndDealPeriodicSnapshotDamage(sim, target, dot.OutcomeTick)
			},
		},

		ApplyEffects: func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
			result := spell.CalcAndDealOutcome(sim, target, spell.OutcomeMagicHit)
			if result.Landed() {
				spell.Dot(target).Apply(sim)
			}
		},
	})
}
