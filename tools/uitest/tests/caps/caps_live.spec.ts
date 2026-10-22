import { expect, type Page, test } from '@playwright/test';

import { AsyncAPIResult, ProgressMetrics, StatSlope, StatSlopeMetrics, StatSlopesRequest, StatSlopesResult } from '../../../../ui/core/proto/api';
import { Class, Stat } from '../../../../ui/core/proto/common';
import { openSim, openSimTab, watchForErrors } from '../lib/page';

const STAT_VALUES = {
	sim_commit: 'abcdef1234567',
	generated_at: '2026-10-22T11:47:49Z',
	builds: [
		{
			key: 'warrior_fury',
			name: 'warrior fury',
			class: 'ClassWarrior',
			spec: 'SpecWarrior',
			tree: 1,
			two_hand: false,
			phases: [1, 2, 3, 4, 5].map(phase => ({ phase, slopes: { StatAttackPower: { dps: 2 }, StatMeleeHaste: { dps: 1 } } })),
		},
	],
};

function slope(stat: Stat, gain: Partial<StatSlopeMetrics>, loss: Partial<StatSlopeMetrics> | null = gain): StatSlope {
	return StatSlope.create({
		stat,
		delta: 100,
		lowDelta: loss ? 100 : 0,
		gain: StatSlopeMetrics.create(gain),
		loss: loss ? StatSlopeMetrics.create(loss) : undefined,
	});
}

// Past the hit cap: more hit does nothing, less costs. Expertise does nothing either way.
const FURY_RESULT = StatSlopesResult.create({
	base: StatSlopeMetrics.create({ dps: 10000 }),
	slopes: [
		slope(Stat.StatStrength, { dps: 3 }, { dps: 2.5 }),
		slope(Stat.StatAttackPower, { dps: 1 }),
		slope(Stat.StatMeleeHit, { dps: 0 }, { dps: 4 }),
		slope(Stat.StatMeleeCrit, { dps: 2 }),
		slope(Stat.StatMeleeHaste, { dps: 1.5 }),
		slope(Stat.StatArmorPenetration, { dps: 2.2 }),
		slope(Stat.StatExpertise, { dps: 0 }),
		slope(Stat.StatIntellect, { dps: 0 }, null),
	],
});
const FURY_ORDER = ['strength', 'armorPen', 'crit', 'haste', 'attackPower', 'hit'];

const HEALER_RESULT = StatSlopesResult.create({
	base: StatSlopeMetrics.create({ hps: 5000 }),
	slopes: [
		slope(Stat.StatSpellPower, { hps: 2 }),
		slope(Stat.StatIntellect, { hps: 1.2 }),
		slope(Stat.StatSpellHaste, { hps: 1.5 }),
		slope(Stat.StatSpellCrit, { hps: 0.8 }),
		slope(Stat.StatMP5, { hps: 0.5 }),
		slope(Stat.StatSpirit, { hps: 0.3 }),
	],
});

const PROGRESS_ID = 'caps-live-stub';

// Answers /statSlopesAsync with a run of its own: progress until release(), then the result. Every
// other async run still reaches the server.
async function stubSlopes(page: Page, result: StatSlopesResult, { held = false } = {}) {
	const requests: Array<StatSlopesRequest> = [];
	let released = !held;
	let served = false;
	await page.route('**/statSlopesAsync', async route => {
		requests.push(StatSlopesRequest.fromBinary(route.request().postDataBuffer()!));
		await route.fulfill({ contentType: 'application/x-protobuf', body: Buffer.from(AsyncAPIResult.toBinary(AsyncAPIResult.create({ progressId: PROGRESS_ID }))) });
	});
	await page.route('**/asyncProgress', async route => {
		const body = route.request().postDataBuffer();
		if (!body || AsyncAPIResult.fromBinary(body).progressId != PROGRESS_ID) {
			return route.fallback();
		}
		if (served) {
			return route.fulfill({ status: 204 });
		}
		let progress = ProgressMetrics.create({ completedSims: 3, totalSims: 7 });
		if (released) {
			progress = ProgressMetrics.create({ finalSlopesResult: result });
			served = true;
		}
		await route.fulfill({ contentType: 'application/x-protobuf', body: Buffer.from(ProgressMetrics.toBinary(progress)) });
	});
	return { requests, release: () => (released = true) };
}

// a run still polling when the test ends would fail it from inside the route handler
test.afterEach(async ({ page }) => page.unrouteAll({ behavior: 'ignoreErrors' }));

const rows = (page: Page) => page.locator('#caps-tab tr.priority-row');
const row = (page: Page, stat: string) => page.locator(`#caps-tab tr.priority-row[data-stat="${stat}"]`);
const measureButton = (page: Page) => page.locator('#caps-tab .priority-measure');
const measureStatus = (page: Page) => page.locator('#caps-tab .priority-measure-status');
const sourceSwitch = (page: Page) => page.locator('#caps-tab .priority-source-switch');
const sourceButton = (page: Page, source: 'shipped' | 'live') => page.locator(`#caps-tab .priority-source-switch button[data-source="${source}"]`);
const priorityTable = (page: Page) => page.locator('#caps-tab .priority-table');
const prioritySource = (page: Page) => page.locator('#caps-tab .priority-source');
const reforgeStatus = (page: Page) => page.locator('#caps-tab .reforge-status');

async function order(page: Page): Promise<string[]> {
	return rows(page).evaluateAll(trs => trs.map(tr => (tr as HTMLElement).dataset.stat ?? ''));
}

async function openCapsTab(page: Page, spec: string) {
	await page.route('**/assets/statweights/stat_values.json', route => route.fulfill({ json: STAT_VALUES }));
	await openSim(page, spec);
	await openSimTab(page, 'caps-tab');
}

test('Measure at my gear shows progress, then ranks by what each stat is worth at your gear', async ({ page }) => {
	const errors = watchForErrors(page);
	const { requests, release } = await stubSlopes(page, FURY_RESULT, { held: true });
	await openCapsTab(page, 'warrior');
	await expect.poll(() => order(page)).toEqual(['attackPower', 'haste']);
	await expect(sourceSwitch(page)).toBeHidden();
	const phase = await page.locator('#caps-tab select.priority-phase').inputValue();

	await measureButton(page).click();
	await expect(measureStatus(page)).toHaveText('Measuring… 3 of 7 sims');
	await expect(measureButton(page)).toBeDisabled();
	expect(requests).toHaveLength(1);
	expect(requests[0].raid?.parties[0].players[0].class).toBe(Class.ClassWarrior);
	expect(requests[0].simOptions?.iterations).toBeGreaterThan(0);
	expect(requests[0].simOptions?.randomSeed).toBeGreaterThan(0n);

	release();
	await expect(sourceButton(page, 'live')).toHaveClass(/active/);
	await expect(measureButton(page)).toBeEnabled();
	await expect(measureStatus(page)).toHaveText('');
	await expect.poll(() => order(page)).toEqual(FURY_ORDER);
	await expect(row(page, 'strength').locator('.priority-value')).toHaveText('3.00');
	await expect(row(page, 'strength').locator('.priority-loss')).toHaveText('2.50');
	// per point of attack power, the warrior's reference stat
	await expect(row(page, 'strength').locator('.priority-ratio')).toHaveText('3.00');
	await expect(row(page, 'hit').locator('.priority-value')).toHaveText('0.00');
	await expect(row(page, 'hit').locator('.priority-loss')).toHaveText('4.00');
	await expect(row(page, 'expertise')).toHaveCount(0);
	await expect(page.locator('#caps-tab select.priority-phase')).toBeDisabled();
	await expect(prioritySource(page)).toContainText('Measured at your gear');
	await expect(priorityTable(page)).not.toHaveClass(/priority-stale/);

	await sourceButton(page, 'shipped').click();
	await expect(sourceButton(page, 'shipped')).toHaveText(`Phase ${phase} BiS`);
	await expect(sourceButton(page, 'shipped')).toHaveClass(/active/);
	await expect(page.locator('#caps-tab select.priority-phase')).toBeEnabled();
	await expect.poll(() => order(page)).toEqual(['attackPower', 'haste']);
	await expect(row(page, 'attackPower').locator('.priority-loss')).toHaveCount(0);
	expect(errors).toEqual([]);
});

test('live values score the reforge suggestions, and a change to the character makes them stale', async ({ page }) => {
	await stubSlopes(page, FURY_RESULT);
	await openCapsTab(page, 'warrior');
	await measureButton(page).click();
	await expect(sourceButton(page, 'live')).toHaveClass(/active/);

	const applyAll = page.locator('#caps-tab .reforge-apply-all');
	await expect(applyAll).toBeEnabled();
	// expertise is worth nothing at this gear, so no suggestion reforges into it
	await expect(page.locator('#caps-tab tr.reforge-row[data-suggestion$="→ Expertise"]')).toHaveCount(0);
	await applyAll.click();

	await expect(reforgeStatus(page)).toHaveText('Your character changed since the live run: measure again to score reforges with it.');
	await expect(priorityTable(page)).toHaveClass(/priority-stale/);
	await expect(prioritySource(page)).toContainText('measure again');
});

test('a healer measures in HPS, and the suggestions use the run', async ({ page }) => {
	await stubSlopes(page, HEALER_RESULT);
	await openCapsTab(page, 'holy_paladin');
	await expect(page.locator('#caps-tab .priority-healer')).toContainText('measure yours at your gear');
	await expect(reforgeStatus(page)).toContainText('Measure at my gear above');

	await measureButton(page).click();
	await expect.poll(() => order(page)).toEqual(['spellPower', 'haste', 'intellect', 'crit', 'mp5', 'spirit']);
	await expect(priorityTable(page).locator('th').nth(1)).toHaveText('HPS per point');
	await expect(sourceSwitch(page)).toBeHidden();
	await expect(reforgeStatus(page)).not.toContainText('Measure at my gear');
});

test("a failed run says why and leaves the shipped values up", async ({ page }) => {
	await stubSlopes(page, StatSlopesResult.create({ errorResult: 'no stat list for SpecUnknown' }));
	await openCapsTab(page, 'warrior');
	await measureButton(page).click();
	await expect(measureStatus(page)).toHaveText('The live run failed: no stat list for SpecUnknown');
	await expect(measureButton(page)).toBeEnabled();
	await expect(sourceSwitch(page)).toBeHidden();
	await expect.poll(() => order(page)).toEqual(['attackPower', 'haste']);
});

test('a real run on the sim server measures the default warrior', async ({ page }) => {
	test.setTimeout(120_000);
	const errors = watchForErrors(page);
	await openSim(page, 'warrior');
	await openSimTab(page, 'caps-tab');
	await measureButton(page).click();
	await expect(sourceButton(page, 'live')).toHaveClass(/active/, { timeout: 90_000 });
	await expect(measureStatus(page)).toHaveText('');
	await expect(priorityTable(page)).not.toHaveClass(/priority-stale/);
	expect(Number(await row(page, 'strength').getAttribute('data-value'))).toBeGreaterThan(0);
	expect(Number(await row(page, 'strength').getAttribute('data-loss'))).toBeGreaterThan(0);
	expect(errors).toEqual([]);
});
