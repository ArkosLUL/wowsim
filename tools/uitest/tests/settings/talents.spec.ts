import { expect, test, type Locator, type Page } from '@playwright/test';

import { openSimTab } from '../lib/page';
import { acceptDialogs, expectActive, openSpec, reloadSim, savedData, storedSettings } from './helpers';

const SPEC = 'mage';

const talentsTab = (page: Page) => page.locator('#talents-tab');
const pointsRemaining = (scope: Locator) => scope.locator('.talents-picker-header .talent-tree-points');
const tree = (scope: Locator, name: string) => scope.locator(`.talent-tree-picker-root:has(.talent-tree-title:text-is("${name}"))`);
const treePoints = (treeElem: Locator) => treeElem.locator('.talent-tree-header .talent-tree-points');
// Rows and columns count from 1, as the talent grid places them.
const talent = (treeElem: Locator, row: number, col: number) => treeElem.locator(`.talent-picker-root[style*="grid-area: ${row} / ${col};"]`);

async function addPoints(talentElem: Locator, count: number) {
	await talentElem.click({ clickCount: count });
}

async function openTalents(page: Page, spec = SPEC) {
	await openSpec(page, spec, 'talents-tab');
}

async function resetAll(page: Page) {
	const resets = talentsTab(page).locator('.talent-tree-reset');
	for (let i = 0; i < (await resets.count()); i++) {
		await resets.nth(i).click();
	}
	await expect(pointsRemaining(talentsTab(page))).toHaveText('71');
}

test('spending, removing and resetting points, and the talents survive a reload', async ({ page }) => {
	await openTalents(page);
	await resetAll(page);
	const frost = tree(talentsTab(page), 'Frost');
	const frostbolt = talent(frost, 1, 2);

	await addPoints(frostbolt, 3);
	await expect(frostbolt).toHaveAttribute('data-points', '3');
	await expect(pointsRemaining(talentsTab(page))).toHaveText('68');
	await expect(treePoints(frost)).toHaveText('3 / 71');

	await frostbolt.click({ button: 'right' });
	await expect(frostbolt).toHaveAttribute('data-points', '2');

	await reloadSim(page);
	await openSimTab(page, 'talents-tab');

	await expect(frostbolt).toHaveAttribute('data-points', '2');
	await expect(treePoints(tree(talentsTab(page), 'Arcane'))).toHaveText('0 / 71');
	await expect(pointsRemaining(talentsTab(page))).toHaveText('69');
	expect((await storedSettings(page, SPEC)).player.talentsString).toBe('--02');
});

test('a talent row opens after five points in the rows above, and holds them', async ({ page }) => {
	await openTalents(page);
	await resetAll(page);
	const fire = tree(talentsTab(page), 'Fire');
	const ignite = talent(fire, 2, 1);
	const fireball = talent(fire, 1, 3);

	await ignite.click();
	await expect(ignite).toHaveAttribute('data-points', '0');

	await addPoints(fireball, 5);
	await ignite.click();
	await expect(ignite).toHaveAttribute('data-points', '1');

	// Taking a point out of row 1 would leave ignite without its five.
	await fireball.click({ button: 'right' });
	await expect(fireball).toHaveAttribute('data-points', '5');
});

test('a talent needs its prerequisite maxed, which then holds its points', async ({ page }) => {
	await openTalents(page);
	await resetAll(page);
	const fire = tree(talentsTab(page), 'Fire');
	for (const [col, max] of [[1, 2], [2, 3], [3, 5]]) await addPoints(talent(fire, 1, col), max);
	for (const [col, max] of [[1, 5], [2, 2], [3, 3]]) await addPoints(talent(fire, 2, col), max);
	await expect(treePoints(fire)).toHaveText('20 / 71');

	const pyroblast = talent(fire, 3, 3);
	const blastWave = talent(fire, 5, 3);
	await blastWave.click();
	await expect(blastWave).toHaveAttribute('data-points', '0');

	await pyroblast.click();
	await blastWave.click();
	await expect(blastWave).toHaveAttribute('data-points', '1');

	await pyroblast.click({ button: 'right' });
	await expect(pyroblast).toHaveAttribute('data-points', '1');
});

const glyphModal = (page: Page) => page.locator('.modal.show:has(.glyph-modal)');
const glyphSlot = (page: Page, i: number) => talentsTab(page).locator('.glyph-picker-root').nth(i).locator('.glyph-picker-icon');

test('a picked glyph survives a reload', async ({ page }) => {
	await openTalents(page);
	const slot = glyphSlot(page, 0);

	await slot.click();
	const modal = glyphModal(page);
	await expect(modal).toBeVisible();
	const choice = modal.locator('.selector-modal-list-item:not(.active)').nth(3).locator('.selector-modal-list-item-link');
	const href = (await choice.getAttribute('href'))!;
	await choice.click();
	await expect(slot).toHaveAttribute('href', href);

	await reloadSim(page);
	await openSimTab(page, 'talents-tab');
	await expect(slot).toHaveAttribute('href', href);
});

test('Enter in the glyph search picks the first glyph found', async ({ page }) => {
	await openTalents(page);
	const slot = glyphSlot(page, 5);

	await slot.click();
	const modal = glyphModal(page);
	const search = modal.locator('.selector-modal-search');
	await search.fill('slow fall');
	const found = modal.locator('.selector-modal-list-item');
	await expect(found).toHaveCount(1);
	const href = (await found.locator('.selector-modal-list-item-link').getAttribute('href'))!;

	await search.press('Enter');
	await expect(slot).toHaveAttribute('href', href);
});

test('saved talents save, load and delete, and survive a reload', async ({ page }) => {
	acceptDialogs(page);
	await openTalents(page);
	const saved = savedData(talentsTab(page), 'Saved Talents');
	const before = (await storedSettings(page, SPEC)).player.talentsString;

	await saved.save('My build');
	await expectActive(saved.chip('My build'), true);

	await saved.chip('Arcane').locator('.saved-data-set-name').click();
	await expectActive(saved.chip('Arcane'), true);
	await expectActive(saved.chip('My build'), false);

	await reloadSim(page);
	await openSimTab(page, 'talents-tab');

	await saved.chip('My build').locator('.saved-data-set-name').click();
	await expectActive(saved.chip('My build'), true);
	expect((await storedSettings(page, SPEC)).player.talentsString).toBe(before);

	await saved.chip('My build').locator('.saved-data-set-delete').click();
	await expect(saved.chip('My build')).toHaveCount(0);
	await reloadSim(page);
	await expect(saved.chip('My build')).toHaveCount(0);
});

test("saving talents under a preset's name is refused", async ({ page }) => {
	const dialogs = acceptDialogs(page);
	await openTalents(page);
	const saved = savedData(talentsTab(page), 'Saved Talents');

	await saved.save('Arcane');

	await expect(saved.chip('Arcane')).toHaveCount(1);
	expect(dialogs).toEqual(['Talents with name Arcane already exists.']);
});

test('pet talents count their own points against the pet budget', async ({ page }) => {
	await openTalents(page, 'hunter');
	await talentsTab(page).locator('a[data-bs-target="#pet-talents-tab"]').click();
	const pet = page.locator('#pet-talents-tab .talents-picker-root:visible');
	const petTree = pet.locator('.talent-tree-picker-root');

	await expect(treePoints(petTree)).toHaveText(/^\d+ \/ 16$/);
	const spent = Number((await treePoints(petTree).innerText()).split(' / ')[0]);
	await expect(pointsRemaining(pet)).toHaveText(String(16 - spent));
});

test('Beast Mastery raises the pet budget to what the server gives', async ({ page }) => {
	await openTalents(page, 'hunter');
	await savedData(talentsTab(page), 'Saved Talents').chip('Beast Mastery').locator('.saved-data-set-name').click();
	await talentsTab(page).locator('a[data-bs-target="#pet-talents-tab"]').click();
	const pet = page.locator('#pet-talents-tab .talents-picker-root:visible');
	const petTree = pet.locator('.talent-tree-picker-root');

	await expect(treePoints(petTree)).toHaveText(/^\d+ \/ 22$/);
	const spent = Number((await treePoints(petTree).innerText()).split(' / ')[0]);
	await expect(pointsRemaining(pet)).toHaveText(String(22 - spent));
});

test('a pet of another family brings its own talent tree, and both survive a reload', async ({ page }) => {
	await openTalents(page, 'hunter');
	await talentsTab(page).locator('a[data-bs-target="#pet-talents-tab"]').click();
	const petType = page.locator('#pet-talents-tab .icon-enum-picker-root');
	const button = petType.locator(':scope > a.icon-picker-button');
	const pet = page.locator('#pet-talents-tab .talents-picker-root:visible');
	const family = pet.locator('.talent-tree-title');
	const oldFamily = await family.innerText();

	// Pets only show as icons, so go through them until one is from another family.
	const options = petType.locator('.dropdown-menu a.icon-picker-button');
	for (let i = 1; i < (await options.count()) && (await family.innerText()) == oldFamily; i++) {
		await button.hover();
		if (await options.nth(i).isVisible()) await options.nth(i).click();
	}
	const newFamily = await family.innerText();
	expect(newFamily).not.toBe(oldFamily);
	const petIcon = await button.evaluate(e => (e as HTMLElement).style.backgroundImage);
	const [spent, budget] = (await treePoints(pet.locator('.talent-tree-picker-root')).innerText()).split(' / ').map(Number);
	await expect(pointsRemaining(pet)).toHaveText(String(budget - spent));

	await reloadSim(page);
	await openSimTab(page, 'talents-tab');
	await talentsTab(page).locator('a[data-bs-target="#pet-talents-tab"]').click();
	await expect(family).toHaveText(newFamily);
	expect(await button.evaluate(e => (e as HTMLElement).style.backgroundImage)).toBe(petIcon);
});
