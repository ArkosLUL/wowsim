import tippy, { Instance } from 'tippy.js';
// eslint-disable-next-line @typescript-eslint/no-unused-vars
import { element, fragment } from 'tsx-vanilla';

import { Component } from '../core/components/component';
import { CURRENT_PHASE, PHASE_LABELS, REPO_NAME } from '../core/constants/other';
import { classColors, getTalentTreeIcon, specNames } from '../core/proto_utils/utils';
import { buildPhase, contentPhases, loadRankings, Rankings, simLink } from './data';
import { ChartMode, PhaseChart } from './phase_chart';
import { rankBuilds, RankedRow, Role, ROLE_LABELS, roleOf, ROLES, TIERS } from './tiers';

const DEFAULT_FIGHT = 180;

interface PageState {
	phase: number;
	fight: number;
	roles: Set<Role>;
}

const whole = new Intl.NumberFormat('en-US', { maximumFractionDigits: 0 });

function fightLabel(seconds: number): string {
	return seconds % 60 == 0 ? `${seconds / 60} min` : `${seconds} s`;
}

function phaseParts(phase: number): [string, string] {
	const label = PHASE_LABELS[phase] ?? `P${phase}`;
	const colon = label.indexOf(':');
	return colon < 0 ? [label, ''] : [label.slice(0, colon), label.slice(colon)];
}

function shortCommit(commit: string): string {
	return commit.slice(0, 9) + (commit.endsWith('-dirty') ? '-dirty' : '');
}

export class RankingsPage extends Component {
	private readonly controlsElem: HTMLElement;
	private readonly headingElem: HTMLElement;
	private readonly tiersElem: HTMLElement;
	private readonly chartSection: HTMLElement;
	private readonly footerElem: HTMLElement;

	private rankings: Rankings | null = null;
	private phases: Array<number> = [];
	private state: PageState = { phase: CURRENT_PHASE, fight: DEFAULT_FIGHT, roles: new Set(ROLES) };
	private chartMode: ChartMode = 'dps';
	private chart: PhaseChart | null = null;
	private tooltips: Array<Instance> = [];

	constructor(parent: HTMLElement) {
		super(parent, 'rankings-root');

		this.rootElem.appendChild(
			<>
				<div className="sim-bg"></div>
				<header className="rankings-header">
					<a className="sim-link rankings-home" href={`/${REPO_NAME}/`}>
						<div className="sim-link-content">
							<img className="sim-link-icon" src={`/${REPO_NAME}/assets/img/WoW-Simulator-Icon.png`} alt="" />
							<div className="d-flex flex-column">
								<span className="sim-link-label">WoWSims - WOTLK</span>
								<h1 className="sim-link-title">DPS Rankings</h1>
							</div>
						</div>
					</a>
					<p className="rankings-intro">
						Every DPS build at its best in slot gear for the content phase, simmed in the same raid against the same boss.
					</p>
				</header>
				<div className="rankings-controls"></div>
				<main className="rankings-main">
					<h2 className="rankings-heading"></h2>
					<div className="rankings-tiers" aria-live="polite">
						<p className="rankings-message">Loading the rankings…</p>
					</div>
					<section className="rankings-chart" hidden></section>
				</main>
				<footer className="rankings-footer"></footer>
			</>,
		);
		this.controlsElem = this.rootElem.querySelector('.rankings-controls')!;
		this.headingElem = this.rootElem.querySelector('.rankings-heading')!;
		this.tiersElem = this.rootElem.querySelector('.rankings-tiers')!;
		this.chartSection = this.rootElem.querySelector('.rankings-chart')!;
		this.footerElem = this.rootElem.querySelector('.rankings-footer')!;

		loadRankings()
			.then(rankings => this.init(rankings))
			.catch(error => {
				console.error(error);
				this.tiersElem.replaceChildren(<p className="rankings-message">{String(error.message ?? error)}</p>);
			});
	}

	private init(rankings: Rankings) {
		this.rankings = rankings;
		this.phases = contentPhases(rankings);
		this.state = this.readUrl();
		this.buildControls();
		this.buildChart();
		this.render();
	}

	private readUrl(): PageState {
		const params = new URLSearchParams(window.location.search);
		const fights = this.rankings!.fights;

		let phase = Number(params.get('phase'));
		if (!this.phases.includes(phase)) {
			phase = this.phases.includes(CURRENT_PHASE) ? CURRENT_PHASE : this.phases[this.phases.length - 1];
		}
		let fight = Number(params.get('fight'));
		if (!fights.includes(fight)) {
			fight = fights.includes(DEFAULT_FIGHT) ? DEFAULT_FIGHT : fights[0];
		}
		const asked = (params.get('role') ?? '').split(',').filter((r): r is Role => ROLES.includes(r as Role));
		return { phase, fight, roles: new Set(asked.length ? asked : ROLES) };
	}

	private writeUrl() {
		const params = new URLSearchParams(window.location.search);
		params.set('phase', String(this.state.phase));
		params.set('fight', String(this.state.fight));
		if (this.state.roles.size == ROLES.length) {
			params.delete('role');
		} else {
			params.set('role', ROLES.filter(r => this.state.roles.has(r)).join(','));
		}
		window.history.replaceState(null, '', `${window.location.pathname}?${params.toString()}`);
	}

	private change(update: Partial<PageState>) {
		this.state = { ...this.state, ...update };
		this.writeUrl();
		this.render();
	}

	private buildControls() {
		const phaseTabs = (
			<nav className="rankings-phases nav nav-tabs" attributes={{ 'aria-label': 'Content phase' }}>
				{this.phases.map(phase => {
					const [short, name] = phaseParts(phase);
					return (
						<button className="nav-link" type="button" dataset={{ phase: String(phase) }} title={PHASE_LABELS[phase] ?? short}>
							<span className="rankings-phase-short">{short}</span>
							<span className="rankings-phase-name">{name}</span>
						</button>
					);
				})}
			</nav>
		);
		phaseTabs
			.querySelectorAll<HTMLElement>('[data-phase]')
			.forEach(button => button.addEventListener('click', () => this.change({ phase: Number(button.dataset.phase) })));

		const fightButtons = (
			<div className="btn-group" attributes={{ role: 'group', 'aria-label': 'Fight length' }}>
				{this.rankings!.fights.map(fight => (
					<button className="btn btn-outline-primary" type="button" dataset={{ fight: String(fight) }}>
						{fightLabel(fight)}
					</button>
				))}
			</div>
		);
		fightButtons
			.querySelectorAll<HTMLElement>('[data-fight]')
			.forEach(button => button.addEventListener('click', () => this.change({ fight: Number(button.dataset.fight) })));

		const roleButtons = (
			<div className="btn-group" attributes={{ role: 'group', 'aria-label': 'Roles shown' }}>
				{ROLES.map(role => (
					<button className="btn btn-outline-primary" type="button" dataset={{ role }}>
						{ROLE_LABELS[role]}
					</button>
				))}
			</div>
		);
		roleButtons.querySelectorAll<HTMLElement>('[data-role]').forEach(button =>
			button.addEventListener('click', () => {
				const role = button.dataset.role as Role;
				const roles = new Set(this.state.roles);
				if (roles.has(role)) {
					// keep at least one role on, or the page shows nothing
					if (roles.size == 1) {
						return;
					}
					roles.delete(role);
				} else {
					roles.add(role);
				}
				this.change({ roles });
			}),
		);

		this.controlsElem.replaceChildren(
			phaseTabs,
			<div className="rankings-filters">
				<div className="rankings-filter">
					<span className="rankings-filter-label">Fight</span>
					{fightButtons}
				</div>
				<div className="rankings-filter">
					<span className="rankings-filter-label">Roles</span>
					{roleButtons}
				</div>
			</div>,
		);
	}

	private buildChart() {
		const modeButtons = (
			<div className="btn-group" attributes={{ role: 'group', 'aria-label': 'Chart values' }}>
				<button className="btn btn-outline-primary btn-sm" type="button" dataset={{ mode: 'dps' }}>
					DPS
				</button>
				<button className="btn btn-outline-primary btn-sm" type="button" dataset={{ mode: 'rank' }}>
					Rank
				</button>
			</div>
		);
		modeButtons.querySelectorAll<HTMLElement>('[data-mode]').forEach(button =>
			button.addEventListener('click', () => {
				this.chartMode = button.dataset.mode as ChartMode;
				this.render();
			}),
		);

		this.chartSection.replaceChildren(
			<div className="rankings-chart-header">
				<div>
					<h2>Across content phases</h2>
					<p className="rankings-chart-note">At the selected fight length. The shaded column is the content phase above.</p>
				</div>
				{modeButtons}
			</div>,
		);
		this.chart = new PhaseChart(this.chartSection, this.rankings!);
		this.chartSection.hidden = false;
	}

	private render() {
		const { phase, fight, roles } = this.state;
		this.setPressed('[data-phase]', b => Number(b.dataset.phase) == phase);
		this.setPressed('[data-fight]', b => Number(b.dataset.fight) == fight);
		this.setPressed('[data-role]', b => roles.has(b.dataset.role as Role));
		this.setPressed('[data-mode]', b => b.dataset.mode == this.chartMode);

		this.headingElem.replaceChildren(
			<span className="rankings-heading-phase">{PHASE_LABELS[phase] ?? `P${phase}`}</span>,
			<span className="rankings-heading-fight">{`${fightLabel(fight)} fight`}</span>,
		);
		this.renderTiers();
		this.renderFooter();
		this.chart?.update({ phase, fight, roles, mode: this.chartMode });
	}

	private setPressed(selector: string, isOn: (button: HTMLElement) => boolean) {
		this.rootElem.querySelectorAll<HTMLElement>(selector).forEach(button => {
			const on = isOn(button);
			button.classList.toggle('active', on);
			button.setAttribute(button.classList.contains('nav-link') ? 'aria-current' : 'aria-pressed', String(on));
		});
	}

	private renderTiers() {
		this.tooltips.forEach(t => t.destroy());
		this.tooltips = [];

		const { phase, fight, roles } = this.state;
		const ranked = rankBuilds(this.rankings!.builds, phase, fight);
		// shared by every row, whisker included, so no bar runs past its column
		const scaleMax = Math.max(1, ...ranked.map(row => row.stats.dps + row.stats.stdev));
		const shown = ranked.filter(row => {
			const role = roleOf(row.build.spec);
			return role !== undefined && roles.has(role);
		});

		if (!shown.length) {
			this.tiersElem.replaceChildren(<p className="rankings-message">No results for this content phase and fight.</p>);
			return;
		}

		const bands = TIERS.map(tier => ({ tier, rows: shown.filter(row => row.tier == tier.letter) })).filter(band => band.rows.length);
		this.tiersElem.replaceChildren(
			...bands.map(({ tier, rows }) => (
				<section className="rankings-tier" dataset={{ tier: tier.letter }} attributes={{ 'aria-label': `${tier.letter} tier` }}>
					<div className="rankings-tier-badge">
						<span className="rankings-tier-letter">{tier.letter}</span>
						<span className="rankings-tier-range">{tier.min > 0 ? `≥ ${tier.min}%` : `< ${TIERS[TIERS.length - 2].min}%`}</span>
					</div>
					<ol className="rankings-rows">{rows.map(row => this.renderRow(row, ranked.length, scaleMax))}</ol>
				</section>
			)),
		);
	}

	private renderRow(row: RankedRow, total: number, scaleMax: number): HTMLElement {
		const { build, stats } = row;
		const { fight } = this.state;
		const phase = buildPhase(build, this.state.phase)!;
		const color = classColors[build.class];
		const across = (value: number) => Math.max(0, Math.min(100, (100 * value) / scaleMax));
		const low = across(stats.dps - stats.stdev);
		const high = across(stats.dps + stats.stdev);

		const link = (
			<a
				className="rankings-row-link"
				href={simLink(build, phase, fight)}
				target="_blank"
				rel="noopener"
				attributes={{ 'aria-label': `Open ${build.name} in the ${specNames[build.spec]} sim` }}>
				<i className="fas fa-external-link-alt" attributes={{ 'aria-hidden': 'true' }}></i>
			</a>
		);
		const elem = (
			<li className="rankings-row" dataset={{ build: build.key }} tabIndex={0}>
				<span className="rankings-row-rank">{String(row.rank)}</span>
				<img className="rankings-row-icon" src={getTalentTreeIcon(build.spec, build.talents)} alt="" />
				<span className="rankings-row-name">{build.name}</span>
				<div className="rankings-row-bar" style={{ '--class-color': color }}>
					<div className="rankings-bar" style={{ width: `${across(stats.dps)}%` }}></div>
					<div className="rankings-whisker" style={{ left: `${low}%`, width: `${high - low}%` }}></div>
				</div>
				<span className="rankings-row-dps">{whole.format(stats.dps)}</span>
				<span className="rankings-row-pct">{`${row.percent.toFixed(1)}%`}</span>
				{link}
			</li>
		) as HTMLElement;

		const details = (
			<div className="rankings-tooltip">
				<div className="rankings-tooltip-title">{`${build.name}, ${fightLabel(fight)} fight`}</div>
				<dl>
					<dt>Average</dt>
					<dd>{`${whole.format(stats.dps)} DPS`}</dd>
					<dt>Spread (±1σ)</dt>
					<dd>{`±${whole.format(stats.stdev)}`}</dd>
					<dt>Min</dt>
					<dd>{whole.format(stats.min)}</dd>
					<dt>Max</dt>
					<dd>{whole.format(stats.max)}</dd>
				</dl>
				<div className="rankings-tooltip-note">{`#${row.rank} of ${total}, ${row.percent.toFixed(1)}% of the top build`}</div>
			</div>
		);
		const triggers = [...elem.children].filter(child => child !== link);
		const rowTooltip = tippy(elem, { content: details, placement: 'top', triggerTarget: triggers });
		this.tooltips.push(
			rowTooltip,
			tippy(link, {
				content: `Opens this exact setup in the ${specNames[build.spec]} sim, in a new tab. It replaces that sim's saved settings in this browser.`,
				placement: 'left',
			}),
		);

		elem.addEventListener('mouseenter', () => this.chart?.highlight(build.key));
		elem.addEventListener('mouseleave', () => this.chart?.highlight(null));
		elem.addEventListener('focus', () => {
			this.chart?.highlight(build.key);
			// a click focuses the row too, and the hover tooltip covers that
			if (elem.matches(':focus-visible')) {
				rowTooltip.show();
			}
		});
		elem.addEventListener('blur', () => {
			this.chart?.highlight(null);
			rowTooltip.hide();
		});
		return elem;
	}

	private renderFooter() {
		const r = this.rankings!;
		const commit = <span title={r.simCommit}>{`sim ${shortCommit(r.simCommit)}`}</span>;
		this.footerElem.replaceChildren(
			<p>
				{commit}
				{` · simmed ${r.generatedAt.slice(0, 10)} · item catalog ${r.catalogDate} · ${whole.format(r.iterations)} iterations`}
				{` · ${fightLabel(this.state.fight)} fight (±5 s) against one level 83 boss`}
				{' · full raid buffs and debuffs, a Draenei in every party, no single-target gifts'}
			</p>,
		);
	}
}
