import { readFileSync } from 'node:fs';
import path from 'node:path';

import { expect, type Page, test } from '@playwright/test';

import { equippedId, openPicker, pane } from '../gear/gear';
import { openSim, watchForErrors } from '../lib/page';
import { expectActive, icon, numberInput } from '../settings/helpers';

const FIXTURE = JSON.parse(readFileSync(path.join(__dirname, 'rankings.fixture.json'), 'utf-8'));
const REPO_ROOT = path.join(__dirname, '..', '..', '..', '..');

const ROLE_OF: Record<string, string> = {
	SpecDeathknight: 'melee',
	SpecHunter: 'ranged',
	SpecMage: 'caster',
	SpecWarlock: 'caster',
};

interface Row {
	key: string;
	tier: string;
	dps: string;
	pct: string;
}

// What the page should show for a content phase and fight, worked out from the fixture alone.
function expected(phase: number, fight: number, roles = ['melee', 'ranged', 'caster']): Row[] {
	const rows = FIXTURE.builds
		.map((b: any) => ({ b, stats: b.phases.find((p: any) => p.phase == phase).fights[String(fight)] }))
		.sort((x: any, y: any) => y.stats.dps - x.stats.dps);
	const top = rows[0].stats.dps;
	return rows
		.map(({ b, stats }: any) => {
			const pct = Math.round((1000 * stats.dps) / top) / 10;
			const tier = pct >= 95 ? 'S' : pct >= 90 ? 'A' : pct >= 85 ? 'B' : pct >= 75 ? 'C' : 'D';
			return { key: b.key, tier, dps: Math.round(stats.dps).toLocaleString('en-US'), pct: `${pct.toFixed(1)}%`, role: ROLE_OF[b.spec] };
		})
		.filter((row: any) => roles.includes(row.role))
		.map(({ key, tier, dps, pct }: any) => ({ key, tier, dps, pct }));
}

async function openRankings(page: Page, query = '') {
	await page.route('**/assets/rankings/rankings.json', route => route.fulfill({ json: FIXTURE }));
	await page.goto(`/wotlk/rankings/${query}`);
	await expect(page.locator('.rankings-row').first()).toBeVisible();
}

async function shownRows(page: Page): Promise<Row[]> {
	return page.locator('.rankings-row').evaluateAll(rows =>
		rows.map(row => ({
			key: (row as HTMLElement).dataset.build!,
			tier: (row.closest('.rankings-tier') as HTMLElement).dataset.tier!,
			dps: row.querySelector('.rankings-row-dps')!.textContent!,
			pct: row.querySelector('.rankings-row-pct')!.textContent!,
		})),
	);
}

const phaseTab = (page: Page, phase: number) => page.locator(`.rankings-phases [data-phase="${phase}"]`);
const fightButton = (page: Page, fight: number) => page.locator(`[data-fight="${fight}"]`);
const roleButton = (page: Page, role: string) => page.locator(`[data-role="${role}"]`);

test('ranks content phase 5 at 3 min by default, graded against the top build', async ({ page }) => {
	const errors = watchForErrors(page);
	await openRankings(page);

	expect(await shownRows(page)).toEqual(expected(5, 180));
	await expectActive(phaseTab(page, 5), true);
	await expectActive(fightButton(page, 180), true);
	await expect(page.locator('.rankings-heading')).toContainText('P5: Ruby Sanctum');
	await expect(page).toHaveTitle('WOTLK DPS Rankings');
	expect(errors).toEqual([]);
});

test('switching the content phase and the fight re-ranks the builds', async ({ page }) => {
	await openRankings(page);

	await phaseTab(page, 1).click();
	expect(await shownRows(page)).toEqual(expected(1, 180));

	await fightButton(page, 60).click();
	expect(await shownRows(page)).toEqual(expected(1, 60));
	// 94.97% of the top shows as 95.0%, and grades as shown
	expect((await shownRows(page)).find(r => r.key == 'hunter_mm')).toMatchObject({ pct: '95.0%', tier: 'S' });

	await fightButton(page, 300).click();
	expect(await shownRows(page)).toEqual(expected(1, 300));

	await phaseTab(page, 5).click();
	expect(await shownRows(page)).toEqual(expected(5, 300));
});

test('the role filter hides rows but keeps their letters', async ({ page }) => {
	await openRankings(page);

	await roleButton(page, 'melee').click();
	await expectActive(roleButton(page, 'melee'), false);
	expect(await shownRows(page)).toEqual(expected(5, 180, ['ranged', 'caster']));
	// the top build is melee, so the hunter would be S if hiding it re-graded the rest
	expect((await shownRows(page))[0]).toMatchObject({ key: 'hunter_mm', tier: 'A' });

	await roleButton(page, 'caster').click();
	expect(await shownRows(page)).toEqual(expected(5, 180, ['ranged']));

	// the last role on stays on
	await roleButton(page, 'ranged').click();
	await expectActive(roleButton(page, 'ranged'), true);
	expect(await shownRows(page)).toEqual(expected(5, 180, ['ranged']));

	await roleButton(page, 'melee').click();
	expect(await shownRows(page)).toEqual(expected(5, 180, ['melee', 'ranged']));
});

test('the URL holds the content phase, fight and roles', async ({ page }) => {
	await openRankings(page, '?phase=1&fight=60&role=caster');

	await expectActive(phaseTab(page, 1), true);
	await expectActive(fightButton(page, 60), true);
	await expectActive(roleButton(page, 'caster'), true);
	await expectActive(roleButton(page, 'melee'), false);
	expect(await shownRows(page)).toEqual(expected(1, 60, ['caster']));

	await phaseTab(page, 5).click();
	await roleButton(page, 'ranged').click();
	expect(new URL(page.url()).search).toBe('?phase=5&fight=60&role=ranged%2Ccaster');

	await page.reload();
	await expect(page.locator('.rankings-row').first()).toBeVisible();
	expect(await shownRows(page)).toEqual(expected(5, 60, ['ranged', 'caster']));

	await roleButton(page, 'melee').click();
	expect(new URL(page.url()).searchParams.has('role')).toBe(false);
});

test('an unknown phase or fight in the URL falls back to the defaults', async ({ page }) => {
	await openRankings(page, '?phase=3&fight=42&role=tank');

	await expectActive(phaseTab(page, 5), true);
	await expectActive(fightButton(page, 180), true);
	expect(await shownRows(page)).toEqual(expected(5, 180));
});

test("each link opens the spec's sim in a new tab, at the selected fight", async ({ page }) => {
	await openRankings(page);

	const links = page.locator('.rankings-row-link');
	await expect(links).toHaveCount(FIXTURE.builds.length);
	const at180: Record<string, string> = {};
	for (const build of FIXTURE.builds) {
		const link = page.locator(`.rankings-row[data-build="${build.key}"] .rankings-row-link`);
		await expect(link).toHaveAttribute('target', '_blank');
		await expect(link).toHaveAttribute('rel', /noopener/);
		const href = (await link.getAttribute('href'))!;
		expect(href).toMatch(new RegExp(`^/wotlk/${build.dir}/\\?i=[a-z]*u[a-z]*#.+`));
		at180[build.key] = href;
	}

	await fightButton(page, 60).click();
	const at60 = await page.locator('.rankings-row[data-build="dk_unholy"] .rankings-row-link').getAttribute('href');
	expect(at60).not.toBe(at180['dk_unholy']);
});

test('the chart legend follows the role filter', async ({ page }) => {
	await openRankings(page);

	const legend = page.locator('.rankings-legend-item');
	await expect(legend).toHaveCount(FIXTURE.builds.length);
	await expect(page.locator('.rankings-chart canvas')).toBeVisible();

	await roleButton(page, 'caster').click();
	await expect(page.locator('.rankings-legend-item:visible')).toHaveCount(2);
	await expect(page.locator('.rankings-legend-item[data-build="mage_fire"]')).toBeHidden();

	await page.locator('[data-mode="rank"]').click();
	await expectActive(page.locator('[data-mode="rank"]'), true);
	await expectActive(page.locator('[data-mode="dps"]'), false);
});

test('a row shows its numbers on hover and on keyboard focus', async ({ page }) => {
	await openRankings(page);

	const tooltip = page.locator('.tippy-box .rankings-tooltip');
	await page.locator('.rankings-row[data-build="hunter_mm"]').focus();
	await expect(tooltip).toBeVisible();
	for (const text of ['Hunter MM, 3 min fight', '9,200 DPS', '±276', '8,280', '10,120', '#2 of 4']) {
		await expect(tooltip).toContainText(text);
	}

	await page.locator('.rankings-row[data-build="hunter_mm"]').blur();
	await expect(tooltip).toHaveCount(0);

	await page.locator('.rankings-row[data-build="mage_fire"] .rankings-row-name').hover();
	await expect(tooltip).toContainText('Mage Fire, 3 min fight');
});

test('at phone width the bars keep one scale and nothing scrolls sideways', async ({ page }) => {
	await page.setViewportSize({ width: 390, height: 844 });
	await openRankings(page);

	const rows = await page.locator('.rankings-row').evaluateAll(rows =>
		rows.map(row => ({
			key: (row as HTMLElement).dataset.build!,
			column: row.querySelector('.rankings-row-bar')!.getBoundingClientRect().width,
			bar: row.querySelector('.rankings-bar')!.getBoundingClientRect().width,
		})),
	);
	expect(new Set(rows.map(row => row.column)).size).toBe(1);
	const dps = (key: string) => FIXTURE.builds.find((b: any) => b.key == key).phases.find((p: any) => p.phase == 5).fights['180'].dps;
	for (const row of rows) {
		expect(row.bar / rows[0].bar).toBeCloseTo(dps(row.key) / dps(rows[0].key), 2);
	}

	const clipped = await page.locator('.rankings-row-dps, .rankings-row-pct').evaluateAll(cells => cells.filter(c => c.scrollWidth > c.clientWidth).length);
	expect(clipped).toBe(0);
	expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBe(390);
});

// Against the committed rankings, which have all five content phases.
test('every content phase tab fits, from phone to wide desktop', async ({ page }) => {
	for (const width of [390, 800, 1100, 1920]) {
		await page.setViewportSize({ width, height: 900 });
		await page.goto('/wotlk/rankings/');
		await expect(page.locator('.rankings-phases [data-phase]')).toHaveCount(5);
		const tabs = await page.locator('.rankings-phases').evaluate(nav => nav.scrollWidth - nav.clientWidth);
		expect(tabs, `tabs overflow at ${width}px`).toBe(0);
	}
});

test('the footer names the sim, the data and the fight', async ({ page }) => {
	await openRankings(page);

	const footer = page.locator('.rankings-footer');
	await expect(footer).toContainText('sim 012345678-dirty');
	await expect(footer).toContainText(`item catalog ${FIXTURE.catalog_date}`);
	await expect(footer).toContainText(`${FIXTURE.iterations.toLocaleString('en-US')} iterations`);
	await expect(footer).toContainText('3 min fight');
	await expect(footer).toContainText('a Draenei in every party');
	await expect(footer).toContainText('playerbot rules');

	await fightButton(page, 300).click();
	await expect(footer).toContainText('5 min fight');
});

test('the homepage and the sim title dropdown link to the rankings', async ({ page }) => {
	await page.goto('/wotlk/');
	const home = page.locator('a.sim-link[href="/wotlk/rankings/"]');
	await expect(home).toContainText('DPS Rankings');

	await page.route('**/assets/rankings/rankings.json', route => route.fulfill({ json: FIXTURE }));
	await home.click();
	await expect(page).toHaveURL(/\/wotlk\/rankings\/$/);
	await expect(page.locator('.rankings-row').first()).toBeVisible();

	await openSim(page, 'mage');
	await expect(page.locator('.sim-title-dropdown-root a[href="/wotlk/rankings/"]')).toHaveCount(1);
});

// Against the committed rankings: the link must load exactly what was simmed.
test('a ranked link opens its sim with that content phase BiS and the standard buffs', async ({ page }) => {
	await page.context().route('https://wow.zamimg.com/js/tooltips.js', route => route.fulfill({ contentType: 'text/javascript', body: '' }));
	await page.goto('/wotlk/rankings/?phase=4&fight=60');
	const row = page.locator('.rankings-row[data-build="shadow_priest"]');
	await expect(row).toBeVisible();

	const [sim] = await Promise.all([page.waitForEvent('popup'), row.locator('.rankings-row-link').click()]);
	await sim.waitForLoadState('networkidle');
	await expect(sim.locator('.sim-ui')).toBeVisible();

	const preset = JSON.parse(readFileSync(path.join(REPO_ROOT, 'ui/shadow_priest/gear_sets/p4_bis.gear.json'), 'utf-8'));
	await expect.poll(() => equippedId(sim, 'head')).toBe(preset.items[0].id);

	const settings = sim.locator('#settings-tab');
	await expect(numberInput(settings.locator('.encounter-settings'), 'Duration')).toHaveValue('60');
	const buffs = settings.locator('.buffs-settings');
	await expectActive(icon(buffs, 'spell', 6562), true); // Heroic Presence
	await expectActive(icon(buffs, 'spell', 2825), true); // Bloodlust
	await expectActive(icon(buffs, 'spell', 54648), false); // Focus Magic

	const picker = await openPicker(sim, 'head');
	await expect(pane(picker, 'Items').locator('.selector-modal-phase-selector select')).toHaveValue('4');
});
