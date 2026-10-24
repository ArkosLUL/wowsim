import { expect, test } from '@playwright/test';

import { numberInput, openSpec, setNumber } from './helpers';

test("the Holy Paladin page's default rotation heals the target dummies", async ({ page }) => {
	await openSpec(page, 'holy_paladin');
	await setNumber(numberInput(page.locator('.sim-sidebar'), 'Iterations'), 50);

	await page.locator('.sim-sidebar-actions button', { hasText: 'Simulate' }).click();

	const hps = page.locator('.sim-sidebar-results .results-sim-hps .topline-result-avg');
	await expect.poll(async () => Number(await hps.innerText()), { timeout: 120_000 }).toBeGreaterThan(1000);
});
