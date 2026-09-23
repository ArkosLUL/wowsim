import { expect, test, type Page } from '@playwright/test';

import { openSimTab, watchForErrors } from '../lib/page';
import {
	GEM_COLOR,
	SLOTS,
	closePicker,
	equippedGems,
	equippedId,
	expectMovedBy,
	gemStats,
	loadDb,
	openGear,
	openPicker,
	pane,
	picker,
	plus,
	readStats,
	statsAfterChange,
	warnings,
} from './gear';

async function allGems(page: Page): Promise<number[][]> {
	return Promise.all(SLOTS.map(slot => equippedGems(page, slot)));
}

// Resolves once `read` has returned the same value for `quietMs`: Suggest Gems re-gems the set
// several times while it works and says nothing when it's done.
async function settled<T>(read: () => Promise<T>, quietMs = 1500): Promise<T> {
	let last = JSON.stringify(await read());
	let since = Date.now();
	await expect
		.poll(
			async () => {
				const now = JSON.stringify(await read());
				if (now != last) {
					last = now;
					since = Date.now();
				}
				return Date.now() - since >= quietMs;
			},
			{ timeout: 30_000 },
		)
		.toBe(true);
	return JSON.parse(last);
}

test('unequipping an item takes its gems, enchant and socket bonus off with it', async ({ page }) => {
	await openGear(page);
	const db = await loadDb(page);
	const item = db.items.get(await equippedId(page, 'chest'))!;
	const gems = await equippedGems(page, 'chest');
	const tooltip = (await picker(page, 'chest').locator('.item-picker-name').getAttribute('data-wowhead')) ?? '';
	const enchantId = Number(/ench=(\d+)/.exec(tooltip)?.[1] ?? 0);
	const enchant = db.enchants.find(e => e.effectId == enchantId);
	expect(enchant, 'the preset chest is enchanted').toBeTruthy();

	const worn = await readStats(page);
	const dialog = await openPicker(page, 'chest');
	await pane(dialog, 'Items').locator('.selector-modal-remove-button').click();
	await expect(dialog.locator('a[data-content-id="Enchants-tab"]')).toHaveCount(1);
	await expect(dialog.locator('a.selector-modal-tab-gem')).toHaveCount(0);
	await expect(dialog.locator('a[data-content-id="Reforging-tab"]')).toHaveCount(0);
	await closePicker(page);

	await expect(picker(page, 'chest').locator('.item-picker-name')).toHaveText('Chest');
	await expect(picker(page, 'chest').locator('.item-picker-enchant')).toHaveText('');
	await expect(picker(page, 'chest').locator('.gem-socket-container')).toHaveCount(0);
	await expect(picker(page, 'chest').locator('.item-picker-icon')).toHaveCSS('background-image', /item_slots\/chest\.jpg/);

	const bare = await statsAfterChange(page, worn);
	expect(await warnings(page)).not.toContain('Meta gem disabled');
	expectMovedBy(bare, worn, plus(plus(item.stats, gemStats(db, item, gems)), enchant!.stats));
});

test('Unequip All Gems empties every socket and the gem summary, and it stays that way after a reload', async ({ page }) => {
	await openGear(page);
	const summary = page.locator('#gear-tab .gem-summary-root');
	await expect(summary.locator('.gem-summary-link')).not.toHaveCount(0);
	const worn = await readStats(page);

	await summary.locator('.gem-reset-button').click();
	await expect.poll(async () => (await allGems(page)).flat().filter(id => id > 0)).toEqual([]);
	await expect(summary.locator('.gem-summary-link')).toHaveCount(0);
	await expect(page.locator('#gear-tab .gear-picker-root .gem-icon:not(.hide)')).toHaveCount(0);
	const bare = await statsAfterChange(page, worn);
	for (const stat of ['Strength', 'Agility', 'Stamina']) expect(bare[stat], stat).toBeLessThan(worn[stat]);

	await page.reload();
	await page.waitForLoadState('networkidle');
	await openSimTab(page, 'gear-tab');
	// the gear has to be drawn again first, or an empty page would pass too
	await expect.poll(() => equippedId(page, 'head')).toBeGreaterThan(0);
	await expect(page.locator('#gear-tab .gear-picker-root .gem-socket-container:not(.hide)')).not.toHaveCount(0);
	expect((await allGems(page)).flat().filter(id => id > 0)).toEqual([]);
	await expect(page.locator('#gear-tab .gear-picker-root .gem-icon:not(.hide)')).toHaveCount(0);
	await expect(summary.locator('.gem-summary-link')).toHaveCount(0);
});

// every spec with a Suggest Gems action
for (const spec of ['warrior', 'feral_druid', 'hunter', 'feral_tank_druid']) {
	test(`Suggest Gems fills every ${spec} socket, keeps the meta gem on and stays within three Dragon's Eyes`, async ({ page }) => {
		test.setTimeout(60_000);
		const errors = watchForErrors(page);
		await openGear(page, spec);
		const db = await loadDb(page);
		await page.locator('#gear-tab .gem-summary-root .gem-reset-button').click();
		await expect.poll(async () => (await allGems(page)).flat().filter(id => id > 0)).toEqual([]);

		await page.locator('.suggest-gems-action').click();
		const gems = await settled(() => allGems(page));

		for (const [i, slot] of SLOTS.entries()) {
			const id = await equippedId(page, slot);
			if (!id) continue;
			const item = db.items.get(id)!;
			// the belt buckle's socket comes on top of the item's own
			const sockets = (item.gemSockets?.length ?? 0) + (item.type == 8 ? 1 : 0);
			expect(gems[i].slice(0, sockets).filter(g => g > 0).length, `${slot} (${item.name})`).toBe(sockets);
		}
		const flat = gems
			.flat()
			.filter(id => id > 0)
			.map(id => db.gems.get(id)!);
		expect(flat.filter(g => g.color == GEM_COLOR.meta)).toHaveLength(1);
		expect(await warnings(page)).not.toContain('Meta gem disabled');
		expect(flat.filter(g => g.requiredProfession).length).toBeLessThanOrEqual(3);
		expect(errors).toEqual([]);
	});
}
