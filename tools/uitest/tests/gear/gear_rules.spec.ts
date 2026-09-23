import { expect, test } from '@playwright/test';

import { closePicker, equippedGems, equippedId, equipRow, loadDb, openGear, openPicker, pane, rowIds, showTab } from './gear';

test('a unique item worn in the other ring or trinket slot leaves the first one', async ({ page }) => {
	await openGear(page);
	const db = await loadDb(page);

	for (const [from, to] of [
		['finger1', 'finger2'],
		['trinket1', 'trinket2'],
	] as const) {
		const id = await equippedId(page, from);
		expect(db.items.get(id)?.unique, `${from} holds a unique item`).toBe(true);
		const items = pane(await openPicker(page, to), 'Items');
		await equipRow(items, id);
		await closePicker(page);
		await expect.poll(() => equippedId(page, to)).toBe(id);
		await expect.poll(() => equippedId(page, from)).toBe(0);
	}
});

test('a unique gem socketed elsewhere comes out of the socket it was in', async ({ page }) => {
	await openGear(page);
	const db = await loadDb(page);
	const headGems = await equippedGems(page, 'head');
	const unique = headGems.find(id => db.gems.get(id)?.unique);
	test.skip(!unique, 'no unique gem in the preset helm');

	const chest = db.items.get(await equippedId(page, 'chest'))!;
	expect(chest.gemSockets?.length).toBeGreaterThan(0);
	const tab = await showTab(await openPicker(page, 'chest'), 'Gem1');
	await tab.locator('.selector-modal-show-matching-gems input').setChecked(false);
	await equipRow(tab, unique!);
	await closePicker(page);

	await expect.poll(async () => (await equippedGems(page, 'chest'))[0]).toBe(unique);
	await expect.poll(async () => (await equippedGems(page, 'head')).includes(unique!)).toBe(false);
});

test('a two-hander in the main hand takes the off hand off, unless the spec can dual wield them', async ({ page }) => {
	await openGear(page, 'deathknight');
	const db = await loadDb(page);
	const offHand = await equippedId(page, 'offHand');
	test.skip(!offHand || db.items.get(await equippedId(page, 'mainHand'))?.handType == 4, 'the preset is not dual wielding');

	const items = pane(await openPicker(page, 'mainHand'), 'Items');
	await items.locator('.selector-modal-show-2h-weapons input').setChecked(true);
	const twoHander = (await rowIds(items)).find(id => db.items.get(id)?.handType == 4)!;
	await equipRow(items, twoHander);
	await closePicker(page);
	await expect.poll(() => equippedId(page, 'mainHand')).toBe(twoHander);
	await expect.poll(() => equippedId(page, 'offHand')).toBe(0);

	// putting a one-hander back in the off hand drops the two-hander instead
	const off = pane(await openPicker(page, 'offHand'), 'Items');
	await equipRow(off, offHand);
	await closePicker(page);
	await expect.poll(() => equippedId(page, 'offHand')).toBe(offHand);
	await expect.poll(() => equippedId(page, 'mainHand')).toBe(0);
});

test("titan's grip keeps two two-handers", async ({ page }) => {
	await openGear(page, 'warrior');
	const db = await loadDb(page);
	const mh = await equippedId(page, 'mainHand');
	const oh = await equippedId(page, 'offHand');
	test.skip(db.items.get(mh)?.handType != 4 || db.items.get(oh)?.handType != 4, 'the preset warrior does not wield two two-handers');

	const items = pane(await openPicker(page, 'mainHand'), 'Items');
	const other = (await rowIds(items)).find(id => id != mh && id != oh && db.items.get(id)?.handType == 4)!;
	await equipRow(items, other);
	await closePicker(page);
	await expect.poll(() => equippedId(page, 'mainHand')).toBe(other);
	expect(await equippedId(page, 'offHand')).toBe(oh);
});

test('a new helm keeps the gems that still fit and the enchant', async ({ page }) => {
	await openGear(page);
	const db = await loadDb(page);
	const [meta] = await equippedGems(page, 'head');
	const tooltip = async () => (await page.locator('#gear-tab .item-picker-root').first().locator('.item-picker-name').getAttribute('data-wowhead')) ?? '';
	const enchant = /ench=(\d+)/.exec(await tooltip())?.[1];
	expect(enchant, 'the preset helm is enchanted').toBeTruthy();

	const worn = await equippedId(page, 'head');
	const items = pane(await openPicker(page, 'head'), 'Items');
	const other = (await rowIds(items)).find(id => id != worn && db.items.get(id)?.gemSockets?.includes(1))!;
	await equipRow(items, other);
	await closePicker(page);

	await expect.poll(() => equippedId(page, 'head')).toBe(other);
	const metaIdx = db.items.get(other)!.gemSockets!.indexOf(1);
	expect((await equippedGems(page, 'head'))[metaIdx]).toBe(meta);
	expect(await tooltip()).toContain(`ench=${enchant}`);
});
