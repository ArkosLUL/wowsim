import { expect, type Locator, type Page, test } from '@playwright/test';

import { openRaid, openSim, openSimTab, SPECS, watchForErrors } from '../lib/page';
import { addPreset, playerEditor, slot } from '../raid/raid';
import { expectActive, icon, isActive, openMultiIcon, openSpec, select } from '../settings/helpers';

const capRow = (page: Page, label: string) => page.locator(`#caps-tab tr.caps-row[data-label="${label}"]`).first();

async function needed(page: Page, label: string): Promise<number> {
	const row = capRow(page, label);
	await expect(row).toBeVisible();
	return Number(await row.getAttribute('data-needed'));
}

async function setIcon(button: Locator, active: boolean) {
	if ((await isActive(button)) != active) {
		await button.click();
	}
	await expectActive(button, active);
}

// The tab works its caps out again when it's shown after a change, so this waits for the new number.
async function neededAfterChange(page: Page, label: string, before: number): Promise<number> {
	await openSimTab(page, 'caps-tab');
	await expect.poll(() => needed(page, label)).not.toBe(before);
	return needed(page, label);
}

for (const spec of SPECS) {
	test(`the ${spec} page has a Caps & Priority tab with its caps`, async ({ page }) => {
		const errors = watchForErrors(page);
		await openSim(page, spec);
		await openSimTab(page, 'caps-tab');
		await expect(page.locator('#caps-tab tr.caps-row').first()).toBeVisible();
		await expect(page.locator('#caps-tab .caps-status')).toHaveText('');
		expect(errors).toEqual([]);
	});
}

test('the tab comes right after the BiS Optimizer', async ({ page }) => {
	await openSim(page, 'warrior');
	const tabs = await page.locator('li.nav-item').evaluateAll(items => items.map(item => item.className));
	const optimizer = tabs.findIndex(name => name.includes('optimizer-tab'));
	expect(optimizer).toBeGreaterThanOrEqual(0);
	expect(tabs[optimizer + 1]).toContain('caps-tab');
});

test("a raider's Edit window in the raid sim has no caps tab", async ({ page }) => {
	await openRaid(page);
	await addPreset(page, 'Fury Warrior', 0);
	await slot(page, 0).locator('.player-edit').click();
	const editor = playerEditor(page);
	await expect(editor.locator('.gear-tab').first()).toBeAttached();
	await expect(editor.locator('.caps-tab')).toHaveCount(0);
});

test('the roster button and the Import menu open the same AzerothCore importer', async ({ page }) => {
	await openSim(page, 'warrior');
	const importer = page.locator('.modal.show', { has: page.locator('.importer') });

	await openSimTab(page, 'caps-tab');
	await page.locator('#caps-tab .caps-roster-import').click();
	await expect(importer).toBeVisible();
	await expect(importer).toContainText('AzerothCore Import');
	await importer.locator('.close-button').first().click();
	await expect(importer).toHaveCount(0);

	await page.locator('.import-link').first().hover();
	await page.locator('.import-dropdown .dropdown-item', { hasText: /^\s*AzerothCore\s*$/ }).first().click();
	await expect(importer).toContainText('AzerothCore Import');
});

test('the roster button is hidden when the sim runs in the browser', async ({ page }) => {
	// a page tells wasm from a sim server by fetching sim_worker.js; the worker itself still loads
	await page.route('**/sim_worker.js', route =>
		route.request().resourceType() == 'fetch' ? route.fulfill({ contentType: 'text/javascript', body: '' }) : route.fallback(),
	);
	await openSim(page, 'warrior');
	await openSimTab(page, 'caps-tab');
	await expect(capRow(page, 'Hit')).toBeVisible();
	await expect(page.locator('#caps-tab .caps-roster-import')).toBeHidden();
});

test("Heroic Presence moves Fury's needed hit by 33", async ({ page }) => {
	await openSpec(page, 'warrior', 'settings-tab');
	const settings = page.locator('#settings-tab');
	// a Draenei brings Heroic Presence along whatever the buff says
	await select(settings.locator('.player-settings'), 'Race').selectOption({ label: 'Human' });
	const buffs = settings.locator('.buffs-settings');
	const heroicPresence = icon(buffs, 'spell', 6562);
	await openMultiIcon(buffs, 'Misc');
	await setIcon(heroicPresence, false);

	await openSimTab(page, 'caps-tab');
	const without = await needed(page, 'Hit');

	await openSimTab(page, 'settings-tab');
	await openMultiIcon(buffs, 'Misc');
	await setIcon(heroicPresence, true);
	const withIt = await neededAfterChange(page, 'Hit', without);
	expect(Math.round(without) - Math.round(withIt)).toBe(33);

	await capRow(page, 'Hit').click();
	const details = page.locator('#caps-tab tr.caps-details').first();
	await expect(details).toBeVisible();
	await expect(details.locator('tr.caps-source', { hasText: 'Heroic Presence' })).toContainText('−33');
});

test("Misery moves Shadow's needed spell hit by 79", async ({ page }) => {
	await openSpec(page, 'shadow_priest', 'settings-tab');
	const debuffs = page.locator('#settings-tab .debuffs-settings');
	const misery = icon(debuffs, 'spell', 33198);
	// Improved Faerie Fire is the same 3%, and the two don't stack
	const faerieFire = icon(debuffs, 'spell', 33602);
	await openMultiIcon(debuffs, 'Spell Hit');
	await setIcon(misery, false);
	await setIcon(faerieFire, false);

	await openSimTab(page, 'caps-tab');
	const without = await needed(page, 'Spell hit');

	await openSimTab(page, 'settings-tab');
	await openMultiIcon(debuffs, 'Spell Hit');
	await setIcon(misery, true);
	const withIt = await neededAfterChange(page, 'Spell hit', without);
	// Misery is 78.7 rating; on whole-rating gear the cap moves by 78 or 79, depending on where it falls
	expect(Math.round(without) - Math.round(withIt)).toBeGreaterThanOrEqual(78);
	expect(Math.round(without) - Math.round(withIt)).toBeLessThanOrEqual(79);

	await capRow(page, 'Spell hit').click();
	const details = page.locator('#caps-tab tr.caps-details').first();
	await expect(details.locator('tr.caps-source', { hasText: 'Misery' })).toContainText('−79');
	await expect(details.locator('tr.caps-source', { hasText: 'Talents on your spells' })).toBeVisible();
});
