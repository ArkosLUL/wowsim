package deathknight

import (
	"math"
	"time"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
	"github.com/wowsims/wotlk/sim/core/stats"
)

type GhoulPet struct {
	core.Pet

	dkOwner *Deathknight

	GhoulFrenzyAura *core.Aura
	Claw            *core.Spell

	uptimePercent float64

	// AggressorAI: the army only swings
	isArmy bool
	// Raise Dead's guardian runs CombatAI, which casts Claw on a timer instead of when it can
	nextClawAt time.Duration
}

// creatureCritChance is Unit::GetUnitCriticalChance for anything but a player: a flat 5%, nothing
// from agility.
const creatureCritChance = 5 * core.CritRatingPerCritChance

// risenGhoulStun is Risen Ghoul Self Stun (47466), which InitStatsForLevel puts on every risen ghoul:
// it stands there for the rising animation before it attacks.
const risenGhoulStun = 4500 * time.Millisecond

const ghoulPetAutocastEnergy = 75

func (dk *Deathknight) NewArmyGhoulPet(_ int) *GhoulPet {
	// No pet_levelstats row for 24207, so InitStatsForLevel's fallback stats, and 2 * Str - 20 AP
	// (Guardian::UpdateAttackPowerAndDamage) on top of Army of the Dead Passive's 6.5% of the owner's.
	armyGhoulPetBaseStats := stats.Stats{
		stats.Stamina:     25,
		stats.Agility:     22,
		stats.Strength:    22,
		stats.AttackPower: -20,
		stats.MeleeCrit:   creatureCritChance,
	}

	ghoulPet := &GhoulPet{
		Pet:     core.NewPet("Army of the Dead", &dk.Character, armyGhoulPetBaseStats, dk.armyGhoulStatInheritance(), false, true),
		dkOwner: dk,
		isArmy:  true,
	}
	ghoulPet.HitScaling = core.PetHitScalingMasterSpell06

	ghoulPet.PseudoStats.DamageTakenMultiplier *= 0.1
	ghoulPet.PseudoStats.MeleeHasteRatingPerHastePercent = dk.PseudoStats.MeleeHasteRatingPerHastePercent

	dk.SetupGhoul(ghoulPet)

	ghoulPet.EnableAutoAttacks(ghoulPet, core.AutoAttackOptions{
		// InitStatsForLevel's NPC_ARMY_OF_THE_DEAD case: level -/+ a quarter
		MainHand: core.Weapon{
			BaseDamageMin:     60,
			BaseDamageMax:     100,
			SwingSpeed:        2,
			CritMultiplier:    2,
			AttackPowerPerDPS: core.DefaultAttackPowerPerDPS,
		},
		AutoSwingMelee: true,
	})

	ghoulPet.AddStatDependency(stats.Strength, stats.AttackPower, 2)

	// command doesn't apply to army ghoul
	if dk.RacialTraits == proto.Race_RaceOrc {
		ghoulPet.PseudoStats.DamageDealtMultiplier /= 1.05
	}

	return ghoulPet
}

func (dk *Deathknight) NewGhoulPet(permanent bool) *GhoulPet {
	// pet_levelstats for 26125 at 80, and IsPetGhoul's AP: 589 + Str + Agi
	ghoulPetBaseStats := stats.Stats{
		// Guardian::UpdateMaxHealth: 4665 plus 10 a point of stamina over 361. The universal
		// stamina dependency (10 a point, -180) adds the rest
		stats.Health:      4665 - 10*361 + 180,
		stats.Stamina:     361,
		stats.Agility:     247,
		stats.Strength:    331,
		stats.AttackPower: 589,
		stats.MeleeCrit:   creatureCritChance,
	}

	ghoulPet := &GhoulPet{
		Pet:     core.NewPet("Ghoul", &dk.Character, ghoulPetBaseStats, dk.ghoulStatInheritance(), permanent, !permanent),
		dkOwner: dk,
	}
	// Master of Ghouls summons a real pet; plain Raise Dead only a guardian.
	ghoulPet.SummonedAsPet = permanent
	ghoulPet.HitScaling = core.PetHitScalingMasterSpell06
	ghoulPet.RisenGhoul = true

	// NightOfTheDead
	ghoulPet.PseudoStats.DamageTakenMultiplier *= 1.0 - float64(dk.Talents.NightOfTheDead)*0.45
	ghoulPet.PseudoStats.MeleeHasteRatingPerHastePercent = dk.PseudoStats.MeleeHasteRatingPerHastePercent

	dk.SetupGhoul(ghoulPet)

	// The pet's weapon is pet_levelstats' 0-0. The guardian never gets one set, so it keeps what
	// Creature::SelectLevel rolled at the template's level 1: 0.1321 and half again.
	weapon := core.Weapon{
		SwingSpeed:        2,
		CritMultiplier:    2,
		AttackPowerPerDPS: core.DefaultAttackPowerPerDPS,
	}
	if !permanent {
		weapon.BaseDamageMin = 0.1321
		weapon.BaseDamageMax = 0.1321 * 1.5
	}
	ghoulPet.EnableAutoAttacks(ghoulPet, core.AutoAttackOptions{
		MainHand:       weapon,
		AutoSwingMelee: true,
	})

	ghoulPet.AddStatDependency(stats.Strength, stats.AttackPower, 1)
	ghoulPet.AddStatDependency(stats.Agility, stats.AttackPower, 1)

	if permanent {
		core.ApplyPetConsumeEffects(&ghoulPet.Character, dk.Consumes)
	}

	return ghoulPet
}

func (dk *Deathknight) SetupGhoul(ghoulPet *GhoulPet) {
	// 51996 (Death Knight Pet Scaling 02): immune to direct haste and slows, Bloodlust included: its
	// own melee swing speed tracks the owner's instead. The permanent ghoul carries it too: it learns
	// 51996 as a Ghoul family passive (skill line 782), which is why Guardian::InitStatsForLevel
	// only adds it when !IsPet().
	ghoulPet.HasteCarrier = true
	ghoulPet.OwnerHasteSource = func() float64 { return ghoulPet.dkOwner.SwingSpeed() }

	ghoulPet.Unit.EnableFocusBar(2, func(sim *core.Simulation) {
		if ghoulPet.GCD.IsReady(sim) {
			ghoulPet.OnGCDReady(sim)
		}
	})

	dk.AddPet(ghoulPet)
}

func (ghoulPet *GhoulPet) GetPet() *core.Pet {
	return &ghoulPet.Pet
}

func (ghoulPet *GhoulPet) Initialize() {
	ghoulPet.Claw = ghoulPet.registerClaw()
}

func (ghoulPet *GhoulPet) Reset(_ *core.Simulation) {
	ghoulPet.nextClawAt = 0
	if !ghoulPet.IsGuardian() {
		ghoulPet.uptimePercent = min(1, max(0, ghoulPet.dkOwner.Inputs.PetUptime))
	} else {
		ghoulPet.uptimePercent = 1.0
	}
}

func (ghoulPet *GhoulPet) ExecuteCustomRotation(sim *core.Simulation) {
	if ghoulPet.isArmy {
		return
	}

	if ghoulPet.uptimePercent < 1.0 { // Apply uptime for permanent pet ghoul
		if sim.GetRemainingDurationPercent() < 1.0-ghoulPet.uptimePercent { // once fight is % completed, disable pet.
			ghoulPet.Pet.Disable(sim)
			return
		}
	}

	if ghoulPet.IsGuardian() && sim.CurrentTime < ghoulPet.nextClawAt {
		ghoulPet.WaitUntil(sim, ghoulPet.nextClawAt)
		return
	}

	if ghoulPet.CurrentFocus() < ghoulPet.Claw.DefaultCast.Cost {
		return
	}
	// PetAI::UpdateAI: a ghoul pet only autocasts from 75 energy
	if !ghoulPet.IsGuardian() && ghoulPet.CurrentFocus() < ghoulPetAutocastEnergy {
		return
	}

	ghoulPet.Claw.Cast(sim, ghoulPet.CurrentTarget)
	if ghoulPet.IsGuardian() {
		ghoulPet.nextClawAt = ghoulPet.clawTimer(sim)
	}
}

// clawTimer is CombatAI's schedule for a spell with no cooldown of its own: AI_DEFAULT_COOLDOWN plus
// rand() % that, run off the AI's event map, so it fires on a map update.
func (ghoulPet *GhoulPet) clawTimer(sim *core.Simulation) time.Duration {
	const aiDefaultCooldown = 5000
	delay := aiDefaultCooldown + int(sim.RandomFloat("Ghoul Claw Timer")*aiDefaultCooldown)
	return sim.NextServerTick(sim.CurrentTime + time.Duration(delay)*time.Millisecond)
}

// rise holds a freshly raised ghoul for Risen Ghoul Self Stun. npc_pet_dk_ghoul then attacks, and its
// CombatAI schedules the first Claw from there.
func (ghoulPet *GhoulPet) rise(sim *core.Simulation) {
	engageAt := sim.NextServerTick(sim.CurrentTime + risenGhoulStun)
	ghoulPet.AutoAttacks.CancelAutoSwing(sim)
	ghoulPet.nextClawAt = core.NeverExpires

	sim.AddPendingAction(&core.PendingAction{
		NextActionAt: engageAt,
		OnAction: func(sim *core.Simulation) {
			if !ghoulPet.IsEnabled() {
				return
			}
			ghoulPet.AutoAttacks.EnableAutoSwing(sim)
			ghoulPet.nextClawAt = ghoulPet.clawTimer(sim)
			ghoulPet.WaitUntil(sim, ghoulPet.nextClawAt)
		},
	})
}

// dkPetHaste is spell_dk_pet_scaling's CalculateHasteAmount: the owner's melee attack speed as whole
// percent, worked in float32, and nothing from a slowed owner.
func dkPetHaste(ownerSwingSpeed float64) float64 {
	modSpeed := min(float32(1/ownerSwingSpeed), 1)
	return 1 + float64(int32((1/modSpeed-1)*100))/100
}

// ghoulStatInheritance is spell_dk_pet_scaling's CalculateStatAmount: 70% of the owner's strength and
// 30% of his stamina, Ravenous Dead adding 20% a rank to each and Glyph of the Ghoul a flat 40, all
// whole percent.
func (dk *Deathknight) ghoulStatInheritance() core.PetStatInheritance {
	glyphPct := int64(0)
	if dk.HasMajorGlyph(proto.DeathknightMajorGlyph_GlyphOfTheGhoul) {
		glyphPct = 40
	}
	ravenousDead := int64(dk.Talents.RavenousDead)
	strengthPct := 70 + 70*20*ravenousDead/100 + glyphPct
	staminaPct := 30 + 30*20*ravenousDead/100 + glyphPct

	if !dk.Talents.MasterOfGhouls {
		// The guardian's amounts are worked out once, at the summon, off whole stats. The pet's move
		// with every change of the owner's instead, so they stay continuous.
		return func(ownerStats stats.Stats) stats.Stats {
			return stats.Stats{
				stats.Stamina:  calculatePct(ownerStats[stats.Stamina], staminaPct),
				stats.Strength: calculatePct(ownerStats[stats.Strength], strengthPct),
			}
		}
	}

	return func(ownerStats stats.Stats) stats.Stats {
		// The owner's melee haste rating isn't inherited here: it reaches the ghoul through the 51996
		// carrier's 2 s resnapshot instead (SetupGhoul's OwnerHasteSource), same as the guardian.
		return stats.Stats{
			stats.Stamina:  ownerStats[stats.Stamina] * float64(staminaPct) / 100,
			stats.Strength: ownerStats[stats.Strength] * float64(strengthPct) / 100,
		}
	}
}

// calculatePct is CalculatePct on an int32 stat and a whole percent.
func calculatePct(stat float64, pct int64) float64 {
	return float64(int64(max(0, stat)) * pct / 100)
}

func (dk *Deathknight) armyGhoulStatInheritance() core.PetStatInheritance {
	return func(ownerStats stats.Stats) stats.Stats {
		return stats.Stats{
			stats.Stamina: ownerStats[stats.Stamina] * 0.2,
			// CalculatePct(int32, 6.5f)
			stats.AttackPower: math.Trunc(math.Trunc(max(0, ownerStats[stats.AttackPower])) * 6.5 / 100),
		}
	}
}

func (ghoulPet *GhoulPet) registerClaw() *core.Spell {
	return ghoulPet.RegisterSpell(core.SpellConfig{
		ActionID:    core.ActionID{SpellID: 47468},
		SpellSchool: core.SpellSchoolPhysical,
		ProcMask:    core.ProcMaskMeleeMHSpecial,
		Flags:       core.SpellFlagMeleeMetrics | core.SpellFlagIncludeTargetBonusDamage,

		FocusCost: core.FocusCostOptions{
			Cost:   40,
			Refund: 0.8,
		},

		Cast: core.CastConfig{
			DefaultCast: core.Cast{
				GCD: time.Second,
			},
			IgnoreHaste: true,
		},

		DamageMultiplier: 1.5,
		CritMultiplier:   2,
		ThreatMultiplier: 1,

		ApplyEffects: func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
			baseDamage := 0 +
				spell.Unit.MHWeaponDamage(sim, spell.MeleeAttackPower()) +
				spell.BonusWeaponDamage()

			result := spell.CalcAndDealDamage(sim, target, baseDamage, spell.OutcomeMeleeSpecialHitAndCrit)
			if !result.Landed() {
				spell.IssueRefund(sim)
			}
		},
	})
}
