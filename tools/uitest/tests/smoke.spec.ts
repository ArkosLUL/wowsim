import { expect, test } from '@playwright/test';

import { openRaid, openSim, SPECS, watchForErrors } from './lib/page';

for (const spec of SPECS) {
	test(`the ${spec} page loads cleanly`, async ({ page }) => {
		const errors = watchForErrors(page);
		await openSim(page, spec);
		await expect(page.locator('#gear-tab .item-picker-root')).toHaveCount(17);
		expect(errors).toEqual([]);
	});
}

test('the raid page loads cleanly', async ({ page }) => {
	const errors = watchForErrors(page);
	await openRaid(page);
	expect(errors).toEqual([]);
});

test('the detailed results page loads with no results', async ({ page }) => {
	await page.goto('/wotlk/detailed_results/index.html');
	await expect(page.locator('.dr-root')).toHaveClass(/dr-no-results/);
});

test('the error watcher skips a file the network dropped, but not one the server is missing', async ({ page }) => {
	const errors = watchForErrors(page);
	await page.route('**/uitest-dropped.png', route => route.abort('failed'));
	await page.goto('/wotlk/detailed_results/index.html');

	const load = (src: string) =>
		page.evaluate(
			src =>
				new Promise(done => {
					const img = new Image();
					img.onload = img.onerror = done;
					img.src = src;
				}),
			src,
		);
	await load('/wotlk/uitest-dropped.png');
	await load('/wotlk/uitest-missing.png');

	// console messages arrive in order, so the dropped one would be in by the time the 404 is
	await expect.poll(() => errors.length).toBeGreaterThan(0);
	expect(errors).toEqual([expect.stringContaining('404')]);
});
