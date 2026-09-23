import { expect, type Locator, type Page, test } from '@playwright/test';

import { openSimTab, watchForErrors } from '../lib/page';
import {
	closePicker,
	detached,
	equippedId,
	equipRow,
	listSize,
	loadDb,
	modal,
	openGear,
	openPicker,
	pane,
	rowEnchants,
	rowIds,
	rowNames,
	showExperimental,
	showTab,
} from './gear';

const bulk = (page: Page) => page.locator('#bulk-tab');
const batchItems = (page: Page) => bulk(page).locator('.bulk-items .bulk-item-picker');
const setupBox = (page: Page, name: string) => bulk(page).locator('.bulk-settings .boolean-picker-root', { hasText: name }).locator('input');
const defaultSockets = (page: Page) => bulk(page).locator('.default-gem-container .gem-socket-container');

async function openBulk(page: Page) {
	await showExperimental(page);
	await openSimTab(page, 'bulk-tab');
}

async function reloadBulk(page: Page) {
	await page.reload();
	await page.waitForLoadState('networkidle');
	await openSimTab(page, 'bulk-tab');
}

// Adds the first item the Add Item search finds for `text`, and returns its name.
async function addBySearch(page: Page, text: string): Promise<string> {
	const input = bulk(page).locator('input[placeholder="search..."]');
	if (!(await input.isVisible())) await bulk(page).locator('button', { hasText: 'Add Item' }).click();
	await input.fill(text);
	const hit = bulk(page).locator('.batch-search-results li').first();
	const name = (await hit.locator('span').first().innerText()).trim();
	await hit.click();
	return name;
}

async function setIterations(page: Page, n: number) {
	const input = page.locator('.iterations-picker input');
	await input.fill(String(n));
	await input.dispatchEvent('change');
}

const dps = async (block: Locator) => Number(await block.locator('.topline-result-avg').innerText());

test('items found with Add Item or sent from the item picker join the batch', async ({ page }) => {
	await openGear(page);
	await openBulk(page);
	const name = await addBySearch(page, 'Penumbra');
	await expect(batchItems(page)).toHaveCount(1);
	await expect(batchItems(page).locator('.item-picker-name')).toContainText(name);

	await openSimTab(page, 'gear-tab');
	const items = pane(await openPicker(page, 'neck'), 'Items');
	await items.locator('.selector-modal-simall-button').click();
	await closePicker(page);
	await openSimTab(page, 'bulk-tab');
	await expect.poll(() => batchItems(page).count()).toBeGreaterThan(1);
});

test('a small batch lists the worn gear and each combination, and Equip wears the pick', async ({ page }) => {
	test.setTimeout(90_000);
	await openGear(page);
	const worn = await equippedId(page, 'neck');
	await setIterations(page, 50);
	await openBulk(page);

	const db = await loadDb(page);
	const name = await addBySearch(page, 'Penumbra');
	const neck = [...db.items.values()].find(i => i.name == name && i.id != worn)!;

	await bulk(page).locator('button', { hasText: 'Simulate Batch' }).click();
	const results = bulk(page).locator('.bulk-results .bulk-result');
	await expect(bulk(page).locator('.bulk-results')).toBeVisible({ timeout: 60_000 });
	await expect(results).toHaveCount(2);
	await expect(bulk(page).locator('button', { hasText: 'Simulate Batch' })).toBeEnabled();

	const base = results.filter({ hasText: 'No changes' });
	const withNeck = results.filter({ hasText: name });
	await expect(base).toHaveCount(1);
	await expect(withNeck).toHaveCount(1);
	const delta = Number(await withNeck.locator('.results-sim-dps span').nth(1).innerText());
	expect(delta).toBeCloseTo((await dps(withNeck)) - (await dps(base)), 1);

	await withNeck.locator('.bulk-equipit').click();
	await expect(page.locator('#gear-tab')).toHaveClass(/show/);
	await expect.poll(() => equippedId(page, 'neck')).toBe(neck.id);
});

test('redrawing the batch list lets go of the sockets it drew before', async ({ page }) => {
	await openGear(page);
	const db = await loadDb(page);
	await openBulk(page);

	// a wrist item, whose extra blacksmithing socket keeps watching the professions
	const name = await addBySearch(page, 'Bracers');
	expect([...db.items.values()].find(i => i.name == name)?.type, name).toBe(6);
	// every add redraws the whole list
	for (let i = 0; i < 4; i++) await addBySearch(page, 'Bracers');
	await expect(batchItems(page)).toHaveCount(5);
	await expect.poll(() => detached(page, '.gem-socket-container')).toBe(0);
});

test('Clear All empties the batch and its results', async ({ page }) => {
	test.setTimeout(90_000);
	await openGear(page);
	await setIterations(page, 50);
	await openBulk(page);
	await addBySearch(page, 'Penumbra');
	await bulk(page).locator('button', { hasText: 'Simulate Batch' }).click();
	await expect(bulk(page).locator('.bulk-results')).toBeVisible({ timeout: 60_000 });

	await bulk(page).locator('button', { hasText: 'Clear All' }).click();
	await expect(batchItems(page)).toHaveCount(0);
	await expect(bulk(page).locator('.bulk-results')).toBeHidden();
});

test('a batch item is offered the enchants that fit it, not the worn one', async ({ page }) => {
	// deathknights list one- and two-handers alike
	await openGear(page, 'deathknight');
	const db = await loadDb(page);
	const worn = db.items.get(await equippedId(page, 'mainHand'))!;
	await openBulk(page);

	// a main hand of the other handedness than the worn one, from the gear tab's own list
	await openSimTab(page, 'gear-tab');
	const items = pane(await openPicker(page, 'mainHand'), 'Items');
	const pick = (await rowIds(items)).find(id => (db.items.get(id)!.handType == 4) != (worn.handType == 4))!;
	expect(pick, 'a listed main hand of the other handedness').toBeTruthy();
	await closePicker(page);
	await openSimTab(page, 'bulk-tab');
	await addBySearch(page, db.items.get(pick)!.name);

	// it opens on the enchants, which have to be listed straight away
	await batchItems(page).first().locator('.item-picker-icon').click();
	await expect(modal(page).locator('#Enchants-tab li.selector-modal-list-item').first()).toBeVisible();
	const inBatch = await showTab(modal(page), 'Enchants');
	const forBatch = { size: await listSize(inBatch), ids: (await rowEnchants(inBatch, db)).map(e => e.effectId).sort() };
	await closePicker(page);

	await openSimTab(page, 'gear-tab');
	const dialog = await openPicker(page, 'mainHand');
	await equipRow(pane(dialog, 'Items'), pick);
	const asWorn = await showTab(dialog, 'Enchants');
	expect(forBatch).toEqual({ size: await listSize(asWorn), ids: (await rowEnchants(asWorn, db)).map(e => e.effectId).sort() });
});

test('the Auto Gem defaults list gems on opening, and a pick shows in its socket after a reload', async ({ page }) => {
	const errors = watchForErrors(page);
	await openGear(page);
	await openBulk(page);

	await defaultSockets(page).nth(1).click();
	await expect(modal(page)).toBeVisible();
	const list = modal(page).locator('li.selector-modal-list-item');
	await expect(list.first()).toBeVisible();
	const name = (await rowNames(modal(page)))[0];
	await list.first().locator('a').click();
	await expect(page.locator('.modal.show')).toHaveCount(0);
	// the icon's url is looked up after the pick
	const icon = defaultSockets(page).nth(1).locator('.gem-icon');
	await expect(icon).toHaveAttribute('src', /\/icons\/large\/\w+\.jpg$/);
	const src = await icon.getAttribute('src');

	// opens again, still listing gems
	await defaultSockets(page).nth(1).click();
	await expect(modal(page).locator('li.selector-modal-list-item').first()).toBeVisible();
	await closePicker(page);

	await reloadBulk(page);
	await expect(defaultSockets(page).nth(1).locator('.gem-icon')).toHaveAttribute('src', src!);
	// the sockets left empty show just the socket, not a broken gem image
	for (const i of [0, 2, 3]) {
		await expect(defaultSockets(page).nth(i).locator('.gem-icon')).toBeHidden();
	}
	expect(errors).toEqual([]);
	expect(name).toBeTruthy();
});

test('turning Auto Enchant off leaves the Auto Gem defaults alone', async ({ page }) => {
	await openGear(page);
	await openBulk(page);
	const defaults = bulk(page).locator('.default-gem-container');
	await expect(defaults).toBeVisible();

	await setupBox(page, 'Auto Enchant').uncheck();
	await expect(defaults).toBeVisible();
	await setupBox(page, 'Auto Gem').uncheck();
	await expect(defaults).toBeHidden();
	await setupBox(page, 'Auto Enchant').check();
	await expect(defaults).toBeHidden();
	await setupBox(page, 'Auto Gem').check();
	await expect(defaults).toBeVisible();
});

test('the batch setup checkboxes keep what was ticked over a reload', async ({ page }) => {
	await openGear(page);
	await openBulk(page);
	const boxes = ['Fast Mode', 'Combinations', 'Auto Enchant', 'Auto Gem', 'Sim Talents'];
	const before: Record<string, boolean> = {};
	for (const name of boxes) before[name] = await setupBox(page, name).isChecked();

	const flipped: Record<string, boolean> = {};
	for (const name of boxes) {
		flipped[name] = !before[name];
		await setupBox(page, name).setChecked(flipped[name]);
	}

	await reloadBulk(page);
	for (const name of boxes) await expect(setupBox(page, name), name).toBeChecked({ checked: flipped[name] });
	await expect(bulk(page).locator('.default-gem-container')).toBeVisible({ visible: flipped['Auto Gem'] });
	await expect(bulk(page).locator('.talents-picker-container')).toBeVisible({ visible: flipped['Sim Talents'] });
});

test('talents saved on the talents tab can be picked for the batch, and the pick survives a reload', async ({ page }) => {
	page.on('dialog', dialog => dialog.accept());
	await openGear(page);
	await openBulk(page);
	await setupBox(page, 'Sim Talents').check();

	await openSimTab(page, 'talents-tab');
	const saved = page.locator('#talents-tab .saved-data-manager-root');
	for (const name of ['Mine', 'Other']) {
		await saved.locator('.saved-data-save-input').fill(name);
		await saved.locator('.saved-data-save-button').click();
	}
	await openSimTab(page, 'bulk-tab');
	const chip = (name: string) => bulk(page).locator('.talents-picker-container .saved-data-set-chip', { hasText: name });
	await expect(chip('Mine')).toBeVisible();
	await expect(chip('Other')).toBeVisible();

	await chip('Mine').locator('a').click();
	await expect(chip('Mine')).toHaveClass(/active/);
	await expect(chip('Other')).not.toHaveClass(/active/);

	await reloadBulk(page);
	await expect(chip('Mine')).toHaveClass(/active/);
	await expect(chip('Other')).not.toHaveClass(/active/);

	await openSimTab(page, 'talents-tab');
	await saved.locator('.saved-data-set-chip', { hasText: 'Mine' }).locator('.saved-data-set-delete').click();
	await openSimTab(page, 'bulk-tab');
	await expect(chip('Mine')).toHaveCount(0);
	await expect(chip('Other')).toBeVisible();

	// the deleted pick went with it, so a new loadout under the same name starts unpicked
	await openSimTab(page, 'talents-tab');
	await saved.locator('.saved-data-save-input').fill('Mine');
	await saved.locator('.saved-data-save-button').click();
	await openSimTab(page, 'bulk-tab');
	await expect(chip('Mine')).toBeVisible();
	await expect(chip('Mine')).not.toHaveClass(/active/);
});

test("the spec's preset talents can be picked for the batch too, and the batch sims them", async ({ page }) => {
	test.setTimeout(90_000);
	await openGear(page);
	await setIterations(page, 50);
	await openBulk(page);
	await setupBox(page, 'Sim Talents').check();

	const presets = '#talents-tab .saved-data-presets .saved-data-set-chip';
	const names = await page.locator(`${presets} .saved-data-set-name`).allTextContents();
	expect(names.length).toBeGreaterThan(1);
	const chip = (name: string) =>
		bulk(page).locator('.talents-picker-container .saved-data-set-chip', { has: page.locator(`.saved-data-set-name:text-is(${JSON.stringify(name)})`) });
	for (const name of names) await expect(chip(name)).toBeVisible();

	// the batch skips a loadout the same as the worn talents
	const pick = (await page.locator(`${presets}:not(.active) .saved-data-set-name`).first().textContent())!;
	expect(await page.locator(`${presets}.active`).count(), 'a preset is worn').toBe(1);
	await chip(pick).locator('a').click();
	await expect(chip(pick)).toHaveClass(/active/);
	await reloadBulk(page);
	await expect(chip(pick)).toHaveClass(/active/);

	await addBySearch(page, 'Penumbra');
	await bulk(page).locator('button', { hasText: 'Simulate Batch' }).click();
	await expect(bulk(page).locator('.bulk-results')).toBeVisible({ timeout: 60_000 });
	await expect(bulk(page).locator('.bulk-results .talent-loadout-text', { hasText: `Talent loadout used: ${pick}` }).first()).toBeVisible();
});
