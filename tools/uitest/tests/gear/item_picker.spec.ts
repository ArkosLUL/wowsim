import { expect, type Locator, test } from '@playwright/test';

import { openSimTab, watchForErrors } from '../lib/page';
import { loadCatalog, obtainableBy } from './catalog';
import {
	closeModal,
	closePicker,
	equippedId,
	equipRow,
	expectMovedBy,
	FLAT_RATINGS,
	listSize,
	loadDb,
	openGear,
	openPicker,
	pane,
	picker,
	readStats,
	rowIds,
	rowNames,
	rows,
	search,
	setPhase,
	stat,
	statsAfterChange,
} from './gear';

test('equipping an item shows it in the slot and moves the stats by what it carries', async ({ page }) => {
	const errors = watchForErrors(page);
	await openGear(page);
	const db = await loadDb(page);

	const dialog = await openPicker(page, 'neck');
	const items = pane(dialog, 'Items');
	const worn = await readStats(page);
	await items.locator('.selector-modal-remove-button').click();
	await expect(picker(page, 'neck').locator('.item-picker-name')).toHaveText('Neck');
	const bare = await statsAfterChange(page, worn);

	// the first listed neck that carries a rating nothing else feeds, so the move is exact
	const ids = await rowIds(items);
	const id = ids.find(id => FLAT_RATINGS.some(key => stat(db.items.get(id)?.stats, key) > 0))!;
	expect(id, 'a listed neck with hit, haste, armor pen or expertise').toBeTruthy();
	const item = db.items.get(id)!;

	await equipRow(items, id);
	await closePicker(page);
	await expect(picker(page, 'neck').locator('.item-picker-name')).toContainText(item.name);
	await expect.poll(() => equippedId(page, 'neck')).toBe(id);
	await expect(picker(page, 'neck').locator('.item-picker-ilvl')).toHaveText(String(item.ilvl));
	const after = await statsAfterChange(page, bare);

	expectMovedBy(bare, after, item.stats);
	expect(errors).toEqual([]);
});

test('the search keeps the items whose names hold every word typed', async ({ page }) => {
	await openGear(page);
	const items = pane(await openPicker(page, 'head'), 'Items');
	const everything = await listSize(items);

	// a word from a listed name, typed with its punctuation left out and in the wrong case
	const name = (await rowNames(items)).find(n => /\w+'\w+/.test(n)) ?? (await rowNames(items))[0];
	const word = /\w+'\w+/.exec(name)?.[0] ?? name.split(' ')[0];
	await search(items, word.replace(/'/g, '').toUpperCase());

	await expect.poll(() => listSize(items)).toBeLessThan(everything);
	const found = await rowNames(items);
	expect(found.length).toBeGreaterThan(0);
	for (const n of found) expect(n.toLowerCase()).toContain(word.toLowerCase());

	const second = name.split(' ').find(w => w != word && /^[A-Za-z]{3,}$/.test(w));
	if (second) {
		await search(items, `${word} ${second}`.replace(/'/g, ''));
		for (const n of await rowNames(items)) {
			expect(n.toLowerCase()).toContain(word.toLowerCase());
			expect(n.toLowerCase()).toContain(second.toLowerCase());
		}
	}

	await search(items, '');
	await expect.poll(() => listSize(items)).toBe(everything);
});

test('the phase filter lists what the server hands out by then, plus what is worn', async ({ page }) => {
	await openGear(page);
	const catalog = await loadCatalog(page);
	const items = pane(await openPicker(page, 'head'), 'Items');
	const worn = await equippedId(page, 'head');

	await setPhase(items, 5);
	const late = await listSize(items);
	await setPhase(items, 1);
	await expect.poll(() => listSize(items)).toBeLessThan(late);

	for (const id of await rowIds(items)) {
		if (id == worn) continue;
		expect(obtainableBy(catalog, id, 1), `item ${id} at phase 1`).toBe(true);
	}

	// the worn helm stays listed at phase 1 even when phase 1 can't hand it out
	const wornName = await picker(page, 'head')
		.locator('.item-picker-name')
		.evaluate(e => e.firstChild?.textContent ?? '');
	await search(items, wornName);
	expect(await rowIds(items)).toContain(worn);
});

test('an item the phase only keeps because it is worn leaves the list once swapped off', async ({ page }) => {
	await openGear(page);
	const catalog = await loadCatalog(page);
	const worn = await equippedId(page, 'head');
	test.skip(obtainableBy(catalog, worn, 1), 'the preset helm drops at phase 1 already');

	const items = pane(await openPicker(page, 'head'), 'Items');
	await setPhase(items, 1);
	const other = (await rowIds(items)).find(id => id != worn)!;
	await equipRow(items, other);
	await expect.poll(() => equippedId(page, 'head')).toBe(other);
	await expect.poll(async () => (await rowIds(items)).includes(worn)).toBe(false);
});

test('unticking a source in the filters drops the items that only come from it', async ({ page }) => {
	await openGear(page);
	const db = await loadDb(page);
	const dialog = await openPicker(page, 'head');
	const items = pane(dialog, 'Items');
	await setPhase(items, 5);

	// a crafted helm the list shows: search each crafted head piece until one turns up
	const crafted = [...db.items.values()].filter(item => item.type == 1 && item.sources?.length && item.sources.every(s => 'crafted' in s));
	let found = 0;
	for (const item of crafted) {
		await search(items, item.name.replace(/[^\w\s]/g, ''));
		if ((await rowIds(items)).includes(item.id)) {
			found = item.id;
			break;
		}
	}
	expect(found, 'a crafted helm in the list').toBeTruthy();

	await items.locator('.selector-modal-filters-button').click();
	const filters = page.locator('.modal.show .filters-menu');
	await expect(filters).toBeVisible();
	const crafting = filters.locator('.boolean-picker-root', { hasText: 'Crafting' }).locator('input');
	await crafting.uncheck();
	await expect.poll(async () => (await rowIds(items)).includes(found)).toBe(false);
	await crafting.check();
	await expect.poll(async () => (await rowIds(items)).includes(found)).toBe(true);
});

test('a favourite item goes to the top of the list and stays there after a reload', async ({ page }) => {
	await openGear(page);
	let items = pane(await openPicker(page, 'neck'), 'Items');
	const ids = await rowIds(items);
	const fav = ids[5];
	await rows(items).nth(5).locator('.selector-modal-list-item-favorite').click();
	await expect.poll(async () => (await rowIds(items))[0]).toBe(fav);
	await expect(rows(items).first()).toHaveAttribute('data-fav', 'true');
	await expect(rows(items).first().locator('.selector-modal-list-item-favorite i')).toHaveClass(/fas/);

	await page.reload();
	await page.waitForLoadState('networkidle');
	await openSimTab(page, 'gear-tab');
	items = pane(await openPicker(page, 'neck'), 'Items');
	await expect.poll(async () => (await rowIds(items))[0]).toBe(fav);

	await rows(items).first().locator('.selector-modal-list-item-favorite').click();
	await expect.poll(async () => (await rowIds(items))[0]).toBe(ids[0]);
});

test("Show EP lists each item's EP and how far it is from the worn one's", async ({ page }) => {
	await openGear(page);
	const items = pane(await openPicker(page, 'neck'), 'Items');
	await expect(items.locator('.ep-label')).toBeHidden();
	await items.locator('.selector-modal-show-ep-values input').check();
	await expect(items.locator('.ep-label')).toBeVisible();

	const readEp = () =>
		rows(items).evaluateAll(lis =>
			lis.map(li => ({
				worn: li.classList.contains('active'),
				ep: Number(li.querySelector('.selector-modal-list-item-ep-value')!.textContent),
				delta: li.querySelector('.selector-modal-list-item-ep-delta')!.textContent ?? '',
			})),
		);
	const check = async () => {
		const shown = await readEp();
		const worn = shown.find(r => r.worn);
		expect(worn, 'the worn neck is listed').toBeTruthy();
		expect(worn!.delta).toBe('');
		for (const row of shown.filter(r => !r.worn && r.delta)) {
			// both EPs are rounded for display, the delta isn't
			expect(Math.abs(Number(row.delta) - (row.ep - worn!.ep))).toBeLessThanOrEqual(1);
		}
	};
	await check();

	// the deltas follow the newly worn item
	await equipRow(items, (await rowIds(items))[3]);
	await expect.poll(async () => (await readEp())[3].worn).toBe(true);
	await check();
});

test('the filters menu narrows the list by faction, armor type and weapon speed, and keeps them after a reload', async ({ page }) => {
	await openGear(page);
	const db = await loadDb(page);
	const menu = page.locator('.modal.show .filters-menu');
	const openFilters = async (slot: 'head' | 'mainHand') => {
		const items = pane(await openPicker(page, slot), 'Items');
		await setPhase(items, 5);
		await items.locator('.selector-modal-filters-button').click();
		await expect(menu).toBeVisible();
		return items;
	};
	const closeFilters = () => closeModal(menu);
	const listed = async (items: Locator) => (await rowIds(items)).map(id => db.items.get(id)!);

	let items = await openFilters('head');
	await menu.locator('.enum-picker-root', { hasText: 'Faction Restrictions' }).locator('select').selectOption({ label: 'Horde only' });
	await menu
		.locator('.boolean-picker-root', { hasText: /^Plate$/ })
		.locator('input')
		.uncheck();
	await expect.poll(async () => (await listed(items)).filter(i => i.factionRestriction == 1 || i.armorType == 4).length).toBe(0);
	expect((await listed(items)).length).toBeGreaterThan(0);
	await closeFilters();
	await closePicker(page);

	items = await openFilters('mainHand');
	const speed = menu.locator('.number-picker-root', { hasText: 'Min MH Speed' }).locator('input');
	await speed.fill('3.5');
	await speed.dispatchEvent('change');
	await expect.poll(async () => (await listed(items)).every(i => (i.weaponSpeed ?? 0) >= 3.5)).toBe(true);
	await closeFilters();
	await closePicker(page);

	await page.reload();
	await page.waitForLoadState('networkidle');
	await openSimTab(page, 'gear-tab');
	items = await openFilters('head');
	await expect(menu.locator('.enum-picker-root', { hasText: 'Faction Restrictions' }).locator('select')).toHaveValue('2');
	await expect(menu.locator('.boolean-picker-root', { hasText: /^Plate$/ }).locator('input')).not.toBeChecked();
	expect((await listed(items)).filter(i => i.armorType == 4)).toEqual([]);
});

test('the 1H and 2H boxes on the main hand list only weapons of that kind', async ({ page }) => {
	await openGear(page);
	const db = await loadDb(page);
	const items = pane(await openPicker(page, 'mainHand'), 'Items');
	const oneHand = items.locator('.selector-modal-show-1h-weapons input');
	const twoHand = items.locator('.selector-modal-show-2h-weapons input');
	const worn = await equippedId(page, 'mainHand');
	const kinds = async () => (await rowIds(items)).filter(id => id != worn).map(id => db.items.get(id)!.handType == 4);

	await oneHand.setChecked(true);
	await twoHand.setChecked(false);
	await expect.poll(async () => (await kinds()).every(twoHander => !twoHander)).toBe(true);
	const oneHanders = await listSize(items);
	await oneHand.setChecked(false);
	await twoHand.setChecked(true);
	await expect.poll(async () => (await kinds()).every(twoHander => twoHander)).toBe(true);
	const twoHanders = await listSize(items);
	await oneHand.setChecked(true);
	await expect.poll(() => listSize(items)).toBe(oneHanders + twoHanders);
});
