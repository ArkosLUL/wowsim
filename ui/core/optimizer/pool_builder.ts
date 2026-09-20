import { BOSS_LEVEL } from '../constants/mechanics.js';
import { OptimizeGearRequest, Player as PlayerProto, RaidSimRequest } from '../proto/api.js';
import {
	Encounter,
	EquipmentSpec,
	GemColor,
	HandType,
	ItemSlot,
	ItemSpec,
	ItemType,
	MobType,
	Profession,
	SimDatabase,
	SimEnchant,
	SimGem,
	SimItem,
	SpellSchool,
	Stat,
	Target,
	WeaponType,
} from '../proto/common.js';
import {
	CandidatePool,
	CatalogItem,
	CatalogSourceKind,
	ItemEnchantOptions,
	LimitGroup,
	MetaColorConstraint,
	MetaGemCondition,
	OptimizerEffort,
	OptimizerMetrics,
	OptimizerObjective,
	OptimizerRacialMode,
	OptimizerSettings,
	SlotPool,
	StatMinimum,
} from '../proto/optimizer.js';
import { DatabaseFilters, UIEnchant as Enchant, UIGem as Gem, UIItem as Item } from '../proto/ui.js';
import { Database } from '../proto_utils/database.js';
import { gemColorMatchesSocket, getMetaGemCondition } from '../proto_utils/gems.js';
import { slotNames } from '../proto_utils/names.js';
import { isValidReforge, Reforging, reforgingFor } from '../proto_utils/reforging.js';
import { Stats } from '../proto_utils/stats.js';
import { enchantAppliesToItem, playerProtoProfessions, raceToFaction } from '../proto_utils/utils.js';
import { distinct, getEnumValues } from '../utils.js';
import { ALL_SOURCE_KINDS, Availability, CatalogIndex, isAvailable, MAX_CONTENT_PHASE, MIN_CONTENT_PHASE } from './catalog.js';
import { filterItemsByFilters } from './item_filters.js';

// Everything the tab lets the user set.
export interface OptimizerTabSettings {
	contentPhase: number;
	effort: OptimizerEffort;
	sources: Array<CatalogSourceKind>;
	statMinimums: Array<StatMinimum>;
	lockedSlots: Array<ItemSlot>;
	excludedItemIds: Array<number>;
}

export function defaultTabSettings(contentPhase: number): OptimizerTabSettings {
	return {
		contentPhase: Math.min(MAX_CONTENT_PHASE, Math.max(MIN_CONTENT_PHASE, contentPhase)),
		effort: OptimizerEffort.OptimizerEffortQuick,
		sources: ALL_SOURCE_KINDS.slice(),
		statMinimums: [],
		lockedSlots: [],
		excludedItemIds: [],
	};
}

export interface PoolBuilderInput {
	// The individual sim's raid request (Sim.makeRaidSimRequest). Its encounter gets replaced; the
	// target's gear and racial traits are the seed.
	base: RaidSimRequest;
	targetRaidIndex: number;
	settings: OptimizerTabSettings;
	catalog: CatalogIndex;
	// The gear picker's filters, Sim.getFilters().
	filters: DatabaseFilters;
	// Player.getItems and Player.getEnchants for the target.
	getItems: (slot: ItemSlot) => Array<Item>;
	getEnchants: (slot: ItemSlot) => Array<Enchant>;
	// Gems, and the seed's items.
	db: Database;
}

export interface BuiltRequest {
	request: OptimizeGearRequest;
	// What the seed lost to fit the pool, one line each.
	seedChanges: Array<string>;
}

export const ALL_SLOTS = (getEnumValues(ItemSlot) as Array<ItemSlot>).sort((a, b) => a - b);

interface ItemOffer {
	item: Item;
	// by effect id
	enchants: Map<number, Enchant>;
}

// slot -> item id -> the item and the enchants it can take there
type SlotOffers = Map<ItemSlot, Map<number, ItemOffer>>;

// worldserver MAX_GEM_SOCKETS, and ItemChoice.Gems' size in sim/optimizer
const MAX_SERVER_SOCKETS = 3;
const MAX_GEMS = 4;

// Same boss as Encounter.defaultTargetProto(). encounter.ts can't be imported here: it drags in
// Player and the rest of the page, and this module has to run under Node for the replay fixtures.
export function plainBossTarget(): Target {
	return Target.create({
		level: BOSS_LEVEL,
		mobType: MobType.MobTypeGiant,
		tankIndex: 0,
		swingSpeed: 1.5,
		minBaseDamage: 65000,
		parryHaste: true,
		spellSchool: SpellSchool.SpellSchoolPhysical,
		stats: Stats.fromMap({
			[Stat.StatArmor]: 10643,
			[Stat.StatAttackPower]: 805,
		}).asArray(),
	});
}

// DPS specs optimize against one plain level 83 boss, whatever the sim's encounter holds. Only its
// length and the server-side settings carry over.
export function optimizerEncounter(current: Encounter | undefined): Encounter {
	return Encounter.create({
		duration: current?.duration ?? 180,
		durationVariation: current?.durationVariation ?? 5,
		executeProportion20: 0.2,
		executeProportion25: 0.25,
		executeProportion35: 0.35,
		targets: [plainBossTarget()],
		raidDifficulty: current?.raidDifficulty,
		serverSettings: current?.serverSettings,
	});
}

// A meta gem's activation rule as linear constraints; undefined when the UI doesn't know the meta,
// which the optimizer then counts as always active.
export function metaGemConditionProto(gemId: number): MetaGemCondition | undefined {
	let condition;
	try {
		condition = getMetaGemCondition(gemId);
	} catch (e) {
		return undefined;
	}
	const constraints: Array<MetaColorConstraint> = [];
	if (condition.minRed > 0) {
		constraints.push(MetaColorConstraint.create({ red: 1, minTotal: condition.minRed }));
	}
	if (condition.minYellow > 0) {
		constraints.push(MetaColorConstraint.create({ yellow: 1, minTotal: condition.minYellow }));
	}
	if (condition.minBlue > 0) {
		constraints.push(MetaColorConstraint.create({ blue: 1, minTotal: condition.minBlue }));
	}
	if (condition.compareColorGreater != GemColor.GemColorUnknown) {
		const greater = colorVector(condition.compareColorGreater);
		const lesser = colorVector(condition.compareColorLesser);
		constraints.push(
			MetaColorConstraint.create({
				red: greater[0] - lesser[0],
				yellow: greater[1] - lesser[1],
				blue: greater[2] - lesser[2],
				minTotal: 1,
			}),
		);
	}
	return MetaGemCondition.create({ gemId, constraints });
}

// How a gem counts toward meta requirements: red, yellow, blue. Orange counts as red and yellow,
// prismatic as all three.
function colorVector(color: GemColor): [number, number, number] {
	return [GemColor.GemColorRed, GemColor.GemColorYellow, GemColor.GemColorBlue].map(primary =>
		color != GemColor.GemColorMeta && gemColorMatchesSocket(color, primary) ? 1 : 0,
	) as [number, number, number];
}

function metConstraints(condition: MetaGemCondition, counts: [number, number, number]): boolean {
	return condition.constraints.every(c => c.red * counts[0] + c.yellow * counts[1] + c.blue * counts[2] >= c.minTotal);
}

// The sockets the server gives the item: its own, plus the prismatic one a buckle (belts) or
// Blacksmithing (bracers, gloves) adds when it has under 3.
function serverSockets(item: Item, blacksmith: boolean): Array<GemColor> {
	const sockets = item.gemSockets.slice();
	const extra = item.type == ItemType.ItemTypeWaist || (blacksmith && [ItemType.ItemTypeWrist, ItemType.ItemTypeHands].includes(item.type));
	if (extra && sockets.length < MAX_SERVER_SOCKETS) {
		sockets.push(GemColor.GemColorPrismatic);
	}
	return sockets.slice(0, MAX_GEMS);
}

function raidPlayer(base: RaidSimRequest, index: number): PlayerProto {
	const player = base.raid?.parties[Math.floor(index / 5)]?.players[index % 5];
	if (!player || !player.class) {
		throw new Error(`Raid slot ${index} is empty.`);
	}
	return player;
}

function toSimItem(item: Item): SimItem {
	return SimItem.create({
		id: item.id,
		name: item.name,
		type: item.type,
		armorType: item.armorType,
		weaponType: item.weaponType,
		handType: item.handType,
		rangedWeaponType: item.rangedWeaponType,
		stats: item.stats,
		gemSockets: item.gemSockets,
		socketBonus: item.socketBonus,
		weaponDamageMin: item.weaponDamageMin,
		weaponDamageMax: item.weaponDamageMax,
		weaponSpeed: item.weaponSpeed,
		setName: item.setName,
		serverStats: item.serverStats,
	});
}

// Builds the OptimizeGearRequest for the individual sim's target. Pure: no DOM and no Player, so the
// replay fixture driver runs it under Node.
//
// Per slot, the pool is the target's usable items, through the gear picker's filters, then the server
// catalog (tier, faction, sources, no PvP) and professions. Enchants follow enchantAppliesToItem.
// Excluded items and gems stay out.
//
// The seed has to pass the optimizer's equip rules, so it's trimmed to what the pool offers: that
// way a phase's result can't keep gear from a later phase. Locked slots keep the seed's choice, but a
// locked second ring, trinket or off-hand weapon moves up, lock and all, when the first slot empties.
export function buildOptimizeRequest(input: PoolBuilderInput): BuiltRequest {
	const { settings, catalog } = input;
	const base = RaidSimRequest.clone(input.base);
	const target = raidPlayer(base, input.targetRaidIndex);
	const professions = playerProtoProfessions(target);
	const blacksmith = professions.includes(Profession.Blacksmithing);
	const hasProfession = (profession: Profession) => profession == Profession.ProfessionUnknown || professions.includes(profession);
	const excluded = new Set(settings.excludedItemIds);
	const locked = new Set(settings.lockedSlots);
	const faction = raceToFaction[target.race];
	const itemAvailability: Availability = { contentPhase: settings.contentPhase, faction, sources: settings.sources };
	const gemAvailability: Availability = { contentPhase: settings.contentPhase, faction };
	const usable = (id: number, profession: Profession, availability: Availability) => {
		const row = catalog.item(id);
		return !excluded.has(id) && isAvailable(row, availability) && hasProfession(profession) && hasProfession(row!.requiredProfession);
	};

	// Locked slots too: the trimmer can move a locked item to another slot, which frees its own.
	const offers: SlotOffers = new Map();
	for (const slot of ALL_SLOTS) {
		const items = filterItemsByFilters(input.getItems(slot), item => item, slot, input.filters).filter(item =>
			usable(item.id, item.requiredProfession, itemAvailability),
		);
		const enchants = input.getEnchants(slot).filter(enchant => enchant.effectId != 0 && hasProfession(enchant.requiredProfession));
		const slotOffers = new Map<number, ItemOffer>();
		for (const item of items) {
			const fits = new Map<number, Enchant>();
			for (const enchant of enchants) {
				if (!fits.has(enchant.effectId) && enchantAppliesToItem(enchant, item)) {
					fits.set(enchant.effectId, enchant);
				}
			}
			slotOffers.set(item.id, { item, enchants: fits });
		}
		offers.set(slot, slotOffers);
	}

	const poolGems = new Map<number, Gem>();
	input.db
		.getGems()
		.filter(gem => usable(gem.id, gem.requiredProfession, gemAvailability))
		.forEach(gem => poolGems.set(gem.id, gem));
	const metaConditions = new Map<number, MetaGemCondition>();
	for (const gem of poolGems.values()) {
		const condition = gem.color == GemColor.GemColorMeta ? metaGemConditionProto(gem.id) : undefined;
		if (condition) {
			metaConditions.set(gem.id, condition);
		}
	}

	const seed = new SeedTrimmer(
		input.db,
		catalog,
		target.equipment,
		locked,
		excluded,
		blacksmith,
		offers,
		poolGems,
		metaConditions,
		reforgingFor(base.encounter?.serverSettings),
	);
	target.equipment = seed.trim(settings.contentPhase);

	const slotPools: Array<SlotPool> = [];
	const poolItems = new Map<number, Item>();
	const poolEnchants = new Map<number, Enchant>();
	for (const slot of ALL_SLOTS.filter(slot => !locked.has(slot))) {
		const enchantOptions: Array<ItemEnchantOptions> = [];
		for (const { item, enchants } of offers.get(slot)!.values()) {
			enchants.forEach((enchant, id) => poolEnchants.has(id) || poolEnchants.set(id, enchant));
			if (enchants.size > 0) {
				enchantOptions.push(ItemEnchantOptions.create({ itemId: item.id, enchantIds: [...enchants.keys()] }));
			}
			poolItems.set(item.id, item);
		}
		slotPools.push(SlotPool.create({ slot, itemIds: [...offers.get(slot)!.keys()], enchantOptions }));
	}

	for (const id of seed.gemIds()) {
		const condition = metaConditions.get(id) || metaGemConditionProto(id);
		if (condition && seed.gemColor(id) == GemColor.GemColorMeta) {
			metaConditions.set(id, condition);
		}
	}

	const catalogIds = distinct([...poolItems.keys(), ...poolGems.keys(), ...seed.itemIds(), ...seed.gemIds()]);
	const catalogItems = catalogIds.map(id => catalog.item(id)).filter((row): row is CatalogItem => !!row);
	const limitGroups = distinct(catalogItems.map(row => row.limitCategory).filter(id => id != 0))
		.map(id => catalog.limitGroup(id))
		.filter((group): group is LimitGroup => !!group);

	base.encounter = optimizerEncounter(base.encounter);
	if (base.simOptions) {
		base.simOptions.debugFirstIteration = false;
	}

	const request = OptimizeGearRequest.create({
		base,
		targetRaidIndex: input.targetRaidIndex,
		settings: OptimizerSettings.create({
			contentPhase: settings.contentPhase,
			effort: settings.effort,
			objective: OptimizerObjective.OptimizerObjectiveOwnMetrics,
			metricWeights: OptimizerMetrics.create({ dps: 1 }),
			statMinimums: settings.statMinimums.map(floor => StatMinimum.clone(floor)),
			racialMode: OptimizerRacialMode.OptimizerRacialKeepCurrent,
			sources: settings.sources.slice(),
			lockedSlots: ALL_SLOTS.filter(slot => locked.has(slot)),
			excludedItemIds: settings.excludedItemIds.slice(),
		}),
		pool: CandidatePool.create({
			slots: slotPools,
			gemIds: [...poolGems.keys()],
			metaConditions: [...metaConditions.values()],
			limitGroups,
			catalogItems,
			database: SimDatabase.create({
				items: [...poolItems.values()].map(toSimItem),
				enchants: [...poolEnchants.values()].map(enchant => SimEnchant.create({ effectId: enchant.effectId, stats: enchant.stats })),
				gems: [...poolGems.values()].map(gem => SimGem.create({ id: gem.id, name: gem.name, color: gem.color, stats: gem.stats })),
			}),
			catalogDate: catalog.date,
		}),
	});
	return { request, seedChanges: seed.changes };
}

// Trims the target's gear until the optimizer's equip rules (sim/optimizer/rules.go) accept it: each
// slot against the pool, then limits, placement and metas. It can move a lock, so it owns `locked`.
class SeedTrimmer {
	readonly changes: Array<string> = [];
	private readonly specs: Array<ItemSpec>;

	constructor(
		private readonly db: Database,
		private readonly catalog: CatalogIndex,
		equipment: EquipmentSpec | undefined,
		private readonly locked: Set<ItemSlot>,
		private readonly excluded: Set<number>,
		private readonly blacksmith: boolean,
		private readonly offers: SlotOffers,
		private readonly poolGems: Map<number, Gem>,
		private readonly metaConditions: Map<number, MetaGemCondition>,
		private readonly reforging: Reforging,
	) {
		this.specs = ALL_SLOTS.map(slot => ItemSpec.clone(equipment?.items[slot] || ItemSpec.create()));
	}

	// Limits go before placement: they can empty a first slot, and moving an item up never breaks one.
	trim(contentPhase: number): EquipmentSpec {
		ALL_SLOTS.forEach(slot => this.trimSlot(slot, contentPhase));
		this.fixLimits();
		this.moveUp(ItemSlot.ItemSlotFinger1, ItemSlot.ItemSlotFinger2);
		this.moveUp(ItemSlot.ItemSlotTrinket1, ItemSlot.ItemSlotTrinket2);
		this.moveUp(ItemSlot.ItemSlotMainHand, ItemSlot.ItemSlotOffHand);
		this.fixMetas();
		return EquipmentSpec.create({ items: this.specs });
	}

	itemIds(): Array<number> {
		return this.specs.map(spec => spec.id).filter(id => id != 0);
	}

	gemIds(): Array<number> {
		return distinct(this.specs.flatMap(spec => spec.gems).filter(id => id != 0));
	}

	gemColor(id: number): GemColor {
		return (this.poolGems.get(id) || this.db.lookupGem(id))?.color || GemColor.GemColorUnknown;
	}

	private item(id: number): Item | undefined {
		return this.db.lookupItemSpec(ItemSpec.create({ id }))?.item;
	}

	private itemName(id: number): string {
		return this.item(id)?.name || `item ${id}`;
	}

	private gemName(id: number): string {
		return (this.poolGems.get(id) || this.db.lookupGem(id))?.name || `gem ${id}`;
	}

	private note(slot: ItemSlot, text: string) {
		this.changes.push(`${slotNames.get(slot)}: ${text}`);
	}

	private clear(slot: ItemSlot, why: string) {
		this.note(slot, `left out ${this.itemName(this.specs[slot].id)} (${why}).`);
		this.specs[slot] = ItemSpec.create();
	}

	private trimSlot(slot: ItemSlot, contentPhase: number) {
		const spec = this.specs[slot];
		if (spec.id == 0) {
			this.specs[slot] = ItemSpec.create();
			return;
		}
		const item = this.item(spec.id);
		if (this.locked.has(slot) || !item) {
			return;
		}
		const offer = this.offers.get(slot)?.get(spec.id);
		if (!offer) {
			this.clear(slot, this.excluded.has(spec.id) ? 'excluded' : `not in the P${contentPhase} pool`);
			return;
		}
		if (spec.enchant != 0 && !offer.enchants.has(spec.enchant)) {
			this.note(slot, `dropped the enchant on ${item.name}, which the pool doesn't offer.`);
			spec.enchant = 0;
		}
		if (spec.reforge && (spec.reforge.fromStatType != 0 || spec.reforge.toStatType != 0) && !isValidReforge(item, spec.reforge, this.reforging)) {
			this.note(slot, `dropped the reforge on ${item.name}, which the server wouldn't allow.`);
			spec.reforge = undefined;
		}
		const sockets = serverSockets(item, this.blacksmith);
		spec.gems = spec.gems.map((id, i) => {
			if (id == 0) {
				return 0;
			}
			const gem = this.poolGems.get(id);
			if (i >= sockets.length || !gem || (gem.color == GemColor.GemColorMeta) != (sockets[i] == GemColor.GemColorMeta)) {
				const why = gem ? '' : this.excluded.has(id) ? ': it is excluded' : `: it isn't in the P${contentPhase} pool`;
				this.note(slot, `took ${this.gemName(id)} out of ${item.name}${why}.`);
				return 0;
			}
			return id;
		});
	}

	// A ring or trinket alone in the second slot, or a weapon alone in the off hand, would move up in
	// core, and the optimizer won't take a seed core rearranges. A locked item moves anyway and takes
	// its lock along. An unlocked one can't move into a locked empty slot, so it's left out.
	private moveUp(first: ItemSlot, second: ItemSlot) {
		const spec = this.specs[second];
		const item = this.item(spec.id);
		if (this.specs[first].id != 0 || !item) {
			return;
		}
		// shields and off-hand-only items stay put
		const weapon = [HandType.HandTypeOneHand, HandType.HandTypeTwoHand].includes(item.handType) && item.weaponType != WeaponType.WeaponTypeShield;
		if (second == ItemSlot.ItemSlotOffHand && !weapon) {
			return;
		}
		const firstOffer = this.offers.get(first)?.get(spec.id);
		if (this.locked.has(second)) {
			if (!this.locked.has(first)) {
				this.locked.delete(second);
				this.locked.add(first);
			}
			this.note(second, `moved ${item.name} to ${slotNames.get(first)}, and its lock with it.`);
		} else if (!this.locked.has(first) && firstOffer) {
			if (!firstOffer.enchants.has(spec.enchant)) {
				spec.enchant = 0;
			}
			this.note(second, `moved ${item.name} to ${slotNames.get(first)}.`);
		} else {
			this.clear(second, `${slotNames.get(first)} is empty`);
			return;
		}
		this.specs[first] = spec;
		this.specs[second] = ItemSpec.create();
	}

	// Unique-equipped, maxcount and ItemLimitCategory. Locked slots count first since only the rest can
	// give way; the optimizer counts in slot order, but only the totals decide whether it passes.
	private fixLimits() {
		const itemCount = new Map<number, number>();
		const gemCount = new Map<number, number>();
		const groupCount = new Map<number, number>();
		const add = (counts: Map<number, number>, id: number, n: number) => counts.set(id, (counts.get(id) || 0) + n);
		const overGroup = (row: CatalogItem | undefined) => {
			const group = row?.limitCategory ? this.catalog.limitGroup(row.limitCategory) : undefined;
			return !!group && (groupCount.get(group.id) || 0) >= group.maxEquipped;
		};

		const lockedFirst = [...ALL_SLOTS.filter(slot => this.locked.has(slot)), ...ALL_SLOTS.filter(slot => !this.locked.has(slot))];
		for (const slot of lockedFirst) {
			const spec = this.specs[slot];
			const item = this.item(spec.id);
			if (spec.id == 0 || !item) {
				continue;
			}
			const row = this.catalog.item(spec.id);
			const limit = row?.uniqueEquipped ? 1 : row?.maxCount || 0;
			if (!this.locked.has(slot) && ((limit > 0 && (itemCount.get(spec.id) || 0) >= limit) || overGroup(row))) {
				this.clear(slot, 'over its equip limit');
				continue;
			}
			add(itemCount, spec.id, 1);
			if (row?.limitCategory) {
				add(groupCount, row.limitCategory, 1);
			}

			const sockets = serverSockets(item, this.blacksmith).length;
			spec.gems = spec.gems.map((id, i) => {
				if (id == 0 || i >= sockets) {
					return id;
				}
				const gemRow = this.catalog.item(id);
				if (!this.locked.has(slot) && ((gemRow?.uniqueEquipped && (gemCount.get(id) || 0) >= 1) || overGroup(gemRow))) {
					this.note(slot, `took ${this.gemName(id)} out of ${item.name}: over its equip limit.`);
					return 0;
				}
				add(gemCount, id, 1);
				if (gemRow?.limitCategory) {
					add(groupCount, gemRow.limitCategory, 1);
				}
				return id;
			});
		}
	}

	// The server doesn't activate a meta whose colors aren't met, and the optimizer won't accept one.
	private fixMetas() {
		const counts: [number, number, number] = [0, 0, 0];
		const metas: Array<[ItemSlot, number]> = [];
		for (const slot of ALL_SLOTS) {
			const item = this.item(this.specs[slot].id);
			if (!item) {
				continue;
			}
			const sockets = serverSockets(item, this.blacksmith).length;
			this.specs[slot].gems.forEach((id, i) => {
				if (id == 0 || i >= sockets) {
					return;
				}
				const color = this.gemColor(id);
				if (color == GemColor.GemColorMeta) {
					metas.push([slot, i]);
				}
				colorVector(color).forEach((n, c) => (counts[c] += n));
			});
		}
		for (const [slot, i] of metas) {
			const id = this.specs[slot].gems[i];
			const condition = this.metaConditions.get(id) || metaGemConditionProto(id);
			if (condition && !metConstraints(condition, counts) && !this.locked.has(slot)) {
				this.note(slot, `took out ${this.gemName(id)}: its colors aren't met.`);
				this.specs[slot].gems[i] = 0;
			}
		}
	}
}
