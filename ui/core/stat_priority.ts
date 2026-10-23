import { StatCap, StatCapKind } from './proto/api.js';
import { ItemReforge, ItemSlot, PseudoStat, Spec, Stat } from './proto/common.js';
import { UIItem as Item } from './proto/ui.js';
import { isValidReforge, reforgeAmount, Reforging, validReforges } from './proto_utils/reforging.js';
import { LiveValues, ShippedBuild, ShippedPhase, Slope, SlopeSet } from './stat_values.js';

// tmi ranks by how much TMI drops, so higher is better for all of them
export type Metric = 'dps' | 'tps' | 'tmi' | 'hps';

// A stat the way gear carries it. Generic hit, crit and haste rating land on both the melee and the
// spell stat, and attack power on ranged attack power too, so their value is the sum of both slopes.
export interface GearStat {
	key: string;
	label: string;
	stats: Array<Stat>;
	pseudoStats: Array<PseudoStat>;
	// the ItemModTypes reforgeStatTypeToStats (sim/core/reforging.go) lands on exactly these stats
	reforgeTypes: Array<number>;
}

function gearStat(key: string, label: string, stats: Array<Stat>, reforgeTypes: Array<number> = [], pseudoStats: Array<PseudoStat> = []): GearStat {
	return { key, label, stats, pseudoStats, reforgeTypes };
}

export const GEAR_STATS: Array<GearStat> = [
	gearStat('strength', 'Strength', [Stat.StatStrength], [4]),
	gearStat('agility', 'Agility', [Stat.StatAgility], [3]),
	gearStat('stamina', 'Stamina', [Stat.StatStamina], [7]),
	gearStat('intellect', 'Intellect', [Stat.StatIntellect], [5]),
	gearStat('spirit', 'Spirit', [Stat.StatSpirit], [6]),
	gearStat('hit', 'Hit rating', [Stat.StatMeleeHit, Stat.StatSpellHit], [31]),
	gearStat('crit', 'Crit rating', [Stat.StatMeleeCrit, Stat.StatSpellCrit], [32]),
	gearStat('haste', 'Haste rating', [Stat.StatMeleeHaste, Stat.StatSpellHaste], [36]),
	gearStat('expertise', 'Expertise rating', [Stat.StatExpertise], [37]),
	gearStat('armorPen', 'Armor penetration rating', [Stat.StatArmorPenetration], [44]),
	gearStat('attackPower', 'Attack power', [Stat.StatAttackPower, Stat.StatRangedAttackPower], [38]),
	gearStat('spellPower', 'Spell power', [Stat.StatSpellPower], [42, 45]),
	gearStat('mp5', 'Mana per 5 sec', [Stat.StatMP5], [43]),
	gearStat('defense', 'Defense rating', [Stat.StatDefense], [12]),
	gearStat('dodge', 'Dodge rating', [Stat.StatDodge], [13]),
	gearStat('parry', 'Parry rating', [Stat.StatParry], [14]),
	gearStat('block', 'Block rating', [Stat.StatBlock], [15]),
	gearStat('blockValue', 'Block value', [Stat.StatBlockValue], [48]),
	gearStat('armor', 'Bonus armor', [Stat.StatArmor]),
	gearStat('mainHandDps', 'Main hand weapon DPS', [], [], [PseudoStat.PseudoStatMainHandDps]),
	gearStat('offHandDps', 'Off hand weapon DPS', [], [], [PseudoStat.PseudoStatOffHandDps]),
	gearStat('rangedDps', 'Ranged weapon DPS', [], [], [PseudoStat.PseudoStatRangedDps]),
];

export interface BuildChoice {
	build: ShippedBuild;
	// false when no build of the spec has the player's main tree, so the spec's first one stands in
	treeMatched: boolean;
}

export function chooseBuild(builds: Array<ShippedBuild>, spec: Spec, tree: number, twoHand: boolean): BuildChoice | null {
	const forSpec = builds.filter(b => b.spec == spec);
	if (forSpec.length == 0) {
		return null;
	}
	const forTree = forSpec.filter(b => b.tree == tree);
	const pool = forTree.length > 0 ? forTree : forSpec;
	return {
		build: pool.find(b => b.twoHand == twoHand) ?? pool[0],
		treeMatched: forTree.length > 0 || forSpec.length == 1,
	};
}

export function phaseOf(build: ShippedBuild, phase: number): ShippedPhase | null {
	return build.phases.find(p => p.phase == phase) ?? null;
}

export function metricValue(slope: Slope | undefined, metric: Metric): number {
	if (!slope) {
		return 0;
	}
	return metric == 'tmi' ? -slope.tmi : slope[metric];
}

export type CapState = { kind: 'below' } | { kind: 'over'; surplus: number } | { kind: 'whiteOnly' };

const BELOW: CapState = { kind: 'below' };

// statweights measures these below their cap, so past it the shipped value says nothing. Defense
// and haste are measured where BiS has them: past crit immunity or the GCD floor they still count.
const CAPPED_STATS = [Stat.StatMeleeHit, Stat.StatSpellHit, Stat.StatExpertise, Stat.StatArmorPenetration];

function surplus(cap: StatCap): number | null {
	return Number.isFinite(cap.needed) ? Math.round(cap.rating) - Math.round(cap.needed) : null;
}

// Where the gear stands against every hard cap on the stat: both hands, dodge and parry. Past the
// yellow hit cap, a dual wielder short of the white one is whiteOnly.
export function capState(stat: Stat, caps: Array<StatCap>): CapState {
	if (!CAPPED_STATS.includes(stat)) {
		return BELOW;
	}
	const own = caps.filter(cap => cap.stat == stat);
	const hard = own.filter(cap => cap.kind != StatCapKind.StatCapSoft);
	if (hard.length == 0) {
		return BELOW;
	}
	let least = Infinity;
	for (const cap of hard) {
		const s = surplus(cap);
		if (s == null || s < 0) {
			return BELOW;
		}
		least = Math.min(least, s);
	}
	for (const cap of own.filter(cap => cap.kind == StatCapKind.StatCapSoft)) {
		const s = surplus(cap);
		if (s == null || s < 0) {
			return { kind: 'whiteOnly' };
		}
		least = Math.min(least, s);
	}
	return { kind: 'over', surplus: least };
}

export interface PriorityRow {
	gearStat: GearStat;
	// per point, after the cap rule; a live run's gain
	value: number;
	// a live run's cost of one point fewer, null for shipped values or a stat the player has none of
	loss: number | null;
	// value over the reference stat's, null when that isn't above zero
	ratio: number | null;
	note: string;
	capped: boolean;
	reforgeable: boolean;
}

export const WHITE_ONLY_NOTE = 'white swings only: measure at your gear';

// for a stat with only some parts capped, which is hit
const PART_NAMES: Partial<Record<Stat, string>> = {
	[Stat.StatMeleeHit]: 'attacks',
	[Stat.StatSpellHit]: 'spells',
};

interface Part {
	stat?: Stat;
	raw: number;
	state: CapState;
}

function capNote(capped: Array<Part>, all: boolean): string {
	if (capped.some(part => part.state.kind == 'whiteOnly')) {
		return WHITE_ONLY_NOTE;
	}
	const least = Math.min(...capped.map(part => (part.state.kind == 'over' ? part.state.surplus : Infinity)));
	if (!all) {
		const names = capped.map(part => (part.stat !== undefined && PART_NAMES[part.stat]) || '').filter(name => name);
		return `capped for ${names.join(' and ') || 'part of it'} (${least} over)`;
	}
	return least > 0 ? `${least} over: reforge away` : 'at the cap';
}

export function isReforgeable(stat: GearStat, reforging: Reforging): boolean {
	return reforging.enabled && stat.reforgeTypes.some(type => reforging.statTypes.includes(type));
}

// The phase's stats as gear carries them, best first. A stat the build wasn't measured on, or that
// did nothing, is left out. Past a hard cap more of a stat is worth 0; a stat with only some of its
// parts capped, like hit for a melee that also casts, keeps the rest.
export function priority(phase: ShippedPhase, metric: Metric, caps: Array<StatCap>, referenceStat: Stat, reforging: Reforging): Array<PriorityRow> {
	const reference = metricValue(phase.stats.get(referenceStat), metric);
	const rows: Array<PriorityRow & { order: number }> = [];
	GEAR_STATS.forEach((stat, order) => {
		const parts: Array<Part> = [
			...stat.stats.map(s => ({ stat: s, raw: metricValue(phase.stats.get(s), metric), state: capState(s, caps) })),
			...stat.pseudoStats.map(s => ({ raw: metricValue(phase.pseudoStats.get(s), metric), state: BELOW })),
		].filter(part => part.raw != 0);
		if (parts.length == 0) {
			return;
		}
		const capped = parts.filter(part => part.state.kind != 'below');
		const value = parts.filter(part => part.state.kind == 'below').reduce((sum, part) => sum + part.raw, 0);
		rows.push({
			gearStat: stat,
			value,
			loss: null,
			ratio: reference > 0 ? value / reference : null,
			note: capped.length > 0 ? capNote(capped, capped.length == parts.length) : '',
			capped: capped.length > 0,
			reforgeable: isReforgeable(stat, reforging),
			order,
		});
	});
	return sortRows(rows);
}

function sortRows(rows: Array<PriorityRow & { order: number }>): Array<PriorityRow> {
	rows.sort((a, b) => b.value - a.value || a.order - b.order);
	return rows.map(({ order: _order, ...row }) => row);
}

function slopesOf(set: SlopeSet, stat: GearStat): Array<Slope | undefined> {
	return [...stat.stats.map(s => set.stats.get(s)), ...stat.pseudoStats.map(s => set.pseudoStats.get(s))];
}

// The live run's stats as gear carries them, best gain first. Measured at the player's gear, caps
// and all, so no cap rule. A stat the run left alone, or that did nothing either way, is left out.
export function livePriority(live: LiveValues, metric: Metric, referenceStat: Stat, reforging: Reforging): Array<PriorityRow> {
	const reference = metricValue(live.gain.stats.get(referenceStat), metric);
	const rows: Array<PriorityRow & { order: number }> = [];
	GEAR_STATS.forEach((stat, order) => {
		const gains = slopesOf(live.gain, stat);
		const losses = slopesOf(live.loss, stat);
		if (gains.every(slope => !slope)) {
			return;
		}
		const value = gains.reduce((sum, slope) => sum + metricValue(slope, metric), 0);
		const loss = losses.some(slope => slope) ? losses.reduce((sum, slope) => sum + metricValue(slope, metric), 0) : null;
		if (value == 0 && !loss) {
			return;
		}
		rows.push({
			gearStat: stat,
			value,
			loss,
			ratio: reference > 0 ? value / reference : null,
			note: '',
			capped: false,
			reforgeable: isReforgeable(stat, reforging),
			order,
		});
	});
	return sortRows(rows);
}

// reforgeStatTypeToStats in sim/core/reforging.go: the sim stats each item_template ItemModType lands on
export const REFORGE_TYPE_STATS: Record<number, Array<Stat>> = {
	0: [Stat.StatMana],
	1: [Stat.StatHealth],
	3: [Stat.StatAgility],
	4: [Stat.StatStrength],
	5: [Stat.StatIntellect],
	6: [Stat.StatSpirit],
	7: [Stat.StatStamina],
	12: [Stat.StatDefense],
	13: [Stat.StatDodge],
	14: [Stat.StatParry],
	15: [Stat.StatBlock],
	16: [Stat.StatMeleeHit],
	17: [Stat.StatMeleeHit],
	18: [Stat.StatSpellHit],
	19: [Stat.StatMeleeCrit],
	20: [Stat.StatMeleeCrit],
	21: [Stat.StatSpellCrit],
	28: [Stat.StatMeleeHaste],
	29: [Stat.StatMeleeHaste],
	30: [Stat.StatSpellHaste],
	31: [Stat.StatMeleeHit, Stat.StatSpellHit],
	32: [Stat.StatMeleeCrit, Stat.StatSpellCrit],
	35: [Stat.StatResilience],
	36: [Stat.StatMeleeHaste, Stat.StatSpellHaste],
	37: [Stat.StatExpertise],
	38: [Stat.StatAttackPower, Stat.StatRangedAttackPower],
	39: [Stat.StatRangedAttackPower],
	42: [Stat.StatSpellPower],
	43: [Stat.StatMP5],
	44: [Stat.StatArmorPenetration],
	45: [Stat.StatSpellPower],
	47: [Stat.StatSpellPenetration],
	48: [Stat.StatBlockValue],
};

// What the reforge does to the sim's stats: nothing when the server settings refuse it, same as
// core.ReforgeStats.
export function reforgeDelta(item: Item, reforge: ItemReforge | null, reforging: Reforging): Map<Stat, number> {
	const delta = new Map<Stat, number>();
	if (!reforge || !isValidReforge(item, reforge, reforging)) {
		return delta;
	}
	const amount = reforgeAmount(item, reforge.fromStatType, reforging);
	const add = (stats: Array<Stat> | undefined, by: number) => stats?.forEach(stat => delta.set(stat, (delta.get(stat) ?? 0) + by));
	add(REFORGE_TYPE_STATS[reforge.fromStatType], -amount);
	add(REFORGE_TYPE_STATS[reforge.toStatType], amount);
	return delta;
}

// One stat's worth against a change in its rating, counted from the gear as it is now.
interface ValueLine {
	// per point added
	gain: number;
	// per point taken away
	loss: number;
	// rating to add before every hard cap on the stat is met; past it more is worth 0
	room: number;
	// the same up to the soft caps too. Rating taken away below this costs the full value: past the
	// yellow hit cap a dual wielder's hit still lands white swings, which the shipped value can't price.
	whiteRoom: number;
}

const UNCAPPED = { room: Infinity, whiteRoom: Infinity };

function roomTo(caps: Array<StatCap>): number {
	return Math.max(-Infinity, ...caps.map(cap => (Number.isFinite(cap.needed) ? Math.round(cap.needed) - Math.round(cap.rating) : Infinity)));
}

// Same hard and soft caps as capState.
function shippedLine(stat: Stat, value: number, caps: Array<StatCap>): ValueLine {
	const own = CAPPED_STATS.includes(stat) ? caps.filter(cap => cap.stat == stat) : [];
	const hard = own.filter(cap => cap.kind != StatCapKind.StatCapSoft);
	if (hard.length == 0) {
		return { gain: value, loss: value, ...UNCAPPED };
	}
	const room = roomTo(hard);
	return { gain: value, loss: value, room, whiteRoom: Math.max(room, roomTo(own.filter(cap => cap.kind == StatCapKind.StatCapSoft))) };
}

// A live run already measured the caps, so no cap rule. A stat the player has none of has no loss,
// and no reforge can take it away.
function liveLine(stat: Stat, live: LiveValues, metric: Metric): ValueLine {
	const gain = metricValue(live.gain.stats.get(stat), metric);
	const loss = live.loss.stats.get(stat);
	return { gain, loss: loss ? metricValue(loss, metric) : gain, ...UNCAPPED };
}

export function isLive(values: ShippedPhase | LiveValues): values is LiveValues {
	return 'gain' in values;
}

// length of [from, to] below limit
function below(from: number, to: number, limit: number): number {
	return Math.max(0, Math.min(to, limit) - from);
}

function moveValue(line: ValueLine, at: number, change: number): number {
	if (change > 0) {
		return line.gain * below(at, at + change, line.room);
	}
	return -line.loss * below(at + change, at, line.whiteRoom);
}

export interface ReforgeSlot {
	slot: ItemSlot;
	item: Item;
	reforge: ItemReforge | null;
}

export interface ReforgeSuggestion extends ReforgeSlot {
	// false when the server settings refuse the equipped reforge, so the sim skips it
	active: boolean;
	// undefined keeps the item as it is, null takes its reforge off
	suggested?: ItemReforge | null;
	gain: number;
	// false when the suggestion keeps the floors only after the picks before it, so it can't be applied alone
	alone: boolean;
}

// float noise between two equal values isn't a gain
const MIN_GAIN = 1e-6;
// a pick must gain at least this share of the value it gives up, so near-equal stats don't churn
const MIN_RELATIVE_GAIN = 0.05;

interface Floor {
	stat: Stat;
	// rating past the cap now, negative when short
	start: number;
	// the lowest the picks may take it
	limit: number;
}

// Every cap that costs when missed. A met one stays met and a short one never gets shorter.
function floorsOf(caps: Array<StatCap>): Array<Floor> {
	return caps
		.filter(cap => cap.kind == StatCapKind.StatCapFloor && Number.isFinite(cap.needed))
		.map(cap => {
			const start = Math.round(cap.rating) - Math.round(cap.needed);
			return { stat: cap.stat, start, limit: Math.min(0, start) };
		});
}

// One suggestion per item, picked greedily: the best gain first, and each pick spends the cap
// surplus or room the later ones are scored against. No pick, after the ones before it, takes a
// floor cap below where it stands or below zero, whichever is lower. An item's current reforge
// comes off before a candidate goes on. Items with nothing to offer and no reforge are left out.
// Live values score added rating at their gain and removed rating at their loss.
export function suggestReforges(
	items: Array<ReforgeSlot>,
	values: ShippedPhase | LiveValues,
	metric: Metric,
	caps: Array<StatCap>,
	reforging: Reforging,
): Array<ReforgeSuggestion> {
	const lines = new Map<Stat, ValueLine>();
	const line = (stat: Stat) => {
		let found = lines.get(stat);
		if (!found) {
			found = isLive(values) ? liveLine(stat, values, metric) : shippedLine(stat, metricValue(values.stats.get(stat), metric), caps);
			lines.set(stat, found);
		}
		return found;
	};
	const moved = new Map<Stat, number>();
	const floors = floorsOf(caps);
	const keepsFloors = (change: Map<Stat, number>, after: Map<Stat, number>) =>
		floors.every(floor => floor.start + (after.get(floor.stat) ?? 0) + (change.get(floor.stat) ?? 0) >= floor.limit);
	// net gain, and what the rating taken away was worth
	const gainOf = (change: Map<Stat, number>) => {
		let gain = 0;
		let given = 0;
		change.forEach((by, stat) => {
			const part = moveValue(line(stat), moved.get(stat) ?? 0, by);
			gain += part;
			given -= Math.min(0, part);
		});
		return { gain, given };
	};

	const results: Array<ReforgeSuggestion> = [];
	const open: Array<{ result: ReforgeSuggestion; candidates: Array<{ reforge: ItemReforge | null; change: Map<Stat, number> }> }> = [];
	for (const { slot, item, reforge } of items) {
		const options: Array<ItemReforge | null> = validReforges(item, reforging).filter(r => !reforge || !ItemReforge.equals(r, reforge));
		if (reforge) {
			options.push(null);
		}
		if (options.length == 0) {
			continue;
		}
		const current = reforgeDelta(item, reforge, reforging);
		const result: ReforgeSuggestion = { slot, item, reforge, active: !!reforge && isValidReforge(item, reforge, reforging), gain: 0, alone: true };
		results.push(result);
		open.push({
			result,
			candidates: options.map(option => {
				const change = reforgeDelta(item, option, reforging);
				current.forEach((by, stat) => change.set(stat, (change.get(stat) ?? 0) - by));
				return { reforge: option, change };
			}),
		});
	}

	for (;;) {
		let best: { index: number; reforge: ItemReforge | null; change: Map<Stat, number>; gain: number } | null = null;
		for (let index = 0; index < open.length; index++) {
			for (const { reforge, change } of open[index].candidates) {
				if (!keepsFloors(change, moved)) {
					continue;
				}
				const { gain, given } = gainOf(change);
				if (gain > MIN_GAIN && gain >= MIN_RELATIVE_GAIN * given && (!best || gain > best.gain)) {
					best = { index, reforge, change, gain };
				}
			}
		}
		if (!best) {
			break;
		}
		const [{ result }] = open.splice(best.index, 1);
		result.suggested = best.reforge;
		result.gain = best.gain;
		result.alone = keepsFloors(best.change, new Map());
		best.change.forEach((by, stat) => moved.set(stat, (moved.get(stat) ?? 0) + by));
	}
	return results;
}
