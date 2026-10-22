import { expect, type Page, test } from '@playwright/test';

import { StatCapKind, StatCapsResult } from '../../../../ui/core/proto/api';
import { openSim, openSimTab, watchForErrors } from '../lib/page';
import { reloadSim } from '../settings/helpers';

type Slopes = Record<string, { dps?: number; tps?: number; tmi?: number }>;

function build(key: string, spec: string, tree: number, phase1: Slopes, later: Slopes) {
	const phases = [1, 2, 3, 4, 5].map(phase => ({ phase, slopes: phase == 1 ? phase1 : later }));
	return { key, name: key.replace('_', ' '), class: 'ClassWarrior', spec, tree, two_hand: false, phases };
}

// Fury ranks strength first from P2 on and haste first in P1. Arms shares the spec, so the page's
// talent tree has to pick Fury.
const STAT_VALUES = {
	sim_commit: 'abcdef1234567',
	generated_at: '2026-10-22T11:47:49Z',
	builds: [
		build(
			'warrior_arms',
			'SpecWarrior',
			0,
			{ StatArmorPenetration: { dps: 9 } },
			{ StatArmorPenetration: { dps: 9 } },
		),
		build(
			'warrior_fury',
			'SpecWarrior',
			1,
			{ StatStrength: { dps: 1 }, StatMeleeCrit: { dps: 1.5 }, StatMeleeHaste: { dps: 3 }, StatAttackPower: { dps: 2 } },
			{ StatStrength: { dps: 3 }, StatMeleeCrit: { dps: 2 }, StatMeleeHaste: { dps: 1.5 }, StatAttackPower: { dps: 0.5 }, StatMeleeHit: { dps: 4 } },
		),
		build(
			'protection_warrior',
			'SpecProtectionWarrior',
			2,
			{},
			{
				StatStamina: { tmi: -0.01 },
				StatArmor: { tps: 0.005, tmi: -0.001 },
				StatStrength: { dps: 1, tps: 2, tmi: -0.002 },
				StatDodge: { tps: -0.1, tmi: -0.005 },
			},
		),
	],
};
const P1_ORDER = ['haste', 'attackPower', 'crit', 'strength'];

async function stubStatValues(page: Page) {
	await page.route('**/assets/statweights/stat_values.json', route => route.fulfill({ json: STAT_VALUES }));
}

// Rewrites the server's caps for the page's player, e.g. to put its gear past the hit cap.
async function stubCaps(page: Page, edit: (result: StatCapsResult) => void) {
	await page.route('**/statCaps', async route => {
		const response = await route.fetch();
		const result = StatCapsResult.fromBinary(await response.body());
		edit(result);
		await route.fulfill({ response, body: Buffer.from(StatCapsResult.toBinary(result)) });
	});
}

// a refresh still in flight when the test ends would fail it from inside the route handler
test.afterEach(async ({ page }) => page.unrouteAll({ behavior: 'ignoreErrors' }));

const rows = (page: Page) => page.locator('#caps-tab tr.priority-row');
const row = (page: Page, stat: string) => page.locator(`#caps-tab tr.priority-row[data-stat="${stat}"]`);
const phaseSelect = (page: Page) => page.locator('#caps-tab select.priority-phase');

async function order(page: Page): Promise<string[]> {
	return rows(page).evaluateAll(trs => trs.map(tr => (tr as HTMLElement).dataset.stat ?? ''));
}

async function openCapsTab(page: Page, spec: string) {
	await openSim(page, spec);
	await openSimTab(page, 'caps-tab');
}

test('the priority ranks the shipped values for the sim phase and the build the talents pick', async ({ page }) => {
	const errors = watchForErrors(page);
	await stubStatValues(page);
	await openCapsTab(page, 'warrior');
	await expect(rows(page).first()).toBeVisible();

	const phase = await phaseSelect(page).inputValue();
	expect(phase).not.toBe('1');
	const strength = row(page, 'strength');
	await expect(strength.locator('.priority-value')).toHaveText('3.00');
	// per point of attack power, the warrior's reference stat
	await expect(strength.locator('.priority-ratio')).toHaveText('6.00');
	await expect(page.locator('#caps-tab .priority-source')).toContainText(`warrior fury's P${phase} BiS gear`);
	await expect(page.locator('#caps-tab .priority-source')).toContainText('sim abcdef123');
	await expect(row(page, 'armorPen')).toHaveCount(0);
	expect((await order(page)).filter(stat => stat != 'hit')).toEqual(['strength', 'crit', 'haste', 'attackPower']);
	expect(errors).toEqual([]);
});

test('the phase picker re-orders the priority, and the pick survives a reload', async ({ page }) => {
	await stubStatValues(page);
	await openCapsTab(page, 'warrior');
	await expect(rows(page).first()).toBeVisible();

	await phaseSelect(page).selectOption('1');
	await expect.poll(() => order(page)).toEqual(P1_ORDER);
	await expect(page.locator('#caps-tab .priority-source')).toContainText("warrior fury's P1 BiS gear");

	await reloadSim(page);
	await openSimTab(page, 'caps-tab');
	await expect(phaseSelect(page)).toHaveValue('1');
	await expect.poll(() => order(page)).toEqual(P1_ORDER);
});

test('a tank ranks by survival first and can switch to threat', async ({ page }) => {
	await stubStatValues(page);
	await openCapsTab(page, 'protection_warrior');
	const survival = page.locator('#caps-tab .priority-metric button[data-metric="tmi"]');
	const threat = page.locator('#caps-tab .priority-metric button[data-metric="tps"]');
	await expect(survival).toHaveClass(/active/);
	await expect.poll(() => order(page)).toEqual(['stamina', 'dodge', 'strength', 'armor']);
	// per point of armor, the stat weights tool's survival reference
	await expect(row(page, 'stamina').locator('.priority-ratio')).toHaveText('10.00');

	await threat.click();
	await expect(threat).toHaveClass(/active/);
	await expect(survival).not.toHaveClass(/active/);
	await expect.poll(() => order(page)).toEqual(['strength', 'armor', 'dodge']);
});

test('a healer page says its values come from a live run', async ({ page }) => {
	let fetched = false;
	await page.route('**/assets/statweights/stat_values.json', route => {
		fetched = true;
		return route.fulfill({ json: STAT_VALUES });
	});
	await openCapsTab(page, 'holy_paladin');
	await expect(page.locator('#caps-tab .priority-healer')).toBeVisible();
	await expect(phaseSelect(page)).toHaveCount(0);
	expect(fetched).toBe(false);
});

test('past the hit cap more hit is worth nothing, and the surplus reads as one to reforge away', async ({ page }) => {
	await stubStatValues(page);
	await stubCaps(page, result => {
		for (const cap of result.caps.filter(cap => cap.label == 'Hit' || cap.kind == StatCapKind.StatCapSoft)) {
			cap.rating = Math.round(cap.needed) + 12;
		}
	});
	await openCapsTab(page, 'warrior');
	const hit = row(page, 'hit');
	await expect(hit.locator('.priority-note')).toHaveText('12 over: reforge away');
	await expect(hit.locator('.priority-value')).toHaveText('0.00');
	await expect(hit).toHaveClass(/priority-capped/);
	expect((await order(page)).at(-1)).toBe('hit');
});

test("a dual wielder past the yellow hit cap but short of the white one is pointed at a live run", async ({ page }) => {
	await stubStatValues(page);
	await stubCaps(page, result => {
		const yellow = result.caps.find(cap => cap.label == 'Hit')!;
		yellow.rating = Math.round(yellow.needed) + 5;
		const white = result.caps.find(cap => cap.kind == StatCapKind.StatCapSoft)!;
		white.rating = Math.round(white.needed) - 50;
	});
	await openCapsTab(page, 'warrior');
	await expect(row(page, 'hit').locator('.priority-note')).toHaveText('white swings only: measure at your gear');
	await expect(row(page, 'hit').locator('.priority-value')).toHaveText('0.00');
});

test('below the hit cap, hit keeps its shipped value', async ({ page }) => {
	await stubStatValues(page);
	await stubCaps(page, result => {
		for (const cap of result.caps.filter(cap => cap.label == 'Hit' || cap.kind == StatCapKind.StatCapSoft)) {
			cap.rating = Math.round(cap.needed) - 40;
		}
	});
	await openCapsTab(page, 'warrior');
	await expect(row(page, 'hit').locator('.priority-value')).toHaveText('4.00');
	await expect(row(page, 'hit').locator('.priority-note')).toHaveText('');
	expect((await order(page))[0]).toBe('hit');
});

test('the shipped file the server serves ranks the default warrior', async ({ page }) => {
	const errors = watchForErrors(page);
	await openCapsTab(page, 'warrior');
	await expect(rows(page).first()).toBeVisible();
	await expect(page.locator('#caps-tab .priority-source')).toContainText("Warrior Fury's");
	await expect(page.locator('#caps-tab .priority-status')).toHaveText('');
	expect(errors).toEqual([]);
});
