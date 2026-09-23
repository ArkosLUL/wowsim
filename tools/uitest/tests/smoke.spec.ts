import { expect, test } from '@playwright/test';

import { SPECS, openRaid, openSim, watchForErrors } from './lib/page';

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
