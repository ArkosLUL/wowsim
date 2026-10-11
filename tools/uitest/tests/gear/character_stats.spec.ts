import { expect, type Page, test } from '@playwright/test';

import { ComputeStatsResult } from '../../../../ui/core/proto/api';
import { PseudoStat } from '../../../../ui/core/proto/common';
import { SPECS } from '../lib/page';
import { closePicker, openGear, openPicker, picker, readStats, rows, search, showTab, statsAfterChange } from './gear';

const statRow = (page: Page, label: string) =>
	page.locator('.character-stats-table-row', { has: page.locator('.character-stats-table-label', { hasText: new RegExp(`^${label}$`) }) });

async function setBonus(page: Page, label: string, value: number) {
	await statRow(page, label).locator('.add-bonus-stats').click();
	const input = page.locator('.bonus-stats-popover input');
	await input.fill(String(value));
	await input.dispatchEvent('change');
	await expect(page.locator('.bonus-stats-popover')).toBeHidden();
}

// The value's tooltip as label -> number.
async function breakdown(page: Page, label: string): Promise<Record<string, number>> {
	await statRow(page, label).locator('.stat-value-link').hover();
	const tip = page.locator('.tooltip.show');
	await expect(tip).toBeVisible();
	const rows = await tip
		.locator('.character-stats-tooltip-row')
		.evaluateAll(divs => divs.map(d => [d.children[0].textContent ?? '', d.children[1].textContent ?? '']));
	await page.mouse.move(0, 0);
	await expect(tip).toBeHidden();
	return Object.fromEntries(rows.map(([k, v]) => [k.replace(':', ''), Number(/-?\d+/.exec(v)![0])]));
}

// The value's tooltip as label -> the number in brackets: a skill or a percent.
async function breakdownDerived(page: Page, label: string): Promise<Record<string, number>> {
	await statRow(page, label).locator('.stat-value-link').hover();
	const tip = page.locator('.tooltip.show');
	await expect(tip).toBeVisible();
	const parts = await tip
		.locator('.character-stats-tooltip-row')
		.evaluateAll(divs => divs.map(d => [d.children[0].textContent ?? '', d.children[1].textContent ?? '']));
	await page.mouse.move(0, 0);
	await expect(tip).toBeHidden();
	return Object.fromEntries(parts.map(([k, v]) => [k.replace(':', ''), Number(/\((-?[\d.]+)/.exec(v)?.[1])]));
}

// A row's rating and the number in brackets after it.
async function ratingAndDerived(page: Page, label: string): Promise<{ rating: number; derived: number }> {
	const text = (await statRow(page, label).locator('.stat-value-link').textContent()) ?? '';
	const match = /(-?\d+) \((-?[\d.]+)/.exec(text);
	return { rating: Number(match?.[1]), derived: Number(match?.[2]) };
}

// Puts the named enchant on the main hand, or takes its enchant off.
async function setMainHandEnchant(page: Page, name?: string) {
	const worn = picker(page, 'mainHand').locator('.item-picker-enchant');
	// through the icon: an empty enchant link can't be clicked
	const tab = await showTab(await openPicker(page, 'mainHand'), 'Enchants');
	if (name) {
		await search(tab, name);
		await rows(tab).filter({ hasText: name }).first().locator('a').click();
		await expect(worn).not.toHaveText('');
	} else {
		await tab.locator('.selector-modal-remove-button').click();
		await expect(worn).toHaveText('');
	}
	await closePicker(page);
	await page.waitForLoadState('networkidle');
}

test("a death knight's defense skill counts the Stoneskin Gargoyle rune, as part of the gear", async ({ page }) => {
	await openGear(page, 'tank_deathknight');
	await setMainHandEnchant(page);
	const plain = await ratingAndDerived(page, 'Defense');
	const plainParts = await breakdownDerived(page, 'Defense');

	await setMainHandEnchant(page, 'Rune of the Stoneskin Gargoyle');
	// 25 skill and no rating: the sim sends it as a pseudo stat, and nothing on the page knows the rune
	await expect.poll(() => ratingAndDerived(page, 'Defense')).toEqual({ rating: plain.rating, derived: plain.derived + 25 });
	const parts = await breakdownDerived(page, 'Defense');
	expect(parts['Gear'] - plainParts['Gear']).toBe(25);
	expect(parts['Total'] - plainParts['Total']).toBe(25);
	for (const part of ['Base', 'Talents', 'Buffs', 'Consumes']) expect(parts[part], part).toBe(plainParts[part]);
});

test('block chance and defense skill add the defense skill bonus the sim reports', async ({ page }) => {
	await openGear(page, 'protection_warrior');
	const defense = await ratingAndDerived(page, 'Defense');
	const block = await ratingAndDerived(page, 'Block');
	const plainParts = await breakdownDerived(page, 'Defense');

	// no warrior has such a bonus, so it's written into the sim's answer: 25 skill from the gear on
	await page.route('**/computeStats', async route => {
		const response = await route.fetch();
		const result = ComputeStatsResult.fromBinary(await response.body());
		for (const party of result.raidStats?.parties ?? []) {
			for (const player of party.players) {
				for (const stats of [player.gearStats, player.talentsStats, player.buffsStats, player.consumesStats, player.finalStats]) {
					if (stats) stats.pseudoStats[PseudoStat.PseudoStatBonusDefenseSkill] = 25;
				}
			}
		}
		await route.fulfill({ response, body: Buffer.from(ComputeStatsResult.toBinary(result)) });
	});
	await page.reload();
	await page.waitForLoadState('networkidle');

	await expect.poll(() => ratingAndDerived(page, 'Defense')).toEqual({ rating: defense.rating, derived: defense.derived + 25 });
	// 0.04% a skill point
	const withBonus = await ratingAndDerived(page, 'Block');
	expect(withBonus.rating).toBe(block.rating);
	expect(withBonus.derived - block.derived).toBeCloseTo(1, 2);
	const parts = await breakdownDerived(page, 'Defense');
	expect(parts['Gear'] - plainParts['Gear']).toBe(25);
	expect(parts['Talents']).toBe(plainParts['Talents']);
});

test('a bonus stat adds to its row and its breakdown, and stays after a reload', async ({ page }) => {
	await openGear(page);
	const before = await readStats(page);
	await setBonus(page, 'Melee Hit', 50);
	const after = await statsAfterChange(page, before);
	expect(after['Melee Hit'] - before['Melee Hit']).toBe(50);
	await expect(statRow(page, 'Melee Hit').locator('.stat-value-link')).toHaveClass(/text-success/);
	expect((await breakdown(page, 'Melee Hit'))['Bonus']).toBe(50);

	await page.reload();
	await page.waitForLoadState('networkidle');
	await expect.poll(() => readStats(page)).toEqual(after);

	await setBonus(page, 'Melee Hit', -20);
	await expect.poll(async () => (await readStats(page))['Melee Hit']).toBe(before['Melee Hit'] - 20);
	await expect(statRow(page, 'Melee Hit').locator('.stat-value-link')).toHaveClass(/text-danger/);
});

test('a saved gear set brings its bonus stats back', async ({ page }) => {
	await openGear(page);
	const sets = page.locator('#gear-tab .saved-data-manager-root');
	const plain = await readStats(page);
	await setBonus(page, 'Expertise', 30);
	const bonus = await statsAfterChange(page, plain);
	await sets.locator('.saved-data-save-input').fill('With Bonus');
	await sets.locator('.saved-data-save-button').click();

	await setBonus(page, 'Expertise', 0);
	await expect.poll(() => readStats(page)).toEqual(plain);
	await sets.locator('.saved-data-set-chip', { hasText: 'With Bonus' }).locator('.saved-data-set-name').click();
	await expect.poll(() => readStats(page)).toEqual(bonus);
});

test('what a stance, form or own shout adds is its own part of the breakdown', async ({ page }) => {
	await openGear(page, 'warrior');
	expect((await breakdown(page, 'Strength'))['Stance & Shout']).toBeGreaterThan(0);
	// the preset's own Commanding Shout, which no stance gives
	expect((await breakdown(page, 'Health'))['Stance & Shout']).toBeGreaterThan(0);
	await openGear(page, 'feral_tank_druid');
	expect((await breakdown(page, 'Armor'))['Form']).toBeGreaterThan(0);
});

// Each stat whose breakdown doesn't add up, as label -> parts minus total.
async function mismatchedBreakdowns(page: Page, spec: string): Promise<Record<string, number>> {
	await openGear(page, spec);
	const off: Record<string, number> = {};
	for (const label of Object.keys(await readStats(page))) {
		if (label == 'Melee Crit Cap') continue;
		const parts = await breakdown(page, label);
		const sum = Object.entries(parts)
			.filter(([k]) => k != 'Total')
			.reduce((a, [, v]) => a + v, 0);
		// each part is rounded on its own
		if (Math.abs(sum - parts['Total']) > 3) off[label] = sum - parts['Total'];
	}
	return off;
}

for (const spec of SPECS) {
	test(`the ${spec} breakdown of every stat adds up to its total`, async ({ page }) => {
		expect(await mismatchedBreakdowns(page, spec)).toEqual({});
	});
}
