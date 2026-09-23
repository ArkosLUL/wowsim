import { expect, type Page, test } from '@playwright/test';

import { openRaid, watchForErrors } from '../lib/page';
import { addPreset, copyPlayer, fixtureRoster, reloadRaid, removePlayer, renamePlayer, roster, seedRaid, slot, statCount, swapPlayers } from './raid';

const raidSize = (page: Page) => page.locator('.raid-controls .enum-picker-root', { hasText: 'Raid Size' }).locator('select');
const defaultGear = (page: Page) => page.locator('.raid-controls .enum-picker-root', { hasText: 'Default Gear' }).locator('select');
const raiderCount = async (page: Page) => {
	const roles = await Promise.all(['Tanks', 'Healers', 'Melee', 'Ranged'].map(role => statCount(page, role)));
	return roles.reduce((a, b) => a + b);
};

test('every preset adds a raider', async ({ page }) => {
	const errors = watchForErrors(page);
	await openRaid(page);
	const presets = await page.locator('.new-player-picker-root a').evaluateAll(links => links.map(link => link.getAttribute('data-bs-title')!));

	for (let start = 0; start < presets.length; start += 40) {
		const batch = presets.slice(start, start + 40);
		await page.evaluate(() => localStorage.clear());
		await reloadRaid(page);
		await raidSize(page).selectOption({ label: '40' });
		for (const [i, preset] of batch.entries()) {
			await addPreset(page, preset, i);
		}
		const names = await roster(page, 40);
		expect(names.slice(0, batch.length).filter(name => name == '')).toEqual([]);
		expect(await raiderCount(page)).toBe(batch.length);
	}
	expect(errors).toEqual([]);
});

test('a 40-player raid with a Demonology warlock gets its character stats', async ({ page }) => {
	await openRaid(page);
	await raidSize(page).selectOption({ label: '40' });
	await addPreset(page, 'Demonology Warlock', 0);

	const answered: Array<Promise<boolean>> = [];
	page.on('request', request => {
		if (request.url().endsWith('/computeStats')) answered.push(request.response().then(response => response?.ok() ?? false));
	});
	await addPreset(page, 'Fury Warrior', 30);
	await expect.poll(() => answered.length).toBeGreaterThan(0);
	expect(await Promise.all(answered)).not.toContain(false);
});

test('swapping two raiders across groups sticks', async ({ page }) => {
	await seedRaid(page);
	const expected = fixtureRoster();
	[expected[0], expected[7]] = [expected[7], expected[0]];

	await swapPlayers(page, 0, 7);
	await expect.poll(() => roster(page)).toEqual(expected);

	await reloadRaid(page);
	await expect.poll(() => roster(page)).toEqual(expected);
});

test('a raider dragged onto an empty slot moves there', async ({ page }) => {
	await seedRaid(page);
	const expected = fixtureRoster();

	await removePlayer(page, 24);
	expected[24] = '';
	await expect.poll(() => roster(page)).toEqual(expected);

	await swapPlayers(page, 3, 24);
	[expected[3], expected[24]] = ['', expected[3]];
	await expect.poll(() => roster(page)).toEqual(expected);

	await reloadRaid(page);
	await expect.poll(() => roster(page)).toEqual(expected);
});

test('a copied raider fills the slot it is dropped on', async ({ page }) => {
	await seedRaid(page);
	const expected = fixtureRoster();

	await copyPlayer(page, 5, 24);
	expected[24] = expected[5];
	await expect.poll(() => roster(page)).toEqual(expected);

	// the copy is its own raider: renaming it leaves the original alone
	await renamePlayer(page, 24, 'Copied');
	expected[24] = 'Copied';
	await reloadRaid(page);
	await expect.poll(() => roster(page)).toEqual(expected);
});

test('dragging a group onto another swaps the two groups', async ({ page }) => {
	await seedRaid(page);
	const names = fixtureRoster();
	const expected = [...names.slice(5, 10), ...names.slice(0, 5), ...names.slice(10)];

	await page.locator('.party-picker-root').nth(0).locator('.party-header').dragTo(page.locator('.party-picker-root').nth(1).locator('.party-header'));
	await expect.poll(() => roster(page)).toEqual(expected);

	await reloadRaid(page);
	await expect.poll(() => roster(page)).toEqual(expected);
});

test('renaming a raider sticks, and an empty name becomes Unnamed', async ({ page }) => {
	await seedRaid(page);
	const expected = fixtureRoster();

	await renamePlayer(page, 3, 'Newname');
	await renamePlayer(page, 8, '');
	expected[3] = 'Newname';
	expected[8] = 'Unnamed';
	await expect.poll(() => roster(page)).toEqual(expected);

	await reloadRaid(page);
	await expect.poll(() => roster(page)).toEqual(expected);
});

test('editing a raider changes their gear for good', async ({ page }) => {
	await seedRaid(page);
	const editor = page.locator('.modal.show', { has: page.locator('.player-editor-modal') });
	const headName = editor.locator('#gear-tab .item-picker-root').first().locator('.item-picker-name');

	await slot(page, 1).locator('.player-edit').click();
	await expect(headName).not.toBeEmpty();
	const before = await headName.innerText();
	// try the other preset gear sets until one brings a different helm
	const presets = editor.locator('.saved-data-presets .saved-data-set-chip:not(.active)');
	let after = before;
	for (let i = 0; i < (await presets.count()) && after == before; i++) {
		await presets.nth(i).click();
		after = await headName.innerText();
	}
	expect(after).not.toBe(before);
	await editor.locator('.close-button').click();
	await expect(editor).toHaveCount(0);

	await reloadRaid(page);
	await slot(page, 1).locator('.player-edit').click();
	await expect(headName).toHaveText(after);
});

test('Default Gear names a phase, and a picked one sticks', async ({ page }) => {
	await openRaid(page);
	await expect(defaultGear(page).locator('option:checked')).toHaveText(/^Phase \d$/);

	await defaultGear(page).selectOption({ label: 'Phase 2' });
	await reloadRaid(page);
	await expect(defaultGear(page).locator('option:checked')).toHaveText('Phase 2');
});

test('Raid Size sets how many groups take part', async ({ page }) => {
	await seedRaid(page);
	await expect(page.locator('.party-picker-root.active')).toHaveCount(5);

	await raidSize(page).selectOption({ label: '10' });
	await expect(page.locator('.party-picker-root.active')).toHaveCount(2);
	const inFirstTwoGroups = fixtureRoster()
		.slice(0, 10)
		.filter(name => name != '');
	expect(await raiderCount(page)).toBe(inFirstTwoGroups.length);

	await reloadRaid(page);
	await expect(page.locator('.party-picker-root.active')).toHaveCount(2);
});
