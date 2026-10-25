import { expect, type Page, test } from '@playwright/test';
import fs from 'node:fs';
import path from 'node:path';

import { openSimTab } from '../lib/page';
import { closeModal, openGear, openPicker, pane, setPhase } from '../gear/gear';
import { expectActive, icon, numberInput, openSpec, reloadSim, setNumber } from './helpers';

// the pages and the variant of their incoming_damage.json a fresh page uses (the priest page opens on Discipline)
const PAGES = [
	{ spec: 'healing_priest', variant: 'disc' },
	{ spec: 'holy_paladin', variant: 'default' },
	{ spec: 'restoration_druid', variant: 'default' },
	{ spec: 'restoration_shaman', variant: 'default' },
];

const DEFAULT_PHASE = 5;

function pageDefault(spec: string, variant: string, phase: number): number {
	const file = path.resolve(__dirname, '../../../../ui', spec, 'incoming_damage.json');
	return JSON.parse(fs.readFileSync(file, 'utf8'))[variant][String(phase)];
}

const settingsTab = (page: Page) => page.locator('#settings-tab');
const incomingDamage = (page: Page) => numberInput(settingsTab(page).locator('.encounter-settings'), 'Incoming Damage');

test('each healer page opens with the incoming damage of its default phase', async ({ page }) => {
	test.setTimeout(180_000);
	for (const { spec, variant } of PAGES) {
		await openSpec(page, spec, 'settings-tab');
		await expect(incomingDamage(page), spec).toHaveValue(String(pageDefault(spec, variant, DEFAULT_PHASE)));
	}
});

test('a DPS page has no incoming damage setting', async ({ page }) => {
	await openSpec(page, 'mage', 'settings-tab');
	await expect(incomingDamage(page)).toHaveCount(0);
});

test('the default incoming damage follows the phase until the user sets one', async ({ page }) => {
	await openGear(page, 'restoration_shaman');
	const dialog = await openPicker(page, 'head');
	await setPhase(pane(dialog, 'Items'), 1);
	await closeModal(dialog);

	await openSimTab(page, 'settings-tab');
	await expect(incomingDamage(page)).toHaveValue(String(pageDefault('restoration_shaman', 'default', 1)));
});

test('incoming damage the user sets survives a reload, and Restore Defaults brings the default back', async ({ page }) => {
	await openSpec(page, 'holy_paladin', 'settings-tab');
	await setNumber(incomingDamage(page), 4321);

	await reloadSim(page);
	await openSimTab(page, 'settings-tab');
	await expect(incomingDamage(page)).toHaveValue('4321');

	await page.locator('.sim-toolbar .sim-options').click();
	await page.locator('.modal.show .restore-defaults-button').click();
	await page.locator('.modal.show .close-button').click();
	await expect(incomingDamage(page)).toHaveValue(String(pageDefault('holy_paladin', 'default', DEFAULT_PHASE)));
});

test('the Resto Druid and Holy Paladin pages start with Wrath of Air on', async ({ page }) => {
	for (const spec of ['restoration_druid', 'holy_paladin']) {
		await openSpec(page, spec, 'settings-tab');
		await expectActive(icon(settingsTab(page).locator('.buffs-settings'), 'spell', 3738), true);
	}
});

for (const { spec } of PAGES) {
	test(`${spec}: a healer sim shows effective HPS, raw HPS and overheal %`, async ({ page }) => {
		await openSpec(page, spec);
		await setNumber(numberInput(page.locator('.sim-sidebar'), 'Iterations'), 50);
		await page.locator('.sim-sidebar-actions button', { hasText: 'Simulate' }).click();

		const results = page.locator('.sim-sidebar-results');
		const value = (cssClass: string) => results.locator(`.${cssClass} .topline-result-avg`);
		await expect(value('results-sim-overheal')).toBeVisible({ timeout: 120_000 });

		const hps = Number(await value('results-sim-hps').innerText());
		const raw = Number(await value('results-sim-raw-hps').innerText());
		const overheal = Number(await value('results-sim-overheal').innerText());
		expect(hps).toBeGreaterThan(1000);
		expect(raw).toBeGreaterThanOrEqual(hps);
		expect(overheal).toBeGreaterThanOrEqual(0);
		expect(overheal).toBeLessThan(100);
		// the overheal share is what separates the two HPS
		expect(Math.abs((1 - overheal / 100) * raw - hps)).toBeLessThan(0.02 * raw);
	});
}
