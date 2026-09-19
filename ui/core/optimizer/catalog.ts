import { Faction } from '../proto/common.js';
import { CatalogItem, CatalogSourceKind, LimitGroup, ServerCatalog } from '../proto/optimizer.js';

export const CATALOG_URL = '/wotlk/assets/database/server_catalog.json';

// Progression tier N + 12 is content phase N: tier 13 opens Naxx (phase 1), 17 opens RS (phase 5).
export const PHASE_TIER_OFFSET = 12;
export const MIN_CONTENT_PHASE = 1;
export const MAX_CONTENT_PHASE = 5;

export function tierCap(contentPhase: number): number {
	return PHASE_TIER_OFFSET + contentPhase;
}

// Source kinds the tab offers as checkboxes, in display order. CatalogSourceUnknown isn't one: those
// sources are Classic fallbacks and always count.
export const SOURCE_KINDS: Array<{ kind: CatalogSourceKind; label: string }> = [
	{ kind: CatalogSourceKind.CatalogSourceDungeon, label: 'Dungeon' },
	{ kind: CatalogSourceKind.CatalogSourceDungeonHeroic, label: 'Heroic dungeon' },
	{ kind: CatalogSourceKind.CatalogSourceRaid10, label: 'Raid 10' },
	{ kind: CatalogSourceKind.CatalogSourceRaid10Heroic, label: 'Raid 10 heroic' },
	{ kind: CatalogSourceKind.CatalogSourceRaid25, label: 'Raid 25' },
	{ kind: CatalogSourceKind.CatalogSourceRaid25Heroic, label: 'Raid 25 heroic' },
	{ kind: CatalogSourceKind.CatalogSourceVendor, label: 'Vendor (emblems, tokens)' },
	{ kind: CatalogSourceKind.CatalogSourceCrafted, label: 'Crafted' },
	{ kind: CatalogSourceKind.CatalogSourceQuest, label: 'Quest' },
	{ kind: CatalogSourceKind.CatalogSourceReputation, label: 'Reputation' },
	{ kind: CatalogSourceKind.CatalogSourceAchievement, label: 'Achievement' },
	{ kind: CatalogSourceKind.CatalogSourceProspecting, label: 'Prospecting' },
	{ kind: CatalogSourceKind.CatalogSourceWorldDrop, label: 'World drop' },
];

export const ALL_SOURCE_KINDS: Array<CatalogSourceKind> = SOURCE_KINDS.map(s => s.kind);

// The server catalog, indexed for the pool builder.
export class CatalogIndex {
	readonly date: string;
	private readonly items: Map<number, CatalogItem>;
	private readonly limitGroups: Map<number, LimitGroup>;

	constructor(catalog: ServerCatalog) {
		this.date = catalog.date;
		this.items = new Map(catalog.items.map(item => [item.id, item]));
		this.limitGroups = new Map(catalog.limitGroups.map(group => [group.id, group]));
	}

	item(id: number): CatalogItem | undefined {
		return this.items.get(id);
	}

	limitGroup(id: number): LimitGroup | undefined {
		return this.limitGroups.get(id);
	}
}

export interface Availability {
	contentPhase: number;
	faction: Faction;
	// Source kinds to count; undefined counts every kind (gems: all of them are crafted or prospected).
	sources?: Array<CatalogSourceKind>;
}

// Whether the server hands the item out by the phase, to this faction, from one of the allowed sources.
// No row means nothing obtainable awards it. PvP items never count.
export function isAvailable(row: CatalogItem | undefined, availability: Availability): boolean {
	if (!row || row.pvp) {
		return false;
	}
	if (row.faction != Faction.Unknown && row.faction != availability.faction) {
		return false;
	}
	const cap = tierCap(availability.contentPhase);
	if (row.sources.length == 0) {
		return row.progressionTier <= cap;
	}
	return row.sources.some(
		source =>
			source.progressionTier <= cap &&
			(source.kind == CatalogSourceKind.CatalogSourceUnknown || !availability.sources || availability.sources.includes(source.kind)),
	);
}

let loadPromise: Promise<CatalogIndex> | null = null;

// Fetches the catalog once per page. A failed fetch isn't cached, so a retry fetches again.
export function loadCatalog(): Promise<CatalogIndex> {
	if (!loadPromise) {
		loadPromise = fetch(CATALOG_URL)
			.then(response => {
				if (!response.ok) {
					throw new Error(`Couldn't load the server catalog (${CATALOG_URL}): HTTP ${response.status}`);
				}
				return response.json();
			})
			.then(json => new CatalogIndex(ServerCatalog.fromJson(json, { ignoreUnknownFields: true })));
		loadPromise.catch(() => (loadPromise = null));
	}
	return loadPromise;
}
