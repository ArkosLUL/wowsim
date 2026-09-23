import { expect, type Page } from '@playwright/test';

// Every spec page under ui/, by its directory name, which is also its URL path.
export const SPECS = [
	'balance_druid',
	'deathknight',
	'elemental_shaman',
	'enhancement_shaman',
	'feral_druid',
	'feral_tank_druid',
	'healing_priest',
	'holy_paladin',
	'hunter',
	'mage',
	'protection_paladin',
	'protection_warrior',
	'restoration_druid',
	'restoration_shaman',
	'retribution_paladin',
	'rogue',
	'shadow_priest',
	'smite_priest',
	'tank_deathknight',
	'warlock',
	'warrior',
];

// A file the network dropped (net::ERR_*): a busy machine drops those, the page didn't. A file the
// server answered with an error, like a 404, still counts.
const NETWORK_DROP = /^Failed to load resource: net::/;

// Collects uncaught errors and console errors. A component that throws while drawing is left half
// built, and nothing on the page says so.
export function watchForErrors(page: Page): string[] {
	const errors: string[] = [];
	page.on('pageerror', error => errors.push(error.message));
	page.on('console', message => {
		if (message.type() === 'error' && !NETWORK_DROP.test(message.text())) errors.push(message.text());
	});
	return errors;
}

// body.ready only means the DOM loaded; the item database arrives later, so wait for the network too.
async function openPage(page: Page, path: string) {
	await page.goto(path);
	await page.waitForLoadState('networkidle');
	await expect(page.locator('.sim-ui')).toBeVisible();
}

export async function openSim(page: Page, spec: string) {
	await openPage(page, `/wotlk/${spec}/`);
}

export async function openRaid(page: Page) {
	await openPage(page, '/wotlk/raid/');
}

// Switches a sim page's top-level tab, e.g. 'gear-tab', 'settings-tab', 'rotation-tab'.
export async function openSimTab(page: Page, id: string) {
	await page.locator(`a[data-bs-target="#${id}"]`).click();
	await expect(page.locator(`#${id}`)).toHaveClass(/show/);
}
