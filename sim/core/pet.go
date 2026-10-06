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

// PetStatInheritance must read nothing but ownerStats: refreshScaling skips a tick that finds them as the last
// one did.
type PetStatInheritance func(ownerStats stats.Stats) stats.Stats

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

	// Which of the owner's swing speeds a haste-carrier pet (Unit.HasteCarrier) tracks: ranged for a
	// hunter pet, melee for the DK's ghoul family. The class sets it before the pet is enabled; nil for
	// a haste-carrier pet with no melee swings of its own to rescale, like the gargoyle.
	OwnerHasteSource func() float64
	// Whether the carrier blocks MOD_MELEE_HASTE too, which keeps Windfury Totem and Improved Icy
	// Talons off the pet: the owner's own melee haste already carries them. Only the Spirit Wolves'
	// 425792 does that. The hunter pet's 425790 and spell_dk_pet_scaling leave it open, so Frenzy and
	// Ghoul Frenzy stack on top there.
	OwnerHasteCoversMeleeHasteAuras bool
	// The carrier's last snapshot, as a swing-speed multiplier. A real pet retakes it every 2 s
	// (ownerHasteRefresh); a guardian keeps what it got at the summon.
	ownerHasteAmount  float64
	ownerHasteRefresh *PendingAction

	// How often the scaling auras rework what the owner hands over (their HandlePeriodic). 0 for a
	// summon that only gets it at the summon, like most guardians. A non-guardian left at 0 follows
	// every owner stat change instead.
	ScalingPeriod  time.Duration
	scalingRefresh *PendingAction
	scalingTick    petScalingTick

	// The aura types the summon's scaling script makes it immune to, so the owner's buffs of those
	// types never land on it (dropBuffsItCannotTake). Pet food and pet scrolls are stat auras too: a
	// summon immune to stats skips ApplyPetConsumeEffects.
	BuffImmunity PetBuffImmunity
	// A guardian without a pet bar. The party and raid casts skip it (Spell.cpp's TARGET_CHECK_PARTY
	// and TARGET_CHECK_RAID), so it only gets the area auras.
	Uncontrollable bool

	// Mana regen is Creature::Regenerate's (creatureManaTick), set by EnableCreatureManaBar.
	creatureManaRegen   bool
	regensWhileCasting  bool
	creatureManaRefresh *PendingAction

	isReset bool

	// Some pets expire after a certain duration. This is the pending action that disables
	// the pet on expiration.
	timeoutAction *PendingAction
}

func NewPet(name string, owner *Character, baseStats stats.Stats, statInheritance PetStatInheritance, enabledOnStart bool, isGuardian bool) *Pet {
	pet := &Pet{
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
	pet.Character.Finalize()
	// After Character.Finalize: that's where the pet's swings get their spells, and the inherited
	// armor pen lands on them.
	pet.inheritOwnerArmorPen()
}

// PetCreateStats is a summon's five create stats and its base armor (Guardian::InitStatsForLevel):
// its pet_levelstats row, or 22/22/25/28/27 and 50 armor a level without one (GuardianCreateStats).
// Guardian::UpdateArmor only counts the agility over the create value, twice, where the universal
// agility dependency counts all of it, so the create agility comes off the armor here.
func PetCreateStats(strength, agility, stamina, intellect, spirit, armor float64) stats.Stats {
	return stats.Stats{
		stats.Strength:  strength,
		stats.Agility:   agility,
		stats.Stamina:   stamina,
		stats.Intellect: intellect,
		stats.Spirit:    spirit,
		stats.Armor:     armor - 2*agility,
	}
}

// GuardianCreateStats are InitStatsForLevel's create stats for a summon without a pet_levelstats row.
var GuardianCreateStats = PetCreateStats(22, 22, 25, 28, 27, 50*CharacterLevel)

// PetScalingPct is a scaling aura's CalculatePct(std::max<int32>(0, ownerValue), pct): the owner's
// value truncated, the percent worked in float32, the amount truncated again.
func PetScalingPct(ownerValue float64, pct float64) float64 {
	product := float32(float32(math.Trunc(max(0, ownerValue))) * float32(pct))
	return math.Trunc(float64(float32(product / 100)))
}

// PetBuffImmunity is the positive aura types a summon is immune to (Unit::ApplySpellImmune).
type PetBuffImmunity uint8

const (
	// SPELL_AURA_MOD_STAT and SPELL_AURA_MOD_TOTAL_STAT_PERCENTAGE
	PetImmuneStats PetBuffImmunity = 1 << iota
	// SPELL_AURA_MOD_ATTACK_POWER and SPELL_AURA_MOD_ATTACK_POWER_PCT
	PetImmuneAttackPower
	// SPELL_AURA_MOD_DAMAGE_DONE
	PetImmuneSpellPower
	// SPELL_AURA_MOD_RESISTANCE
	PetImmuneArmor
)

// PetImmuneToScaling is a guardian scaling script that makes the summon immune to every type its
// scaling auras carry: the Infernal's, the treants', the Shadowfiend's, the Spirit Wolves', the
// Fire Elemental's, and the mage's for anything but a pet.
const PetImmuneToScaling = PetImmuneStats | PetImmuneAttackPower | PetImmuneSpellPower | PetImmuneArmor

// dropBuffsItCannotTake clears the owner's buffs that never land on this pet on the server. A buff
// with an effect of each kind (Gift of the Wild's stats and armor) goes as a whole.
func (pet *Pet) dropBuffsItCannotTake(raid *proto.RaidBuffs, party *proto.PartyBuffs, individual *proto.IndividualBuffs) {
	if pet.Uncontrollable {
		raid.GiftOfTheWild = 0
		raid.DrumsOfTheWild = false
		raid.PowerWordFortitude = 0
		raid.CommandingShout = 0
		raid.HornOfWinter = false
		raid.ArcaneBrilliance = false
		raid.DivineSpirit = false
		raid.BattleShout = 0
		raid.ShadowProtection = false
		raid.Thorns = 0
		raid.DrumsOfForgottenKings = false
		raid.ScrollOfProtection = false
		raid.ScrollOfStamina = false
		raid.ScrollOfStrength = false
		raid.ScrollOfAgility = false
		raid.ScrollOfIntellect = false
		raid.ScrollOfSpirit = false
		individual.BlessingOfKings = false
		individual.BlessingOfWisdom = 0
		individual.BlessingOfMight = 0
		individual.BlessingOfSanctuary = false
		individual.Vigilance = false
		individual.RenewedHope = false
	}
	if pet.BuffImmunity&PetImmuneStats != 0 {
		raid.GiftOfTheWild = 0
		raid.DrumsOfTheWild = false
		raid.PowerWordFortitude = 0
		raid.HornOfWinter = false
		raid.StrengthOfEarthTotem = 0
		raid.ArcaneBrilliance = false
		raid.FelIntelligence = 0
		raid.DivineSpirit = false
		raid.DrumsOfForgottenKings = false
		raid.ScrollOfStamina = false
		raid.ScrollOfStrength = false
		raid.ScrollOfAgility = false
		raid.ScrollOfIntellect = false
		raid.ScrollOfSpirit = false
		individual.BlessingOfKings = false
		individual.BlessingOfSanctuary = false
	}
	if pet.BuffImmunity&PetImmuneAttackPower != 0 {
		raid.BattleShout = 0
		raid.TrueshotAura = false
		raid.UnleashedRage = false
		raid.AbominationsMight = false
		individual.BlessingOfMight = 0
	}
	if pet.BuffImmunity&PetImmuneSpellPower != 0 {
		raid.FlametongueTotem = false
		raid.TotemOfWrath = false
		raid.DemonicPactSp = 0
		party.AtieshWarlock = 0
		party.EyeOfTheNight = false
	}
	if pet.BuffImmunity&PetImmuneArmor != 0 {
		raid.DevotionAura = 0
		raid.StoneskinTotem = 0
		raid.ScrollOfProtection = false
	}
	// Every pet's hit scaling aura makes it immune to the hit auras it carries itself. The DK's 67561
	// has no melee hit, so Heroic Presence's 1% melee hit still lands there, its spell hit never does.
	if party.HeroicPresence {
		party.HeroicPresence = false
		if pet.HitScaling == PetHitScalingMasterSpell06 {
			pet.AddStat(stats.MeleeHit, MeleeHitRatingPerHitChance)
		}
	}
}

// EnableCreatureManaBar is a creature's mana (Guardian::UpdateMaxPower): its create mana plus
// manaPerIntellect a point of intellect over the create value, and spell costs off the create mana.
// It regenerates the creature's way (creatureManaTick), not off spirit like a player.
func (pet *Pet) EnableCreatureManaBar(createMana, createIntellect, manaPerIntellect float64, regensWhileCasting bool) {
	pet.EnableManaBarWithModifier(manaPerIntellect / 15)
	pet.AddStat(stats.Mana, createMana-manaPerIntellect*createIntellect-(20-20*manaPerIntellect))
	pet.BaseMana = createMana
	pet.PseudoStats.SpiritRegenMultiplier = 0
	pet.creatureManaRegen = true
	pet.regensWhileCasting = regensWhileCasting
}

// HasCreatureManaRegen is whether the pet's mana regen is creatureManaTick's.
func (pet *Pet) HasCreatureManaRegen() bool {
	return pet.creatureManaRegen
}

// CreatureManaRegen is Creature::Regenerate's mana for a unit a player owns, a tick every 2 s:
// spirit/5 + 17, truncated. Under the five second rule the Imp and the Water Elemental still get
// spirit/8 + 17 (regensWhileCasting), any other creature nothing. MP5 comes on top, through core's
// regular mana tick.
func (pet *Pet) CreatureManaRegen(underFiveSecondRule bool) float64 {
	spirit := math.Trunc(pet.GetStat(stats.Spirit))
	switch {
	case !underFiveSecondRule:
		return math.Trunc(spirit/5 + 17)
	case pet.regensWhileCasting:
		return math.Trunc(spirit/8 + 17)
	}
	return 0
}

func (pet *Pet) creatureManaTick(sim *Simulation) {
	casting := sim.CurrentTime < pet.PseudoStats.FiveSecondRuleRefreshTime
	amount := pet.CreatureManaRegen(casting)
	if amount <= 0 {
		return
	}
	metrics := pet.manaNotCastingMetrics
	if casting {
		metrics = pet.manaCastingMetrics
	}
	pet.AddMana(sim, amount, metrics)
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

	// The haste carrier's own recalculation cadence (spell_dk_pet_scaling's and mod-spell-tweaks'
	// CalcPeriodic), also real-pet only.
	petOwnerHasteRefreshInterval = 2 * time.Second

	// CREATURE_REGEN_INTERVAL
	creatureRegenInterval = 2 * time.Second
)

// ownerHitScaling is what the scaling aura is worth right now. Which of the owner's hit chances 61017
// reads depends on the owner: ranged for a hunter, spell for anyone casting off mana, else melee.
// 67561 always reads melee.
func (pet *Pet) ownerHitScaling() stats.Stats {
	hit, spellHit, expertise := pet.ownerHitAmounts()
	return stats.Stats{stats.MeleeHit: hit, stats.SpellHit: spellHit, stats.Expertise: expertise}
}

// ownerHitAmounts is ownerHitScaling's three stats, the only ones it sets.
func (pet *Pet) ownerHitAmounts() (hit, spellHit, expertise float64) {
	owner := pet.Owner
	hitPct, hitCap := owner.stats[stats.MeleeHit]/MeleeHitRatingPerHitChance, petOwnerHitCap
	if pet.HitScaling == PetHitScalingMasterSpell06 {
		return 0, math.Trunc(hitPct/hitCap*petSpellHitAmount) * SpellHitRatingPerHitChance,
			math.Trunc(hitPct/hitCap*petExpertiseAmount) * ExpertisePerQuarterPercentReduction
	}
	if owner.Class != proto.Class_ClassHunter && owner.GetCurrentPowerBar() == ManaBar {
		hitPct, hitCap = owner.stats[stats.SpellHit]/SpellHitRatingPerHitChance, petOwnerSpellHitCap
	}

	return math.Trunc(hitPct/hitCap*petHitAmount) * MeleeHitRatingPerHitChance,
		math.Trunc(hitPct/hitCap*petSpellHitAmount) * SpellHitRatingPerHitChance,
		math.Trunc(hitPct/hitCap*petExpertiseAmount) * ExpertisePerQuarterPercentReduction
}

// refreshOwnerHitScaling is the aura's 3 s tick: recalculate, and move the pet by the difference.
func (pet *Pet) refreshOwnerHitScaling(sim *Simulation) {
	hit, spellHit, expertise := pet.ownerHitAmounts()
	// ownerHit holds nothing but these three, so the rest of the difference is always 0
	if hit-pet.ownerHit[stats.MeleeHit] == 0 && spellHit-pet.ownerHit[stats.SpellHit] == 0 &&
		expertise-pet.ownerHit[stats.Expertise] == 0 {
		return
	}

	scaling := stats.Stats{stats.MeleeHit: hit, stats.SpellHit: spellHit, stats.Expertise: expertise}
	change := scaling.Subtract(pet.ownerHit)
	pet.ownerHit = scaling
	pet.inheritedStats.AddInplace(&change)
	pet.scalingTick.fresh = false
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

// inheritsOwnerAttackSpeed reports whether this pet carries a haste-carrier aura. It also decides
// which haste buffs the pet has to be kept away from, since the carrier makes it immune to them
// (applyPetBuffEffects, BloodlustAura, Unit.MultiplyAttackSpeed).
func (pet *Pet) inheritsOwnerAttackSpeed() bool {
	return pet.HasteCarrier
}

// ownerHasteMultiplier is OwnerHasteSource's swing speed, the way CalculateHasteAmount reads it: the
// owner's attack time modifier as a truncated whole percent, worked in float32. A slowed owner hands
// over nothing, since the server clamps the modifier at 1.
func (pet *Pet) ownerHasteMultiplier() float64 {
	return inheritedSwingSpeed(pet.OwnerHasteSource())
}

func inheritedSwingSpeed(ownerSwingSpeed float64) float64 {
	modSpeed := float32(1 / ownerSwingSpeed)
	if modSpeed > 1 {
		modSpeed = 1
	}
	return 1 + math.Trunc(float64((1/modSpeed-1)*100))/100
}

// refreshOwnerHaste is the haste carrier's periodic tick, real pets only (a guardian's carrier never
// ticks, so it keeps what it got at the summon): recalculate the owner's current swing speed and move
// the pet's own melee swing speed by the difference.
func (pet *Pet) refreshOwnerHaste(sim *Simulation) {
	amount := pet.ownerHasteMultiplier()
	if amount == pet.ownerHasteAmount {
		return
	}
	pet.MultiplyMeleeSpeed(sim, amount/pet.ownerHasteAmount)
	pet.ownerHasteAmount = amount
}

// ownerScaling is what the scaling auras hand over right now: the class's share of the owner's
// stats, with the hit scaling aura's hit and expertise in place of whatever the class gives.
func (pet *Pet) ownerScaling() stats.Stats {
	var scaling stats.Stats
	pet.ownerScalingInto(&scaling)
	return scaling
}

func (pet *Pet) ownerScalingInto(scaling *stats.Stats) {
	*scaling = pet.statInheritance(pet.Owner.stats)
	scaling[stats.MeleeHit] = 0
	scaling[stats.SpellHit] = 0
	scaling[stats.Expertise] = 0
	scaling.AddInplace(&pet.ownerHit)
}

// refreshScaling is the scaling auras' periodic tick: rework the amounts off the owner's stats now,
// and move the pet by the difference. Hot enough that it skips out when nothing it reads has moved and
// otherwise works in pet-held scratch, not Stats copies.
func (pet *Pet) refreshScaling(sim *Simulation) {
	tick := &pet.scalingTick
	if tick.fresh && tick.owner == pet.Owner.stats {
		return
	}
	tick.owner, tick.fresh = pet.Owner.stats, true

	pet.ownerScalingInto(&tick.scaling)
	changed := false
	for i := range tick.change {
		tick.change[i] = tick.scaling[i] - pet.inheritedStats[i]
		changed = changed || tick.change[i] != 0
	}
	if changed {
		pet.inheritedStats = tick.scaling
		pet.AddStatsDynamic(sim, tick.change)
	}
}

// petScalingTick is refreshScaling's state between ticks.
type petScalingTick struct {
	// the owner's stats the last tick read, and whether ownerHit and inheritedStats are as it left them:
	// every other write to either clears fresh
	owner stats.Stats
	fresh bool

	scaling, change stats.Stats
}

// ResyncScaling catches a summoned pet up with its owner's stats now, hit included, as the next tick
// of each of its scaling auras would.
func (pet *Pet) ResyncScaling(sim *Simulation) {
	pet.refreshOwnerHitScaling(sim)
	pet.refreshScaling(sim)
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
	pet.scalingTick.fresh = false
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

	pet.ownerHit = pet.ownerHitScaling()
	pet.inheritedStats = pet.ownerScaling()
	pet.scalingTick.fresh = false
	pet.AddStatsDynamic(sim, pet.inheritedStats)
	// summons start at full health, owner's stamina included (Guardian::InitStatsForLevel)
	pet.healthBar.reset(sim)

	// A guardian's scaling aura never ticks, so what it got at summon is what it keeps.
	if pet.SummonedAsPet {
		pet.ownerHitRefresh = StartPeriodicAction(sim, PeriodicActionOptions{
			Period:   petOwnerHitRefreshInterval,
			OnAction: pet.refreshOwnerHitScaling,
		})
	}

	// The haste carrier's own snapshot: a real pet retakes it every 2 s, a guardian only now, at the
	// summon (same IsPet gate as the hit scaling above).
	if pet.OwnerHasteSource != nil {
		pet.ownerHasteAmount = pet.ownerHasteMultiplier()
		pet.MultiplyMeleeSpeed(sim, pet.ownerHasteAmount)

		if pet.SummonedAsPet {
			pet.ownerHasteRefresh = StartPeriodicAction(sim, PeriodicActionOptions{
				Period:   petOwnerHasteRefreshInterval,
				OnAction: pet.refreshOwnerHaste,
			})
		}
	}

	switch {
	case pet.ScalingPeriod > 0:
		pet.scalingRefresh = StartPeriodicAction(sim, PeriodicActionOptions{
			Period:   pet.ScalingPeriod,
			OnAction: pet.refreshScaling,
		})
	case !pet.isGuardian:
		pet.Owner.DynamicStatsPets = append(pet.Owner.DynamicStatsPets, pet)
		pet.dynamicStatInheritance = pet.statInheritance
	}

	//reset current mana after applying stats
	pet.manaBar.reset()
	if pet.creatureManaRegen {
		pet.creatureManaRefresh = StartPeriodicAction(sim, PeriodicActionOptions{
			Period:   creatureRegenInterval,
			OnAction: pet.creatureManaTick,
		})
	}

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

	if pet.scalingRefresh != nil {
		pet.scalingRefresh.Cancel(sim)
		pet.scalingRefresh = nil
	}

	if pet.creatureManaRefresh != nil {
		pet.creatureManaRefresh.Cancel(sim)
		pet.creatureManaRefresh = nil
	}

	if pet.ownerHasteRefresh != nil {
		pet.ownerHasteRefresh.Cancel(sim)
		pet.ownerHasteRefresh = nil
	}
	if pet.ownerHasteAmount != 0 {
		pet.MultiplyMeleeSpeed(sim, 1/pet.ownerHasteAmount)
		pet.ownerHasteAmount = 0
	}

	// Remove inherited stats on dismiss if not permanent
	if pet.isGuardian || pet.timeoutAction != nil {
		pet.AddStatsDynamic(sim, pet.inheritedStats.Invert())
		pet.inheritedStats = stats.Stats{}
		pet.ownerHit = stats.Stats{}
		pet.scalingTick.fresh = false
	}

	if pet.dynamicStatInheritance != nil {
		if idx := slices.Index(pet.Owner.DynamicStatsPets, pet); idx != -1 {
			pet.Owner.DynamicStatsPets = removeBySwappingToBack(pet.Owner.DynamicStatsPets, idx)
		}
		pet.dynamicStatInheritance = nil
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
