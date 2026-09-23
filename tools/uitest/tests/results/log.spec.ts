import { expect, type Page, test } from '@playwright/test';

import { Fixture, label, loadFixture, showFixture } from '../lib/fixture';
import { findPlayer, openTab, pickPlayer, pickTarget } from './helpers';

let logged: Fixture;

// A small raid with three targets: the 25-player log runs to tens of thousands of lines, too
// heavy to page through in every test.
test.beforeAll(() => {
	logged = loadFixture('results-multitarget');
});

const lines = (page: Page) => page.locator('#logTab .log-runner-logs tr');
const lineTexts = (page: Page) => lines(page).evaluateAll(rows => rows.map(row => row.textContent ?? ''));
const debugToggle = (page: Page) => page.locator('#logTab .show-debug-picker input');
const search = (page: Page) => page.locator('#logTab .log-search-input');

const openLog = async (page: Page) => {
	await showFixture(page, logged);
	await openTab(page, 'logTab');
	await expect(lines(page).first()).toBeVisible();
	return lines(page).count();
};

test('debug lines only show while the toggle is on', async ({ page }) => {
	const total = await openLog(page);
	expect((await lineTexts(page)).filter(text => text.includes('[DEBUG]'))).toEqual([]);

	await debugToggle(page).check();
	await expect.poll(() => lines(page).count()).toBeGreaterThan(total);
	const debugLines = (await lineTexts(page)).filter(text => text.includes('[DEBUG]'));
	const expected = (logged.run.result.logs as string).split('\n').filter(line => line.includes('[DEBUG]'));
	expect(debugLines).toHaveLength(expected.length);

	await debugToggle(page).uncheck();
	await expect(lines(page)).toHaveCount(total);
});

test('the search narrows the log to the lines that match', async ({ page }) => {
	const total = await openLog(page);

	await search(page).fill('arcane BLAST');
	await expect.poll(() => lines(page).count()).toBeLessThan(total);
	const texts = await lineTexts(page);
	expect(texts.length).toBeGreaterThan(0);
	for (const text of texts) {
		expect(text.toLowerCase()).toContain('arcane blast');
	}

	await search(page).fill('');
	await expect(lines(page)).toHaveCount(total);
});

test('a search nothing matches empties the log', async ({ page }) => {
	await openLog(page);
	await search(page).fill('no such event anywhere');
	await expect(lines(page)).toHaveCount(0);
});

test('picking a raider narrows the log to the lines about them', async ({ page }) => {
	const total = await openLog(page);
	const mage = findPlayer(logged, 'Arcane');

	await pickPlayer(page, label(mage));
	await expect.poll(() => lines(page).count()).toBeLessThan(total);
	const texts = await lineTexts(page);
	expect(texts.length).toBeGreaterThan(0);
	// As the source the log tags them "[Arcane 5]", as the target it keeps "Arcane (#5)".
	const tags = [`[${mage.name} ${mage.raidIndex + 1}]`, label(mage)];
	for (const text of texts) {
		expect(
			tags.some(tag => text.includes(tag)),
			text,
		).toBe(true);
	}

	await pickPlayer(page, 'All Players');
	await expect(lines(page)).toHaveCount(total);
});

test('picking a target narrows the log to the lines about it', async ({ page }) => {
	const total = await openLog(page);
	const target = logged.targets[1];

	await pickTarget(page, target.name);
	await expect.poll(() => lines(page).count()).toBeLessThan(total);
	const texts = await lineTexts(page);
	expect(texts.length).toBeGreaterThan(0);
	for (const text of texts) {
		expect(text, text).toContain(target.name);
	}
});

test('the search and the raider picked narrow the log together', async ({ page }) => {
	await openLog(page);
	const mage = findPlayer(logged, 'Arcane');
	await pickPlayer(page, label(mage));
	const forMage = await lines(page).count();

	await search(page).fill('arcane blast');
	await expect.poll(() => lines(page).count()).toBeLessThan(forMage);
	for (const text of await lineTexts(page)) {
		expect(text.toLowerCase()).toContain('arcane blast');
		expect(text).toContain(mage.name);
	}
});
