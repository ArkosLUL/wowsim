import { expect, type Page, test } from '@playwright/test';

import { OptimizeGearRequest } from '../../../../ui/core/proto/api';
import { ItemSlot } from '../../../../ui/core/proto/common';
import { closePicker, equippedId, equipRow, openPicker, pane, search } from '../gear/gear';
import { openSimTab } from '../lib/page';
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

const DRAGONSPINE_TROPHY = 28830;
const BUBBLING_BRIGHTBREW_CHARM = 49118;

// The slot pools a request offers the item in.
const poolsWith = (request: OptimizeGearRequest, id: number) => request.pool!.slots.filter(pool => pool.itemIds.includes(id)).map(pool => pool.slot);

test('outclassed gear stays out of the pool unless worn, and then only in its own slot', async ({ page }) => {
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
	expect(poolsWith(requests[0], DRAGONSPINE_TROPHY)).toEqual([]);
	// Brewfest loot reads vanilla in AtlasLoot, but it's item level 200
	expect(poolsWith(requests[0], BUBBLING_BRIGHTBREW_CHARM)).toEqual([ItemSlot.ItemSlotTrinket1, ItemSlot.ItemSlotTrinket2]);

	await openSimTab(page, 'gear-tab');
	const items = pane(await openPicker(page, 'trinket1'), 'Items');
	await search(items, 'Dragonspine Trophy');
	await equipRow(items, DRAGONSPINE_TROPHY);
	await closePicker(page);
	await expect.poll(() => equippedId(page, 'trinket1')).toBe(DRAGONSPINE_TROPHY);

	await openSimTab(page, 'optimizer-tab');
	await expect(optimizeButton(page)).toBeEnabled();
	await optimizeButton(page).click();
	await expect.poll(() => requests.length).toBe(2);
	expect(poolsWith(requests[1], DRAGONSPINE_TROPHY)).toEqual([ItemSlot.ItemSlotTrinket1]);
});
