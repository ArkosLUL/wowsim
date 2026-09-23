import { expect, test } from '@playwright/test';

import { Fixture, loadFixture } from '../lib/fixture';
import { watchForErrors } from '../lib/page';
import { openTab, showIndividualFixture, topline } from './helpers';

let single: Fixture;

test.beforeAll(() => {
	single = loadFixture('results-single');
});

const TABS = ['damageTab', 'healingTab', 'damageTakenTab', 'buffsTab', 'debuffsTab', 'castsTab', 'resourcesTab', 'timelineTab', 'logTab'];

test('the page shows the one raider, with no player dropdown or raid table', async ({ page }) => {
	await showIndividualFixture(page, single);
	const [player] = single.players;

	await expect(page.locator('.player-filter-root')).toBeHidden();
	await expect(page.locator('.target-filter-root')).toBeVisible();
	await expect(page.locator('.player-damage-metrics-root')).toBeHidden();
	await expect(topline(page, 'damage-content', 'dps')).toHaveText(player.dps.toFixed(2));
	await expect(page.locator('.spell-metrics-root tbody tr').first()).toBeVisible();
	await expect(page.locator('.melee-metrics-root tbody tr').first()).toBeVisible();
});

test("the timeline opens on the raider's rotation", async ({ page }) => {
	await showIndividualFixture(page, single);
	await openTab(page, 'timelineTab');

	await expect(page.locator('.timeline-chart-picker')).toHaveValue('rotation');
	await expect(page.locator('.rotation-plot')).toBeVisible();
	await expect(page.locator('.rotation-labels .rotation-label').first()).toBeVisible();

	// A hunter has mana, so the chart has their DPS, mana and threat.
	await page.locator('.timeline-chart-picker').selectOption('dps');
	await expect(page.locator('.dps-resources-plot .apexcharts-series')).toHaveCount(3);
});

test('the log shows the whole fight', async ({ page }) => {
	await showIndividualFixture(page, single);
	await openTab(page, 'logTab');
	await expect(page.locator('#logTab .log-runner-logs tr').first()).toBeVisible();
});

test('every tab draws without an error', async ({ page }) => {
	const errors = watchForErrors(page);
	await showIndividualFixture(page, single);

	for (const tab of TABS) {
		await openTab(page, tab);
	}
	expect(errors).toEqual([]);
});
