import { expect, type Locator, type Page, test } from '@playwright/test';

import { closePicker, openPicker, pane } from '../gear/gear';
import { errorReply, NOT_CONFIGURED, stubServer } from '../lib/acore';
import { openSimTab, watchForErrors } from '../lib/page';
import { exportJson, openImporter, paste, readRoster } from '../raid/raid';
import { acceptDialogs, openSpec, reloadSim, storageKey, storedSettings } from '../settings/helpers';

const SPEC = 'deathknight';
const rosterJson = JSON.parse(readRoster());
const DEATH_KNIGHTS = ['Deathsong (Unholy)', 'Obliteration (Frost)', 'Felesta (Blood)'];
const character = (roster: any, name: string) => roster.characters.find((char: any) => char.name == name);
const copy = (roster: any) => JSON.parse(JSON.stringify(roster));

// Deathsong's neck carries a reforge (acSlot 1), his chest (4) and legs (6) too, his head (0), shoulders (2) and belt (5) none
const NECK = 1;
const SHOULDERS = 2;
const CHEST = 4;
const LEGS = 6;
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

// The roster with Deathsong's gear changed: crit into expertise on every slot without a reforge, so
// whatever the sim leaves unreforged reads as a removal; a reforge from crit on the shoulders (which
// have none, so the sim can't take it), no reforge on the neck, another chest, no main hand.
function changedRoster(): string {
	const changed = JSON.parse(JSON.stringify(rosterJson));
	const gear: Array<any> = changed.characters.find((char: any) => char.name == 'Deathsong').gear;
	for (const item of gear) {
		item.reforge ??= { fromStatType: 32, toStatType: 37 };
	}
	gear.find(item => item.acSlot == SHOULDERS).reforge = { fromStatType: 32, toStatType: 37 };
	delete gear.find(item => item.acSlot == NECK).reforge;
	gear.find(item => item.acSlot == CHEST).id = 99999;
	gear.splice(
		gear.findIndex(item => item.acSlot == MAIN_HAND),
		1,
	);
	return JSON.stringify(changed);
}

const tab = (page: Page) => page.locator('#caps-tab');
const characterSelect = (page: Page) => tab(page).locator('.caps-character');
const preview = (page: Page) => tab(page).locator('.caps-send-preview');
const line = (page: Page, acSlot: number) => preview(page).locator(`.caps-send-line[data-ac-slot="${acSlot}"]`);

// Opens the Caps tab and loads Deathsong, the preselected character, with Reload. Returns the roster stub.
async function loadDeathsong(page: Page, dialogs: string[]) {
	const server = await stubServer(page, { body: JSON.stringify(rosterJson) });
	await openSpec(page, SPEC);
	await openSimTab(page, 'caps-tab');
	await expect(characterSelect(page).locator('option')).toHaveText(DEATH_KNIGHTS);
	await expect(characterSelect(page)).toHaveValue('Deathsong');
	await tab(page).locator('.caps-character-reload').click();
	await expect(tab(page).locator('.caps-import-loaded')).toContainText('Loaded Deathsong from the server');
	await expect(tab(page).locator('.caps-character-status')).toHaveText('');
	expect(dialogs).toEqual([]);
	return server;
}

async function startSend(page: Page): Promise<Locator> {
	const send = tab(page).locator('.reforge-apply-send');
	await expect(send).toBeEnabled();
	await send.click();
	await expect(preview(page)).toBeVisible();
	await expect(preview(page).locator('.caps-send-title, .caps-send-message').first()).not.toContainText('Reading');
	return preview(page);
}

// No suggestions then, so a send applies none first: the sim keeps exactly the imported reforges.
async function turnReforgingOff(page: Page) {
	await openSimTab(page, 'settings-tab');
	await page
		.locator('#settings-tab .server-settings-picker-root .input-root', { has: page.locator('label', { hasText: /^Reforging$/ }) })
		.locator('input')
		.uncheck();
	await openSimTab(page, 'caps-tab');
	await expect(tab(page).locator('.reforge-status')).toHaveText('Your server settings have reforging off.');
}

test('picking a character loads it without a window or an alert, and Reload loads it again', async ({ page }) => {
	const errors = watchForErrors(page);
	const dialogs = acceptDialogs(page);
	const server = await loadDeathsong(page, dialogs);
	const deathsong = rosterJson.characters.find((char: any) => char.name == 'Deathsong');
	const notes = tab(page).locator('.caps-import-notes');
	await expect(notes).toContainText(`(saved ${rosterJson.exportedAt})`);
	await expect(notes).toContainText('Heroic Presence on');
	expect((await storedSettings(page, SPEC)).player.talentsString).toBe(deathsong.talents);
	expect(await page.evaluate(key => localStorage.getItem(key), storageKey(SPEC, 'acore-import.v1'))).toBe(JSON.stringify({ character: 'Deathsong' }));

	const obliteration = rosterJson.characters.find((char: any) => char.name == 'Obliteration');
	await characterSelect(page).selectOption('Obliteration');
	await expect(notes).toContainText('Loaded Obliteration from the server');
	expect((await storedSettings(page, SPEC)).player.talentsString).toBe(obliteration.talents);

	// each pick and each Reload reads the roster again
	const requests = server.requests.length;
	await tab(page).locator('.caps-character-reload').click();
	await expect.poll(() => server.requests.length).toBe(requests + 1);
	await expect(notes).toContainText('Loaded Obliteration from the server');
	expect(dialogs).toEqual([]);
	expect(errors).toEqual([]);
});

test('a pick waits for its load, and a raider gone from the roster fails without loading anyone else', async ({ page }) => {
	const dialogs = acceptDialogs(page);
	const server = await loadDeathsong(page, dialogs);
	const notes = tab(page).locator('.caps-import-notes');

	let release!: () => void;
	server.reply = { body: JSON.stringify(rosterJson), after: new Promise<void>(resolve => (release = () => resolve())) };
	await characterSelect(page).selectOption('Obliteration');
	await expect(tab(page).locator('.caps-character-status')).toHaveText('Loading Obliteration from the server…');
	await expect(characterSelect(page)).toBeDisabled();
	await expect(tab(page).locator('.caps-character-reload')).toBeDisabled();
	release();
	await expect(notes).toContainText('Loaded Obliteration from the server');
	await expect(characterSelect(page)).toBeEnabled();

	const withoutDeathsong = JSON.parse(JSON.stringify(rosterJson));
	withoutDeathsong.characters = withoutDeathsong.characters.filter((char: any) => char.name != 'Deathsong');
	server.reply = { body: JSON.stringify(withoutDeathsong) };
	await characterSelect(page).selectOption('Deathsong');
	await expect(tab(page).locator('.caps-character-status')).toHaveText("Couldn't load Deathsong: Deathsong isn't in this roster.");
	await expect(notes).toContainText('Loaded Obliteration from the server');
	const obliteration = rosterJson.characters.find((char: any) => char.name == 'Obliteration');
	expect((await storedSettings(page, SPEC)).player.talentsString).toBe(obliteration.talents);
	expect(dialogs).toEqual([]);
});

test('the Import menu window imports without an alert and its notes reach the tab', async ({ page }) => {
	const dialogs = acceptDialogs(page);
	await stubServer(page, { body: JSON.stringify(rosterJson) });
	await openSpec(page, SPEC);
	const modal = await openImporter(page, 'AzerothCore');
	await expect(modal.locator('.acore-character-select')).toHaveValue('Deathsong');
	await modal.locator('.import-button').click();
	await expect(modal).toHaveCount(0);

	await openSimTab(page, 'caps-tab');
	await expect(tab(page).locator('.caps-import-loaded')).toContainText('Loaded Deathsong from the server');
	await expect(tab(page).locator('.caps-import-notes')).toContainText('Heroic Presence on');
	expect(dialogs).toEqual([]);
	expect((await storedSettings(page, SPEC)).player.talentsString).toBe(rosterJson.characters.find((char: any) => char.name == 'Deathsong').talents);
	await expect(tab(page).locator('.reforge-apply-send')).toBeVisible();
});

test('an import error from the window still alerts', async ({ page }) => {
	const dialogs = acceptDialogs(page);
	await stubServer(page, { body: JSON.stringify(rosterJson) });
	await openSpec(page, SPEC);
	const modal = await openImporter(page, 'AzerothCore');
	await paste(modal.locator('.importer-textarea'), '{"version": 99}');
	await modal.locator('.import-button').click();
	await expect.poll(() => dialogs.length).toBe(1);
	expect(dialogs[0]).toMatch(/^Import error:/);
});

test("the server's refusal shows inline in the character block, and Reload reads the list again", async ({ page }) => {
	const dialogs = acceptDialogs(page);
	const server = await stubServer(page, errorReply(503, 'not_configured', NOT_CONFIGURED));
	await openSpec(page, SPEC);
	await openSimTab(page, 'caps-tab');
	await expect(tab(page).locator('.caps-character-status')).toHaveText(`Couldn't read the roster off the server: ${NOT_CONFIGURED}`);
	await expect(tab(page).locator('.caps-character-status')).toHaveClass(/caps-error/);
	await expect(characterSelect(page)).toBeDisabled();

	server.reply = { body: JSON.stringify(rosterJson) };
	await tab(page).locator('.caps-character-reload').click();
	await expect(characterSelect(page).locator('option')).toHaveText(DEATH_KNIGHTS);
	await expect(tab(page).locator('.caps-character-status')).toHaveText('');
	await expect(tab(page).locator('.caps-import-loaded')).toHaveCount(0);
	expect(dialogs).toEqual([]);
});

test('each option names the main talent tree, and a pick still loads that character by name', async ({ page }) => {
	const dialogs = acceptDialogs(page);
	const untalented = copy(rosterJson);
	character(untalented, 'Obliteration').talents = '';
	await stubServer(page, { body: JSON.stringify(untalented) });
	await openSpec(page, SPEC);
	await openSimTab(page, 'caps-tab');
	const options = characterSelect(page).locator('option');
	await expect(options).toHaveText(['Deathsong (Unholy)', 'Obliteration', 'Felesta (Blood)']);
	expect(await options.evaluateAll(opts => opts.map(opt => (opt as HTMLOptionElement).value))).toEqual(['Deathsong', 'Obliteration', 'Felesta']);

	await characterSelect(page).selectOption({ label: 'Felesta (Blood)' });
	await expect(tab(page).locator('.caps-import-loaded')).toContainText('Loaded Felesta from the server');
	expect((await storedSettings(page, SPEC)).player.talentsString).toBe(character(rosterJson, 'Felesta').talents);

	// the saved pick is a name, and still preselects its labelled option
	await reloadSim(page);
	await openSimTab(page, 'caps-tab');
	await expect(characterSelect(page)).toHaveValue('Felesta');
	await expect(characterSelect(page).locator('option:checked')).toHaveText('Felesta (Blood)');
	expect(dialogs).toEqual([]);
});

test.describe('healers load only on healer pages', () => {
	// the roster's priests: Malediction plays Shadow, Prayer Discipline
	const PRIEST_PAGES = [
		{ spec: 'shadow_priest', listed: 'Malediction', tree: 'Shadow' },
		{ spec: 'smite_priest', listed: 'Malediction', tree: 'Shadow' },
		{ spec: 'healing_priest', listed: 'Prayer', tree: 'Discipline' },
	];
	for (const { spec, listed, tree } of PRIEST_PAGES) {
		test(`the ${spec} page lists only ${listed}, in the tab and in the window`, async ({ page }) => {
			// the healer page alerts that its sim is experimental
			acceptDialogs(page);
			await stubServer(page, { body: JSON.stringify(rosterJson) });
			await openSpec(page, spec);
			await openSimTab(page, 'caps-tab');
			await expect(characterSelect(page).locator('option')).toHaveText([`${listed} (${tree})`]);
			const modal = await openImporter(page, 'AzerothCore');
			await expect(modal.locator('.acore-server-status')).toContainText('From the server');
			await expect(modal.locator('.acore-character-select option')).toHaveText([listed]);
		});
	}

	test('a character who respecced to heal since the list was read is refused, and nobody else loads', async ({ page }) => {
		const dialogs = acceptDialogs(page);
		const beforeRespec = copy(rosterJson);
		character(beforeRespec, 'Holylight').talents = character(rosterJson, 'Justice').talents;
		const server = await stubServer(page, { body: JSON.stringify(beforeRespec) });
		await openSpec(page, 'retribution_paladin');
		await openSimTab(page, 'caps-tab');
		await expect(characterSelect(page).locator('option')).toHaveText(['Justice (Retribution)', 'Bulwark (Protection)', 'Holylight (Retribution)']);
		const talents = async () => (await storedSettings(page, 'retribution_paladin'))?.player?.talentsString;
		const before = await talents();

		server.reply = { body: JSON.stringify(rosterJson) };
		await characterSelect(page).selectOption('Holylight');
		const status = tab(page).locator('.caps-character-status');
		await expect(status).toHaveText("Couldn't load Holylight: Holylight is specced Holy: load them on a healer page.");
		await expect(status).toHaveClass(/caps-error/);
		await expect(tab(page).locator('.caps-import-loaded')).toHaveCount(0);
		expect(await talents()).toBe(before);
		expect(dialogs).toEqual([]);
	});

	test('a DPS page with only healers of its class says so, in the tab and in the window', async ({ page }) => {
		const dialogs = acceptDialogs(page);
		const healersOnly = copy(rosterJson);
		healersOnly.characters = healersOnly.characters.filter((char: any) => char.name != 'Malediction');
		await stubServer(page, { body: JSON.stringify(healersOnly) });
		await openSpec(page, 'shadow_priest');
		await openSimTab(page, 'caps-tab');
		await expect(tab(page).locator('.caps-character-status')).toHaveText('Only Priest healers on the server: they load on a healer page.');
		await expect(characterSelect(page).locator('option')).toHaveCount(0);
		await expect(characterSelect(page)).toBeDisabled();

		const modal = await openImporter(page, 'AzerothCore');
		await expect(modal.locator('.acore-character-select option')).toHaveText(['Only Priest healers in this roster: they load on a healer page']);
		await modal.locator('.import-button').click();
		await expect.poll(() => dialogs).toEqual(['Import error:\nOnly Priest healers in this roster: they load on a healer page.']);
	});

	test('the healer page with no healer of its class says so', async ({ page }) => {
		acceptDialogs(page);
		const noHealer = copy(rosterJson);
		noHealer.characters = noHealer.characters.filter((char: any) => char.name != 'Prayer');
		await stubServer(page, { body: JSON.stringify(noHealer) });
		await openSpec(page, 'healing_priest');
		await openSimTab(page, 'caps-tab');
		await expect(tab(page).locator('.caps-character-status')).toHaveText('No Priest healers on the server.');
		await expect(characterSelect(page)).toBeDisabled();
	});
});

test('the preview lists only the slots that change and counts the ones left out', async ({ page }) => {
	const errors = watchForErrors(page);
	const dialogs = acceptDialogs(page);
	const server = await loadDeathsong(page, dialogs);
	await turnReforgingOff(page);
	server.reply = { body: changedRoster() };

	await startSend(page);
	await expect(line(page, NECK).locator('.caps-send-text')).toHaveText(/^none → .+/);
	await expect(preview(page).locator('.caps-send-text', { hasText: '(Will remove)' }).first()).toBeVisible();
	for (const left of [CHEST, SHOULDERS, MAIN_HAND, LEGS]) {
		await expect(line(page, left)).toHaveCount(0);
	}
	const skipped = preview(page).locator('.caps-send-skipped');
	await expect(skipped).toContainText("the sim's copy of the item can't take the server's reforge");
	await expect(skipped).toContainText('different item on the server');
	await expect(skipped).toContainText('nothing equipped on the server');
	// the same reforge on both sides: neither listed nor left out
	await expect(skipped).not.toContainText('Legs');
	await expect(preview(page).locator('.caps-send-notes')).toContainText('A bot redoes its own reforges when it refreshes its gear.');
	await expect(preview(page).locator('.caps-send-confirm')).toHaveText('Confirm send');
	expect(dialogs).toEqual([]);
	expect(errors).toEqual([]);
});

test('Confirm posts exactly the changing slots and shows each answer and online or offline', async ({ page }) => {
	const dialogs = acceptDialogs(page);
	const server = await loadDeathsong(page, dialogs);
	await turnReforgingOff(page);
	server.reply = { body: changedRoster() };
	const posted = await stubReforge(page, body => ({
		online: false,
		slots: body.items.map(item => ({ acSlot: item.acSlot, status: 'applied', text: `slot ${item.acSlot} done` })),
	}));

	await startSend(page);
	const listed = await preview(page)
		.locator('.caps-send-line')
		.evaluateAll(lines => lines.map(line => Number((line as HTMLElement).dataset.acSlot)));
	expect(posted).toEqual([]);
	await preview(page).locator('.caps-send-confirm').click();
	await expect(line(page, NECK).locator('.caps-send-text')).toHaveText(`applied: slot ${NECK} done`);
	await expect(preview(page).locator('.caps-send-result')).toContainText('Deathsong was offline');

	expect(posted).toHaveLength(1);
	expect(posted[0].character).toBe('Deathsong');
	const slots = posted[0].items.map(item => item.acSlot);
	expect([...slots].sort((a, b) => a - b)).toEqual([...listed].sort((a, b) => a - b));
	expect(slots).toContain(NECK);
	expect(posted[0].items.some(item => item.from == 0 && item.to == 0)).toBe(true);
	for (const left of [CHEST, SHOULDERS, MAIN_HAND, LEGS]) {
		expect(slots).not.toContain(left);
	}
	expect(new Set(slots).size).toBe(slots.length);
	expect(posted[0].items.find(item => item.acSlot == NECK)!.itemId).toBe(45193);
	// one send per preview
	await expect(preview(page).locator('.caps-send-confirm')).toBeHidden();
});

test('an online character says so', async ({ page }) => {
	const dialogs = acceptDialogs(page);
	const server = await loadDeathsong(page, dialogs);
	server.reply = { body: changedRoster() };
	await stubReforge(page, body => ({ online: true, slots: body.items.map(item => ({ acSlot: item.acSlot, status: 'applied', text: 'ok' })) }));
	await startSend(page);
	await preview(page).locator('.caps-send-confirm').click();
	await expect(preview(page).locator('.caps-send-result')).toContainText('Deathsong was online, so the reforges are in effect now');
});

test('Cancel posts nothing and closes the preview', async ({ page }) => {
	const dialogs = acceptDialogs(page);
	const server = await loadDeathsong(page, dialogs);
	server.reply = { body: changedRoster() };
	const posted = await stubReforge(page, { body: { online: false, slots: [] } });

	await startSend(page);
	await preview(page).locator('.caps-send-cancel').click();
	await expect(preview(page)).toBeHidden();
	expect(posted).toEqual([]);
});

test('a gear change or another load drops the preview', async ({ page }) => {
	const dialogs = acceptDialogs(page);
	const server = await loadDeathsong(page, dialogs);
	server.reply = { body: changedRoster() };
	await startSend(page);

	await openSimTab(page, 'gear-tab');
	const dialog = await openPicker(page, 'head');
	await pane(dialog, 'Items').locator('.selector-modal-remove-button').click();
	await closePicker(page);
	await openSimTab(page, 'caps-tab');
	await expect(preview(page)).toBeHidden();

	await startSend(page);
	await characterSelect(page).selectOption('Obliteration');
	await expect(tab(page).locator('.caps-import-loaded')).toContainText('Loaded Obliteration');
	await expect(preview(page)).toBeHidden();
});

test('another importer leaves nothing to send to', async ({ page }) => {
	const dialogs = acceptDialogs(page);
	await loadDeathsong(page, dialogs);
	await expect(tab(page).locator('.reforge-apply-send')).toBeVisible();

	const settings = await exportJson(page);
	const modal = await openImporter(page, 'JSON');
	await paste(modal.locator('.importer-textarea'), settings);
	await modal.locator('.import-button').click();
	await expect(modal).toHaveCount(0);
	await expect(tab(page).locator('.reforge-apply-send')).toBeHidden();
	await expect(tab(page).locator('.caps-import-loaded')).toHaveCount(0);
});

test("the server's refusal shows in the preview and Confirm stays available", async ({ page }) => {
	const dialogs = acceptDialogs(page);
	const server = await loadDeathsong(page, dialogs);
	server.reply = { body: changedRoster() };
	await stubReforge(page, { status: 503, body: { code: 'not_configured', error: 'This sim server is not set up to reforge on AzerothCore' } });

	await startSend(page);
	await preview(page).locator('.caps-send-confirm').click();
	await expect(preview(page).locator('.caps-send-result')).toHaveText('This sim server is not set up to reforge on AzerothCore');
	await expect(preview(page).locator('.caps-send-confirm')).toBeEnabled();
});

test('once the server has the reforges the preview says so and offers no Confirm', async ({ page }) => {
	const dialogs = acceptDialogs(page);
	const server = await loadDeathsong(page, dialogs);
	const posted = await stubReforge(page, body => ({
		online: false,
		slots: body.items.map(item => ({ acSlot: item.acSlot, status: 'applied', text: 'ok' })),
	}));

	await startSend(page);
	await preview(page).locator('.caps-send-confirm').click();
	await expect(preview(page).locator('.caps-send-result')).toContainText('Deathsong was offline');
	expect(posted).toHaveLength(1);

	// the server now holds what was sent
	const updated = JSON.parse(JSON.stringify(rosterJson));
	const gear: Array<any> = updated.characters.find((char: any) => char.name == 'Deathsong').gear;
	for (const item of posted[0].items) {
		const held = gear.find(g => g.acSlot == item.acSlot);
		if (item.from || item.to) {
			held.reforge = { fromStatType: item.from, toStatType: item.to };
		} else {
			delete held.reforge;
		}
	}
	server.reply = { body: JSON.stringify(updated) };
	await startSend(page);
	await expect(preview(page)).toContainText('The server already has these reforges.');
	await expect(preview(page).locator('.caps-send-confirm')).toHaveCount(0);
	expect(posted).toHaveLength(1);
});

test("reforging off in the page's server settings hides the suggestions, and the send strips nothing", async ({ page }) => {
	const dialogs = acceptDialogs(page);
	await loadDeathsong(page, dialogs);
	await turnReforgingOff(page);
	await expect(tab(page).locator('.reforge-apply-all')).toBeHidden();
	await expect(tab(page).locator('.reforge-apply-send')).toHaveText('Send to server…');

	await startSend(page);
	await expect(preview(page)).toContainText('The server already has these reforges.');
	await expect(preview(page).locator('.caps-send-confirm')).toHaveCount(0);
});

test('the character block and the send button are hidden when the sim runs in the browser', async ({ page }) => {
	await page.route('**/sim_worker.js', route =>
		route.request().resourceType() == 'fetch' ? route.fulfill({ contentType: 'text/javascript', body: '' }) : route.fallback(),
	);
	const server = await stubServer(page, { body: JSON.stringify(rosterJson) });
	await openSpec(page, SPEC);
	await openSimTab(page, 'caps-tab');
	await expect(tab(page).locator('tr.caps-row').first()).toBeVisible();
	await expect(characterSelect(page)).toBeHidden();
	await expect(tab(page).locator('.reforge-apply-send')).toBeHidden();
	expect(server.requests).toEqual([]);
});
