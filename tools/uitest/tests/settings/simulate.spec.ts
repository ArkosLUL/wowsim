import { expect, test, type Page } from '@playwright/test';

import { checkbox, numberInput, openSpec, reloadSim, setNumber } from './helpers';

const SPEC = 'mage';
// Real sims run here, so keep them small.
const ITERATIONS = 50;

async function openSmallSim(page: Page) {
	await openSpec(page, SPEC);
	await setNumber(numberInput(page.locator('.sim-sidebar'), 'Iterations'), ITERATIONS);
}

async function openStatWeights(page: Page) {
	await page.locator('.sim-sidebar-actions button', { hasText: 'Stat Weights' }).click();
	const modal = page.locator('.modal.show:has(.ep-weights-menu)');
	await expect(modal).toBeVisible();
	return modal;
}

test('a small simulation shows its DPS', async ({ page }) => {
	await openSmallSim(page);

	await page.locator('.sim-sidebar-actions button', { hasText: 'Simulate' }).click();

	const dps = page.locator('.sim-sidebar-results .results-sim-dps .topline-result-avg');
	await expect.poll(async () => Number(await dps.innerText()), { timeout: 60_000 }).toBeGreaterThan(0);
	await expect(page.locator('.sim-sidebar-results')).not.toContainText('iterations complete');
});

test('a fixed RNG seed gives the same DPS every run, and shows as the last seed used', async ({ page }) => {
	await openSmallSim(page);
	await page.locator('.sim-toolbar .sim-options').click();
	const options = page.locator('.modal.show:has(.settings-menu)');
	await setNumber(numberInput(options, 'Fixed RNG Seed'), 4242);
	await options.locator('.close-button').click();
	await expect(page.locator('.modal.show')).toHaveCount(0);

	const dps = page.locator('.sim-sidebar-results .results-sim-dps .topline-result-avg');
	const runs: string[] = [];
	for (let i = 0; i < 2; i++) {
		await page.locator('.sim-sidebar-actions button', { hasText: 'Simulate' }).click();
		await expect(page.locator('.sim-sidebar-results')).toContainText('iterations complete');
		await expect(page.locator('.sim-sidebar-results')).not.toContainText('iterations complete', { timeout: 60_000 });
		runs.push(await dps.innerText());
	}
	expect(runs[1]).toBe(runs[0]);

	await page.locator('.sim-toolbar .sim-options').click();
	await expect(page.locator('.modal.show .last-used-rng-seed')).toHaveText('4242');
});

test('stat weights fill the table, and copying them sets the current EP for good', async ({ page }) => {
	await openSmallSim(page);
	const modal = await openStatWeights(page);
	const rows = modal.locator('.results-ep-table tbody tr');
	const shownEp = rows.locator('td.damage-metrics.type-ep .results-avg');
	const currentEp = rows.locator('td.current-ep input');
	await expect(shownEp.first()).toHaveText('N/A');
	const defaults = await currentEp.evaluateAll(inputs => inputs.map(i => (i as HTMLInputElement).value));

	await modal.locator('.calc-weights').click();
	await expect(shownEp.first()).toHaveText(/^-?\d+\.\d\d$/, { timeout: 60_000 });
	// The reference stat is the one everything else is measured against.
	await expect(shownEp.filter({ hasText: /^1\.00$/ })).not.toHaveCount(0);

	await modal.locator('th.damage-metrics.type-ep .col-action').click();
	const calculated = await shownEp.allInnerTexts();
	await expect.poll(async () => currentEp.evaluateAll(inputs => inputs.map(i => (i as HTMLInputElement).value))).toEqual(calculated);

	await reloadSim(page);
	const reopened = await openStatWeights(page);
	const reopenedEp = reopened.locator('.results-ep-table tbody tr td.current-ep input');
	expect(await reopenedEp.evaluateAll(inputs => inputs.map(i => (i as HTMLInputElement).value))).toEqual(calculated);

	// The Current EP header's button puts the spec's defaults back.
	await reopened.locator('thead th:has-text("Current EP") .col-action').click();
	await expect.poll(() => reopenedEp.evaluateAll(inputs => inputs.map(i => (i as HTMLInputElement).value))).toEqual(defaults);
});

test('Show All Stats lists every stat, weapon DPS included', async ({ page }) => {
	await openSpec(page, SPEC);
	const modal = await openStatWeights(page);
	const names = modal.locator('.results-ep-table tbody tr td:first-child');
	const shown = await names.count();

	await checkbox(modal, 'Show All Stats').setChecked(true);

	await expect.poll(() => names.count()).toBeGreaterThan(shown);
	for (const name of ['Main Hand DPS', 'Off Hand DPS', 'Ranged DPS']) {
		await expect(names.filter({ hasText: name })).toHaveCount(1);
	}
});
