package core

import (
	"fmt"
	"math"
	"slices"
	"time"

	"github.com/wowsims/wotlk/sim/core/proto"
	"github.com/wowsims/wotlk/sim/core/stats"
)

// Extension of Agent interface, for Pets.
type PetAgent interface {
	Agent

	// The Pet controlled by this PetAgent.
	GetPet() *Pet
}

type OnPetEnable func(sim *Simulation)
type OnPetDisable func(sim *Simulation)

type PetStatInheritance func(ownerStats stats.Stats) stats.Stats
type PetMeleeSpeedInheritance func(amount float64)

// Pet is an extension of Character, for any entity created by a player that can
// take actions on its own.
type Pet struct {
	Character

	Owner *Character

	isGuardian     bool
	enabledOnStart bool

	OnPetEnable  OnPetEnable
	OnPetDisable OnPetDisable

	// Calculates inherited stats based on owner stats or stat changes.
	statInheritance        PetStatInheritance
	dynamicStatInheritance PetStatInheritance
	inheritedStats         stats.Stats

	// Hit, spell hit and expertise from the owner's scaling aura, which the class's inheritance
	// doesn't get a say in, plus the action that refreshes them.
	ownerHit        stats.Stats
	ownerHitRefresh *PendingAction

	// Which scaling aura hands this pet its owner's hit. The class sets it before the pet is enabled.
	HitScaling PetHitScaling
	// NPC_RISEN_GHOUL: Unit::CalcArmorReducedDamage hands any risen ghoul its owner's armor pen,
	// the permanent pet and Raise Dead's guardian alike.
	RisenGhoul bool

	// DK pets also inherit their owner's MeleeSpeed. This replace OwnerAttackSpeedChanged.
	dynamicMeleeSpeedInheritance PetMeleeSpeedInheritance

	isReset bool

	// Some pets expire after a certain duration. This is the pending action that disables
	// the pet on expiration.
	timeoutAction *PendingAction
}

func NewPet(name string, owner *Character, baseStats stats.Stats, statInheritance PetStatInheritance, enabledOnStart bool, isGuardian bool) Pet {
	pet := Pet{
		Character: Character{
			Unit: Unit{
				Type:        PetUnit,
				Index:       owner.Party.Raid.getNextPetIndex(),
				Label:       fmt.Sprintf("%s - %s", owner.Label, name),
				Level:       CharacterLevel,
				PseudoStats: newPseudoStats(),
				auraTracker: newAuraTracker(),
				Metrics:     NewUnitMetrics(),

				StatDependencyManager: stats.NewStatDependencyManager(),
			},
			Name:       name,
			Party:      owner.Party,
			PartyIndex: owner.PartyIndex,
			baseStats:  baseStats,
		},
		Owner:           owner,
		statInheritance: statInheritance,
		enabledOnStart:  enabledOnStart,
		isGuardian:      isGuardian,
	}
	pet.GCD = pet.NewTimer()

	pet.AddStats(baseStats)
	pet.addUniversalStatDependencies()
	pet.PseudoStats.InFrontOfTarget = owner.PseudoStats.InFrontOfTarget
	// Pets that get armor penetration get their owner's, which converts it at the owner's rate.
	pet.PseudoStats.ArmorPenRatingPerPercent = owner.PseudoStats.ArmorPenRatingPerPercent
	// A pet avoids like any other creature: flat dodge, nothing from agility or defense.
	pet.PseudoStats.BaseDodge = CreatureDodgeChance

	return pet
}

// Finalize wires up what a pet takes from its owner, now that the owner's spells and talents are
// all in place.
func (pet *Pet) Finalize() {
	if pet.Env.IsFinalized() {
		return
	}
	pet.inheritOwnerAttackSpeed()
	pet.Character.Finalize()
	// After Character.Finalize: that's where the pet's swings get their spells, and the inherited
	// armor pen lands on them.
	pet.inheritOwnerArmorPen()
}

// PetHitScaling is the serverside aura that hands a pet its owner's hit (Guardian::InitStatsForLevel,
// or the pet's AI): the owner's hit chance rescaled to each stat's own cap and truncated to a whole
// point, so a pet needs 8% owner hit for the full 8% hit, 17% spell hit and 26 expertise.
type PetHitScaling uint8

const (
	// spell_pet_hit_expertise_scalling (61013, 61017): hit, spell hit and expertise
	PetHitScalingDefault PetHitScaling = iota
	// Pet Scaling - Master Spell 06 (67561), on the DK's risen ghouls, gargoyle and army: spell hit
	// and expertise off the owner's melee hit, no melee hit at all
	PetHitScalingMasterSpell06
)

const (
	petOwnerHitCap      = 8.0
	petOwnerSpellHitCap = 17.0

	petHitAmount       = 8.0
	petSpellHitAmount  = 17.0
	petExpertiseAmount = 26.0

	// Only a real pet recalculates them, and only this often.
	petOwnerHitRefreshInterval = 3 * time.Second
)

// ownerHitScaling is what the scaling aura is worth right now. Which of the owner's hit chances 61017
// reads depends on the owner: ranged for a hunter, spell for anyone casting off mana, else melee.
// 67561 always reads melee.
func (pet *Pet) ownerHitScaling() stats.Stats {
	owner := pet.Owner
	hitPct, hitCap := owner.stats[stats.MeleeHit]/MeleeHitRatingPerHitChance, petOwnerHitCap
	if pet.HitScaling == PetHitScalingMasterSpell06 {
		return stats.Stats{
			stats.SpellHit:  math.Trunc(hitPct/hitCap*petSpellHitAmount) * SpellHitRatingPerHitChance,
			stats.Expertise: math.Trunc(hitPct/hitCap*petExpertiseAmount) * ExpertisePerQuarterPercentReduction,
		}
	}
	if owner.Class != proto.Class_ClassHunter && owner.GetCurrentPowerBar() == ManaBar {
		hitPct, hitCap = owner.stats[stats.SpellHit]/SpellHitRatingPerHitChance, petOwnerSpellHitCap
	}

	return stats.Stats{
		stats.MeleeHit:  math.Trunc(hitPct/hitCap*petHitAmount) * MeleeHitRatingPerHitChance,
		stats.SpellHit:  math.Trunc(hitPct/hitCap*petSpellHitAmount) * SpellHitRatingPerHitChance,
		stats.Expertise: math.Trunc(hitPct/hitCap*petExpertiseAmount) * ExpertisePerQuarterPercentReduction,
	}
}

// refreshOwnerHitScaling is the aura's 3 s tick: recalculate, and move the pet by the difference.
func (pet *Pet) refreshOwnerHitScaling(sim *Simulation) {
	scaling := pet.ownerHitScaling()
	change := scaling.Subtract(pet.ownerHit)
	if change.Equals(stats.Stats{}) {
		return
	}

	pet.ownerHit = scaling
	pet.inheritedStats.AddInplace(&change)
	pet.AddStatsDynamic(sim, change)
}

// inheritOwnerArmorPen points the pet's attacks at the owner's armor penetration, for the pets
// mod-spell-tweaks does it for (Unit::CalcArmorReducedDamage): hunter pets and risen ghouls.
func (pet *Pet) inheritOwnerArmorPen() {
	tweaks := pet.Owner.Server().SpellTweaks
	switch pet.Owner.Class {
	case proto.Class_ClassHunter:
		if !pet.SummonedAsPet || !tweaks.HunterPetArmorPen {
			return
		}
	case proto.Class_ClassDeathknight:
		if !pet.RisenGhoul || !tweaks.DKGhoulArmorPen {
			return
		}
	default:
		return
	}

	pet.armorPenSource = &pet.Owner.Unit

	if pet.AutoAttacks.AutoSwingMelee && pet.AutoAttacks.MHAuto() == nil {
		panic(pet.Label + ": armor pen inherited before the pet's swings were registered")
	}

	// A percent-ArP aura of the owner's reaches the pet's white swings too: a swing carries no
	// spell, so there is nothing for the aura's class mask to fail against, while a pet ability
	// never matches one. Blood Gorged is the only such aura the sim keeps as bonus rating on the
	// owner's own white hits.
	ownerWhite := ownerWhiteSwingArmorPen(&pet.Owner.AutoAttacks)
	if ownerWhite == 0 {
		return
	}
	for _, swing := range []*Spell{pet.AutoAttacks.MHAuto(), pet.AutoAttacks.OHAuto()} {
		if swing != nil {
			swing.BonusArmorPenRating += ownerWhite
		}
	}
}

// ownerWhiteSwingArmorPen is the bonus armor penetration rating an owner's own white hits carry,
// off the ranged slot for a class that has no melee swings of its own.
func ownerWhiteSwingArmorPen(auto *AutoAttacks) float64 {
	for _, swing := range []*Spell{auto.MHAuto(), auto.RangedAuto()} {
		if swing != nil {
			return swing.BonusArmorPenRating
		}
	}
	return 0
}

// inheritsOwnerAttackSpeed reports whether mod-spell-tweaks gives this pet the carrier aura. It
// also decides which haste buffs the pet has to be kept away from, since the carrier makes it
// immune to them (applyPetBuffEffects, BloodlustAura).
func (pet *Pet) inheritsOwnerAttackSpeed() bool {
	return pet.SummonedAsPet && pet.Owner.Class == proto.Class_ClassHunter && pet.Owner.Server().SpellTweaks.HunterPetHaste
}

// inheritOwnerAttackSpeed hands hunter pets their owner's ranged attack speed, through the carrier
// aura mod-spell-tweaks gives them (SpellTweaks.HunterPetHaste). It stacks with the pet's own melee
// haste, Frenzy included; what it blocks is melee-and-ranged haste like Bloodlust, which the owner's
// speed already carries, and cast speed.
func (pet *Pet) inheritOwnerAttackSpeed() {
	if !pet.inheritsOwnerAttackSpeed() {
		return
	}

	pet.ownerSwingSpeed = func() float64 { return inheritedSwingSpeed(pet.Owner.RangedSwingSpeed()) }
	pet.Owner.hasteInheritingPets = append(pet.Owner.hasteInheritingPets, pet)
}

// inheritedSwingSpeed is the carrier aura's amount, the owner's haste as whole percent. The server
// clamps the owner's attack time modifier at 1, so a slowed owner hands over nothing, and works in
// float32 throughout.
func inheritedSwingSpeed(ownerSwingSpeed float64) float64 {
	modSpeed := float32(1 / ownerSwingSpeed)
	if modSpeed > 1 {
		modSpeed = 1
	}
	return 1 + math.Trunc(float64((1/modSpeed-1)*100))/100
}

// Updates the stats for this pet in response to a stat change on the owner.
// addedStats is the amount of stats added to the owner (will be negative if the
// owner lost stats).
func (pet *Pet) addOwnerStats(sim *Simulation, addedStats stats.Stats) {
	inheritedChange := pet.dynamicStatInheritance(addedStats)
	// The scaling aura owns these, on its own schedule.
	inheritedChange[stats.MeleeHit] = 0
	inheritedChange[stats.SpellHit] = 0
	inheritedChange[stats.Expertise] = 0

	pet.inheritedStats.AddInplace(&inheritedChange)
	pet.AddStatsDynamic(sim, inheritedChange)
}

func (pet *Pet) reset(sim *Simulation, agent PetAgent) {
	if pet.isReset {
		return
	}
	pet.isReset = true

	pet.Character.reset(sim, agent)

	pet.CancelGCDTimer(sim)
	pet.AutoAttacks.CancelAutoSwing(sim)

	pet.enabled = false
	if pet.enabledOnStart {
		pet.Enable(sim, agent)
	}
}
func (pet *Pet) doneIteration(sim *Simulation) {
	pet.Character.doneIteration(sim)
	pet.isReset = false
}

func (pet *Pet) IsGuardian() bool {
	return pet.isGuardian
}

// petAgent should be the PetAgent which embeds this Pet.
func (pet *Pet) Enable(sim *Simulation, petAgent PetAgent) {
	if pet.enabled {
		if sim.Log != nil {
			pet.Log(sim, "Pet already summoned")
		}
		return
	}

	// In case of Pre-pull guardian summoning we need to reset
	// TODO: Check if this has side effects
	if !pet.isReset {
		pet.reset(sim, petAgent)
	}

	pet.inheritedStats = pet.statInheritance(pet.Owner.GetStats())
	pet.inheritedStats[stats.MeleeHit] = 0
	pet.inheritedStats[stats.SpellHit] = 0
	pet.inheritedStats[stats.Expertise] = 0
	pet.ownerHit = pet.ownerHitScaling()
	pet.inheritedStats.AddInplace(&pet.ownerHit)
	pet.AddStatsDynamic(sim, pet.inheritedStats)

	// A guardian's scaling aura never ticks, so what it got at summon is what it keeps.
	if pet.SummonedAsPet {
		pet.ownerHitRefresh = StartPeriodicAction(sim, PeriodicActionOptions{
			Period:   petOwnerHitRefreshInterval,
			OnAction: pet.refreshOwnerHitScaling,
		})
	}

	if !pet.isGuardian {
		pet.Owner.DynamicStatsPets = append(pet.Owner.DynamicStatsPets, pet)
		pet.dynamicStatInheritance = pet.statInheritance
	}

	//reset current mana after applying stats
	pet.manaBar.reset()

	// Call onEnable callbacks before enabling auto swing
	// to not have to reorder PAs multiple times
	pet.enabled = true

	if pet.OnPetEnable != nil {
		pet.OnPetEnable(sim)
	}

	pet.SetGCDTimer(sim, max(0, sim.CurrentTime))
	if sim.CurrentTime >= 0 {
		pet.AutoAttacks.EnableAutoSwing(sim)
	} else {
		sim.AddPendingAction(&PendingAction{
			NextActionAt: 0,
			OnAction:     pet.AutoAttacks.EnableAutoSwing,
		})
	}

	if sim.Log != nil {
		pet.Log(sim, "Pet stats: %s", pet.GetStats().FlatString())
		pet.Log(sim, "Pet inherited stats: %s", pet.ApplyStatDependencies(pet.inheritedStats).FlatString())
		pet.Log(sim, "Pet summoned")
	}

	sim.addTracker(&pet.auraTracker)

	if pet.HasFocusBar() {
		pet.focusBar.enable(sim)
	}
}

// Helper for enabling a pet that will expire after a certain duration.
func (pet *Pet) EnableWithTimeout(sim *Simulation, petAgent PetAgent, petDuration time.Duration) {
	pet.Enable(sim, petAgent)

	pet.timeoutAction = &PendingAction{
		NextActionAt: sim.CurrentTime + petDuration,
		OnAction: func(sim *Simulation) {
			pet.Disable(sim)
		},
	}
	sim.AddPendingAction(pet.timeoutAction)
}

// Enables and possibly updates how the pet inherits its owner's stats. DK use only.
func (pet *Pet) EnableDynamicStats(inheritance PetStatInheritance) {
	if !slices.Contains(pet.Owner.DynamicStatsPets, pet) {
		pet.Owner.DynamicStatsPets = append(pet.Owner.DynamicStatsPets, pet)
	}
	pet.dynamicStatInheritance = inheritance
}

// Enables and possibly updates how the pet inherits its owner's melee speed. DK use only.
func (pet *Pet) EnableDynamicMeleeSpeed(inheritance PetMeleeSpeedInheritance) {
	if !slices.Contains(pet.Owner.DynamicMeleeSpeedPets, pet) {
		pet.Owner.DynamicMeleeSpeedPets = append(pet.Owner.DynamicMeleeSpeedPets, pet)
	}
	pet.dynamicMeleeSpeedInheritance = inheritance
}

func (pet *Pet) Disable(sim *Simulation) {
	if !pet.enabled {
		if sim.Log != nil {
			pet.Log(sim, "No pet summoned")
		}
		return
	}

	if pet.ownerHitRefresh != nil {
		pet.ownerHitRefresh.Cancel(sim)
		pet.ownerHitRefresh = nil
	}

	// Remove inherited stats on dismiss if not permanent
	if pet.isGuardian || pet.timeoutAction != nil {
		pet.AddStatsDynamic(sim, pet.inheritedStats.Invert())
		pet.inheritedStats = stats.Stats{}
		pet.ownerHit = stats.Stats{}
	}

	if pet.dynamicStatInheritance != nil {
		if idx := slices.Index(pet.Owner.DynamicStatsPets, pet); idx != -1 {
			pet.Owner.DynamicStatsPets = removeBySwappingToBack(pet.Owner.DynamicStatsPets, idx)
		}
		pet.dynamicStatInheritance = nil
	}

	if pet.dynamicMeleeSpeedInheritance != nil {
		if idx := slices.Index(pet.Owner.DynamicMeleeSpeedPets, pet); idx != -1 {
			pet.Owner.DynamicMeleeSpeedPets = removeBySwappingToBack(pet.Owner.DynamicMeleeSpeedPets, idx)
		}
		pet.dynamicMeleeSpeedInheritance = nil
	}

	pet.CancelGCDTimer(sim)
	pet.focusBar.disable(sim)
	pet.AutoAttacks.CancelAutoSwing(sim)
	pet.enabled = false

	// If a pet is immediately re-summoned it might try to use GCD, so we need to clear it.
	pet.Hardcast = Hardcast{}

	if pet.timeoutAction != nil {
		pet.timeoutAction.Cancel(sim)
		pet.timeoutAction = nil
	}

	if pet.OnPetDisable != nil {
		pet.OnPetDisable(sim)
	}

	pet.auraTracker.expireAll(sim)

	sim.removeTracker(&pet.auraTracker)

	if sim.Log != nil {
		pet.Log(sim, "Pet dismissed")
		pet.Log(sim, pet.GetStats().FlatString())
	}
}

// Default implementations for some Agent functions which most Pets don't need.
func (pet *Pet) GetCharacter() *Character {
	return &pet.Character
}
func (pet *Pet) AddRaidBuffs(_ *proto.RaidBuffs)   {}
func (pet *Pet) AddPartyBuffs(_ *proto.PartyBuffs) {}
func (pet *Pet) ApplyTalents()                     {}
func (pet *Pet) OnGCDReady(_ *Simulation)          {}
