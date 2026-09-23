import { expect, test, type Locator, type Page } from '@playwright/test';

import { openSimTab } from '../lib/page';
import {
	acceptDialogs,
	expectActive,
	iconEnum,
	openSpec,
	pickFromDropdown,
	reloadSim,
	savedData,
	select,
	storedSettings,
} from './helpers';

const SPEC = 'mage';

const rotationTab = (page: Page) => page.locator('#rotation-tab');
const priorityList = (page: Page) => rotationTab(page).locator('.apl-list-item-picker > .list-picker-items > .list-picker-item-container');
const prepullList = (page: Page) => rotationTab(page).locator('.apl-prepull-action-picker > .list-picker-items > .list-picker-item-container');
const actionKind = (item: Locator) => item.locator('.apl-action-picker-action > .dropdown-picker-root').first();
// What a row shows for its action, e.g. 'Cast Fireball', without its condition.
const actionText = async (item: Locator) => (await item.locator('.apl-action-picker-action').first().innerText()).replace(/\s+/g, ' ').trim();

const CONST_INPUT = '.apl-value-picker-root:has(> .dropdown-picker-root > .dropdown-picker-button:text-is("Const")) .adaptive-string-picker-root input';

async function openRotation(page: Page) {
	await openSpec(page, SPEC, 'rotation-tab');
}

async function loadPreset(page: Page, name: string) {
	await savedData(rotationTab(page), 'Saved Rotations').chip(name).locator('.saved-data-set-name').click();
	await expectActive(savedData(rotationTab(page), 'Saved Rotations').chip(name), true);
}

async function reloadRotation(page: Page) {
	await reloadSim(page);
	await openSimTab(page, 'rotation-tab');
}

async function actionTexts(page: Page, items = priorityList(page)): Promise<string[]> {
	const texts: string[] = [];
	for (let i = 0; i < (await items.count()); i++) {
		texts.push(await actionText(items.nth(i)));
	}
	return texts;
}

test('the rotation type survives a reload', async ({ page }) => {
	await openRotation(page);
	const type = select(rotationTab(page).locator('.rotation-tab-header'), 'Rotation Type');

	await type.selectOption({ label: 'Simple' });
	await expect(rotationTab(page)).toHaveClass(/rotation-type-simple/);

	await reloadRotation(page);
	await expect(type.locator('option:checked')).toHaveText('Simple');
	await expect(rotationTab(page)).toHaveClass(/rotation-type-simple/);
});

test('an added action survives a reload', async ({ page }) => {
	await openRotation(page);
	await loadPreset(page, 'Fire');
	const count = await priorityList(page).count();

	await rotationTab(page).locator('.apl-list-item-picker > .list-picker-new-button').click();
	await expect(priorityList(page)).toHaveCount(count + 1);
	const added = priorityList(page).last();
	await pickFromDropdown(actionKind(added), 'Cast');
	await pickFromDropdown(added.locator('.apl-action-castSpell > .dropdown-picker-root').first(), 'Spells', 'Frostbolt');
	await expect.poll(() => actionText(added)).toBe('Cast Frostbolt');

	await reloadRotation(page);
	await expect(priorityList(page)).toHaveCount(count + 1);
	await expect.poll(() => actionText(priorityList(page).last())).toBe('Cast Frostbolt');
	const stored = (await storedSettings(page, SPEC)).player.rotation.priorityList;
	expect(stored[stored.length - 1].action.castSpell).toBeDefined();
});

test('deleting, copying and disabling actions survive a reload', async ({ page }) => {
	await openRotation(page);
	await loadPreset(page, 'Fire');
	const before = await actionTexts(page);

	await priorityList(page).last().locator('.list-picker-item-delete').click();
	await expect(priorityList(page)).toHaveCount(before.length - 1);
	await expectActive(savedData(rotationTab(page), 'Saved Rotations').chip('Fire'), false);

	await priorityList(page).nth(1).locator('.list-picker-item-copy').click();
	await expect(priorityList(page)).toHaveCount(before.length);

	await priorityList(page).nth(0).locator('.hide-picker-button').click();
	await expect(priorityList(page).nth(0).locator('.apl-list-item-picker-root')).toHaveClass(/disabled/);

	const expected = [before[0], before[1], before[1], ...before.slice(2, -1)];
	expect(await actionTexts(page)).toEqual(expected);

	await reloadRotation(page);
	expect(await actionTexts(page)).toEqual(expected);
	await expect(priorityList(page).nth(0).locator('.apl-list-item-picker-root')).toHaveClass(/disabled/);
	await expect(priorityList(page).nth(1).locator('.apl-list-item-picker-root')).not.toHaveClass(/disabled/);
});

test('dragging an action reorders the list, and the order survives a reload', async ({ page }) => {
	await openRotation(page);
	await loadPreset(page, 'Fire');
	const before = await actionTexts(page);

	await priorityList(page).nth(2).locator('.list-picker-item-move').dragTo(priorityList(page).nth(0));

	const expected = [before[2], before[0], before[1], ...before.slice(3)];
	await expect.poll(() => actionTexts(page)).toEqual(expected);

	await reloadRotation(page);
	expect(await actionTexts(page)).toEqual(expected);
});

test('an edited value survives a reload', async ({ page }) => {
	await openRotation(page);
	await loadPreset(page, 'Fire');

	// The first constant in the list, like a Scorch refresh window.
	const constant = priorityList(page).locator(CONST_INPUT).first();
	await constant.fill('7s');
	await constant.blur();

	await reloadRotation(page);
	await expect(priorityList(page).locator(CONST_INPUT).first()).toHaveValue('7s');
});

test('a condition built from nested values survives a reload', async ({ page }) => {
	await openRotation(page);
	await loadPreset(page, 'Fire');
	await rotationTab(page).locator('.apl-list-item-picker > .list-picker-new-button').click();
	const added = priorityList(page).last();
	await pickFromDropdown(actionKind(added), 'Cast');
	await pickFromDropdown(added.locator('.apl-action-castSpell > .dropdown-picker-root').first(), 'Spells', 'Frostbolt');

	const condition = added.locator('.apl-action-condition');
	await pickFromDropdown(condition.locator(':scope > .dropdown-picker-root'), 'Logic', 'Compare');
	const compare = condition.locator(':scope > .apl-picker-builder-root');
	await pickFromDropdown(compare.locator(':scope > .apl-value-picker-root').nth(0).locator(':scope > .dropdown-picker-root'), 'Encounter', 'Remaining Time');
	await pickFromDropdown(compare.locator(':scope > .dropdown-picker-root'), '<');
	await pickFromDropdown(compare.locator(':scope > .apl-value-picker-root').nth(1).locator(':scope > .dropdown-picker-root'), 'Const');
	const threshold = compare.locator('.adaptive-string-picker-root input');
	await threshold.fill('30s');
	await threshold.blur();

	await reloadRotation(page);
	const reloaded = priorityList(page).last().locator('.apl-action-condition');
	await expect.poll(async () => (await reloaded.innerText()).replace(/\s+/g, ' ').trim()).toBe('If: Compare Remaining Time < Const');
	await expect(reloaded.locator('.adaptive-string-picker-root input')).toHaveValue('30s');
	const stored = (await storedSettings(page, SPEC)).player.rotation.priorityList;
	expect(stored[stored.length - 1].action.condition).toEqual({
		cmp: { op: 'OpLt', lhs: { remainingTime: {} }, rhs: { const: { val: '30s' } } },
	});
});

test('prepull actions can be added and timed, and survive a reload', async ({ page }) => {
	await openRotation(page);
	await loadPreset(page, 'Fire');
	const count = await prepullList(page).count();

	await rotationTab(page).locator('.apl-prepull-action-picker > .list-picker-new-button').click();
	const added = prepullList(page).last();
	await pickFromDropdown(actionKind(added), 'Cast');
	await pickFromDropdown(added.locator('.apl-action-castSpell > .dropdown-picker-root').first(), 'Spells', 'Frostbolt');
	const doAt = added.locator('.apl-prepull-actions-doat input');
	await doAt.fill('-3s');
	await doAt.blur();

	await reloadRotation(page);
	await expect(prepullList(page)).toHaveCount(count + 1);
	await expect(prepullList(page).last().locator('.apl-prepull-actions-doat input')).toHaveValue('-3s');
	await expect.poll(() => actionText(prepullList(page).last())).toBe('Cast Frostbolt');
});

test('prepull actions can be reordered and removed, and survive a reload', async ({ page }) => {
	await openRotation(page);
	await loadPreset(page, 'Fire');
	await rotationTab(page).locator('.apl-prepull-action-picker > .list-picker-new-button').click();
	const added = prepullList(page).last();
	await pickFromDropdown(actionKind(added), 'Cast');
	await pickFromDropdown(added.locator('.apl-action-castSpell > .dropdown-picker-root').first(), 'Spells', 'Frostbolt');
	const before = await actionTexts(page, prepullList(page));
	expect(before.length).toBeGreaterThanOrEqual(3);

	await prepullList(page).last().locator('.list-picker-item-move').dragTo(prepullList(page).first());
	const moved = [before[before.length - 1], ...before.slice(0, -1)];
	await expect.poll(() => actionTexts(page, prepullList(page))).toEqual(moved);

	await prepullList(page).nth(1).locator('.list-picker-item-delete').click();
	const expected = [moved[0], ...moved.slice(2)];
	await expect.poll(() => actionTexts(page, prepullList(page))).toEqual(expected);

	await reloadRotation(page);
	expect(await actionTexts(page, prepullList(page))).toEqual(expected);
});

test('saved rotations save, load and delete, and survive a reload', async ({ page }) => {
	acceptDialogs(page);
	await openRotation(page);
	const saved = savedData(rotationTab(page), 'Saved Rotations');
	await loadPreset(page, 'Fire');
	await priorityList(page).last().locator('.list-picker-item-delete').click();
	const mine = await actionTexts(page);

	await saved.save('Short fire');
	await expectActive(saved.chip('Short fire'), true);
	await loadPreset(page, 'Fire AOE');

	await reloadRotation(page);
	await saved.chip('Short fire').locator('.saved-data-set-name').click();
	expect(await actionTexts(page)).toEqual(mine);

	await saved.chip('Short fire').locator('.saved-data-set-delete').click();
	await reloadRotation(page);
	await expect(saved.chip('Short fire')).toHaveCount(0);
});

test('simple rotation cooldowns survive a reload', async ({ page }) => {
	await openRotation(page);
	await select(rotationTab(page).locator('.rotation-tab-header'), 'Rotation Type').selectOption({ label: 'Simple' });
	const cooldowns = rotationTab(page).locator('.cooldowns-picker-root .cooldown-picker');
	const count = await cooldowns.count();

	// The last row is the empty one that adds a cooldown. Mirror Image is a major cooldown every mage has.
	await iconEnum(cooldowns.last(), 'spell', 55342).pick('spell', 55342);
	await expect(cooldowns).toHaveCount(count + 1);
	const timings = cooldowns.nth(count - 1).locator('.cooldown-timings-picker input');
	await timings.fill('20, 140');
	await timings.blur();

	await reloadRotation(page);
	await expect(cooldowns).toHaveCount(count + 1);
	await expect(cooldowns.nth(count - 1).locator('.cooldown-picker-label')).toHaveText('Mirror Image');
	await expect(cooldowns.nth(count - 1).locator('.cooldown-timings-picker input')).toHaveValue('20,140');
});
