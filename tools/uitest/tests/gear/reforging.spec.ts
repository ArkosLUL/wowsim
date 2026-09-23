import { expect, type Page, test } from '@playwright/test';

import { openSimTab } from '../lib/page';
import { closePicker, type DbItem, equippedId, loadDb, openGear, openPicker, pane, picker, readStats, showTab, statsAfterChange } from './gear';

// mod-reforging's names for the item_template stat types, and the stats panel rows each lands on.
const STAT_TYPES: Record<number, { name: string; label?: string }> = {
	6: { name: 'Spirit' },
	13: { name: 'Dodge' },
	14: { name: 'Parry' },
	31: { name: 'Hit', label: 'Melee Hit' },
	32: { name: 'Crit', label: 'Melee Crit' },
	36: { name: 'Haste', label: 'Melee Haste' },
	37: { name: 'Expertise', label: 'Expertise' },
};

interface Config {
	enabled: boolean;
	percentage: number;
	statTypes: number[];
}

const serverInput = (page: Page, label: string) =>
	page
		.locator('#settings-tab .server-settings-picker-root .input-root', { has: page.locator('label', { hasText: new RegExp(`^${label}$`) }) })
		.locator('input');

// The reforge config as the settings tab shows it, which is what the sim runs under.
async function readConfig(page: Page): Promise<Config> {
	await openSimTab(page, 'settings-tab');
	const config = {
		enabled: await serverInput(page, 'Reforging').isChecked(),
		percentage: Number(await serverInput(page, 'Reforge %').inputValue()),
		statTypes: (await serverInput(page, 'Reforgeable stats').inputValue()).split(',').map(s => Number(s.trim())),
	};
	await openSimTab(page, 'gear-tab');
	return config;
}

const serverValue = (item: DbItem, statType: number) => item.serverStats?.find(s => s.statType == statType && s.value > 0)?.value ?? 0;
const amount = (item: DbItem, statType: number, config: Config) => Math.floor((serverValue(item, statType) * config.percentage) / 100);

// What mod-reforging lets this item take: a stat it has, moved to a listed stat it doesn't.
function allowedReforges(item: DbItem, config: Config) {
	const out: { from: number; to: number; amount: number }[] = [];
	const count = item.serverStats?.length ?? 0;
	if (!config.enabled || count == 0 || count >= 10) return out;
	for (const from of config.statTypes) {
		for (const to of config.statTypes) {
			if (from == to || serverValue(item, to) > 0 || amount(item, from, config) < 1) continue;
			out.push({ from, to, amount: amount(item, from, config) });
		}
	}
	return out;
}

const label = (r: { from: number; to: number; amount: number }) => `-${r.amount} ${STAT_TYPES[r.from].name} → +${r.amount} ${STAT_TYPES[r.to].name}`;

test('the reforging tab offers exactly the reforges the server allows on the item', async ({ page }) => {
	await openGear(page);
	const db = await loadDb(page);
	const config = await readConfig(page);
	expect(
		config.statTypes.every(t => t in STAT_TYPES),
		`the test knows every configured stat type: ${config.statTypes}`,
	).toBe(true);

	for (const slot of ['head', 'chest', 'legs', 'finger1'] as const) {
		const item = db.items.get(await equippedId(page, slot))!;
		const expected = allowedReforges(item, config).map(label).sort();
		const dialog = await openPicker(page, slot);
		if (expected.length == 0) {
			await expect(dialog.locator('a[data-content-id="Reforging-tab"]'), item.name).toHaveCount(0);
		} else {
			const tab = await showTab(dialog, 'Reforging');
			expect((await tab.locator('li.selector-modal-list-item').allInnerTexts()).map(t => t.trim()).sort(), item.name).toEqual(expected);
		}
		await closePicker(page);
	}
});

test('a reforge moves its two stats by the amount shown, survives a reload, and comes off again', async ({ page }) => {
	await openGear(page);
	const db = await loadDb(page);
	const config = await readConfig(page);
	const item = db.items.get(await equippedId(page, 'head'))!;
	const reforge = allowedReforges(item, config).find(r => STAT_TYPES[r.from].label && STAT_TYPES[r.to].label)!;
	expect(reforge, 'a reforge between two stats the warrior panel shows').toBeTruthy();
	const from = STAT_TYPES[reforge.from].label!;
	const to = STAT_TYPES[reforge.to].label!;

	const before = await readStats(page);
	const tab = await showTab(await openPicker(page, 'head'), 'Reforging');
	const row = tab.locator('li.selector-modal-list-item', { hasText: label(reforge) });
	await row.locator('a').click();
	await expect(row).toHaveClass(/active/);
	await closePicker(page);

	const shown = `Reforged: ${reforge.amount} ${STAT_TYPES[reforge.from].name} → ${STAT_TYPES[reforge.to].name}`;
	await expect(picker(page, 'head').locator('.item-picker-reforge')).toHaveText(shown);
	const after = await statsAfterChange(page, before);
	expect(after[from] - before[from], from).toBe(-reforge.amount);
	expect(after[to] - before[to], to).toBe(reforge.amount);

	await page.reload();
	await page.waitForLoadState('networkidle');
	await openSimTab(page, 'gear-tab');
	await expect(picker(page, 'head').locator('.item-picker-reforge')).toHaveText(shown);
	await expect.poll(() => readStats(page)).toEqual(after);

	const again = await showTab(await openPicker(page, 'head', 'reforge'), 'Reforging');
	await expect(again.locator('li.selector-modal-list-item.active')).toHaveText(label(reforge));
	await again.locator('.selector-modal-remove-button').click();
	await expect(picker(page, 'head').locator('.item-picker-reforge')).toHaveText('');
	await expect.poll(() => readStats(page)).toEqual(before);
});

test('with reforging off the server settings, a kept reforge is flagged and stops counting', async ({ page }) => {
	await openGear(page);
	const db = await loadDb(page);
	const config = await readConfig(page);
	const item = db.items.get(await equippedId(page, 'head'))!;
	const reforge = allowedReforges(item, config).find(r => STAT_TYPES[r.from].label && STAT_TYPES[r.to].label)!;

	const before = await readStats(page);
	const tab = await showTab(await openPicker(page, 'head'), 'Reforging');
	await tab
		.locator('li.selector-modal-list-item', { hasText: label(reforge) })
		.locator('a')
		.click();
	await closePicker(page);
	await statsAfterChange(page, before);

	await openSimTab(page, 'settings-tab');
	await serverInput(page, 'Reforging').uncheck();
	await openSimTab(page, 'gear-tab');
	await expect(picker(page, 'head').locator('.item-picker-reforge')).toContainText("your server settings don't allow it");
	await expect.poll(() => readStats(page)).toEqual(before);

	// the tab stays so the reforge can come off, but offers nothing new
	const off = await showTab(await openPicker(page, 'head'), 'Reforging');
	await expect(off.locator('li.selector-modal-list-item')).toHaveCount(0);
	await off.locator('.selector-modal-remove-button').click();
	await expect(picker(page, 'head').locator('.item-picker-reforge')).toHaveText('');
	await closePicker(page);
	await expect(pane(await openPicker(page, 'head'), 'Reforging')).toHaveCount(0);
});

test('a new reforge percentage in the server settings moves every amount shown and simmed', async ({ page }) => {
	await openGear(page);
	const db = await loadDb(page);
	const config = await readConfig(page);
	const item = db.items.get(await equippedId(page, 'head'))!;
	const reforge = allowedReforges(item, config).find(r => STAT_TYPES[r.from].label && STAT_TYPES[r.to].label)!;
	const from = STAT_TYPES[reforge.from].label!;

	const before = await readStats(page);
	const tab = await showTab(await openPicker(page, 'head'), 'Reforging');
	await tab
		.locator('li.selector-modal-list-item', { hasText: label(reforge) })
		.locator('a')
		.click();
	await closePicker(page);
	await statsAfterChange(page, before);

	const lower = { ...config, percentage: config.percentage - 10 };
	await openSimTab(page, 'settings-tab');
	const percent = serverInput(page, 'Reforge %');
	await percent.fill(String(lower.percentage));
	await percent.dispatchEvent('change');
	await openSimTab(page, 'gear-tab');

	const moved = { ...reforge, amount: amount(item, reforge.from, lower) };
	await expect(picker(page, 'head').locator('.item-picker-reforge')).toHaveText(
		`Reforged: ${moved.amount} ${STAT_TYPES[reforge.from].name} → ${STAT_TYPES[reforge.to].name}`,
	);
	await expect.poll(async () => (await readStats(page))[from]).toBe(before[from] - moved.amount);
	const again = await showTab(await openPicker(page, 'head', 'reforge'), 'Reforging');
	await expect(again.locator('li.selector-modal-list-item.active')).toHaveText(label(moved));
});
