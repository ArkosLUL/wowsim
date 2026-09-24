import { ContentBlock } from '../core/components/content_block';
import {
	beatsEquipped,
	BUSY_PATTERN,
	button,
	cancelOptimizerRun,
	effortName,
	EFFORTS,
	formatDelta,
	formatNumber,
	LoadoutView,
	newElement,
	PHASE_LABELS,
	RACIAL_MODES,
	scoreUnit,
	section,
	seedTrimmed,
	simServerAvailable,
	sourceCheckboxes,
	SOURCES_HINT,
	tableRow,
	trimmedWhere,
	withUnit,
} from '../core/components/individual_sim_ui/optimizer_tab';
import { SimTab } from '../core/components/sim_tab';
import { IndividualSimUIConfig } from '../core/individual_sim_ui';
import { ALL_SOURCE_KINDS, loadCatalog, MAX_CONTENT_PHASE, MIN_CONTENT_PHASE } from '../core/optimizer/catalog';
import { buildOptimizeRequest, BuiltRequest, defaultTabSettings } from '../core/optimizer/pool_builder';
import { getSpecConfig, Player } from '../core/player';
import { ProgressMetrics, RaidSimRequest, RaidSimResult } from '../core/proto/api';
import { EquipmentSpec, Glyphs, Race, SimDatabase } from '../core/proto/common';
import {
	CatalogSourceKind,
	OptimizerBatchEntry,
	OptimizerBatchExport,
	OptimizerEffort,
	OptimizerObjective,
	OptimizerProgress,
	OptimizerRacialMode,
	OptimizerResult,
} from '../core/proto/optimizer';
import { SavedGearSet } from '../core/proto/ui';
import { Database } from '../core/proto_utils/database';
import { raceNames } from '../core/proto_utils/names';
import { isHealingSpec, isTankSpec, specNames, specToLocalStorageKey } from '../core/proto_utils/utils';
import { SimError } from '../core/sim';
import { EventID, TypedEvent } from '../core/typed_event';
import { downloadString } from '../core/utils';
import { WorkerPool } from '../core/worker_pool';
import { RaidSimUI } from './raid_sim_ui';

const PHASES = Array.from({ length: MAX_CONTENT_PHASE - MIN_CONTENT_PHASE + 1 }, (_, i) => MIN_CONTENT_PHASE + i);

// How long a job waits before asking again while another optimization holds the server.
const BUSY_RETRY_SECONDS = 15;
// A job the server keeps stopping (nobody polled it for 2 minutes, or it was cancelled elsewhere)
// fails after this many tries, so a stuck tab doesn't loop forever.
const MAX_SERVER_STOPS = 3;
// Batches kept in localStorage, one per roster, newest first.
const MAX_STORED_BATCHES = 3;
const STORAGE_PREFIX = 'optimizer-batch.v1.';

type JobState = 'queued' | 'running' | 'done' | 'failed';

// 1: everyone's own-metrics BiS. 2: DPS raiders only, re-optimized for raid DPS with everyone else
// wearing their stage 1 pick.
type Stage = 1 | 2;

// One raider in one content phase, one stage.
interface Job {
	raidIndex: number;
	raider: string;
	phase: number;
	stage: Stage;
	state: JobState;
	result?: OptimizerResult;
	error?: string;
	// what the starting gear lost to fit the phase's pool, and the boss the run fought
	seedChanges: Array<string>;
	bossName: string;
	serverStops: number;
	// only while it runs
	progress?: OptimizerProgress;
}

interface BatchSettings {
	phases: Array<number>;
	effort: OptimizerEffort;
	racialMode: OptimizerRacialMode;
	sources: Array<CatalogSourceKind>;
	// raid indexes left out of the batch
	skipped: Array<number>;
}

interface Batch {
	fingerprint: string;
	settings: BatchSettings;
	jobs: Map<string, Job>;
	// still running when the page went away, so a reload picks it up again
	running: boolean;
}

interface BatchRun {
	stopped: boolean;
	abort: AbortController;
	// picked up again after a reload
	resumed: boolean;
}

// What a phase's Validate found: raid DPS as the raid is, and with the phase's BiS on.
interface Validation {
	phase: number;
	raiders: number;
	now?: { dps: number; se: number };
	bis?: { dps: number; se: number };
	error?: string;
}

function jobKey(raidIndex: number, phase: number, stage: Stage): string {
	return `${raidIndex}:${phase}:${stage}`;
}

// Identifies a grid cell, independent of which stage's job it's currently showing.
function cellKey(raidIndex: number, phase: number): string {
	return `${raidIndex}:${phase}`;
}

function defaultSettings(): BatchSettings {
	return {
		phases: PHASES.slice(),
		effort: OptimizerEffort.OptimizerEffortQuick,
		racialMode: OptimizerRacialMode.OptimizerRacialSearch,
		sources: ALL_SOURCE_KINDS.slice(),
		skipped: [],
	};
}

// cyrb53: a quick 53-bit string hash, plenty to tell rosters apart.
function hashString(text: string): string {
	let h1 = 0xdeadbeef;
	let h2 = 0x41c6ce57;
	for (let i = 0; i < text.length; i++) {
		const ch = text.charCodeAt(i);
		h1 = Math.imul(h1 ^ ch, 2654435761);
		h2 = Math.imul(h2 ^ ch, 1597334677);
	}
	h1 = Math.imul(h1 ^ (h1 >>> 16), 2246822507) ^ Math.imul(h2 ^ (h2 >>> 13), 3266489909);
	h2 = Math.imul(h2 ^ (h2 >>> 16), 2246822507) ^ Math.imul(h1 ^ (h1 >>> 13), 3266489909);
	return (4294967296 * (2097151 & h2) + (h1 >>> 0)).toString(16);
}

// localStorage can throw (private windows, blocked site data, a full quota), and the batch has to
// work without it, so every access goes through these.
function readStorage(key: string): string | null {
	try {
		return window.localStorage.getItem(key);
	} catch (e) {
		return null;
	}
}

function writeStorage(key: string, value: string): boolean {
	try {
		window.localStorage.setItem(key, value);
		return true;
	} catch (e) {
		console.warn(`Couldn't store ${key}: ${e}`);
		return false;
	}
}

function removeStorage(key: string) {
	try {
		window.localStorage.removeItem(key);
	} catch (e) {
		console.warn(`Couldn't remove ${key}: ${e}`);
	}
}

function storageKeys(): Array<string> {
	try {
		return Object.keys(window.localStorage);
	} catch (e) {
		return [];
	}
}

// The top sets are most of a result's size, and the grid never shows them: without them a whole
// roster's batch fits in localStorage.
function compactResult(result: OptimizerResult): OptimizerResult {
	const out = OptimizerResult.clone(result);
	out.top = [];
	return out;
}

function sleep(ms: number, stop: { stopped: boolean }): Promise<void> {
	return new Promise(resolve => {
		const started = Date.now();
		const tick = (): void => {
			if (stop.stopped || Date.now() - started >= ms) {
				resolve();
			} else {
				setTimeout(tick, 250);
			}
		};
		tick();
	});
}

function count(n: number, noun: string): string {
	return `${n} ${noun}${n == 1 ? '' : 's'}`;
}

function shortCommit(commit: string): string {
	return commit.replace(/^([0-9a-f]{12})[0-9a-f]+/, '$1');
}

// The raid batch: every non-healer's BiS for each content phase, one optimizer run after another on
// the sim's web server (it runs one at a time). Stage 1 runs everyone on their own metrics, with the
// buffs and debuffs the rest of the roster gives them; tanks stay on the survival/threat blend there.
// Stage 2 re-optimizes DPS raiders for raid DPS, with everyone else in their stage 1 pick. Cells show
// whichever stage last ran for that raider and phase.
export class OptimizerBatchTab extends SimTab {
	readonly simUI: RaidSimUI;

	private batch: Batch;
	private run: BatchRun | null = null;
	// null until the worker check answers
	private serverAvailable: boolean | null = null;
	private selected: string | null = null;
	private validations = new Map<number, Validation>();
	private validationPool: WorkerPool | null = null;
	private storageFailed = false;
	private rosterCheck: number | null = null;
	// grid cells by cell key, so progress redraws only the running one
	private cells = new Map<string, HTMLTableCellElement>();

	private readonly setupBody: HTMLElement;
	private readonly statusElem: HTMLElement;
	private readonly gridBody: HTMLElement;
	private readonly detailBody: HTMLElement;
	private readonly validationBody: HTMLElement;

	constructor(parentElem: HTMLElement, simUI: RaidSimUI) {
		super(parentElem, simUI, { identifier: 'bis-batch-tab', title: 'BiS Batch' });
		this.simUI = simUI;
		// reuses the optimizer tab's styles
		this.rootElem.classList.add('optimizer-tab');
		this.batch = { fingerprint: '', settings: defaultSettings(), jobs: new Map(), running: false };

		const leftPanel = newElement('div', 'optimizer-tab-left tab-panel-left flex-column');
		const rightPanel = newElement('div', 'optimizer-tab-right tab-panel-right');
		this.contentContainer.append(leftPanel, rightPanel);

		const gridBlock = new ContentBlock(leftPanel, 'optimizer-batch-grid', { header: { title: 'Raiders by phase' } });
		this.gridBody = gridBlock.bodyElement;
		this.validationBody = newElement('div');
		leftPanel.appendChild(this.validationBody);
		const detailBlock = new ContentBlock(leftPanel, 'optimizer-batch-detail', { header: { title: 'Result' } });
		this.detailBody = detailBlock.bodyElement;

		const setupBlock = new ContentBlock(rightPanel, 'optimizer-setup', { header: { title: 'Setup' } });
		this.setupBody = setupBlock.bodyElement;
		this.statusElem = newElement('div', 'optimizer-status');

		this.buildTabContent();
		simServerAvailable().then(available => {
			this.serverAvailable = available;
			this.buildTabContent();
			this.resumeIfRunning();
		});
		this.simUI.sim.waitForInit().then(() => this.checkRoster());
		[this.simUI.compChangeEmitter, this.simUI.sim.raid.changeEmitter].forEach(emitter => emitter.on(() => this.scheduleRosterCheck()));
	}

	protected buildTabContent() {
		this.buildSetup();
		this.renderGrid();
		this.renderDetail();
		this.renderValidations();
	}

	// Raiders the batch can run: everyone but healers, in raid order.
	private raiders(): Array<Player<any>> {
		return this.simUI.sim.raid
			.getActivePlayers()
			.filter(player => !isHealingSpec(player.spec))
			.sort((a, b) => a.getRaidIndex() - b.getRaidIndex());
	}

	// DPS raiders get a stage 2: tanks keep their survival/threat blend either way, so re-running them
	// against the raid wouldn't change their pick.
	private dpsRaiders(): Array<Player<any>> {
		return this.raiders().filter(player => !isTankSpec(player.spec));
	}

	// Identifies the roster: who sits where, with their spec, race, talents, glyphs and professions.
	// Gear and racial traits stay out, so Apply doesn't orphan the batch it came from.
	private fingerprint(): string {
		const players = this.simUI.sim.raid
			.getActivePlayers()
			.map(player => [
				player.getRaidIndex(),
				player.getName(),
				player.spec,
				player.getRace(),
				player.getTalentsString(),
				Glyphs.toJsonString(player.getGlyphs()),
				player.getProfessions(),
			])
			.sort((a, b) => (a[0] as number) - (b[0] as number));
		return players.length > 0 ? hashString(JSON.stringify(players)) : '';
	}

	private storageKey(fingerprint: string): string {
		return this.simUI.getStorageKey(STORAGE_PREFIX + fingerprint);
	}

	// Raid changes come in bursts (a whole raid loads player by player), so check once they settle.
	private scheduleRosterCheck() {
		if (this.rosterCheck != null) {
			return;
		}
		this.rosterCheck = window.setTimeout(() => {
			this.rosterCheck = null;
			this.checkRoster();
		}, 200);
	}

	// Switches to the stored batch for the current roster. A running batch keeps its roster and stops
	// itself before its next job.
	private checkRoster() {
		const fingerprint = this.fingerprint();
		if (this.run || fingerprint == this.batch.fingerprint) {
			this.renderGrid();
			return;
		}
		this.batch = this.loadBatch(fingerprint);
		this.selected = null;
		this.validations.clear();
		this.buildTabContent();
		this.resumeIfRunning();
	}

	private loadBatch(fingerprint: string): Batch {
		const batch: Batch = { fingerprint, settings: defaultSettings(), jobs: new Map(), running: false };
		const stored = fingerprint ? readStorage(this.storageKey(fingerprint)) : null;
		if (!stored) {
			return batch;
		}
		try {
			const saved = JSON.parse(stored);
			const numbers = (value: any) => (Array.isArray(value) ? value.filter((v: any) => typeof v == 'number') : []);
			batch.settings = {
				phases: numbers(saved.settings?.phases).filter(phase => PHASES.includes(phase)),
				effort: EFFORTS.some(e => e.effort == saved.settings?.effort) ? saved.settings.effort : batch.settings.effort,
				racialMode: RACIAL_MODES.some(m => m.mode == saved.settings?.racialMode) ? saved.settings.racialMode : batch.settings.racialMode,
				// older batches have no sources: those count every one
				sources: Array.isArray(saved.settings?.sources)
					? numbers(saved.settings.sources).filter(kind => ALL_SOURCE_KINDS.includes(kind))
					: batch.settings.sources,
				skipped: numbers(saved.settings?.skipped),
			};
			batch.running = saved.running == true;
			for (const job of Array.isArray(saved.jobs) ? saved.jobs : []) {
				const state: JobState = job.state == 'done' || job.state == 'failed' ? job.state : 'queued';
				const stage: Stage = job.stage == 2 ? 2 : 1;
				batch.jobs.set(jobKey(job.raidIndex, job.phase, stage), {
					raidIndex: job.raidIndex,
					raider: String(job.raider || ''),
					phase: job.phase,
					stage,
					state,
					result: job.result ? OptimizerResult.fromJson(job.result, { ignoreUnknownFields: true }) : undefined,
					error: typeof job.error == 'string' ? job.error : undefined,
					seedChanges: Array.isArray(job.seedChanges) ? job.seedChanges.map(String) : [],
					bossName: String(job.bossName || ''),
					serverStops: 0,
				});
			}
		} catch (e) {
			console.warn(`Ignoring the stored raid batch: ${e}`);
			return { fingerprint, settings: defaultSettings(), jobs: new Map(), running: false };
		}
		return batch;
	}

	private storeBatch() {
		if (!this.batch.fingerprint) {
			return;
		}
		const jobs = [...this.batch.jobs.values()].map(job => ({
			raidIndex: job.raidIndex,
			raider: job.raider,
			phase: job.phase,
			stage: job.stage,
			state: job.state == 'running' ? 'queued' : job.state,
			result: job.result ? OptimizerResult.toJson(job.result) : undefined,
			error: job.error,
			seedChanges: job.seedChanges,
			bossName: job.bossName,
		}));
		const text = JSON.stringify({ savedAt: Date.now(), running: this.batch.running, settings: this.batch.settings, jobs });
		const key = this.storageKey(this.batch.fingerprint);
		let stored = writeStorage(key, text);
		if (!stored) {
			// other rosters' batches can take up the quota this one needs
			this.batchKeys()
				.filter(other => other != key)
				.forEach(removeStorage);
			stored = writeStorage(key, text);
		}
		if (!stored && !this.storageFailed) {
			this.storageFailed = true;
			this.showStatus("Couldn't save the batch in this browser (storage is full or blocked), so a reload starts it over.", 'warning');
		}
		this.pruneStorage();
	}

	private batchKeys(): Array<string> {
		const prefix = this.simUI.getStorageKey(STORAGE_PREFIX);
		return storageKeys().filter(key => key.startsWith(prefix));
	}

	// Keeps the newest few rosters' batches.
	private pruneStorage() {
		const batches = this.batchKeys()
			.map(key => {
				let savedAt = 0;
				try {
					savedAt = JSON.parse(readStorage(key) || '{}').savedAt || 0;
				} catch (e) {
					// unreadable, so it goes first
				}
				return { key, savedAt };
			})
			.sort((a, b) => b.savedAt - a.savedAt);
		batches.slice(MAX_STORED_BATCHES).forEach(({ key }) => removeStorage(key));
	}

	private changeSettings(change: (settings: BatchSettings) => void) {
		change(this.batch.settings);
		this.storeBatch();
		this.buildTabContent();
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

	private select<T extends number>(options: Array<{ value: T; label: string }>, current: T, onChange: (value: T) => void): HTMLSelectElement {
		const elem = newElement('select', 'form-select');
		for (const { value, label } of options) {
			const option = newElement('option', undefined, label);
			option.value = String(value);
			option.selected = value == current;
			elem.appendChild(option);
		}
		elem.addEventListener('change', () => onChange(parseInt(elem.value) as T));
		return elem;
	}

	private buildSetup() {
		const settings = this.batch.settings;
		const available = this.serverAvailable == true;
		this.setupBody.replaceChildren();
		if (this.serverAvailable == false) {
			this.setupBody.appendChild(
				newElement(
					'div',
					'optimizer-availability',
					"The batch runs on the sim's web server, and this page runs the sim in your browser (wasm). Start the sim with its web server to use it.",
				),
			);
		}
		this.setupBody.appendChild(
			newElement(
				'p',
				'optimizer-hint',
				"Finds each DPS and tank raider's best gear for every content phase you pick, one optimizer run at a time, with the " +
					"buffs and debuffs the rest of this roster gives them. Tanks trade 70% survival against 30% threat and have to end up " +
					'crit immune. Healers sit this one out. Each phase runs twice: first everyone gets their own-metrics pick, then DPS ' +
					"raiders run again scored on the whole raid's DPS with everyone else wearing their first pick.",
			),
		);

		const effort = this.select(
			EFFORTS.map(e => ({ value: e.effort, label: e.label })),
			settings.effort,
			value => this.changeSettings(s => (s.effort = value)),
		);
		this.setupBody.appendChild(this.labeled('Effort', effort, 'Per run. Quick gets a whole roster done in a sitting, Normal is one for overnight.'));

		const racial = this.select(
			RACIAL_MODES.map(m => ({ value: m.mode, label: m.mode == OptimizerRacialMode.OptimizerRacialKeepCurrent ? 'Keep their traits' : m.label })),
			settings.racialMode,
			value => this.changeSettings(s => (s.racialMode = value)),
		);
		this.setupBody.appendChild(this.labeled('Racial traits', racial));

		const sources = sourceCheckboxes(settings.sources, picked => this.changeSettings(s => (s.sources = picked)));
		this.setupBody.appendChild(this.labeled('Item sources', sources, SOURCES_HINT));

		const jobs = this.plannedJobs();
		const left = jobs.filter(job => job.state != 'done').length;
		const actions = newElement('div', 'optimizer-actions');
		const started = jobs.some(job => job.state == 'done' || job.state == 'failed');
		const start = button(started ? 'Resume' : 'Start', 'btn-primary', () => this.start());
		start.disabled = !available || this.run != null || left == 0;
		const stop = button('Stop', 'btn-secondary', () => this.stop());
		stop.hidden = this.run == null;
		const retry = button('Run failed ones again', 'btn-secondary', () => this.retryFailed());
		retry.hidden = !jobs.some(job => job.state == 'failed');
		retry.disabled = !available;
		const exportButton = button('Export JSON', 'btn-secondary', () => this.exportJson());
		exportButton.disabled = ![...this.batch.jobs.values()].some(job => job.state == 'done');
		const clear = button('Clear results', 'btn-secondary', () => this.clear());
		// the grid fills the job map with queued jobs just by drawing, so look for ones that ran
		clear.disabled = this.run != null || ![...this.batch.jobs.values()].some(job => job.state == 'done' || job.state == 'failed');
		actions.append(start, stop, retry, exportButton, clear);
		this.setupBody.appendChild(actions);
		let progressText: string;
		if (jobs.length == 0) {
			progressText = 'Tick at least one raider and one phase on the grid.';
		} else if (left == 0) {
			progressText = `${count(jobs.length, 'run')} done, none left.`;
		} else {
			progressText = `${left} of ${count(jobs.length, 'run')} to go. Pick raiders and phases with the checkboxes on the grid.`;
		}
		this.setupBody.appendChild(newElement('div', 'optimizer-hint', progressText));
		if (this.run?.resumed) {
			this.setupBody.appendChild(newElement('div', 'optimizer-hint', 'Picked up where it left off before the reload.'));
		}
		this.setupBody.appendChild(this.statusElem);
	}

	private showStatus(text: string, kind: 'info' | 'warning' | 'error' = 'info'): HTMLElement {
		this.statusElem.replaceChildren();
		this.statusElem.className = `optimizer-status optimizer-status-${kind}`;
		const actions = newElement('div', 'optimizer-status-actions');
		if (text) {
			this.statusElem.append(newElement('div', undefined, text), actions);
		}
		return actions;
	}

	// The runs the settings ask for, in the order they go: phase by phase, stage 1 for everyone before
	// stage 2 for that phase's DPS raiders, so each raider's run can start from their BiS of the phase
	// before. A phase's stage 2 jobs only join the list once every raider's stage 1 there has settled
	// (done or failed): stage 2 needs the others' stage 1 picks to score raid DPS against.
	private plannedJobs(): Array<Job> {
		const skipped = new Set(this.batch.settings.skipped);
		const phases = this.batch.settings.phases.slice().sort((a, b) => a - b);
		const raiders = this.raiders().filter(player => !skipped.has(player.getRaidIndex()));
		const dps = this.dpsRaiders().filter(player => !skipped.has(player.getRaidIndex()));
		const out: Array<Job> = [];
		for (const phase of phases) {
			const stage1 = raiders.map(player => this.job(player, phase, 1));
			out.push(...stage1);
			if (dps.length > 0 && stage1.every(job => job.state == 'done' || job.state == 'failed')) {
				out.push(...dps.map(player => this.job(player, phase, 2)));
			}
		}
		return out;
	}

	private job(player: Player<any>, phase: number, stage: Stage): Job {
		const key = jobKey(player.getRaidIndex(), phase, stage);
		let job = this.batch.jobs.get(key);
		if (!job) {
			job = { raidIndex: player.getRaidIndex(), raider: player.getName(), phase, stage, state: 'queued', seedChanges: [], bossName: '', serverStops: 0 };
			this.batch.jobs.set(key, job);
		}
		return job;
	}

	// The raider's job for a phase that's most worth showing: stage 2 once it's started, else stage 1.
	private currentJob(raidIndex: number, phase: number): Job | undefined {
		const stage2 = this.batch.jobs.get(jobKey(raidIndex, phase, 2));
		if (stage2 && stage2.state != 'queued') {
			return stage2;
		}
		return this.batch.jobs.get(jobKey(raidIndex, phase, 1)) ?? stage2;
	}

	// The raider's best finished pick for a phase, for Apply, Save and Validate: stage 2's once it has
	// one, else stage 1's.
	private bestUsableJob(raidIndex: number, phase: number): Job | undefined {
		const stage2 = this.batch.jobs.get(jobKey(raidIndex, phase, 2));
		if (stage2?.state == 'done' && stage2.result?.best?.equipment) {
			return stage2;
		}
		const stage1 = this.batch.jobs.get(jobKey(raidIndex, phase, 1));
		return stage1?.state == 'done' && stage1.result?.best?.equipment ? stage1 : undefined;
	}

	private resumeIfRunning() {
		if (this.batch.running && this.serverAvailable && !this.run && this.plannedJobs().some(job => job.state == 'queued')) {
			this.start(true);
		}
	}

	private async start(resumed = false) {
		if (this.run || !this.serverAvailable) {
			return;
		}
		const run: BatchRun = { stopped: false, abort: new AbortController(), resumed };
		this.run = run;
		this.batch.running = true;
		this.storeBatch();
		this.buildSetup();
		const fingerprint = this.batch.fingerprint;
		try {
			while (!run.stopped) {
				if (this.fingerprint() != fingerprint) {
					this.showStatus('The roster changed, so the batch stopped. Its results stay with the old roster and come back with it.', 'warning');
					break;
				}
				const job = this.plannedJobs().find(job => job.state == 'queued');
				if (!job) {
					const failed = this.plannedJobs().filter(job => job.state == 'failed').length;
					this.showStatus(
						failed > 0 ? `Done, but ${count(failed, 'run')} failed. Click one to see why.` : 'All done.',
						failed > 0 ? 'warning' : 'info',
					);
					break;
				}
				await this.runJob(job, run);
			}
		} finally {
			this.run = null;
			// only a reload mid-run skips this, and that's the one case that should resume
			this.batch.running = false;
			if (fingerprint == this.batch.fingerprint) {
				this.storeBatch();
			}
			this.checkRoster();
			this.buildTabContent();
		}
	}

	private stop() {
		if (!this.run) {
			return;
		}
		this.run.stopped = true;
		this.run.abort.abort();
		this.batch.running = false;
		this.storeBatch();
		this.showStatus('Stopped. Resume picks it up again, starting with the run that got cut off.');
	}

	private retryFailed() {
		this.plannedJobs()
			.filter(job => job.state == 'failed')
			.forEach(job => this.requeue(job));
		this.storeBatch();
		this.buildTabContent();
		this.start();
	}

	private requeue(job: Job) {
		job.state = 'queued';
		job.result = undefined;
		job.error = undefined;
		job.serverStops = 0;
	}

	private clear() {
		if (!confirm("Clear this roster's batch results?")) {
			return;
		}
		this.batch.jobs.clear();
		this.batch.running = false;
		this.selected = null;
		this.validations.clear();
		removeStorage(this.storageKey(this.batch.fingerprint));
		this.showStatus('');
		this.buildTabContent();
	}

	private async buildRequest(player: Player<any>, job: Job): Promise<BuiltRequest> {
		const catalog = await loadCatalog();
		const settings = defaultTabSettings(job.phase);
		settings.effort = this.batch.settings.effort;
		settings.racialMode = this.batch.settings.racialMode;
		settings.sources = this.batch.settings.sources.slice();

		const warmStarts: Array<EquipmentSpec> = [];
		// the latest earlier phase that has a result, preferring its stage 2 pick when it ran one
		const earlier = PHASES.filter(phase => phase < job.phase)
			.sort((a, b) => b - a)
			.map(phase => this.bestUsableJob(job.raidIndex, phase))
			.find(prev => prev);
		if (earlier) {
			warmStarts.push(earlier.result!.best!.equipment!);
		}

		const base = this.simUI.sim.makeRaidSimRequest(false);
		let objective: OptimizerObjective | undefined;
		if (job.stage == 2) {
			objective = OptimizerObjective.OptimizerObjectiveRaidDps;
			this.wearStage1Picks(base, job.phase, job.raidIndex);
			// this phase's own stage 1 pick is a strong start for the raid-scored search too, and it's
			// already inside this target's own pool, so it's safe to warm-start from
			const own = this.batch.jobs.get(jobKey(job.raidIndex, job.phase, 1));
			if (own?.state == 'done' && own.result?.best?.equipment) {
				warmStarts.push(own.result.best.equipment);
			}
		}

		return buildOptimizeRequest({
			base,
			targetRaidIndex: job.raidIndex,
			settings,
			catalog,
			filters: this.simUI.sim.getFilters(),
			getItems: slot => player.getItems(slot),
			getEnchants: slot => player.getEnchants(slot),
			db: this.simUI.sim.db,
			warmStarts,
			objective,
		});
	}

	// Wears every other active raider's phase stage 1 pick, so a stage 2 request's raid sims score the
	// target against the raid stage 1 leaves them in. Raiders without a done stage 1 there (skipped, or
	// mid-run) keep whatever gear the live raid has them in.
	private wearStage1Picks(base: RaidSimRequest, phase: number, exceptRaidIndex: number) {
		for (const player of this.raiders()) {
			const raidIndex = player.getRaidIndex();
			if (raidIndex == exceptRaidIndex) {
				continue;
			}
			const stage1 = this.batch.jobs.get(jobKey(raidIndex, phase, 1));
			const best = stage1?.state == 'done' ? stage1.result?.best : undefined;
			if (!best?.equipment) {
				continue;
			}
			const raidPlayer = base.raid?.parties[Math.floor(raidIndex / 5)]?.players[raidIndex % 5];
			if (!raidPlayer) {
				continue;
			}
			const gear = this.simUI.sim.db.lookupEquipmentSpec(best.equipment);
			raidPlayer.equipment = gear.asSpec();
			// the server has no item database of its own, so the request carries what everyone wears
			raidPlayer.database = Database.mergeSimDatabases(raidPlayer.database || SimDatabase.create(), gear.toDatabase());
			if (best.racialTraits != Race.RaceUnknown) {
				raidPlayer.racialTraits = best.racialTraits;
			}
		}
	}

	private async runJob(job: Job, run: BatchRun) {
		const player = this.simUI.sim.raid.getPlayer(job.raidIndex);
		if (!player) {
			job.state = 'failed';
			job.error = 'That raid slot is empty now.';
			return;
		}
		job.state = 'running';
		job.progress = undefined;
		this.showStatus(`Running ${job.raider}, P${job.phase}.`);
		this.renderGrid();
		try {
			const built = await this.buildRequest(player, job);
			if (run.stopped) {
				job.state = 'queued';
				return;
			}
			const result = await this.simUI.sim.runGearOptimizer(built.request, progress => this.showProgress(job, progress), run.abort.signal);
			if (result.cancelled) {
				job.state = 'queued';
				if (!run.stopped) {
					this.serverStopped(job);
				}
				return;
			}
			job.state = 'done';
			job.result = compactResult(result);
			job.error = undefined;
			job.seedChanges = built.seedChanges;
			job.bossName = built.bossName;
			job.serverStops = 0;
		} catch (e) {
			const message = e instanceof SimError ? e.errorStr : String(e);
			const busy = BUSY_PATTERN.exec(message);
			if (busy) {
				job.state = 'queued';
				this.renderGrid();
				await this.waitForServer(busy[1], run);
				return;
			}
			job.state = 'failed';
			job.error = message;
		} finally {
			job.progress = undefined;
			this.storeBatch();
			this.renderGrid();
			if (this.selected == cellKey(job.raidIndex, job.phase)) {
				this.renderDetail();
			}
		}
	}

	// The server cancels a run nobody has polled for 2 minutes (a background tab can do that), and
	// another page can cancel it too. Either way the job goes again, a few times at most.
	private serverStopped(job: Job) {
		job.serverStops++;
		if (job.serverStops >= MAX_SERVER_STOPS) {
			job.state = 'failed';
			job.error = `The server stopped this run ${job.serverStops} times without being asked. Keeping this tab in the foreground usually helps.`;
			return;
		}
		this.showStatus(`The server stopped ${job.raider}'s P${job.phase} run without being asked, so it goes again.`, 'warning');
	}

	private async waitForServer(progressId: string, run: { stopped: boolean }) {
		const actions = this.showStatus(
			`Another optimization is running on the server, so the batch waits and tries again every ${BUSY_RETRY_SECONDS} seconds.`,
			'warning',
		);
		const cancelOther = button('Cancel that run', 'btn-secondary btn-sm', async () => {
			cancelOther.disabled = true;
			try {
				cancelOther.textContent = (await cancelOptimizerRun(progressId)) ? 'Cancel sent' : 'It already finished';
			} catch (err) {
				cancelOther.textContent = `Cancel failed: ${err}`;
			}
		});
		actions.appendChild(cancelOther);
		await sleep(BUSY_RETRY_SECONDS * 1000, run);
	}

	private showProgress(job: Job, metrics: ProgressMetrics) {
		const progress = metrics.optimizerProgress;
		if (!progress || job.state != 'running') {
			return;
		}
		job.progress = progress;
		const parts = [`${job.raider}, P${job.phase}: ${progress.stage || 'running'}`];
		if (progress.totalSteps > 0) {
			parts.push(`step ${progress.completedSteps} of ${progress.totalSteps}`);
		}
		if (progress.totalSims > 0) {
			parts.push(`${progress.completedSims} of about ${progress.totalSims} sims`);
		}
		if (progress.elapsedSeconds) {
			parts.push(`${Math.round(progress.elapsedSeconds)} s`);
		}
		this.showStatus(parts.join(', '));
		// progress lands every half second: redrawing the whole grid that often eats clicks and
		// tooltips on the other cells
		const td = this.cells.get(cellKey(job.raidIndex, job.phase));
		if (td) {
			this.fillCell(td, job, true);
		}
	}

	private renderGrid() {
		const raiders = this.raiders();
		this.gridBody.replaceChildren();
		this.cells.clear();
		if (raiders.length == 0) {
			this.gridBody.appendChild(newElement('p', 'optimizer-hint', 'No DPS or tank raiders yet. Add some on the Raid tab, or import a roster.'));
			return;
		}
		const settings = this.batch.settings;
		const skipped = new Set(settings.skipped);
		const locked = this.run != null;
		const table = newElement('table', 'table table-sm optimizer-table');
		const head = newElement('tr');
		head.appendChild(newElement('th', undefined, 'Raider'));
		for (const phase of PHASES) {
			const th = newElement('th');
			th.title = PHASE_LABELS[phase];
			th.appendChild(
				this.checkbox(`P${phase}`, settings.phases.includes(phase), locked, checked =>
					this.changeSettings(s => (s.phases = checked ? [...s.phases, phase] : s.phases.filter(p => p != phase))),
				),
			);
			th.appendChild(this.phaseActions(phase));
			head.appendChild(th);
		}
		table.appendChild(head);

		for (const player of raiders) {
			const index = player.getRaidIndex();
			const tr = newElement('tr');
			const name = newElement('td');
			const raider = this.checkbox(player.getName(), !skipped.has(index), locked, checked =>
				this.changeSettings(s => (s.skipped = checked ? s.skipped.filter(i => i != index) : [...s.skipped, index])),
			);
			raider
				.querySelector('.form-check-label')!
				.appendChild(newElement('div', 'optimizer-hint', `${specNames[player.spec]}${isTankSpec(player.spec) ? ' (tank)' : ''}`));
			name.appendChild(raider);
			tr.appendChild(name);
			for (const phase of PHASES) {
				const job = this.currentJob(index, phase);
				const planned = settings.phases.includes(phase) && !skipped.has(index);
				const td = newElement('td');
				this.fillCell(td, job, planned);
				this.cells.set(cellKey(index, phase), td);
				tr.appendChild(td);
			}
			table.appendChild(tr);
		}
		this.gridBody.appendChild(table);
		this.gridBody.appendChild(
			newElement(
				'div',
				'optimizer-hint',
				"Each cell is the run's score gain over the raider's gear as equipped, ± its standard error, in points of their EP " +
					"reference stat (AP, SP or RAP), not DPS, with a tank's survival in armor points. It can come out negative when the " +
					"raider wears gear the phase rules out. A DPS raider's cell also gets a raid DPS line once their stage 2 run scores " +
					'it. Click one for the gear.',
			),
		);
	}

	private checkbox(label: string, checked: boolean, disabled: boolean, onChange: (checked: boolean) => void): HTMLElement {
		const wrapper = newElement('label', 'form-check');
		const input = newElement('input', 'form-check-input');
		input.type = 'checkbox';
		input.checked = checked;
		input.disabled = disabled;
		input.addEventListener('change', () => onChange(input.checked));
		wrapper.append(input, newElement('span', 'form-check-label', label));
		return wrapper;
	}

	private fillCell(td: HTMLTableCellElement, job: Job | undefined, planned: boolean) {
		td.replaceChildren();
		td.className = '';
		const key = job ? cellKey(job.raidIndex, job.phase) : null;
		if (key && key == this.selected) {
			td.classList.add('table-active');
		}
		if (!job || (job.state == 'queued' && !planned)) {
			td.appendChild(newElement('span', 'optimizer-hint', planned ? 'queued' : ''));
			return;
		}
		const open = () => {
			this.selected = key;
			this.renderGrid();
			this.renderDetail();
		};
		const link = newElement('a', undefined);
		link.href = 'javascript:void(0)';
		link.addEventListener('click', open);
		td.appendChild(link);

		switch (job.state) {
			case 'queued':
				link.appendChild(newElement('span', 'optimizer-hint', 'queued'));
				break;
			case 'running': {
				const progress = job.progress;
				const text = progress?.totalSteps ? `${progress.stage}, step ${progress.completedSteps} of ${progress.totalSteps}` : 'starting';
				link.appendChild(newElement('span', 'optimizer-hint', text));
				break;
			}
			case 'failed':
				link.appendChild(newElement('span', 'text-danger', job.stage == 2 ? 'stage 2 failed' : 'failed'));
				link.title = job.error || '';
				break;
			case 'done': {
				const result = job.result!;
				const best = result.best;
				// no gain: best is the starting gear, whose delta is what the phase took off the gear as equipped
				const delta = best ? withUnit(formatDelta(best.scoreDelta, best.scoreDeltaSe), scoreUnit(result.scoreStats)) : '';
				const trimmed = !!best && seedTrimmed(result) && !!(best.scoreDelta || best.scoreDeltaSe);
				const gain = result.improved && best ? delta : trimmed ? `no gain, ${delta}` : 'no gain';
				link.appendChild(newElement('div', result.improved ? undefined : 'optimizer-hint', gain));
				// raid-sim numbers, once a run measures them
				if (best && (best.raidDpsDelta || best.raidDpsDeltaSe)) {
					link.appendChild(newElement('div', 'optimizer-hint', `raid DPS ${formatDelta(best.raidDpsDelta, best.raidDpsDeltaSe)}`));
				}
				const warnings = this.warnings(result);
				if (warnings.length > 0) {
					const icon = newElement('i', 'fa fa-exclamation-triangle text-warning ms-1');
					icon.title = warnings.join('\n');
					link.firstElementChild?.appendChild(icon);
				}
				if (!planned) {
					td.classList.add('opacity-50');
				}
				break;
			}
		}
	}

	// What a cell flags: the run's warnings, its pick's, effects the sim doesn't model and a
	// calibration gap over 3%.
	private warnings(result: OptimizerResult): Array<string> {
		const out = [...result.warnings, ...(result.best?.warnings || [])];
		const unmodeled = result.best?.unmodeledEffectItemIds.length || 0;
		if (unmodeled > 0) {
			out.push(`The sim doesn't model the effects of ${unmodeled} of these items.`);
		}
		if (Math.abs(result.calibrationGap) > 0.03) {
			out.push(`Full raid vs the derived buffs differ by ${formatNumber(result.calibrationGap * 100)}%.`);
		}
		return out;
	}

	private phaseJobs(phase: number): Array<Job> {
		return this.raiders()
			.map(player => this.bestUsableJob(player.getRaidIndex(), phase))
			.filter((job): job is Job => !!job)
			.sort((a, b) => a.raidIndex - b.raidIndex);
	}

	private phaseActions(phase: number): HTMLElement {
		const wrapper = newElement('div', 'd-flex flex-wrap gap-2');
		const done = this.phaseJobs(phase).length;
		const apply = button('Apply', 'btn-link btn-sm p-0', () => this.applyPhase(phase));
		apply.title = `Put every raider in their P${phase} BiS here in the raid sim.`;
		const save = button('Save', 'btn-link btn-sm p-0', () => this.savePhase(phase));
		save.title = `Save each raider's P${phase} BiS as a gear set named "<Raider> P${phase} BiS".`;
		const validate = button('Validate', 'btn-link btn-sm p-0', () => this.validate(phase));
		validate.title = `Run the raid sim twice on the same random numbers: the raid as it is, and with everyone's P${phase} BiS on.`;
		[apply, save, validate].forEach(elem => (elem.disabled = done == 0));
		validate.disabled ||= this.serverAvailable != true;
		wrapper.append(apply, save, validate);
		return wrapper;
	}

	// a running batch keeps its old roster until the current run ends, so after a swap its raid
	// indexes can point at someone else. Warns and returns true then
	private rosterChanged(): boolean {
		if (this.fingerprint() == this.batch.fingerprint) {
			return false;
		}
		this.showStatus(
			"The roster changed since this batch ran, so its results could land on the wrong raider. The grid switches to the new roster once the current run ends.",
			'warning',
		);
		return true;
	}

	private applyPhase(phase: number) {
		if (this.rosterChanged()) {
			return;
		}
		const jobs = this.phaseJobs(phase);
		const eventID = TypedEvent.nextEventID();
		TypedEvent.freezeAllAndDo(() => jobs.forEach(job => this.apply(eventID, job)));
		this.showStatus(`Put ${count(jobs.length, 'raider')} in their P${phase} BiS.`);
	}

	private savePhase(phase: number) {
		if (this.rosterChanged()) {
			return;
		}
		const jobs = this.phaseJobs(phase);
		const saved = jobs.filter(job => this.save(job)).length;
		this.showStatus(
			saved == jobs.length
				? `Saved ${count(saved, 'gear set')}. They show up under each raider's Gear Sets.`
				: `Saved ${saved} of ${count(jobs.length, 'gear set')}. This browser wouldn't store the rest.`,
			saved == jobs.length ? 'info' : 'warning',
		);
	}

	// Equips the job's pick on its raider, racial traits included, the way the tab's Equip does.
	private apply(eventID: EventID, job: Job) {
		const player = this.simUI.sim.raid.getPlayer(job.raidIndex);
		const best = job.result?.best;
		if (!player || !best) {
			return;
		}
		player.setGear(eventID, this.simUI.sim.db.lookupEquipmentSpec(best.equipment || EquipmentSpec.create()));
		if (best.racialTraits != Race.RaceUnknown && best.racialTraits != player.getEffectiveRacialTraits()) {
			player.setRacialTraits(eventID, best.racialTraits == player.getRace() ? Race.RaceUnknown : best.racialTraits);
		}
	}

	// Stores the pick with the raider's spec's saved gear sets, where their Gear tab lists them. False
	// when the browser won't store it.
	private save(job: Job): boolean {
		const player = this.simUI.sim.raid.getPlayer(job.raidIndex);
		const best = job.result?.best;
		if (!player || !best) {
			return false;
		}
		const key = specToLocalStorageKey[player.spec] + '__savedGear__';
		let sets: Record<string, any> = {};
		try {
			sets = JSON.parse(readStorage(key) || '{}') || {};
		} catch (e) {
			console.warn(`Replacing unreadable saved gear sets in ${key}: ${e}`);
		}
		sets[`${job.raider} P${job.phase} BiS`] = SavedGearSet.toJson(
			SavedGearSet.create({
				gear: this.simUI.sim.db.lookupEquipmentSpec(best.equipment || EquipmentSpec.create()).asSpec(),
				bonusStatsStats: player.getBonusStats().toProto(),
			}),
		);
		return writeStorage(key, JSON.stringify(sets));
	}

	private async validate(phase: number) {
		const jobs = this.phaseJobs(phase);
		if (jobs.length == 0 || !this.serverAvailable || this.rosterChanged()) {
			return;
		}
		const validation: Validation = { phase, raiders: jobs.length };
		this.validations.set(phase, validation);
		this.renderValidations();
		try {
			const now = this.simUI.sim.makeRaidSimRequest(false);
			now.simOptions!.debugFirstIteration = false;
			const bis = RaidSimRequest.clone(now);
			for (const job of jobs) {
				const player = bis.raid?.parties[Math.floor(job.raidIndex / 5)]?.players[job.raidIndex % 5];
				const best = job.result!.best!;
				if (!player) {
					continue;
				}
				const gear = this.simUI.sim.db.lookupEquipmentSpec(best.equipment!);
				player.equipment = gear.asSpec();
				// the server has no item database of its own, so the request carries what it wears
				player.database = Database.mergeSimDatabases(player.database || SimDatabase.create(), gear.toDatabase());
				if (best.racialTraits != Race.RaceUnknown) {
					player.racialTraits = best.racialTraits;
				}
			}
			this.validationPool ??= new WorkerPool(1);
			const iterations = now.simOptions!.iterations;
			const measure = async (request: RaidSimRequest) => {
				const result: RaidSimResult = await this.validationPool!.raidSimAsync(request, () => undefined);
				if (result.errorResult) {
					throw new SimError(result.errorResult);
				}
				const dps = result.raidMetrics?.dps;
				return { dps: dps?.avg || 0, se: (dps?.stdev || 0) / Math.sqrt(Math.max(1, iterations)) };
			};
			validation.now = await measure(now);
			this.renderValidations();
			validation.bis = await measure(bis);
		} catch (e) {
			validation.error = e instanceof SimError ? e.errorStr : String(e);
		}
		this.renderValidations();
	}

	private renderValidations() {
		this.validationBody.replaceChildren();
		const validations = [...this.validations.values()].sort((a, b) => a.phase - b.phase);
		if (validations.length == 0) {
			return;
		}
		const list = newElement('ul', 'optimizer-list');
		for (const v of validations) {
			let text: string;
			if (v.error) {
				text = `P${v.phase}: the raid sim failed: ${v.error}`;
			} else if (!v.now || !v.bis) {
				text = `P${v.phase}: running the raid sim…`;
			} else {
				const gain = v.bis.dps - v.now.dps;
				text =
					`P${v.phase}: ${formatNumber(v.now.dps)} ± ${formatNumber(v.now.se)} raid DPS as the raid is now, ` +
					`${formatNumber(v.bis.dps)} ± ${formatNumber(v.bis.se)} with ${count(v.raiders, 'raider')} in their P${v.phase} BiS ` +
					`(${gain >= 0 ? '+' : ''}${formatNumber(gain)}).`;
			}
			list.appendChild(newElement('li', undefined, text));
		}
		this.validationBody.appendChild(
			section('Validate', list, "The raid's own encounter, so tanks don't fight the boss they were optimized for. Both runs share random numbers."),
		);
	}

	private renderDetail() {
		this.detailBody.replaceChildren();
		const parsed = this.selected?.split(':').map(Number);
		const job = parsed ? this.currentJob(parsed[0], parsed[1]) : undefined;
		if (!job) {
			this.detailBody.appendChild(newElement('p', 'optimizer-hint', 'Click a cell to see its gear, runners-up and character sheet.'));
			return;
		}
		const player = this.simUI.sim.raid.getPlayer(job.raidIndex);
		this.detailBody.appendChild(newElement('h5', undefined, `${job.raider}, ${PHASE_LABELS[job.phase] || `P${job.phase}`}`));
		const actions = newElement('div', 'optimizer-actions');
		const again = button('Run it again', 'btn-secondary', () => {
			this.requeue(job);
			this.storeBatch();
			this.buildTabContent();
		});
		// unticked, the requeued job would never run, and its result would be gone
		const unticked = !this.batch.settings.phases.includes(job.phase) || this.batch.settings.skipped.includes(job.raidIndex);
		again.disabled = job.state == 'running' || job.state == 'queued' || unticked;
		const againHint = newElement('span', 'optimizer-hint', unticked ? `To run it again, tick ${job.raider} and P${job.phase} on the grid.` : '');

		if (job.state != 'done' || !job.result || !player) {
			const text =
				job.state == 'failed'
					? `This run failed: ${job.error}`
					: job.state == 'running'
					? "It's running now."
					: !player
					? 'That raid slot is empty now.'
					: "It hasn't run yet.";
			this.detailBody.appendChild(newElement('div', job.state == 'failed' ? 'optimizer-warning' : 'optimizer-hint', text));
			actions.append(again, againHint);
			this.detailBody.appendChild(actions);
			return;
		}

		const result = job.result;
		const provenance: Array<string> = [job.stage == 2 ? 'stage 2, raid DPS' : 'stage 1, own metrics'];
		if (result.settings) {
			provenance.push(effortName(result.settings.effort));
		}
		if (result.simCommit) {
			provenance.push(`sim ${shortCommit(result.simCommit)}`);
		}
		if (result.catalogDate) {
			provenance.push(`catalog ${result.catalogDate}`);
		}
		if (job.bossName) {
			provenance.push(job.bossName);
		}
		if (result.totalSims) {
			provenance.push(`${result.totalSims} sims`);
		}
		if (result.elapsedSeconds) {
			provenance.push(`${formatNumber(result.elapsedSeconds)} s`);
		}
		this.detailBody.appendChild(newElement('div', 'optimizer-provenance', provenance.join(' · ')));
		const warnings = [...result.warnings];
		if (result.calibrationGap) {
			const gap = `Full raid vs the derived buffs differ by ${formatNumber(result.calibrationGap * 100)}%.`;
			warnings.push(Math.abs(result.calibrationGap) > 0.03 ? `${gap} That's over 3%, so treat raid numbers with care.` : gap);
		}
		warnings.forEach(w => this.detailBody.appendChild(newElement('div', 'optimizer-warning', w)));

		const name = job.raider;
		if (result.improved) {
			this.detailBody.appendChild(
				newElement(
					'div',
					'optimizer-improved',
					beatsEquipped(result)
						? `Beats ${name}'s gear as equipped by more than the noise.`
						: `Doesn't beat ${name}'s gear as equipped by more than the noise, but this run can't keep that gear as it is (see the warnings above). ` +
								'This is the best set it found that it can.',
				),
			);
		} else {
			const trimmedAt = trimmedWhere(result, job.seedChanges.length > 0);
			const trimmed = trimmedAt ? `, minus what the phase left out (see ${trimmedAt})` : '';
			this.detailBody.appendChild(
				newElement(
					'div',
					'optimizer-improved optimizer-not-improved',
					`Nothing beat ${name}'s gear by more than the noise at ${effortName(result.settings?.effort ?? this.batch.settings.effort)} effort, ` +
						`so the set below is just that gear${trimmed}.`,
				),
			);
		}

		const note = newElement('span', 'optimizer-hint');
		const setName = `${name} P${job.phase} BiS`;
		actions.append(
			button(`Apply to ${name}`, 'btn-primary', () => {
				if (this.rosterChanged()) {
					return;
				}
				const eventID = TypedEvent.nextEventID();
				TypedEvent.freezeAllAndDo(() => this.apply(eventID, job));
				note.textContent = `${name} has it on in the raid sim now.`;
			}),
			button(`Save as "${setName}"`, 'btn-secondary', () => {
				if (this.rosterChanged()) {
					return;
				}
				note.textContent = this.save(job) ? `Saved under ${name}'s Gear Sets.` : "This browser wouldn't store it.";
			}),
			again,
			againHint,
			note,
		);
		this.detailBody.appendChild(actions);

		const view = new LoadoutView({
			player,
			displayStats: (getSpecConfig(player.spec) as IndividualSimUIConfig<any>).displayStats,
			tank: isTankSpec(player.spec),
			possessive: `${name}'s`,
			object: name,
		});
		const unit = scoreUnit(result.scoreStats);
		if (result.best) {
			this.detailBody.appendChild(view.renderLoadout(result.improved ? 'Best' : `${name}'s gear`, result.best, result.seed, unit));
		}
		if (result.alternatives.length > 0) {
			this.detailBody.appendChild(view.renderAlternatives(result.alternatives, result.best, result.improved, unit));
		}
		if (result.racialScreen.length > 0) {
			const table = newElement('table', 'table table-sm optimizer-table');
			table.appendChild(tableRow('th', ['Racial traits', unit.short ? `Score (${unit.short})` : 'Score', 'Finalist']));
			for (const screen of result.racialScreen) {
				table.appendChild(
					tableRow('td', [
						raceNames.get(screen.racialTraits) || '',
						`${formatNumber(screen.score)} ± ${formatNumber(screen.scoreSe)}`,
						screen.finalist ? 'yes' : '',
					]),
				);
			}
			this.detailBody.appendChild(section('Racial screen', table));
		}
		if (job.seedChanges.length > 0) {
			const list = newElement('ul', 'optimizer-list');
			job.seedChanges.forEach(change => list.appendChild(newElement('li', undefined, change)));
			this.detailBody.appendChild(
				section(`${name}'s gear, trimmed to the pool`, list, `The run started from ${name}'s gear without these: the phase rules them out.`),
			);
		}
	}

	private exportJson() {
		const done = [...this.batch.jobs.values()]
			.filter(job => job.state == 'done' && job.result)
			.sort((a, b) => a.phase - b.phase || a.raidIndex - b.raidIndex || a.stage - b.stage);
		const first = done[0]?.result;
		const batchExport = OptimizerBatchExport.create({
			simCommit: first?.simCommit || '',
			catalogDate: first?.catalogDate || '',
			exportedAt: new Date().toISOString(),
			rosterFingerprint: this.batch.fingerprint,
			entries: done.map(job =>
				OptimizerBatchEntry.create({ raider: job.raider, raidIndex: job.raidIndex, contentPhase: job.phase, stage: job.stage, result: job.result }),
			),
		});
		downloadString(OptimizerBatchExport.toJsonString(batchExport, { prettySpaces: 2 }), 'optimizer-batch.json');
	}
}
