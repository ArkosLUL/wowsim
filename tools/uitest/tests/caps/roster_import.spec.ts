import { expect, type Locator, type Page, test } from '@playwright/test';

import { errorReply, NOT_CONFIGURED, stubServer } from '../lib/acore';
import { openSimTab, watchForErrors } from '../lib/page';
import { openImporter, paste, readRoster, rosterPath } from '../raid/raid';
import { acceptDialogs, openSpec, reloadSim, savedData, storageKey, storedSettings } from '../settings/helpers';

const SPEC = 'deathknight';
const ROSTER = readRoster();
const rosterJson = JSON.parse(ROSTER);
const SERVER_STATUS = `From the server, exported ${rosterJson.exportedAt}.`;

// The roster's death knights: Deathsong plays unholy DPS, Obliteration frost DPS, Felesta tanks.
const DEATH_KNIGHTS = ['Deathsong', 'Obliteration', 'Felesta'];

const character = (name: string) => rosterJson.characters.find((char: any) => char.name == name);

// raid.json with some characters moved to another subgroup
function regrouped(moves: Record<string, number>): string {
	const changed = JSON.parse(ROSTER);
	changed.characters.forEach((char: any) => (char.subgroup = moves[char.name] ?? char.subgroup));
	return JSON.stringify(changed);
}

const picker = (modal: Locator) => modal.locator('.acore-character-select');

// Imports whoever the dialog picked off the server's roster, and returns the notes the Caps tab shows. No alert.
async function importFromServer(page: Page, dialogs: string[], rosterText: string): Promise<string> {
	const modal = await openImporter(page, 'AzerothCore');
	await expect(modal.locator('.importer-textarea')).toHaveValue(rosterText);
	await modal.locator('.import-button').click();
	await expect(modal).toHaveCount(0, { timeout: 60_000 });
	expect(dialogs).toEqual([]);
	return notesOnTab(page);
}

async function notesOnTab(page: Page): Promise<string> {
	await openSimTab(page, 'caps-tab');
	const notes = page.locator('#caps-tab .caps-import-notes');
	await expect(notes).toContainText('Loaded ');
	return notes.innerText();
}

const heroicPresence = async (page: Page) => (await storedSettings(page, SPEC)).partyBuffs?.heroicPresence ?? false;

test('the dialog opens on the server roster with this spec on this talent tree picked', async ({ page }) => {
	const errors = watchForErrors(page);
	const server = await stubServer(page, { body: ROSTER });
	await openSpec(page, SPEC);

	let modal = await openImporter(page, 'AzerothCore');
	await expect(modal.locator('.import-description')).toContainText('saveall');
	await expect(modal.locator('.importer-textarea')).toHaveValue(ROSTER);
	await expect(modal.locator('.acore-server-status')).toHaveText(SERVER_STATUS);
	await expect(picker(modal).locator('option')).toHaveText(DEATH_KNIGHTS);
	await expect(picker(modal)).toHaveValue('Deathsong');
	await expect(modal.locator('.fetch-button')).toBeEnabled();
	await modal.locator('.close-button').click();
	await expect(modal).toHaveCount(0);

	// both DPS death knights fit the page's spec, so its talent tree decides
	await openSimTab(page, 'talents-tab');
	await savedData(page.locator('#talents-tab'), 'Saved Talents').chip('Frost BL').locator('.saved-data-set-name').click();
	modal = await openImporter(page, 'AzerothCore');
	await expect(modal.locator('.importer-textarea')).toHaveValue(ROSTER);
	await expect(picker(modal)).toHaveValue('Obliteration');

	expect(server.requests).toHaveLength(2);
	expect(errors).toEqual([]);
});

test('the pick survives a reload, while that raider is still in the roster', async ({ page }) => {
	const server = await stubServer(page, { body: ROSTER });
	await openSpec(page, SPEC);

	let modal = await openImporter(page, 'AzerothCore');
	await expect(picker(modal)).toHaveValue('Deathsong');
	await picker(modal).selectOption('Felesta');
	expect(await page.evaluate(key => localStorage.getItem(key), storageKey(SPEC, 'acore-import.v1'))).toBe(JSON.stringify({ character: 'Felesta' }));

	await reloadSim(page);
	modal = await openImporter(page, 'AzerothCore');
	await expect(modal.locator('.importer-textarea')).toHaveValue(ROSTER);
	await expect(picker(modal)).toHaveValue('Felesta');
	await modal.locator('.close-button').click();
	await expect(modal).toHaveCount(0);

	const withoutFelesta = JSON.parse(ROSTER);
	withoutFelesta.characters = withoutFelesta.characters.filter((char: any) => char.name != 'Felesta');
	server.reply = { body: JSON.stringify(withoutFelesta) };
	modal = await openImporter(page, 'AzerothCore');
	await expect(modal.locator('.importer-textarea')).toHaveValue(JSON.stringify(withoutFelesta));
	await expect(picker(modal)).toHaveValue('Deathsong');
});

test('Import brings over the gear and talents, and Heroic Presence follows the party', async ({ page }) => {
	const errors = watchForErrors(page);
	const dialogs = acceptDialogs(page);
	const server = await stubServer(page, { body: ROSTER });
	await openSpec(page, SPEC);
	expect(await heroicPresence(page)).toBe(false);

	const deathsong = character('Deathsong');
	let notes = await importFromServer(page, dialogs, ROSTER);
	expect(notes).toContain("Heroic Presence on: Obliteration in Deathsong's party has Draenei racials.");
	const imported = (await storedSettings(page, SPEC)).player;
	expect(imported.talentsString).toBe(deathsong.talents);
	expect(imported.race).toBe('RaceNightElf');
	expect(imported.racialTraits).toBe('RaceOrc');
	expect(imported.equipment.items[0].id).toBe(deathsong.gear.find((item: any) => item.acSlot == 0).id);
	expect(Object.values(imported.glyphs).filter(Boolean)).toHaveLength([...deathsong.glyphs.major, ...deathsong.glyphs.minor].filter(Boolean).length);
	expect(await heroicPresence(page)).toBe(true);

	// Felesta is a Draenei with Orc racials, which give none
	let rosterText = regrouped({ Obliteration: 1, Felesta: 0 });
	server.reply = { body: rosterText };
	notes = await importFromServer(page, dialogs, rosterText);
	expect(notes).toContain("Heroic Presence off: nobody in Deathsong's party has Draenei racials.");
	expect(await heroicPresence(page)).toBe(false);

	// Druidica is a Night Elf with Draenei racials
	rosterText = regrouped({ Obliteration: 1, Druidica: 0 });
	server.reply = { body: rosterText };
	notes = await importFromServer(page, dialogs, rosterText);
	expect(notes).toContain("Heroic Presence on: Druidica in Deathsong's party has Draenei racials.");
	expect(await heroicPresence(page)).toBe(true);
	expect(errors).toEqual([]);
});

test('a page that runs the sim in the browser never asks the server', async ({ page }) => {
	// Only the page's own look at the worker file is swapped, so the real worker still loads.
	await page.route('**/sim_worker.js', route =>
		route.request().resourceType() == 'fetch' ? route.fulfill({ contentType: 'text/javascript', body: '// wasm worker' }) : route.fallback(),
	);
	const server = await stubServer(page, { body: ROSTER });
	await openSpec(page, SPEC);

	const looked = page.waitForResponse('**/sim_worker.js');
	const modal = await openImporter(page, 'AzerothCore');
	await (await looked).finished();
	// the fetch would only start a moment after the answer
	await page.waitForTimeout(250);
	await expect(modal.locator('.import-button')).toBeVisible();
	await expect(modal.locator('.fetch-button')).toHaveCount(0);
	await expect(modal.locator('.acore-fetch-hint')).toHaveCount(0);
	await expect(modal.locator('.acore-server-status')).toBeHidden();
	await expect(modal.locator('.importer-textarea')).toHaveValue('');
	expect(server.requests).toEqual([]);
});

test('a roster file still imports when the server refuses', async ({ page }) => {
	const dialogs = acceptDialogs(page);
	await stubServer(page, errorReply(503, 'not_configured', NOT_CONFIGURED));
	await openSpec(page, SPEC);

	const modal = await openImporter(page, 'AzerothCore');
	await expect(modal.locator('.acore-server-status')).toHaveText(`Couldn't read the roster off the server: ${NOT_CONFIGURED}`);
	await expect(modal.locator('.importer-textarea')).toHaveValue('');
	expect(dialogs).toEqual([]);

	await modal.locator('.importer-upload-input').setInputFiles(rosterPath());
	await expect(picker(modal)).toHaveValue('Deathsong');
	await modal.locator('.import-button').click();
	await expect(modal).toHaveCount(0, { timeout: 60_000 });
	expect(await notesOnTab(page)).toContain('Heroic Presence on');
	expect(dialogs).toEqual([]);
	expect((await storedSettings(page, SPEC)).player.talentsString).toBe(character('Deathsong').talents);
});

test('a roster added before the server answers stays, until Fetch from server', async ({ page }) => {
	let answer!: () => void;
	const held = new Promise<void>(resolve => (answer = resolve));
	await stubServer(page, { body: ROSTER, after: held });
	await openSpec(page, SPEC);

	const modal = await openImporter(page, 'AzerothCore');
	const button = modal.locator('.fetch-button');
	await expect(button).toBeDisabled();
	const file = JSON.parse(ROSTER);
	file.characters = file.characters.filter((char: any) => char.name != 'Deathsong');
	await modal.locator('.importer-upload-input').setInputFiles({ name: 'raid.json', mimeType: 'application/json', buffer: Buffer.from(JSON.stringify(file)) });
	await expect(picker(modal).locator('option')).toHaveText(['Obliteration', 'Felesta']);

	answer();
	await expect(button).toBeEnabled();
	await expect(modal.locator('.acore-server-status')).toHaveText("Kept the roster you added. 'Fetch from server' swaps in the server's.");
	await expect(modal.locator('.importer-textarea')).toHaveValue(JSON.stringify(file));

	await button.click();
	await expect(modal.locator('.importer-textarea')).toHaveValue(ROSTER);
	await expect(modal.locator('.acore-server-status')).toHaveText(SERVER_STATUS);
	await expect(picker(modal).locator('option')).toHaveText(DEATH_KNIGHTS);
	// the one already picked is still there, so it stays picked
	await expect(picker(modal)).toHaveValue('Obliteration');
});

test('a textarea emptied before the server answers takes its roster', async ({ page }) => {
	let answer!: () => void;
	const held = new Promise<void>(resolve => (answer = resolve));
	await stubServer(page, { body: ROSTER, after: held });
	await openSpec(page, SPEC);

	const modal = await openImporter(page, 'AzerothCore');
	const textarea = modal.locator('.importer-textarea');
	await paste(textarea, '{"version": 2}');
	await paste(textarea, '');
	answer();
	await expect(modal.locator('.acore-server-status')).toHaveText(SERVER_STATUS);
	await expect(textarea).toHaveValue(ROSTER);
	await expect(picker(modal)).toHaveValue('Deathsong');
});

// Unstubbed: runs only with UITEST_ACORE_LIVE=1, against a sim server started with the --acore* flags.
// It reads the AzerothCore database and writes nothing.
test('the real route fills the dialog and imports the picked raider', async ({ page }) => {
	test.skip(!process.env.UITEST_ACORE_LIVE, 'needs a sim server that reads a live AzerothCore database');
	const errors = watchForErrors(page);
	const dialogs = acceptDialogs(page);
	await openSpec(page, SPEC);

	const reply = page.waitForResponse(/\/acoreRoster$/);
	const modal = await openImporter(page, 'AzerothCore');
	const live = await (await reply).text();
	await expect(modal.locator('.importer-textarea')).toHaveValue(live);
	await expect(modal.locator('.acore-server-status')).toHaveText(`From the server, exported ${JSON.parse(live).exportedAt}.`);
	const deathKnights = JSON.parse(live)
		.characters.filter((char: any) => char.classId == 6)
		.map((char: any) => char.name);
	expect(deathKnights.length).toBeGreaterThan(0);
	await expect(picker(modal).locator('option')).toHaveText(deathKnights);
	const picked = await picker(modal).inputValue();

	await modal.locator('.import-button').click();
	await expect(modal).toHaveCount(0, { timeout: 60_000 });
	expect(dialogs).toEqual([]);
	expect(await notesOnTab(page)).toMatch(new RegExp(`^Loaded ${picked}[\\s\\S]*Heroic Presence (on|off)`));
	expect((await storedSettings(page, SPEC)).player.talentsString).toBe(JSON.parse(live).characters.find((char: any) => char.name == picked).talents);
	expect(errors).toEqual([]);
});
