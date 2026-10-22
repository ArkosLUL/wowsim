import { expect, type Page, test } from '@playwright/test';

import { StatCap, StatCapKind } from '../../../../ui/core/proto/api';
import { ItemReforge, ItemSlot, Stat } from '../../../../ui/core/proto/common';
import { UIItem } from '../../../../ui/core/proto/ui';
import { type Reforging } from '../../../../ui/core/proto_utils/reforging';
import { suggestReforges } from '../../../../ui/core/stat_priority';
import { type LiveValues, type ShippedPhase } from '../../../../ui/core/stat_values';
import { closePicker, openPicker, picker, showTab, type Slot } from '../gear/gear';
import { openSim, openSimTab } from '../lib/page';

const HIT = 31;
const CRIT = 32;
const HASTE = 36;
const EXPERTISE = 37;
const DODGE = 13;

const REFORGING: Reforging = { enabled: true, percentage: 40, statTypes: [HIT, CRIT, HASTE, EXPERTISE] };

// 40% of 25 is 10, so each of these items moves 10 rating
function item(id: number, stats: Record<number, number>): UIItem {
	return UIItem.create({ id, name: `item ${id}`, serverStats: Object.entries(stats).map(([statType, value]) => ({ statType: Number(statType), value })) });
}

const reforge = (fromStatType: number, toStatType: number) => ItemReforge.create({ fromStatType, toStatType });

// hit 4, haste 2, crit 1 per point; expertise isn't worth anything
const PHASE: ShippedPhase = {
	phase: 1,
	stats: new Map([
		[Stat.StatMeleeHit, { dps: 4, tps: 0, tmi: 0, hps: 0 }],
		[Stat.StatMeleeHaste, { dps: 2, tps: 0, tmi: 0, hps: 0 }],
		[Stat.StatMeleeCrit, { dps: 1, tps: 0, tmi: 0, hps: 0 }],
	]),
	pseudoStats: new Map(),
};

function hitCap(rating: number, needed: number, kind = StatCapKind.StatCapFloor): StatCap {
	return StatCap.create({ label: kind == StatCapKind.StatCapSoft ? 'Dual wield white hit' : 'Hit', stat: Stat.StatMeleeHit, kind, rating, needed });
}

function suggest(items: Array<{ item: UIItem; reforge?: ItemReforge }>, caps: Array<StatCap>, reforging = REFORGING) {
	const slots = items.map(({ item, reforge }, i) => ({ slot: ItemSlot.ItemSlotHead + i, item, reforge: reforge ?? null }));
	return suggestReforges(slots, PHASE, 'dps', caps, reforging);
}

test.describe('reforge scoring', () => {
	test('a surplus over the hit cap is reforged away, but no further than pays', () => {
		const [first, second] = suggest([{ item: item(1, { [HIT]: 25 }) }, { item: item(2, { [HIT]: 25 }) }], [hitCap(300, 288)]);
		expect(first.suggested).toEqual(reforge(HIT, HASTE));
		expect(first.gain).toBe(20);
		// 2 surplus left: 8 of its 10 hit would cost 4 each against 20 from haste
		expect(second.suggested).toBeUndefined();
	});

	test('room under the hit cap fills up to the cap, then the next best stat takes over', () => {
		const [first, second] = suggest([{ item: item(1, { [CRIT]: 25 }) }, { item: item(2, { [CRIT]: 25 }) }], [hitCap(288, 300)]);
		expect(first.suggested).toEqual(reforge(CRIT, HIT));
		expect(first.gain).toBe(30);
		expect(second.suggested).toEqual(reforge(CRIT, HASTE));
		expect(second.gain).toBe(10);
	});

	test('a suggestion can replace the reforge the item has', () => {
		const [only] = suggest([{ item: item(1, { [CRIT]: 25 }), reforge: reforge(CRIT, HIT) }], [hitCap(318, 288)]);
		expect(only.active).toBe(true);
		expect(only.suggested).toEqual(reforge(CRIT, HASTE));
		expect(only.gain).toBe(20);
	});

	test('the reforge comes off when nothing it could become beats the stat it took', () => {
		const reforging = { ...REFORGING, statTypes: [HIT, CRIT] };
		const [only] = suggest([{ item: item(1, { [CRIT]: 25 }), reforge: reforge(CRIT, HIT) }], [hitCap(318, 288)], reforging);
		expect(only.suggested).toBeNull();
		expect(only.gain).toBe(10);
	});

	test("past the yellow hit cap a dual wielder's hit isn't spent while the white cap is short", () => {
		const caps = [hitCap(300, 288), hitCap(300, 400, StatCapKind.StatCapSoft)];
		const [only] = suggest([{ item: item(1, { [HIT]: 25 }) }], caps);
		expect(only.suggested).toBeUndefined();
	});

	test('a reforge the server settings refuse counts for nothing, so any good one replaces it', () => {
		const [only] = suggest([{ item: item(1, { [CRIT]: 25 }), reforge: reforge(CRIT, DODGE) }], [hitCap(300, 300)]);
		expect(only.active).toBe(false);
		expect(only.suggested).toEqual(reforge(CRIT, HASTE));
		expect(only.gain).toBe(10);
	});

	test('items with nothing to reforge are left out', () => {
		expect(suggest([{ item: item(1, { [DODGE]: 25 }) }], [])).toEqual([]);
	});

	test('live values score added rating at its gain and removed rating at its loss, with no cap rule', () => {
		const dps = (value: number) => ({ dps: value, tps: 0, tmi: 0, hps: 0 });
		const live: LiveValues = {
			gain: {
				stats: new Map([
					[Stat.StatMeleeCrit, dps(1)],
					[Stat.StatMeleeHaste, dps(0.8)],
					[Stat.StatMeleeHit, dps(3)],
				]),
				pseudoStats: new Map(),
			},
			loss: {
				stats: new Map([
					[Stat.StatMeleeCrit, dps(0.5)],
					[Stat.StatMeleeHaste, dps(2)],
					[Stat.StatMeleeHit, dps(3)],
				]),
				pseudoStats: new Map(),
			},
		};
		const slots = [item(1, { [CRIT]: 25 }), item(2, { [HASTE]: 25 })].map((it, i) => ({ slot: ItemSlot.ItemSlotHead + i, item: it, reforge: null }));
		const reforging = { ...REFORGING, statTypes: [CRIT, HASTE] };
		const [crit, haste] = suggestReforges(slots, live, 'dps', [], reforging);
		// one value per stat would swap these the other way round
		expect(crit.suggested).toEqual(reforge(CRIT, HASTE));
		expect(crit.gain).toBe(3);
		expect(haste.suggested).toBeUndefined();

		// the run measured hit at the gear, so a shipped value's cap rule doesn't apply
		const [toHit] = suggestReforges(slots.slice(0, 1), live, 'dps', [hitCap(400, 300)], { ...REFORGING, statTypes: [CRIT, HIT] });
		expect(toHit.suggested).toEqual(reforge(CRIT, HIT));
		expect(toHit.gain).toBe(25);
	});
});

type Slopes = Record<string, { dps?: number }>;

function statValues(slopes: (phase: number) => Slopes) {
	const build = (key: string, tree: number) => ({
		key,
		name: key,
		class: 'ClassWarrior',
		spec: 'SpecWarrior',
		tree,
		two_hand: false,
		phases: [1, 2, 3, 4, 5].map(phase => ({ phase, slopes: slopes(phase) })),
	});
	return { sim_commit: 'abcdef1234567', generated_at: '2026-10-22T11:47:49Z', builds: [build('warrior_arms', 0), build('warrior_fury', 1)] };
}

// Only haste is worth anything, so every item that can take haste should, and a reforge that took
// haste away should come off.
const HASTE_ONLY = statValues(() => ({ StatMeleeHaste: { dps: 10 } }));

async function openCapsTab(page: Page, values: object) {
	await page.route('**/assets/statweights/stat_values.json', route => route.fulfill({ json: values }));
	await openSim(page, 'warrior');
	await openSimTab(page, 'caps-tab');
}

const SLOT_NAMES: Record<number, Slot> = {
	[ItemSlot.ItemSlotHead]: 'head',
	[ItemSlot.ItemSlotNeck]: 'neck',
	[ItemSlot.ItemSlotShoulder]: 'shoulder',
	[ItemSlot.ItemSlotBack]: 'back',
	[ItemSlot.ItemSlotChest]: 'chest',
	[ItemSlot.ItemSlotWrist]: 'wrist',
	[ItemSlot.ItemSlotHands]: 'hands',
	[ItemSlot.ItemSlotWaist]: 'waist',
	[ItemSlot.ItemSlotLegs]: 'legs',
	[ItemSlot.ItemSlotFeet]: 'feet',
	[ItemSlot.ItemSlotFinger1]: 'finger1',
	[ItemSlot.ItemSlotFinger2]: 'finger2',
	[ItemSlot.ItemSlotTrinket1]: 'trinket1',
	[ItemSlot.ItemSlotTrinket2]: 'trinket2',
	[ItemSlot.ItemSlotMainHand]: 'mainHand',
	[ItemSlot.ItemSlotOffHand]: 'offHand',
	[ItemSlot.ItemSlotRanged]: 'ranged',
};

const rows = (page: Page) => page.locator('#caps-tab tr.reforge-row');
const suggested = (page: Page) => page.locator('#caps-tab tr.reforge-row:not(.reforge-keep)');

interface Suggestion {
	itemSlot: number;
	slot: Slot;
	current: string;
	// "32 Crit → Haste", or "remove"
	suggestion: string;
}

async function readSuggestions(page: Page): Promise<Suggestion[]> {
	await expect(rows(page).first()).toBeVisible();
	await expect(page.locator('#caps-tab .reforge-table')).not.toHaveClass(/caps-loading/);
	const read = await suggested(page).evaluateAll(trs =>
		trs.map(tr => {
			const data = (tr as HTMLElement).dataset;
			return { itemSlot: Number(data.slot), current: data.current ?? '', suggestion: data.suggestion ?? '' };
		}),
	);
	return read.map(s => ({ ...s, slot: SLOT_NAMES[s.itemSlot] }));
}

// the gear picker's Reforging tab writes "-32 Crit → +32 Haste" for the label "32 Crit → Haste"
const pickerRow = (label: string) => label.replace(/^(\d+) (.+) → (.+)$/, '-$1 $2 → +$1 $3');

async function expectReforgeInGearPicker(page: Page, slot: Slot, suggestion: string) {
	const shown = picker(page, slot).locator('.item-picker-reforge');
	if (suggestion == 'remove') {
		await expect(shown, slot).toHaveText('');
		return;
	}
	await expect(shown, slot).toHaveText(`Reforged: ${suggestion}`);
	const tab = await showTab(await openPicker(page, slot, 'reforge'), 'Reforging');
	await expect(tab.locator('li.selector-modal-list-item.active'), slot).toHaveText(pickerRow(suggestion));
	await closePicker(page);
}

test.describe('the suggestions block', () => {
	// a refresh still in flight when the test ends would fail it from inside the route handler
	test.afterEach(async ({ page }) => page.unrouteAll({ behavior: 'ignoreErrors' }));

	test("Apply sets the item's reforge, which the gear picker then shows", async ({ page }) => {
		await openCapsTab(page, HASTE_ONLY);
		const pick = (await readSuggestions(page)).find(s => s.suggestion.endsWith('→ Haste'));
		expect(pick, 'an item the default Fury gear could reforge into haste').toBeTruthy();
		const row = page.locator(`#caps-tab tr.reforge-row[data-slot="${pick!.itemSlot}"]`);
		await expect(row.locator('.reforge-gain')).toHaveText(/^\+[\d,.]+ DPS$/);

		await row.locator('.reforge-apply').click();
		await expect(row).toHaveAttribute('data-suggestion', 'keep');
		await expect(row.locator('.reforge-now')).toHaveText(pick!.suggestion);

		await openSimTab(page, 'gear-tab');
		await expectReforgeInGearPicker(page, pick!.slot, pick!.suggestion);
	});

	test('Apply all puts every suggestion on, replacing reforges the items had', async ({ page }) => {
		await openCapsTab(page, HASTE_ONLY);
		const picks = await readSuggestions(page);
		expect(picks.length).toBeGreaterThan(1);
		expect(
			picks.some(p => p.current),
			'the default Fury gear has a reforge worth replacing',
		).toBe(true);

		await page.locator('#caps-tab .reforge-apply-all').click();
		await expect(suggested(page)).toHaveCount(0);
		await expect(page.locator('#caps-tab .reforge-apply-all')).toBeDisabled();

		await openSimTab(page, 'gear-tab');
		for (const { slot, suggestion } of picks) {
			await expectReforgeInGearPicker(page, slot, suggestion);
		}
	});

	test('the phase picker re-ranks the suggestions too', async ({ page }) => {
		await openCapsTab(
			page,
			statValues(phase => (phase == 1 ? { StatMeleeHaste: { dps: 10 } } : { StatMeleeCrit: { dps: 10 } })),
		);
		const toStat = async (stat: string) => (await readSuggestions(page)).filter(s => s.suggestion.endsWith(`→ ${stat}`)).length;
		await page.locator('#caps-tab select.priority-phase').selectOption('1');
		await expect.poll(() => toStat('Haste')).toBeGreaterThan(0);

		await page.locator('#caps-tab select.priority-phase').selectOption('2');
		await expect.poll(() => toStat('Haste')).toBe(0);
		expect(await toStat('Crit')).toBeGreaterThan(0);
	});

	test('with reforging off the server settings there is nothing to suggest', async ({ page }) => {
		await openCapsTab(page, HASTE_ONLY);
		await expect(suggested(page).first()).toBeVisible();

		await openSimTab(page, 'settings-tab');
		await page
			.locator('#settings-tab .server-settings-picker-root .input-root', { has: page.locator('label', { hasText: /^Reforging$/ }) })
			.locator('input')
			.uncheck();
		await openSimTab(page, 'caps-tab');
		await expect(page.locator('#caps-tab .reforge-status')).toHaveText('Your server settings have reforging off.');
		await expect(page.locator('#caps-tab .reforge-table')).toBeHidden();
	});

	test('a healer page has no shipped values to score reforges with', async ({ page }) => {
		await openSim(page, 'holy_paladin');
		await openSimTab(page, 'caps-tab');
		await expect(page.locator('#caps-tab .reforge-status')).toContainText('healers get theirs from a live run');
		await expect(page.locator('#caps-tab .reforge-apply-all')).toBeHidden();
	});
});
