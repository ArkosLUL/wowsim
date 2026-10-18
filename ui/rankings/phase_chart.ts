import { Chart, ChartDataset, Plugin, TooltipItem } from 'chart.js';

import { PHASE_LABELS } from '../core/constants/other';
import { classColors } from '../core/proto_utils/utils';
import { contentPhases, RankedBuild, Rankings } from './data';
import { rankBuilds, RankedRow, Role, roleOf } from './tiers';

export type ChartMode = 'dps' | 'rank';

export interface ChartState {
	phase: number;
	fight: number;
	roles: Set<Role>;
	mode: ChartMode;
}

type LineData = Array<number | null>;

// A class's builds share its colour, so the dash tells them apart, in the order the data lists them.
const DASHES: Array<Array<number>> = [[], [7, 4], [2, 3], [10, 3, 2, 3]];
const FADED_ALPHA = 0.15;
const GRID = 'rgba(255, 255, 255, 0.08)';
const AXIS = 'rgba(255, 255, 255, 0.25)';
const TICKS = 'rgba(255, 255, 255, 0.75)';
const SELECTED_PHASE = 'rgba(224, 163, 53, 0.12)';

const compact = new Intl.NumberFormat('en-US', { notation: 'compact', maximumFractionDigits: 1 });
const whole = new Intl.NumberFormat('en-US', { maximumFractionDigits: 0 });

function withAlpha(color: string, alpha: number): string {
	let hex = color.replace('#', '');
	if (hex.length == 3) {
		hex = [...hex].map(c => c + c).join('');
	}
	const value = parseInt(hex, 16);
	return `rgba(${(value >> 16) & 255}, ${(value >> 8) & 255}, ${value & 255}, ${alpha})`;
}

function shortPhase(phase: number): string {
	return (PHASE_LABELS[phase] ?? `P${phase}`).split(':')[0];
}

// One line per build across the content phases, at the selected fight length.
export class PhaseChart {
	readonly rootElem: HTMLElement;
	private readonly chart: Chart<'line', LineData, string>;
	private readonly builds: Array<RankedBuild>;
	private readonly phases: Array<number>;
	private readonly colors: Array<string>;
	private readonly dashes: Array<Array<number>>;
	// the page background as "r, g, b", for the rings around the points
	private readonly surface: string;
	private readonly legendItems = new Map<string, HTMLElement>();
	// rows[build][phase index], undefined where the build has no result
	private rows: Array<Array<RankedRow | undefined>> = [];
	private state: ChartState | null = null;
	private highlighted: string | null = null;

	constructor(parent: HTMLElement, rankings: Rankings) {
		this.builds = rankings.builds;
		this.phases = contentPhases(rankings);
		this.colors = this.builds.map(build => classColors[build.class]);
		const perClass = new Map<number, number>();
		this.dashes = this.builds.map(build => {
			const n = perClass.get(build.class) ?? 0;
			perClass.set(build.class, n + 1);
			return DASHES[n % DASHES.length];
		});
		this.surface = getComputedStyle(parent).getPropertyValue('--theme-background-color').trim();

		this.rootElem = document.createElement('div');
		this.rootElem.classList.add('rankings-chart-body');
		parent.appendChild(this.rootElem);

		const plot = document.createElement('div');
		plot.classList.add('rankings-chart-plot');
		this.rootElem.appendChild(plot);
		const canvas = document.createElement('canvas');
		canvas.setAttribute('role', 'img');
		canvas.setAttribute('aria-label', 'DPS of every build across the content phases');
		plot.appendChild(canvas);
		canvas.addEventListener('mouseleave', () => this.highlight(null));

		this.rootElem.appendChild(this.buildLegend());

		const selectedPhase: Plugin<'line'> = {
			id: 'rankingsSelectedPhase',
			beforeDatasetsDraw: chart => this.drawSelectedPhase(chart),
		};

		this.chart = new Chart<'line', LineData, string>(canvas, {
			type: 'line',
			data: {
				labels: this.phases.map(shortPhase),
				datasets: this.builds.map((build, i) => this.dataset(build, i)),
			},
			plugins: [selectedPhase],
			options: {
				responsive: true,
				maintainAspectRatio: false,
				animation: false,
				interaction: { mode: 'nearest', intersect: false, axis: 'xy' },
				onHover: (event, _elements, chart) => {
					if (event.type == 'mouseout' || !event.native) {
						this.highlight(null);
						return;
					}
					const hit = chart.getElementsAtEventForMode(event.native, 'nearest', { intersect: false, axis: 'xy' }, false)[0];
					this.highlight(hit ? this.builds[hit.datasetIndex].key : null);
				},
				scales: {
					x: {
						offset: true,
						grid: { display: false },
						border: { color: AXIS },
						ticks: { color: TICKS },
					},
					y: {
						grid: { color: GRID },
						border: { color: AXIS },
						ticks: { color: TICKS, precision: 0 },
					},
				},
				plugins: {
					legend: { display: false },
					tooltip: {
						usePointStyle: true,
						callbacks: {
							title: items => (items.length ? this.builds[items[0].datasetIndex].name : ''),
							label: item => this.tooltipLabel(item),
							labelPointStyle: () => ({ pointStyle: 'line', rotation: 0 }),
						},
					},
				},
			},
		});
	}

	private dataset(build: RankedBuild, i: number): ChartDataset<'line', LineData> {
		return {
			label: build.name,
			data: this.phases.map(() => null),
			borderColor: this.colors[i],
			backgroundColor: this.colors[i],
			borderWidth: 2,
			borderDash: this.dashes[i],
			borderCapStyle: 'round',
			borderJoinStyle: 'round',
			pointRadius: 4,
			pointHoverRadius: 5,
			// scriptable, since Chart.js caches plain point options and a highlight wouldn't reach them
			pointBackgroundColor: () => this.color(i),
			pointBorderColor: () => `rgba(${this.surface}, ${this.isFaded(i) ? FADED_ALPHA : 1})`,
			pointBorderWidth: 2,
			tension: 0,
		};
	}

	private buildLegend(): HTMLElement {
		const legend = document.createElement('ul');
		legend.classList.add('rankings-legend');
		const order = this.builds.map((_, i) => i).sort((a, b) => this.builds[a].name.localeCompare(this.builds[b].name));
		for (const i of order) {
			const build = this.builds[i];
			const item = document.createElement('li');
			item.classList.add('rankings-legend-item');
			item.dataset.build = build.key;
			item.tabIndex = 0;
			item.appendChild(this.lineKey(i));
			const name = document.createElement('span');
			name.textContent = build.name;
			item.appendChild(name);
			item.addEventListener('mouseenter', () => this.highlight(build.key));
			item.addEventListener('mouseleave', () => this.highlight(null));
			item.addEventListener('focus', () => this.highlight(build.key));
			item.addEventListener('blur', () => this.highlight(null));
			legend.appendChild(item);
			this.legendItems.set(build.key, item);
		}
		return legend;
	}

	private lineKey(i: number): SVGElement {
		const ns = 'http://www.w3.org/2000/svg';
		const svg = document.createElementNS(ns, 'svg');
		svg.setAttribute('width', '28');
		svg.setAttribute('height', '10');
		svg.setAttribute('aria-hidden', 'true');
		const line = document.createElementNS(ns, 'line');
		line.setAttribute('x1', '2');
		line.setAttribute('x2', '26');
		line.setAttribute('y1', '5');
		line.setAttribute('y2', '5');
		line.setAttribute('stroke', this.colors[i]);
		line.setAttribute('stroke-width', '2');
		line.setAttribute('stroke-linecap', 'round');
		if (this.dashes[i].length) {
			line.setAttribute('stroke-dasharray', this.dashes[i].join(' '));
		}
		svg.appendChild(line);
		return svg;
	}

	private tooltipLabel(item: TooltipItem<'line'>): string {
		const row = this.rows[item.datasetIndex]?.[item.dataIndex];
		if (!row) {
			return '';
		}
		return `${shortPhase(this.phases[item.dataIndex])}: ${whole.format(row.stats.dps)} DPS, #${row.rank} of ${this.rankedCount(item.dataIndex)}`;
	}

	private rankedCount(phaseIndex: number): number {
		return this.rows.filter(rows => rows[phaseIndex]).length;
	}

	private drawSelectedPhase(chart: Chart<'line'>) {
		const index = this.state ? this.phases.indexOf(this.state.phase) : -1;
		const x = chart.scales.x;
		if (index < 0 || !x) {
			return;
		}
		const width = x.width / Math.max(1, this.phases.length);
		const area = chart.chartArea;
		const ctx = chart.ctx;
		ctx.save();
		ctx.fillStyle = SELECTED_PHASE;
		ctx.fillRect(x.getPixelForValue(index) - width / 2, area.top, width, area.bottom - area.top);
		ctx.restore();
	}

	update(state: ChartState) {
		this.state = state;
		const byPhase = this.phases.map(phase => new Map(rankBuilds(this.builds, phase, state.fight).map(row => [row.build.key, row])));
		this.rows = this.builds.map(build => byPhase.map(rows => rows.get(build.key)));

		this.chart.data.datasets.forEach((dataset, i) => {
			dataset.data = this.rows[i].map(row => (row ? (state.mode == 'dps' ? row.stats.dps : row.rank) : null));
			const role = roleOf(this.builds[i].spec);
			dataset.hidden = !role || !state.roles.has(role);
		});
		this.legendItems.forEach((item, key) => {
			const build = this.builds.find(b => b.key == key)!;
			const role = roleOf(build.spec);
			item.hidden = !role || !state.roles.has(role);
		});

		const y = this.chart.options.scales!.y!;
		y.reverse = state.mode == 'rank';
		y.min = state.mode == 'rank' ? 1 : undefined;
		y.ticks!.callback = (value: string | number) => (state.mode == 'rank' ? `#${value}` : compact.format(Number(value)));
		this.applyHighlight();
		this.chart.update();
	}

	// Brings one build's line forward and fades the rest; null shows them all evenly.
	highlight(key: string | null) {
		if (key == this.highlighted) {
			return;
		}
		this.highlighted = key;
		this.applyHighlight();
		this.chart.update('none');
	}

	private isFaded(i: number): boolean {
		return this.highlighted != null && this.builds[i].key != this.highlighted;
	}

	private color(i: number): string {
		return this.isFaded(i) ? withAlpha(this.colors[i], FADED_ALPHA) : this.colors[i];
	}

	private applyHighlight() {
		this.chart.data.datasets.forEach((dataset, i) => {
			const on = this.builds[i].key == this.highlighted;
			dataset.borderColor = this.color(i);
			dataset.borderWidth = on ? 3 : 2;
			dataset.order = on ? 0 : 1;
		});
		this.legendItems.forEach((item, key) => item.classList.toggle('faded', this.highlighted != null && key != this.highlighted));
	}
}
