import { expect, test } from '@playwright/test';

import { Fixture, loadFixture, showFixture } from '../lib/fixture';
import { watchForErrors } from '../lib/page';

let fixture: Fixture;

test.beforeAll(() => {
	fixture = loadFixture();
});

test('the damage table lists every raider, highest first', async ({ page }) => {
	await showFixture(page, fixture);

	const rows = page.locator('.player-damage-metrics-root .player-damage-row');
	await expect(rows).toHaveCount(fixture.players.length);

	const shown = await page.locator('.player-damage-metrics-root .player-damage-row td:last-child').allInnerTexts();
	const expected = [...fixture.players].sort((a, b) => b.dps - a.dps).map(player => player.dps.toFixed(1));
	expect(shown).toEqual(expected);
});

test('hovering a row puts its chart in the tooltip, not in the row', async ({ page }) => {
	await showFixture(page, fixture);

	const row = page.locator('.player-damage-metrics-root .player-damage-row').first();
	const before = (await row.boundingBox())!.height;

	await row.hover();
	await expect(page.locator('.tippy-content canvas')).toBeVisible();

	expect(await row.locator('canvas').count()).toBe(0);
	expect((await row.boundingBox())!.height).toBe(before);
});

test("the row's tooltip draws its damage breakdown", async ({ page }) => {
	const errors = watchForErrors(page);
	await showFixture(page, fixture);

	await page.locator('.player-damage-metrics-root .player-damage-row').first().hover();
	const canvas = page.locator('.tippy-content canvas');
	await expect(canvas).toBeVisible();

	const drawn = () =>
		canvas.evaluate((elem: HTMLCanvasElement) => {
			const pixels = elem.getContext('2d')!.getImageData(0, 0, elem.width, elem.height).data;
			for (let alpha = 3; alpha < pixels.length; alpha += 4) {
				if (pixels[alpha] > 0) return true;
			}
			return false;
		});
	await expect.poll(drawn).toBe(true);
	expect(errors).toEqual([]);
});

test('the percentages add up to the whole raid', async ({ page }) => {
	await showFixture(page, fixture);

	const percents = await page.locator('.player-damage-metrics-root .player-damage-percent').allInnerTexts();
	const total = percents.reduce((sum, text) => sum + parseFloat(text), 0);
	// Each one rounds to 0.01%.
	expect(Math.abs(total - 100)).toBeLessThan(0.005 * percents.length);
});
