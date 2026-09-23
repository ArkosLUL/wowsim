import { expect, test, type Page } from '@playwright/test';

import { closePicker, detached, openGear, openPicker } from './gear';

// The window's resize listeners by function name, read over the DevTools protocol since the page
// can't list its own listeners.
async function resizeListeners(page: Page): Promise<string[]> {
	const cdp = await page.context().newCDPSession(page);
	const { result } = await cdp.send('Runtime.evaluate', { expression: 'window', objectGroup: 'listeners' });
	const { listeners } = await cdp.send('DOMDebugger.getEventListeners', { objectId: result.objectId! });
	const names: string[] = [];
	for (const listener of listeners.filter(l => l.type == 'resize')) {
		const { result: name } = await cdp.send('Runtime.callFunctionOn', {
			functionDeclaration: 'function () { return this.name; }',
			objectId: listener.handler!.objectId!,
			returnByValue: true,
		});
		names.push(String(name.value));
	}
	await cdp.send('Runtime.releaseObjectGroup', { objectGroup: 'listeners' });
	await cdp.detach();
	return names;
}

// the item lists' virtual scroller listens with its resizeEv method (virtual_scroll/clusterize.ts)
const scrollers = (names: string[]) => names.filter(n => n == 'resizeEv').length;

// A modal only disposes of its contents once its fade out ends and it leaves the page.
async function openAndClose(page: Page, times: number) {
	for (let i = 0; i < times; i++) {
		await openPicker(page, 'head');
		await closePicker(page);
		await expect(page.locator('.selector-modal')).toHaveCount(0);
	}
}

test('closing an item picker lets go of its lists', async ({ page }) => {
	await openGear(page);
	await openAndClose(page, 3);
	expect(scrollers(await resizeListeners(page))).toBe(0);
});

test('redrawing the gear tab lets go of the sockets it drew before', async ({ page }) => {
	await openGear(page);
	const presets = page.locator('#gear-tab .saved-data-presets .saved-data-set-chip .saved-data-set-name');
	expect(await presets.count()).toBeGreaterThan(1);

	// every gear change redraws all 17 slots, sockets included, and no picker is opened here
	for (let i = 0; i < 10; i++) await presets.nth(i % 2).click();
	await expect.poll(() => detached(page, '.gem-socket-container')).toBe(0);
});

test.fixme('closing a modal lets go of every window listener it added', async ({ page }) => {
	// base_modal.ts never disposes the Bootstrap Modal, which keeps a window resize listener
	await openGear(page);
	await openAndClose(page, 1);
	const settled = (await resizeListeners(page)).length;
	await openAndClose(page, 3);
	expect((await resizeListeners(page)).length).toBe(settled);
});
