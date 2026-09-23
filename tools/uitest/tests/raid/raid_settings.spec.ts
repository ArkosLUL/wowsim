import { expect, type Locator, type Page, test } from '@playwright/test';

import { openRaid, openSimTab } from '../lib/page';
import {
	addPreset,
	assignmentPicker,
	collectDialogs,
	fixtureRoster,
	fixtureSettings,
	importRoster,
	pickTarget,
	readRoster,
	reloadRaid,
	removePlayer,
	renamePlayer,
	roster,
	seedRaid,
	statCount,
	swapPlayers,
	tankNames,
	tankPickers,
	targetName,
} from './raid';

const fixtureTanks = () => {
	const names = fixtureRoster();
	const tanks = fixtureSettings().raid.tanks.map((tank: any) => names[tank.index ?? 0]);
	return [...tanks, 'Unassigned', 'Unassigned', 'Unassigned', 'Unassigned'].slice(0, 4);
};

const settingsTab = (page: Page) => openSimTab(page, 'raid-settings-tab');
const raidTab = (page: Page) => openSimTab(page, 'raid-tab');

test.describe('tanks', () => {
	test('the picker shows the raid tanks, and a picked one sticks', async ({ page }) => {
		await seedRaid(page);
		await settingsTab(page);
		await expect.poll(() => tankNames(page)).toEqual(fixtureTanks());

		await pickTarget(tankPickers(page).nth(1), 'Fury');
		const expected = fixtureTanks();
		expected[1] = 'Fury';
		await expect.poll(() => tankNames(page)).toEqual(expected);

		await reloadRaid(page);
		await settingsTab(page);
		await expect.poll(() => tankNames(page)).toEqual(expected);
	});

	test('a tank stays a tank when moved to another group', async ({ page }) => {
		await seedRaid(page);
		await swapPlayers(page, 0, 7);

		await settingsTab(page);
		await expect.poll(() => tankNames(page)).toEqual(fixtureTanks());
		await reloadRaid(page);
		await settingsTab(page);
		await expect.poll(() => tankNames(page)).toEqual(fixtureTanks());
	});

	test('a renamed tank shows up under the new name', async ({ page }) => {
		await seedRaid(page);
		await renamePlayer(page, 0, 'Bobtank');

		await settingsTab(page);
		const expected = fixtureTanks();
		expected[0] = 'Bobtank';
		await expect.poll(() => tankNames(page)).toEqual(expected);
		await expect(tankPickers(page).nth(1).locator('.dropdown-option', { hasText: 'Bobtank' })).toHaveCount(1);
	});

	test('a new tank dropped on the main tank takes their place', async ({ page }) => {
		await seedRaid(page);
		await addPreset(page, 'Protection Warrior', 0);
		const newTank = (await roster(page))[0];
		expect(newTank).not.toBe(fixtureRoster()[0]);

		await settingsTab(page);
		const expected = fixtureTanks();
		expected[0] = newTank;
		await expect.poll(() => tankNames(page)).toEqual(expected);
	});

	test('tanks added one by one fill every tank slot', async ({ page }) => {
		await openRaid(page);
		for (let i = 0; i < 4; i++) {
			await addPreset(page, 'Protection Warrior', i);
		}
		const tanks = (await roster(page)).slice(0, 4);
		expect(tanks).not.toContain('');

		await settingsTab(page);
		await expect.poll(() => tankNames(page)).toEqual(tanks);
	});

	test('a removed tank leaves their place unassigned', async ({ page }) => {
		await seedRaid(page);
		await removePlayer(page, 0);

		await settingsTab(page);
		const expected = fixtureTanks();
		expected[0] = 'Unassigned';
		await expect.poll(() => tankNames(page)).toEqual(expected);
		await reloadRaid(page);
		await settingsTab(page);
		await expect.poll(() => tankNames(page)).toEqual(expected);
	});
});

test('an external buff target sticks and follows its raider around', async ({ page }) => {
	await seedRaid(page);
	await settingsTab(page);
	const innervate = assignmentPicker(page, 'Innervate', 'Balance');

	await pickTarget(innervate, 'Arcane');
	await expect.poll(() => targetName(innervate)).toBe('Arcane');

	await raidTab(page);
	const names = fixtureRoster();
	await swapPlayers(page, names.indexOf('Arcane'), names.indexOf('Fury'));
	await settingsTab(page);
	await expect.poll(() => targetName(innervate)).toBe('Arcane');

	await reloadRaid(page);
	await settingsTab(page);
	await expect.poll(() => targetName(innervate)).toBe('Arcane');
});

test.describe('blessings', () => {
	const paladins = () =>
		fixtureSettings()
			.raid.parties.flatMap((party: any) => party.players)
			.filter((player: any) => player.class == 'ClassPaladin');
	// one row per spec, one picker per paladin
	const firstRow = (page: Page) => page.locator('.blessings-picker-row').first().locator('.blessing-picker');
	const enabled = (pickers: Locator) => pickers.and(pickers.page().locator(':not(.disabled)'));
	const icon = (elem: Locator) => elem.evaluate(e => getComputedStyle(e).backgroundImage);
	const button = (picker: Locator) => picker.locator('> .icon-picker-button');
	const option = (picker: Locator, index: number) => picker.locator('.dropdown-option').nth(index).locator('a');
	// options go Unassigned, Kings, Might, Wisdom, Sanctuary
	const UNASSIGNED = 0;
	const MIGHT = 2;

	async function pickBlessing(picker: Locator, index: number) {
		await button(picker).hover();
		await expect(option(picker, index)).toBeVisible();
		// wowhead's tooltip for the hovered blessing covers the top options
		await option(picker, index).dispatchEvent('click');
	}

	async function expectBlessing(picker: Locator, index: number) {
		const expected = await icon(option(picker, index));
		await expect.poll(() => icon(button(picker))).toBe(expected);
		if (index == UNASSIGNED) {
			await expect(button(picker)).not.toHaveClass(/active/);
		} else {
			await expect(button(picker)).toHaveClass(/active/);
		}
	}

	test('each paladin gets a column, and a picked blessing sticks', async ({ page }) => {
		await seedRaid(page);
		await settingsTab(page);
		await expect(enabled(firstRow(page))).toHaveCount(paladins().length);

		await pickBlessing(firstRow(page).first(), MIGHT);
		await expectBlessing(firstRow(page).first(), MIGHT);

		await reloadRaid(page);
		await settingsTab(page);
		await expectBlessing(firstRow(page).first(), MIGHT);
	});

	test('a paladin leaving the raid takes their column along', async ({ page }) => {
		await seedRaid(page);
		const firstPaladin = fixtureRoster().indexOf(paladins()[0].name);
		await removePlayer(page, firstPaladin);

		await settingsTab(page);
		await expect(enabled(firstRow(page))).toHaveCount(paladins().length - 1);
	});

	test('a paladin joining after a roster import gets a working column', async ({ page }) => {
		const dialogs = collectDialogs(page);
		await openRaid(page);
		await importRoster(page, dialogs, readRoster());
		const before = await enabled(firstRow(page)).count();
		await removePlayer(page, 0);
		await addPreset(page, 'Retribution Paladin', 0);

		await settingsTab(page);
		await expect(enabled(firstRow(page))).toHaveCount(before + 1);
		const newColumn = firstRow(page).nth(before);
		await pickBlessing(newColumn, MIGHT);
		await expectBlessing(newColumn, MIGHT);

		await reloadRaid(page);
		await settingsTab(page);
		await expectBlessing(firstRow(page).nth(before), MIGHT);
	});

	test('a cleared blessing stays cleared after a reload', async ({ page }) => {
		await openRaid(page);
		await addPreset(page, 'Holy Paladin', 0);
		await settingsTab(page);
		await expect(button(firstRow(page).first())).toHaveClass(/active/);

		await pickBlessing(firstRow(page).first(), UNASSIGNED);
		await expectBlessing(firstRow(page).first(), UNASSIGNED);

		await reloadRaid(page);
		await settingsTab(page);
		await expectBlessing(firstRow(page).first(), UNASSIGNED);
	});
});

test('raid consumables and Strength of Wrynn stick and count in the raid stats', async ({ page }) => {
	await seedRaid(page);
	const stamina = await statCount(page, 'Stamina');

	await settingsTab(page);
	const scroll = page.locator('.consumes-settings .icon-picker-button[href*="item=37094"]');
	const wrynn = page.locator('.other-settings .icon-picker-button[href*="spell=73828"]');
	await scroll.click();
	await wrynn.click();
	await expect(scroll).toHaveClass(/active/);
	await expect(wrynn).toHaveClass(/active/);

	await raidTab(page);
	await expect.poll(() => statCount(page, 'Stamina')).toBe(stamina + 1);

	await reloadRaid(page);
	await expect.poll(() => statCount(page, 'Stamina')).toBe(stamina + 1);
	await settingsTab(page);
	await expect(scroll).toHaveClass(/active/);
	await expect(wrynn).toHaveClass(/active/);
});

test('the encounter sticks', async ({ page }) => {
	await seedRaid(page);
	await settingsTab(page);
	const duration = page.locator('.encounter-settings .number-picker-root', { hasText: 'Duration' }).locator('input').first();
	const changed = String(Number(await duration.inputValue()) + 17);

	await duration.fill(changed);
	await duration.press('Enter');
	await reloadRaid(page);
	await settingsTab(page);
	await expect(duration).toHaveValue(changed);
});
