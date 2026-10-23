import * as Mechanics from '../../constants/mechanics';
import { IndividualSimUI } from '../../individual_sim_ui';
import { MAX_CONTENT_PHASE, MIN_CONTENT_PHASE } from '../../optimizer/catalog';
import { ProgressMetrics, Raid as RaidProto, StatCap, StatCapHand, StatCapKind, StatCapSource } from '../../proto/api';
import { Encounter as EncounterProto, HandType, ItemSlot, Spec, Stat } from '../../proto/common';
import { slotNames, statNames } from '../../proto_utils/names';
import { reforgeLabel, Reforging, reforgingFor } from '../../proto_utils/reforging';
import { isHealingSpec, isTankSpec } from '../../proto_utils/utils';
import {
	BuildChoice,
	chooseBuild,
	livePriority,
	Metric,
	phaseOf,
	priority,
	PriorityRow,
	ReforgeSlot,
	ReforgeSuggestion,
	suggestReforges,
} from '../../stat_priority';
import { LiveValues, liveValues, loadStatValues, ShippedPhase, StatValues } from '../../stat_values';
import { TypedEvent } from '../../typed_event';
import { getEnumValues } from '../../utils';
import { ContentBlock } from '../content_block';
import { SimTab } from '../sim_tab';
import { AcoreReforgeDialog } from './acore_reforge_dialog';
import { button, formatNumber, newElement, PHASE_LABELS, simServerAvailable, tableRow } from './optimizer_tab';

// long enough to fold a burst of changes (a gear set swap fires several) into one request
const REFRESH_DELAY_MS = 300;

const HAND_NAMES: Record<StatCapHand, string> = {
	[StatCapHand.StatCapHandBoth]: '',
	[StatCapHand.StatCapHandMain]: 'main hand',
	[StatCapHand.StatCapHandOff]: 'off hand',
};

const KIND_TAGS: Record<StatCapKind, string> = {
	[StatCapKind.StatCapFloor]: '',
	[StatCapKind.StatCapCeiling]: 'ceiling',
	[StatCapKind.StatCapSoft]: 'soft',
};

const METRICS: Record<Metric, { label: string; short: string; unit: string; digits: number }> = {
	dps: { label: 'DPS', short: 'DPS', unit: 'DPS per point', digits: 2 },
	tps: { label: 'Threat (TPS)', short: 'TPS', unit: 'TPS per point', digits: 2 },
	tmi: { label: 'Survival (TMI)', short: 'TMI', unit: 'TMI drop per point', digits: 4 },
	hps: { label: 'Healing (HPS)', short: 'HPS', unit: 'HPS per point', digits: 2 },
};
const TANK_METRICS: Array<Metric> = ['tmi', 'tps'];

type Source = 'shipped' | 'live';

// A live run and the settings it ran on: once they change, its values are stale.
interface LiveRun {
	values: LiveValues;
	raid: RaidProto;
	encounter: EncounterProto;
	iterations: number;
}

const REFERENCE_NAMES: Partial<Record<Stat, string>> = {
	[Stat.StatAttackPower]: 'AP',
	[Stat.StatSpellPower]: 'SP',
	[Stat.StatRangedAttackPower]: 'RAP',
	[Stat.StatArmor]: 'armor',
};

// What the tab keeps per spec page. Without a phase it follows the sim's.
interface StoredSettings {
	phase?: number;
	metric?: Metric;
}

export function capLabel(cap: StatCap): string {
	const hand = HAND_NAMES[cap.hand];
	return hand ? `${cap.label}, ${hand}` : cap.label;
}

// Expertise also reads as points, the way the in-game sheet shows it.
export function formatRating(cap: StatCap, value: number): string {
	if (!Number.isFinite(value)) {
		return '—';
	}
	const rating = formatNumber(Math.round(value), 0);
	if (cap.stat == Stat.StatExpertise) {
		return `${rating} · ${formatNumber(value / Mechanics.EXPERTISE_PER_QUARTER_PERCENT_REDUCTION, 1)} exp`;
	}
	return rating;
}

// How far the gear is from the cap, and the CSS class saying whether that's a problem: short of a
// floor or past a ceiling is, a surplus over a floor is rating that could go elsewhere.
export function capStanding(cap: StatCap): { text: string; className: string } {
	if (!Number.isFinite(cap.needed)) {
		return { text: 'out of reach', className: 'caps-unreachable' };
	}
	const diff = Math.round(cap.rating) - Math.round(cap.needed);
	if (diff == 0) {
		return { text: 'at the cap', className: 'caps-at' };
	}
	const amount = formatNumber(Math.abs(diff), 0);
	if (cap.kind == StatCapKind.StatCapCeiling) {
		return diff > 0 ? { text: `${amount} over`, className: 'caps-wasted' } : { text: `${amount} under`, className: 'caps-room' };
	}
	if (diff > 0) {
		return { text: `${amount} over`, className: 'caps-over' };
	}
	return { text: `${amount} short`, className: cap.kind == StatCapKind.StatCapSoft ? 'caps-soft-short' : 'caps-short' };
}

function signed(value: number, digits: number): string {
	return `${value < 0 ? '−' : ''}${formatNumber(Math.abs(value), digits)}`;
}

// a tmi gain is TMI going down
function formatGain(metric: Metric, gain: number): string {
	const { short, digits } = METRICS[metric];
	return `${metric == 'tmi' ? '−' : '+'}${formatNumber(gain, digits)} ${short}`;
}

function capKey(cap: StatCap): string {
	return `${cap.label}|${cap.hand}`;
}

function clampPhase(phase: number): number {
	return Math.min(MAX_CONTENT_PHASE, Math.max(MIN_CONTENT_PHASE, phase));
}

export class CapsTab extends SimTab {
	readonly simUI: IndividualSimUI<Spec>;

	private readonly capsStatus: HTMLElement;
	private readonly capsTable: HTMLTableElement;
	private caps: Array<StatCap> = [];
	// the caps answer the latest request for the player as they are now
	private capsFresh = false;
	private capsError = '';
	// rows the user opened, kept across refreshes
	private readonly expanded = new Set<string>();

	// a reply to anything but the latest request is dropped
	private requestId = 0;
	private stale = true;
	private refreshTimer: number | null = null;

	private readonly priorityStatus: HTMLElement;
	private readonly prioritySource: HTMLElement;
	private readonly priorityTable: HTMLTableElement;
	private readonly phaseSelect: HTMLSelectElement;
	private readonly metricButtons = new Map<Metric, HTMLButtonElement>();
	private readonly shipsValues: boolean;
	private values: StatValues | null = null;
	private valuesError = '';
	private valuesRequested = false;
	// null follows the sim's phase
	private pickedPhase: number | null = null;
	private metric: Metric;

	private readonly sourceGroup: HTMLElement;
	private readonly sourceButtons = new Map<Source, HTMLButtonElement>();
	private readonly measureButton: HTMLButtonElement;
	private readonly measureStatus: HTMLElement;
	private live: LiveRun | null = null;
	private liveStale = false;
	private measuring = false;
	// healers only ever have live values
	private source: Source = 'shipped';

	private readonly reforgeStatus: HTMLElement;
	private readonly reforgeControls: HTMLElement;
	private readonly reforgeTable: HTMLTableElement;
	private readonly applyAllButton: HTMLButtonElement;
	private suggestions: Array<ReforgeSuggestion> = [];

	constructor(parentElem: HTMLElement, simUI: IndividualSimUI<Spec>) {
		super(parentElem, simUI, { identifier: 'caps-tab', title: 'Caps & Priority' });
		this.simUI = simUI;

		const leftPanel = newElement('div', 'caps-tab-left tab-panel-left');
		const rightPanel = newElement('div', 'caps-tab-right tab-panel-right');
		this.contentContainer.append(leftPanel, rightPanel);
		const topRow = newElement('div', 'caps-tab-top');
		leftPanel.appendChild(topRow);

		const capsBlock = new ContentBlock(topRow, 'caps-tab-caps', { header: { title: 'Caps' } });
		capsBlock.bodyElement.appendChild(
			newElement(
				'p',
				'caps-hint',
				'Rating from your gear, gems, enchants, food, elixirs and bonus stats, against what your race, talents, buffs ' +
					"and the target's debuffs leave for gear to cover. Click a row to see where its cap comes from.",
			),
		);
		this.capsStatus = newElement('div', 'caps-status');
		this.capsTable = newElement('table', 'table table-sm caps-table');
		capsBlock.bodyElement.append(this.capsStatus, this.capsTable);

		const spec = simUI.player.spec;
		const stored = this.loadSettings();
		this.pickedPhase = stored.phase ?? null;
		this.shipsValues = !isHealingSpec(spec);
		if (!this.shipsValues) {
			this.metric = 'hps';
		} else {
			this.metric = isTankSpec(spec) ? (stored.metric && TANK_METRICS.includes(stored.metric) ? stored.metric : 'tmi') : 'dps';
		}

		const priorityBlock = new ContentBlock(topRow, 'caps-tab-priority', { header: { title: 'Stat priority' } });
		this.phaseSelect = newElement('select', 'form-select priority-phase');
		this.sourceGroup = newElement('div', 'btn-group priority-source-switch');
		this.priorityStatus = newElement('div', 'priority-status');
		this.prioritySource = newElement('p', 'caps-hint priority-source');
		this.priorityTable = newElement('table', 'table table-sm priority-table');
		this.measureButton = button('Measure at my gear', 'btn-outline-primary priority-measure', () => this.measure());
		this.measureStatus = newElement('span', 'priority-measure-status');
		const measureRow = newElement('div', 'priority-measure-row');
		measureRow.append(this.measureButton, this.measureStatus);
		if (!this.shipsValues) {
			priorityBlock.bodyElement.append(
				newElement('p', 'priority-healer', 'Healers have no shipped stat values: measure yours at your gear.'),
				measureRow,
				this.prioritySource,
				this.priorityTable,
			);
		} else {
			priorityBlock.bodyElement.append(this.priorityControls(spec), measureRow, this.prioritySource, this.priorityStatus, this.priorityTable);
		}

		const reforgeBlock = new ContentBlock(leftPanel, 'caps-tab-reforge', { header: { title: 'Reforge suggestions' } });
		this.reforgeStatus = newElement('p', 'reforge-status');
		this.reforgeControls = newElement('div', 'reforge-controls');
		this.reforgeTable = newElement('table', 'table table-sm reforge-table');
		this.applyAllButton = button('Apply all', 'btn-primary reforge-apply-all', () =>
			this.applySuggestions(this.suggestions.filter(s => s.suggested !== undefined)),
		);
		this.reforgeControls.appendChild(this.applyAllButton);
		reforgeBlock.bodyElement.append(
			newElement(
				'p',
				'caps-hint',
				"Scored with the stat priority's values. Rating past a cap counts for nothing, and each pick uses up the cap room " +
					'or surplus before the next one is scored.',
			),
			newElement('p', 'caps-hint reforge-floor-hint', 'Suggestions keep every hit and expertise cap you meet.'),
			this.reforgeStatus,
			this.reforgeControls,
			this.reforgeTable,
		);

		const rosterBlock = new ContentBlock(rightPanel, 'caps-tab-roster', { header: { title: 'Your character' } });
		rosterBlock.bodyElement.append(
			newElement('p', 'caps-hint', "Loads a raider from the server: gear with gems, enchants and reforges, talents, glyphs and racials."),
			button('Import from roster…', 'btn-outline-primary caps-roster-import', () => this.simUI.openAcoreImporter()),
			button('Apply reforges on server…', 'btn-outline-primary ms-2 caps-server-reforge', () => new AcoreReforgeDialog(this.simUI.rootElem, this.simUI)),
		);
		rosterBlock.rootElem.hidden = true;
		simServerAvailable().then(available => (rosterBlock.rootElem.hidden = !available));

		this.buildTabContent();

		const player = simUI.player;
		const raid = simUI.sim.raid;
		TypedEvent.onAny([
			player.currentStatsEmitter,
			// right away, so a suggestion can't be applied to gear it wasn't worked out for
			player.gearChangeEmitter,
			player.buffsChangeEmitter,
			player.talentsChangeEmitter,
			player.inFrontOfTargetChangeEmitter,
			...(player.getParty() ? [player.getParty()!.buffsChangeEmitter] : []),
			raid.buffsChangeEmitter,
			raid.debuffsChangeEmitter,
			raid.tanksChangeEmitter,
			simUI.sim.encounter.changeEmitter,
		]).on(() => this.markStale());
		simUI.sim.phaseChangeEmitter.on(() => {
			if (this.pickedPhase == null) {
				this.renderValues();
			}
		});
		this.navLink.addEventListener('shown.bs.tab', () => {
			this.requestValues();
			if (this.stale) {
				this.scheduleRefresh();
			}
		});
	}

	protected buildTabContent() {
		this.renderCaps();
		this.renderValues();
	}

	private isShown(): boolean {
		return this.rootElem.classList.contains('active');
	}

	private markStale() {
		this.stale = true;
		this.capsFresh = false;
		if (this.live) {
			const liveStale = this.isLiveStale();
			if (liveStale != this.liveStale) {
				this.liveStale = liveStale;
				this.renderPriority();
			}
		}
		this.renderSuggestions();
		if (this.isShown()) {
			this.scheduleRefresh();
		}
	}

	private scheduleRefresh() {
		if (this.refreshTimer != null) {
			window.clearTimeout(this.refreshTimer);
		}
		this.refreshTimer = window.setTimeout(() => this.refresh(), REFRESH_DELAY_MS);
	}

	private async refresh() {
		this.refreshTimer = null;
		if (!this.isShown()) {
			return;
		}
		this.stale = false;
		const id = ++this.requestId;
		this.capsTable.classList.add('caps-loading');
		let error = '';
		let caps: Array<StatCap> = [];
		try {
			const result = await this.simUI.sim.statCaps(this.simUI.player);
			error = result.errorResult;
			caps = result.caps;
		} catch (e) {
			error = String(e);
		}
		if (id != this.requestId) {
			return;
		}
		this.capsTable.classList.remove('caps-loading');
		this.caps = error ? [] : caps;
		this.capsError = error;
		this.capsFresh = !this.stale;
		this.capsStatus.textContent = error ? `Couldn't work out your caps: ${error}` : '';
		this.capsStatus.classList.toggle('caps-error', !!error);
		this.renderCaps();
		this.renderValues();
	}

	private renderCaps() {
		const head = newElement('thead');
		head.appendChild(tableRow('th', ['Cap', 'Yours', 'Needed', 'Over / short']));
		const body = newElement('tbody');
		if (this.caps.length == 0 && !this.capsStatus.textContent) {
			const row = newElement('tr');
			const cell = newElement('td', 'caps-empty', this.stale ? 'Working out your caps…' : 'No caps for this spec.');
			cell.colSpan = 4;
			row.appendChild(cell);
			body.appendChild(row);
		}
		for (const cap of this.caps) {
			body.append(...this.capRows(cap));
		}
		this.capsTable.replaceChildren(head, body);
	}

	private capRows(cap: StatCap): Array<HTMLTableRowElement> {
		const key = capKey(cap);
		const row = newElement('tr', 'caps-row');
		// tools/uitest finds rows by these
		row.dataset.label = cap.label;
		row.dataset.hand = String(cap.hand);
		row.dataset.rating = String(cap.rating);
		row.dataset.needed = String(cap.needed);

		const name = newElement('td', 'caps-name', capLabel(cap));
		const tag = KIND_TAGS[cap.kind];
		if (tag) {
			name.appendChild(newElement('span', 'caps-kind badge', tag));
		}
		const standing = capStanding(cap);
		row.append(
			name,
			newElement('td', 'caps-yours', formatRating(cap, cap.rating)),
			newElement('td', 'caps-needed', formatRating(cap, cap.needed)),
			newElement('td', `caps-standing ${standing.className}`, standing.text),
		);
		if (cap.sources.length == 0 && !cap.note) {
			return [row];
		}

		const details = newElement('tr', 'caps-details');
		const cell = newElement('td');
		cell.colSpan = 4;
		if (cap.sources.length > 0) {
			cell.appendChild(this.sourcesTable(cap));
		}
		if (cap.note) {
			cell.appendChild(newElement('div', 'caps-note', cap.note));
		}
		details.appendChild(cell);
		details.hidden = !this.expanded.has(key);

		row.classList.add('caps-expandable');
		row.addEventListener('click', () => {
			details.hidden = !details.hidden;
			if (details.hidden) {
				this.expanded.delete(key);
			} else {
				this.expanded.add(key);
			}
		});
		return [row, details];
	}

	private sourcesTable(cap: StatCap): HTMLTableElement {
		const table = newElement('table', 'caps-sources');
		const sourceRow = (source: StatCapSource) => {
			const tr = tableRow('td', [source.label, signed(source.rating, 0), `${signed(source.percent, 2)}%`]);
			tr.classList.add('caps-source');
			return tr;
		};
		table.append(...cap.sources.map(sourceRow));
		if (Number.isFinite(cap.needed)) {
			const total = tableRow('td', ['Needed from gear', formatRating(cap, cap.needed), '']);
			total.classList.add('caps-source-total');
			table.appendChild(total);
		}
		return table;
	}

	private settingsKey(): string {
		return this.simUI.getStorageKey('caps-tab.v1');
	}

	private loadSettings(): StoredSettings {
		try {
			const stored = JSON.parse(window.localStorage.getItem(this.settingsKey()) || '{}');
			const phase = Number(stored.phase);
			return {
				phase: Number.isInteger(phase) && phase >= MIN_CONTENT_PHASE && phase <= MAX_CONTENT_PHASE ? phase : undefined,
				metric: stored.metric,
			};
		} catch (e) {
			return {};
		}
	}

	private saveSettings() {
		const settings: StoredSettings = { phase: this.pickedPhase ?? undefined, metric: this.metric };
		try {
			window.localStorage.setItem(this.settingsKey(), JSON.stringify(settings));
		} catch (e) {
			// private windows can refuse storage; the pick then lasts until the page closes
		}
	}

	private phase(): number {
		return this.pickedPhase ?? clampPhase(this.simUI.sim.getPhase());
	}

	private priorityControls(spec: Spec): HTMLElement {
		const controls = newElement('div', 'priority-controls');
		for (let phase = MIN_CONTENT_PHASE; phase <= MAX_CONTENT_PHASE; phase++) {
			const option = newElement('option', undefined, PHASE_LABELS[phase]);
			option.value = String(phase);
			this.phaseSelect.appendChild(option);
		}
		this.phaseSelect.addEventListener('change', () => {
			this.pickedPhase = parseInt(this.phaseSelect.value);
			this.saveSettings();
			this.renderValues();
		});
		controls.appendChild(this.phaseSelect);

		if (isTankSpec(spec)) {
			const group = newElement('div', 'btn-group priority-metric');
			group.setAttribute('role', 'group');
			for (const metric of TANK_METRICS) {
				const metricButton = button(METRICS[metric].label, 'btn-outline-primary', () => {
					this.metric = metric;
					this.saveSettings();
					this.renderValues();
				});
				metricButton.dataset.metric = metric;
				this.metricButtons.set(metric, metricButton);
				group.appendChild(metricButton);
			}
			controls.appendChild(group);
		}

		// stays hidden until there's a live run to switch to
		this.sourceGroup.setAttribute('role', 'group');
		this.sourceGroup.hidden = true;
		for (const source of ['shipped', 'live'] as Array<Source>) {
			const sourceButton = button(source == 'live' ? 'My gear' : '', 'btn-outline-primary', () => {
				this.source = source;
				this.renderValues();
			});
			sourceButton.dataset.source = source;
			this.sourceButtons.set(source, sourceButton);
			this.sourceGroup.appendChild(sourceButton);
		}
		controls.appendChild(this.sourceGroup);
		return controls;
	}

	private usingLive(): boolean {
		return !this.shipsValues || (this.source == 'live' && this.live != null);
	}

	private isLiveStale(): boolean {
		const sim = this.simUI.sim;
		return (
			this.live != null &&
			!(RaidProto.equals(this.live.raid, sim.getModifiedRaidProto()) && EncounterProto.equals(this.live.encounter, sim.encounter.toProto()))
		);
	}

	private async measure() {
		if (this.measuring) {
			return;
		}
		const sim = this.simUI.sim;
		this.measuring = true;
		this.measureButton.disabled = true;
		this.measureStatus.textContent = 'Starting the live run…';
		this.measureStatus.classList.remove('caps-error');
		let error = '';
		try {
			await sim.waitForInit();
			const raid = sim.getModifiedRaidProto();
			const encounter = sim.encounter.toProto();
			const iterations = sim.getIterations();
			const result = await sim.statSlopes(this.simUI.player, progress => this.showMeasureProgress(progress));
			if (result.errorResult) {
				error = result.errorResult;
			} else {
				this.live = { values: liveValues(result), raid, encounter, iterations };
				this.source = 'live';
				this.liveStale = this.isLiveStale();
			}
		} catch (e) {
			error = String(e);
		}
		this.measuring = false;
		this.measureButton.disabled = false;
		this.measureStatus.textContent = error ? `The live run failed: ${error}` : '';
		this.measureStatus.classList.toggle('caps-error', !!error);
		this.renderValues();
	}

	private showMeasureProgress(progress: ProgressMetrics) {
		if (this.measuring && progress.totalSims > 0) {
			this.measureStatus.textContent = `Measuring… ${progress.completedSims} of ${progress.totalSims} sims`;
		}
	}

	private requestValues() {
		if (this.valuesRequested || !this.shipsValues) {
			return;
		}
		this.valuesRequested = true;
		this.valuesError = '';
		this.renderValues();
		loadStatValues()
			.then(values => (this.values = values))
			.catch(e => {
				this.valuesError = String(e);
				// another try the next time the tab opens
				this.valuesRequested = false;
			})
			.finally(() => this.renderValues());
	}

	private referenceStat(): Stat {
		return this.metric == 'tmi' ? this.simUI.tankRefStat ?? Stat.StatArmor : this.simUI.individualConfig.epReferenceStat;
	}

	private renderPriority() {
		const phase = this.phase();
		const live = this.usingLive();
		this.phaseSelect.value = String(phase);
		// live values don't depend on the phase
		this.phaseSelect.disabled = live;
		this.metricButtons.forEach((metricButton, metric) => metricButton.classList.toggle('active', metric == this.metric));
		this.sourceGroup.hidden = !this.live || !this.shipsValues;
		this.sourceButtons.get('shipped')?.replaceChildren(`Phase ${phase} BiS`);
		this.sourceButtons.forEach((sourceButton, source) => sourceButton.classList.toggle('active', source == (live ? 'live' : 'shipped')));

		const reference = this.referenceStat();
		const head = newElement('thead');
		const headings = ['Stat', METRICS[this.metric].unit, ...(live ? ['Losing one'] : []), `vs ${REFERENCE_NAMES[reference] ?? statNames.get(reference)}`, ''];
		head.appendChild(tableRow('th', headings));
		const body = newElement('tbody');
		this.priorityTable.replaceChildren(head, body);
		this.priorityTable.classList.toggle('priority-stale', live && this.liveStale);
		this.prioritySource.textContent = '';
		const valuesError = live ? '' : this.valuesError;
		this.priorityStatus.textContent = valuesError ? `Couldn't load the stat values: ${valuesError}` : '';
		this.priorityStatus.classList.toggle('caps-error', !!valuesError);

		const empty = (text: string) => {
			const cell = newElement('td', 'priority-empty', text);
			cell.colSpan = headings.length;
			const row = newElement('tr');
			row.appendChild(cell);
			body.appendChild(row);
		};
		if (live) {
			if (!this.live) {
				empty('Measure at your gear to see what each stat is worth to you.');
				return;
			}
			let source = `Measured at your gear: ${formatNumber(this.live.iterations, 0)} iterations a sim, every sim at one seed, each stat stepped up for its gain and down for its loss.`;
			if (this.liveStale) {
				source += ' Your character changed since: measure again.';
			}
			this.prioritySource.textContent = source;
			const rows = livePriority(this.live.values, this.metric, reference, this.reforging());
			if (rows.length == 0) {
				empty('No stat moved anything in the live run.');
			}
			body.append(...rows.map(row => this.priorityRow(row, true)));
			return;
		}
		if (!this.values) {
			if (!this.valuesError) {
				empty('Loading the stat values…');
			}
			return;
		}

		const shipped = this.shippedPhase();
		if (!shipped) {
			empty(`No shipped values for this spec in P${phase}.`);
			return;
		}
		const { choice } = shipped;
		let source = `Measured at ${choice.build.name}'s P${phase} BiS gear (sim ${this.values.simCommit.slice(0, 9)}, ${this.values.generatedAt.slice(0, 10)}).`;
		if (!choice.treeMatched) {
			source += ` No build has your main talent tree, so these are ${choice.build.name}'s.`;
		}
		this.prioritySource.textContent = source;

		const rows = priority(shipped.phase, this.metric, this.caps, reference, this.reforging());
		if (rows.length == 0) {
			empty('Nothing was measured for this build.');
		}
		body.append(...rows.map(row => this.priorityRow(row, false)));
	}

	private renderValues() {
		this.renderPriority();
		this.renderSuggestions();
	}

	private reforging(): Reforging {
		return reforgingFor(this.simUI.sim.encounter.getServerSettings());
	}

	// the build and phase the priority and the suggestions both score with
	private shippedPhase(): { choice: BuildChoice; phase: ShippedPhase } | null {
		if (!this.values) {
			return null;
		}
		const player = this.simUI.player;
		const twoHand = player.getGear().getEquippedItem(ItemSlot.ItemSlotMainHand)?.item.handType == HandType.HandTypeTwoHand;
		const choice = chooseBuild(this.values.builds, player.spec, player.getTalentTree(), twoHand);
		const phase = choice && phaseOf(choice.build, this.phase());
		return choice && phase ? { choice, phase } : null;
	}

	// Why there's nothing to suggest, or '' once there's enough to score with.
	private suggestionsBlocker(reforging: Reforging): string {
		const live = this.usingLive();
		if (live && !this.live) {
			return 'Reforge suggestions need stat values, and healers get theirs from a live run: Measure at my gear above.';
		}
		if (!reforging.enabled || reforging.statTypes.length == 0) {
			return 'Your server settings have reforging off.';
		}
		if (live) {
			return this.liveStale ? 'Your character changed since the live run: measure again to score reforges with it.' : '';
		}
		if (this.valuesError) {
			return "Couldn't load the stat values to score reforges with.";
		}
		if (!this.values) {
			return 'Loading the stat values…';
		}
		if (!this.shippedPhase()) {
			return `No shipped values for this spec in P${this.phase()}.`;
		}
		if (this.capsError) {
			return "Couldn't work out your caps, which the suggestions need.";
		}
		return '';
	}

	private renderSuggestions() {
		const reforging = this.reforging();
		const blocker = this.suggestionsBlocker(reforging);
		this.reforgeStatus.textContent = blocker;
		this.reforgeControls.hidden = !!blocker;
		this.reforgeTable.hidden = !!blocker;
		if (blocker) {
			this.suggestions = [];
			return;
		}
		if (!this.capsFresh) {
			// the rows stay up, greyed out, until the caps for the new gear arrive
			this.reforgeTable.classList.add('caps-loading');
			this.reforgeBlockButtons().forEach(b => (b.disabled = true));
			if (this.suggestions.length == 0) {
				this.reforgeStatus.textContent = 'Working out your caps…';
				this.reforgeControls.hidden = true;
				this.reforgeTable.hidden = true;
			}
			return;
		}

		const gear = this.simUI.player.getGear();
		const items: Array<ReforgeSlot> = [];
		for (const slot of getEnumValues<ItemSlot>(ItemSlot)) {
			const equipped = gear.getEquippedItem(slot);
			if (equipped) {
				items.push({ slot, item: equipped.item, reforge: equipped.reforge });
			}
		}
		const values = this.usingLive() ? this.live!.values : this.shippedPhase()!.phase;
		this.suggestions = suggestReforges(items, values, this.metric, this.caps, reforging);
		const picked = this.suggestions.filter(s => s.suggested !== undefined);
		const total = picked.reduce((sum, s) => sum + s.gain, 0);

		this.reforgeTable.classList.remove('caps-loading');
		this.applyAllButton.textContent = picked.length > 0 ? `Apply all (${formatGain(this.metric, total)})` : 'Apply all';
		this.applyAllButton.disabled = picked.length == 0;
		if (this.suggestions.length == 0) {
			this.reforgeStatus.textContent = 'Nothing you have equipped can be reforged.';
		} else if (picked.length == 0) {
			this.reforgeStatus.textContent = 'Every reforge is already the best one for these values.';
		}

		const head = newElement('thead');
		head.appendChild(tableRow('th', ['Item', 'Reforged now', 'Suggestion', 'Gain', '']));
		const body = newElement('tbody');
		body.append(...this.suggestions.map(s => this.suggestionRow(s, reforging)));
		this.reforgeTable.replaceChildren(head, body);
	}

	private suggestionRow(s: ReforgeSuggestion, reforging: Reforging): HTMLTableRowElement {
		const tr = newElement('tr', 'reforge-row');
		// tools/uitest finds the row's item and suggestion by these
		tr.dataset.slot = String(s.slot);
		tr.dataset.item = String(s.item.id);
		tr.dataset.current = s.reforge ? reforgeLabel(s.item, s.reforge, reforging) : '';
		tr.dataset.suggestion = s.suggested === undefined ? 'keep' : s.suggested ? reforgeLabel(s.item, s.suggested, reforging) : 'remove';

		const item = newElement('td', 'reforge-item');
		item.append(newElement('span', 'reforge-slot', slotNames.get(s.slot) ?? ''), newElement('span', 'reforge-item-name', s.item.name));
		let now = tr.dataset.current || '—';
		if (s.reforge && !s.active) {
			now += " (your server settings don't allow it)";
		}
		let suggestion = 'keep';
		if (s.suggested !== undefined) {
			suggestion = s.suggested ? reforgeLabel(s.item, s.suggested, reforging) : 'Remove the reforge';
		}
		const action = newElement('td', 'reforge-action');
		if (s.suggested !== undefined) {
			const apply = button('Apply', 'btn-sm btn-outline-primary reforge-apply', () => this.applySuggestions([s]));
			if (!s.alone) {
				apply.disabled = true;
				action.title = 'Alone this would drop a hit or expertise cap: it spends what other suggestions add. Use Apply all.';
			}
			action.appendChild(apply);
		}
		tr.classList.toggle('reforge-keep', s.suggested === undefined);
		tr.append(
			item,
			newElement('td', 'reforge-now', now),
			newElement('td', 'reforge-suggestion', suggestion),
			newElement('td', 'reforge-gain', s.suggested === undefined ? '' : formatGain(this.metric, s.gain)),
			action,
		);
		return tr;
	}

	private reforgeBlockButtons(): Array<HTMLButtonElement> {
		return [this.applyAllButton, ...Array.from(this.reforgeTable.querySelectorAll('button'))];
	}

	// The way the gear picker's Reforging tab sets one, all in one gear change.
	private applySuggestions(suggestions: Array<ReforgeSuggestion>) {
		const player = this.simUI.player;
		let gear = player.getGear();
		for (const s of suggestions) {
			const equipped = gear.getEquippedItem(s.slot);
			if (s.suggested === undefined || !equipped || equipped.id != s.item.id) {
				continue;
			}
			gear = gear.withEquippedItem(s.slot, equipped.withReforge(s.suggested), player.canDualWield2H());
		}
		player.setGear(TypedEvent.nextEventID(), gear);
	}

	private priorityRow(row: PriorityRow, live: boolean): HTMLTableRowElement {
		const tr = newElement('tr', 'priority-row');
		// tools/uitest reads the order and values off these
		tr.dataset.stat = row.gearStat.key;
		tr.dataset.value = String(row.value);
		tr.dataset.loss = row.loss == null ? '' : String(row.loss);
		tr.classList.toggle('priority-capped', row.capped);

		const name = newElement('td', 'priority-name', row.gearStat.label);
		if (row.reforgeable) {
			const marker = newElement('span', 'priority-reforgeable badge', 'reforgeable');
			marker.title = 'Your server lets a reforge move this stat.';
			name.appendChild(marker);
		}
		const digits = METRICS[this.metric].digits;
		tr.append(name, newElement('td', 'priority-value', signed(row.value, digits)));
		if (live) {
			tr.appendChild(newElement('td', 'priority-loss', row.loss == null ? '—' : signed(row.loss, digits)));
		}
		tr.append(newElement('td', 'priority-ratio', row.ratio == null ? '—' : signed(row.ratio, 2)), newElement('td', 'priority-note', row.note));
		return tr;
	}
}
