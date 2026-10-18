import { expect, type Page, test } from '@playwright/test';

import { openSimTab, watchForErrors } from '../lib/page';
import { checkbox, openSpec, reloadSim, storedSettings, stubWowheadTooltips } from './helpers';

const SPEC = 'mage';

const playerbotBox = (page: Page) => checkbox(page.locator('#settings-tab .player-settings'), 'Playerbot');

test('the Playerbot checkbox starts off and survives a reload', async ({ page }) => {
	const errors = watchForErrors(page);
	await openSpec(page, SPEC, 'settings-tab');
	await expect(playerbotBox(page)).not.toBeChecked();

	await playerbotBox(page).setChecked(true);
	await reloadSim(page);
	await openSimTab(page, 'settings-tab');

	await expect(playerbotBox(page)).toBeChecked();
	expect((await storedSettings(page, SPEC)).player.playerbot).toBe(true);
	expect(errors).toEqual([]);
});

// The rankings' links carry the switch the same way.
test('a link with Playerbot on opens with the checkbox ticked', async ({ page, browser }) => {
	await openSpec(page, SPEC, 'settings-tab');
	await playerbotBox(page).setChecked(true);

	await page.locator('.export-dropdown .export-link').hover();
	await page.locator('.export-dropdown .dropdown-item:text-is("Link")').click();
	const link = await page.locator('.modal.show .exporter-textarea').inputValue();

	// a second profile, so nothing reaches it through localStorage
	const context = await browser.newContext({ viewport: { width: 1600, height: 1100 } });
	const other = await context.newPage();
	await stubWowheadTooltips(other);
	await other.goto(link);
	// only the hash differs from the page already open, which doesn't load it again
	await other.reload();
	await other.waitForLoadState('networkidle');
	await openSimTab(other, 'settings-tab');

	await expect(playerbotBox(other)).toBeChecked();
	await context.close();
});
