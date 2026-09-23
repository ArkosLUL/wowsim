import { expect, type Locator, type Page } from '@playwright/test';

import type { Fixture, FixturePlayer } from '../lib/fixture';

// showFixture for the page the individual sims open, which hides the raid-only parts.
export const showIndividualFixture = async (page: Page, fixture: Fixture) => {
	await page.goto('/wotlk/detailed_results/index.html?isIndividualSim');
	await page.evaluate(s => window.postMessage({ settings: s }, '*'), {
		showDamageMetrics: true,
		showThreatMetrics: true,
		showHealingMetrics: true,
		showExperimental: true,
	});
	await page.evaluate(run => window.postMessage({ runData: { run } }, '*'), fixture.run);
	await page.locator('.dr-root:not(.dr-no-results)').waitFor();
};

export const pickPlayer = async (page: Page, text: string) => {
	await page.locator('.player-filter-root .dropdown-picker-button').click();
	await page.locator('.player-filter-root .dropdown-picker-item button', { hasText: text }).first().click();
};

export const pickTarget = async (page: Page, text: string) => {
	await page.locator('.target-filter-root .dropdown-picker-button').click();
	await page.locator('.target-filter-root .dropdown-picker-item button', { hasText: text }).first().click();
};

export const openTab = async (page: Page, tab: string) => {
	await page.locator(`[data-bs-target="#${tab}"]`).click();
	await expect(page.locator(`#${tab}`)).toHaveClass(/show/);
};

// Each tab has its own topline, so the metric has to be looked up inside the tab that shows it.
export const topline = (page: Page, content: string, metric: string) => page.locator(`.${content} .results-sim-${metric} .topline-result-avg`);

export const rowsOf = (table: Locator) => table.locator('tbody tr');

export const columnTexts = async (table: Locator, column: number): Promise<string[]> =>
	table.locator('tbody tr').evaluateAll((rows, column) => rows.map(row => (row.children[column]?.textContent ?? '').trim()), column);

export const headerCell = (table: Locator, name: string) => table.locator('th', { has: table.page().locator(`span:text-is("${name}")`) });

// Tablesorter drops a click that lands while it's still applying the previous sort, so wait until
// the header says the sort took.
export const sortBy = async (table: Locator, name: string) => {
	const header = headerCell(table, name);
	const before = await header.getAttribute('aria-sort');
	await header.click();
	await expect(header).not.toHaveAttribute('aria-sort', before ?? 'none');
	return (await header.getAttribute('aria-sort')) as 'ascending' | 'descending';
};

const iterations = (fixture: Fixture) => Number(fixture.run.request?.simOptions?.iterations ?? 1);
const duration = (fixture: Fixture) => Number(fixture.run.result?.avgIterationDuration ?? 1);

const metricsFor = (fixture: Fixture, player: FixturePlayer) => {
	const party = Math.floor(player.raidIndex / 5);
	return fixture.run.result.raidMetrics.parties[party].players[player.raidIndex % 5];
};

const damageTo = (unit: any, unitIndex: number): number => {
	const own = (unit.actions ?? [])
		.flatMap((action: any) => action.targets ?? [])
		.filter((target: any) => (target.unitIndex ?? 0) === unitIndex)
		.reduce((total: number, target: any) => total + (target.damage ?? 0), 0);
	return own + (unit.pets ?? []).reduce((total: number, pet: any) => total + damageTo(pet, unitIndex), 0);
};

// A raider's DPS against one target, pets included, worked out the way the page does it.
export const dpsAgainst = (fixture: Fixture, player: FixturePlayer, targetUnitIndex: number) =>
	damageTo(metricsFor(fixture, player), targetUnitIndex) / iterations(fixture) / duration(fixture);

export const dpsTakenFrom = (fixture: Fixture, targetUnitIndex: number, player: FixturePlayer) => {
	const target = fixture.run.result.encounterMetrics.targets.find((t: any) => (t.unitIndex ?? 0) === targetUnitIndex);
	return damageTo({ actions: target.actions }, player.unitIndex) / iterations(fixture) / duration(fixture);
};

export const unitMetrics = (fixture: Fixture, player: FixturePlayer) => metricsFor(fixture, player);

export const findPlayer = (fixture: Fixture, name: string) => {
	const player = fixture.players.find(p => p.name === name);
	if (!player) throw new Error(`${name} isn't in the fixture`);
	return player;
};
