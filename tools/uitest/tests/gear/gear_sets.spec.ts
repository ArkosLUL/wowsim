import { expect, type Page, test } from '@playwright/test';

import { openSimTab } from '../lib/page';
import { closePicker, equippedId, openGear, openPicker, pane, picker, readStats, SLOTS, statsAfterChange } from './gear';

const sets = (page: Page) => page.locator('#gear-tab .saved-data-manager-root');
const chip = (page: Page, name: string) =>
	sets(page).locator('.saved-data-set-chip', { has: page.locator('.saved-data-set-name', { hasText: new RegExp(`^${name}$`) }) });

async function wornIds(page: Page): Promise<number[]> {
	return Promise.all(SLOTS.map(slot => equippedId(page, slot)));
}

// Every test here keeps a helm on, and its link fills in once the gear has loaded.
async function reload(page: Page) {
	await page.reload();
	await page.waitForLoadState('networkidle');
	await openSimTab(page, 'gear-tab');
	await expect.poll(() => equippedId(page, 'head')).toBeGreaterThan(0);
}

async function unequip(page: Page, slot: (typeof SLOTS)[number]) {
	const dialog = await openPicker(page, slot);
	await pane(dialog, 'Items').locator('.selector-modal-remove-button').click();
	await closePicker(page);
	await expect.poll(() => equippedId(page, slot)).toBe(0);
}

test('a saved gear set loads back what was worn, and it and its deletion survive a reload', async ({ page }) => {
	page.on('dialog', dialog => dialog.accept());
	await openGear(page);
	const worn = await wornIds(page);
	const stats = await readStats(page);

	await sets(page).locator('.saved-data-save-input').fill('Test Set');
	await sets(page).locator('.saved-data-save-button').click();
	await expect(chip(page, 'Test Set')).toHaveClass(/active/);

	await unequip(page, 'neck');
	await expect(chip(page, 'Test Set')).not.toHaveClass(/active/);
	await statsAfterChange(page, stats);

	await reload(page);
	await expect(chip(page, 'Test Set')).toHaveCount(1);
	await expect.poll(() => equippedId(page, 'neck')).toBe(0);

	await chip(page, 'Test Set').locator('.saved-data-set-name').click();
	await expect.poll(() => wornIds(page)).toEqual(worn);
	await expect.poll(() => readStats(page)).toEqual(stats);
	await expect(chip(page, 'Test Set')).toHaveClass(/active/);

	await chip(page, 'Test Set').locator('.saved-data-set-delete').click();
	await expect(chip(page, 'Test Set')).toHaveCount(0);
	await reload(page);
	await expect(chip(page, 'Test Set')).toHaveCount(0);
	await expect.poll(() => wornIds(page)).toEqual(worn);
});

test('saving under a name already used replaces that set', async ({ page }) => {
	await openGear(page);
	const save = async () => {
		await sets(page).locator('.saved-data-save-input').fill('Swap');
		await sets(page).locator('.saved-data-save-button').click();
	};
	await save();
	await unequip(page, 'neck');
	await save();
	await expect(chip(page, 'Swap')).toHaveCount(1);
	await expect(chip(page, 'Swap')).toHaveClass(/active/);

	await reload(page);
	const neckless = await wornIds(page);
	await chip(page, 'Swap').locator('.saved-data-set-name').click();
	await expect.poll(() => wornIds(page)).toEqual(neckless);
	expect(neckless[SLOTS.indexOf('neck')]).toBe(0);
});

test('a preset gear set loads its items, and the loaded preset shows as active', async ({ page }) => {
	await openGear(page);
	const presets = sets(page).locator('.saved-data-presets .saved-data-set-chip');
	expect(await presets.count()).toBeGreaterThan(1);

	const active = await presets.evaluateAll(chips => chips.findIndex(c => c.classList.contains('active')));
	const other = active == 0 ? 1 : 0;
	const before = await wornIds(page);
	await presets.nth(other).locator('.saved-data-set-name').click();
	await expect(presets.nth(other)).toHaveClass(/active/);
	await expect.poll(async () => JSON.stringify(await wornIds(page)) != JSON.stringify(before)).toBe(true);
	if (active >= 0) await expect(presets.nth(active)).not.toHaveClass(/active/);

	const loaded = await wornIds(page);
	await reload(page);
	await expect.poll(() => wornIds(page)).toEqual(loaded);
	await expect(presets.nth(other)).toHaveClass(/active/);
	await expect(picker(page, 'head').locator('.item-picker-name')).not.toHaveText('Head');
});
