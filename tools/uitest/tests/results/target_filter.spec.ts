import { expect, test } from '@playwright/test';

import { Fixture, label, loadFixture, showFixture } from '../lib/fixture';
import { dpsAgainst, dpsTakenFrom, findPlayer, pickPlayer, pickTarget, topline } from './helpers';

let fixture: Fixture;

test.beforeAll(() => {
	fixture = loadFixture('results-multitarget');
});

const damageRows = (page: any) => page.locator('.player-damage-metrics-root .player-damage-row');

test('the target dropdown offers every target once, and names the one picked', async ({ page }) => {
	await showFixture(page, fixture);

	const options = await page.locator('.target-filter-root .dropdown-picker-item button').allInnerTexts();
	expect(options).toEqual(['All Targets', ...fixture.targets.map(target => target.name)]);

	for (const target of fixture.targets) {
		await pickTarget(page, target.name);
		await expect(page.locator('.target-filter-root .dropdown-picker-button')).toHaveText(target.name);
	}
});

test("a raider's damage narrows to the target picked", async ({ page }) => {
	await showFixture(page, fixture);
	const rogue = findPlayer(fixture, 'Combat');
	await pickPlayer(page, label(rogue));

	for (const target of fixture.targets) {
		await pickTarget(page, target.name);
		await expect(topline(page, 'damage-content', 'dps')).toHaveText(dpsAgainst(fixture, rogue, target.unitIndex).toFixed(2));
	}

	await pickTarget(page, 'All Targets');
	await expect(topline(page, 'damage-content', 'dps')).toHaveText(rogue.dps.toFixed(2));
});

test("the raid's damage table narrows to the target picked", async ({ page }) => {
	await showFixture(page, fixture);

	for (const target of fixture.targets) {
		await pickTarget(page, target.name);

		const expected = fixture.players
			.map(player => dpsAgainst(fixture, player, target.unitIndex))
			.sort((a, b) => b - a)
			.map(dps => dps.toFixed(1));
		await expect(damageRows(page).locator('td:last-child')).toHaveText(expected);
	}

	await pickTarget(page, 'All Targets');
	const everything = [...fixture.players].sort((a, b) => b.dps - a.dps).map(player => player.dps.toFixed(1));
	await expect(damageRows(page).locator('td:last-child')).toHaveText(everything);
});

test("the raid's topline DPS narrows to the target picked", async ({ page }) => {
	await showFixture(page, fixture);

	for (const target of fixture.targets) {
		await pickTarget(page, target.name);
		const raidDps = fixture.players.reduce((total, player) => total + dpsAgainst(fixture, player, target.unitIndex), 0);
		await expect(topline(page, 'damage-content', 'dps')).toHaveText(raidDps.toFixed(2));
	}
});

test('a raider in the damage table keeps their name whatever the target', async ({ page }) => {
	await showFixture(page, fixture);
	await pickTarget(page, fixture.targets[1].name);

	const names = await damageRows(page).locator('.metrics-action-name').allInnerTexts();
	expect([...names].sort()).toEqual(fixture.players.map(label).sort());
});

test("a tank's damage taken narrows to the target picked", async ({ page }) => {
	await showFixture(page, fixture);
	const tank = findPlayer(fixture, 'Tankpal');
	await pickPlayer(page, label(tank));

	for (const target of fixture.targets) {
		await pickTarget(page, target.name);
		await expect(topline(page, 'damage-content', 'dtps')).toHaveText(dpsTakenFrom(fixture, target.unitIndex, tank).toFixed(2));
	}
});
