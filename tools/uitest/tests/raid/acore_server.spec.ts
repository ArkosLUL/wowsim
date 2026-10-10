import { expect, type Page, type Request, test } from '@playwright/test';

import { openRaid, watchForErrors } from '../lib/page';
import { collectDialogs, fixtureRoster, openImporter, readRoster, roster, rosterSlots, savedSettings, seedRaid } from './raid';

const ROSTER = readRoster();
const rosterJson = JSON.parse(ROSTER);

// Synthetic version 2 roster of six, quick to import twice in one test.
const LOADOUT = readRoster('loadout');

// The sim server's own texts (sim/web/acroster/handler.go)
const NOT_CONFIGURED = "This sim server isn't set up to import from AzerothCore: it was started without --acoreDsn, --acoreDbcDir, --acoreNamesFile";
const DATABASE_DOWN = "The AzerothCore database didn't answer. The sim server's log says why.";

interface Reply {
	status?: number;
	contentType?: string;
	body: string;
	// the answer is held back until this resolves
	after?: Promise<void>;
}

interface StubbedServer {
	reply: Reply;
	requests: Array<Request>;
}

const errorReply = (status: number, code: string, error: string): Reply => ({ status, body: JSON.stringify({ code, error }) });

// Stands in for the sim server's /acoreRoster, which needs a live AzerothCore database. Set `reply`
// to change the next answer. A request with a query string lands here too, so a test can catch one.
async function stubServer(page: Page, reply: Reply): Promise<StubbedServer> {
	const server: StubbedServer = { reply, requests: [] };
	await page.route(/\/acoreRoster(\?.*)?$/, async route => {
		server.requests.push(route.request());
		const { status = 200, contentType = 'application/json', body, after } = server.reply;
		await after;
		await route.fulfill({ status, contentType, body });
	});
	return server;
}

const serverButton = (page: Page) => page.locator('.raid-controls .acore-server-import');

// Clicks the Raid tab's button and returns the alert it ends with.
async function importFromServer(page: Page, dialogs: Array<string>): Promise<string> {
	const seen = dialogs.length;
	await serverButton(page).click();
	await expect.poll(() => dialogs.length, { timeout: 60_000 }).toBeGreaterThan(seen);
	return dialogs[seen];
}

// The raider as the raid page saved them, in protojson.
async function savedRaider(page: Page, name: string): Promise<any> {
	const settings = await savedSettings(page);
	return (settings.raid?.parties ?? []).flatMap((party: any) => party.players ?? []).find((player: any) => player.name == name);
}

test('Fetch from server fills the dialog like an upload, and Import takes it from there', async ({ page }) => {
	const dialogs = collectDialogs(page);
	await stubServer(page, { body: ROSTER });
	await openRaid(page);

	const modal = await openImporter(page, 'AzerothCore');
	await expect(modal.locator('.import-description')).toContainText('saveall');
	await modal.locator('.fetch-button').click();
	await expect(modal.locator('.importer-textarea')).toHaveValue(ROSTER);
	await expect(modal.locator('.fetch-button')).toBeEnabled();
	// only Import touches the raid
	expect(dialogs).toEqual([]);
	expect((await roster(page)).filter(name => name)).toEqual([]);

	await modal.locator('.import-button').click();
	await expect.poll(() => dialogs.length, { timeout: 60_000 }).toBe(1);
	expect(dialogs[0].split('\n')[0]).toBe(`AzerothCore import (Replace): ${rosterJson.group.leader}'s raid, exported ${rosterJson.exportedAt}.`);
	await expect(modal).toHaveCount(0);
	await expect.poll(() => roster(page)).toEqual(rosterSlots(ROSTER));
});

test('a sim server that is not set up says so in the dialog and changes nothing', async ({ page }) => {
	const dialogs = collectDialogs(page);
	await stubServer(page, errorReply(503, 'not_configured', NOT_CONFIGURED));
	await seedRaid(page);

	const modal = await openImporter(page, 'AzerothCore');
	await modal.locator('.fetch-button').click();
	await expect.poll(() => dialogs.length).toBe(1);
	expect(dialogs[0]).toBe(`Import error:\n${NOT_CONFIGURED}`);
	await expect(modal.locator('.importer-textarea')).toHaveValue('');
	await expect(modal.locator('.fetch-button')).toBeEnabled();

	await modal.locator('.close-button').click();
	await expect(modal).toHaveCount(0);
	await expect.poll(() => roster(page)).toEqual(fixtureRoster());
});

test('Import from server builds an empty raid from the roster', async ({ page }) => {
	const errors = watchForErrors(page);
	const dialogs = collectDialogs(page);
	await stubServer(page, { body: ROSTER });
	await openRaid(page);
	await expect(serverButton(page)).toHaveAttribute('data-bs-title', /as of each character's last save/);

	const lines = (await importFromServer(page, dialogs)).split('\n');
	expect(lines[0]).toBe(`AzerothCore import (Replace): ${rosterJson.group.leader}'s raid, exported ${rosterJson.exportedAt}.`);
	expect(lines[1]).toBe(`${rosterJson.characters.length} of ${rosterJson.characters.length} characters in 5 parties.`);
	await expect.poll(() => roster(page)).toEqual(rosterSlots(ROSTER));
	await expect(serverButton(page)).toBeEnabled();
	expect(errors).toEqual([]);
});

test('Import from server updates a seated raid, consumables included, and drops who left the roster', async ({ page }) => {
	const dialogs = collectDialogs(page);
	const server = await stubServer(page, { body: LOADOUT });
	await openRaid(page);
	await importFromServer(page, dialogs);
	await expect.poll(() => roster(page)).toEqual(rosterSlots(LOADOUT));
	await expect.poll(async () => (await savedRaider(page, 'Fletcher'))?.consumes?.flask).toBe('FlaskOfEndlessRage');

	// Fletcher switched flask and Frostfinger left the raid
	const changed = JSON.parse(LOADOUT);
	changed.characters = changed.characters.filter((char: any) => char.name != 'Frostfinger');
	changed.characters.find((char: any) => char.name == 'Fletcher').consumes.flask = { value: 'FlaskOfPureMojo', source: 'matrix', itemId: 46378, spellId: 54212 };
	server.reply = { body: JSON.stringify(changed) };

	const report = await importFromServer(page, dialogs);
	expect(report.split('\n')[0]).toContain('AzerothCore import (Update)');
	expect(report).toContain("Removed, they aren't in the roster: Frostfinger");
	expect(report).not.toContain('kept as they were');
	await expect.poll(() => roster(page)).toEqual(rosterSlots(JSON.stringify(changed)));
	await expect.poll(async () => (await savedRaider(page, 'Fletcher'))?.consumes?.flask).toBe('FlaskOfPureMojo');
});

test('the button is off while the server works, and a failed import frees it and leaves the raid alone', async ({ page }) => {
	const dialogs = collectDialogs(page);
	let answer!: () => void;
	const held = new Promise<void>(resolve => (answer = resolve));
	const server = await stubServer(page, { ...errorReply(502, 'database', DATABASE_DOWN), after: held });
	await seedRaid(page);

	const button = serverButton(page);
	await button.click();
	await expect(button).toBeDisabled();
	expect(dialogs).toEqual([]);
	answer();
	await expect.poll(() => dialogs.length).toBe(1);
	expect(dialogs[0]).toBe(`Import error:\n${DATABASE_DOWN}`);
	await expect(button).toBeEnabled();
	await expect(button).toHaveText('Import from server');

	// a sim server built before the route existed answers with its 404 page
	server.reply = { status: 404, contentType: 'text/plain', body: '404 page not found\n' };
	expect(await importFromServer(page, dialogs)).toBe("Import error:\nThis sim server can't import from AzerothCore");
	await expect(button).toBeEnabled();
	await expect.poll(() => roster(page)).toEqual(fixtureRoster());
});

test('both buttons ask the server with a bare GET', async ({ page }) => {
	const dialogs = collectDialogs(page);
	const server = await stubServer(page, errorReply(503, 'not_configured', NOT_CONFIGURED));
	await openRaid(page);

	await importFromServer(page, dialogs);
	const modal = await openImporter(page, 'AzerothCore');
	await modal.locator('.fetch-button').click();
	await expect.poll(() => dialogs.length).toBe(2);

	expect(server.requests).toHaveLength(2);
	for (const request of server.requests) {
		const url = new URL(request.url());
		expect(request.method()).toBe('GET');
		expect(url.pathname + url.search).toBe('/acoreRoster');
		expect(request.postData()).toBeNull();
		expect(request.headers()['content-type']).toBeUndefined();
	}
});

test('a page that runs the sim in the browser gets neither button', async ({ page }) => {
	// Static hosting serves the wasm worker here, which knows none of the server's routes. Only the
	// page's own look at the file is swapped, so the real worker still loads.
	await page.route('**/sim_worker.js', route =>
		route.request().resourceType() == 'fetch' ? route.fulfill({ contentType: 'text/javascript', body: '// wasm worker' }) : route.fallback(),
	);
	await openRaid(page);
	await expect(page.locator('.raid-controls')).toBeVisible();
	await expect(serverButton(page)).toHaveCount(0);

	const looked = page.waitForResponse('**/sim_worker.js');
	const modal = await openImporter(page, 'AzerothCore');
	await (await looked).finished();
	// the button would only show up a moment after the answer
	await page.waitForTimeout(250);
	await expect(modal.locator('.import-button')).toBeVisible();
	await expect(modal.locator('.fetch-button')).toHaveCount(0);
	await expect(modal.locator('.acore-fetch-hint')).toHaveCount(0);
});

// Unstubbed: runs only with UITEST_ACORE_LIVE=1, against a sim server started with the --acore* flags.
// It reads the AzerothCore database and writes nothing.
test('the real route fills an empty raid, then updates it in place', async ({ page }) => {
	test.skip(!process.env.UITEST_ACORE_LIVE, 'needs a sim server that reads a live AzerothCore database');
	const errors = watchForErrors(page);
	const dialogs = collectDialogs(page);
	await openRaid(page);

	let reply = page.waitForResponse(/\/acoreRoster$/);
	const built = await importFromServer(page, dialogs);
	expect(built.split('\n')[0]).toContain('AzerothCore import (Replace)');
	let live = await (await reply).text();
	expect(built.split('\n')[0]).toContain(JSON.parse(live).exportedAt);
	await expect.poll(() => roster(page)).toEqual(rosterSlots(live));

	reply = page.waitForResponse(/\/acoreRoster$/);
	const updated = await importFromServer(page, dialogs);
	expect(updated.split('\n')[0]).toContain('AzerothCore import (Update)');
	live = await (await reply).text();
	expect(updated).not.toContain('Added:');
	expect(updated).not.toContain('Removed');
	await expect.poll(() => roster(page)).toEqual(rosterSlots(live));
	expect(errors).toEqual([]);
});
