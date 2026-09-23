import { expect, type Page, test } from '@playwright/test';

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

test.fixme('the breakdown of every stat adds up to its total', async ({ page }) => {
	// sim/core measures the phases before stance and form multipliers, which only the total has
	await openGear(page);
	for (const label of Object.keys(await readStats(page))) {
		if (label == 'Melee Crit Cap') continue;
		const parts = await breakdown(page, label);
		const sum = Object.entries(parts)
			.filter(([k]) => k != 'Total')
			.reduce((a, [, v]) => a + v, 0);
		// each part is rounded on its own
		expect(Math.abs(sum - parts['Total']), label).toBeLessThanOrEqual(3);
	}
});
