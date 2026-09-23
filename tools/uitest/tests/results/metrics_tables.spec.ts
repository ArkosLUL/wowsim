import { expect, type Locator, type Page, test } from '@playwright/test';

import { Fixture, label, loadFixture, showFixture } from '../lib/fixture';
import { columnTexts, dpsTakenFrom, findPlayer, headerCell, openTab, pickPlayer, pickTarget, sortBy, unitMetrics } from './helpers';

let raid: Fixture;
let multi: Fixture;

test.beforeAll(() => {
	raid = loadFixture('raid25');
	multi = loadFixture('results-multitarget');
});

// Two raiders who share no abilities, so a table showing the wrong one can't pass by accident.
const MAGE = 'Arcane';
const ROGUE = 'Combat';

const table = (page: Page, root: string) => page.locator(`.${root}`);
const rowNamed = (t: Locator, name: string) => t.locator('tbody tr', { has: t.page().locator(`.metrics-action-name:text-is("${name}")`) });
const pick = (page: Page, fixture: Fixture, name: string) => pickPlayer(page, label(findPlayer(fixture, name)));

test('the spell and melee tables show the raider picked', async ({ page }) => {
	await showFixture(page, raid);

	await pick(page, raid, MAGE);
	await expect(rowNamed(table(page, 'spell-metrics-root'), 'Arcane Blast')).toHaveCount(1);
	await expect(table(page, 'melee-metrics-root')).toHaveClass(/hide/);

	await pick(page, raid, ROGUE);
	await expect(rowNamed(table(page, 'melee-metrics-root'), 'Sinister Strike')).toHaveCount(1);
	await expect(rowNamed(table(page, 'spell-metrics-root'), 'Arcane Blast')).toHaveCount(0);
});

test('the healing table shows the healer picked, with no NaN in it', async ({ page }) => {
	await showFixture(page, raid);
	await openTab(page, 'healingTab');

	await pickPlayer(page, label(findPlayer(raid, 'Disc')));
	const healing = table(page, 'healing-metrics-root');
	await expect(rowNamed(healing, 'Penance')).toHaveCount(1);
	await expect(healing.locator('tbody')).not.toContainText('NaN');

	await pick(page, raid, MAGE);
	await expect(rowNamed(healing, 'Penance')).toHaveCount(0);
});

test('the casts table counts the raider picked', async ({ page }) => {
	await showFixture(page, raid);
	await openTab(page, 'castsTab');
	await pick(page, raid, MAGE);

	const mage = unitMetrics(raid, findPlayer(raid, MAGE));
	const arcaneBlast = mage.actions.find((action: any) => action.id.spellId === 42897);
	const casts = arcaneBlast.targets.reduce((total: number, target: any) => total + (target.casts ?? 0), 0);
	const iterations = Number(raid.run.request.simOptions.iterations);
	await expect(rowNamed(table(page, 'cast-metrics-root'), 'Arcane Blast').locator('td').nth(1)).toHaveText((casts / iterations).toFixed(1));
});

test('the buffs table shows the raider picked', async ({ page }) => {
	await showFixture(page, raid);
	await openTab(page, 'buffsTab');
	const buffs = table(page, 'buff-metrics-root');

	await pick(page, raid, ROGUE);
	await expect(rowNamed(buffs, 'Slice and Dice')).toHaveCount(1);
	await expect(rowNamed(buffs, 'Arcane Power')).toHaveCount(0);

	await pick(page, raid, MAGE);
	await expect(rowNamed(buffs, 'Arcane Power')).toHaveCount(1);
	await expect(rowNamed(buffs, 'Slice and Dice')).toHaveCount(0);
});

test('the resources tab shows the raider picked', async ({ page }) => {
	await showFixture(page, raid);
	await openTab(page, 'resourcesTab');
	const title = (name: string) => page.locator('.resource-metrics-table-container:not(.hide) .resource-metrics-table-title', { hasText: name });

	await pick(page, raid, ROGUE);
	await expect(title('Energy')).toBeVisible();
	await expect(title('Combo Points')).toBeVisible();
	await expect(title('Mana')).toHaveCount(0);

	await pick(page, raid, MAGE);
	await expect(title('Mana')).toBeVisible();
	await expect(title('Energy')).toHaveCount(0);
});

test('the debuffs table shows the target picked, with its procs and uptime', async ({ page }) => {
	await showFixture(page, multi);
	await openTab(page, 'debuffsTab');
	const duration = Number(multi.run.result.avgIterationDuration);
	const debuffs = table(page, 'debuff-metrics-root');

	for (const target of multi.targets) {
		await pickTarget(page, target.name);

		// One row per aura: its copies' procs added up, and the longest of their uptimes.
		const byAura = new Map<string, { procs: number; uptime: number }>();
		const metrics = multi.run.result.encounterMetrics.targets.find((t: any) => (t.unitIndex ?? 0) === target.unitIndex);
		for (const aura of metrics.auras ?? []) {
			const key = JSON.stringify({ ...aura.id, tag: undefined });
			const seen = byAura.get(key) ?? { procs: 0, uptime: 0 };
			byAura.set(key, { procs: seen.procs + (aura.procsAvg ?? 0), uptime: Math.max(seen.uptime, aura.uptimeSecondsAvg ?? 0) });
		}
		const expected = [...byAura.values()]
			.filter(aura => aura.uptime > 0)
			.map(aura => `${aura.procs.toFixed(2)} ${((aura.uptime / duration) * 100).toFixed(2)}%`)
			.sort();
		expect(expected.some(row => !row.startsWith('0.00 '))).toBe(true);

		await expect(debuffs.locator('tbody tr:not(.child-metric)')).toHaveCount(expected.length);
		const shown = await debuffs
			.locator('tbody tr:not(.child-metric)')
			.evaluateAll(rows => rows.map(row => `${row.children[1].textContent} ${row.children[3].textContent}`));
		expect(shown.sort()).toEqual(expected);
	}
});

test('the damage taken table shows only the hits the raider took', async ({ page }) => {
	await showFixture(page, multi);
	await openTab(page, 'damageTakenTab');
	const damageTaken = table(page, 'dtps-melee-metrics-root');

	// Each tank has one target on them, and the other targets never swing at them.
	for (const name of ['Tankwar', 'Tankpal']) {
		const tank = findPlayer(multi, name);
		await pickPlayer(page, label(tank));

		const expected = multi.targets
			.map(target => dpsTakenFrom(multi, target.unitIndex, tank))
			.filter(dps => dps > 0)
			.map(dps => dps.toFixed(1));
		expect(expected).toHaveLength(1);
		await expect(damageTaken.locator('tbody tr td:nth-child(2)')).toHaveText(expected);
	}

	await pick(page, multi, MAGE);
	await expect(damageTaken).toHaveClass(/hide/);
});

// Each table, opened on a raider who fills it, and a numeric column to sort it by.
const SORTABLE = [
	{ tab: 'damageTab', root: 'player-damage-metrics-root', raider: null, column: 'DPS' },
	{ tab: 'damageTab', root: 'spell-metrics-root', raider: MAGE, column: 'Casts' },
	{ tab: 'damageTab', root: 'melee-metrics-root', raider: ROGUE, column: 'Hits' },
	{ tab: 'healingTab', root: 'healing-metrics-root', raider: 'Disc', column: 'HPS' },
	{ tab: 'buffsTab', root: 'buff-metrics-root', raider: ROGUE, column: 'Uptime' },
	{ tab: 'debuffsTab', root: 'debuff-metrics-root', raider: null, column: 'Uptime' },
	{ tab: 'castsTab', root: 'cast-metrics-root', raider: MAGE, column: 'CPM' },
	{ tab: 'resourcesTab', root: 'resource-metrics-table-root:not(.hide)', raider: MAGE, column: 'Gain' },
];

const topLevelValues = async (t: Locator, column: number) =>
	(
		await t.locator('tbody tr:not(.child-metric)').evaluateAll((rows, column) => rows.map(row => (row.children[column]?.textContent ?? '').trim()), column)
	).map(text => parseFloat(text));

for (const { tab, root, raider, column } of SORTABLE) {
	test(`the ${root.split(':')[0]} table sorts by ${column}`, async ({ page }) => {
		await showFixture(page, raid);
		await openTab(page, tab);
		if (raider) await pick(page, raid, raider);

		const t = page.locator(`.${root}`).first();
		const index = await headerCell(t, column).evaluate(th => Array.from(th.parentElement!.children).indexOf(th));
		expect(await t.locator('tbody tr:not(.child-metric)').count()).toBeGreaterThan(2);

		for (let click = 0; click < 2; click++) {
			const direction = await sortBy(t, column);
			const values = await topLevelValues(t, index);
			const sorted = [...values].sort((a, b) => (direction === 'ascending' ? a - b : b - a));
			expect(values, direction).toEqual(sorted);
		}
	});
}

test('the damage table sorts by name', async ({ page }) => {
	await showFixture(page, raid);
	const t = table(page, 'player-damage-metrics-root');

	const direction = await sortBy(t, 'Name');
	const names = await columnTexts(t, 0);
	const sorted = [...names].sort((a, b) => a.localeCompare(b, undefined, { sensitivity: 'base' }));
	expect(names).toEqual(direction === 'ascending' ? sorted : sorted.reverse());
});

test('a sort sticks when another raider is picked', async ({ page }) => {
	await showFixture(page, raid);
	await pick(page, raid, MAGE);
	const spells = table(page, 'spell-metrics-root');
	const index = await headerCell(spells, 'Casts').evaluate(th => Array.from(th.parentElement!.children).indexOf(th));

	const direction = await sortBy(spells, 'Casts');
	await pick(page, raid, ROGUE);
	await expect(headerCell(spells, 'Casts')).toHaveAttribute('aria-sort', direction);

	const values = await topLevelValues(spells, index);
	const sorted = [...values].sort((a, b) => (direction === 'ascending' ? a - b : b - a));
	expect(values).toEqual(sorted);
});

test('the casts table puts pets that share a name in one group', async ({ page }) => {
	await showFixture(page, raid);
	await openTab(page, 'castsTab');
	const dk = findPlayer(raid, 'Unholy');
	await pickPlayer(page, label(dk));

	const army = (unitMetrics(raid, dk).pets ?? []).filter((pet: any) => pet.name === 'Army of the Dead');
	expect(army.length).toBeGreaterThan(1);
	const casts = army
		.flatMap((pet: any) => pet.actions ?? [])
		.flatMap((action: any) => action.targets ?? [])
		.reduce((total: number, target: any) => total + (target.casts ?? 0), 0);
	const iterations = Number(raid.run.request.simOptions.iterations);

	const groups = table(page, 'cast-metrics-root').locator('tbody tr.parent-metric', {
		has: page.locator('.metrics-action-name:text-is("Army of the Dead")'),
	});
	await expect(groups).toHaveCount(1);
	await expect(groups.locator('td').nth(1)).toHaveText((casts / iterations).toFixed(1));
});

test('the buffs table puts pets that share a name in one group, and keeps their procs', async ({ page }) => {
	await showFixture(page, raid);
	await openTab(page, 'buffsTab');
	const buffs = table(page, 'buff-metrics-root');

	await pickPlayer(page, label(findPlayer(raid, 'Unholy')));
	const groups = buffs.locator('tbody tr.parent-metric', { has: page.locator('.metrics-action-name:text-is("Army of the Dead")') });
	await expect(groups).toHaveCount(1);

	// A lone pet's buffs show the procs the fixture has for them.
	const hunter = findPlayer(raid, 'Marks');
	await pickPlayer(page, label(hunter));
	const [wolf] = unitMetrics(raid, hunter).pets;
	const procs = (wolf.auras ?? []).map((aura: any) => Number(aura.procsAvg ?? 0)).filter((p: number) => p > 0);
	expect(procs.length).toBeGreaterThan(0);
	const shown = await buffs.locator('tbody tr.child-metric td:nth-child(2)').allInnerTexts();
	for (const p of procs) {
		expect(shown).toContain(p.toFixed(2));
	}
});
