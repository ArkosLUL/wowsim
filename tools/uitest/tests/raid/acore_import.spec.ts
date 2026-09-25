import { expect, type Locator, type Page, test } from '@playwright/test';

import { openRaid, openSimTab, watchForErrors } from '../lib/page';
import { openSpec, storedSettings } from '../settings/helpers';
import {
	assignmentPicker,
	collectDialogs,
	fixtureRoster,
	importRoster,
	openImporter,
	paste,
	pickTarget,
	playerEditor,
	readRoster,
	reloadRaid,
	roster,
	rosterMainTank,
	rosterPath,
	rosterSlots,
	savedSettings,
	seedRaid,
	slot,
	tankNames,
	targetName,
} from './raid';

const ROSTER = readRoster();
const rosterJson = JSON.parse(ROSTER);
const mainTank = rosterMainTank(ROSTER);

// Synthetic version 2 roster: pets, ammo, and bots' and played characters' consumables.
const LOADOUT = readRoster('loadout');

// The raider as the raid page saved them, in protojson.
async function savedRaider(page: Page, name: string): Promise<any> {
	const settings = await savedSettings(page);
	return (settings.raid?.parties ?? []).flatMap((party: any) => party.players ?? []).find((player: any) => player.name == name);
}

// A closed Edit window stays in the page with an import link of its own, so this takes the top bar's.
async function openTopImporter(page: Page): Promise<Locator> {
	await page.locator('.import-link').first().hover();
	await page.locator('.import-dropdown .dropdown-item', { hasText: /^\s*AzerothCore\s*$/ }).first().click();
	const modal = page.locator('.modal.show', { has: page.locator('.importer') });
	await expect(modal).toBeVisible();
	return modal;
}

async function updateWithConsumables(page: Page, dialogs: Array<string>, rosterText: string, refresh: boolean): Promise<string> {
	const seen = dialogs.length;
	const modal = await openTopImporter(page);
	await modal.locator('.acore-import-mode input[value=update]').check();
	await modal.locator('.acore-refresh-consumes').setChecked(refresh);
	await paste(modal.locator('.importer-textarea'), rosterText);
	await modal.locator('.import-button').click();
	await expect.poll(() => dialogs.length, { timeout: 60_000 }).toBeGreaterThan(seen);
	await expect(modal).toHaveCount(0);
	return dialogs[seen];
}

// The loadout roster with Fletcher on other ammo and flask, and a new hunter in the third group.
function changedLoadout(): string {
	const changed = JSON.parse(LOADOUT);
	const fletcher = changed.characters.find((char: any) => char.name == 'Fletcher');
	fletcher.ammo = { itemId: 41165, dps: 67.5, value: 'SaroniteRazorheads' };
	fletcher.consumes.flask = { value: 'FlaskOfPureMojo', source: 'matrix', itemId: 46378, spellId: 54212 };
	changed.characters.push({ ...JSON.parse(JSON.stringify(fletcher)), name: 'Newcomer', subgroup: 2 });
	return JSON.stringify(changed);
}

test('the dialog starts on Replace for an empty raid and on Update once there is one', async ({ page }) => {
	const dialogs = collectDialogs(page);
	await openRaid(page);
	let modal = await openImporter(page, 'AzerothCore');
	await expect(modal.locator('.modal-title')).toHaveText('AzerothCore Import');
	await expect(modal.locator('.acore-import-mode input[type=radio]:checked')).toHaveValue('replace');
	// Replace takes every raider's consumables anyway, so the box only counts for Update
	const refresh = modal.locator('.acore-refresh-consumes');
	await expect(refresh).toBeChecked();
	await expect(refresh).toBeDisabled();
	await modal.locator('.close-button').click();
	await expect(modal).toHaveCount(0);

	await importRoster(page, dialogs, ROSTER);
	modal = await openImporter(page, 'AzerothCore');
	await expect(modal.locator('.acore-import-mode input[type=radio]:checked')).toHaveValue('update');
	await expect(refresh).toBeChecked();
	await expect(refresh).toBeEnabled();
	await modal.locator('.acore-import-mode input[value=replace]').check();
	await expect(refresh).toBeDisabled();
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

	const editor = playerEditor(page);
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

test('Replace gives raiders their pet, ammo and consumables, and says where they came from', async ({ page }) => {
	const errors = watchForErrors(page);
	const dialogs = collectDialogs(page);
	await openRaid(page);

	const report = await importRoster(page, dialogs, LOADOUT);
	expect(report).toContain(
		[
			'Pets, ammo and consumables:',
			'  consumables from bot rules: Anvil, Fletcher, Burrower and 1 more',
			'  consumables from saved buffs and bags: Grimoire',
			'  consumables from bags: Frostfinger',
			'  Fletcher: Cat with its talents, Iceblade Arrow',
			'  Burrower: Worm, no talents; kept, the sim has no value for: ammo item 99999, flask (spell 67890, item 12345), potion (item 88888)',
			'  Grimoire: Succubus',
			'  Pactbound: kept, the sim has no value for: pet Doomguard (family 19)',
		].join('\n'),
	);

	await expect.poll(async () => (await savedRaider(page, 'Fletcher'))?.hunter?.options?.petType).toBe('Cat');
	const fletcher = await savedRaider(page, 'Fletcher');
	expect(fletcher.hunter.options.ammo).toBe('IcebladeArrow');
	expect(fletcher.hunter.options.petTalents).toMatchObject({ cobraReflexes: 2, bloodthirsty: 2, spidersBite: 3, callOfTheWild: true });
	// the hunter preset brings pet food, which a bot never uses
	expect(fletcher.consumes).toEqual({ flask: 'FlaskOfEndlessRage', food: 'FoodHeartyRhino', defaultPotion: 'PotionOfSpeed' });

	// nothing the sim has a value for: the preset's ammo, flask and potion stay, and so does the food the roster leaves out
	const burrower = await savedRaider(page, 'Burrower');
	expect(burrower.hunter.options.petType).toBe('Worm');
	expect(burrower.hunter.options.petTalents ?? {}).toEqual({});
	expect(burrower.hunter.options.ammo).toBe('SaroniteRazorheads');
	expect(burrower.consumes).toEqual({ flask: 'FlaskOfEndlessRage', food: 'FoodFishFeast', defaultPotion: 'PotionOfSpeed' });

	const grimoire = await savedRaider(page, 'Grimoire');
	expect(grimoire.warlock.options.summon).toBe('Succubus');
	expect(grimoire.consumes).toMatchObject({ prepopPotion: 'PotionOfWildMagic', thermalSapper: true, fillerExplosive: 'ExplosiveSaroniteBomb' });
	expect((await savedRaider(page, 'Pactbound')).warlock.options.summon).toBe('Felguard');
	expect(errors).toEqual([]);
});

test('Update always brings the pet and ammo, and consumables only with the box ticked', async ({ page }) => {
	const dialogs = collectDialogs(page);
	await openRaid(page);
	await importRoster(page, dialogs, LOADOUT);
	const changed = changedLoadout();

	let report = await updateWithConsumables(page, dialogs, changed, false);
	expect(report).toContain('  consumables kept as they were: Anvil, Fletcher, Burrower and 3 more');
	expect(report).toContain('  consumables from bot rules: Newcomer');
	await expect.poll(async () => (await savedRaider(page, 'Fletcher'))?.hunter?.options?.ammo).toBe('SaroniteRazorheads');
	expect((await savedRaider(page, 'Fletcher')).consumes.flask).toBe('FlaskOfEndlessRage');
	// new to the raid, so there's nothing of theirs to keep
	expect((await savedRaider(page, 'Newcomer')).consumes.flask).toBe('FlaskOfPureMojo');

	report = await updateWithConsumables(page, dialogs, changed, true);
	expect(report).not.toContain('kept as they were');
	await expect.poll(async () => (await savedRaider(page, 'Fletcher'))?.consumes?.flask).toBe('FlaskOfPureMojo');
});

test("Update keeps the roster's pet talents after the hunter's Edit window was open", async ({ page }) => {
	const dialogs = collectDialogs(page);
	await openRaid(page);
	await importRoster(page, dialogs, LOADOUT);
	// the Edit window's pet talents picker keeps listening after it closes
	await slot(page, rosterSlots(LOADOUT).indexOf('Burrower')).locator('.player-edit').click();
	const editor = playerEditor(page);
	await expect(editor).toBeVisible();
	await editor.locator('.close-button').click();
	await expect(editor).toHaveCount(0);

	const changed = JSON.parse(LOADOUT);
	changed.characters.find((char: any) => char.name == 'Burrower').pet = { name: 'Grub', family: 2, familyName: 'Cat', petType: 'Cat', talents: '' };
	await updateWithConsumables(page, dialogs, JSON.stringify(changed), true);
	await expect.poll(async () => (await savedRaider(page, 'Burrower'))?.hunter?.options?.petType).toBe('Cat');
	expect((await savedRaider(page, 'Burrower')).hunter.options.petTalents ?? {}).toEqual({});
});

test('a version 1 roster leaves pets, ammo and consumables alone', async ({ page }) => {
	const dialogs = collectDialogs(page);
	await openRaid(page);
	await importRoster(page, dialogs, LOADOUT);
	const before = await savedRaider(page, 'Fletcher');

	const oldFormat = JSON.parse(changedLoadout());
	oldFormat.version = 1;
	for (const char of oldFormat.characters) {
		delete char.bot;
		delete char.pet;
		delete char.ammo;
		delete char.consumes;
	}
	const report = await updateWithConsumables(page, dialogs, JSON.stringify(oldFormat), true);
	expect(report).toContain("Pets, ammo and consumables: a version 1 roster has none, so they're the sim's.");
	await expect.poll(() => roster(page)).toContain('Newcomer');
	const after = await savedRaider(page, 'Fletcher');
	expect(after.hunter.options).toEqual(before.hunter.options);
	expect(after.consumes).toEqual(before.consumes);

	const replaced = await importRoster(page, dialogs, ROSTER, 'replace');
	expect(replaced).toContain("Pets, ammo and consumables: a version 1 roster has none, so they're the sim's.");
});

test('the individual importer sets the pet, ammo and consumables', async ({ page }) => {
	const errors = watchForErrors(page);
	const dialogs = collectDialogs(page);
	await openSpec(page, 'hunter');
	const pick = async (rosterText: string, name: string) => {
		const seen = dialogs.length;
		const modal = await openImporter(page, 'AzerothCore');
		await paste(modal.locator('.importer-textarea'), rosterText);
		await modal.locator('.acore-character-select').selectOption(name);
		await modal.locator('.import-button').click();
		await expect.poll(() => dialogs.length).toBe(seen + 2);
		return dialogs[seen + 1];
	};
	const hunter = async () => (await storedSettings(page, 'hunter')).player;

	const note = await pick(LOADOUT, 'Fletcher');
	expect(note).toContain('Pet and ammo: Cat with its talents, Iceblade Arrow.');
	expect(note).toContain('Consumables from bot rules.');
	await expect.poll(async () => (await hunter()).hunter.options.petType).toBe('Cat');
	expect((await hunter()).hunter.options.ammo).toBe('IcebladeArrow');
	expect((await hunter()).hunter.options.petTalents).toMatchObject({ bloodthirsty: 2, callOfTheWild: true });
	expect((await hunter()).consumes).toMatchObject({ flask: 'FlaskOfEndlessRage', food: 'FoodHeartyRhino' });
	expect((await hunter()).consumes.petFood).toBeUndefined();

	await pick(LOADOUT, 'Burrower');
	await expect.poll(async () => (await hunter()).hunter.options.petType).toBe('Worm');
	expect((await hunter()).hunter.options.petTalents ?? {}).toEqual({});

	// Same empty talents on a pet from another tree: the talents picker would put its own set back
	const untrained = JSON.parse(LOADOUT);
	untrained.characters.find((char: any) => char.name == 'Fletcher').pet.talents = '';
	await pick(JSON.stringify(untrained), 'Fletcher');
	await expect.poll(async () => (await hunter()).hunter.options.petType).toBe('Cat');
	expect((await hunter()).hunter.options.petTalents ?? {}).toEqual({});
	expect(errors).toEqual([]);
});
