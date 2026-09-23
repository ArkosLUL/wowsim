import { expect, type Page, test } from '@playwright/test';

import { openSimTab } from '../lib/page';
import { fixtureSettings, removePlayer, roster, seedRaid, slot } from './raid';

const selectedPlayer = (page: Page) => page.locator('.player-filter-root .dropdown-picker-button');
// the healing tab has its own topline
const toplineDps = (page: Page) => page.locator('.damage-content .results-sim-dps .topline-result-avg');

async function pickPlayer(page: Page, label: string) {
	await selectedPlayer(page).click();
	await page.locator('.player-filter-root .dropdown-picker-item button', { hasText: label }).first().click();
}

test('a live raid sim names the right raider in the results', async ({ page }) => {
	test.setTimeout(300_000);
	// a short fight: the Log tab draws every line of the first iteration, and a 3 minute one leaves the
	// page with over a million elements that every locator has to wade through
	const settings = fixtureSettings();
	settings.encounter.duration = 10;
	await seedRaid(page, settings);
	// empty seats pull the sim's own unit numbering away from the raid slots
	await removePlayer(page, 4);
	await removePlayer(page, 12);
	const names = await roster(page);
	const raiders = names.flatMap((name, raidIndex) => (name ? [{ name, raidIndex, label: `${name} (#${raidIndex + 1})` }] : []));
	// the first raider, the ones right after each empty seat, and the last
	const checked = [raiders[0], ...[5, 13].map(raidIndex => raiders.find(raider => raider.raidIndex == raidIndex)!), raiders[raiders.length - 1]];

	const iterations = page.locator('.iterations-picker input');
	await iterations.fill('20');
	await iterations.press('Enter');
	await page.locator('.dps-action').click();
	await expect(slot(page, raiders[0].raidIndex).locator('.player-results-dps')).toHaveText(/DPS$/, { timeout: 150_000 });

	// the Raid tab puts each raider's DPS in their own slot, so the results page has to agree with it
	const slotDps = await page
		.locator('.party-picker-root .player-picker-root')
		.evaluateAll(slots => slots.map(elem => parseFloat(elem.querySelector('.player-results-dps')?.textContent ?? '')));

	await openSimTab(page, 'detailed-results-tab-tab');
	const options = await page.locator('.player-filter-root .dropdown-picker-item button').allInnerTexts();
	expect(options).toEqual(['All Players', ...raiders.map(raider => raider.label)]);

	for (const raider of checked) {
		await pickPlayer(page, raider.label);
		await expect(selectedPlayer(page)).toHaveText(raider.label);
		const dps = parseFloat(await toplineDps(page).innerText());
		expect(Math.abs(dps - slotDps[raider.raidIndex]), raider.label).toBeLessThanOrEqual(0.051);
	}

	await pickPlayer(page, 'All Players');
	for (const raider of checked) {
		await page.locator('.player-damage-metrics-root .player-damage-row', { hasText: raider.label }).first().click();
		await expect(selectedPlayer(page)).toHaveText(raider.label);
		await pickPlayer(page, 'All Players');
	}
});
