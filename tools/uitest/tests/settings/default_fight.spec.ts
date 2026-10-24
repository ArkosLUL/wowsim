import { expect, type Page, test } from '@playwright/test';

import { openRaid, openSimTab, SPECS } from '../lib/page';
import { numberInput, openSpec, reloadSim, setNumber } from './helpers';

const HEALERS = ['healing_priest', 'holy_paladin', 'restoration_druid', 'restoration_shaman'];

const duration = (page: Page) => numberInput(page.locator('#settings-tab .encounter-settings'), 'Duration');

test('healer pages default to a 300 s fight, every other page to 180 s', async ({ page }) => {
	test.setTimeout(180_000);
	for (const spec of SPECS) {
		await openSpec(page, spec, 'settings-tab');
		await expect(duration(page), spec).toHaveValue(HEALERS.includes(spec) ? '300' : '180');
	}
});

test('the raid sim defaults to a 180 s fight', async ({ page }) => {
	await openRaid(page);
	await openSimTab(page, 'raid-settings-tab');
	const raidDuration = page.locator('.encounter-settings .number-picker-root', { hasText: 'Duration' }).locator('input').first();
	await expect(raidDuration).toHaveValue('180');
});

test('a healer page keeps the fight it saved until Restore Defaults', async ({ page }) => {
	await openSpec(page, 'holy_paladin', 'settings-tab');
	await setNumber(duration(page), 180);

	await reloadSim(page);
	await openSimTab(page, 'settings-tab');
	await expect(duration(page)).toHaveValue('180');

	await page.locator('.sim-toolbar .sim-options').click();
	await page.locator('.modal.show .restore-defaults-button').click();
	await page.locator('.modal.show .close-button').click();
	await expect(duration(page)).toHaveValue('300');
});
