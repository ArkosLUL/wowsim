import { expect, test } from '@playwright/test';

import { openSimTab } from '../lib/page';
import { numberInput, openSpec, reloadSim, setNumber, storedSettings } from './helpers';

const SPEC = 'mage';

// Each input of the server section that shapes the update grid, with the proto field it stores.
const TIMING_FIELDS = [
	{ label: 'Map update interval (ms)', field: 'mapUpdateIntervalMs', value: 50 },
	{ label: 'World update interval (ms)', field: 'worldUpdateIntervalMs', value: 20 },
	{ label: 'World loop time (ms)', field: 'worldLoopMs', value: 12.5 },
	{ label: 'Map update jitter (ms)', field: 'mapUpdateJitterMs', value: 3.5 },
	{ label: 'World long tick (ms)', field: 'worldLongTickMs', value: 25.5 },
	{ label: 'World long tick jitter (ms)', field: 'worldLongTickJitterMs', value: 4.5 },
];

const serverSettings = async (page: Parameters<typeof storedSettings>[0]) => (await storedSettings(page, SPEC)).encounter?.serverSettings ?? {};

for (const { label, field, value } of TIMING_FIELDS) {
	test(`${label} starts at the live value, saves a change and clears when set back`, async ({ page }) => {
		await openSpec(page, SPEC, 'settings-tab');
		const input = numberInput(page.locator('#settings-tab .server-settings'), label);

		const live = await input.inputValue();
		expect(Number(live)).toBeGreaterThan(0);
		expect(await serverSettings(page)).not.toHaveProperty(field);

		await setNumber(input, value);
		expect((await serverSettings(page))[field]).toBe(value);

		await reloadSim(page);
		await openSimTab(page, 'settings-tab');
		expect(Number(await input.inputValue())).toBe(value);

		await setNumber(input, live);
		expect(await serverSettings(page)).not.toHaveProperty(field);
	});
}
