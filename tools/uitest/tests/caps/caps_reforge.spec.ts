import { expect, type Page, test } from '@playwright/test';

import { StatCap, StatCapKind, StatCapsResult } from '../../../../ui/core/proto/api';
import { ItemReforge, ItemSlot, Stat } from '../../../../ui/core/proto/common';
import { UIItem } from '../../../../ui/core/proto/ui';
import { type Reforging } from '../../../../ui/core/proto_utils/reforging';
import { suggestReforges } from '../../../../ui/core/stat_priority';
import { type LiveValues, type ShippedPhase } from '../../../../ui/core/stat_values';
import { closePicker, openPicker, picker, showTab, type Slot } from '../gear/gear';
import { openSim, openSimTab } from '../lib/page';
import { acceptDialogs, openSpec, savedData, select } from '../settings/helpers';

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

// only haste is worth anything
const HASTE_PHASE: ShippedPhase = { ...PHASE, stats: new Map([[Stat.StatMeleeHaste, { dps: 2, tps: 0, tmi: 0, hps: 0 }]]) };

function suggest(items: Array<{ item: UIItem; reforge?: ItemReforge }>, caps: Array<StatCap>, reforging = REFORGING, phase = PHASE) {
	const slots = items.map(({ item, reforge }, i) => ({ slot: ItemSlot.ItemSlotHead + i, item, reforge: reforge ?? null }));
	return suggestReforges(slots, phase, 'dps', caps, reforging);
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

	test('a hit or expertise cap you meet stays met, even when the stat is worth nothing', () => {
		// hit and expertise are worth nothing, so only the floor holds the item back
		const expertise = (rating: number, needed = 212) => StatCap.create({ label: 'Expertise', stat: Stat.StatExpertise, rating, needed });
		const items = [{ item: item(1, { [EXPERTISE]: 25 }) }];
		expect(suggest(items, [expertise(215)], REFORGING, HASTE_PHASE)[0].suggested).toBeUndefined();
		expect(suggest(items, [expertise(222)], REFORGING, HASTE_PHASE)[0].suggested).toEqual(reforge(EXPERTISE, HASTE));
		const [first, second] = suggest([...items, { item: item(2, { [EXPERTISE]: 25 }) }], [expertise(222)], REFORGING, HASTE_PHASE);
		expect(first.suggested).toEqual(reforge(EXPERTISE, HASTE));
		expect(second.suggested).toBeUndefined();

		const hit = [{ item: item(1, { [HIT]: 25 }) }];
		expect(suggest(hit, [hitCap(305, 300)], REFORGING, HASTE_PHASE)[0].suggested).toBeUndefined();
		expect(suggest(hit, [hitCap(310, 300)], REFORGING, HASTE_PHASE)[0].suggested).toEqual(reforge(HIT, HASTE));
	});

	test('a floor cap you are short of never gets shorter', () => {
		const caps = [hitCap(280, 300)];
		const [only] = suggest([{ item: item(1, { [HIT]: 25 }) }], caps, REFORGING, HASTE_PHASE);
		expect(only.suggested).toBeUndefined();
		// reforging crit into hit moves it closer
		expect(suggest([{ item: item(1, { [CRIT]: 25 }) }], caps)[0].suggested).toEqual(reforge(CRIT, HIT));
	});

	test('a pick that keeps a floor only after an earlier one is flagged, so Apply can refuse it alone', () => {
		// crit into hit overshoots the cap by 6 and item 2's hit into haste spends that, but alone it leaves hit 14 short
		const [first, second] = suggest([{ item: item(1, { [CRIT]: 25, [HASTE]: 25 }) }, { item: item(2, { [HIT]: 25 }) }], [hitCap(296, 300)]);
		expect(first.suggested).toEqual(reforge(CRIT, HIT));
		expect(first.alone).toBe(true);
		expect(second.suggested).toEqual(reforge(HIT, HASTE));
		expect(second.alone).toBe(false);
	});

	test('a generic hit reforge moves spell hit too, and the spell hit floor holds it', () => {
		const spellHit = StatCap.create({ label: 'Spell hit', stat: Stat.StatSpellHit, rating: 300, needed: 300 });
		const slots = [{ slot: ItemSlot.ItemSlotHead, item: item(1, { [HIT]: 25 }), reforge: null }];
		expect(suggestReforges(slots, HASTE_PHASE, 'dps', [], REFORGING)[0].suggested).toEqual(reforge(HIT, HASTE));
		expect(suggestReforges(slots, HASTE_PHASE, 'dps', [spellHit], REFORGING)[0].suggested).toBeUndefined();
	});

	test('live values keep the floors too', () => {
		const dps = (value: number) => ({ dps: value, tps: 0, tmi: 0, hps: 0 });
		const live: LiveValues = {
			gain: { stats: new Map([[Stat.StatMeleeHaste, dps(2)]]), pseudoStats: new Map() },
			loss: { stats: new Map([[Stat.StatMeleeHaste, dps(2)]]), pseudoStats: new Map() },
		};
		const slots = [{ slot: ItemSlot.ItemSlotHead, item: item(1, { [EXPERTISE]: 25 }), reforge: null }];
		const expertise = (rating: number) => StatCap.create({ label: 'Expertise', stat: Stat.StatExpertise, rating, needed: 212 });
		expect(suggestReforges(slots, live, 'dps', [expertise(212)], REFORGING)[0].suggested).toBeUndefined();
		expect(suggestReforges(slots, live, 'dps', [expertise(230)], REFORGING)[0].suggested).toEqual(reforge(EXPERTISE, HASTE));

		const hit = [{ slot: ItemSlot.ItemSlotHead, item: item(1, { [HIT]: 25 }), reforge: null }];
		expect(suggestReforges(hit, live, 'dps', [hitCap(300, 300)], REFORGING)[0].suggested).toBeUndefined();
	});

	test('a gain under 5% of the value given up is skipped', () => {
		const dps = (value: number) => ({ dps: value, tps: 0, tmi: 0, hps: 0 });
		const near = (haste: number): ShippedPhase => ({
			...PHASE,
			stats: new Map([
				[Stat.StatMeleeCrit, dps(1)],
				[Stat.StatMeleeHaste, dps(haste)],
			]),
		});
		const slots = [{ slot: ItemSlot.ItemSlotHead, item: item(1, { [CRIT]: 25 }), reforge: null }];
		const crit = { ...REFORGING, statTypes: [CRIT, HASTE] };
		expect(suggestReforges(slots, near(1.04), 'dps', [], crit)[0].suggested).toBeUndefined();
		expect(suggestReforges(slots, near(1.1), 'dps', [], crit)[0].suggested).toEqual(reforge(CRIT, HASTE));
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

type Slopes = Record<string, { dps?: number; tmi?: number }>;

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

	test('a suggestion that only keeps the caps after another one has no Apply of its own', async ({ page }) => {
		await openCapsTab(page, HASTE_ONLY);
		const leaning = page.locator('#caps-tab tr.reforge-row:has(.reforge-apply:disabled)');
		await expect(leaning.first()).toBeVisible();
		await expect(leaning.first().locator('.reforge-action')).toHaveAttribute('title', /^Alone this would drop your hit and expertise caps: .*Use Apply all\.$/);
		await expect(page.locator('#caps-tab .reforge-apply-all')).toBeEnabled();
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

interface CapReading {
	label: string;
	hand: string;
	surplus: number;
}

// hit, expertise and Defense: the floors. White hit is soft, armor penetration and haste are ceilings.
const FLOOR_LABEL = /^(?!White hit|Armor penetration|Haste)/;

async function readFloors(page: Page): Promise<CapReading[]> {
	await expect(page.locator('#caps-tab .caps-table')).not.toHaveClass(/caps-loading/);
	await expect(page.locator('#caps-tab .reforge-table')).not.toHaveClass(/caps-loading/);
	const read = await page.locator('#caps-tab tr.caps-row').evaluateAll(trs =>
		trs.map(tr => {
			const data = (tr as HTMLElement).dataset;
			return { label: data.label ?? '', hand: data.hand ?? '', rating: Number(data.rating), needed: Number(data.needed) };
		}),
	);
	return read
		.filter(cap => FLOOR_LABEL.test(cap.label) && Number.isFinite(cap.needed))
		.map(cap => ({ label: cap.label, hand: cap.hand, surplus: Math.round(cap.rating) - Math.round(cap.needed) }));
}

// the rogue page opens on Assassination
async function combatRogue(page: Page) {
	await openSimTab(page, 'talents-tab');
	await savedData(page.locator('#talents-tab'), 'Saved Talents').chip('Combat Fists').locator('.saved-data-set-name').click();
	await openSimTab(page, 'gear-tab');
	await savedData(page.locator('#gear-tab'), 'Gear Sets').chip('P3 Combat').locator('.saved-data-set-name').click();
}

const FLOOR_PAGES: Array<{ name: string; spec: string; protoSpec: string; setup?: (page: Page) => Promise<void> }> = [
	{ name: 'Unholy death knight', spec: 'deathknight', protoSpec: 'SpecDeathknight' },
	{ name: 'Fury warrior', spec: 'warrior', protoSpec: 'SpecWarrior' },
	{ name: 'Combat rogue', spec: 'rogue', protoSpec: 'SpecRogue', setup: combatRogue },
	{ name: 'Protection paladin', spec: 'protection_paladin', protoSpec: 'SpecProtectionPaladin' },
];

// Only haste is worth anything (tmi for the tank), so without the floors Apply all would spend every
// point of hit and expertise.
function hasteOnlyFor(protoSpec: string) {
	const values = statValues(() => ({ StatMeleeHaste: { dps: 10, tmi: -10 } }));
	values.builds.forEach(b => (b.spec = protoSpec));
	return values;
}

test.describe('Apply all keeps the floors', () => {
	for (const { name, spec, protoSpec, setup } of FLOOR_PAGES) {
		test(`${name} at phase 3: a met cap stays met and a short one gets no shorter`, async ({ page }) => {
			await page.route('**/assets/statweights/stat_values.json', route => route.fulfill({ json: hasteOnlyFor(protoSpec) }));
			await openSim(page, spec);
			await setup?.(page);
			await openSimTab(page, 'caps-tab');
			await page.locator('#caps-tab select.priority-phase').selectOption('3');
			await expect(rows(page).first()).toBeVisible();
			const before = await readFloors(page);
			expect(before.length, 'the page has a floor cap').toBeGreaterThan(0);

			// the rows stay greyed out, with the old reforges, until the caps for the new gear arrive
			const reforges = async () => {
				await expect(page.locator('#caps-tab .reforge-table')).not.toHaveClass(/caps-loading/);
				return rows(page).evaluateAll(trs => trs.map(tr => (tr as HTMLElement).dataset.current).join());
			};
			const reforgesBefore = await reforges();
			const applyAll = page.locator('#caps-tab .reforge-apply-all');
			await expect(applyAll, 'the page has something to apply').toBeEnabled();
			await applyAll.click();
			await expect.poll(reforges).not.toBe(reforgesBefore);
			const after = await readFloors(page);
			expect(after.length).toBe(before.length);
			after.forEach((cap, i) => {
				const was = before[i];
				expect(cap.surplus, `${cap.label} ${cap.hand}: was ${was.surplus}`).toBeGreaterThanOrEqual(Math.min(0, was.surplus));
			});
		});
	}
});

test.describe('the floor hint', () => {
	test.afterEach(async ({ page }) => page.unrouteAll({ behavior: 'ignoreErrors' }));

	const hint = (page: Page) => page.locator('#caps-tab .reforge-floor-hint');
	const many = (list: string) => `Suggestions keep your ${list} caps once you meet them, and never take you further from them.`;
	const one = (name: string) => `Suggestions keep your ${name} cap once you meet it, and never take you further from it.`;

	const PAGES = [
		{ spec: 'warrior', text: many('hit and expertise') },
		{ spec: 'tank_deathknight', text: many('hit, expertise and Defense') },
		{ spec: 'mage', text: one('spell hit') },
	];
	for (const { spec, text } of PAGES) {
		test(`the ${spec} page names its floor caps`, async ({ page }) => {
			await openSim(page, spec);
			await openSimTab(page, 'caps-tab');
			await expect(hint(page)).toHaveText(text);
		});
	}

	test('a healer page holds no floor cap, so there is no hint', async ({ page }) => {
		// the page alerts that the healing sim is experimental
		acceptDialogs(page);
		await openSim(page, 'healing_priest');
		await openSimTab(page, 'caps-tab');
		await expect(page.locator('#caps-tab tr.caps-row[data-label="Haste"]')).toBeVisible();
		await expect(page.locator('#caps-tab .caps-table')).not.toHaveClass(/caps-loading/);
		await expect(hint(page)).toBeHidden();
	});

	test('the hint follows the caps when they refresh, and a cap out of reach is no floor', async ({ page }) => {
		let expertiseOutOfReach = false;
		await page.route('**/statCaps', async route => {
			const response = await route.fetch();
			const result = StatCapsResult.fromBinary(await response.body());
			if (expertiseOutOfReach) {
				result.caps.filter(cap => cap.stat == Stat.StatExpertise).forEach(cap => (cap.needed = Infinity));
			}
			await route.fulfill({ response, body: Buffer.from(StatCapsResult.toBinary(result)) });
		});
		await openSpec(page, 'warrior', 'caps-tab');
		await expect(hint(page)).toHaveText(many('hit and expertise'));

		expertiseOutOfReach = true;
		await openSimTab(page, 'settings-tab');
		await select(page.locator('#settings-tab .player-settings'), 'Race').selectOption({ label: 'Dwarf' });
		await openSimTab(page, 'caps-tab');
		await expect(hint(page)).toHaveText(one('hit'));
	});
});
