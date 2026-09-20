import { REPO_NAME } from '../../constants/other';
import { setItemQualityCssClass } from '../../css_utils';
import { IndividualSimUI } from '../../individual_sim_ui';
import { ALL_SOURCE_KINDS, loadCatalog, MAX_CONTENT_PHASE, MIN_CONTENT_PHASE, SOURCE_KINDS } from '../../optimizer/catalog';
import {
	ALL_SLOTS,
	buildOptimizeRequest,
	BuiltRequest,
	defaultTabSettings,
	OptimizerTabSettings,
	tankEncounter,
	tankMetricWeights,
} from '../../optimizer/pool_builder';
import { AsyncAPIResult, OptimizeGearRequest, ProgressMetrics } from '../../proto/api';
import { EquipmentSpec, ItemSpec, Race, Spec, Stat } from '../../proto/common';
import {
	OptimizerEffort,
	OptimizerLoadoutResult,
	OptimizerMetrics,
	OptimizerProgress,
	OptimizerRacialMode,
	OptimizerResult,
	OptimizerSlotAlternative,
	StatMinimum,
} from '../../proto/optimizer';
import { SavedGearSet } from '../../proto/ui';
import { EquippedItem } from '../../proto_utils/equipped_item';
import { Gear } from '../../proto_utils/gear';
import { getClassStatName, raceNames, slotNames } from '../../proto_utils/names';
import { Stats } from '../../proto_utils/stats';
import { isTankSpec } from '../../proto_utils/utils';
import { SimError } from '../../sim';
import { TypedEvent } from '../../typed_event';
import { downloadString } from '../../utils';
import { ContentBlock } from '../content_block';
import { ItemRenderer } from '../gear_picker';
import { SimTab } from '../sim_tab';
import { GearTab } from './gear_tab';

const PHASE_LABELS: Record<number, string> = {
	1: 'P1: Naxxramas, Eye of Eternity, Obsidian Sanctum',
	2: 'P2: Ulduar',
	3: 'P3: Trial of the Crusader',
	4: 'P4: Icecrown Citadel',
	5: 'P5: Ruby Sanctum',
};

const EFFORTS: Array<{ effort: OptimizerEffort; label: string }> = [
	{ effort: OptimizerEffort.OptimizerEffortQuick, label: 'Quick (seconds)' },
	{ effort: OptimizerEffort.OptimizerEffortNormal, label: 'Normal (1 to 2 minutes)' },
	{ effort: OptimizerEffort.OptimizerEffortThorough, label: 'Thorough (5 to 10 minutes)' },
];

const METRIC_LABELS: Array<[keyof OptimizerMetrics, string]> = [
	['dps', 'DPS'],
	['hps', 'HPS'],
	['tps', 'TPS'],
	['dtps', 'DTPS'],
	['tmi', 'TMI'],
	['pDeath', 'Death chance'],
];

const RACIAL_MODES: Array<{ mode: OptimizerRacialMode; label: string }> = [
	{ mode: OptimizerRacialMode.OptimizerRacialSearch, label: 'Search every race' },
	{ mode: OptimizerRacialMode.OptimizerRacialKeepCurrent, label: 'Keep my traits' },
];

// The web server's refusal while another run holds the optimizer (sim/web/main.go).
const BUSY_PATTERN = /Another optimization is already running \(progress id ([^)]+)\)/;

// Under wasm (static hosting, or the server's --wasm), sim_worker.js runs the sim in the browser.
// Otherwise the server swaps in net_worker.js, which calls its async routes.
async function simServerAvailable(): Promise<boolean> {
	try {
		const response = await fetch(`/${REPO_NAME}/sim_worker.js`, { cache: 'no-cache' });
		return response.ok && (await response.text()).includes('/asyncProgress');
	} catch (e) {
		return false;
	}
}

function formatNumber(value: number, digits = 1): string {
	return value.toLocaleString(undefined, { minimumFractionDigits: digits, maximumFractionDigits: digits });
}

function formatDelta(delta: number, se: number, digits = 1): string {
	return `${delta >= 0 ? '+' : ''}${formatNumber(delta, digits)} ± ${formatNumber(se, digits)}`;
}

function newElement<K extends keyof HTMLElementTagNameMap>(tag: K, className?: string, text?: string): HTMLElementTagNameMap[K] {
	const elem = document.createElement(tag);
	if (className) {
		elem.className = className;
	}
	if (text != undefined) {
		elem.textContent = text;
	}
	return elem;
}

function button(label: string, className: string, onClick: () => void): HTMLButtonElement {
	const elem = newElement('button', `btn ${className}`, label);
	elem.addEventListener('click', onClick);
	return elem;
}

// The BiS optimizer for the individual sim's player. It builds the candidate pool here, runs the
// search on the sim's web server and shows what comes back.
export class OptimizerTab extends SimTab {
	readonly simUI: IndividualSimUI<Spec>;
	private readonly gearTab: GearTab;

	private settings: OptimizerTabSettings;
	private readonly leftPanel: HTMLElement;
	private readonly rightPanel: HTMLElement;

	private readonly availabilityElem: HTMLElement;
	private readonly setupBody: HTMLElement;
	private readonly constraintsBody: HTMLElement;
	private readonly statusElem: HTMLElement;
	private readonly resultsBody: HTMLElement;
	private runButton: HTMLButtonElement | null = null;
	private cancelButton: HTMLButtonElement | null = null;

	private running: AbortController | null = null;
	private cancelRequested = false;
	// null until the worker check answers
	private serverAvailable: boolean | null = null;

	constructor(parentElem: HTMLElement, simUI: IndividualSimUI<Spec>, gearTab: GearTab) {
		super(parentElem, simUI, { identifier: 'optimizer-tab', title: 'BiS Optimizer' });
		this.simUI = simUI;
		this.gearTab = gearTab;
		this.settings = defaultTabSettings(MAX_CONTENT_PHASE);

		this.leftPanel = newElement('div', 'optimizer-tab-left tab-panel-left');
		this.rightPanel = newElement('div', 'optimizer-tab-right tab-panel-right');
		this.contentContainer.appendChild(this.leftPanel);
		this.contentContainer.appendChild(this.rightPanel);

		const resultsBlock = new ContentBlock(this.leftPanel, 'optimizer-results', { header: { title: 'Result' } });
		this.resultsBody = resultsBlock.bodyElement;
		this.resultsBody.appendChild(
			newElement(
				'p',
				'optimizer-hint',
				'Finds the best items, gems, enchants, reforges and racial traits for the chosen content phase, from what the server hands ' +
					'out by then. It optimizes your own metrics against the buffs on the Settings tab, over your encounter length. ' +
					'Results follow the sim, so they are only as good as its model of the server.',
			),
		);

		const setupBlock = new ContentBlock(this.rightPanel, 'optimizer-setup', { header: { title: 'Setup' } });
		this.setupBody = setupBlock.bodyElement;
		this.availabilityElem = newElement('div', 'optimizer-availability');
		this.statusElem = newElement('div', 'optimizer-status');
		const constraintsBlock = new ContentBlock(this.rightPanel, 'optimizer-constraints', { header: { title: 'Constraints' } });
		this.constraintsBody = constraintsBlock.bodyElement;

		this.buildTabContent();
		this.simUI.sim.waitForInit().then(() => {
			this.loadSettings();
			this.buildTabContent();
		});
		simServerAvailable().then(available => {
			this.serverAvailable = available;
			this.updateAvailability();
		});
	}

	private getSettingsKey(): string {
		return this.simUI.getStorageKey('optimizer-settings.v1');
	}

	private loadSettings() {
		const defaults = defaultTabSettings(this.simUI.sim.getPhase());
		try {
			const stored = window.localStorage.getItem(this.getSettingsKey());
			const saved = stored ? JSON.parse(stored) : {};
			const numbers = (value: any, fallback: Array<number>) =>
				Array.isArray(value) && value.every(v => typeof v == 'number') ? (value as Array<number>) : fallback;
			this.settings = {
				contentPhase: defaultTabSettings(typeof saved.contentPhase == 'number' ? saved.contentPhase : defaults.contentPhase).contentPhase,
				effort: EFFORTS.some(e => e.effort == saved.effort) ? saved.effort : defaults.effort,
				sources: numbers(saved.sources, defaults.sources).filter(kind => ALL_SOURCE_KINDS.includes(kind)),
				statMinimums: Array.isArray(saved.statMinimums) ? saved.statMinimums.map((floor: any) => StatMinimum.fromJson(floor)) : [],
				lockedSlots: numbers(saved.lockedSlots, []).filter(slot => ALL_SLOTS.includes(slot)),
				excludedItemIds: numbers(saved.excludedItemIds, []),
				tankSurvival: typeof saved.tankSurvival == 'number' ? Math.min(1, Math.max(0, saved.tankSurvival)) : defaults.tankSurvival,
				requireCritImmunity: typeof saved.requireCritImmunity == 'boolean' ? saved.requireCritImmunity : defaults.requireCritImmunity,
				racialMode: RACIAL_MODES.some(m => m.mode == saved.racialMode) ? saved.racialMode : defaults.racialMode,
				metricWeights: saved.metricWeights ? OptimizerMetrics.fromJson(saved.metricWeights) : undefined,
			};
		} catch (e) {
			console.warn('Ignoring saved optimizer settings: ' + e);
			this.settings = defaults;
		}
	}

	private storeSettings() {
		const settings = {
			...this.settings,
			statMinimums: this.settings.statMinimums.map(floor => StatMinimum.toJson(floor)),
			metricWeights: this.settings.metricWeights ? OptimizerMetrics.toJson(this.settings.metricWeights) : undefined,
		};
		window.localStorage.setItem(this.getSettingsKey(), JSON.stringify(settings));
	}

	private changeSettings(change: (settings: OptimizerTabSettings) => void) {
		change(this.settings);
		this.storeSettings();
		this.buildTabContent();
	}

	protected buildTabContent() {
		this.buildSetup();
		this.buildConstraints();
		this.updateAvailability();
	}

	private updateAvailability() {
		const available = this.serverAvailable == true;
		this.availabilityElem.textContent =
			this.serverAvailable == false
				? "The optimizer runs on the sim's web server, and this page runs the sim in your browser (wasm). " +
				  'Start the sim with its web server to use it.'
				: '';
		this.availabilityElem.hidden = this.serverAvailable != false;
		this.rightPanel.querySelectorAll<HTMLInputElement | HTMLSelectElement | HTMLButtonElement>('input, select, button').forEach(elem => {
			elem.disabled = !available || (this.running != null && elem != this.cancelButton);
		});
		if (this.cancelButton) {
			this.cancelButton.hidden = this.running == null;
		}
	}

	private buildSetup() {
		this.setupBody.replaceChildren(this.availabilityElem);

		const phaseSelect = newElement('select', 'form-select');
		for (let phase = MIN_CONTENT_PHASE; phase <= MAX_CONTENT_PHASE; phase++) {
			const option = newElement('option', undefined, PHASE_LABELS[phase]);
			option.value = String(phase);
			option.selected = phase == this.settings.contentPhase;
			phaseSelect.appendChild(option);
		}
		phaseSelect.addEventListener('change', () => this.changeSettings(s => (s.contentPhase = parseInt(phaseSelect.value))));
		this.setupBody.appendChild(this.labeled('Content phase', phaseSelect));

		this.setupBody.appendChild(
			this.labeled('Fight', newElement('div', undefined, this.bossName()), 'Your encounter length and server settings carry over.'),
		);

		const effortSelect = newElement('select', 'form-select');
		for (const { effort, label } of EFFORTS) {
			const option = newElement('option', undefined, label);
			option.value = String(effort);
			option.selected = effort == this.settings.effort;
			effortSelect.appendChild(option);
		}
		effortSelect.addEventListener('change', () => this.changeSettings(s => (s.effort = parseInt(effortSelect.value))));
		this.setupBody.appendChild(this.labeled('Effort', effortSelect));

		this.buildObjective();

		const racialSelect = newElement('select', 'form-select');
		for (const { mode, label } of RACIAL_MODES) {
			const option = newElement('option', undefined, label);
			option.value = String(mode);
			option.selected = mode == this.settings.racialMode;
			racialSelect.appendChild(option);
		}
		racialSelect.addEventListener('change', () => this.changeSettings(s => (s.racialMode = parseInt(racialSelect.value))));
		this.setupBody.appendChild(
			this.labeled('Racial traits', racialSelect, 'A search scores all ten races on your gear, then runs the best few in full.'),
		);

		const sources = newElement('div', 'optimizer-checkbox-grid');
		for (const { kind, label } of SOURCE_KINDS) {
			sources.appendChild(
				this.checkbox(label, this.settings.sources.includes(kind), checked =>
					this.changeSettings(s => (s.sources = checked ? [...s.sources, kind] : s.sources.filter(k => k != kind))),
				),
			);
		}
		this.setupBody.appendChild(this.labeled('Item sources', sources, 'Gems count from every source. PvP gear never does.'));

		const actions = newElement('div', 'optimizer-actions');
		this.runButton = button('Optimize', 'btn-primary', () => this.run());
		this.cancelButton = button('Cancel', 'btn-secondary', () => this.cancel());
		actions.append(
			this.runButton,
			this.cancelButton,
			button('Export request', 'btn-secondary', () => this.exportRequest()),
		);
		this.setupBody.append(actions, this.statusElem);
	}

	private isTank(): boolean {
		return isTankSpec(this.simUI.player.spec);
	}

	// The database only has the preset bosses once the sim has loaded it, and the tab builds itself
	// before that.
	private bossName(): string {
		const plain = 'a plain level 83 boss';
		if (!this.isTank()) {
			return plain;
		}
		try {
			return tankEncounter(undefined, this.settings.contentPhase, this.simUI.sim.db)?.targets[0]?.name || plain;
		} catch (e) {
			return plain;
		}
	}

	// Tanks trade survival against threat on one slider, with the six metric weights under Advanced
	// for anyone who wants them. Everyone else optimizes their own DPS.
	private buildObjective() {
		if (!this.isTank()) {
			return;
		}
		const survival = Math.round(this.settings.tankSurvival * 100);
		const slider = newElement('input', 'form-range');
		slider.type = 'range';
		slider.min = '0';
		slider.max = '100';
		slider.step = '5';
		slider.value = String(survival);
		slider.addEventListener('change', () =>
			this.changeSettings(s => {
				s.tankSurvival = parseInt(slider.value) / 100;
				s.metricWeights = undefined;
			}),
		);
		this.setupBody.appendChild(
			this.labeled(
				`Survival ${survival}% / threat ${100 - survival}%`,
				slider,
				this.settings.metricWeights
					? 'The weights under Advanced are in charge. Move the slider to hand it back.'
					: 'Survival is the damage you take and how spiky it is. Threat is TPS.',
			),
		);
		this.setupBody.appendChild(
			this.labeled(
				'Crit immunity',
				this.checkbox('Never let the boss crit me', this.settings.requireCritImmunity, checked =>
					this.changeSettings(s => (s.requireCritImmunity = checked)),
				),
				"From the sim's own crit formula for this boss, not a flat 540 defense.",
			),
		);
		this.setupBody.appendChild(this.advancedWeights());
	}

	private advancedWeights(): HTMLElement {
		const details = newElement('details', 'optimizer-advanced');
		details.open = this.settings.metricWeights != undefined;
		details.appendChild(newElement('summary', 'form-label', 'Advanced: metric weights'));
		const weights = this.settings.metricWeights ?? tankMetricWeights(this.settings.tankSurvival);
		const grid = newElement('div', 'optimizer-floors');
		for (const [key, label] of METRIC_LABELS) {
			const row = newElement('div', 'optimizer-inline');
			row.appendChild(newElement('span', 'optimizer-slot', label));
			const input = newElement('input', 'form-control form-control-sm');
			input.type = 'number';
			input.min = '0';
			input.step = '0.05';
			input.value = String(weights[key]);
			input.addEventListener('change', () =>
				this.changeSettings(s => {
					const next = OptimizerMetrics.clone(s.metricWeights ?? tankMetricWeights(s.tankSurvival));
					next[key] = Math.max(0, parseFloat(input.value) || 0);
					s.metricWeights = next;
				}),
			);
			row.append(input);
			grid.appendChild(row);
		}
		grid.appendChild(button('Back to the slider', 'btn-secondary btn-sm', () => this.changeSettings(s => (s.metricWeights = undefined))));
		details.appendChild(grid);
		details.appendChild(
			newElement(
				'div',
				'optimizer-hint',
				"Importance, not signs: less DTPS, TMI and death chance always count as better. A metric its reference stat doesn't move measurably is dropped with a warning.",
			),
		);
		return details;
	}

	private buildConstraints() {
		this.constraintsBody.replaceChildren();

		const locks = newElement('div', 'optimizer-checkbox-grid');
		for (const slot of ALL_SLOTS) {
			locks.appendChild(
				this.checkbox(slotNames.get(slot)!, this.settings.lockedSlots.includes(slot), checked =>
					this.changeSettings(s => (s.lockedSlots = checked ? [...s.lockedSlots, slot] : s.lockedSlots.filter(l => l != slot))),
				),
			);
		}
		this.constraintsBody.appendChild(this.labeled('Keep equipped', locks, 'These slots keep your item, gems, enchant and reforge.'));

		const floors = newElement('div', 'optimizer-floors');
		this.settings.statMinimums.forEach((floor, i) => floors.appendChild(this.floorRow(floor, i)));
		floors.appendChild(
			button('Add a floor', 'btn-secondary btn-sm', () =>
				this.changeSettings(s => s.statMinimums.push(StatMinimum.create({ stat: this.floorStats()[0], minValue: 0 }))),
			),
		);
		this.constraintsBody.appendChild(this.labeled('Stat floors', floors, 'Minimum final stats, as the character sheet shows them.'));

		const excludes = newElement('div', 'optimizer-excludes');
		for (const id of this.settings.excludedItemIds) {
			const chip = newElement('span', 'optimizer-exclude badge rounded-pill', `${this.itemName(id)} `);
			chip.appendChild(
				button('×', 'btn-link btn-sm', () => this.changeSettings(s => (s.excludedItemIds = s.excludedItemIds.filter(e => e != id)))),
			);
			excludes.appendChild(chip);
		}
		const idInput = newElement('input', 'form-control form-control-sm');
		idInput.type = 'number';
		idInput.placeholder = 'Item or gem ID';
		const add = () => {
			const id = parseInt(idInput.value);
			if (id > 0 && !this.settings.excludedItemIds.includes(id)) {
				this.changeSettings(s => s.excludedItemIds.push(id));
			}
		};
		idInput.addEventListener('keyup', event => event.key == 'Enter' && add());
		const addRow = newElement('div', 'optimizer-inline');
		addRow.append(idInput, button('Exclude', 'btn-secondary btn-sm', add));
		excludes.appendChild(addRow);
		this.constraintsBody.appendChild(this.labeled('Excluded items', excludes, 'Never picked. Results have an Exclude button on each item.'));
	}

	private floorStats(): Array<Stat> {
		return this.simUI.individualConfig.displayStats;
	}

	private floorRow(floor: StatMinimum, index: number): HTMLElement {
		const row = newElement('div', 'optimizer-inline');
		const statSelect = newElement('select', 'form-select form-select-sm');
		const stats = this.floorStats().includes(floor.stat) ? this.floorStats() : [floor.stat, ...this.floorStats()];
		for (const stat of stats) {
			const option = newElement('option', undefined, getClassStatName(stat, this.simUI.player.getClass()));
			option.value = String(stat);
			option.selected = stat == floor.stat;
			statSelect.appendChild(option);
		}
		statSelect.addEventListener('change', () => this.changeSettings(s => (s.statMinimums[index].stat = parseInt(statSelect.value))));
		const valueInput = newElement('input', 'form-control form-control-sm');
		valueInput.type = 'number';
		valueInput.value = String(floor.minValue);
		valueInput.addEventListener('change', () => this.changeSettings(s => (s.statMinimums[index].minValue = parseFloat(valueInput.value) || 0)));
		row.append(
			statSelect,
			valueInput,
			button('×', 'btn-link btn-sm', () => this.changeSettings(s => s.statMinimums.splice(index, 1))),
		);
		return row;
	}

	private labeled(label: string, content: HTMLElement, hint?: string): HTMLElement {
		const group = newElement('div', 'optimizer-field');
		group.appendChild(newElement('label', 'form-label', label));
		group.appendChild(content);
		if (hint) {
			group.appendChild(newElement('div', 'optimizer-hint', hint));
		}
		return group;
	}

	private checkbox(label: string, checked: boolean, onChange: (checked: boolean) => void): HTMLElement {
		const wrapper = newElement('label', 'form-check');
		const input = newElement('input', 'form-check-input');
		input.type = 'checkbox';
		input.checked = checked;
		input.addEventListener('change', () => onChange(input.checked));
		wrapper.append(input, newElement('span', 'form-check-label', label));
		return wrapper;
	}

	private itemName(id: number): string {
		const item = this.simUI.sim.db.lookupItemSpec(ItemSpec.create({ id }));
		return item?.item.name || this.simUI.sim.db.lookupGem(id)?.name || `Item ${id}`;
	}

	private async buildRequest(): Promise<BuiltRequest> {
		const player = this.simUI.player;
		const catalog = await loadCatalog();
		return buildOptimizeRequest({
			base: this.simUI.sim.makeRaidSimRequest(false),
			targetRaidIndex: player.getRaidIndex(),
			settings: this.settings,
			catalog,
			filters: this.simUI.sim.getFilters(),
			getItems: slot => player.getItems(slot),
			getEnchants: slot => player.getEnchants(slot),
			db: this.simUI.sim.db,
		});
	}

	private async exportRequest() {
		try {
			const { request } = await this.buildRequest();
			downloadString(OptimizeGearRequest.toJsonString(request), `optimizer-request-p${this.settings.contentPhase}.json`);
		} catch (e) {
			this.showStatus(`Couldn't build the request: ${e}`, 'error');
		}
	}

	private cancel() {
		this.cancelRequested = true;
		this.running?.abort();
	}

	private async run() {
		if (this.running) {
			return;
		}
		const controller = new AbortController();
		this.running = controller;
		this.cancelRequested = false;
		this.updateAvailability();
		this.showStatus('Building the candidate pool…');

		try {
			const built = await this.buildRequest();
			const result = await this.simUI.sim.runGearOptimizer(built.request, progress => this.showProgress(progress), controller.signal);
			this.showResult(result, built);
			if (!result.cancelled) {
				this.showStatus('');
			} else if (this.cancelRequested) {
				this.showStatus('Cancelled. Below is the best set verified before it stopped.', 'warning', true);
			} else {
				this.showStatus(
					'The server stopped this run: nobody asked for its progress for 2 minutes (a background tab can do that), ' +
						'or it was cancelled from another page. Below is the best set verified before it stopped.',
					'warning',
					true,
				);
			}
		} catch (e) {
			this.showRunError(e);
		} finally {
			this.running = null;
			this.updateAvailability();
		}
	}

	private showRunError(e: any) {
		const message = e instanceof SimError ? e.errorStr : String(e);
		const busy = BUSY_PATTERN.exec(message);
		if (!busy) {
			this.showStatus(`The optimizer failed: ${message}`, 'error', true);
			return;
		}
		const progressId = busy[1];
		this.showStatus('Another optimization is running on the server. Wait for it to finish, or cancel it.', 'warning', true);
		const cancelOther = button('Cancel that run', 'btn-secondary btn-sm', async () => {
			cancelOther.disabled = true;
			try {
				const response = await fetch('/cancelAsync', {
					method: 'POST',
					headers: { 'Content-Type': 'application/x-protobuf' },
					body: AsyncAPIResult.toBinary(AsyncAPIResult.create({ progressId })),
				});
				cancelOther.textContent = response.ok ? 'Cancel sent' : 'It already finished';
			} catch (err) {
				cancelOther.textContent = `Cancel failed: ${err}`;
			}
		});
		this.statusElem.querySelector('.optimizer-status-actions')?.prepend(cancelOther);
	}

	private showStatus(text: string, kind: 'info' | 'warning' | 'error' = 'info', retry = false) {
		this.statusElem.replaceChildren();
		this.statusElem.className = `optimizer-status optimizer-status-${kind}`;
		if (!text) {
			return;
		}
		this.statusElem.appendChild(newElement('div', undefined, text));
		const actions = newElement('div', 'optimizer-status-actions');
		if (retry) {
			actions.appendChild(button('Retry', 'btn-secondary btn-sm', () => this.run()));
		}
		this.statusElem.appendChild(actions);
	}

	private showProgress(metrics: ProgressMetrics) {
		const progress: OptimizerProgress | undefined = metrics.optimizerProgress;
		if (!progress) {
			return;
		}
		const parts = [progress.stage || 'Running'];
		if (progress.totalSteps > 0) {
			parts.push(`step ${progress.completedSteps} of ${progress.totalSteps}`);
		}
		if (progress.totalSims > 0) {
			parts.push(`${progress.completedSims} of about ${progress.totalSims} sims`);
		}
		if (progress.racialTraits) {
			parts.push(`${raceNames.get(progress.racialTraits)} traits`);
		}
		if (progress.bestScoreDelta) {
			parts.push(`best so far ${progress.bestScoreDelta >= 0 ? '+' : ''}${formatNumber(progress.bestScoreDelta)}`);
		}
		if (progress.elapsedSeconds) {
			parts.push(`${Math.round(progress.elapsedSeconds)} s`);
		}
		this.showStatus(parts.join(', '));
	}

	private showResult(result: OptimizerResult, built: BuiltRequest) {
		this.resultsBody.replaceChildren();
		const phase = result.settings?.contentPhase || built.request.settings!.contentPhase;

		const provenance: Array<string> = [PHASE_LABELS[phase] || `P${phase}`];
		if (result.settings) {
			provenance.push(EFFORTS.find(e => e.effort == result.settings!.effort)?.label.split(' ')[0] || '');
		}
		if (result.simCommit) {
			// short hash, keeping a -dirty suffix
			provenance.push(`sim ${result.simCommit.replace(/^([0-9a-f]{12})[0-9a-f]+/, '$1')}`);
		}
		if (result.catalogDate) {
			provenance.push(`catalog ${result.catalogDate}`);
		}
		provenance.push(built.bossName);
		if (result.totalSims) {
			provenance.push(`${result.totalSims} sims`);
		}
		if (result.elapsedSeconds) {
			provenance.push(`${formatNumber(result.elapsedSeconds)} s`);
		}
		this.resultsBody.appendChild(newElement('div', 'optimizer-provenance', provenance.filter(p => p).join(' · ')));

		const warnings = [...result.warnings];
		if (result.calibrationGap) {
			const gap = `Full raid vs the derived buffs differ by ${formatNumber(result.calibrationGap * 100)}%.`;
			warnings.push(Math.abs(result.calibrationGap) > 0.03 ? `${gap} That's over 3%, so treat raid numbers with care.` : gap);
		}
		warnings.forEach(w => this.resultsBody.appendChild(newElement('div', 'optimizer-warning', w)));
		if (result.improved) {
			// improved is also set when the starting gear broke a rule, where the pick can score lower
			const beatsSeed = (result.best?.scoreDelta || 0) > 0;
			this.resultsBody.appendChild(
				newElement(
					'div',
					'optimizer-improved',
					beatsSeed ? 'Beats your starting gear by more than the noise.' : 'Replaces your starting gear, which breaks a rule (see above).',
				),
			);
		}

		if (result.best) {
			this.resultsBody.appendChild(this.renderActions(result, phase));
			this.resultsBody.appendChild(this.renderLoadout('Best', result.best, result.seed));
		}
		if (result.alternatives.length > 0) {
			this.resultsBody.appendChild(this.renderAlternatives(result.alternatives, result.best));
		}
		if (result.top.length > 0) {
			this.resultsBody.appendChild(this.renderTop(result.top, result.best));
		}
		if (result.racialScreen.length > 0) {
			const table = newElement('table', 'table table-sm optimizer-table');
			table.appendChild(this.row('th', ['Racial traits', 'Score', 'Finalist']));
			for (const screen of result.racialScreen) {
				table.appendChild(
					this.row('td', [raceNames.get(screen.racialTraits) || '', `${formatNumber(screen.score)} ± ${formatNumber(screen.scoreSe)}`, screen.finalist ? 'yes' : '']),
				);
			}
			this.resultsBody.appendChild(this.section('Racial screen', table));
		}
		if (built.seedChanges.length > 0) {
			const list = newElement('ul', 'optimizer-list');
			built.seedChanges.forEach(change => list.appendChild(newElement('li', undefined, change)));
			this.resultsBody.appendChild(
				this.section('Starting gear, trimmed to the pool', list, 'The search started from your gear without these: the phase or your settings rule them out.'),
			);
		}
	}

	private renderActions(result: OptimizerResult, phase: number): HTMLElement {
		const best = result.best!;
		const actions = newElement('div', 'optimizer-actions');
		const note = newElement('span', 'optimizer-hint');
		const name = `P${phase} BiS (optimizer)`;
		actions.append(
			button('Equip', 'btn-primary', () => this.equip(best)),
			button(`Save as "${name}"`, 'btn-secondary', () => {
				this.saveGearSet(name, best.equipment || EquipmentSpec.create());
				note.textContent = "Saved under the Gear tab's Gear Sets.";
			}),
			button('Export JSON', 'btn-secondary', () =>
				downloadString(OptimizerResult.toJsonString(result, { prettySpaces: 2 }), `optimizer-result-p${phase}.json`),
			),
			note,
		);
		return actions;
	}

	private equip(best: OptimizerLoadoutResult) {
		const player = this.simUI.player;
		const eventID = TypedEvent.nextEventID();
		TypedEvent.freezeAllAndDo(() => {
			player.setGear(eventID, this.simUI.sim.db.lookupEquipmentSpec(best.equipment || EquipmentSpec.create()));
			if (best.racialTraits != Race.RaceUnknown && best.racialTraits != player.getEffectiveRacialTraits()) {
				player.setRacialTraits(eventID, best.racialTraits == player.getRace() ? Race.RaceUnknown : best.racialTraits);
			}
		});
		// simHeader.activateTab clicks the <li>, which Bootstrap's tab handler ignores
		this.gearTab.navLink.click();
	}

	// current bonus stats, which the run simmed with; the set then shows as active after Equip
	private saveGearSet(name: string, equipment: EquipmentSpec) {
		this.gearTab.savedGearManager.saveUserEntry(
			name,
			SavedGearSet.create({
				gear: this.simUI.sim.db.lookupEquipmentSpec(equipment).asSpec(),
				bonusStatsStats: this.simUI.player.getBonusStats().toProto(),
			}),
		);
	}

	private renderLoadout(title: string, loadout: OptimizerLoadoutResult, seed: OptimizerLoadoutResult | undefined): HTMLElement {
		const body = newElement('div', 'optimizer-loadout');

		const summary: Array<string> = [];
		if (loadout.score) {
			summary.push(`Score ${formatNumber(loadout.score, 2)}`);
		}
		if (loadout.scoreDelta || loadout.scoreDeltaSe) {
			summary.push(`Δ ${formatDelta(loadout.scoreDelta, loadout.scoreDeltaSe, 2)} over your gear`);
		}
		if (loadout.raidDpsDelta || loadout.raidDpsDeltaSe) {
			summary.push(`raid DPS ${formatDelta(loadout.raidDpsDelta, loadout.raidDpsDeltaSe)}`);
		}
		if (loadout.racialTraits) {
			const changed = seed && seed.racialTraits != loadout.racialTraits ? ', changed' : '';
			summary.push(`${raceNames.get(loadout.racialTraits)} racial traits${changed}`);
		}
		if (summary.length > 0) {
			body.appendChild(newElement('div', 'optimizer-summary', summary.join(', ')));
		}
		const metrics = this.metricsText(loadout.metrics, seed?.metrics);
		if (metrics) {
			body.appendChild(newElement('div', 'optimizer-metrics', metrics));
		}
		loadout.warnings.forEach(w => body.appendChild(newElement('div', 'optimizer-warning', w)));
		if (loadout.unmodeledEffectItemIds.length > 0) {
			body.appendChild(
				newElement(
					'div',
					'optimizer-warning',
					`The sim doesn't model the effects of: ${loadout.unmodeledEffectItemIds.map(id => this.itemName(id)).join(', ')}.`,
				),
			);
		}

		const gear = newElement('div', 'optimizer-gear');
		const seedItems = seed?.equipment?.items || [];
		const loadoutGear = this.simUI.sim.db.lookupEquipmentSpec(loadout.equipment || EquipmentSpec.create());
		(loadout.equipment?.items || []).forEach((spec, slot) => {
			const equipped = spec.id ? this.simUI.sim.db.lookupItemSpec(spec) : null;
			if (!equipped) {
				return;
			}
			const changed = seed != undefined && !ItemSpec.equals(spec, seedItems[slot] || ItemSpec.create());
			const row = newElement('div', `optimizer-gear-row${changed ? ' optimizer-changed' : ''}`);
			row.appendChild(newElement('span', 'optimizer-slot', slotNames.get(slot) || ''));
			new ItemRenderer(row, newElement('div'), this.simUI.player).update(equipped, loadoutGear);
			row.appendChild(this.excludeButton(spec.id));
			gear.appendChild(row);
		});
		body.appendChild(gear);

		const sheet = this.renderSheet(loadout);
		if (sheet) {
			body.appendChild(sheet);
		}
		return this.section(title, body);
	}

	private metricsText(metrics: OptimizerMetrics | undefined, seed: OptimizerMetrics | undefined): string {
		if (!metrics) {
			return '';
		}
		return METRIC_LABELS.filter(([key]) => metrics[key])
			.map(([key, label]) => {
				const value = key == 'pDeath' ? `${formatNumber(metrics[key] * 100)}%` : formatNumber(metrics[key]);
				return seed?.[key] ? `${label} ${value} (was ${key == 'pDeath' ? `${formatNumber(seed[key] * 100)}%` : formatNumber(seed[key])})` : `${label} ${value}`;
			})
			.join(', ');
	}

	private renderSheet(loadout: OptimizerLoadoutResult): HTMLElement | null {
		const finalStats = loadout.finalStats ? Stats.fromProto(loadout.finalStats) : null;
		// a tank's 0% is the crit-immunity answer, so it gets the row too
		const showCrit = loadout.meleeCritTakenChance > 0 || this.isTank();
		if (!finalStats && loadout.caps.length == 0 && !showCrit) {
			return null;
		}
		const playerClass = this.simUI.player.getClass();
		const table = newElement('table', 'table table-sm optimizer-table');
		const capped = new Set(loadout.caps.map(cap => cap.stat));
		if (finalStats) {
			for (const stat of this.simUI.individualConfig.displayStats.filter(s => !capped.has(s))) {
				table.appendChild(this.row('td', [getClassStatName(stat, playerClass), formatNumber(finalStats.getStat(stat), 0), '']));
			}
		}
		for (const cap of loadout.caps) {
			table.appendChild(
				this.row('td', [getClassStatName(cap.stat, playerClass), formatNumber(cap.value, 0), `cap ${formatNumber(cap.cap, 0)}`]),
			);
		}
		if (showCrit) {
			table.appendChild(this.row('td', ['Boss melee crit chance on you', `${formatNumber(loadout.meleeCritTakenChance * 100, 2)}%`, '']));
		}
		return this.section('Character sheet', table);
	}

	private renderAlternatives(alternatives: Array<OptimizerSlotAlternative>, best: OptimizerLoadoutResult | undefined): HTMLElement {
		const table = newElement('table', 'table table-sm optimizer-table');
		const raid = alternatives.some(a => a.raidDpsDelta || a.raidDpsDeltaSe);
		table.appendChild(this.row('th', ['Slot', 'Instead', 'Score vs best', ...(raid ? ['Raid DPS'] : []), '']));
		const bestGear = best?.equipment ? this.simUI.sim.db.lookupEquipmentSpec(best.equipment) : null;
		for (const alt of alternatives) {
			const tr = newElement('tr');
			tr.appendChild(newElement('td', undefined, slotNames.get(alt.slot) || ''));

			const itemCell = newElement('td');
			const equipped = alt.item ? this.simUI.sim.db.lookupItemSpec(alt.item) : null;
			if (equipped) {
				// the set this runner-up would make, so its tooltip counts the right set pieces
				const gear = bestGear?.withEquippedItem(alt.slot, equipped, this.simUI.player.canDualWield2H());
				itemCell.appendChild(this.itemLink(equipped, gear));
			} else if (alt.item) {
				itemCell.textContent = this.itemName(alt.item.id);
			}
			tr.appendChild(itemCell);

			tr.appendChild(newElement('td', undefined, formatDelta(alt.scoreDelta, alt.scoreDeltaSe, 2)));
			if (raid) {
				tr.appendChild(newElement('td', undefined, formatDelta(alt.raidDpsDelta, alt.raidDpsDeltaSe)));
			}
			const cell = newElement('td');
			if (alt.item?.id) {
				cell.appendChild(this.excludeButton(alt.item.id));
			}
			tr.appendChild(cell);
			table.appendChild(tr);
		}
		return this.section('Runners-up per slot', table, 'Each swaps one slot of the best set.');
	}

	// The icon and hover tooltip the gear tab gives an item, sized for a table row.
	private itemLink(equipped: EquippedItem, gear: Gear | undefined): HTMLElement {
		const wrapper = newElement('span', 'optimizer-item');
		const icon = newElement('a', 'optimizer-item-icon');
		const name = newElement('a', 'optimizer-item-name', equipped.item.name);
		setItemQualityCssClass(name, equipped.item.quality);
		if (equipped.item.heroic) {
			name.appendChild(newElement('span', 'heroic-label', '[H]'));
		}
		this.simUI.player.setWowheadData(equipped, icon, gear);
		this.simUI.player.setWowheadData(equipped, name, gear);
		equipped
			.asActionId()
			.fill()
			.then(filled => {
				filled.setBackgroundAndHref(icon);
				filled.setWowheadHref(name);
			});
		wrapper.append(icon, name);
		return wrapper;
	}

	private renderTop(top: Array<OptimizerLoadoutResult>, best: OptimizerLoadoutResult | undefined): HTMLElement {
		const list = newElement('ol', 'optimizer-list');
		const bestItems = best?.equipment?.items || [];
		for (const loadout of top) {
			const differs = (loadout.equipment?.items || [])
				.map((spec, slot) => (ItemSpec.equals(spec, bestItems[slot] || ItemSpec.create()) ? '' : `${slotNames.get(slot)}: ${spec.id ? this.itemName(spec.id) : 'empty'}`))
				.filter(d => d);
			const parts: Array<string> = [];
			if (loadout.scoreDelta || loadout.scoreDeltaSe) {
				parts.push(`score ${formatDelta(loadout.scoreDelta, loadout.scoreDeltaSe, 2)} over your gear`);
			}
			parts.push(differs.length > 0 ? differs.join(', ') : 'the best set');
			const li = newElement('li', undefined, parts.join('; ') + ' ');
			li.appendChild(button('Equip', 'btn-link btn-sm', () => this.equip(loadout)));
			list.appendChild(li);
		}
		return this.section('Top sets', list);
	}

	private excludeButton(id: number): HTMLButtonElement {
		const excludeButton = button('Exclude', 'btn-link btn-sm optimizer-exclude-button', () => {
			if (!this.settings.excludedItemIds.includes(id)) {
				this.changeSettings(s => s.excludedItemIds.push(id));
			}
			excludeButton.disabled = true;
		});
		excludeButton.title = 'Never pick this item in later runs';
		return excludeButton;
	}

	private section(title: string, content: HTMLElement, hint?: string): HTMLElement {
		const section = newElement('div', 'optimizer-section');
		section.appendChild(newElement('h6', 'optimizer-section-title', title));
		if (hint) {
			section.appendChild(newElement('div', 'optimizer-hint', hint));
		}
		section.appendChild(content);
		return section;
	}

	private row(cell: 'td' | 'th', values: Array<string>): HTMLTableRowElement {
		const tr = newElement('tr');
		values.forEach(value => tr.appendChild(newElement(cell, undefined, value)));
		return tr;
	}
}
