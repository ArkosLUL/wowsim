import { readFileSync } from 'node:fs';
import path from 'node:path';

import { test as base, expect, type Page } from '@playwright/test';

import { openSimTab, watchForErrors } from '../lib/page';
import { acceptDialogs, checkbox, icon, numberInput, openSpec, savedData, select, setNumber, storedSettings, stubWowheadTooltips } from './helpers';

const SPEC = 'mage';
const ROSTER = path.join(__dirname, '..', '..', '..', '..', 'ui', 'raid', 'acore_harness', 'testdata', 'raid.json');

// A copied text lands here instead of the clipboard, which headless Chrome won't always grant.
async function captureClipboard(page: Page) {
	await page.addInitScript(() => {
		(window as any).__copied = null;
		navigator.clipboard.writeText = async (text: string) => {
			(window as any).__copied = text;
		};
	});
}

// Moves a mage away from its defaults, so a round trip that drops something shows.
async function customize(page: Page) {
	await openSimTab(page, 'talents-tab');
	await savedData(page.locator('#talents-tab'), 'Saved Talents').chip('Arcane').locator('.saved-data-set-name').click();
	await openSimTab(page, 'settings-tab');
	const settings = page.locator('#settings-tab');
	await icon(settings.locator('.buffs-settings'), 'spell', 2825).click();
	await setNumber(numberInput(settings.locator('.encounter-settings'), 'Duration'), 222);
	await select(settings.locator('.player-settings'), 'Race').selectOption({ label: 'Gnome' });
	await checkbox(settings.locator('.player-settings'), 'Mining').setChecked(true);
	await openSimTab(page, 'rotation-tab');
	await savedData(page.locator('#rotation-tab'), 'Saved Rotations').chip('Arcane').locator('.saved-data-set-name').click();
}

async function openExporter(page: Page, name: string) {
	await page.locator('.export-dropdown .export-link').hover();
	await page.locator(`.export-dropdown .dropdown-item:text-is("${name}")`).click();
	const modal = page.locator('.modal.show');
	await expect(modal.locator('.exporter-textarea')).toBeVisible();
	return modal;
}

async function exportText(page: Page, name: string): Promise<string> {
	const modal = await openExporter(page, name);
	const text = await modal.locator('.exporter-textarea').inputValue();
	await modal.locator('.close-button').click();
	await expect(page.locator('.modal.show')).toHaveCount(0);
	return text;
}

async function openImporter(page: Page, name: string) {
	await page.locator('.import-dropdown .import-link').hover();
	await page.locator(`.import-dropdown .dropdown-item:text-is("${name}")`).click();
	const modal = page.locator('.modal.show');
	await expect(modal.locator('.importer-textarea')).toBeVisible();
	return modal;
}

async function importText(page: Page, name: string, text: string) {
	const modal = await openImporter(page, name);
	await modal.locator('.importer-textarea').fill(text);
	await modal.locator('.import-button').click();
	return modal;
}

// `other` is a page in a second browser profile, so nothing reaches it through localStorage.
const test = base.extend<{ other: Page }>({
	other: async ({ browser }, use) => {
		const context = await browser.newContext({ viewport: { width: 1600, height: 1100 } });
		await use(await context.newPage());
		await context.close();
	},
});

async function openLink(page: Page, link: string) {
	await stubWowheadTooltips(page);
	await page.goto(link);
	// Only the hash differs from the page already open, which doesn't load it again.
	await page.reload();
	await page.waitForLoadState('networkidle');
	await expect(page.locator('.sim-ui')).toBeVisible();
}

// The UI settings (iterations, EP weights and such) never go in a link.
function withoutUiSettings(settings: any) {
	const { settings: _ui, epWeightsStats: _ep, epRatios: _ratios, ...rest } = settings;
	return rest;
}

test('a sharable link opens with the same settings', async ({ page, other }) => {
	await openSpec(page, SPEC);
	await customize(page);
	const original = await storedSettings(page, SPEC);

	const link = await exportText(page, 'Link');

	await openLink(other, link);
	expect(withoutUiSettings(await storedSettings(other, SPEC))).toEqual(withoutUiSettings(original));
});

test('a link with only some categories leaves the rest alone', async ({ page, other }) => {
	await openSpec(page, SPEC);
	await customize(page);
	const original = await storedSettings(page, SPEC);

	const modal = await openExporter(page, 'Link');
	for (const category of ['Gear', 'Rotation', 'Consumes', 'Buffs & Debuffs', 'Misc', 'Encounter']) {
		await checkbox(modal, category).setChecked(false);
	}
	const link = await modal.locator('.exporter-textarea').inputValue();
	expect(new URL(link).searchParams.get('i')).toBe('t');

	await openSpec(other, SPEC);
	const defaults = await storedSettings(other, SPEC);
	await openLink(other, link);
	const imported = await storedSettings(other, SPEC);

	expect(imported.player.talentsString).toBe(original.player.talentsString);
	expect(imported.player.glyphs).toEqual(original.player.glyphs);
	expect(imported.encounter).toEqual(defaults.encounter);
	expect(imported.player.race).toBe(defaults.player.race);
});

test('a JSON export imports back to the same settings', async ({ page, other }) => {
	await openSpec(page, SPEC);
	await customize(page);
	const original = await storedSettings(page, SPEC);

	const json = await exportText(page, 'JSON');

	await openSpec(other, SPEC);
	await importText(other, 'JSON', json);
	await expect(other.locator('.modal.show')).toHaveCount(0);
	expect(await storedSettings(other, SPEC)).toEqual(original);
});

test('a downloaded JSON export uploads back to the same settings', async ({ page, other }, testInfo) => {
	await openSpec(page, SPEC);
	await customize(page);
	const original = await storedSettings(page, SPEC);

	const modal = await openExporter(page, 'JSON');
	const download = page.waitForEvent('download');
	await modal.locator('.download-button').click();
	const file = testInfo.outputPath((await download).suggestedFilename());
	await (await download).saveAs(file);

	await openSpec(other, SPEC);
	const importer = await openImporter(other, 'JSON');
	await importer.locator('.importer-upload-input').setInputFiles(file);
	await importer.locator('.import-button').click();
	await expect(other.locator('.modal.show')).toHaveCount(0);
	expect(await storedSettings(other, SPEC)).toEqual(original);
});

test('a Wowhead export imports back to the same gear, talents, glyphs and race', async ({ page, other }) => {
	await openSpec(page, SPEC);
	await customize(page);
	const original = await storedSettings(page, SPEC);

	const url = await exportText(page, 'WoWHead');
	expect(url).toContain('/gear-planner/mage/gnome/');

	const dialogs = acceptDialogs(other);
	await openSpec(other, SPEC);
	await importText(other, 'WoWHead', url);
	await expect.poll(() => dialogs).toContain('Import successful!');

	const imported = (await storedSettings(other, SPEC)).player;
	expect(imported.race).toBe(original.player.race);
	expect(imported.talentsString).toBe(original.player.talentsString);
	expect(imported.glyphs).toEqual(original.player.glyphs);
	expect(imported.equipment).toEqual(original.player.equipment);
});

test('an addon export imports gear, talents, race and professions', async ({ page, other }) => {
	await openSpec(page, SPEC);
	await customize(page);
	const original = (await storedSettings(page, SPEC)).player;

	const addon = {
		class: 'mage',
		race: 'Gnome',
		professions: [
			{ name: 'Mining', level: 450 },
			{ name: 'Tailoring', level: 450 },
		],
		talents: original.talentsString,
		glyphs: { major: [], minor: [] },
		gear: { items: original.equipment.items },
	};

	const dialogs = acceptDialogs(other);
	await openSpec(other, SPEC);
	await importText(other, 'Addon', JSON.stringify(addon));
	await expect.poll(() => dialogs).toContain('Import successful!');

	const imported = (await storedSettings(other, SPEC)).player;
	expect(imported.race).toBe('RaceGnome');
	expect(imported.talentsString).toBe(original.talentsString);
	expect(imported.equipment).toEqual(original.equipment);
	expect(imported.professions).toEqual(['Mining', 'Tailoring']);
});

test('an 80 Upgrades export imports gear, talents and race', async ({ page }) => {
	const dialogs = acceptDialogs(page);
	await openSpec(page, SPEC);
	const original = (await storedSettings(page, SPEC)).player;
	const items = original.equipment.items.filter((item: any) => item.id);

	const export80U = {
		character: { gameClass: 'MAGE', race: 'GNOME', level: 80 },
		items: items.map((item: any) => ({
			id: item.id,
			enchant: item.enchant ? { id: item.enchant } : undefined,
			gems: (item.gems || []).map((id: number) => ({ id })),
		})),
		talents: [],
	};
	await importText(page, '80U', JSON.stringify(export80U));
	await expect.poll(() => dialogs).toContain('Import successful!');

	const imported = (await storedSettings(page, SPEC)).player;
	expect(imported.race).toBe('RaceGnome');
	expect(imported.equipment.items.map((item: any) => item.id).filter(Boolean)).toEqual(items.map((item: any) => item.id));
});

test('every exporter shows its title and closes', async ({ page }) => {
	await openSpec(page, SPEC);
	const titles: Record<string, string> = {
		Link: 'Sharable Link',
		JSON: 'JSON Export',
		WoWHead: 'Wowhead Export',
		'80U EP': '80Upgrades EP Export',
		'Pawn EP': 'Pawn EP Export',
		CLI: 'CLI Export',
	};

	for (const [name, title] of Object.entries(titles)) {
		const modal = await openExporter(page, name);
		await expect(modal.locator('.modal-title')).toHaveText(title);
		await modal.locator('.close-button').click();
		await expect(page.locator('.modal.show')).toHaveCount(0);
	}
});

test('an addon export with an unknown profession names it', async ({ page }) => {
	const dialogs = acceptDialogs(page);
	await openSpec(page, SPEC);

	const addon = { class: 'mage', race: 'Troll', professions: [{ name: 'Fishing', level: 450 }], talents: '', glyphs: { major: [], minor: [] }, gear: { items: [] } };
	await importText(page, 'Addon', JSON.stringify(addon));
	await expect.poll(() => dialogs).toEqual([`Import error:\nCould not parse profession 'Fishing'`]);
});

const WRONG_CLASS = 'Wrong Class! Expected Mage but found Warrior!';

test('importing another class is refused with a message', async ({ page }) => {
	const dialogs = acceptDialogs(page);
	await openSpec(page, SPEC);
	const before = await storedSettings(page, SPEC);

	const addon = { class: 'warrior', race: 'Human', professions: [], talents: '', glyphs: { major: [], minor: [] }, gear: { items: [] } };
	const modal = await importText(page, 'Addon', JSON.stringify(addon));
	await expect.poll(() => dialogs).toEqual([`Import error:\n${WRONG_CLASS}`]);
	await modal.locator('.close-button').click();

	await importText(page, '80U', JSON.stringify({ character: { gameClass: 'WARRIOR', race: 'HUMAN', level: 80 }, items: [], talents: [] }));
	await expect.poll(() => dialogs.length).toBe(2);
	expect(dialogs[1]).toBe(`Import error:\n${WRONG_CLASS}`);

	expect(await storedSettings(page, SPEC)).toEqual(before);
});

test('a Wowhead link for another class is refused with a message', async ({ page }) => {
	const dialogs = acceptDialogs(page);
	await openSpec(page, SPEC);

	const url = (await exportText(page, 'WoWHead')).replace('/gear-planner/mage/', '/gear-planner/warrior/').replace(/\/troll\/|\/gnome\//, '/human/');
	await importText(page, 'WoWHead', url);
	await expect.poll(() => dialogs).toEqual([`Import error:\n${WRONG_CLASS}`]);
});

test('the AzerothCore importer brings over the picked character', async ({ page }) => {
	const errors = watchForErrors(page);
	const dialogs = acceptDialogs(page);
	const character = JSON.parse(readFileSync(ROSTER, 'utf-8')).characters.find((c: any) => c.name == 'Smartface');
	await openSpec(page, SPEC);

	const modal = await openImporter(page, 'AzerothCore');
	await modal.locator('.importer-upload-input').setInputFiles(ROSTER);
	const picker = modal.locator('.acore-character-select');
	await expect(picker.locator('option')).toHaveText(['Power', 'Smartface']);
	await picker.selectOption('Smartface');
	await modal.locator('.import-button').click();
	await expect.poll(() => dialogs).toContain('Import successful!');

	const imported = (await storedSettings(page, SPEC)).player;
	expect(imported.talentsString).toBe(character.talents);
	expect(imported.race).toBe('RaceGnome');
	expect(imported.equipment.items[0].id).toBe(character.gear.find((g: any) => g.acSlot == 0).id);
	expect(imported.professions).toHaveLength(character.professions.length);
	expect(errors).toEqual([]);
});

test('the AzerothCore importer says so when the roster has no one of this class', async ({ page }) => {
	const dialogs = acceptDialogs(page);
	const roster = JSON.parse(readFileSync(ROSTER, 'utf-8'));
	roster.characters = roster.characters.filter((c: any) => c.classId != 8);
	await openSpec(page, SPEC);

	const modal = await openImporter(page, 'AzerothCore');
	await modal.locator('.importer-upload-input').setInputFiles({ name: 'raid.json', mimeType: 'application/json', buffer: Buffer.from(JSON.stringify(roster)) });
	await expect(modal.locator('.acore-character-select option')).toHaveText(['No Mage in this roster']);
	await modal.locator('.import-button').click();
	await expect.poll(() => dialogs).toEqual(['Import error:\nThis roster has no Mage in it.']);
});

test('the CLI export is the sim request for this page', async ({ page }) => {
	await openSpec(page, SPEC);
	const settings = await storedSettings(page, SPEC);

	const request = JSON.parse(await exportText(page, 'CLI'));
	const player = request.raid.parties[0].players[0];
	expect(player.class).toBe('ClassMage');
	expect(player.talentsString).toBe(settings.player.talentsString);
	expect(request.encounter.duration).toBe(settings.encounter.duration);
});

test('the EP exports carry the current weights, and Copy copies them as shown', async ({ page }) => {
	await captureClipboard(page);
	await openSpec(page, SPEC);

	for (const name of ['80U EP', 'Pawn EP']) {
		const modal = await openExporter(page, name);
		const text = await modal.locator('.exporter-textarea').inputValue();
		expect(text).toMatch(name == 'Pawn EP' ? /Class=Mage,.*SpellDamage=\d/ : /[?&]spellDamage=\d/);

		await modal.locator('.copy-button, button:has-text("Copy")').first().click();
		await expect.poll(() => page.evaluate(() => (window as any).__copied)).toBe(text);
		await modal.locator('.close-button').click();
		await expect(page.locator('.modal.show')).toHaveCount(0);
	}
});

test('talents copy as their talent string', async ({ page }) => {
	await captureClipboard(page);
	await openSpec(page, SPEC, 'talents-tab');

	await page.locator('#talents-tab .copy-talents').click();
	const settings = await storedSettings(page, SPEC);
	await expect.poll(() => page.evaluate(() => (window as any).__copied)).toBe(settings.player.talentsString);
});

test('a broken link opens the page with the settings it already had', async ({ page }) => {
	const errors = watchForErrors(page);
	await openSpec(page, SPEC, 'settings-tab');
	await setNumber(numberInput(page.locator('#settings-tab .encounter-settings'), 'Duration'), 199);
	const before = await storedSettings(page, SPEC);

	await openLink(page, `${page.url().split('#')[0]}#not-a-real-link`);

	expect(await storedSettings(page, SPEC)).toEqual(before);
	expect(errors).toEqual([]);
});
