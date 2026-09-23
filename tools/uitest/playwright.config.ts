import { defineConfig } from '@playwright/test';

// The sim is served by a container, never by Playwright, so there's no `webServer` here.
// UITEST_URL points at whichever one holds the build under test.
const baseURL = process.env.UITEST_URL ?? 'http://localhost:3334';

export default defineConfig({
	testDir: './tests',
	fullyParallel: true,
	forbidOnly: !!process.env.CI,
	retries: 0,
	reporter: process.env.CI ? [['list'], ['html', { open: 'never' }]] : [['list']],
	use: {
		baseURL,
		// The host has Chrome but not Playwright's own browsers. `npx playwright install chromium`
		// then UITEST_BROWSER=chromium if you want the pinned build instead.
		channel: process.env.UITEST_BROWSER ?? 'chrome',
		headless: true,
		viewport: { width: 1600, height: 1100 },
		trace: 'retain-on-failure',
		screenshot: 'only-on-failure',
	},
});
