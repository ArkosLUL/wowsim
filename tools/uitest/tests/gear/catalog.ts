import type { Page } from '@playwright/test';

interface CatalogRow {
	id: number;
	progressionTier: number;
	pvp?: boolean;
	sources?: { progressionTier: number }[];
}

let catalogPromise: Promise<Map<number, CatalogRow>> | null = null;

// The server catalog the page reads its phase filter from (ADR 0005).
export function loadCatalog(page: Page): Promise<Map<number, CatalogRow>> {
	if (!catalogPromise) {
		catalogPromise = page.request
			.get('/wotlk/assets/database/server_catalog.json')
			.then(response => response.json())
			.then(json => new Map(json.items.map((row: CatalogRow) => [row.id, row])));
	}
	return catalogPromise;
}

// Progression tier 12 + N opens content phase N. No row, or a PvP one, is never obtainable.
export function obtainableBy(catalog: Map<number, CatalogRow>, id: number, phase: number): boolean {
	const row = catalog.get(id);
	if (!row || row.pvp) return false;
	const cap = 12 + phase;
	if (!row.sources?.length) return row.progressionTier <= cap;
	return row.sources.some(source => source.progressionTier <= cap);
}
