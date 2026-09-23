import { expect, type Locator, type Page } from '@playwright/test';

import { openSim, openSimTab } from '../lib/page';

// enhancement's key keeps an old typo, so its saved settings survive
const KEY_PREFIX: Record<string, string> = { enhancement_shaman: '__wotlk_enhacement_shaman' };

export const storageKey = (spec: string, part = '__currentSettings__') => `${KEY_PREFIX[spec] ?? `__wotlk_${spec}`}${part}`;

// What the page wrote to localStorage for its settings, as protojson.
export async function storedSettings(page: Page, spec: string): Promise<any> {
	return page.evaluate(key => JSON.parse(localStorage.getItem(key) ?? 'null'), storageKey(spec));
}

// Wowhead's tooltips pop up over whatever the mouse last hovered and swallow the next click.
export async function stubWowheadTooltips(page: Page) {
	await page.route('https://wow.zamimg.com/js/tooltips.js', route => route.fulfill({ contentType: 'text/javascript', body: '' }));
}

// Opens a spec page, and one of its tabs if given, e.g. 'settings-tab'.
export async function openSpec(page: Page, spec: string, tab?: string) {
	await stubWowheadTooltips(page);
	await openSim(page, spec);
	if (tab) {
		await openSimTab(page, tab);
	}
}

export async function reloadSim(page: Page) {
	await page.reload();
	await page.waitForLoadState('networkidle');
	await expect(page.locator('.sim-ui')).toBeVisible();
}

type Scope = Page | Locator;

// Labels aren't tied to their inputs, so an input is found through the root it shares with its label.
export const inputRoot = (scope: Scope, label: string) => scope.locator(`.input-root:has(> label:text-is(${JSON.stringify(label)}))`).first();
export const numberInput = (scope: Scope, label: string) => inputRoot(scope, label).locator('input.number-picker-input');
export const checkbox = (scope: Scope, label: string) => inputRoot(scope, label).locator('input[type=checkbox]');
export const select = (scope: Scope, label: string) => inputRoot(scope, label).locator('select');

// Pickers only store a number on change, which a text input fires on blur.
export async function setNumber(input: Locator, value: number | string) {
	await input.fill(String(value));
	await input.blur();
}

// An icon picker's button, by the wowhead link its icon carries, e.g. icon(page, 'spell', 2825).
export const icon = (scope: Scope, kind: 'spell' | 'item', id: number) =>
	scope.locator(`.icon-picker-root > a.icon-picker-button[href*="${kind}=${id}"]`);

// A dropdown of icons, like a buff category, by its label.
export const multiIcon = (scope: Scope, label: string) => scope.locator(`.multi-icon-picker-root:has(> label:text-is(${JSON.stringify(label)}))`);

// Icon dropdowns open on hover, and a click on their button toggles them shut again.
export async function openMultiIcon(scope: Scope, label: string) {
	const root = multiIcon(scope, label);
	await root.locator('.dropend > a.icon-picker-button').hover();
	await expect(root.locator('.dropdown-menu')).toBeVisible();
	return root;
}

// A dropdown of icons, like the flask picker, found by one of its options.
export function iconEnum(scope: Scope, kind: 'spell' | 'item', optionId: number) {
	const root = scope.locator(`.icon-enum-picker-root:has(> .dropdown-menu a[href*="${kind}=${optionId}"])`);
	const button = root.locator(':scope > a.icon-picker-button');
	return {
		root,
		button,
		async pick(kind: 'spell' | 'item', id: number) {
			await button.hover();
			await root.locator(`.dropdown-menu a.icon-picker-button[href*="${kind}=${id}"]`).click();
		},
	};
}

export async function isActive(button: Locator): Promise<boolean> {
	return (await button.getAttribute('class'))!.split(' ').includes('active');
}

export async function expectActive(button: Locator, active: boolean) {
	if (active) {
		await expect(button).toHaveClass(/(^|\s)active(\s|$)/);
	} else {
		await expect(button).not.toHaveClass(/(^|\s)active(\s|$)/);
	}
}

// Accepts confirm() prompts, e.g. deleting a saved entry. Playwright dismisses them otherwise.
export function acceptDialogs(page: Page): string[] {
	const messages: string[] = [];
	page.on('dialog', dialog => {
		messages.push(dialog.message());
		dialog.accept();
	});
	return messages;
}

// A saved data manager: its chips and its Save box. `title` is its header, e.g. 'Saved Talents'.
export function savedData(scope: Scope, title: string) {
	const root = scope.locator(`.saved-data-manager-root:has(.content-block-title:text-is(${JSON.stringify(title)}))`);
	return {
		root,
		chip: (name: string) => root.locator(`.saved-data-set-chip:has(> .saved-data-set-name:text-is(${JSON.stringify(name)}))`),
		async save(name: string) {
			await root.locator('.saved-data-save-input').fill(name);
			await root.locator('.saved-data-save-button').click();
		},
	};
}

// Picks an entry of a dropdown picker, through its submenus: pickFromDropdown(root, 'Spells', 'Frostbolt').
export async function pickFromDropdown(root: Locator, ...path: string[]) {
	await root.locator(':scope > .dropdown-picker-button').click();
	let menu = root.locator(':scope > .dropdown-picker-list');
	for (const submenu of path.slice(0, -1)) {
		const toggle = menu.locator(`:scope > li > .dropend > button.dropdown-item:text-is(${JSON.stringify(`${submenu} »`)})`);
		await toggle.hover();
		menu = root.locator(`.dropend:has(> button.dropdown-item:text-is(${JSON.stringify(`${submenu} »`)})) > .dropdown-menu`).first();
	}
	await menu.locator(`:scope > li > button.dropdown-item:text-is(${JSON.stringify(path[path.length - 1])})`).click();
}

// Everything the inputs in scope show, in page order, so two snapshots line up.
export async function snapshot(scope: Locator): Promise<string[]> {
	return scope.evaluate(root =>
		[...root.querySelectorAll('.input-root')]
			// a modal's inputs only count when the scope is that modal
			.filter(input => input.closest('.modal') == root.closest('.modal'))
			.map(input => {
				const own = (selector: string) => [...input.querySelectorAll(selector)].filter(e => e.closest('.input-root') == input);
				const values = own('input, select').map(e => {
					const elem = e as HTMLInputElement;
					return elem.type == 'checkbox' ? String(elem.checked) : elem.value;
				});
				const icons = own('a.icon-picker-button').map(a => `${a.getAttribute('href')}${a.classList.contains('active') ? '*' : ''}`);
				return `${input.querySelector('label')?.textContent ?? ''}: ${values.concat(icons).join(' ')}`;
			}),
	);
}

// Changes every input a player can reach in the scope once, to something it isn't.
export async function changeEverything(scope: Locator) {
	const selects = scope.locator('.enum-picker-root:visible select:enabled');
	for (let i = 0; i < (await selects.count()); i++) {
		const select = selects.nth(i);
		if (!(await select.isVisible())) continue;
		const options = await select.locator('option').evaluateAll(opts => opts.map(o => (o as HTMLOptionElement).value));
		const current = await select.inputValue();
		const other = options.find(o => o != current);
		if (other !== undefined) await select.selectOption(other);
	}

	const checkboxes = scope.locator('.boolean-picker-root:visible input[type=checkbox]:enabled');
	for (let i = 0; i < (await checkboxes.count()); i++) {
		const box = checkboxes.nth(i);
		if (await box.isVisible()) await box.setChecked(!(await box.isChecked()));
	}

	const numbers = scope.locator('.number-picker-root:visible input:enabled');
	for (let i = 0; i < (await numbers.count()); i++) {
		const input = numbers.nth(i);
		if (!(await input.isVisible())) continue;
		const value = Number(await input.inputValue()) || 0;
		await input.fill(String(value + 3));
		await input.blur();
	}

	// Item swaps belong to the gear pickers, which open a modal.
	const icons = scope.locator('.icon-picker-root:visible > a.icon-picker-button');
	for (let i = 0; i < (await icons.count()); i++) {
		const icon = icons.nth(i);
		if ((await icon.isVisible()) && !(await icon.evaluate(e => !!e.closest('.item-swap-picker-root')))) await icon.click();
	}
}
