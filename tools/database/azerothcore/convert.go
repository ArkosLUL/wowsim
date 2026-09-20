package azerothcore

import (
	"slices"
	"strings"

	"github.com/wowsims/wotlk/sim/core/proto"
	"github.com/wowsims/wotlk/tools/database"
)

// ConvertedItem is an item_template row expressed in the sim's UIItem terms.
type ConvertedItem struct {
	Item *proto.UIItem

	// Spells the item casts or applies that aren't flat stats (procs, on-use, special equip effects).
	EffectSpells []ItemSpell
	// Stat types the sim has no stat for, e.g. hit taken rating.
	UnmappedStatTypes []int32
	// Random enchant and heirloom-scaling items have stats that item_template doesn't hold.
	NotComparable string
}

type ItemSpell struct {
	SpellID          int32
	Trigger          int32
	PPMRate          float64
	Cooldown         int32 // ms, -1 when unset
	CategoryCooldown int32 // ms, -1 when unset
}

// CooldownMs returns the item's cooldown for the spell, or -1 when the item leaves it to the spell
// (Player::AddSpellAndCategoryCooldowns only falls back when both item cooldowns are unset).
func (s ItemSpell) CooldownMs() int32 {
	if s.Cooldown < 0 && s.CategoryCooldown < 0 {
		return -1
	}
	return max(s.Cooldown, s.CategoryCooldown, 0)
}

// ConvertItem reads an item_template row the way the sim's item DB stores it. Equip spells that
// only grant flat stats, in any form, become stats; every other spell is listed in EffectSpells.
func ConvertItem(row *ItemRow, dbc *DBC) *ConvertedItem {
	var stats database.Stats
	converted := &ConvertedItem{}

	// ItemTemplate::ItemStat, compacted the way ObjectMgr::LoadItemTemplates fills it: zero values
	// dropped, so the length is StatsCount. mod-reforging reads these.
	var serverStats []*proto.ItemStat
	for i := 0; i < 10; i++ {
		if row.StatValues[i] == 0 {
			continue
		}
		serverStats = append(serverStats, &proto.ItemStat{StatType: row.StatTypes[i], Value: row.StatValues[i]})
		if !AddItemMod(&stats, row.StatTypes[i], row.StatValues[i]) {
			converted.UnmappedStatTypes = append(converted.UnmappedStatTypes, row.StatTypes[i])
		}
	}

	for i := 0; i < 5; i++ {
		spellID := row.SpellIDs[i]
		if spellID <= 0 {
			continue
		}
		if row.SpellTriggers[i] == ItemSpellTriggerOnEquip {
			// form-only auras stay effects: sim stats apply in every form, so pre-3.0 feral AP ("in Cat,
			// Bear, Dire Bear, and Moonkin forms only") would count as plain AP for every class
			if spell := dbc.Spells[spellID]; spell != nil && spell.Stances == 0 && AddEquipSpellStats(&stats, spell) {
				continue
			}
		}
		converted.EffectSpells = append(converted.EffectSpells, ItemSpell{
			SpellID:          spellID,
			Trigger:          row.SpellTriggers[i],
			PPMRate:          row.SpellPPMRates[i],
			Cooldown:         row.SpellCooldowns[i],
			CategoryCooldown: row.SpellCategoryCooldowns[i],
		})
	}

	armor, bonusArmor := splitArmor(row)
	stats[proto.Stat_StatArmor] += armor
	stats[proto.Stat_StatBonusArmor] += bonusArmor
	stats[proto.Stat_StatFireResistance] += float64(row.FireRes)
	stats[proto.Stat_StatNatureResistance] += float64(row.NatureRes)
	stats[proto.Stat_StatFrostResistance] += float64(row.FrostRes)
	stats[proto.Stat_StatShadowResistance] += float64(row.ShadowRes)
	stats[proto.Stat_StatArcaneResistance] += float64(row.ArcaneRes)
	stats[proto.Stat_StatBlockValue] += float64(row.Block)

	item := &proto.UIItem{
		Id:             row.Entry,
		Name:           row.Name,
		Stats:          stats[:],
		Ilvl:           row.ItemLevel,
		Quality:        proto.ItemQuality(row.Quality),
		Heroic:         row.Flags&ItemFlagHeroicTooltip != 0,
		ClassAllowlist: AllowedClasses(row.AllowableClass),
		ServerStats:    serverStats,
	}
	if class, ok := relicClasses[row.Subclass]; ok && row.InventoryType == inventoryTypeRelic && item.ClassAllowlist == nil {
		item.ClassAllowlist = []proto.Class{class}
	}

	for _, color := range row.SocketColors {
		if color != 0 {
			item.GemSockets = append(item.GemSockets, SocketGemColor(color))
		}
	}
	var socketBonus database.Stats
	if enchant := dbc.Enchantments[row.SocketBonus]; enchant != nil {
		socketBonus, _, _ = EnchantmentStats(enchant, dbc)
	}
	item.SocketBonus = socketBonus[:]

	if row.Delay > 0 && (row.DmgMin > 0 || row.DmgMax > 0) {
		item.WeaponDamageMin = row.DmgMin
		item.WeaponDamageMax = row.DmgMax
		item.WeaponSpeed = float64(row.Delay) / 1000
	}

	if set := dbc.ItemSets[row.ItemSet]; set != nil {
		item.SetName = simSetName(set.Name, row.Name)
	}

	switch {
	case row.RandomProperty != 0 || row.RandomSuffix != 0:
		converted.NotComparable = "random enchant"
	case row.ScalingStatDistribution != 0:
		converted.NotComparable = "scaling stats"
	}

	converted.Item = item
	return converted
}

// ApplyTo overwrites every field of item that ConvertItem fills from the server, everything but Id
// and Name, zero values and empty lists included. Skip items with NotComparable set: item_template
// doesn't hold their stats, so they end up with no ServerStats and the sim won't reforge them,
// which is stricter than the server but keeps a reforge off stats the sim didn't get from there.
func (c *ConvertedItem) ApplyTo(item *proto.UIItem) {
	// field by field, since googleProto.Merge (MergeItem) appends lists and skips zero values
	src := c.Item
	item.Ilvl = src.Ilvl
	item.Quality = src.Quality
	item.Stats = slices.Clone(src.Stats)
	item.GemSockets = slices.Clone(src.GemSockets)
	item.SocketBonus = slices.Clone(src.SocketBonus)
	item.WeaponDamageMin = src.WeaponDamageMin
	item.WeaponDamageMax = src.WeaponDamageMax
	item.WeaponSpeed = src.WeaponSpeed
	item.Heroic = src.Heroic
	item.ClassAllowlist = slices.Clone(src.ClassAllowlist)
	item.SetName = src.SetName
	item.ServerStats = slices.Clone(src.ServerStats)
}

const inventoryTypeRelic = 28

// Relics leave AllowableClass open; the relic proficiency decides who can equip one.
var relicClasses = map[int32]proto.Class{
	7:  proto.Class_ClassPaladin,     // libram
	8:  proto.Class_ClassDruid,       // idol
	9:  proto.Class_ClassShaman,      // totem
	10: proto.Class_ClassDeathknight, // sigil
}

// simSetName is the name the sim's Go set bonuses know a set by, which isn't always the
// ItemSet.dbc name.
func simSetName(dbcName, itemName string) string {
	name := database.NormalizeSetName(dbcName)
	// every TBC arena season shares one set per class, named like the WotLK season sets Go registers
	// ("Gladiator's Pursuit"); keeping the season stops S2-S4 pieces from getting the WotLK bonuses
	for _, season := range []string{"Merciless", "Vengeful", "Brutal"} {
		if strings.HasPrefix(itemName, season+" Gladiator's ") && strings.HasPrefix(name, "Gladiator's ") {
			return season + " " + name
		}
	}
	// sim/mage's ItemSetKirinTorGarb has Wowhead's 10-man spelling as its AlternativeName, and
	// with_db builds panic if no item carries it
	if name == "Kirin Tor Garb" && strings.HasPrefix(itemName, "Valorous ") {
		return "Kirin'dor Garb"
	}
	return name
}

// splitArmor follows the tooltip parser: armor slots and shields keep base armor separate from bonus
// armor, while armor on jewelry, trinkets and weapons counts entirely as bonus armor.
func splitArmor(row *ItemRow) (armor, bonusArmor float64) {
	total := float64(row.Armor)
	bonus := row.ArmorDamageModifier
	if total == 0 {
		// Ranged weapons and a few odd rows set ArmorDamageModifier without any armor; tooltips show no armor for them.
		return 0, 0
	}
	switch row.InventoryType {
	case 2, 11, 12, 13, 17, 21, 22, 23: // neck, finger, trinket, weapons and held off-hands
		return 0, total
	}
	return total - bonus, bonus
}

// ConvertGem returns nil when the item isn't a gem.
func ConvertGem(row *ItemRow, dbc *DBC) (*proto.UIGem, []EnchantSpell) {
	props := dbc.GemProperties[row.GemProperties]
	if props == nil {
		return nil, nil
	}
	gem := &proto.UIGem{
		Id:      row.Entry,
		Name:    row.Name,
		Color:   GemPropertiesColor(props.Color),
		Quality: proto.ItemQuality(row.Quality),
	}
	var stats database.Stats
	var spells []EnchantSpell
	if enchant := dbc.Enchantments[props.EnchantID]; enchant != nil {
		stats, spells, _ = EnchantmentStats(enchant, dbc)
	}
	gem.Stats = stats[:]
	return gem, spells
}
