import { expect, type Page, test } from '@playwright/test';

import { Fixture, label, loadFixture } from '../lib/fixture';
import { openSim, openSimTab, watchForErrors } from '../lib/page';
import { openTab } from './helpers';

// Real sims, kept small: one spec, a handful of iterations.
const SPEC = 'mage';
const RESULTS_TAB = 'detailed-results-tab-tab';

const sidebar = (page: Page) => page.locator('.sim-sidebar-results');
const sidebarDps = (page: Page) => sidebar(page).locator('.results-sim-dps .topline-result-avg');
const embedded = (page: Page) => page.locator(`#${RESULTS_TAB}`);
const embeddedDps = (page: Page) => embedded(page).locator('.damage-content .results-sim-dps .topline-result-avg');

const setIterations = async (page: Page, iterations: number) => {
	const input = page.locator('.iterations-picker input');
	await input.fill(String(iterations));
	await input.dispatchEvent('change');
};

// The sidebar shows progress where the result goes, and a quick run can finish before a test sees
// it, so mark the old result and wait for one without the mark.
const simulate = async (page: Page) => {
	await sidebar(page).evaluate(elem => elem.querySelector('.results-sim-reference')?.setAttribute('data-stale', ''));
	await page.locator('.dps-action').click();
	await expect(sidebar(page).locator('.results-sim-reference:not([data-stale])')).toHaveCount(1);
	return sidebarDps(page).innerText();
};

const spellRows = (root: ReturnType<Page['locator']>) => root.locator('.spell-metrics-root tbody tr').evaluateAll(rows => rows.map(row => row.textContent));

test('Simulate fills the results tab, and the separate tab gets the same run', async ({ page, context }) => {
	const errors = watchForErrors(page);
	await openSim(page, SPEC);
	await setIterations(page, 20);
	const dps = await simulate(page);

	await openSimTab(page, RESULTS_TAB);
	await expect(embeddedDps(page)).toHaveText(dps);
	const rows = await spellRows(embedded(page));
	expect(rows.length).toBeGreaterThan(0);

	const [tab] = await Promise.all([context.waitForEvent('page'), embedded(page).locator('.detailed-results-new-tab-button').click()]);
	await tab.locator('.dr-root:not(.dr-no-results)').waitFor();
	await expect(tab.locator('body')).toHaveClass(/individual-sim/);
	await expect(tab.locator('.player-filter-root')).toBeHidden();
	await expect(tab.locator('.damage-content .results-sim-dps .topline-result-avg')).toHaveText(dps);
	expect(await spellRows(tab.locator('body'))).toEqual(rows);

	// A later run reaches the tab that's already open.
	await setIterations(page, 10);
	const next = await simulate(page);
	await expect(tab.locator('.damage-content .results-sim-dps .topline-result-avg')).toHaveText(next);
	expect(errors).toEqual([]);
});

test('Save as Reference compares the next run against it, and Swap and Cancel follow', async ({ page }) => {
	await openSim(page, SPEC);
	await setIterations(page, 20);
	const reference = await simulate(page);
	await sidebar(page).locator('.results-sim-set-reference').click();
	await expect(sidebar(page).locator('.results-sim-reference')).toHaveClass(/has-reference/);

	await setIterations(page, 10);
	const current = await simulate(page);
	const diff = sidebar(page).locator('.results-sim-dps .results-reference-diff');
	await expect(sidebar(page).locator('.results-sim-dps .results-reference')).toBeVisible();
	// The page subtracts the unrounded numbers, so allow for the rounding of the two shown.
	expect(Math.abs(parseFloat(await diff.innerText()) - (parseFloat(current) - parseFloat(reference)))).toBeLessThanOrEqual(0.011);

	await openSimTab(page, RESULTS_TAB);
	await expect(embeddedDps(page)).toHaveText(current);

	await sidebar(page).locator('.results-sim-reference-swap').click();
	await expect(sidebarDps(page)).toHaveText(reference);
	await expect(embeddedDps(page)).toHaveText(reference);
	expect(Math.abs(parseFloat(await diff.innerText()) - (parseFloat(reference) - parseFloat(current)))).toBeLessThanOrEqual(0.011);

	await sidebar(page).locator('.results-sim-reference-delete').click();
	await expect(sidebar(page).locator('.results-sim-reference')).not.toHaveClass(/has-reference/);
	await expect(sidebar(page).locator('.results-sim-dps .results-reference')).toBeHidden();
	await expect(sidebarDps(page)).toHaveText(reference);
});

test('Sim 1 Iteration fills the log', async ({ page }) => {
	await openSim(page, SPEC);
	await openSimTab(page, RESULTS_TAB);
	await embedded(page).locator('.detailed-results-1-iteration-button').click();

	await expect(embedded(page).locator('.dr-root')).not.toHaveClass(/dr-no-results/);
	await openTab(page, 'logTab');
	await expect(embedded(page).locator('.log-runner-logs tr').first()).toBeVisible();
});

test.describe('a run posted with a reference', () => {
	let current: Fixture;
	let reference: Fixture;

	test.beforeAll(() => {
		current = loadFixture('raid25');
		reference = loadFixture('results-multitarget');
	});

	test('shows the current run, not the reference', async ({ page }) => {
		await page.goto('/wotlk/detailed_results/index.html');
		await page.evaluate(data => window.postMessage({ runData: data }, '*'), { run: current.run, referenceRun: reference.run });
		await page.locator('.dr-root:not(.dr-no-results)').waitFor();

		const options = await page.locator('.player-filter-root .dropdown-picker-item button').allInnerTexts();
		expect(options).toEqual(['All Players', ...current.players.map(label)]);
		const raidDps = Number(current.run.result.raidMetrics.dps.avg);
		await expect(page.locator('.damage-content .results-sim-dps .topline-result-avg')).toHaveText(raidDps.toFixed(2));
	});
});
