import { expect, type Page, test } from '@playwright/test';

import { RaidSimRequest } from '../../../../ui/core/proto/api';
import { openSimTab } from '../lib/page';
import { numberInput, openSpec, reloadSim, setNumber } from './helpers';

// a mage's Mirror Image reads the reach, and a mage page hides the threat and tank options
const SPEC = 'mage';
const ITERATIONS = 50;

async function openFirstTarget(page: Page) {
	await page.locator('#settings-tab .encounter-settings .advanced-button').click();
	const modal = page.locator('.modal.show:has(.advanced-encounter-picker-modal)');
	await expect(modal).toBeVisible();
	return modal.locator('.targets-picker .target-picker-root').first();
}

async function closeModal(page: Page) {
	await page.locator('.modal.show .close-button').click();
	await expect(page.locator('.modal.show')).toHaveCount(0);
}

test('a Combat Reach set on the target survives a reload and goes out in the sim request', async ({ page }) => {
	await openSpec(page, SPEC, 'settings-tab');
	await expect(page.locator('.sim-ui')).toHaveClass(/(^|\s)hide-threat-metrics(\s|$)/);

	let target = await openFirstTarget(page);
	// the threat options stay hidden, the reach doesn't
	await expect(numberInput(target, 'Swing Speed')).toBeHidden();
	let reach = numberInput(target, 'Combat Reach');
	await expect(reach).toBeVisible();
	// 0 leaves it unset, which the sim takes as the boss dummy's 2.5
	await expect(reach).toHaveValue('0.00');
	await setNumber(reach, 4.5);
	await closeModal(page);

	await reloadSim(page);
	await openSimTab(page, 'settings-tab');
	target = await openFirstTarget(page);
	reach = numberInput(target, 'Combat Reach');
	await expect(reach).toHaveValue('4.50');
	await closeModal(page);
	await setNumber(numberInput(page.locator('.sim-sidebar'), 'Iterations'), ITERATIONS);

	const requests: RaidSimRequest[] = [];
	await page.route('**/raidSimAsync', async route => {
		const body = route.request().postDataBuffer();
		if (body) requests.push(RaidSimRequest.fromBinary(body));
		await route.continue();
	});
	await page.locator('.sim-sidebar-actions button', { hasText: 'Simulate' }).click();

	await expect.poll(() => requests.length).toBeGreaterThan(0);
	expect(requests[0].encounter?.targets[0].combatReach).toBe(4.5);
	// the server takes the request and sims it
	const dps = page.locator('.sim-sidebar-results .results-sim-dps .topline-result-avg');
	await expect.poll(async () => Number(await dps.innerText()), { timeout: 60_000 }).toBeGreaterThan(0);
});
