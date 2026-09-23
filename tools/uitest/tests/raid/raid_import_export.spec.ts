import { expect, type Page, test } from '@playwright/test';

import { openRaid, openSimTab } from '../lib/page';
import {
	assignmentPicker,
	collectDialogs,
	exportJson,
	fixtureRoster,
	importRoster,
	openImporter,
	paste,
	pickTarget,
	readRoster,
	reloadRaid,
	roster,
	seedRaid,
	tankNames,
	tankPickers,
	targetName,
} from './raid';

// The raid25 fixture with a few of a raider's own choices on top, so the round trip has more to carry.
async function customizedRaid(page: Page) {
	await seedRaid(page);
	await openSimTab(page, 'raid-settings-tab');
	await pickTarget(tankPickers(page).nth(1), 'Fury');
	await pickTarget(assignmentPicker(page, 'Innervate', 'Balance'), 'Arcane');
	await openSimTab(page, 'raid-tab');
}

async function importJson(page: Page, json: string) {
	const modal = await openImporter(page, 'JSON');
	await paste(modal.locator('.importer-textarea'), json);
	await modal.locator('.import-button').click();
	await expect(modal).toHaveCount(0);
}

// What a raider sees of the raid: who sits where, the tanks and buff targets, the raid stats, and the
// raid half of the export (the settings half is the browser's own, like its language).
async function raidView(page: Page) {
	await openSimTab(page, 'raid-settings-tab');
	const tanks = await tankNames(page);
	const innervatePicker = assignmentPicker(page, 'Innervate', 'Balance');
	const innervate = (await innervatePicker.count()) ? await targetName(innervatePicker) : 'no Innervate row for Balance';
	await openSimTab(page, 'raid-tab');
	const stats = await page.locator('.raid-stats-category').allInnerTexts();
	const { raid, blessings, encounter } = JSON.parse(await exportJson(page));
	return { roster: await roster(page), tanks, innervate, stats, exported: { raid, blessings, encounter } };
}

test('a JSON export imports into a fresh browser as the same raid', async ({ page, browser }) => {
	await customizedRaid(page);
	const expected = await raidView(page);
	expect(expected.tanks[1]).toBe('Fury');
	expect(expected.innervate).toBe('Arcane');
	const json = await exportJson(page);

	const other = await (await browser.newContext()).newPage();
	await openRaid(other);
	await importJson(other, json);
	expect(await raidView(other)).toEqual(expected);

	await reloadRaid(other);
	expect(await raidView(other)).toEqual(expected);
});

test('a JSON export imports over another raid as the same raid', async ({ page, browser }) => {
	await customizedRaid(page);
	const expected = await raidView(page);
	const json = await exportJson(page);

	const other = await (await browser.newContext()).newPage();
	const dialogs = collectDialogs(other);
	await openRaid(other);
	await importRoster(other, dialogs, readRoster());
	await importJson(other, json);
	expect(await raidView(other)).toEqual(expected);
});

test('a saved raid group loads back over another raid, and can be deleted', async ({ page }) => {
	const dialogs = collectDialogs(page);
	await seedRaid(page);
	const saved = page.locator('.saved-data-manager-root', { hasText: 'Saved Raid Groups' });
	await saved.locator('.saved-data-save-input').fill('Main raid');
	await saved.locator('.saved-data-save-button').click();
	const chip = saved.locator('.saved-data-set-chip', { hasText: 'Main raid' });
	await expect(chip).toHaveClass(/active/);

	await importRoster(page, dialogs, readRoster());
	await expect(chip).not.toHaveClass(/active/);

	await reloadRaid(page);
	await chip.click();
	await expect.poll(() => roster(page)).toEqual(fixtureRoster());
	await expect(chip).toHaveClass(/active/);
	await openSimTab(page, 'raid-settings-tab');
	await expect.poll(async () => (await tankNames(page))[0]).toBe(fixtureRoster()[0]);

	await openSimTab(page, 'raid-tab');
	await chip.locator('.saved-data-set-delete').click();
	await expect.poll(() => dialogs.at(-1)).toBe("Delete saved Raid 'Main raid'?");
	await expect(chip).toHaveCount(0);
	await reloadRaid(page);
	await expect(chip).toHaveCount(0);
});
