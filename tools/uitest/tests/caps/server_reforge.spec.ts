import { expect, type Locator, type Page, test } from '@playwright/test';

import { stubServer } from '../lib/acore';
import { openSimTab, watchForErrors } from '../lib/page';
import { openImporter, readRoster } from '../raid/raid';
import { acceptDialogs, openSpec, storageKey } from '../settings/helpers';

const SPEC = 'deathknight';
const rosterJson = JSON.parse(readRoster());

// Deathsong's neck carries a reforge (acSlot 1), his chest too (4), his head (0), shoulders (2) and belt (5) none
const HEAD = 0;
const NECK = 1;
const SHOULDERS = 2;
const CHEST = 4;
const BELT = 5;
const MAIN_HAND = 15;

interface Posted {
	character: string;
	items: Array<{ acSlot: number; itemId: number; from: number; to: number }>;
}

// Stands in for /acoreReforge: records the bodies, answers `reply`.
async function stubReforge(page: Page, reply: { status?: number; body: unknown } | ((posted: Posted) => unknown)) {
	const posted: Array<Posted> = [];
	await page.route(/\/acoreReforge$/, async route => {
		const body = route.request().postDataJSON() as Posted;
		posted.push(body);
		const answer = typeof reply == 'function' ? { body: reply(body) } : reply;
		await route.fulfill({ status: answer.status ?? 200, contentType: 'application/json', body: JSON.stringify(answer.body) });
	});
	return posted;
}

// The roster with Deathsong's gear changed: crit into haste on the head, a reforge from crit on the
// shoulders (which have none, so the sim can't take it), no reforge on the neck, another chest, no main hand.
function changedRoster(): string {
	const changed = JSON.parse(JSON.stringify(rosterJson));
	const gear: Array<any> = changed.characters.find((char: any) => char.name == 'Deathsong').gear;
	gear.find(item => item.acSlot == HEAD).reforge = { fromStatType: 32, toStatType: 36 };
	gear.find(item => item.acSlot == SHOULDERS).reforge = { fromStatType: 32, toStatType: 37 };
	delete gear.find(item => item.acSlot == NECK).reforge;
	gear.find(item => item.acSlot == CHEST).id = 99999;
	gear.splice(gear.findIndex(item => item.acSlot == MAIN_HAND), 1);
	return JSON.stringify(changed);
}

// Puts Deathsong's server gear on the page, then returns the roster stub to change.
async function importDeathsong(page: Page) {
	const dialogs = acceptDialogs(page);
	const server = await stubServer(page, { body: JSON.stringify(rosterJson) });
	await openSpec(page, SPEC);
	const modal = await openImporter(page, 'AzerothCore');
	await expect(modal.locator('.acore-character-select')).toHaveValue('Deathsong');
	await modal.locator('.import-button').click();
	await expect.poll(() => dialogs.length, { timeout: 60_000 }).toBe(2);
	await expect(modal).toHaveCount(0);
	return server;
}

async function openDialog(page: Page): Promise<Locator> {
	await openSimTab(page, 'caps-tab');
	await page.locator('#caps-tab .caps-server-reforge').click();
	const dialog = page.locator('.modal.show', { has: page.locator('.acore-reforge-dialog') });
	await expect(dialog).toBeVisible();
	return dialog;
}

const row = (dialog: Locator, acSlot: number) => dialog.locator(`tr[data-ac-slot="${acSlot}"]`);

test('the dialog preselects the importer pick and sorts each slot into change, same or left out', async ({ page }) => {
	const errors = watchForErrors(page);
	const server = await importDeathsong(page);
	server.reply = { body: changedRoster() };

	const dialog = await openDialog(page);
	const picker = dialog.locator('.acore-reforge-character');
	await expect(picker.locator('option')).toHaveText(['Deathsong', 'Obliteration', 'Felesta']);
	await expect(picker).toHaveValue('Deathsong');
	expect(await page.evaluate(key => localStorage.getItem(key), storageKey(SPEC, 'acore-import.v1'))).toBe(JSON.stringify({ character: 'Deathsong' }));

	await expect(row(dialog, NECK).locator('td').nth(2)).toHaveText('none');
	await expect(row(dialog, NECK).locator('td').nth(3)).not.toHaveText('none');
	await expect(row(dialog, NECK).locator('.acore-reforge-row-status')).toHaveText('Will change');
	await expect(row(dialog, BELT).locator('.acore-reforge-row-status')).toHaveText('Same');
	await expect(row(dialog, HEAD).locator('.acore-reforge-row-status')).toHaveText('Will remove');
	await expect(row(dialog, SHOULDERS).locator('td').nth(2)).toHaveText('Crit → Expertise');
	await expect(row(dialog, SHOULDERS).locator('.acore-reforge-row-status')).toHaveText("Left out: the sim's copy of the item can't take the server's reforge");
	await expect(row(dialog, CHEST).locator('.acore-reforge-row-status')).toHaveText('Different item on the server, left out');
	await expect(row(dialog, MAIN_HAND).locator('.acore-reforge-row-status')).toContainText('nothing equipped on the server');
	expect(errors).toEqual([]);
});

test('Send posts every slot with the same item and shows each slot\'s answer', async ({ page }) => {
	const errors = watchForErrors(page);
	const server = await importDeathsong(page);
	server.reply = { body: changedRoster() };
	const posted = await stubReforge(page, body => ({
		online: false,
		slots: body.items.map(item => ({
			acSlot: item.acSlot,
			status: item.acSlot == NECK ? 'applied' : 'unchanged',
			text: item.acSlot == NECK ? 'Crit -> Expertise' : 'same reforge',
		})),
	}));

	const dialog = await openDialog(page);
	await dialog.locator('.acore-reforge-send').click();
	await expect(row(dialog, NECK).locator('.acore-reforge-row-status')).toHaveText('applied: Crit -> Expertise');
	await expect(row(dialog, BELT).locator('.acore-reforge-row-status')).toHaveText('unchanged: same reforge');
	await expect(row(dialog, CHEST).locator('.acore-reforge-row-status')).toHaveText('Different item on the server, left out');
	await expect(dialog.locator('.acore-reforge-status')).toContainText('offline');

	expect(posted).toHaveLength(1);
	expect(posted[0].character).toBe('Deathsong');
	const slots = posted[0].items.map(item => item.acSlot);
	expect(slots).toContain(NECK);
	expect(slots).toContain(BELT);
	expect(slots).not.toContain(CHEST);
	expect(slots).not.toContain(MAIN_HAND);
	expect(slots).not.toContain(SHOULDERS);
	expect(new Set(slots).size).toBe(slots.length);
	expect(posted[0].items.find(item => item.acSlot == NECK)).toEqual({ acSlot: NECK, itemId: 45193, from: 32, to: 37 });
	expect(posted[0].items.find(item => item.acSlot == BELT)).toEqual({ acSlot: BELT, itemId: 40278, from: 0, to: 0 });
	expect(posted[0].items.find(item => item.acSlot == HEAD)).toEqual({ acSlot: HEAD, itemId: 45472, from: 0, to: 0 });
	expect(errors).toEqual([]);
});

test("reforging off in the page's server settings doesn't make Send strip the server's reforges", async ({ page }) => {
	await importDeathsong(page);
	await openSimTab(page, 'settings-tab');
	await page
		.locator('#settings-tab .server-settings-picker-root .input-root', { has: page.locator('label', { hasText: /^Reforging$/ }) })
		.locator('input')
		.uncheck();

	const dialog = await openDialog(page);
	await expect(row(dialog, NECK).locator('td').nth(3)).not.toHaveText('none');
	await expect(row(dialog, NECK).locator('.acore-reforge-row-status')).toHaveText('Same');
	await expect(row(dialog, CHEST).locator('.acore-reforge-row-status')).toHaveText('Same');
});

test("picking another character lists that character's server gear", async ({ page }) => {
	await importDeathsong(page);
	const dialog = await openDialog(page);
	await dialog.locator('.acore-reforge-character').selectOption('Felesta');
	await expect(row(dialog, NECK).locator('.acore-reforge-row-status')).toHaveText('Different item on the server, left out');
});

test("the server's refusal shows as the dialog's status", async ({ page }) => {
	await importDeathsong(page);
	await stubReforge(page, { status: 503, body: { code: 'not_configured', error: 'This sim server is not set up to reforge on AzerothCore' } });
	const dialog = await openDialog(page);
	await dialog.locator('.acore-reforge-send').click();
	await expect(dialog.locator('.acore-reforge-status')).toHaveText('This sim server is not set up to reforge on AzerothCore');
	await expect(dialog.locator('.acore-reforge-send')).toBeEnabled();
});

test('the button is hidden when the sim runs in the browser', async ({ page }) => {
	await page.route('**/sim_worker.js', route =>
		route.request().resourceType() == 'fetch' ? route.fulfill({ contentType: 'text/javascript', body: '' }) : route.fallback(),
	);
	await stubServer(page, { body: JSON.stringify(rosterJson) });
	await openSpec(page, SPEC);
	await openSimTab(page, 'caps-tab');
	await expect(page.locator('#caps-tab .caps-roster-import')).toBeHidden();
	await expect(page.locator('#caps-tab .caps-server-reforge')).toBeHidden();
});
