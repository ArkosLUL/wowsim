import { expect, test, type Page } from '@playwright/test';

import { openSimTab, watchForErrors } from '../lib/page';
import {
	acceptDialogs,
	changeEverything,
	checkbox,
	expectActive,
	icon,
	iconEnum,
	inputRoot,
	isActive,
	multiIcon,
	numberInput,
	openMultiIcon,
	openSpec,
	reloadSim,
	savedData,
	select,
	setNumber,
	snapshot,
	storageKey,
	storedSettings,
} from './helpers';

const SPEC = 'mage';

const settingsTab = (page: Page) => page.locator('#settings-tab');

async function openSettings(page: Page, spec = SPEC) {
	await openSpec(page, spec, 'settings-tab');
}

test('raid buffs and debuffs survive a reload', async ({ page }) => {
	await openSettings(page);
	const buffs = settingsTab(page).locator('.buffs-settings');
	const debuffs = settingsTab(page).locator('.debuffs-settings');

	const lust = icon(buffs, 'spell', 2825);
	const lustWas = await isActive(lust);
	await lust.click();
	await expectActive(lust, !lustWas);

	const revitalize = icon(buffs, 'spell', 26982);
	await openMultiIcon(buffs, 'Revit');
	const revitalizeWas = await isActive(revitalize);
	await revitalize.click();
	await expectActive(revitalize, !revitalizeWas);

	const wisdom = icon(debuffs, 'spell', 53408);
	const wisdomWas = await isActive(wisdom);
	await wisdom.click();
	await expectActive(wisdom, !wisdomWas);

	await reloadSim(page);
	await openSimTab(page, 'settings-tab');

	await expectActive(lust, !lustWas);
	await expectActive(revitalize, !revitalizeWas);
	await expectActive(wisdom, !wisdomWas);
	const stored = await storedSettings(page, SPEC);
	expect(!!stored.raidBuffs.bloodlust).toBe(!lustWas);
	expect(!!stored.debuffs.judgementOfWisdom).toBe(!wisdomWas);
});

test('a buff category shows the buff picked in it', async ({ page }) => {
	await openSettings(page);
	const revit = multiIcon(settingsTab(page).locator('.buffs-settings'), 'Revit');
	const button = revit.locator('.dropend > a.icon-picker-button');

	await expectActive(button, false);
	await openMultiIcon(settingsTab(page).locator('.buffs-settings'), 'Revit');
	await icon(revit, 'spell', 26982).click();
	await expectActive(button, true);

	// A right click on the category clears it.
	await button.click({ button: 'right' });
	await expectActive(button, false);
	await expectActive(icon(revit, 'spell', 26982), false);
});

test('consumables survive a reload', async ({ page }) => {
	await openSettings(page);
	const consumes = settingsTab(page).locator('.consumes-settings');

	const flask = iconEnum(consumes, 'item', 46378);
	await flask.pick('item', 46378);
	await expect(flask.button).toHaveAttribute('href', /item=46378/);

	const food = iconEnum(consumes, 'item', 43015);
	await food.pick('item', 43015);
	await expect(food.button).toHaveAttribute('href', /item=43015/);

	const prepot = iconEnum(consumes.locator('.consumes-prepot'), 'item', 40212);
	await prepot.pick('item', 40212);
	await expect(prepot.button).toHaveAttribute('href', /item=40212/);

	const sapper = icon(consumes, 'item', 42641);
	const sapperWas = await isActive(sapper);
	await sapper.click();
	await expectActive(sapper, !sapperWas);

	await reloadSim(page);
	await openSimTab(page, 'settings-tab');

	await expect(flask.button).toHaveAttribute('href', /item=46378/);
	await expect(food.button).toHaveAttribute('href', /item=43015/);
	await expect(prepot.button).toHaveAttribute('href', /item=40212/);
	await expectActive(sapper, !sapperWas);
});

test('the player section survives a reload', async ({ page }) => {
	await openSettings(page);
	const player = settingsTab(page).locator('.player-settings');

	const race = select(player, 'Race');
	const oldRace = await race.inputValue();
	const races = await race.locator('option').evaluateAll(options => options.map(o => (o as HTMLOptionElement).value));
	const newRace = races.find(r => r != oldRace)!;
	await race.selectOption(newRace);

	await select(player, 'Racial Traits').selectOption({ label: 'Orc' });

	const tailoring = checkbox(player, 'Tailoring');
	const engineering = checkbox(player, 'Engineering');
	const tailoringWas = await tailoring.isChecked();
	const engineeringWas = await engineering.isChecked();
	await tailoring.setChecked(!tailoringWas);
	await engineering.setChecked(!engineeringWas);

	const armor = iconEnum(player, 'spell', 43024);
	await armor.pick('spell', 43024);

	await reloadSim(page);
	await openSimTab(page, 'settings-tab');

	await expect(race).toHaveValue(newRace);
	await expect(select(player, 'Racial Traits').locator('option:checked')).toHaveText('Orc');
	await expect(tailoring).toBeChecked({ checked: !tailoringWas });
	await expect(engineering).toBeChecked({ checked: !engineeringWas });
	await expect(armor.button).toHaveAttribute('href', /spell=43024/);
});

test('engineering consumables only show for engineers', async ({ page }) => {
	await openSettings(page);
	const engineering = checkbox(settingsTab(page).locator('.player-settings'), 'Engineering');
	const sapper = icon(settingsTab(page).locator('.consumes-settings'), 'item', 42641);

	await engineering.setChecked(true);
	await expect(sapper).toBeVisible();

	await engineering.setChecked(false);
	await expect(sapper).toBeHidden();

	await reloadSim(page);
	await openSimTab(page, 'settings-tab');
	await expect(sapper).toBeHidden();
});

test('the other section survives a reload', async ({ page }) => {
	await openSettings(page);
	const other = settingsTab(page).locator('.other-settings');

	await setNumber(numberInput(other, 'Reaction Time'), 321);
	await setNumber(numberInput(other, 'Distance From Target'), 17);
	await setNumber(numberInput(other, 'Focus Magic Percent Uptime'), 42);

	await reloadSim(page);

	await expect(numberInput(other, 'Reaction Time')).toHaveValue('321');
	await expect(numberInput(other, 'Distance From Target')).toHaveValue('17');
	await expect(numberInput(other, 'Focus Magic Percent Uptime')).toHaveValue('42');
});

test('the encounter survives a reload', async ({ page }) => {
	await openSettings(page);
	const encounter = settingsTab(page).locator('.encounter-settings');

	await setNumber(numberInput(encounter, 'Duration'), 240);
	await setNumber(numberInput(encounter, 'Duration +/-'), 12);
	// Percents whose fraction has no exact float, like 0.29.
	await setNumber(numberInput(encounter, 'Execute Duration 20 (%)'), 29);
	await setNumber(numberInput(encounter, 'Execute Duration 25 (%)'), 58);
	await setNumber(numberInput(encounter, 'Execute Duration 35 (%)'), 57);

	await reloadSim(page);

	await expect(numberInput(encounter, 'Duration')).toHaveValue('240');
	await expect(numberInput(encounter, 'Duration +/-')).toHaveValue('12');
	await expect(numberInput(encounter, 'Execute Duration 20 (%)')).toHaveValue('29');
	await expect(numberInput(encounter, 'Execute Duration 25 (%)')).toHaveValue('58');
	await expect(numberInput(encounter, 'Execute Duration 35 (%)')).toHaveValue('57');
	const stored = (await storedSettings(page, SPEC)).encounter;
	expect(stored.duration).toBe(240);
	expect(stored.executeProportion20).toBeCloseTo(0.29);
	expect(stored.executeProportion35).toBeCloseTo(0.57);
});

test('a preset NPC survives a reload', async ({ page }) => {
	await openSettings(page);
	const npc = select(settingsTab(page).locator('.encounter-settings'), 'NPC');

	const names = await npc.locator('option').allInnerTexts();
	const preset = names[names.length - 1];
	await npc.selectOption({ label: preset });

	await reloadSim(page);

	await expect(npc.locator('option:checked')).toHaveText(preset);
});

const advancedModal = (page: Page) => page.locator('.modal:has(.advanced-encounter-picker-modal)');

async function openAdvanced(page: Page) {
	await settingsTab(page).locator('.encounter-settings .advanced-button').click();
	await expect(advancedModal(page)).toBeVisible();
	return advancedModal(page);
}

async function closeModal(page: Page) {
	await page.locator('.modal.show .close-button').click();
	await expect(page.locator('.modal.show')).toHaveCount(0);
}

test('targets added in the advanced encounter survive a reload', async ({ page }) => {
	await openSettings(page);

	let modal = await openAdvanced(page);
	await modal.locator('.targets-picker > .list-picker-new-button').click();
	const targets = modal.locator('.targets-picker .target-picker-root');
	await expect(targets).toHaveCount(2);
	await select(targets.nth(1), 'Level').selectOption('80');
	await setNumber(numberInput(targets.nth(1), 'Armor'), 5000);
	await closeModal(page);

	await reloadSim(page);
	await openSimTab(page, 'settings-tab');

	modal = await openAdvanced(page);
	await expect(targets).toHaveCount(2);
	await expect(select(targets.nth(1), 'Level')).toHaveValue('80');
	await expect(numberInput(targets.nth(1), 'Armor')).toHaveValue('5000');
});

test('every field of an advanced encounter target survives a reload', async ({ page }) => {
	await openSettings(page);
	let modal = await openAdvanced(page);
	await modal.locator('.targets-picker > .list-picker-new-button').click();
	const target = modal.locator('.targets-picker .target-picker-root').nth(1);

	await changeEverything(target);
	const before = await snapshot(target);
	await closeModal(page);

	await reloadSim(page);
	await openSimTab(page, 'settings-tab');
	modal = await openAdvanced(page);
	expect(await snapshot(target)).toEqual(before);
});

test('a healer keeps the number of allies it is given', async ({ page }) => {
	await openSettings(page, 'healing_priest');
	const allies = numberInput(settingsTab(page).locator('.encounter-settings'), 'Num Allies');

	await setNumber(allies, 4);
	await reloadSim(page);
	await expect(allies).toHaveValue('4');
});

test('editing a preset NPC makes it custom, and picking the preset again restores it', async ({ page }) => {
	await openSettings(page);
	const npc = select(settingsTab(page).locator('.encounter-settings'), 'NPC');
	const names = await npc.locator('option').allInnerTexts();
	const preset = names[1];
	await npc.selectOption({ label: preset });

	const modal = await openAdvanced(page);
	const target = modal.locator('.targets-picker .target-picker-root').first();
	const presetArmor = await numberInput(target, 'Armor').inputValue();
	await setNumber(numberInput(target, 'Armor'), Number(presetArmor) + 1234);
	await closeModal(page);

	await expect(npc.locator('option:checked')).toHaveText('Custom');

	await npc.selectOption({ label: names[2] });
	await npc.selectOption({ label: preset });
	await openAdvanced(page);
	await expect(numberInput(target, 'Armor')).toHaveValue(presetArmor);
});

test('editing a preset encounter makes it custom, and picking the preset again restores it', async ({ page }) => {
	await openSettings(page);
	const modal = await openAdvanced(page);
	const encounterPreset = select(modal.locator('.modal-header'), 'Encounter');
	const names = await encounterPreset.locator('option').allInnerTexts();
	await encounterPreset.selectOption({ label: names[1] });

	const armor = numberInput(modal.locator('.target-picker-root').first(), 'Armor');
	const presetArmor = await armor.inputValue();
	await setNumber(armor, Number(presetArmor) + 777);
	await expect(encounterPreset.locator('option:checked')).toHaveText('Custom');

	await encounterPreset.selectOption({ label: names[2] });
	await encounterPreset.selectOption({ label: names[1] });
	await expect(armor).toHaveValue(presetArmor);
});

test('tank specs keep the boss damage they are given', async ({ page }) => {
	await openSettings(page, 'protection_warrior');
	const damage = numberInput(settingsTab(page).locator('.encounter-settings'), 'Min Base Damage');

	await setNumber(damage, 43210);
	await expect(damage).toHaveValue('43210');

	await reloadSim(page);
	await expect(damage).toHaveValue('43210');
	expect((await storedSettings(page, 'protection_warrior')).encounter.targets[0].minBaseDamage).toBe(43210);
});

test('the server section survives a reload', async ({ page }) => {
	await openSettings(page);
	const server = settingsTab(page).locator('.server-settings');

	await setNumber(numberInput(server, 'Map update interval (ms)'), 50);
	await select(server, 'Raid difficulty').selectOption({ label: '10 player heroic' });
	const bossHealth = server.locator('.dungeon-scale-picker input').nth(2);
	await setNumber(bossHealth, 2.5);
	const armorPen = checkbox(server, 'Hunter pet armor pen');
	const armorPenWas = await armorPen.isChecked();
	await armorPen.setChecked(!armorPenWas);
	await setNumber(numberInput(server, 'Exotic pet damage %'), 10);
	await setNumber(numberInput(server, 'Reforge %'), 30);
	await setNumber(server.locator('.input-root:has(> label:text-is("Reforgeable stats")) input'), '6,13');

	await reloadSim(page);

	await expect(numberInput(server, 'Map update interval (ms)')).toHaveValue('50');
	await expect(select(server, 'Raid difficulty').locator('option:checked')).toHaveText('10 player heroic');
	await expect(bossHealth).toHaveValue('2.50');
	await expect(armorPen).toBeChecked({ checked: !armorPenWas });
	await expect(numberInput(server, 'Exotic pet damage %')).toHaveValue('10.00');
	await expect(numberInput(server, 'Reforge %')).toHaveValue('30.00');
	await expect(server.locator('.input-root:has(> label:text-is("Reforgeable stats")) input')).toHaveValue('6,13');
});

test('turning spell tweaks off greys out the switches it gates', async ({ page }) => {
	await openSettings(page);
	const server = settingsTab(page).locator('.server-settings');

	await checkbox(server, 'Spell tweaks').setChecked(false);
	await expect(checkbox(server, 'Hunter pet haste')).toBeDisabled();
	await expect(checkbox(server, 'Hunter pet haste')).not.toBeChecked();
	await expect(checkbox(server, 'Hunter pet armor pen')).toBeEnabled();

	await reloadSim(page);
	await expect(checkbox(server, 'Spell tweaks')).not.toBeChecked();
	await expect(checkbox(server, 'Hunter pet haste')).toBeDisabled();
});

test('saved settings save, load and delete, and survive a reload', async ({ page }) => {
	acceptDialogs(page);
	await openSettings(page);
	const saved = savedData(settingsTab(page), 'Saved Settings');
	const lust = icon(settingsTab(page).locator('.buffs-settings'), 'spell', 2825);
	const lustWas = await isActive(lust);

	await saved.save('Raid night');
	await expectActive(saved.chip('Raid night'), true);

	await lust.click();
	await expectActive(saved.chip('Raid night'), false);

	await reloadSim(page);
	await openSimTab(page, 'settings-tab');

	await expectActive(lust, !lustWas);
	await saved.chip('Raid night').locator('.saved-data-set-name').click();
	await expectActive(lust, lustWas);
	await expectActive(saved.chip('Raid night'), true);

	await saved.chip('Raid night').locator('.saved-data-set-delete').click();
	await expect(saved.chip('Raid night')).toHaveCount(0);

	await reloadSim(page);
	await expect(saved.chip('Raid night')).toHaveCount(0);
});

test('a saved encounter keeps its targets after it is loaded and edited', async ({ page }) => {
	await openSettings(page);
	const saved = savedData(settingsTab(page), 'Saved Encounters');

	await saved.save('Stock boss');
	await saved.chip('Stock boss').locator('.saved-data-set-name').click();

	const modal = await openAdvanced(page);
	const armor = numberInput(modal.locator('.target-picker-root').first(), 'Armor');
	const savedArmor = await armor.inputValue();
	await setNumber(armor, Number(savedArmor) + 1000);
	await closeModal(page);

	await expectActive(saved.chip('Stock boss'), false);

	// Saving another one writes every saved encounter back to storage.
	await saved.save('Tougher boss');
	await reloadSim(page);
	await openSimTab(page, 'settings-tab');

	await saved.chip('Stock boss').locator('.saved-data-set-name').click();
	await openAdvanced(page);
	await expect(armor).toHaveValue(savedArmor);
});

test('a boss with inputs shows them, and they survive a reload', async ({ page }) => {
	await openSettings(page);
	const encounter = settingsTab(page).locator('.encounter-settings');
	await select(encounter, 'NPC').selectOption({ label: 'Ulduar 25/Hodir' });

	const uptime = numberInput(encounter, 'Starlight Uptime %');
	await expect(checkbox(encounter, 'Stormpower Prio')).toBeVisible();
	await setNumber(uptime, 55);

	await reloadSim(page);
	await openSimTab(page, 'settings-tab');
	await expect(uptime).toHaveValue('55');
});

test("switching bosses shows the new boss's inputs", async ({ page }) => {
	await openSettings(page);
	const encounter = settingsTab(page).locator('.encounter-settings');
	await select(encounter, 'NPC').selectOption({ label: 'Ulduar 25/Hodir' });
	await expect(checkbox(encounter, 'Stormpower Prio')).toBeVisible();

	await select(encounter, 'NPC').selectOption({ label: 'ICC 25/Sindragosa (Heroic)' });
	await expect(checkbox(encounter, 'Include Mystic Buffet')).toBeVisible();
	await expect(inputRoot(encounter, 'Stormpower Prio')).toHaveCount(0);
	await expect(inputRoot(encounter, 'Starlight Uptime %')).toHaveCount(0);
});

test('deleting a target leaves the other targets editable', async ({ page }) => {
	const errors = watchForErrors(page);
	await openSettings(page);
	const modal = await openAdvanced(page);
	const targets = modal.locator('.targets-picker .target-picker-root');
	const newTarget = modal.locator('.targets-picker > .list-picker-new-button');

	await newTarget.click();
	await expect(targets).toHaveCount(2);
	await modal.locator('.targets-picker .list-picker-item-delete').nth(1).click();
	await expect(targets).toHaveCount(1);

	await newTarget.click();
	await expect(targets).toHaveCount(2);
	await select(targets.nth(1), 'NPC').selectOption({ label: 'Ulduar 25/Hodir' });
	await expect(select(targets.nth(1), 'AI').locator('option:checked')).toHaveText('Ulduar 25/Hodir');
	expect(errors).toEqual([]);
	await expect(checkbox(targets.nth(1), 'Stormpower Prio')).toBeVisible();
});

test('with every target deleted, the encounter stays editable and Simulate and Stat Weights say so', async ({ page }) => {
	const errors = watchForErrors(page);
	const dialogs = acceptDialogs(page);
	await openSettings(page, 'protection_warrior');
	const modal = await openAdvanced(page);
	await modal.locator('.targets-picker .list-picker-item-delete').first().click();
	await expect(modal.locator('.targets-picker .target-picker-root')).toHaveCount(0);
	await closeModal(page);
	const damage = numberInput(settingsTab(page).locator('.encounter-settings'), 'Min Base Damage');
	await setNumber(damage, 43210);
	await expect(damage).toHaveValue('0');
	expect(errors).toEqual([]);

	const noTargets = 'Error: Encounter has no targets! Try adding some targets first.';
	await page.locator('.sim-sidebar-actions button', { hasText: 'Simulate' }).click();
	await expect.poll(() => dialogs).toEqual([noTargets]);

	await page.locator('.sim-sidebar-actions button', { hasText: 'Stat Weights' }).click();
	const weights = page.locator('.modal.show:has(.ep-weights-menu)');
	await weights.locator('.calc-weights').click();
	await expect.poll(() => dialogs).toEqual([noTargets, noTargets]);
	await expect(weights.locator('.calc-weights')).not.toHaveClass(/disabled/);
});

test('a saved boss whose inputs are missing gets its preset inputs back', async ({ page }) => {
	await openSettings(page);
	const npc = select(settingsTab(page).locator('.encounter-settings'), 'NPC');
	await npc.selectOption({ label: 'Ulduar 25/Hodir' });
	await expect(inputRoot(settingsTab(page).locator('.encounter-settings'), 'Stormpower Prio')).toBeVisible();
	const presetUptime = await numberInput(settingsTab(page).locator('.encounter-settings'), 'Starlight Uptime %').inputValue();

	// Like settings saved before the boss had inputs.
	await page.evaluate(key => {
		const settings = JSON.parse(localStorage.getItem(key)!);
		delete settings.encounter.targets[0].targetInputs;
		localStorage.setItem(key, JSON.stringify(settings));
	}, storageKey(SPEC));
	await reloadSim(page);
	await openSimTab(page, 'settings-tab');

	await expect(inputRoot(settingsTab(page).locator('.encounter-settings'), 'Stormpower Prio')).toBeVisible();
	await expect(numberInput(settingsTab(page).locator('.encounter-settings'), 'Starlight Uptime %')).toHaveValue(presetUptime);
});

test('a saved encounter shows on every spec page', async ({ page }) => {
	await openSettings(page);
	await savedData(settingsTab(page), 'Saved Encounters').save('Shared boss');

	await openSettings(page, 'warlock');
	await expect(savedData(settingsTab(page), 'Saved Encounters').chip('Shared boss')).toHaveCount(1);
});
