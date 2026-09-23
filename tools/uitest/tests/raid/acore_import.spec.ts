import { expect, test } from '@playwright/test';

import { openRaid, openSimTab, watchForErrors } from '../lib/page';
import {
	assignmentPicker,
	collectDialogs,
	fixtureRoster,
	importRoster,
	openImporter,
	pickTarget,
	readRoster,
	reloadRaid,
	roster,
	rosterMainTank,
	rosterPath,
	rosterSlots,
	seedRaid,
	slot,
	tankNames,
	targetName,
} from './raid';

const ROSTER = readRoster();
const rosterJson = JSON.parse(ROSTER);
const mainTank = rosterMainTank(ROSTER);

test('the dialog starts on Replace for an empty raid and on Update once there is one', async ({ page }) => {
	const dialogs = collectDialogs(page);
	await openRaid(page);
	let modal = await openImporter(page, 'AzerothCore');
	await expect(modal.locator('.modal-title')).toHaveText('AzerothCore Import');
	await expect(modal.locator('.acore-import-mode input:checked')).toHaveValue('replace');
	await modal.locator('.close-button').click();
	await expect(modal).toHaveCount(0);

	await importRoster(page, dialogs, ROSTER);
	modal = await openImporter(page, 'AzerothCore');
	await expect(modal.locator('.acore-import-mode input:checked')).toHaveValue('update');
});

test('Replace seats every raider in their subgroup and reports it', async ({ page }) => {
	const errors = watchForErrors(page);
	const dialogs = collectDialogs(page);
	await openRaid(page);

	const report = await importRoster(page, dialogs, ROSTER);
	const lines = report.split('\n');
	expect(lines[0]).toBe(`AzerothCore import (Replace): ${rosterJson.group.leader}'s raid, exported ${rosterJson.exportedAt}.`);
	expect(lines[1]).toBe(`${rosterJson.characters.length} of ${rosterJson.characters.length} characters in 5 parties.`);
	expect(report).toContain(`${mainTank}: `);
	expect(report).toContain('(main tank)');

	await expect.poll(() => roster(page)).toEqual(rosterSlots(ROSTER));
	await openSimTab(page, 'raid-settings-tab');
	await expect.poll(async () => (await tankNames(page))[0]).toBe(mainTank);

	await reloadRaid(page);
	await expect.poll(() => roster(page)).toEqual(rosterSlots(ROSTER));
	expect(errors).toEqual([]);
});

test('Replace over a raid shows the roster tanks straight away', async ({ page }) => {
	const dialogs = collectDialogs(page);
	await seedRaid(page);

	await importRoster(page, dialogs, ROSTER, 'replace');
	await expect.poll(() => roster(page)).toEqual(rosterSlots(ROSTER));
	await openSimTab(page, 'raid-settings-tab');
	const tanks = await tankNames(page);
	expect(tanks[0]).toBe(mainTank);
	expect(tanks.filter(name => fixtureRoster().includes(name))).toEqual([]);

	await reloadRaid(page);
	await openSimTab(page, 'raid-settings-tab');
	await expect.poll(() => tankNames(page)).toEqual(tanks);
});

test('Update keeps raiders, replaces a respec and drops who left the roster', async ({ page }) => {
	const dialogs = collectDialogs(page);
	await openRaid(page);
	await importRoster(page, dialogs, ROSTER);
	await openSimTab(page, 'raid-settings-tab');
	const innervate = assignmentPicker(page, 'Innervate', 'Druidica');
	await pickTarget(innervate, 'Malediction');

	let report = await importRoster(page, dialogs, readRoster('raid-justice-prot'), 'update');
	expect(report.split('\n')[0]).toContain('AzerothCore import (Update)');
	expect(report).toMatch(/Replaced, their top talent tree changed:\n {2}Justice: /);
	await expect.poll(() => roster(page)).toEqual(rosterSlots(ROSTER));

	report = await importRoster(page, dialogs, readRoster('raid-no-tree'), 'update');
	expect(report).toContain("Removed, they aren't in the roster: Tree");
	await expect.poll(() => roster(page)).toEqual(rosterSlots(readRoster('raid-no-tree')));
	await expect.poll(() => targetName(innervate)).toBe('Malediction');
});

test('a raider with no room in their subgroup is skipped, and the report says so', async ({ page }) => {
	const dialogs = collectDialogs(page);
	await openRaid(page);
	const overfull = JSON.parse(ROSTER);
	// the first raider of the second group joins the first, which is already full
	const extra = overfull.characters.find((char: any) => char.subgroup == 1);
	extra.subgroup = 0;

	const report = await importRoster(page, dialogs, JSON.stringify(overfull));
	expect(report.split('\n')[1]).toBe(`${overfull.characters.length - 1} of ${overfull.characters.length} characters in 5 parties.`);
	expect(report).toContain(`Skipped:\n  ${extra.name}: subgroup 0 has no room in the raid`);
	await expect.poll(() => roster(page)).not.toContain(extra.name);
});

test('a file that is not a roster gets an error and leaves the raid alone', async ({ page }) => {
	const dialogs = collectDialogs(page);
	await seedRaid(page);

	const modal = await openImporter(page, 'AzerothCore');
	await modal.locator('.importer-textarea').fill('{"version": 1, "characters": [');
	await modal.locator('.import-button').click();
	await expect.poll(() => dialogs.length).toBe(1);
	expect(dialogs[0]).toMatch(/^Import error:\nThat isn't valid JSON/);

	await modal.locator('.importer-textarea').fill(JSON.stringify({ ...rosterJson, version: 99 }));
	await modal.locator('.import-button').click();
	await expect.poll(() => dialogs.length).toBe(2);
	expect(dialogs[1]).toContain("Roster version 99 isn't supported");

	await expect(modal).toBeVisible();
	await modal.locator('.close-button').click();
	await expect.poll(() => roster(page)).toEqual(fixtureRoster());
});

test('Upload File reads the roster into the dialog', async ({ page }) => {
	const dialogs = collectDialogs(page);
	await openRaid(page);

	const modal = await openImporter(page, 'AzerothCore');
	await modal.locator('.importer-upload-input').setInputFiles(rosterPath());
	await expect(modal.locator('.importer-textarea')).toHaveValue(ROSTER);
	await modal.locator('.import-button').click();
	await expect.poll(() => dialogs.length).toBe(1);
	await expect.poll(() => roster(page)).toEqual(rosterSlots(ROSTER));
});

test('gear only the leftovers database knows survives a reload', async ({ page }) => {
	const dialogs = collectDialogs(page);
	await openRaid(page);
	const leftover = readRoster('raid-leftover');
	const angry = JSON.parse(leftover).characters.find((char: any) => char.name == 'Angry');
	// the one item this variant swaps in, in a ring slot
	const itemId = angry.gear.find((item: any) => item.acSlot == 10).id;
	await importRoster(page, dialogs, leftover);

	const editor = page.locator('.modal.show', { has: page.locator('.player-editor-modal') });
	const item = editor.locator(`#gear-tab .item-picker-root .item-picker-name[href*="item=${itemId}"]`);
	const raidIndex = rosterSlots(leftover).indexOf('Angry');
	await slot(page, raidIndex).locator('.player-edit').click();
	await expect(item).toHaveCount(1);
	const name = await item.innerText();
	await editor.locator('.close-button').click();
	await expect(editor).toHaveCount(0);

	await reloadRaid(page);
	await slot(page, raidIndex).locator('.player-edit').click();
	await expect(item).toHaveText(name);
});
