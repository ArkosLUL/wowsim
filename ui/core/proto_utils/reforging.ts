import { ItemReforge, Stat } from '../proto/common.js';
import { UIItem as Item } from '../proto/ui.js';

// Server defaults from mod_reforging.conf: Reforging.Percentage and Reforging.ReforgeableStats.
// Keep in sync with sim/core/reforging.go.
export const REFORGE_PERCENTAGE = 0.4;
export const REFORGEABLE_STAT_TYPES = [6, 13, 14, 31, 32, 36, 37];

// ItemModType -> sim stats. hit, crit and haste ratings count for both melee and spell
const reforgeStatTypeToStats: Record<number, Array<Stat>> = {
	6: [Stat.StatSpirit],
	13: [Stat.StatDodge],
	14: [Stat.StatParry],
	31: [Stat.StatMeleeHit, Stat.StatSpellHit],
	32: [Stat.StatMeleeCrit, Stat.StatSpellCrit],
	36: [Stat.StatMeleeHaste, Stat.StatSpellHaste],
	37: [Stat.StatExpertise],
};

const reforgeStatTypeNames: Record<number, string> = {
	6: 'Spirit',
	13: 'Dodge',
	14: 'Parry',
	31: 'Hit',
	32: 'Crit',
	36: 'Haste',
	37: 'Expertise',
};

export function reforgeStatTypeName(statType: number): string {
	return reforgeStatTypeNames[statType] || `Stat ${statType}`;
}

function itemStatTypeValue(item: Item, statType: number): number {
	return Math.max(0, ...(reforgeStatTypeToStats[statType] || []).map(stat => item.stats[stat] || 0));
}

export function reforgeAmount(item: Item, fromStatType: number): number {
	return Math.floor(REFORGE_PERCENTAGE * itemStatTypeValue(item, fromStatType));
}

export function isValidReforge(item: Item, reforge: ItemReforge | null | undefined): boolean {
	return !!reforge
		&& reforge.fromStatType != reforge.toStatType
		&& REFORGEABLE_STAT_TYPES.includes(reforge.fromStatType)
		&& REFORGEABLE_STAT_TYPES.includes(reforge.toStatType)
		&& itemStatTypeValue(item, reforge.toStatType) == 0
		&& reforgeAmount(item, reforge.fromStatType) >= 1;
}

export function validReforges(item: Item): Array<ItemReforge> {
	return REFORGEABLE_STAT_TYPES.flatMap(fromStatType => REFORGEABLE_STAT_TYPES.map(toStatType => ItemReforge.create({ fromStatType, toStatType })))
		.filter(reforge => isValidReforge(item, reforge));
}

// e.g. "33 Crit → Haste"
export function reforgeLabel(item: Item, reforge: ItemReforge): string {
	return `${reforgeAmount(item, reforge.fromStatType)} ${reforgeStatTypeName(reforge.fromStatType)} → ${reforgeStatTypeName(reforge.toStatType)}`;
}
