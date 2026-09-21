package hunter

import (
	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
	"github.com/wowsims/wotlk/sim/core/stats"
)

type HunterPet struct {
	core.Pet

	config PetConfig

	hunterOwner *Hunter

	CobraStrikesAura *core.Aura
	KillCommandAura  *core.Aura

	specialAbility *core.Spell
	focusDump      *core.Spell

	uptimePercent    float64
	hasOwnerCooldown bool
}

func (hunter *Hunter) NewHunterPet() *HunterPet {
	if hunter.Options.PetType == proto.Hunter_Options_PetNone {
		return nil
	}
	if hunter.Options.PetUptime <= 0 {
		return nil
	}
	petConfig := PetConfigs[hunter.Options.PetType]

	hp := &HunterPet{
		Pet:         core.NewPet(petConfig.Name, &hunter.Character, hunterPetBaseStats, hunter.makeStatInheritance(), true, false),
		config:      petConfig,
		hunterOwner: hunter,

		hasOwnerCooldown: petConfig.SpecialAbility == FuriousHowl || petConfig.SpecialAbility == SavageRend,
	}
	hp.SummonedAsPet = true

	// mod-spell-tweaks' 425790: immune to direct haste and slows, Bloodlust included; its own melee
	// swing speed tracks the owner's ranged speed instead (hunters carry their haste there).
	if hunter.Server().SpellTweaks.HunterPetHaste {
		hp.HasteCarrier = true
		hp.OwnerHasteSource = func() float64 { return hunter.RangedSwingSpeed() }
	}

	// Creature::Regenerate gives a hunter pet 24 focus every 4 s, where core's focus bar has 5 a second
	hp.EnableFocusBar(1.2*(1.0+0.5*float64(hunter.Talents.BestialDiscipline)), func(sim *core.Simulation) {
		if hp.GCD.IsReady(sim) {
			hp.OnGCDReady(sim)
		}
	})

	// Pet::InitStatsForLevel: level -/+ a quarter of it
	hp.EnableAutoAttacks(hp, core.AutoAttackOptions{
		MainHand: core.Weapon{
			BaseDamageMin:  60,
			BaseDamageMax:  100,
			SwingSpeed:     2,
			CritMultiplier: 2,
		},
		AutoSwingMelee: true,
	})
	// Cobra Reflexes is SPELL_AURA_MOD_ATTACKSPEED, plain haste: a hit's weapon damage still reads the
	// 2 s base attack time (Guardian::UpdateDamagePhysical), so hits don't get smaller.
	hp.PseudoStats.MeleeSpeedMultiplier *= 1 + 0.15*float64(hp.Talents().CobraReflexes)

	// a happy pet's +25% only goes on its weapon damage (Guardian::UpdateDamagePhysical), not abilities
	hp.AutoAttacks.MHConfig().DamageMultiplier *= 1.25

	// Pet family bonus is now the same for all pets.
	hp.PseudoStats.SchoolDamageDealtMultiplier[stats.SchoolIndexPhysical] *= 1.05

	hp.AddStatDependency(stats.Strength, stats.AttackPower, 2)
	core.ApplyPetConsumeEffects(&hp.Character, hunter.Consumes)

	hunter.AddPet(hp)

	return hp
}

func (hp *HunterPet) GetPet() *core.Pet {
	return &hp.Pet
}

func (hp *HunterPet) Talents() *proto.HunterPetTalents {
	if talents := hp.hunterOwner.Options.PetTalents; talents != nil {
		return talents
	}
	return &proto.HunterPetTalents{}
}

func (hp *HunterPet) Initialize() {
	hp.specialAbility = hp.NewPetAbility(hp.config.SpecialAbility, true)
	hp.focusDump = hp.NewPetAbility(hp.config.FocusDump, false)

	// mod-spell-tweaks' exotic pet bonus rides Unit::DealDamage, so it scales all the pet deals
	if exoticPetTypes[hp.hunterOwner.Options.PetType] {
		hp.PseudoStats.DamageDealtMultiplier *= hp.Server().SpellTweaks.ExoticPetDamageMultiplier
	}
}

// exoticPetTypes are the families only Beast Mastery tames (CREATURE_TYPE_FLAG_TAMEABLE_EXOTIC on
// every tameable template of theirs).
var exoticPetTypes = map[proto.Hunter_Options_PetType]bool{
	proto.Hunter_Options_Chimaera:    true,
	proto.Hunter_Options_CoreHound:   true,
	proto.Hunter_Options_Devilsaur:   true,
	proto.Hunter_Options_Rhino:       true,
	proto.Hunter_Options_Silithid:    true,
	proto.Hunter_Options_SpiritBeast: true,
	proto.Hunter_Options_Worm:        true,
}

func (hp *HunterPet) Reset(_ *core.Simulation) {
	hp.uptimePercent = min(1, max(0, hp.hunterOwner.Options.PetUptime))
}

func (hp *HunterPet) ExecuteCustomRotation(sim *core.Simulation) {
	percentRemaining := sim.GetRemainingDurationPercent()
	if percentRemaining < 1.0-hp.uptimePercent { // once fight is % completed, disable pet.
		hp.Disable(sim)
		return
	}

	// PetAI::UpdateAI only runs on the pet's own update, so that's when it can cast
	if next := sim.NextServerTick(sim.CurrentTime); next > sim.CurrentTime {
		hp.WaitUntil(sim, next)
		return
	}

	if hp.hasOwnerCooldown && hp.CurrentFocus() < 50 {
		// When a major ability (Furious Howl or Savage Rend) is ready, pool enough
		// energy to use on-demand.
		return
	}

	target := hp.CurrentTarget

	if hp.focusDump == nil {
		hp.specialAbility.Cast(sim, target)
		return
	}
	if hp.specialAbility == nil {
		hp.focusDump.Cast(sim, target)
		return
	}

	// the AI casts one of the autocast spells it could cast right now, picked at random
	special := hp.specialAbility.CanCast(sim, target)
	dump := hp.focusDump.CanCast(sim, target)
	if special && dump {
		if sim.RandomFloat("Hunter Pet Ability") < 0.5 {
			dump = false
		} else {
			special = false
		}
	}
	if special {
		hp.specialAbility.Cast(sim, target)
	} else if dump {
		hp.focusDump.Cast(sim, target)
	}
}

func (hp *HunterPet) killCommandMult() float64 {
	return 1 + 0.2*float64(hp.KillCommandAura.GetStacks())
}

var hunterPetBaseStats = stats.Stats{
	// pet_levelstats, creature_entry 1 at level 80
	stats.Agility:     158,
	stats.Strength:    192,
	stats.AttackPower: -20, // Apparently pets and warriors have a AP penalty.

	// a creature crits 5% plus auras, melee and magic alike, and agility adds nothing
	// (Unit::GetUnitCriticalChance, Unit::SpellDoneCritChance)
	stats.MeleeCrit: 5 * core.CritRatingPerCritChance,
	stats.SpellCrit: 5 * core.CritRatingPerCritChance,
}

// makeStatInheritance is spell_hun_generic_scaling on 34902: 45% of the owner's stamina, 22% of their
// ranged AP as AP and 12.87% as spell damage for the magic schools (126), plus 35% of their armor. Wild
// Hunt's dummy raises each by AddPct, which truncates the int32 stamina and AP percents (54/63, 25/28)
// but not the float spell one. Hunter vs. Wild's share of stamina goes on top of the owner's ranged AP,
// which already counts it once. Hit and expertise come from core's scaling aura, not from here.
func (hunter *Hunter) makeStatInheritance() core.PetStatInheritance {
	hvw := hunter.Talents.HunterVsWild

	petTalents := hunter.Options.PetTalents
	var wildHunt int32
	if petTalents != nil {
		wildHunt = petTalents.WildHunt
	}
	// Wild Hunt (62758, 62762): +20% stamina and +15% AP and spell damage a rank
	staminaPct := float64(45 + 45*20*wildHunt/100)
	apPct := float64(22 + 22*15*wildHunt/100)
	spellPct := 12.87 * (1 + 0.15*float64(wildHunt))

	return func(ownerStats stats.Stats) stats.Stats {
		ownerAP := ownerStats[stats.RangedAttackPower] + ownerStats[stats.Stamina]*0.1*float64(hvw)

		return stats.Stats{
			stats.Stamina:     ownerStats[stats.Stamina] * staminaPct / 100,
			stats.Armor:       ownerStats[stats.Armor] * 0.35,
			stats.AttackPower: ownerAP * apPct / 100,
			stats.SpellPower:  ownerStats[stats.RangedAttackPower] * spellPct / 100,
		}
	}
}

type PetConfig struct {
	Name string

	SpecialAbility PetAbilityType
	FocusDump      PetAbilityType
}

// Abilities reference: https://wotlk.wowhead.com/hunter-pets
// https://wotlk.wowhead.com/guides/hunter-dps-best-pets-taming-loyalty-burning-crusade-classic
var PetConfigs = map[proto.Hunter_Options_PetType]PetConfig{
	proto.Hunter_Options_Bat: {
		Name:           "Bat",
		SpecialAbility: SonicBlast,
		FocusDump:      Claw,
	},
	proto.Hunter_Options_Bear: {
		Name:           "Bear",
		SpecialAbility: Swipe,
		FocusDump:      Claw,
	},
	proto.Hunter_Options_BirdOfPrey: {
		Name:           "Bird of Prey",
		SpecialAbility: Snatch,
		FocusDump:      Claw,
	},
	proto.Hunter_Options_Boar: {
		Name:           "Boar",
		SpecialAbility: Gore,
		FocusDump:      Bite,
	},
	proto.Hunter_Options_CarrionBird: {
		Name:           "Carrion Bird",
		SpecialAbility: DemoralizingScreech,
		FocusDump:      Bite,
	},
	proto.Hunter_Options_Cat: {
		Name:           "Cat",
		SpecialAbility: Rake,
		FocusDump:      Claw,
	},
	proto.Hunter_Options_Chimaera: {
		Name:           "Chimaera",
		SpecialAbility: FroststormBreath,
		FocusDump:      Bite,
	},
	proto.Hunter_Options_CoreHound: {
		Name:           "Core Hound",
		SpecialAbility: LavaBreath,
		FocusDump:      Bite,
	},
	proto.Hunter_Options_Crab: {
		Name:           "Crab",
		SpecialAbility: Pin,
		FocusDump:      Claw,
	},
	proto.Hunter_Options_Crocolisk: {
		Name: "Crocolisk",
		//SpecialAbility: BadAttitude,
		FocusDump: Bite,
	},
	proto.Hunter_Options_Devilsaur: {
		Name:           "Devilsaur",
		SpecialAbility: MonstrousBite,
		FocusDump:      Bite,
	},
	proto.Hunter_Options_Dragonhawk: {
		Name:           "Dragonhawk",
		SpecialAbility: FireBreath,
		FocusDump:      Bite,
	},
	proto.Hunter_Options_Gorilla: {
		Name: "Gorilla",
		//SpecialAbility: Pummel,
		FocusDump: Smack,
	},
	proto.Hunter_Options_Hyena: {
		Name:           "Hyena",
		SpecialAbility: TendonRip,
		FocusDump:      Bite,
	},
	proto.Hunter_Options_Moth: {
		Name: "Moth",
		//SpecialAbility:   SerentiyDust,
		FocusDump: Smack,
	},
	proto.Hunter_Options_NetherRay: {
		Name:           "Nether Ray",
		SpecialAbility: NetherShock,
		FocusDump:      Bite,
	},
	proto.Hunter_Options_Raptor: {
		Name:           "Raptor",
		SpecialAbility: SavageRend,
		FocusDump:      Claw,
	},
	proto.Hunter_Options_Ravager: {
		Name:           "Ravager",
		SpecialAbility: Ravage,
		FocusDump:      Bite,
	},
	proto.Hunter_Options_Rhino: {
		Name:           "Rhino",
		SpecialAbility: Stampede,
		FocusDump:      Bite,
	},
	proto.Hunter_Options_Scorpid: {
		Name:           "Scorpid",
		SpecialAbility: ScorpidPoison,
		FocusDump:      Bite,
	},
	proto.Hunter_Options_Serpent: {
		Name:           "Serpent",
		SpecialAbility: PoisonSpit,
		FocusDump:      Bite,
	},
	proto.Hunter_Options_Silithid: {
		Name:           "Silithid",
		SpecialAbility: VenomWebSpray,
		FocusDump:      Claw,
	},
	proto.Hunter_Options_Spider: {
		Name: "Spider",
		//SpecialAbility:   Web,
		FocusDump: Bite,
	},
	proto.Hunter_Options_SpiritBeast: {
		Name:           "Spirit Beast",
		SpecialAbility: SpiritStrike,
		FocusDump:      Claw,
	},
	proto.Hunter_Options_SporeBat: {
		Name:           "Spore Bat",
		SpecialAbility: SporeCloud,
		FocusDump:      Smack,
	},
	proto.Hunter_Options_Tallstrider: {
		Name: "Tallstrider",
		//SpecialAbility:   DustCloud,
		FocusDump: Claw,
	},
	proto.Hunter_Options_Turtle: {
		Name: "Turtle",
		//SpecialAbility: ShellShield,
		FocusDump: Bite,
	},
	proto.Hunter_Options_WarpStalker: {
		Name: "Warp Stalker",
		//SpecialAbility:   Warp,
		FocusDump: Bite,
	},
	proto.Hunter_Options_Wasp: {
		Name:           "Wasp",
		SpecialAbility: Sting,
		FocusDump:      Smack,
	},
	proto.Hunter_Options_WindSerpent: {
		Name:           "Wind Serpent",
		SpecialAbility: LightningBreath,
		FocusDump:      Bite,
	},
	proto.Hunter_Options_Wolf: {
		Name:           "Wolf",
		SpecialAbility: FuriousHowl,
		FocusDump:      Bite,
	},
	proto.Hunter_Options_Worm: {
		Name:           "Worm",
		SpecialAbility: AcidSpit,
		FocusDump:      Bite,
	},
}
