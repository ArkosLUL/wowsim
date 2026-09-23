import { expect, type Page, test } from '@playwright/test';

import { openSimTab, watchForErrors } from '../lib/page';
import {
	closePicker,
	type Db,
	equippedGems,
	equippedId,
	equipRow,
	expectMovedBy,
	GEM_COLOR,
	gemFits,
	loadDb,
	minus,
	openGear,
	openPicker,
	pane,
	picker,
	plus,
	readStats,
	rowEnchants,
	rowIds,
	rows,
	showTab,
	SLOTS,
	statsAfterChange,
	warnings,
} from './gear';

const summary = (page: Page) => page.locator('#gear-tab .gem-summary-root');

// The gem summary as gem name -> count.
async function summaryCounts(page: Page): Promise<Record<string, number>> {
	const lines = (await summary(page).locator('.content-block-body').innerText()).split('\n').filter(l => l.trim());
	const counts: Record<string, number> = {};
	for (let i = 0; i + 1 < lines.length; i += 2) counts[lines[i].trim()] = Number(lines[i + 1]);
	return counts;
}

async function enchantText(page: Page, effectId: number, fallback: string): Promise<string> {
	const descriptions = await (await page.request.get('/wotlk/assets/enchants/descriptions.json')).json();
	return descriptions[effectId] ?? fallback;
}

// Gems a player could drop in without side effects the test would have to model: no meta, no
// unique or profession gems.
function plainGems(db: Db, ids: number[]) {
	return ids.map(id => db.gems.get(id)!).filter(gem => gem && gem.color != GEM_COLOR.meta && !gem.unique && !gem.requiredProfession);
}

async function allGems(page: Page): Promise<number[]> {
	return (await Promise.all(SLOTS.map(slot => equippedGems(page, slot)))).flat();
}

async function toggleProfession(page: Page, name: string, on: boolean) {
	await openSimTab(page, 'settings-tab');
	const box = page.locator('#settings-tab .professions-picker .boolean-picker-root', { hasText: name }).locator('input');
	await box.setChecked(on);
	await openSimTab(page, 'gear-tab');
}

test('the enchant link opens the enchant tab, and swapping the enchant moves the stats by the difference', async ({ page }) => {
	const errors = watchForErrors(page);
	await openGear(page);
	const db = await loadDb(page);

	const dialog = await openPicker(page, 'head', 'enchant');
	await expect(dialog.locator('a[data-content-id="Enchants-tab"]')).toHaveClass(/active/);
	const enchants = pane(dialog, 'Enchants');

	const listed = await rowEnchants(enchants, db);
	const activeIdx = await rows(enchants).evaluateAll(lis => lis.findIndex(li => li.classList.contains('active')));
	expect(activeIdx, 'the worn enchant is marked').toBeGreaterThanOrEqual(0);
	const worn = listed[activeIdx];
	await expect(picker(page, 'head').locator('.item-picker-enchant')).toHaveText(await enchantText(page, worn.effectId, worn.name));

	const withWorn = await readStats(page);
	await enchants.locator('.selector-modal-remove-button').click();
	await expect(picker(page, 'head').locator('.item-picker-enchant')).toHaveText('');
	const bare = await statsAfterChange(page, withWorn);
	expectMovedBy(bare, withWorn, worn.stats);

	const other = listed.find(e => e.effectId != worn.effectId && e.stats.some(v => v > 0))!;
	await rows(enchants).nth(listed.indexOf(other)).locator('a').click();
	await expect(picker(page, 'head').locator('.item-picker-enchant')).toHaveText(await enchantText(page, other.effectId, other.name));
	const after = await statsAfterChange(page, bare);
	expectMovedBy(bare, after, other.stats);
	expect(errors).toEqual([]);
});

test('each gem tab lists the gems that fit its socket', async ({ page }) => {
	await openGear(page);
	const db = await loadDb(page);
	const item = db.items.get(await equippedId(page, 'head'))!;
	const sockets = item.gemSockets!;
	const dialog = await openPicker(page, 'head');

	await expect(dialog.locator('a.selector-modal-tab-gem')).toHaveCount(sockets.length);
	for (let i = 0; i < sockets.length; i++) {
		const socketName = Object.entries(GEM_COLOR).find(([, c]) => c == sockets[i])![0];
		const tab = await showTab(dialog, `Gem${i + 1}`);
		await expect(dialog.locator(`a[data-content-id="Gem${i + 1}-tab"] .socket-icon`)).toHaveAttribute('src', new RegExp(`socket-${socketName}`));

		const matching = tab.locator('.selector-modal-show-matching-gems input');
		await matching.setChecked(false);
		const gems = (await rowIds(tab)).map(id => db.gems.get(id)!);
		expect(gems.length).toBeGreaterThan(0);
		for (const gem of gems) {
			expect(gem.color == GEM_COLOR.meta, `${gem.name} in a ${socketName} socket`).toBe(sockets[i] == GEM_COLOR.meta);
		}

		await matching.setChecked(true);
		await expect.poll(async () => (await rowIds(tab)).every(id => gemFits(db.gems.get(id)!.color, sockets[i]))).toBe(true);
		await matching.setChecked(false);
	}
});

test('a gem swap shows in the socket and the gem summary, and moves the stats', async ({ page }) => {
	await openGear(page);
	const db = await loadDb(page);
	const item = db.items.get(await equippedId(page, 'neck'))!;
	const [oldId] = await equippedGems(page, 'neck');
	const oldGem = db.gems.get(oldId)!;
	const counts = await summaryCounts(page);

	const tab = await showTab(await openPicker(page, 'neck'), 'Gem1');
	// another gem for the same colour, so the socket bonus stays as it was
	const gem = plainGems(db, await rowIds(tab)).find(g => g.id != oldId && gemFits(g.color, item.gemSockets![0]) && !(g.name in counts))!;
	const before = await readStats(page);
	await equipRow(tab, gem.id);

	await expect.poll(() => equippedGems(page, 'neck')).toEqual([gem.id]);
	await expect(rows(tab).filter({ has: page.locator(`a[href*="item=${gem.id}?"]`) })).toHaveClass(/active/);
	await closePicker(page);
	const after = await statsAfterChange(page, before);
	expectMovedBy(before, after, minus(gem.stats, oldGem.stats));

	const now = await summaryCounts(page);
	expect(now[gem.name]).toBe(1);
	expect(now[oldGem.name] ?? 0).toBe(counts[oldGem.name] - 1);
});

test('a socket bonus counts only while every socket holds a gem of its colour', async ({ page }) => {
	await openGear(page);
	const db = await loadDb(page);
	const item = db.items.get(await equippedId(page, 'neck'))!;
	expect(item.gemSockets?.length, 'the preset neck has a socket').toBe(1);
	const socket = item.gemSockets![0];

	const tab = await showTab(await openPicker(page, 'neck'), 'Gem1');
	await tab.locator('.selector-modal-show-matching-gems input').setChecked(false);
	const worn = await readStats(page);
	await tab.locator('.selector-modal-remove-button').click();
	const bare = await statsAfterChange(page, worn);

	const gems = plainGems(db, await rowIds(tab));
	const off = gems.find(g => !gemFits(g.color, socket))!;
	const on = gems.find(g => gemFits(g.color, socket))!;

	await equipRow(tab, off.id);
	const withOff = await statsAfterChange(page, bare);
	expectMovedBy(bare, withOff, off.stats);

	await equipRow(tab, on.id);
	const withOn = await statsAfterChange(page, withOff);
	expectMovedBy(bare, withOn, plus(on.stats, item.socketBonus));
});

test('the meta gem turns off while its colours are missing, and back on', async ({ page }) => {
	await openGear(page);
	const db = await loadDb(page);
	const headGems = await equippedGems(page, 'head');
	const meta = db.gems.get(headGems.find(id => db.gems.get(id)?.color == GEM_COLOR.meta)!)!;
	expect(await warnings(page)).not.toContain('Meta gem disabled');

	// the preset's only blue-counting gem sits in the helm, so taking it out breaks the meta
	const blueIdx = headGems.findIndex(id => id && gemFits(db.gems.get(id)!.color, GEM_COLOR.blue));
	test.skip(blueIdx < 0, 'no blue-counting gem in the preset helm');
	const blue = db.gems.get(headGems[blueIdx])!;
	const gemIdsWorn = await allGems(page);
	test.skip(gemIdsWorn.filter(id => id && gemFits(db.gems.get(id)!.color, GEM_COLOR.blue)).length != 1, 'more than one blue-counting gem');

	const tab = await showTab(await openPicker(page, 'head'), `Gem${blueIdx + 1}`);
	const active = await readStats(page);
	await tab.locator('.selector-modal-remove-button').click();
	await expect.poll(() => warnings(page)).toContain(`Meta gem disabled (${meta.name})`);
	const inactive = await statsAfterChange(page, active);
	// the meta's own stats go with it, on top of the gem taken out
	expect(active['Agility'] - inactive['Agility']).toBeGreaterThanOrEqual(blue.stats[1] + meta.stats[1]);

	await equipRow(tab, blue.id);
	await expect.poll(() => warnings(page)).not.toContain('Meta gem disabled');
	await expect.poll(readStats.bind(null, page)).toEqual(active);
});

test('blacksmithing opens the extra wrist and hands sockets, and their gems count only while it is known', async ({ page }) => {
	await openGear(page);
	const db = await loadDb(page);
	const extra = (slot: 'wrist' | 'hands') => picker(page, slot).locator('.gem-socket-container').last();
	await expect(extra('hands')).toBeHidden();

	await toggleProfession(page, 'Blacksmithing', true);
	await expect(extra('hands')).toBeVisible();
	await expect(extra('wrist')).toBeVisible();

	const hands = db.items.get(await equippedId(page, 'hands'))!;
	const extraTab = `Gem${hands.gemSockets!.length + 1}`;
	const dialog = await openPicker(page, 'hands');
	const tab = await showTab(dialog, extraTab);
	const counts = await summaryCounts(page);
	const gem = plainGems(db, await rowIds(tab)).find(g => !(g.name in counts))!;
	const before = await readStats(page);
	await equipRow(tab, gem.id);
	await closePicker(page);
	const withGem = await statsAfterChange(page, before);
	expectMovedBy(before, withGem, gem.stats);
	await expect.poll(() => summaryCounts(page)).toMatchObject({ [gem.name]: 1 });

	await toggleProfession(page, 'Blacksmithing', false);
	await expect(extra('hands')).toBeHidden();
	const without = await statsAfterChange(page, withGem);
	expectMovedBy(without, withGem, gem.stats);
	await expect.poll(async () => (await summaryCounts(page))[gem.name]).toBeUndefined();
});

test("a fourth Dragon's Eye brings up the jewelcrafting warning, and leaving jewelcrafting flags them all", async ({ page }) => {
	await openGear(page);
	const db = await loadDb(page);
	const jc = (await allGems(page)).filter(id => db.gems.get(id)?.requiredProfession);
	test.skip(jc.length != 3, 'the preset should wear three jewelcrafter gems');

	// a jewelcrafter-only gem into the neck's socket makes four
	const tab = await showTab(await openPicker(page, 'neck'), 'Gem1');
	const fourth = (await rowIds(tab)).find(id => db.gems.get(id)?.requiredProfession)!;
	await equipRow(tab, fourth);
	await closePicker(page);
	await expect.poll(() => warnings(page)).toContain('Only 3 Jewelcrafting Gems are allowed, but 4 are equipped.');

	await toggleProfession(page, 'Jewelcrafting', false);
	await expect.poll(() => warnings(page)).toContain(`${db.gems.get(fourth)!.name} requires Jewelcrafting, but it is not selected.`);
	// and the gem tabs stop offering them
	const again = await showTab(await openPicker(page, 'neck'), 'Gem1');
	await again.locator('.selector-modal-show-matching-gems input').setChecked(false);
	for (const id of await rowIds(again)) {
		if (id == fourth) continue;
		expect(db.gems.get(id)!.requiredProfession ?? 0, db.gems.get(id)!.name).toBe(0);
	}
});
