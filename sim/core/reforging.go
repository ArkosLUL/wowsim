package core

import (
	"math"
	"slices"

	"github.com/wowsims/wotlk/sim/core/proto"
	"github.com/wowsims/wotlk/sim/core/stats"
)

// MaxItemProtoStats is the worldserver's MAX_ITEM_PROTO_STATS. mod-reforging refuses an item whose
// StatsCount is 0 or at least this, so only 1 to 9 stats can be reforged.
const MaxItemProtoStats = 10

// reforgeStatTypeToStats turns an item_template ItemModType into the sim stats it lands on. It has
// to agree with the item database's conversion (AddItemMod in tools/database/azerothcore), because
// a reforge moves the template's value and the sim moves it between the stats that value became:
// generic hit, crit and haste ratings count for both melee and spell, and attack power also counts
// as ranged attack power. A type missing here is one the sim has no stat for, and a reforge naming
// it does nothing rather than inventing or losing a stat.
var reforgeStatTypeToStats = map[int32][]stats.Stat{
	0:  {stats.Mana},
	1:  {stats.Health},
	3:  {stats.Agility},
	4:  {stats.Strength},
	5:  {stats.Intellect},
	6:  {stats.Spirit},
	7:  {stats.Stamina},
	12: {stats.Defense},
	13: {stats.Dodge},
	14: {stats.Parry},
	15: {stats.Block},
	16: {stats.MeleeHit},
	17: {stats.MeleeHit},
	18: {stats.SpellHit},
	19: {stats.MeleeCrit},
	20: {stats.MeleeCrit},
	21: {stats.SpellCrit},
	28: {stats.MeleeHaste},
	29: {stats.MeleeHaste},
	30: {stats.SpellHaste},
	31: {stats.MeleeHit, stats.SpellHit},
	32: {stats.MeleeCrit, stats.SpellCrit},
	35: {stats.Resilience},
	36: {stats.MeleeHaste, stats.SpellHaste},
	37: {stats.Expertise},
	38: {stats.AttackPower, stats.RangedAttackPower},
	39: {stats.RangedAttackPower},
	42: {stats.SpellPower},
	43: {stats.MP5},
	44: {stats.ArmorPenetration},
	45: {stats.SpellPower},
	47: {stats.SpellPenetration},
	48: {stats.BlockValue},
}

// ReforgeStats returns the stat change a reforge makes on the item, or zero stats when the server
// wouldn't allow it. reforging is the raid's mod-reforging config, from Character.Server(); nil
// means the live server's.
//
// It follows ItemReforge::IsReforgeable and ::Reforge, which read item_template rather than what the
// sim made of it: the source has to be one of the item's own stats, and the target one it doesn't
// have. So a rating only an equip spell grants is no source, and neither is a melee-only or
// spell-only rating standing in for the combined one.
func ReforgeStats(item *Item, reforge *proto.ItemReforge, reforging *Reforging) stats.Stats {
	var delta stats.Stats
	if item == nil || reforge == nil {
		return delta
	}
	if reforging == nil {
		reforging = LiveReforging()
	}
	if !reforging.Enabled || !reforging.reforgeableItem(item) {
		return delta
	}
	if !reforging.ReforgeableStatPair(reforge.FromStatType, reforge.ToStatType) {
		return delta
	}

	fromValue := item.serverStatValue(reforge.FromStatType)
	// the server refuses a target stat the item already has
	if fromValue <= 0 || item.serverStatValue(reforge.ToStatType) > 0 {
		return delta
	}
	fromStats, toStats := reforgeStatTypeToStats[reforge.FromStatType], reforgeStatTypeToStats[reforge.ToStatType]
	if len(fromStats) == 0 || len(toStats) == 0 {
		return delta
	}

	amount := reforging.Amount(fromValue)
	if amount < 1 {
		return delta
	}
	for _, stat := range fromStats {
		delta[stat] -= amount
	}
	for _, stat := range toStats {
		delta[stat] += amount
	}
	return delta
}

// CanReforge says whether the server would let this reforge sit on the item. It's the same rule
// ReforgeStats applies, so callers that only need the yes/no can't drift from it.
func CanReforge(item *Item, reforge *proto.ItemReforge, reforging *Reforging) bool {
	return ReforgeStats(item, reforge, reforging) != stats.Stats{}
}

// Amount is what CalculateReforgePct moves off a template stat of that value: the configured
// percentage of it, floored, in float32 like the server.
func (r *Reforging) Amount(statValue int32) float64 {
	if statValue <= 0 {
		return 0
	}
	return math.Floor(float64(float32(statValue) * (float32(r.Percentage) / 100)))
}

// ReforgeableStatPair says whether the config's stat list allows this pair of ItemModTypes at all.
// It's the half of the rule that doesn't need the item; ReforgeStats applies the rest.
func (r *Reforging) ReforgeableStatPair(fromStatType, toStatType int32) bool {
	return fromStatType != toStatType &&
		slices.Contains(r.StatTypes, fromStatType) && slices.Contains(r.StatTypes, toStatType)
}

// reforgeableItem is ItemReforge::IsReforgeable's item half: a stat list to reforge with, and 1 to
// 9 template stats. Its quality check needs no counterpart here, since the item database only
// carries ServerStats for items whose item_template row holds their stats, and heirlooms, the only
// equippable quality above legendary, scale theirs.
func (r *Reforging) reforgeableItem(item *Item) bool {
	return len(r.StatTypes) > 0 &&
		len(item.ServerStats) > 0 && len(item.ServerStats) < MaxItemProtoStats
}

// serverStatValue is what LoadItemStatInfo plus FindItemStat give for a stat type: the first
// item_template row of that type with a value above zero, else 0.
func (item *Item) serverStatValue(statType int32) int32 {
	for _, stat := range item.ServerStats {
		if stat.Type == statType && stat.Value > 0 {
			return stat.Value
		}
	}
	return 0
}
