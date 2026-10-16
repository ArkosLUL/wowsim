package warlock

import (
	"time"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
	"github.com/wowsims/wotlk/sim/core/stats"
)

type WarlockPet struct {
	*core.Pet

	owner *Warlock

	primaryAbility   *core.Spell
	secondaryAbility *core.Spell

	DemonicEmpowermentAura *core.Aura

	// Improved Demonic Tactics' crit: what the warlock's aura holds since its last tick, and what of it
	// is on the pet
	tacticsCrit, tacticsCritOnPet stats.Stats
}

func (warlock *Warlock) NewWarlockPet() *WarlockPet {
	// pet_levelstats at 80, its weapon range included. Guardian::UpdateMaxPower gives a demon 11.5 mana a
	// point of intellect over the create value, the Imp 4.95. Guardian::UpdateMaxHealth has its own
	// health a point of stamina for each
	var cfg struct {
		Name             string
		Stats            stats.Stats
		CreateHealth     float64
		HealthPerStamina float64
		CreateMana       float64
		ManaPerIntellect float64
		AutoAttacks      core.AutoAttackOptions
	}
	creatureCrit := stats.Stats{
		stats.MeleeCrit: 5 * core.CritRatingPerCritChance,
		stats.SpellCrit: 5 * core.CritRatingPerCritChance,
	}

	switch warlock.Options.Summon {
	case proto.Warlock_Options_Felguard:
		cfg.Name = "Felguard"
		cfg.Stats = core.PetCreateStats(314, 90, 328, 150, 209, 14033)
		cfg.CreateHealth, cfg.HealthPerStamina = 5428, 11
		cfg.CreateMana, cfg.ManaPerIntellect = 3077, 11.5
		cfg.AutoAttacks = core.AutoAttackOptions{
			MainHand: core.Weapon{
				BaseDamageMin:  329,
				BaseDamageMax:  495,
				SwingSpeed:     2,
				CritMultiplier: 2,
			},
			AutoSwingMelee: true,
		}
	case proto.Warlock_Options_Imp:
		cfg.Name = "Imp"
		cfg.Stats = core.PetCreateStats(297, 79, 118, 369, 367, 6273)
		cfg.CreateHealth, cfg.HealthPerStamina = 3867, 8.4
		cfg.CreateMana, cfg.ManaPerIntellect = 2908, 4.95
	case proto.Warlock_Options_Succubus:
		cfg.Name = "Succubus"
		cfg.Stats = core.PetCreateStats(314, 90, 328, 150, 209, 9706)
		cfg.CreateHealth, cfg.HealthPerStamina = 4567, 9.1
		cfg.CreateMana, cfg.ManaPerIntellect = 3077, 11.5
		cfg.AutoAttacks = core.AutoAttackOptions{
			MainHand: core.Weapon{
				BaseDamageMin:  350,
				BaseDamageMax:  524,
				SwingSpeed:     2,
				CritMultiplier: 2,
			},
			AutoSwingMelee: true,
		}
	case proto.Warlock_Options_Felhunter:
		cfg.Name = "Felhunter"
		cfg.Stats = core.PetCreateStats(314, 90, 328, 150, 209, 7782)
		cfg.CreateHealth, cfg.HealthPerStamina = 4787, 9.5
		cfg.CreateMana, cfg.ManaPerIntellect = 3077, 11.5
		cfg.AutoAttacks = core.AutoAttackOptions{
			MainHand: core.Weapon{
				BaseDamageMin:  246,
				BaseDamageMax:  379,
				SwingSpeed:     2,
				CritMultiplier: 2,
			},
			AutoSwingMelee: true,
		}
	}
	isImp := warlock.Options.Summon == proto.Warlock_Options_Imp

	wp := &WarlockPet{
		Pet:   core.NewPet(cfg.Name, &warlock.Character, cfg.Stats.Add(creatureCrit), warlock.makeStatInheritance(), true, false),
		owner: warlock,
	}
	wp.SummonedAsPet = true
	// spell_warl_generic_scaling's HandlePeriodic
	wp.ScalingPeriod = 2 * time.Second

	wp.SetCreateHealth(cfg.CreateHealth, cfg.Stats[stats.Stamina], cfg.HealthPerStamina)
	wp.EnableCreatureManaBar(cfg.CreateMana, cfg.Stats[stats.Intellect], cfg.ManaPerIntellect, isImp)
	if isImp {
		// casts Firebolt from where it follows the warlock
		wp.DistanceFromTarget = wp.FollowSpotDistance()
	}

	// Guardian::UpdateAttackPowerAndDamage: the Imp's is strength - 10
	if isImp {
		wp.AddStatDependency(stats.Strength, stats.AttackPower, 1)
		wp.AddStat(stats.AttackPower, -10)
	} else {
		wp.AddStatDependency(stats.Strength, stats.AttackPower, 2)
		wp.AddStat(stats.AttackPower, -20)
	}

	wp.AddStats(stats.Stats{
		stats.MeleeCrit: float64(warlock.Talents.DemonicTactics) * 2 * core.CritRatingPerCritChance,
		stats.SpellCrit: float64(warlock.Talents.DemonicTactics) * 2 * core.CritRatingPerCritChance,
	})

	wp.PseudoStats.DamageDealtMultiplier *= 1.0 + 0.04*float64(warlock.Talents.UnholyPower)

	if warlock.Options.Summon != proto.Warlock_Options_Imp { // imps generally don't meele
		wp.EnableAutoAttacks(wp, cfg.AutoAttacks)
	}

	if warlock.Options.Summon == proto.Warlock_Options_Felguard {
		if wp.owner.HasMajorGlyph(proto.WarlockMajorGlyph_GlyphOfFelguard) {
			wp.MultiplyStat(stats.AttackPower, 1.2)
		}

		statDeps := []*stats.StatDependency{nil}
		for i := 1; i <= 10; i++ {
			statDeps = append(statDeps, wp.NewDynamicMultiplyStat(stats.AttackPower,
				1+float64(i)*(0.05+0.01*float64(warlock.Talents.DemonicBrutality))))
		}

		DemonicFrenzyAura := wp.RegisterAura(core.Aura{
			Label:     "Demonic Frenzy",
			ActionID:  core.ActionID{SpellID: 32851},
			Duration:  time.Second * 10,
			MaxStacks: 10,
			OnStacksChange: func(aura *core.Aura, sim *core.Simulation, oldStacks int32, newStacks int32) {
				if oldStacks != 0 {
					aura.Unit.DisableDynamicStatDep(sim, statDeps[oldStacks])
				}
				if newStacks != 0 {
					aura.Unit.EnableDynamicStatDep(sim, statDeps[newStacks])
				}
			},
		})
		wp.RegisterAura(core.Aura{
			Label:    "Demonic Frenzy Hidden Aura",
			Duration: core.NeverExpires,
			OnReset: func(aura *core.Aura, sim *core.Simulation) {
				aura.Activate(sim)
			},
			OnSpellHitDealt: func(aura *core.Aura, sim *core.Simulation, spell *core.Spell, result *core.SpellResult) {
				if !result.Landed() || !spell.ProcMask.Matches(core.ProcMaskMelee) {
					return
				}
				DemonicFrenzyAura.Activate(sim)
				DemonicFrenzyAura.AddStack(sim)
			},
		})
	}

	if warlock.Talents.FelVitality > 0 {
		bonus := 1.0 + 0.05*float64(warlock.Talents.FelVitality)
		wp.MultiplyStat(stats.Intellect, bonus)
		wp.MultiplyStat(stats.Stamina, bonus)
	}

	var enableMasterDemonologist, disableMasterDemonologist func(sim *core.Simulation)
	if warlock.Talents.MasterDemonologist > 0 {
		val := 1.0 + 0.01*float64(warlock.Talents.MasterDemonologist)
		md := core.Aura{
			Label:    "Master Demonologist",
			ActionID: core.ActionID{SpellID: 35706}, // many different spells associated with this talent
			Duration: core.NeverExpires,
			OnGain: func(aura *core.Aura, _ *core.Simulation) {
				switch warlock.Options.Summon {
				case proto.Warlock_Options_Imp:
					aura.Unit.PseudoStats.SchoolDamageDealtMultiplier[stats.SchoolIndexFire] *= val
				case proto.Warlock_Options_Succubus:
					aura.Unit.PseudoStats.SchoolDamageDealtMultiplier[stats.SchoolIndexShadow] *= val
				case proto.Warlock_Options_Felguard:
					aura.Unit.PseudoStats.DamageDealtMultiplier *= val
				}
			},
			OnExpire: func(aura *core.Aura, _ *core.Simulation) {
				switch warlock.Options.Summon {
				case proto.Warlock_Options_Imp:
					aura.Unit.PseudoStats.SchoolDamageDealtMultiplier[stats.SchoolIndexFire] /= val
				case proto.Warlock_Options_Succubus:
					aura.Unit.PseudoStats.SchoolDamageDealtMultiplier[stats.SchoolIndexShadow] /= val
				case proto.Warlock_Options_Felguard:
					aura.Unit.PseudoStats.DamageDealtMultiplier /= val
				}
			},
		}

		mdLockAura := warlock.RegisterAura(md)
		mdPetAura := wp.RegisterAura(md)

		masterDemonologist := float64(warlock.Talents.MasterDemonologist) * core.CritRatingPerCritChance
		masterDemonologistFireCrit := core.TernaryFloat64(warlock.Options.Summon == proto.Warlock_Options_Imp, masterDemonologist, 0)
		masterDemonologistShadowCrit := core.TernaryFloat64(warlock.Options.Summon == proto.Warlock_Options_Succubus, masterDemonologist, 0)

		enableMasterDemonologist = func(sim *core.Simulation) {
			mdLockAura.Activate(sim)
			mdPetAura.Activate(sim)

			spellbook := make([]*core.Spell, 0)
			spellbook = append(spellbook, warlock.Spellbook...)
			spellbook = append(spellbook, wp.Spellbook...)

			for _, spell := range spellbook {
				if spell.SpellSchool.Matches(core.SpellSchoolFire) {
					spell.BonusCritRating += masterDemonologistFireCrit
				}

				if spell.SpellSchool.Matches(core.SpellSchoolShadow) {
					spell.BonusCritRating += masterDemonologistShadowCrit
				}
			}
		}

		disableMasterDemonologist = func(sim *core.Simulation) {
			mdLockAura.Deactivate(sim)
			mdPetAura.Deactivate(sim)

			spellbook := make([]*core.Spell, 0)
			spellbook = append(spellbook, warlock.Spellbook...)
			spellbook = append(spellbook, wp.Spellbook...)

			for _, spell := range spellbook {
				if spell.SpellSchool.Matches(core.SpellSchoolFire) {
					spell.BonusCritRating -= masterDemonologistFireCrit
				}

				if spell.SpellSchool.Matches(core.SpellSchoolShadow) {
					spell.BonusCritRating -= masterDemonologistShadowCrit
				}
			}
		}
	}

	wp.OnPetEnable = func(sim *core.Simulation) {
		if enableMasterDemonologist != nil {
			enableMasterDemonologist(sim)
		}
		wp.applyTacticsCrit(sim)
	}
	wp.OnPetDisable = func(sim *core.Simulation) {
		if disableMasterDemonologist != nil {
			disableMasterDemonologist(sim)
		}
		// a demon keeps what it has from its owner when it goes, but the crit comes back with the passive
		wp.AddStatsDynamic(sim, wp.tacticsCritOnPet.Invert())
		wp.tacticsCritOnPet = stats.Stats{}
	}

	core.ApplyPetConsumeEffects(&wp.Character, warlock.Consumes)

	warlock.AddPet(wp)

	return wp
}

// improvedDemonicTacticsPeriod is spell_warl_improved_demonic_tactics' CalcPeriodic.
const improvedDemonicTacticsPeriod = 10 * time.Second

// startImprovedDemonicTactics is the talent's aura on the warlock (54347) as a fight starts. Its two
// amounts are flat spell mods on the demon's passive 35695: a tenth a point of the warlock's sheet spell
// crit as the demon's spell crit, and the same share of his melee crit as its melee crit. The script
// reworks them every 10 s, so the demon runs up to that far behind.
func (wp *WarlockPet) startImprovedDemonicTactics(sim *core.Simulation) {
	wp.tacticsCrit, wp.tacticsCritOnPet = stats.Stats{}, stats.Stats{}
	if wp.owner.Talents.ImprovedDemonicTactics == 0 {
		return
	}

	// the aura has been ticking since long before the fight: its last tick saw him as he stands here,
	// and the next comes anywhere in the period
	wp.tacticsCrit = wp.tacticsCritNow()

	due := sim.PrepullStartTime() + time.Duration(sim.RandomFloat("Improved Demonic Tactics")*float64(improvedDemonicTacticsPeriod))
	// his aura ticks on his own update, after what he cast in that world tick
	pa := &core.PendingAction{NextActionAt: sim.NextWorldTick(due), Priority: core.ActionPriorityLow}
	pa.OnAction = func(sim *core.Simulation) {
		wp.tacticsCrit = wp.tacticsCritNow()
		if wp.IsEnabled() {
			wp.applyTacticsCrit(sim)
		}
		due += improvedDemonicTacticsPeriod
		pa.NextActionAt = sim.NextWorldTick(due)
		sim.AddPendingAction(pa)
	}
	sim.AddPendingAction(pa)
}

// tacticsCritNow is the script's CalculateAmount for both effects: CalculatePct<int32, float> of the
// crit chance on the warlock's sheet, which truncates the chance and then the share.
func (wp *WarlockPet) tacticsCritNow() stats.Stats {
	pct := float32(10 * wp.owner.Talents.ImprovedDemonicTactics)
	share := func(ownerCrit float64) float64 {
		chance := int32(ownerCrit / core.CritRatingPerCritChance)
		return float64(int32(float32(chance)*pct/100)) * core.CritRatingPerCritChance
	}
	return stats.Stats{
		stats.SpellCrit: share(wp.owner.GetStat(stats.SpellCrit)),
		stats.MeleeCrit: share(wp.owner.GetStat(stats.MeleeCrit)),
	}
}

// applyTacticsCrit moves the demon to what the warlock's aura holds now.
func (wp *WarlockPet) applyTacticsCrit(sim *core.Simulation) {
	if wp.tacticsCrit == wp.tacticsCritOnPet {
		return
	}
	wp.AddStatsDynamic(sim, wp.tacticsCrit.Subtract(wp.tacticsCritOnPet))
	wp.tacticsCritOnPet = wp.tacticsCrit
}

func (wp *WarlockPet) GetPet() *core.Pet {
	return wp.Pet
}

func (wp *WarlockPet) Initialize() {
	switch wp.owner.Options.Summon {
	case proto.Warlock_Options_Felguard:
		wp.registerCleaveSpell()
		wp.registerInterceptSpell()
	case proto.Warlock_Options_Succubus:
		wp.registerLashOfPainSpell()
	case proto.Warlock_Options_Felhunter:
		wp.registerShadowBiteSpell()
	case proto.Warlock_Options_Imp:
		wp.registerFireboltSpell()
	}
}

func (wp *WarlockPet) Reset(sim *core.Simulation) {
	wp.startImprovedDemonicTactics(sim)
}

func (wp *WarlockPet) ExecuteCustomRotation(sim *core.Simulation) {
	if !wp.primaryAbility.IsReady(sim) {
		wp.WaitUntil(sim, wp.primaryAbility.CD.ReadyAt())
		return
	}

	wp.primaryAbility.Cast(sim, wp.CurrentTarget)
}

// makeStatInheritance is spell_warl_generic_scaling: 75% of the owner's stamina, 30% of his intellect,
// 35% of his armor, and 57% and 15% of his spell power as attack and spell power.
func (warlock *Warlock) makeStatInheritance() core.PetStatInheritance {
	return func(ownerStats stats.Stats) stats.Stats {
		// TODO: Account for sunfire/soulfrost
		return stats.Stats{
			stats.Stamina:          core.PetScalingPct(ownerStats[stats.Stamina], 75),
			stats.Intellect:        core.PetScalingPct(ownerStats[stats.Intellect], 30),
			stats.Armor:            core.PetScalingPct(ownerStats[stats.Armor], 35),
			stats.AttackPower:      core.PetScalingPct(ownerStats[stats.SpellPower], 57),
			stats.SpellPower:       core.PetScalingPct(ownerStats[stats.SpellPower], 15),
			stats.SpellPenetration: ownerStats[stats.SpellPenetration],
			// not MeleeHit/SpellHit/Expertise: pet.ownerHitScaling owns those and overwrites
			// whatever this returns for them (spell_pet_hit_expertise_scalling)
		}
	}
}
