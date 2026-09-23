import { expect, type Page, test } from '@playwright/test';

import { Fixture, label, loadFixture, showFixture } from '../lib/fixture';
import { watchForErrors } from '../lib/page';
import { findPlayer, openTab, pickPlayer, showIndividualFixture } from './helpers';

let logged: Fixture;

// A small raid, since the 25-player log makes every step take seconds.
test.beforeAll(() => {
	logged = loadFixture('results-multitarget');
});

const picker = (page: Page) => page.locator('.timeline-chart-picker');
const dpsPlot = (page: Page) => page.locator('.dps-resources-plot');
const rotationPlot = (page: Page) => page.locator('.rotation-plot');
const chartLines = (page: Page) => page.locator('.dps-resources-plot .apexcharts-series');

// The rotation's label column, cut into its sections at the separator rows: the raider's own
// resources and casts first, the target's debuffs last. Each row is named by its icon's link, so
// two spells that share a name stay apart.
const rotationSections = (page: Page) =>
	page.locator('.rotation-labels').evaluate(root => {
		const sections: string[][] = [[]];
		for (const child of Array.from(root.children)) {
			if (child.classList.contains('rotation-timeline-separator')) {
				sections.push([]);
				continue;
			}
			const icon = child.querySelector<HTMLAnchorElement>('.rotation-label-icon');
			if (icon?.href && child.querySelector('.fa-eye-slash')) {
				sections[sections.length - 1].push(icon.href);
			}
		}
		return sections;
	});

test('the whole raid gets a DPS chart with a line per raider', async ({ page }) => {
	await showFixture(page, logged);
	await openTab(page, 'timelineTab');

	await expect(picker(page)).toHaveValue('dps');
	await expect(dpsPlot(page)).toBeVisible();
	await expect(rotationPlot(page)).toBeHidden();
	await expect(chartLines(page)).toHaveCount(logged.players.length);
	await expect(page.locator('.timeline-chart-picker option.rotation-option')).toHaveClass(/hide/);
});

test('the whole raid threat chart has a line per raider', async ({ page }) => {
	await showFixture(page, logged);
	await openTab(page, 'timelineTab');

	await picker(page).selectOption('threat');
	await expect(dpsPlot(page)).toBeVisible();
	await expect(chartLines(page)).toHaveCount(logged.players.length);
});

for (const chart of ['dps', 'threat']) {
	test(`each raider line on the raid ${chart} chart is drawn in a colour`, async ({ page }) => {
		await showFixture(page, logged);
		await openTab(page, 'timelineTab');
		await picker(page).selectOption(chart);
		await expect(chartLines(page)).toHaveCount(logged.players.length);

		// A raider with nothing to plot, like a healer on the DPS chart, gets no line at all.
		const strokes = await page
			.locator('.dps-resources-plot .apexcharts-series path.apexcharts-line')
			.evaluateAll(paths => paths.map(path => getComputedStyle(path).stroke));
		expect(strokes.length).toBeGreaterThan(0);
		for (const stroke of strokes) {
			expect(stroke).toMatch(/^rgb/);
		}
	});
}

test('a raider gets their rotation, and All Players goes back to the raid chart', async ({ page }) => {
	await showFixture(page, logged);
	await openTab(page, 'timelineTab');

	await pickPlayer(page, label(findPlayer(logged, 'Arcane')));
	await expect(picker(page)).toHaveValue('rotation');
	await expect(rotationPlot(page)).toBeVisible();
	await expect(dpsPlot(page)).toBeHidden();
	await expect(page.locator('.rotation-labels .rotation-label-text', { hasText: 'Arcane Blast' }).first()).toBeVisible();

	await pickPlayer(page, 'All Players');
	await expect(picker(page)).toHaveValue('dps');
	await expect(dpsPlot(page)).toBeVisible();
	await expect(rotationPlot(page)).toBeHidden();
	await expect(chartLines(page)).toHaveCount(logged.players.length);
});

test("a raider's DPS chart shows their DPS, mana and threat", async ({ page }) => {
	await showFixture(page, logged);
	await openTab(page, 'timelineTab');
	await pickPlayer(page, label(findPlayer(logged, 'Arcane')));

	await picker(page).selectOption('dps');
	await expect(dpsPlot(page)).toBeVisible();
	await expect(rotationPlot(page)).toBeHidden();
	await expect(chartLines(page)).toHaveCount(3);
});

test('a buff already drawn in its cast row gets no second row', async ({ page }) => {
	await showFixture(page, logged);
	await openTab(page, 'timelineTab');
	await pickPlayer(page, label(findPlayer(logged, 'Arcane')));
	await expect(page.locator('.rotation-labels .rotation-label').first()).toBeVisible();

	const sections = await rotationSections(page);
	const casts = sections[0];
	// The target's debuffs are listed even when a cast row draws them too, so they don't count.
	const allButDebuffs = sections.slice(0, -1).flat();
	expect(casts.length).toBeGreaterThan(0);
	for (const cast of casts) {
		expect(
			allButDebuffs.filter(row => row === cast),
			cast,
		).toHaveLength(1);
	}
});

// One of each shape: a caster with a pet, a hunter, melee, a tank, and a healer who deals no damage.
const TIMELINE_RAIDERS = ['Arcane', 'Marks', 'Combat', 'Tankwar', 'Restosham'];

test('each kind of raider, healers too, gets a timeline without an error', async ({ page }) => {
	const errors = watchForErrors(page);
	await showFixture(page, logged);
	await openTab(page, 'timelineTab');

	for (const player of TIMELINE_RAIDERS.map(name => findPlayer(logged, name))) {
		await pickPlayer(page, label(player));
		await expect(page.locator('.player-filter-root .dropdown-picker-button')).toHaveText(label(player));
		await expect(page.locator('.rotation-labels .rotation-label').first()).toBeVisible();
		expect(errors, label(player)).toEqual([]);
	}
});

test('a raider with no damage still switches the page to that raider', async ({ page }) => {
	const errors = watchForErrors(page);
	await showFixture(page, logged);
	await openTab(page, 'timelineTab');

	const healer = logged.players.find(player => player.dps === 0)!;
	await pickPlayer(page, label(healer));
	await openTab(page, 'damageTab');

	await expect(page.locator('.dr-root')).toHaveClass(/single-player/);
	await expect(page.locator('.damage-content .results-sim-dps .topline-result-avg')).toHaveText(healer.dps.toFixed(2));
	expect(errors).toEqual([]);
});

// What the chart's tooltip shows for each point of a line, asked the way ApexCharts asks on hover.
const tooltipsOf = (page: Page, seriesName: string) =>
	page.evaluate(name => {
		const chart = (window as any).Apex._chartInstances.find((instance: any) => instance.id === 'dpsResources').chart;
		const w = chart.w;
		const seriesIndex = w.config.series.findIndex((series: any) => series.name === name);
		return w.config.series[seriesIndex].data.map((point: { x: number; y: number }, dataPointIndex: number) => ({
			point,
			tooltip: w.config.tooltip.custom({ series: w.globals.series, seriesIndex, dataPointIndex, w }) as string,
		}));
	}, seriesName);

test('a tooltip on the DPS chart describes the point under it, even with a cast before the pull', async ({ page }) => {
	const prepull = loadFixture('results-prepull');
	const [warlock] = prepull.players;
	const prepullThreat = (prepull.run.result.logs as string)
		.split('\n')
		.filter(line => line.startsWith('[-') && line.includes(`[${label(warlock)}]`) && /\(Threat: [1-9]/.test(line));
	expect(prepullThreat.length).toBeGreaterThan(0);

	await showIndividualFixture(page, prepull);
	await openTab(page, 'timelineTab');
	await picker(page).selectOption('dps');
	await expect(chartLines(page)).toHaveCount(3);

	for (const [series, value] of [
		['DPS', (y: number) => `DPS: ${y.toFixed(2)}`],
		['Threat', (y: number) => `After: ${y.toFixed(1)}`],
	] as const) {
		const tooltips = await tooltipsOf(page, series);
		expect(tooltips.length, series).toBeGreaterThan(0);
		for (const { point, tooltip } of tooltips) {
			expect(tooltip, `${series} at ${point.x}`).toContain(`>${point.x.toFixed(2)}s<`);
			expect(tooltip, `${series} at ${point.x}`).toContain(value(point.y));
		}
	}
});

test('a rotation row can be hidden and shown again', async ({ page }) => {
	await showIndividualFixture(page, loadFixture('results-single'));
	await openTab(page, 'timelineTab');

	const labels = page.locator('.rotation-labels .rotation-label:not(.hide)');
	const rows = page.locator('.rotation-timeline .rotation-timeline-row:not(.hide)');
	const hiddenList = page.locator('.rotation-hidden-ids .rotation-label:not(.hide)');
	await expect(labels.first()).toBeVisible();
	const shown = await labels.count();
	const rowCount = await rows.count();

	// The first row with an eye icon is a cast, not a resource.
	const row = labels.filter({ has: page.locator('.fa-eye-slash') }).first();
	const name = await row.locator('.rotation-label-text').innerText();
	await row.locator('.fa-eye-slash').click();
	await expect(labels).toHaveCount(shown - 1);
	await expect(rows).toHaveCount(rowCount - 1);
	await expect(hiddenList).toHaveText([name]);

	await hiddenList.first().locator('.fa-eye').click();
	await expect(labels).toHaveCount(shown);
	await expect(rows).toHaveCount(rowCount);
	await expect(hiddenList).toHaveCount(0);
});
