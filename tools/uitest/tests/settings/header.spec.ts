import { expect, type Page, test } from '@playwright/test';

import { SPECS } from '../lib/page';
import {
	checkbox,
	expectActive,
	icon,
	isActive,
	numberInput,
	openSpec,
	reloadSim,
	savedData,
	select,
	setNumber,
	storedSettings,
	stubWowheadTooltips,
} from './helpers';

const SPEC = 'mage';

async function openOptions(page: Page) {
	await page.locator('.sim-toolbar .sim-options').click();
	const modal = page.locator('.modal.show:has(.settings-menu)');
	await expect(modal).toBeVisible();
	return modal;
}

async function closeModal(page: Page) {
	await page.locator('.modal.show .close-button').click();
	await expect(page.locator('.modal.show')).toHaveCount(0);
}

test('the sim options survive a reload', async ({ page }) => {
	await openSpec(page, SPEC);
	const batchTab = page.locator('a[data-bs-target="#bulk-tab"]');
	await expect(page.locator('.sim-ui')).toHaveClass(/hide-threat-metrics/);
	await expect(batchTab).toBeHidden();

	let options = await openOptions(page);
	await setNumber(numberInput(options, 'Fixed RNG Seed'), 1234);
	await checkbox(options, 'Show Threat/Tank Options').setChecked(true);
	await checkbox(options, 'Show Experimental').setChecked(true);
	await closeModal(page);

	await expect(page.locator('.sim-ui')).not.toHaveClass(/hide-threat-metrics/);
	await expect(batchTab).toBeVisible();

	await reloadSim(page);
	await expect(page.locator('.sim-ui')).not.toHaveClass(/hide-threat-metrics/);
	await expect(batchTab).toBeVisible();
	options = await openOptions(page);
	await expect(numberInput(options, 'Fixed RNG Seed')).toHaveValue('1234');
	await expect(checkbox(options, 'Show Threat/Tank Options')).toBeChecked();
	await expect(checkbox(options, 'Show Experimental')).toBeChecked();
});

test('picking a language reloads the page in it', async ({ page }) => {
	await openSpec(page, SPEC);
	const options = await openOptions(page);

	const reloaded = page.waitForEvent('load');
	await select(options, 'Language').selectOption({ label: 'Deutsch' });
	await reloaded;
	await page.waitForLoadState('networkidle');

	expect((await storedSettings(page, SPEC)).settings.language).toBe('de');
	await expect(select(await openOptions(page), 'Language').locator('option:checked')).toHaveText('Deutsch');
});

test('Restore Defaults puts the settings back but keeps what was saved', async ({ page }) => {
	await openSpec(page, SPEC, 'settings-tab');
	const settings = page.locator('#settings-tab');
	const duration = numberInput(settings.locator('.encounter-settings'), 'Duration');
	const lust = icon(settings.locator('.buffs-settings'), 'spell', 2825);
	const defaultDuration = await duration.inputValue();
	const lustWas = await isActive(lust);

	await setNumber(duration, 250);
	await lust.click();
	await savedData(settings, 'Saved Settings').save('Keep me');

	const options = await openOptions(page);
	await options.locator('.restore-defaults-button').click();
	await closeModal(page);

	await expect(duration).toHaveValue(defaultDuration);
	await expectActive(lust, lustWas);
	await expect(savedData(settings, 'Saved Settings').chip('Keep me')).toHaveCount(1);

	await reloadSim(page);
	await expect(duration).toHaveValue(defaultDuration);
	await expect(savedData(settings, 'Saved Settings').chip('Keep me')).toHaveCount(1);
});

test('the iteration count survives a reload', async ({ page }) => {
	await openSpec(page, SPEC);
	const iterations = numberInput(page.locator('.sim-sidebar'), 'Iterations');

	await setNumber(iterations, 77);
	await reloadSim(page);

	await expect(iterations).toHaveValue('77');
	expect((await storedSettings(page, SPEC)).settings.iterations).toBe(77);
});

const titleMenu = (page: Page) => page.locator('.sim-title .sim-link-dropdown > .dropdown-menu').first();

test('the sim title menu links every sim page, and each one opens', async ({ page }) => {
	await openSpec(page, SPEC);
	await page.locator('.sim-title .sim-link').first().click();
	await expect(titleMenu(page)).toBeVisible();

	const hrefs = await titleMenu(page)
		.locator('a.sim-link')
		.evaluateAll(links => links.map(a => a.getAttribute('href')!).filter(href => !href.startsWith('javascript')));
	const paths = hrefs.map(href => new URL(href).pathname);

	expect(paths).toContain('/wotlk/raid/');
	expect(paths).toContain('/wotlk/rankings/');
	for (const spec of SPECS) {
		// Unlaunched sims stay out of the menu.
		if (['restoration_druid', 'restoration_shaman', 'holy_paladin'].includes(spec)) {
			expect(paths).not.toContain(`/wotlk/${spec}/`);
		} else {
			expect(paths).toContain(`/wotlk/${spec}/`);
		}
	}
	for (const href of new Set(hrefs)) {
		expect((await page.request.get(href)).status(), href).toBe(200);
	}
});

test("a class's specs open from its submenu", async ({ page }) => {
	await openSpec(page, SPEC);
	await page.locator('.sim-title .sim-link').first().click();

	const druid = titleMenu(page).locator('.dropend:has(> a.sim-link:has-text("Druid"))');
	await druid.locator('> a.sim-link').hover();
	const feral = druid.locator('.dropdown-menu a.sim-link:has-text("Feral Tank")');
	await expect(feral).toBeVisible();

	await feral.click();
	await expect(page).toHaveURL(/\/wotlk\/feral_tank_druid\/$/);
	await page.waitForLoadState('networkidle');
	await expect(page.locator('.sim-title .sim-link-title').first()).toHaveText('Feral Tank Druid');
});

test('clicking a class name keeps the menu open instead of navigating', async ({ page }) => {
	await openSpec(page, SPEC);
	await page.locator('.sim-title .sim-link').first().click();

	const warrior = titleMenu(page).locator('.dropend > a.sim-link:has-text("Warrior")');
	await warrior.click();
	await expect(titleMenu(page)).toBeVisible();
	await expect(page).toHaveURL(/\/wotlk\/mage\/$/);
});

const downloadLink = (page: Page) => page.locator('.sim-toolbar .downbin');

// e.g. a laptop or phone opening the sim by the PC's LAN address
test('a page the sim server serves has no download link under any host name', async ({ page, baseURL }) => {
	const url = new URL(`/wotlk/${SPEC}/`, baseURL);
	if (url.hostname == 'localhost') url.hostname = '127.0.0.1';
	await stubWowheadTooltips(page);
	const version = page.waitForResponse(response => new URL(response.url()).pathname == '/version');
	await page.goto(url.toString());
	await page.waitForLoadState('networkidle');
	await expect(page.locator('.sim-ui')).toBeVisible();

	await version;
	await expect(downloadLink(page)).toHaveCount(0);
});

test('a static host without /version shows the download link', async ({ page }) => {
	await page.route('**/version', route => route.fulfill({ status: 404, body: '404 page not found' }));
	await openSpec(page, SPEC);
	await expect(downloadLink(page)).toHaveCount(1);
});
