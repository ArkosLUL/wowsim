import { expect, type Page, test } from '@playwright/test';

import { OptimizeGearRequest } from '../../../../ui/core/proto/api';
import { openSpec, reloadSim, storageKey } from './helpers';

const SPEC = 'mage';

const optimizerTab = (page: Page) => page.locator('#optimizer-tab');
const statCapsBox = (page: Page) => optimizerTab(page).getByLabel('Respect hit, expertise and haste caps', { exact: true });
const optimizeButton = (page: Page) => optimizerTab(page).getByRole('button', { name: 'Optimize', exact: true });

async function openOptimizer(page: Page, spec = SPEC) {
	await openSpec(page, spec, 'optimizer-tab');
}

test('stat caps start ticked and stick unticked across a reload', async ({ page }) => {
	await openOptimizer(page);
	await expect(statCapsBox(page)).toBeChecked();

	await statCapsBox(page).setChecked(false);
	await expect(statCapsBox(page)).not.toBeChecked();

	await reloadSim(page);
	await openSpec(page, SPEC, 'optimizer-tab');
	await expect(statCapsBox(page)).not.toBeChecked();
});

test('a page saved before the setting existed restores it ticked', async ({ page }) => {
	await openOptimizer(page);
	await statCapsBox(page).setChecked(false);
	await expect(statCapsBox(page)).not.toBeChecked();

	const stripped = await page.evaluate(key => {
		const saved = JSON.parse(localStorage.getItem(key)!);
		delete saved.respectStatCaps;
		localStorage.setItem(key, JSON.stringify(saved));
		return true;
	}, storageKey(SPEC, 'optimizer-settings.v1'));
	expect(stripped).toBe(true);

	await reloadSim(page);
	await openSpec(page, SPEC, 'optimizer-tab');
	await expect(statCapsBox(page)).toBeChecked();
});

// No real run: the route answers empty, so the run fails right after sending its request.
test('a run sends the ticked setting, unticking sends it off', async ({ page }) => {
	const requests: Array<OptimizeGearRequest> = [];
	await page.route('**/optimizeGearAsync', async route => {
		const body = route.request().postDataBuffer();
		if (body) requests.push(OptimizeGearRequest.fromBinary(body));
		await route.fulfill({ status: 200, body: '' });
	});
	await page.route('**/asyncProgress', route => route.fulfill({ status: 204 }));

	await openOptimizer(page);
	await expect(optimizeButton(page)).toBeEnabled();
	await optimizeButton(page).click();
	await expect.poll(() => requests.length).toBe(1);
	expect(requests[0].settings?.disableStatCaps).toBe(false);

	await statCapsBox(page).setChecked(false);
	await expect(optimizeButton(page)).toBeEnabled();
	await optimizeButton(page).click();
	await expect.poll(() => requests.length).toBe(2);
	expect(requests[1].settings?.disableStatCaps).toBe(true);
});
