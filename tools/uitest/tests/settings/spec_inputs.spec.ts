import { expect, test } from '@playwright/test';

import { openSimTab, SPECS, watchForErrors } from '../lib/page';
import { changeEverything, numberInput, openSpec, pickFromDropdown, reloadSim, select, setNumber, snapshot } from './helpers';

for (const spec of SPECS) {
	test(`the ${spec} player and spec settings survive a reload`, async ({ page }) => {
		const errors = watchForErrors(page);
		await openSpec(page, spec, 'settings-tab');
		const sections = page.locator('#settings-tab').locator('.player-settings, .other-settings, .custom-section');

		await changeEverything(sections);
		const before = await snapshot(page.locator('#settings-tab'));

		await reloadSim(page);
		await openSimTab(page, 'settings-tab');
		expect(await snapshot(page.locator('#settings-tab'))).toEqual(before);
		expect(errors).toEqual([]);
	});
}

// Only specs with a simple rotation show these inputs, under the Simple rotation type.
for (const spec of ['feral_druid', 'feral_tank_druid', 'hunter', 'mage']) {
	test(`the ${spec} simple rotation settings survive a reload`, async ({ page }) => {
		const errors = watchForErrors(page);
		await openSpec(page, spec, 'rotation-tab');
		const tab = page.locator('#rotation-tab');
		await tab.locator('.rotation-tab-header select').selectOption({ label: 'Simple' });
		await expect(tab.locator('.rotation-settings')).toBeVisible();

		await changeEverything(tab.locator('.rotation-settings'));
		const before = await snapshot(tab.locator('.rotation-settings'));

		await reloadSim(page);
		await openSimTab(page, 'rotation-tab');
		expect(await snapshot(tab.locator('.rotation-settings'))).toEqual(before);
		expect(errors).toEqual([]);
	});
}

test('a warrior with Blood Elf racial traits can cast Arcane Torrent, and loads cleanly', async ({ page }) => {
	const errors = watchForErrors(page);
	await openSpec(page, 'warrior', 'settings-tab');

	const statsUpdated = page.waitForResponse(response => response.url().endsWith('/computeStats'));
	await select(page.locator('#settings-tab .player-settings'), 'Racial Traits').selectOption({ label: 'Blood Elf' });
	await statsUpdated;

	await openSimTab(page, 'rotation-tab');
	const tab = page.locator('#rotation-tab');
	await select(tab.locator('.rotation-tab-header'), 'Rotation Type').selectOption({ label: 'APL' });
	await tab.locator('.apl-list-item-picker > .list-picker-new-button').click();
	const added = tab.locator('.apl-list-item-picker > .list-picker-items > .list-picker-item-container').last();
	await pickFromDropdown(added.locator('.apl-action-picker-action > .dropdown-picker-root').first(), 'Cast');
	// the list names a spell once its tooltip is in, which is also when a failed one logs its error
	await pickFromDropdown(added.locator('.apl-action-castSpell > .dropdown-picker-root').first(), 'Cooldowns', 'Arcane Torrent');
	await expect(added.locator('.apl-action-picker-action').first()).toContainText('Arcane Torrent');
	expect(errors).toEqual([]);
});

test('a percent typed into a spec input reads the same after a reload', async ({ page }) => {
	await openSpec(page, 'hunter', 'settings-tab');
	const uptime = numberInput(page.locator('#settings-tab'), 'Pet Uptime (%)');

	await setNumber(uptime, 57);
	await reloadSim(page);

	await expect(uptime).toHaveValue('57');
});
