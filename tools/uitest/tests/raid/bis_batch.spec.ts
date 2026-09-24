import { readFileSync } from 'node:fs';

import { expect, type Locator, type Page, test } from '@playwright/test';

import { openRaid, openSimTab } from '../lib/page';
import { collectDialogs, importRoster, playerEditor, readRoster, reloadRaid, roster, rosterMainTank, slot, statCount } from './raid';

const ROSTER = readRoster();
const mainTank = rosterMainTank(ROSTER);

const batchTab = (page: Page) => page.locator('#bis-batch-tab');
const rows = (page: Page) => batchTab(page).locator('.optimizer-batch-grid table tr');
const phaseBoxes = (page: Page) => rows(page).first().locator('th input');
// the spec follows the name with no space, and one name can start another (Fel, Felesta)
const raiderRow = (page: Page, name: string) => rows(page).filter({ has: page.locator('td .form-check-label', { hasText: new RegExp(`^${name}(?![a-z])`) }) });
const raiderBox = (page: Page, name: string) => raiderRow(page, name).locator('td').first().locator('input');
const cell = (page: Page, name: string, phase: number) => raiderRow(page, name).locator('td').nth(phase);
const progress = (page: Page) => batchTab(page).locator('.optimizer-setup .optimizer-hint').last();
const sourceBoxes = (page: Page) => batchTab(page).locator('.optimizer-setup .optimizer-checkbox-grid input');
const sourceBox = (page: Page, label: string) => batchTab(page).locator('.optimizer-setup .optimizer-checkbox-grid').getByLabel(label, { exact: true });
const tickedSources = (page: Page) => sourceBoxes(page).evaluateAll((boxes: Array<HTMLInputElement>) => boxes.map(box => box.checked));

const BATCH_KEY_PREFIX = '__wotlk_raid__optimizer-batch.v1.';
const REQUEST_LOG = 'Optimize gear request: ';

// the label holds the name, then the spec on a line of its own
const gridNames = (page: Page) =>
	rows(page)
		.locator('td:first-child .form-check-label')
		.evaluateAll(labels => labels.map(label => label.firstChild!.textContent!));

// Every tick redraws the grid, so each box is looked up again right before it's clicked.
async function setTicked(box: Locator, ticked: boolean) {
	if ((await box.isChecked()) != ticked) {
		await box.click();
	}
	await expect(box).toBeChecked({ checked: ticked });
}

async function tickOnly(page: Page, names: Array<string>, phases: Array<number>) {
	for (let phase = 1; phase <= (await phaseBoxes(page).count()); phase++) {
		await setTicked(phaseBoxes(page).nth(phase - 1), phases.includes(phase));
	}
	for (const name of await gridNames(page)) {
		await setTicked(raiderBox(page, name), names.includes(name));
	}
}

async function openBatch(page: Page) {
	const dialogs = collectDialogs(page);
	await openRaid(page);
	await importRoster(page, dialogs, ROSTER);
	await openSimTab(page, 'bis-batch-tab');
	return dialogs;
}

test('the grid lists everyone but the healers, and the ticks stick', async ({ page }) => {
	await openBatch(page);
	const healers = await statCount(page, 'Healers');
	const names = (await roster(page)).filter(name => name);

	const grid = await gridNames(page);
	expect(grid).toHaveLength(names.length - healers);
	// raid order, healers left out
	expect(names.filter(name => grid.includes(name))).toEqual(grid);
	const phases = await phaseBoxes(page).count();
	await expect(progress(page)).toHaveText(
		`${grid.length * phases} of ${grid.length * phases} runs to go. Pick raiders and phases with the checkboxes on the grid.`,
	);

	await tickOnly(page, grid.slice(0, 2), [1, 3]);
	await expect(progress(page)).toHaveText('4 of 4 runs to go. Pick raiders and phases with the checkboxes on the grid.');
	await expect(cell(page, grid[0], 1)).toHaveText('queued');
	await expect(cell(page, grid[0], 2)).toHaveText('');
	await expect(cell(page, grid[2], 1)).toHaveText('');

	await reloadRaid(page);
	await openSimTab(page, 'bis-batch-tab');
	await expect(progress(page)).toHaveText('4 of 4 runs to go. Pick raiders and phases with the checkboxes on the grid.');
	expect(await phaseBoxes(page).evaluateAll(boxes => boxes.map(box => (box as HTMLInputElement).checked))).toEqual(
		Array.from({ length: phases }, (_, i) => i == 0 || i == 2),
	);
	await expect(raiderBox(page, grid[1])).toBeChecked();
	await expect(raiderBox(page, grid[2])).not.toBeChecked();
});

test('the item sources start all ticked and stick, and a batch saved without them gets them all', async ({ page }) => {
	await openBatch(page);
	const all = await tickedSources(page);
	expect(all.length).toBeGreaterThan(2);
	expect(all).not.toContain(false);

	await setTicked(sourceBox(page, 'Raid 25 heroic'), false);
	await setTicked(sourceBox(page, 'World drop'), false);
	const picked = await tickedSources(page);
	expect(picked.filter(ticked => !ticked)).toHaveLength(2);

	await reloadRaid(page);
	await openSimTab(page, 'bis-batch-tab');
	await expect.poll(() => tickedSources(page)).toEqual(picked);

	// what a batch stored by an older build looks like
	const stripped = await page.evaluate(prefix => {
		const keys = Object.keys(localStorage).filter(key => key.startsWith(prefix));
		for (const key of keys) {
			const saved = JSON.parse(localStorage.getItem(key)!);
			delete saved.settings.sources;
			localStorage.setItem(key, JSON.stringify(saved));
		}
		return keys.length;
	}, BATCH_KEY_PREFIX);
	expect(stripped).toBeGreaterThan(0);
	await reloadRaid(page);
	await openSimTab(page, 'bis-batch-tab');
	await expect.poll(() => tickedSources(page)).toEqual(all);
});

// No real run: the routes answer empty, so each job fails right after sending its request.
test('a run asks the server for the ticked item sources only', async ({ page }) => {
	await page.route('**/optimizeGearAsync', route => route.fulfill({ status: 200, body: '' }));
	await page.route('**/asyncProgress', route => route.fulfill({ status: 204 }));
	const requests: Array<any> = [];
	page.on('console', message => {
		if (message.text().startsWith(REQUEST_LOG)) {
			requests.push(JSON.parse(message.text().slice(REQUEST_LOG.length)));
		}
	});
	const sorted = (kinds: Array<number>) => kinds.slice().sort((a, b) => a - b);
	const tickedKinds = async () =>
		sorted(await sourceBoxes(page).evaluateAll((boxes: Array<HTMLInputElement>) => boxes.filter(box => box.checked).map(box => Number(box.value))));
	const requested = (i: number) => sorted(requests[i].settings.sources ?? []);

	await openBatch(page);
	await tickOnly(page, [mainTank], [1]);
	await batchTab(page).getByRole('button', { name: 'Start' }).click();
	await expect(cell(page, mainTank, 1)).toHaveText('failed');
	expect(requests).toHaveLength(1);
	expect(requested(0)).toEqual(await tickedKinds());
	expect(requested(0)).toHaveLength(await sourceBoxes(page).count());

	await setTicked(sourceBox(page, 'Raid 10 heroic'), false);
	await setTicked(sourceBox(page, 'Crafted'), false);
	await batchTab(page).getByRole('button', { name: 'Run failed ones again' }).click();
	await expect.poll(() => requests.length).toBe(2);
	expect(requested(1)).toEqual(await tickedKinds());
	expect(requested(1)).toHaveLength(requested(0).length - 2);
});

// The server runs one optimization at a time, so this is the suite's only real run: a tank, since a
// DPS raider gets a second, raid-scored run after the first.
test('one Quick run fills its cell, and its gear can be saved and cleared', async ({ page }) => {
	test.setTimeout(240_000);
	const dialogs = await openBatch(page);
	await expect(batchTab(page).locator('.optimizer-setup select').first().locator('option:checked')).toHaveText(/^Quick/);
	await tickOnly(page, [mainTank], [1]);
	await expect(progress(page)).toHaveText('1 of 1 run to go. Pick raiders and phases with the checkboxes on the grid.');

	await batchTab(page).getByRole('button', { name: 'Start' }).click();
	await expect(progress(page)).toHaveText('1 run done, none left.', { timeout: 180_000 });
	await expect(batchTab(page).locator('.optimizer-status')).toHaveText('All done.');
	const result = await cell(page, mainTank, 1).innerText();
	expect(result).toMatch(/^(\+|-|no gain)/);

	await cell(page, mainTank, 1).locator('a').click();
	const detail = batchTab(page).locator('.optimizer-batch-detail');
	await expect(detail.locator('h5')).toHaveText(new RegExp(`^${mainTank}, P1`));
	await detail.getByRole('button', { name: `Save as "${mainTank} P1 BiS"` }).click();
	await expect(detail).toContainText(`Saved under ${mainTank}'s Gear Sets.`);

	const download = page.waitForEvent('download');
	await batchTab(page).getByRole('button', { name: 'Export JSON' }).click();
	const exported = JSON.parse(readFileSync(await (await download).path(), 'utf-8'));
	expect(exported.entries.map((entry: any) => [entry.raider, entry.contentPhase])).toEqual([[mainTank, 1]]);

	await reloadRaid(page);
	await openSimTab(page, 'bis-batch-tab');
	await expect(cell(page, mainTank, 1)).toHaveText(result);
	await expect(progress(page)).toHaveText('1 run done, none left.');

	// the saved set shows up with the raider's own gear sets
	await openSimTab(page, 'raid-tab');
	const names = await roster(page);
	await slot(page, names.indexOf(mainTank)).locator('.player-edit').click();
	const editor = playerEditor(page);
	await expect(editor.locator('.saved-data-set-chip', { hasText: `${mainTank} P1 BiS` })).toHaveCount(1);
	await editor.locator('.close-button').click();
	await expect(editor).toHaveCount(0);

	await openSimTab(page, 'bis-batch-tab');
	await batchTab(page).getByRole('button', { name: 'Clear results' }).click();
	await expect.poll(() => dialogs.at(-1)).toBe("Clear this roster's batch results?");
	await expect(cell(page, mainTank, 1)).toHaveText('queued');
	await expect(progress(page)).toHaveText('1 of 1 run to go. Pick raiders and phases with the checkboxes on the grid.');
});
