import { expect, type Locator, type Page } from '@playwright/test';

import { openSim, openSimTab } from '../lib/page';

// The gear tab's item pickers in page order: the left column, then the right one.
export const SLOTS = [
	'head',
	'neck',
	'shoulder',
	'back',
	'chest',
	'wrist',
	'mainHand',
	'offHand',
	'ranged',
	'hands',
	'waist',
	'legs',
	'feet',
	'finger1',
	'finger2',
	'trinket1',
	'trinket2',
] as const;
export type Slot = (typeof SLOTS)[number];

// Indexes into an item's `stats` array in db.json, from the Stat enum in proto/common.proto.
export const STAT = {
	strength: 0,
	agility: 1,
	stamina: 2,
	intellect: 3,
	spirit: 4,
	spellPower: 5,
	spellHit: 7,
	spellCrit: 8,
	spellHaste: 9,
	attackPower: 11,
	meleeHit: 12,
	meleeCrit: 13,
	meleeHaste: 14,
	armorPen: 15,
	expertise: 16,
	armor: 20,
	dodge: 25,
	parry: 26,
} as const;

// The character stats table's labels for the stats above.
export const STAT_LABEL: Record<keyof typeof STAT, string> = {
	strength: 'Strength',
	agility: 'Agility',
	stamina: 'Stamina',
	intellect: 'Intellect',
	spirit: 'Spirit',
	spellPower: 'Spell Dmg',
	spellHit: 'Spell Hit',
	spellCrit: 'Spell Crit',
	spellHaste: 'Spell Haste',
	attackPower: 'Attack Power',
	meleeHit: 'Melee Hit',
	meleeCrit: 'Melee Crit',
	meleeHaste: 'Melee Haste',
	armorPen: 'Armor Pen',
	expertise: 'Expertise',
	armor: 'Armor',
	dodge: 'Dodge',
	parry: 'Parry',
};

// Ratings no talent, buff or other stat converts into, so gear moves them one for one.
export const FLAT_RATINGS = ['meleeHit', 'meleeHaste', 'armorPen', 'expertise'] as const;

export const GEM_COLOR = { meta: 1, red: 2, blue: 3, yellow: 4, green: 5, orange: 6, purple: 7, prismatic: 8 } as const;

export interface DbItem {
	id: number;
	name: string;
	type: number;
	ilvl: number;
	handType?: number;
	armorType?: number;
	weaponSpeed?: number;
	factionRestriction?: number;
	stats: number[];
	gemSockets?: number[];
	socketBonus: number[];
	heroic?: boolean;
	unique?: boolean;
	serverStats?: { statType: number; value: number }[];
	sources?: any[];
}

export interface DbGem {
	id: number;
	name: string;
	color: number;
	stats: number[];
	unique?: boolean;
	requiredProfession?: number;
}

export interface DbEnchant {
	effectId: number;
	spellId?: number;
	itemId?: number;
	name: string;
	type: number;
	stats: number[];
}

export interface Db {
	items: Map<number, DbItem>;
	gems: Map<number, DbGem>;
	enchants: DbEnchant[];
}

let dbPromise: Promise<Db> | null = null;

// The item database the page itself loads, so expectations follow whatever items the build ships.
export function loadDb(page: Page): Promise<Db> {
	if (!dbPromise) {
		dbPromise = page.request
			.get('/wotlk/assets/database/db.json')
			.then(response => response.json())
			.then(json => ({
				items: new Map(json.items.map((item: DbItem) => [item.id, item])),
				gems: new Map(json.gems.map((gem: DbGem) => [gem.id, gem])),
				enchants: json.enchants,
			}));
	}
	return dbPromise;
}

export const stat = (values: number[] | undefined, key: keyof typeof STAT) => values?.[STAT[key]] ?? 0;

export async function openGear(page: Page, spec = 'warrior') {
	await openSim(page, spec);
	await openSimTab(page, 'gear-tab');
}

export const picker = (page: Page, slot: Slot) => page.locator('#gear-tab .gear-picker-root .item-picker-root').nth(SLOTS.indexOf(slot));

// The name link's href is a wowhead item URL once the item's icon data has loaded; 0 for an empty slot.
export async function equippedId(page: Page, slot: Slot): Promise<number> {
	const href = (await picker(page, slot).locator('.item-picker-name').getAttribute('href')) ?? '';
	return Number(/item=(\d+)/.exec(href)?.[1] ?? 0);
}

// The gem ids the slot's tooltip data carries, 0 for an empty socket.
export async function equippedGems(page: Page, slot: Slot): Promise<number[]> {
	const data = (await picker(page, slot).locator('.item-picker-name').getAttribute('data-wowhead')) ?? '';
	const gems = /gems=([\d:]+)/.exec(data)?.[1];
	return gems ? gems.split(':').map(Number) : [];
}

export const modal = (page: Page) => page.locator('.modal.show .selector-modal');

// Opens the slot's selector on the tab its link leads to: the item, its enchant or its reforge.
export async function openPicker(page: Page, slot: Slot, via: 'icon' | 'enchant' | 'reforge' = 'icon'): Promise<Locator> {
	const target = { icon: '.item-picker-icon', enchant: '.item-picker-enchant', reforge: '.item-picker-reforge' }[via];
	await picker(page, slot).locator(target).click();
	await expect(modal(page)).toBeVisible();
	return modal(page);
}

// Bootstrap ignores a close while the modal is still fading in, so this clicks until it takes. A
// slow page can take an earlier click after its retry gave up, so only click while it's still open.
export async function closeModal(dialog: Locator) {
	await expect(async () => {
		if (await dialog.isVisible()) await dialog.locator('.close-button').first().click({ timeout: 1000 });
		await expect(dialog).toBeHidden({ timeout: 1000 });
	}).toPass();
}

export async function closePicker(page: Page) {
	await closeModal(modal(page));
	await expect(page.locator('.modal.show')).toHaveCount(0);
}

// A selector tab's pane: 'Items', 'Enchants', 'Reforging', 'Gem1'...
export const pane = (dialog: Locator, id: string) => dialog.locator(`#${id}-tab`);

export async function showTab(dialog: Locator, id: string): Promise<Locator> {
	await dialog.locator(`a[data-content-id="${id}-tab"]`).click();
	await expect(pane(dialog, id)).toHaveClass(/show/);
	return pane(dialog, id);
}

export const rows = (tab: Locator) => tab.locator('li.selector-modal-list-item');

export async function search(tab: Locator, text: string) {
	await tab.locator('.selector-modal-search').fill(text);
}

export async function rowNames(tab: Locator): Promise<string[]> {
	return rows(tab).locator('.selector-modal-list-item-name').allInnerTexts();
}

// The drawn rows' links, once every one has its wowhead href: item=ID, or spell=ID for most enchants.
async function rowHrefs(tab: Locator): Promise<string[]> {
	let hrefs: string[] = [];
	await expect
		.poll(async () => {
			hrefs = await rows(tab)
				.locator('a.selector-modal-list-item-link')
				.evaluateAll(links => links.map(link => link.getAttribute('href') ?? ''));
			return hrefs.every(href => /(item|spell)=\d+/.test(href));
		})
		.toBe(true);
	return hrefs;
}

export async function rowIds(tab: Locator): Promise<number[]> {
	return (await rowHrefs(tab)).map(href => Number(/item=(\d+)/.exec(href)?.[1] ?? 0));
}

// The db.json enchants the drawn rows of an Enchants tab stand for.
export async function rowEnchants(tab: Locator, db: Db): Promise<DbEnchant[]> {
	return (await rowHrefs(tab)).map(href => {
		const [, kind, id] = /(item|spell)=(\d+)/.exec(href)!;
		return db.enchants.find(e => (kind == 'spell' ? e.spellId : e.itemId) == Number(id))!;
	});
}

// Rows in the virtual list, drawn or not: every row is 56px tall.
export async function listSize(tab: Locator): Promise<number> {
	return tab.locator('.selector-modal-list').evaluate(list => Math.round(list.scrollHeight / 56));
}

export async function setPhase(tab: Locator, phase: number) {
	await tab.locator('.selector-modal-phase-selector select').selectOption(String(phase));
}

// Clicks the row for this item or gem id, so the test equips exactly what it looked up. The list only
// draws the rows near its scroll position, so it scrolls until the row is drawn.
export async function equipRow(tab: Locator, id: number) {
	const row = tab.locator(`li.selector-modal-list-item a.selector-modal-list-item-link[href*="item=${id}?"]`);
	const list = tab.locator('.selector-modal-list');
	const height = await list.evaluate(l => l.scrollHeight);
	for (let top = 0; top <= height && (await row.count()) == 0; top += 400) {
		await list.evaluate((l, y) => {
			l.scrollTop = y;
			l.dispatchEvent(new Event('scroll'));
		}, top);
		await tab.page().waitForTimeout(50);
	}
	await row.click();
}

// Every stat the character stats panel shows, by label, as the number before any percentage.
export async function readStats(page: Page): Promise<Record<string, number>> {
	const cells = await page
		.locator('.character-stats-table-row')
		.evaluateAll(rowElems =>
			rowElems.map(row => [
				row.querySelector('.character-stats-table-label')?.textContent ?? '',
				row.querySelector('.stat-value-link')?.textContent ?? '',
			]),
		);
	const out: Record<string, number> = {};
	for (const [label, value] of cells) {
		const match = /-?\d+/.exec(value);
		if (match) out[label.trim()] = Number(match[0]);
	}
	return out;
}

// Waits for the stats panel to show something other than `before`: the sim works stats out off the
// page, so they land a moment after the gear changes.
export async function statsAfterChange(page: Page, before: Record<string, number>): Promise<Record<string, number>> {
	let after = before;
	await expect
		.poll(async () => {
			after = await readStats(page);
			return JSON.stringify(after) != JSON.stringify(before);
		})
		.toBe(true);
	return after;
}

// Checks the panel moved the way `values` (a db.json stats array) says. Ratings nothing converts
// move exactly; crit too unless agility came along; primary stats get multiplied by buffs and
// talents, so they move at least that far, and not at all when `values` has none.
export function expectMovedBy(before: Record<string, number>, after: Record<string, number>, values: number[]) {
	const moved = (key: keyof typeof STAT) => after[STAT_LABEL[key]] - before[STAT_LABEL[key]];
	for (const key of FLAT_RATINGS) {
		expect(moved(key), STAT_LABEL[key]).toBe(stat(values, key));
	}
	if (stat(values, 'agility') == 0) {
		expect(moved('meleeCrit'), 'Melee Crit').toBe(stat(values, 'meleeCrit'));
	}
	for (const key of ['strength', 'agility', 'stamina'] as const) {
		if (stat(values, key) == 0) {
			expect(moved(key), STAT_LABEL[key]).toBe(0);
		} else {
			expect(moved(key), STAT_LABEL[key]).toBeGreaterThanOrEqual(stat(values, key));
		}
	}
}

export const minus = (a: number[], b: number[]) => a.map((v, i) => v - (b[i] ?? 0));
export const plus = (a: number[], b: number[]) => a.map((v, i) => v + (b[i] ?? 0));

// The warnings the sidebar's triangle lists, as text.
export async function warnings(page: Page): Promise<string> {
	const titles = await page.locator('.link-warning').evaluateAll(links => links.map(link => link.getAttribute('data-bs-title') ?? ''));
	return titles.join(' ');
}

// Sums what the gems and a met socket bonus add, the way the sim does.
export function gemStats(db: Db, item: DbItem, gems: number[]): number[] {
	const total = new Array(40).fill(0);
	const add = (values: number[]) => values.forEach((v, i) => (total[i] += v));
	gems.filter(id => id > 0).forEach(id => add(db.gems.get(id)!.stats));
	const sockets = item.gemSockets ?? [];
	const bonusMet = sockets.every((color, i) => gems[i] > 0 && gemFits(db.gems.get(gems[i])!.color, color));
	if (sockets.length > 0 && bonusMet) add(item.socketBonus);
	return total;
}

// Which gem colours count for a socket's bonus.
export function gemFits(gemColor: number, socketColor: number): boolean {
	const fits: Record<number, number[]> = {
		[GEM_COLOR.meta]: [GEM_COLOR.meta],
		[GEM_COLOR.red]: [GEM_COLOR.red, GEM_COLOR.orange, GEM_COLOR.purple, GEM_COLOR.prismatic],
		[GEM_COLOR.yellow]: [GEM_COLOR.yellow, GEM_COLOR.orange, GEM_COLOR.green, GEM_COLOR.prismatic],
		[GEM_COLOR.blue]: [GEM_COLOR.blue, GEM_COLOR.green, GEM_COLOR.purple, GEM_COLOR.prismatic],
		[GEM_COLOR.prismatic]: Object.values(GEM_COLOR).filter(c => c != GEM_COLOR.meta),
	};
	return (fits[socketColor] ?? []).includes(gemColor);
}

// Turns on the header menu's "Show Experimental", which reveals the Batch tab.
export async function showExperimental(page: Page) {
	await page.locator('.sim-options').click();
	const menu = page.locator('.modal.show .settings-menu');
	await expect(menu).toBeVisible();
	await menu.locator('.show-experimental-picker input').check();
	await closeModal(menu);
}

// Elements matching `selector` that are off the page but still held in memory after a garbage
// collection, which means some listener or cache still points at them.
export async function detached(page: Page, selector: string): Promise<number> {
	const cdp = await page.context().newCDPSession(page);
	await cdp.send('HeapProfiler.collectGarbage');
	const { result: proto } = await cdp.send('Runtime.evaluate', { expression: 'HTMLElement.prototype', objectGroup: 'heap' });
	const { objects } = await cdp.send('Runtime.queryObjects', { prototypeObjectId: proto.objectId!, objectGroup: 'heap' });
	// the query also finds each element type's prototype, which throws on isConnected
	const { result } = await cdp.send('Runtime.callFunctionOn', {
		functionDeclaration: `function (selector) {
			return this.filter(e => {
				try {
					return !e.isConnected && e.matches(selector);
				} catch {
					return false;
				}
			}).length;
		}`,
		objectId: objects.objectId!,
		arguments: [{ value: selector }],
		returnByValue: true,
	});
	await cdp.send('Runtime.releaseObjectGroup', { objectGroup: 'heap' });
	await cdp.detach();
	return Number(result.value);
}
