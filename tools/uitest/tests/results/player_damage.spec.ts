import { expect, test } from '@playwright/test';

import { Fixture, loadFixture, showFixture } from '../lib/fixture';

let fixture: Fixture;

test.beforeAll(() => {
	fixture = loadFixture();
});

test('the damage table lists every raider, highest first', async ({ page }) => {
	await showFixture(page, fixture);

	const rows = page.locator('.player-damage-metrics-root .player-damage-row');
	await expect(rows).toHaveCount(fixture.players.length);

	const shown = await page.locator('.player-damage-metrics-root .player-damage-row td:last-child').allInnerTexts();
	const expected = [...fixture.players].sort((a, b) => b.dps - a.dps).map(player => player.dps.toFixed(1));
	expect(shown).toEqual(expected);
});

test('hovering a row puts its chart in the tooltip, not in the row', async ({ page }) => {
	await showFixture(page, fixture);

	const row = page.locator('.player-damage-metrics-root .player-damage-row').first();
	const before = (await row.boundingBox())!.height;

	await row.hover();
	await expect(page.locator('.tippy-content canvas')).toBeVisible();

	expect(await row.locator('canvas').count()).toBe(0);
	expect((await row.boundingBox())!.height).toBe(before);
});
