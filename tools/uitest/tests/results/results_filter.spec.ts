import { expect, type Page, test } from '@playwright/test';

import { Fixture, label, loadFixture, showFixture } from '../lib/fixture';
import { pickPlayer, topline } from './helpers';

let fixture: Fixture;

test.beforeAll(() => {
	fixture = loadFixture();
});

const selectedPlayer = (page: Page) => page.locator('.player-filter-root .dropdown-picker-button');
const toplineDps = (page: Page) => topline(page, 'damage-content', 'dps');

test('the player dropdown offers every raider once', async ({ page }) => {
	await showFixture(page, fixture);

	const options = await page.locator('.player-filter-root .dropdown-picker-item button').allInnerTexts();
	expect(options).toEqual(['All Players', ...fixture.players.map(label)]);
});

test('picking a raider shows that raider, not their neighbour', async ({ page }) => {
	await showFixture(page, fixture);

	for (const player of fixture.players) {
		await pickPlayer(page, label(player));
		await expect(selectedPlayer(page)).toHaveText(label(player));
		await expect(toplineDps(page)).toHaveText(player.dps.toFixed(2));
	}
});

test('clicking a damage row names that raider in the dropdown', async ({ page }) => {
	await showFixture(page, fixture);

	// The rows are sorted by DPS, so the fixture's own order says nothing about which row is which.
	for (const player of fixture.players) {
		await page.locator('.player-damage-metrics-root .player-damage-row', { hasText: label(player) }).first().click();
		await expect(selectedPlayer(page)).toHaveText(label(player));
		await expect(toplineDps(page)).toHaveText(player.dps.toFixed(2));
		await pickPlayer(page, 'All Players');
	}
});

test('All Players goes back to the whole raid', async ({ page }) => {
	await showFixture(page, fixture);

	await pickPlayer(page, label(fixture.players[0]));
	await pickPlayer(page, 'All Players');

	await expect(page.locator('.player-damage-metrics-root .player-damage-row')).toHaveCount(fixture.players.length);
});
