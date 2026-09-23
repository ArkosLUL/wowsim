import { expect, test } from '@playwright/test';

import { Fixture, label, loadFixture, showFixture } from '../lib/fixture';
import { watchForErrors } from '../lib/page';
import { openTab, pickPlayer } from './helpers';

let fixture: Fixture;
let logged: Fixture;

test.beforeAll(() => {
	fixture = loadFixture();
	logged = loadFixture('raid25-logged');
});

const TABS = [
	'damageTab',
	'healingTab',
	'damageTakenTab',
	'buffsTab',
	'debuffsTab',
	'castsTab',
	'resourcesTab',
	'timelineTab',
	'logTab',
];

for (const tab of TABS) {
	test(`the ${tab} draws for the whole raid`, async ({ page }) => {
		test.slow(tab == 'logTab', "the raid's log puts tens of thousands of rows on the page");
		const errors = watchForErrors(page);
		await showFixture(page, logged);
		await openTab(page, tab);
		expect(errors).toEqual([]);
	});

	test(`the ${tab} draws for one raider`, async ({ page }) => {
		const errors = watchForErrors(page);
		await showFixture(page, logged);
		await pickPlayer(page, label(logged.players[7]));
		await openTab(page, tab);
		expect(errors).toEqual([]);
	});
}

test('the casts tab counts casts for the raider that was picked', async ({ page }) => {
	await showFixture(page, fixture);
	await openTab(page, 'castsTab');

	const rowsFor = async (player: (typeof fixture.players)[number]) => {
		await pickPlayer(page, label(player));
		return page.locator('#castsTab .metrics-table tbody tr').allInnerTexts();
	};

	// Two specs that share no abilities, so a mixed-up filter can't pass by accident.
	const mage = fixture.players.find(p => p.name === 'Arcane')!;
	const rogue = fixture.players.find(p => p.name === 'Combat')!;

	expect((await rowsFor(mage)).join('\n')).toContain('Arcane Blast');
	expect((await rowsFor(rogue)).join('\n')).not.toContain('Arcane Blast');
});

test('the log tab shows the fight from the start', async ({ page }) => {
	test.slow(true, "the raid's log puts tens of thousands of rows on the page");
	await showFixture(page, logged);
	await openTab(page, 'logTab');

	const lines = page.locator('#logTab .log-runner-logs tr');
	expect(await lines.count()).toBeGreaterThan(0);
	await expect(lines.first().locator('.log-timestamp')).toHaveText('00:00:000');
});

test('every tab still draws when they are opened one after another', async ({ page }) => {
	// Nine tab switches, each waiting out a fade, take well over the default budget.
	test.setTimeout(120_000);
	const errors = watchForErrors(page);
	await showFixture(page, logged);

	for (const tab of TABS) {
		await openTab(page, tab);
	}

	expect(errors).toEqual([]);
});
