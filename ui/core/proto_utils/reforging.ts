import {
	LIVE_SERVER_DEFAULTS,
	MAX_REFORGEABLE_STAT_TYPES,
	REFORGE_DEFAULT_PERCENTAGE,
	REFORGE_MAX_PERCENTAGE,
	REFORGE_MIN_PERCENTAGE,
} from '../constants/server_defaults_auto_gen.js';
import { ItemReforge, ServerSettings } from '../proto/common.js';
import { UIItem as Item } from '../proto/ui.js';

// MAX_ITEM_PROTO_STATS: mod-reforging refuses an item whose StatsCount is 0 or at least this.
const MAX_ITEM_PROTO_STATS = 10;

// Reforging is mod-reforging's config as the sim resolves it; core.Reforging in Go.
export interface Reforging {
	enabled: boolean;
	// Percentage of the source stat a reforge moves, e.g. 40.
	percentage: number;
	// ItemModType ids a reforge can take from or give to.
	statTypes: Array<number>;
}

// reforgingFor resolves an encounter's settings over the live server's, the same way newReforging
// does in sim/core/server_settings.go.
export function reforgingFor(settings?: ServerSettings | null): Reforging {
	const live = LIVE_SERVER_DEFAULTS.reforge;
	const reforge = settings?.reforge;

	let percentage = reforge?.percentage ?? live?.percentage ?? REFORGE_DEFAULT_PERCENTAGE;
	if (percentage < REFORGE_MIN_PERCENTAGE || percentage > REFORGE_MAX_PERCENTAGE) {
		percentage = REFORGE_DEFAULT_PERCENTAGE;
	}

	let statTypes = reforge?.statTypes?.length ? reforge.statTypes : live?.statTypes ?? [];
	if (statTypes.length > MAX_REFORGEABLE_STAT_TYPES) {
		statTypes = [];
	}

	return { enabled: reforge?.enable ?? live?.enable ?? false, percentage, statTypes: [...statTypes] };
}

export const LIVE_REFORGING = reforgingFor();

// mod-reforging's StatTypeToString, for the ItemModTypes a config can list.
const reforgeStatTypeNames: Record<number, string> = {
	0: 'Mana',
	1: 'Health',
	3: 'Agility',
	4: 'Strength',
	5: 'Intellect',
	6: 'Spirit',
	7: 'Stamina',
	12: 'Defense',
	13: 'Dodge',
	14: 'Parry',
	15: 'Block',
	16: 'Melee Hit',
	17: 'Ranged Hit',
	18: 'Spell Hit',
	19: 'Melee Crit',
	20: 'Ranged Crit',
	21: 'Spell Crit',
	28: 'Melee Haste',
	29: 'Ranged Haste',
	30: 'Spell Haste',
	31: 'Hit',
	32: 'Crit',
	35: 'Resilience',
	36: 'Haste',
	37: 'Expertise',
	38: 'Attack Power',
	39: 'Ranged Attack Power',
	42: 'Spell Power',
	43: 'Mana Regen',
	44: 'Armor Penetration',
	45: 'Spell Power',
	47: 'Spell Penetration',
	48: 'Block Value',
};

export function reforgeStatTypeName(statType: number): string {
	return reforgeStatTypeNames[statType] || `Stat ${statType}`;
}

// The stat types a config can sensibly list: the ones the sim has a stat for, which is the same
// set reforgeStatTypeToStats covers in sim/core/reforging.go.
export const KNOWN_REFORGE_STAT_TYPES = Object.keys(reforgeStatTypeNames).map(Number);

// serverStatValue is LoadItemStatInfo plus FindItemStat: the first item_template row of that type
// with a value above zero, else 0.
function serverStatValue(item: Item, statType: number): number {
	return item.serverStats.find(stat => stat.statType == statType && stat.value > 0)?.value ?? 0;
}

// reforgeAmount is CalculateReforgePct: the configured percentage of the template stat, floored, in
// float32 like the server.
export function reforgeAmount(item: Item, fromStatType: number, config: Reforging = LIVE_REFORGING): number {
	const value = serverStatValue(item, fromStatType);
	if (value <= 0) {
		return 0;
	}
	return Math.floor(Math.fround(Math.fround(value) * Math.fround(Math.fround(config.percentage) / 100)));
}

// isValidReforge follows ItemReforge::IsReforgeable and ::Reforge, which read the item_template
// stats rather than what the sim made of them. A stat type the sim has no stat for is out: core's
// ReforgeStats drops the pair, so offering it here would show a reforge the sim then ignores.
export function isValidReforge(item: Item, reforge: ItemReforge | null | undefined, config: Reforging = LIVE_REFORGING): boolean {
	return (
		!!reforge &&
		config.enabled &&
		config.statTypes.length > 0 &&
		item.serverStats.length > 0 &&
		item.serverStats.length < MAX_ITEM_PROTO_STATS &&
		reforge.fromStatType != reforge.toStatType &&
		config.statTypes.includes(reforge.fromStatType) &&
		config.statTypes.includes(reforge.toStatType) &&
		KNOWN_REFORGE_STAT_TYPES.includes(reforge.fromStatType) &&
		KNOWN_REFORGE_STAT_TYPES.includes(reforge.toStatType) &&
		serverStatValue(item, reforge.toStatType) == 0 &&
		reforgeAmount(item, reforge.fromStatType, config) >= 1
	);
}

export function validReforges(item: Item, config: Reforging = LIVE_REFORGING): Array<ItemReforge> {
	return config.statTypes
		.flatMap(fromStatType => config.statTypes.map(toStatType => ItemReforge.create({ fromStatType, toStatType })))
		.filter(reforge => isValidReforge(item, reforge, config));
}

// e.g. "33 Crit → Haste"
export function reforgeLabel(item: Item, reforge: ItemReforge, config: Reforging = LIVE_REFORGING): string {
	return `${reforgeAmount(item, reforge.fromStatType, config)} ${reforgeStatTypeName(reforge.fromStatType)} → ${reforgeStatTypeName(
		reforge.toStatType,
	)}`;
}
