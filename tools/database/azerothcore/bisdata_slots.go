package azerothcore

import (
	"strings"

	"github.com/wowsims/wotlk/sim/core/proto"
)

// The BisTooltipAC addon's names, which decoded blocks have to use: its class and spec keys
// (Bistooltip_spec_icons), slot_name values and phase keys.

// BisClassNames are the addon's class names for the AzerothCore class ids.
var BisClassNames = map[int32]string{
	1: "Warrior", 2: "Paladin", 3: "Hunter", 4: "Rogue", 5: "Priest",
	6: "Death knight", 7: "Shaman", 8: "Mage", 9: "Warlock", 11: "Druid",
}

// BisSpecNames are each class's specs in the addon.
var BisSpecNames = map[int32][]string{
	1:  {"Arms", "Fury", "Fury-Prot", "Protection"},
	2:  {"Holy", "Protection", "Retribution"},
	3:  {"Beast mastery", "Marksmanship", "Survival"},
	4:  {"Assassination", "Combat", "Subtlety"},
	5:  {"Discipline", "Holy", "Shadow"},
	6:  {"Blood dps", "Blood tank", "Frost", "Unholy"},
	7:  {"Elemental", "Enhancement", "Restoration"},
	8:  {"Arcane", "Fire", "Fire FFB", "Frost"},
	9:  {"Affliction", "Demonology", "Destruction", "Destruction fire"},
	11: {"Balance", "Feral dps", "Feral tank", "Restoration"},
}

// bisTreeSpecs names a raider's spec after their main talent tree, in tab order.
var bisTreeSpecs = map[int32][3]string{
	1:  {"Arms", "Fury", "Protection"},
	2:  {"Holy", "Protection", "Retribution"},
	3:  {"Beast mastery", "Marksmanship", "Survival"},
	4:  {"Assassination", "Combat", "Subtlety"},
	5:  {"Discipline", "Holy", "Shadow"},
	6:  {"Blood dps", "Frost", "Unholy"},
	7:  {"Elemental", "Enhancement", "Restoration"},
	8:  {"Arcane", "Fire", "Frost"},
	9:  {"Affliction", "Demonology", "Destruction"},
	11: {"Balance", "Feral dps", "Restoration"},
}

// bisTankTreeSpecs are trees that tank and DPS share; the raid's main tank flag picks the tank spec.
var bisTankTreeSpecs = map[[2]int32]string{
	{6, 0}:  "Blood tank",
	{11, 1}: "Feral tank",
}

// MainTalentTree is the tree holding the most points in a sim talent string, the first on a tie.
func MainTalentTree(talents string) int {
	best, bestPoints := 0, -1
	for i, tree := range strings.Split(talents, "-") {
		points := 0
		for _, digit := range tree {
			if digit >= '0' && digit <= '9' {
				points += int(digit - '0')
			}
		}
		if points > bestPoints {
			best, bestPoints = i, points
		}
	}
	return best
}

// RaiderSpecName is the addon spec a roster character plays, going by their main talent tree. It
// can't tell builds within a tree apart, e.g. Fury-Prot from Protection.
func RaiderSpecName(character *RosterCharacter) string {
	tree := MainTalentTree(character.Talents)
	if tree > 2 {
		return ""
	}
	if spec, ok := bisTankTreeSpecs[[2]int32{character.ClassID, int32(tree)}]; ok && character.MemberFlags&2 != 0 {
		return spec
	}
	return bisTreeSpecs[character.ClassID][tree]
}

// BisPhaseKeys maps content phases 1 to 5 to the addon's phase keys.
var BisPhaseKeys = [...]string{1: "T7", 2: "T8", 3: "T9", 4: "T10", 5: "RS"}

const BisMaxContentPhase = 5

// bisRelicClasses have a relic in the ranged slot: paladin, death knight, shaman, druid.
var bisRelicClasses = map[int32]bool{2: true, 6: true, 7: true, 11: true}

// BisSlotName is the addon's slot_name for an equipment slot. Both rings are "Finger" and both
// trinkets "Trinket", each with its own ranked list.
func BisSlotName(slot proto.ItemSlot, classID int32) string {
	switch slot {
	case proto.ItemSlot_ItemSlotHead:
		return "Head"
	case proto.ItemSlot_ItemSlotNeck:
		return "Neck"
	case proto.ItemSlot_ItemSlotShoulder:
		return "Shoulder"
	case proto.ItemSlot_ItemSlotBack:
		return "Back"
	case proto.ItemSlot_ItemSlotChest:
		return "Chest"
	case proto.ItemSlot_ItemSlotWrist:
		return "Wrist"
	case proto.ItemSlot_ItemSlotHands:
		return "Hands"
	case proto.ItemSlot_ItemSlotWaist:
		return "Waist"
	case proto.ItemSlot_ItemSlotLegs:
		return "Legs"
	case proto.ItemSlot_ItemSlotFeet:
		return "Feet"
	case proto.ItemSlot_ItemSlotFinger1, proto.ItemSlot_ItemSlotFinger2:
		return "Finger"
	case proto.ItemSlot_ItemSlotTrinket1, proto.ItemSlot_ItemSlotTrinket2:
		return "Trinket"
	case proto.ItemSlot_ItemSlotMainHand:
		return "Weapon"
	case proto.ItemSlot_ItemSlotOffHand:
		return "Off hand"
	case proto.ItemSlot_ItemSlotRanged:
		if bisRelicClasses[classID] {
			return "Relic"
		}
		return "Ranged"
	}
	return ""
}
