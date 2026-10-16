package core

import (
	"fmt"
	"time"

	"github.com/wowsims/wotlk/sim/core/proto"
	"github.com/wowsims/wotlk/sim/core/stats"
)

type OnSwapItem func(*Simulation)

const offset = proto.ItemSlot_ItemSlotMainHand

type ItemSwap struct {
	character       *Character
	onSwapCallbacks []OnSwapItem

	mhCritMultiplier     float64
	ohCritMultiplier     float64
	rangedCritMultiplier float64

	// Which slots to actually swap.
	slots []proto.ItemSlot

	// Holds items that are currently not equipped
	unEquippedItems [3]Item
	swapped         bool

	// set by Unit.reset, which already put back the stats the main set was built with
	unitReset bool
}

/*
TODO All the extra parameters here and the code in multiple places for handling the Weapon struct is really messy,

	we'll need to figure out something cleaner as this will be quite error-prone
*/
func (character *Character) enableItemSwap(itemSwap *proto.ItemSwap, mhCritMultiplier float64, ohCritMultiplier float64, rangedCritMultiplier float64) {
	var slots []proto.ItemSlot
	hasMhSwap := itemSwap.MhItem != nil && itemSwap.MhItem.Id != 0
	hasOhSwap := itemSwap.OhItem != nil && itemSwap.OhItem.Id != 0
	hasRangedSwap := itemSwap.RangedItem != nil && itemSwap.RangedItem.Id != 0

	mainItems := [3]Item{
		character.Equipment[proto.ItemSlot_ItemSlotMainHand],
		character.Equipment[proto.ItemSlot_ItemSlotOffHand],
		character.Equipment[proto.ItemSlot_ItemSlotRanged],
	}
	swapItems := [3]Item{
		toItem(itemSwap.MhItem, &character.Server().Reforging),
		toItem(itemSwap.OhItem, &character.Server().Reforging),
		toItem(itemSwap.RangedItem, &character.Server().Reforging),
	}

	// Handle MH and OH together, because present MH + empty OH --> swap MH and unequip OH
	// note: a slot with the same item id never swaps, even if enchant, gems or reforge differ
	if hasMhSwap || hasOhSwap {
		if swapItems[0].ID != mainItems[0].ID {
			slots = append(slots, proto.ItemSlot_ItemSlotMainHand)
		}
		if swapItems[1].ID != mainItems[1].ID {
			slots = append(slots, proto.ItemSlot_ItemSlotOffHand)
		}
	}
	if hasRangedSwap {
		if swapItems[2].ID != mainItems[2].ID {
			slots = append(slots, proto.ItemSlot_ItemSlotRanged)
		}
	}

	if len(slots) == 0 {
		return
	}

	character.ItemSwap = ItemSwap{
		mhCritMultiplier:     mhCritMultiplier,
		ohCritMultiplier:     ohCritMultiplier,
		rangedCritMultiplier: rangedCritMultiplier,
		slots:                slots,
		unEquippedItems:      swapItems,
		swapped:              false,
	}
}

func (swap *ItemSwap) initialize(character *Character) {
	swap.character = character
	if !swap.IsEnabled() {
		return
	}
	character.RegisterResetEffect(func(*Simulation) {
		swap.unitReset = true
	})
	seen := map[int32]bool{}
	for _, item := range append(weaponSlotItems(&character.Equipment), swap.unEquippedItems[:]...) {
		id := item.Enchant.EffectID
		if setup, ok := enchantSwapEffects[id]; ok && !seen[id] {
			seen[id] = true
			swap.followStaticEnchant(id, setup(character))
		}
	}
	swap.followWeaponSpecialization()
}

// followWeaponSpecialization moves a racial weapon expertise with the swap: Player::UpdateExpertise counts the
// race's MOD_EXPERTISE aura on a hand only while that hand's weapon fits it.
func (swap *ItemSwap) followWeaponSpecialization() {
	character := swap.character
	spec, ok := RacialWeaponSpecialization(character.RacialTraits)
	if !ok {
		return
	}
	type covered struct {
		mask   ProcMask
		onStat bool
	}
	coveredNow := func() covered {
		mask, onStat := spec.hands(character.MainHand(), character.OffHand())
		return covered{mask, onStat}
	}
	onSpells := func(c covered, sign float64) {
		if c.onStat {
			return
		}
		for _, spell := range character.Spellbook {
			if spell.ProcMask.Matches(c.mask) {
				spell.BonusExpertiseRating += sign * spec.Expertise
			}
		}
	}
	onStat := func(c covered) float64 {
		return TernaryFloat64(c.onStat, spec.Expertise, 0)
	}

	built := coveredNow()
	current := built
	character.RegisterResetEffect(func(*Simulation) {
		// Unit.reset puts the built stats back, but not a spell's expertise
		if current != built {
			onSpells(current, -1)
			onSpells(built, 1)
			current = built
		}
	})
	character.RegisterOnItemSwap(func(sim *Simulation) {
		now := coveredNow()
		if now == current {
			return
		}
		onSpells(current, -1)
		onSpells(now, 1)
		if change := onStat(now) - onStat(current); change != 0 {
			character.AddStatDynamic(sim, stats.Expertise, change)
		}
		current = now
	})
}

// EnchantSwapEffect changes a static enchant effect mid-fight: sign 1 puts it on once more, -1 takes one off.
type EnchantSwapEffect func(sim *Simulation, sign float64)

var enchantSwapEffects = map[int32]func(*Character) EnchantSwapEffect{}

// NewEnchantSwapEffect makes a weapon enchant's static effect follow an item swap, as Player::SwapItem takes the
// leaving weapon's enchants off and puts the new one's on (_ApplyItemMods). The enchant's NewEnchantEffect still
// applies it at the build, once for each worn weapon holding it; setup runs at the build too, for a character
// whose swap weapons hold it.
func NewEnchantSwapEffect(id int32, setup func(*Character) EnchantSwapEffect) {
	enchantSwapEffects[id] = setup
}

func (swap *ItemSwap) followStaticEnchant(id int32, effect EnchantSwapEffect) {
	character := swap.character
	built := wornEnchants(&character.Equipment, id)
	worn := built
	character.RegisterResetEffect(func(*Simulation) {
		worn = built
	})
	character.RegisterOnItemSwap(func(sim *Simulation) {
		for now := wornEnchants(&character.Equipment, id); worn != now; {
			if worn < now {
				worn++
				effect(sim, 1)
			} else {
				worn--
				effect(sim, -1)
			}
		}
	})
}

func weaponSlotItems(equipment *Equipment) []Item {
	return []Item{
		equipment[proto.ItemSlot_ItemSlotMainHand],
		equipment[proto.ItemSlot_ItemSlotOffHand],
		equipment[proto.ItemSlot_ItemSlotRanged],
	}
}

func wornEnchants(equipment *Equipment, id int32) int {
	n := 0
	for _, item := range weaponSlotItems(equipment) {
		if item.Enchant.EffectID == id {
			n++
		}
	}
	return n
}

// EnchantSwapRangedHit is NewEnchantSwapEffect's setup for an AddBonusRangedHitRating.
func EnchantSwapRangedHit(rating float64) func(*Character) EnchantSwapEffect {
	return func(character *Character) EnchantSwapEffect {
		return func(_ *Simulation, sign float64) {
			for _, spell := range character.Spellbook {
				if spell.ProcMask.Matches(ProcMaskRanged) {
					spell.BonusHitRating += sign * rating
				}
			}
		}
	}
}

// EnchantSwapRangedCrit is NewEnchantSwapEffect's setup for an AddBonusRangedCritRating.
func EnchantSwapRangedCrit(rating float64) func(*Character) EnchantSwapEffect {
	return func(character *Character) EnchantSwapEffect {
		return func(_ *Simulation, sign float64) {
			for _, spell := range character.Spellbook {
				if spell.ProcMask.Matches(ProcMaskRanged) {
					spell.BonusCritRating += sign * rating
				}
			}
		}
	}
}

// NewEnchantSwapMultiplyStat is an EnchantSwapEffect for a static MultiplyStat, on up to two weapons.
func (character *Character) NewEnchantSwapMultiplyStat(stat stats.Stat, amount float64) EnchantSwapEffect {
	var up, down [2]*stats.StatDependency
	for i := range up {
		up[i] = character.NewDynamicMultiplyStat(stat, amount)
		down[i] = character.NewDynamicMultiplyStat(stat, 1/amount)
	}
	// how many more of the enchant are worn than the build counted
	level := 0
	character.RegisterResetEffect(func(*Simulation) {
		level = 0
	})
	return func(sim *Simulation, sign float64) {
		switch {
		case sign > 0 && level < 0:
			character.DisableDynamicStatDep(sim, down[-level-1])
		case sign > 0:
			character.EnableDynamicStatDep(sim, up[level])
		case level > 0:
			character.DisableDynamicStatDep(sim, up[level-1])
		default:
			character.EnableDynamicStatDep(sim, down[-level])
		}
		level += int(sign)
	}
}

func (character *Character) RegisterOnItemSwap(callback OnSwapItem) {
	if character == nil || !character.ItemSwap.IsEnabled() {
		return
	}

	character.ItemSwap.onSwapCallbacks = append(character.ItemSwap.onSwapCallbacks, callback)
}

// Enchants whose effect goes on and off with the swap, through the two helpers below or their own
// RegisterOnItemSwap (3273, 3370). Any other enchant's effect is static, so the weapon in the bag leaves it
// unapplied, or it would stay on beside the worn one's: a swap moves it through NewEnchantSwapEffect, or the
// weapon damage ones through swapWeapon. Add an enchant here when it registers a swap callback.
var swapFollowingEnchants = map[int32]bool{
	1900: true, 2673: true, 3225: true, 3239: true, 3241: true, 3251: true, 3273: true, 3368: true, 3369: true,
	3370: true, 3748: true, 3784: true, 3789: true, 3790: true,
}

func checkSwapFollowingEnchant(effectID int32) {
	if !swapFollowingEnchants[effectID] {
		panic(fmt.Sprintf("enchant %d follows the item swap: add it to swapFollowingEnchants", effectID))
	}
}

// Helper for handling Effects that use PPMManager to toggle the aura on/off
func (swap *ItemSwap) RegisterOnSwapItemForEffectWithPPMManager(effectID int32, ppm float64, ppmm *PPMManager, aura *Aura) {
	checkSwapFollowingEnchant(effectID)
	character := swap.character
	character.RegisterOnItemSwap(func(sim *Simulation) {
		procMask := character.GetProcMaskForEnchant(effectID)
		*ppmm = character.AutoAttacks.NewPPMManager(ppm, procMask)

		if ppmm.Chance(procMask) == 0 {
			aura.Deactivate(sim)
		} else {
			aura.Activate(sim)
		}
	})

}

// Helper for handling Effects that use the itemID to toggle the aura on and off
func (swap *ItemSwap) RegisterOnSwapItemForItemEffect(itemID int32, aura *Aura) {
	character := swap.character
	character.RegisterOnItemSwap(func(sim *Simulation) {
		procMask := character.GetProcMaskForItem(itemID)

		if procMask == ProcMaskUnknown {
			aura.Deactivate(sim)
		} else {
			aura.Activate(sim)
		}
	})
}

// Helper for handling Effects that use the effectID to toggle the aura on and off
func (swap *ItemSwap) RegisterOnSwapItemForEnchantEffect(effectID int32, aura *Aura) {
	checkSwapFollowingEnchant(effectID)
	character := swap.character
	character.RegisterOnItemSwap(func(sim *Simulation) {
		procMask := character.GetProcMaskForEnchant(effectID)

		if procMask == ProcMaskUnknown {
			aura.Deactivate(sim)
		} else {
			aura.Activate(sim)
		}
	})
}

func (swap *ItemSwap) IsEnabled() bool {
	return swap.character != nil && len(swap.slots) > 0
}

func (swap *ItemSwap) IsSwapped() bool {
	return swap.swapped
}

func (swap *ItemSwap) GetItem(slot proto.ItemSlot) *Item {
	if slot-offset < 0 {
		panic("Not able to swap Item " + slot.String() + " not supported")
	}
	return &swap.unEquippedItems[slot-offset]
}

func (swap *ItemSwap) CalcStatChanges(slots []proto.ItemSlot) stats.Stats {
	newStats := stats.Stats{}
	after := swap.character.Equipment
	for _, slot := range slots {
		oldItemStats := swap.character.Equipment[slot].TotalStats()
		newItemStats := swap.GetItem(slot).TotalStats()
		newStats = newStats.Add(newItemStats.Subtract(oldItemStats))
		after[slot] = *swap.GetItem(slot)
	}

	return newStats.Add(metaGemStatChange(&swap.character.Equipment, &after))
}

// metaGemStatChange is the head's meta gem going on or off between two sets of gear: the server checks its
// condition again on every equip (Player::CorrectMetaGemEnchants). These are the gem's stats. Its effect
// and its crit damage follow through metaGemSwap.
func metaGemStatChange(before, after *Equipment) stats.Stats {
	wasOff, isOff := before.inactiveMetaGem(), after.inactiveMetaGem()
	switch {
	case wasOff == nil && isOff != nil:
		return isOff.Stats.Invert()
	case wasOff != nil && isOff == nil:
		return wasOff.Stats
	}
	return stats.Stats{}
}

func (swap *ItemSwap) SwapItems(sim *Simulation, slots []proto.ItemSlot) {
	if !swap.IsEnabled() {
		return
	}

	character := swap.character

	newStats := stats.Stats{}
	before := character.Equipment
	has2H := swap.GetItem(proto.ItemSlot_ItemSlotMainHand).HandType == proto.HandType_HandTypeTwoHand
	for _, slot := range slots {

		//will swap both on the MainHand Slot for 2H.
		if slot == proto.ItemSlot_ItemSlotOffHand && has2H {
			continue
		}

		if ok, swapStats := swap.swapItem(slot, has2H); ok {
			newStats = newStats.Add(swapStats)
		}
	}
	newStats = newStats.Add(metaGemStatChange(&before, &character.Equipment))

	character.AddStatsDynamic(sim, newStats)
	character.updateMetaGemSwap(sim)

	if sim.Log != nil {
		sim.Log("Item Swap Stats: %v", newStats)
	}

	for _, onSwap := range swap.onSwapCallbacks {
		onSwap(sim)
	}

	// No swing reset: equipping a weapon only changes its attack time (Player::_ApplyWeaponDamage),
	// and the running timer keeps what it had left.

	// If GCD is ready then use the GCD, otherwise we assume it's being used along side a spell.
	if character.GCD.IsReady(sim) {
		newGCD := sim.CurrentTime + 1500*time.Millisecond
		character.SetGCDTimer(sim, newGCD)
	}

	swap.swapped = !swap.swapped
}

// otherSet is the gear with every swap slot holding its other item, as SwapItems leaves it.
func (swap *ItemSwap) otherSet(worn *Equipment) Equipment {
	other := *worn
	has2H := swap.GetItem(proto.ItemSlot_ItemSlotMainHand).HandType == proto.HandType_HandTypeTwoHand
	for _, slot := range swap.slots {
		switch item := swap.GetItem(slot); {
		case slot == proto.ItemSlot_ItemSlotOffHand && has2H:
		case item.ID != 0:
			other[slot] = *item
			if has2H && slot == proto.ItemSlot_ItemSlotMainHand {
				other[proto.ItemSlot_ItemSlotOffHand] = *swap.GetItem(proto.ItemSlot_ItemSlotOffHand)
			}
		}
	}
	return other
}

func (swap *ItemSwap) swapItem(slot proto.ItemSlot, has2H bool) (bool, stats.Stats) {
	oldItem := swap.character.Equipment[slot]
	newItem := swap.GetItem(slot)

	if newItem.ID == 0 && !(has2H && slot == proto.ItemSlot_ItemSlotOffHand) {
		return false, stats.Stats{}
	}

	swap.character.Equipment[slot] = *newItem
	oldItemStats := oldItem.TotalStats()
	newItemStats := newItem.TotalStats()
	newStats := newItemStats.Subtract(oldItemStats)

	//2H will swap out the offhand also.
	if has2H && slot == proto.ItemSlot_ItemSlotMainHand {
		_, ohStats := swap.swapItem(proto.ItemSlot_ItemSlotOffHand, has2H)
		newStats = newStats.Add(ohStats)
	}

	swap.unEquippedItems[slot-offset] = oldItem
	swap.swapWeapon(slot)

	return true, newStats
}

func (swap *ItemSwap) swapWeapon(slot proto.ItemSlot) {
	character := swap.character

	switch slot {
	case proto.ItemSlot_ItemSlotMainHand:
		if character.AutoAttacks.AutoSwingMelee {
			character.AutoAttacks.SetMH(character.WeaponFromMainHand(swap.mhCritMultiplier))
			swap.applyWeaponEnchant(slot)
		}
	case proto.ItemSlot_ItemSlotOffHand:
		if character.AutoAttacks.AutoSwingMelee {
			weapon := character.WeaponFromOffHand(swap.ohCritMultiplier)
			character.AutoAttacks.SetOH(weapon)
			swap.applyWeaponEnchant(slot)

			character.AutoAttacks.IsDualWielding = weapon.SwingSpeed != 0
			character.PseudoStats.CanBlock = character.OffHand().WeaponType == proto.WeaponType_WeaponTypeShield
		}
	case proto.ItemSlot_ItemSlotRanged:
		if character.AutoAttacks.AutoSwingRanged {
			character.AutoAttacks.SetRanged(character.WeaponFromRanged(swap.rangedCritMultiplier))
			swap.applyWeaponEnchant(slot)
		}
	}
}

// applyWeaponEnchant puts the new weapon's damage enchant (AddWeaponEffect) on it: the build added it to the
// weapon it was worn on, which a swap replaces.
func (swap *ItemSwap) applyWeaponEnchant(slot proto.ItemSlot) {
	character := swap.character
	if effect, ok := weaponEffects[character.Equipment[slot].Enchant.EffectID]; ok {
		effect(character.Env.Raid.GetPlayerFromUnit(&character.Unit), slot)
	}
}

// reset puts the main set back. Mid-fight (the APL's swap to the main set) that's a swap like any other; right
// after Unit.reset it skips SwapItems' stat change and GCD, since the stats are already the main set's.
func (swap *ItemSwap) reset(sim *Simulation) {
	if !swap.IsEnabled() {
		return
	}
	unitReset := swap.unitReset
	swap.unitReset = false
	if !swap.IsSwapped() {
		return
	}
	if !unitReset {
		swap.SwapItems(sim, swap.slots)
		return
	}

	has2H := swap.GetItem(proto.ItemSlot_ItemSlotMainHand).HandType == proto.HandType_HandTypeTwoHand
	for _, slot := range swap.slots {
		if slot != proto.ItemSlot_ItemSlotOffHand || !has2H {
			swap.swapItem(slot, has2H)
		}
	}
	swap.swapped = false
	for _, onSwap := range swap.onSwapCallbacks {
		onSwap(sim)
	}
}

func toItem(itemSpec *proto.ItemSpec, reforging *Reforging) Item {
	if itemSpec == nil {
		return Item{}
	}

	return NewItem(ItemSpec{
		ID:      itemSpec.Id,
		Gems:    itemSpec.Gems,
		Enchant: itemSpec.Enchant,
		Reforge: itemSpec.Reforge,
	}, reforging)
}
