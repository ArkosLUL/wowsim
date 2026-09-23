import { expect, type Locator, type Page, test } from '@playwright/test';

import { openSimTab } from '../lib/page';
import { closePicker, type Db, equippedId, equipRow, listSize, loadDb, modal, openGear, openPicker, pane, rowEnchants, rowIds, showTab } from './gear';

// Deathknight swaps main and off hand.
const SPEC = 'deathknight';
const TWO_HAND = 4;

const swapRoot = (page: Page) => page.locator('#settings-tab .item-swap-picker-root');
const swapIcon = (page: Page, i: number) => swapRoot(page).locator('.icon-picker-button').nth(i);
const iconId = async (icon: Locator) => Number(/item=(\d+)/.exec((await icon.getAttribute('href')) ?? '')?.[1] ?? 0);

async function enableSwap(page: Page) {
	await openSimTab(page, 'settings-tab');
	await swapRoot(page).locator('.boolean-picker-root input').check();
	await expect(swapIcon(page, 0)).toBeVisible();
}

async function openSwapPicker(page: Page, i: number): Promise<Locator> {
	await swapIcon(page, i).click();
	await expect(modal(page)).toBeVisible();
	return modal(page);
}

// A main hand whose two-handedness differs from the worn one, so enchant rules tell them apart.
async function contrastingWeapon(db: Db, items: Locator, wornId: number): Promise<number> {
	const wornTwoHand = db.items.get(wornId)?.handType == TWO_HAND;
	return (await rowIds(items)).find(id => id != wornId && (db.items.get(id)?.handType == TWO_HAND) != wornTwoHand)!;
}

async function enchantIds(tab: Locator, db: Db): Promise<{ size: number; ids: number[] }> {
	return { size: await listSize(tab), ids: (await rowEnchants(tab, db)).map(e => e.effectId).sort() };
}

test('a swap weapon shows on its icon, survives a reload, and the swap button trades it with the worn one', async ({ page }) => {
	await openGear(page, SPEC);
	const db = await loadDb(page);
	const wornMh = await equippedId(page, 'mainHand');
	await enableSwap(page);

	const dialog = await openSwapPicker(page, 0);
	const pick = (await rowIds(pane(dialog, 'Items'))).find(id => id != wornMh)!;
	await equipRow(pane(dialog, 'Items'), pick);
	await closePicker(page);
	await expect(swapIcon(page, 0)).toHaveClass(/active/);
	await expect.poll(() => iconId(swapIcon(page, 0))).toBe(pick);

	await page.reload();
	await page.waitForLoadState('networkidle');
	await openSimTab(page, 'settings-tab');
	await expect(swapRoot(page).locator('.boolean-picker-root input')).toBeChecked();
	await expect.poll(() => iconId(swapIcon(page, 0))).toBe(pick);

	await swapRoot(page).locator('.gear-swap-icon').click();
	await expect.poll(() => iconId(swapIcon(page, 0))).toBe(wornMh);
	await openSimTab(page, 'gear-tab');
	await expect.poll(() => equippedId(page, 'mainHand')).toBe(pick);
	expect(db.items.get(pick)).toBeTruthy();
});

test('unequipping the swap weapon empties its icon, and turning swapping off hides the icons', async ({ page }) => {
	await openGear(page, SPEC);
	await enableSwap(page);
	const dialog = await openSwapPicker(page, 0);
	const items = pane(dialog, 'Items');
	await equipRow(items, (await rowIds(items))[0]);
	await expect(swapIcon(page, 0)).toHaveClass(/active/);
	await items.locator('.selector-modal-remove-button').click();
	await closePicker(page);
	await expect(swapIcon(page, 0)).not.toHaveClass(/active/);
	await expect(swapIcon(page, 0)).toHaveCSS('background-image', /item_slots\/mainhand\.jpg/);

	await swapRoot(page).locator('.boolean-picker-root input').uncheck();
	await expect(swapIcon(page, 0)).toBeHidden();
});

test('the swap weapon is offered the enchants that fit it, not the worn weapon', async ({ page }) => {
	await openGear(page, SPEC);
	const db = await loadDb(page);
	const wornMh = await equippedId(page, 'mainHand');
	await enableSwap(page);

	const swap = await openSwapPicker(page, 0);
	const pick = await contrastingWeapon(db, pane(swap, 'Items'), wornMh);
	expect(pick, 'a listed main hand of the other handedness').toBeTruthy();
	await equipRow(pane(swap, 'Items'), pick);
	const forSwap = await enchantIds(await showTab(swap, 'Enchants'), db);
	await closePicker(page);

	// the same weapon worn for real is the reference
	await openSimTab(page, 'gear-tab');
	const worn = await openPicker(page, 'mainHand');
	await equipRow(pane(worn, 'Items'), pick);
	await expect.poll(() => equippedId(page, 'mainHand')).toBe(pick);
	const forWorn = await enchantIds(await showTab(worn, 'Enchants'), db);
	expect(forSwap).toEqual(forWorn);
});
