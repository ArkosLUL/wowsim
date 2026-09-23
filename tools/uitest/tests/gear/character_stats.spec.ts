import { expect, type Page, test } from '@playwright/test';

import { SPECS } from '../lib/page';
import { openGear, readStats, statsAfterChange } from './gear';

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

// presets pairing Blessing of Might or Battle Shout with a 10% attack power buff like Abomination's Might
const MIGHT_TWICE_MULTIPLIED = ['deathknight', 'enhancement_shaman', 'hunter', 'protection_paladin', 'retribution_paladin', 'rogue', 'tank_deathknight'];
// enhancement and retribution turn part of their attack power into spell power
const MIGHT_TWICE_STATS = ['Attack Power', 'Ranged AP', 'Spell Dmg'];

for (const spec of SPECS) {
	const mightTwice = MIGHT_TWICE_MULTIPLIED.includes(spec);
	test(`the ${spec} breakdown of every stat adds up to its total`, async ({ page }) => {
		test.fixme(mightTwice, "sim/core's Buffs snapshot applies the 10% attack power buff to Might twice");
		expect(await mismatchedBreakdowns(page, spec)).toEqual({});
	});
	if (mightTwice) {
		test(`the ${spec} breakdown of every stat but attack and spell power adds up to its total`, async ({ page }) => {
			const off = await mismatchedBreakdowns(page, spec);
			expect(Object.keys(off).filter(label => !MIGHT_TWICE_STATS.includes(label))).toEqual([]);
		});
	}
}
