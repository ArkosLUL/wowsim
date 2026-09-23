import { expect, test } from '@playwright/test';

import { addPreset, fixtureSettings, removePlayer, seedRaid, statCount } from './raid';

// Bloodlust and Innervate each come from one class, so the fixture says how many to expect.
const classSlots = (playerClass: string): Array<number> =>
	fixtureSettings().raid.parties.flatMap((party: any, partyIndex: number) =>
		party.players.flatMap((player: any, slot: number) => (player.class == playerClass ? [partyIndex * 5 + slot] : [])),
	);

test('the raid stats follow raiders coming and going', async ({ page }) => {
	await seedRaid(page);
	const shamans = classSlots('ClassShaman');
	const druids = classSlots('ClassDruid');
	await expect.poll(() => statCount(page, 'Bloodlust')).toBe(shamans.length);
	await expect.poll(() => statCount(page, 'Innervate')).toBe(druids.length);

	await removePlayer(page, shamans[0]);
	await expect.poll(() => statCount(page, 'Bloodlust')).toBe(shamans.length - 1);

	// a druid in the shaman's old seat
	await addPreset(page, 'Balance Druid', shamans[0]);
	await expect.poll(() => statCount(page, 'Bloodlust')).toBe(shamans.length - 1);
	await expect.poll(() => statCount(page, 'Innervate')).toBe(druids.length + 1);

	await page.locator('.raid-controls .enum-picker-root', { hasText: 'Raid Size' }).locator('select').selectOption({ label: '10' });
	await expect.poll(() => statCount(page, 'Bloodlust')).toBe(shamans.filter(raidIndex => raidIndex < 10 && raidIndex != shamans[0]).length);
	await expect.poll(() => statCount(page, 'Innervate')).toBe([...druids, shamans[0]].filter(raidIndex => raidIndex < 10).length);
});
