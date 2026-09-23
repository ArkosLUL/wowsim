import { readFileSync } from 'node:fs';
import path from 'node:path';

import { expect, type Locator, type Page } from '@playwright/test';

import { loadFixture } from '../lib/fixture';
import { openRaid } from '../lib/page';

const exactly = (text: string) => new RegExp(`^\\s*${text}\\s*$`);

export const SETTINGS_KEY = '__wotlk_raid____currentSettings__';

const ROSTER_DIR = path.join(__dirname, '..', '..', '..', '..', 'ui', 'raid', 'acore_harness', 'testdata');

// An acraid export from ui/raid/acore_harness/testdata: 'raid', or one of its variants.
export const rosterPath = (name = 'raid') => path.join(ROSTER_DIR, `${name}.json`);

// with plain newlines, as a textarea hands them back
export function readRoster(name = 'raid'): string {
	return readFileSync(rosterPath(name), 'utf-8').replace(/\r\n/g, '\n');
}

// Pastes the text in one go: typing a 5000-line roster through fill() takes most of a test's time.
export async function paste(textarea: Locator, text: string) {
	await textarea.evaluate((elem, value) => {
		(elem as HTMLTextAreaElement).value = value;
		elem.dispatchEvent(new Event('input', { bubbles: true }));
	}, text);
}

// Who the importer seats where: each subgroup fills its party in roster order.
export function rosterSlots(rosterText: string): Array<string> {
	const slots: Array<string> = new Array(25).fill('');
	const filled: Record<number, number> = {};
	for (const char of JSON.parse(rosterText).characters) {
		const position = filled[char.subgroup] ?? 0;
		filled[char.subgroup] = position + 1;
		// no room past the fifth: the importer skips them
		if (position < 5) {
			slots[char.subgroup * 5 + position] = char.name;
		}
	}
	return slots;
}

// memberFlags 2: the main tank flag the raid leader sets in game
export const rosterMainTank = (rosterText: string): string => JSON.parse(rosterText).characters.find((char: any) => char.memberFlags & 2).name;

// The raid25 fixture's raid and encounter, shaped like the settings the raid page saves.
export function fixtureSettings(): any {
	const request = loadFixture().run.request;
	return { raid: request.raid, encounter: request.encounter };
}

export function fixtureRoster(): Array<string> {
	return fixtureSettings().raid.parties.flatMap((party: any) => party.players.map((player: any) => player.name ?? ''));
}

export async function reloadRaid(page: Page) {
	await page.reload();
	await page.waitForLoadState('networkidle');
	await expect(page.locator('.sim-ui')).toBeVisible();
}

// Opens the raid page with these settings already saved, as if a raider had built the raid before.
export async function seedRaid(page: Page, settings: any = fixtureSettings()) {
	await openRaid(page);
	await page.evaluate(([key, value]) => localStorage.setItem(key, value), [SETTINGS_KEY, JSON.stringify(settings)]);
	await reloadRaid(page);
	await expect(slot(page, 0).locator('.player-name')).toBeVisible();
}

export async function savedSettings(page: Page): Promise<any> {
	return JSON.parse((await page.evaluate(key => localStorage.getItem(key), SETTINGS_KEY)) || '{}');
}

// A raid slot on the Raid tab, 0 to 39 in group order.
export const slot = (page: Page, raidIndex: number) => page.locator('.party-picker-root .player-picker-root').nth(raidIndex);

// The name in each of the first `size` slots, '' where the slot is empty.
export async function roster(page: Page, size = 25): Promise<Array<string>> {
	const names = await page
		.locator('.party-picker-root .player-picker-root')
		.evaluateAll(slots => slots.map(elem => (elem.querySelector('.player-name') as HTMLInputElement | null)?.value ?? ''));
	return names.slice(0, size);
}

// A raid stats category's counter, e.g. 'Tanks' or 'Bloodlust'.
export async function statCount(page: Page, label: string): Promise<number> {
	const category = page.locator('.raid-stats-category', { has: page.locator('.raid-stats-category-label', { hasText: exactly(label) }) });
	return Number(await category.locator('.raid-stats-category-counter').innerText());
}

// The presets' tooltips are their spec names, e.g. 'Protection Paladin'.
export async function addPreset(page: Page, preset: string, raidIndex: number) {
	await page.locator(`.new-player-picker-root a[data-bs-title="${preset}"]`).dragTo(slot(page, raidIndex));
}

// Dragging a raider's icon onto another slot swaps the two.
export async function swapPlayers(page: Page, from: number, to: number) {
	await slot(page, from).locator('.player-icon').dragTo(slot(page, to));
}

export async function copyPlayer(page: Page, from: number, to: number) {
	await slot(page, from).locator('.player-copy').dragTo(slot(page, to));
}

export async function removePlayer(page: Page, raidIndex: number) {
	await slot(page, raidIndex).locator('.player-delete').click();
}

export async function renamePlayer(page: Page, raidIndex: number, name: string) {
	const input = slot(page, raidIndex).locator('.player-name');
	await input.fill(name);
	await input.blur();
}

// The name a raid target picker shows, or 'Unassigned'.
export async function targetName(picker: Locator): Promise<string> {
	return (await picker.locator('.raid-target-picker-button').innerText()).trim();
}

// Dropdowns here open on hover (ui/shared/bootstrap_overrides.ts), and a click on the toggle closes
// them again.
export async function pickTarget(picker: Locator, name: string) {
	await picker.locator('.raid-target-picker-button').hover();
	await picker.locator('.dropdown-option', { hasText: exactly(name) }).click();
}

export const tankPickers = (page: Page) => page.locator('.tanks-picker-root .tank-picker');

export async function tankNames(page: Page): Promise<Array<string>> {
	return (await tankPickers(page).locator('.raid-target-picker-button').allInnerTexts()).map(name => name.trim());
}

// The target picker of one external buff, e.g. 'Innervate' from 'Balance'.
export function assignmentPicker(page: Page, buff: string, source: string): Locator {
	return page
		.locator('.assigned-buff-picker-root', { has: page.locator('.assignmented-buff-label', { hasText: buff }) })
		.locator('.assigned-buff-player', {
			has: page.locator('.raid-target-picker-root:not(.assigned-buff-target-picker) .player-name', { hasText: exactly(source) }),
		})
		.locator('.assigned-buff-target-picker');
}

export async function openImporter(page: Page, label: string): Promise<Locator> {
	await page.locator('.import-link').hover();
	await page.locator('.import-dropdown .dropdown-item', { hasText: exactly(label) }).click();
	const modal = page.locator('.modal.show', { has: page.locator('.importer') });
	await expect(modal).toBeVisible();
	return modal;
}

export async function exportJson(page: Page): Promise<string> {
	await page.locator('.export-link').hover();
	await page.locator('.export-dropdown .dropdown-item', { hasText: exactly('JSON') }).click();
	const modal = page.locator('.modal.show', { has: page.locator('.exporter') });
	await expect(modal.locator('.modal-title')).toHaveText('JSON Export');
	const text = await modal.locator('.exporter-textarea').inputValue();
	await modal.locator('.close-button').click();
	await expect(modal).toHaveCount(0);
	return text;
}

// Playwright dismisses every dialog by default, which answers each confirm with no. This accepts
// them instead and keeps their text.
export function collectDialogs(page: Page): Array<string> {
	const messages: Array<string> = [];
	page.on('dialog', dialog => {
		messages.push(dialog.message());
		dialog.accept();
	});
	return messages;
}

// Runs the AzerothCore importer on an acraid export and returns its report.
export async function importRoster(page: Page, dialogs: Array<string>, rosterText: string, mode: 'replace' | 'update' = 'replace'): Promise<string> {
	const seen = dialogs.length;
	const modal = await openImporter(page, 'AzerothCore');
	await modal.locator(`.acore-import-mode input[value=${mode}]`).check();
	await paste(modal.locator('.importer-textarea'), rosterText);
	await modal.locator('.import-button').click();
	await expect.poll(() => dialogs.length, { timeout: 60_000 }).toBeGreaterThan(seen);
	await expect(modal).toHaveCount(0);
	return dialogs[seen];
}
