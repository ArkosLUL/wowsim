import { expect, test } from '@playwright/test';

import { Fixture, label, loadFixture, showFixture } from '../lib/fixture';
import { dpsTakenFrom, findPlayer, openTab, pickPlayer, topline, unitMetrics } from './helpers';

let raid: Fixture;

test.beforeAll(() => {
	raid = loadFixture('raid25');
});

const TANKS = ['Tankwar', 'Tankpal', 'Tankbear', 'Tankdk'];
const HEALERS = ['Restosham', 'Holypal', 'Disc', 'Restodruid'];

const avg = (metrics: any) => Number(metrics?.avg ?? 0);

test("each tank's damage taken, TMI and chance of death are their own", async ({ page }) => {
	await showFixture(page, raid);

	for (const name of TANKS) {
		const tank = findPlayer(raid, name);
		const metrics = unitMetrics(raid, tank);
		await pickPlayer(page, label(tank));

		await expect(topline(page, 'damage-content', 'dtps')).toHaveText(avg(metrics.dtps).toFixed(2));
		await expect(topline(page, 'damage-content', 'tmi')).toHaveText(avg(metrics.tmi).toFixed(2));
		await expect(topline(page, 'damage-content', 'cod')).toHaveText((Number(metrics.chanceOfDeath ?? 0) * 100).toFixed(2));
	}
});

test("the damage taken table shows the boss's hits on the tank picked", async ({ page }) => {
	await showFixture(page, raid);
	await openTab(page, 'damageTakenTab');
	const [boss] = raid.targets;

	for (const name of TANKS) {
		const tank = findPlayer(raid, name);
		await pickPlayer(page, label(tank));

		const taken = dpsTakenFrom(raid, boss.unitIndex, tank);
		const rows = page.locator('.dtps-melee-metrics-root tbody tr td:nth-child(2)');
		if (taken > 0) {
			await expect(rows).toHaveText([taken.toFixed(1)]);
		} else {
			await expect(page.locator('.dtps-melee-metrics-root')).toHaveClass(/hide/);
		}
	}
});

test("each healer's HPS is their own", async ({ page }) => {
	await showFixture(page, raid);
	await openTab(page, 'healingTab');

	for (const name of HEALERS) {
		const healer = findPlayer(raid, name);
		await pickPlayer(page, label(healer));
		await expect(topline(page, 'healing-content', 'hps')).toHaveText(avg(unitMetrics(raid, healer).hps).toFixed(2));
	}
});

test("the healing table adds up to the healer's healing", async ({ page }) => {
	await showFixture(page, raid);
	await openTab(page, 'healingTab');
	const iterations = Number(raid.run.request.simOptions.iterations);
	const duration = Number(raid.run.result.avgIterationDuration);

	for (const name of HEALERS) {
		const healer = findPlayer(raid, name);
		await pickPlayer(page, label(healer));

		const healing = (unitMetrics(raid, healer).actions ?? [])
			.flatMap((action: any) => action.targets ?? [])
			.reduce((total: number, target: any) => total + (target.healing ?? 0) + (target.shielding ?? 0), 0);
		const shown = (await page
			.locator('.healing-metrics-root tbody tr:not(.child-metric)')
			.evaluateAll(rows => rows.map(row => parseFloat(row.children[6]?.textContent ?? '0')))) as number[];
		const total = shown.reduce((sum, hps) => sum + hps, 0);
		// Each row rounds to 0.1.
		expect(Math.abs(total - healing / iterations / duration), name).toBeLessThan(0.05 * Math.max(shown.length, 1) + 0.001);
	}
});
