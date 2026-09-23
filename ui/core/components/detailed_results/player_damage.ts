import { UnitMetrics, SimResult, SimResultFilter } from '../../proto_utils/sim_result.js';
import { sum } from '../../utils.js';

import { ColumnSortType, MetricsTable } from './metrics_table.js';
import { ResultComponent, ResultComponentConfig, SimResultData } from './result_component.js';
import { ResultsFilter } from './results_filter.js';
import { SourceChart } from './source_chart.js';
import tippy from 'tippy.js';

export class PlayerDamageMetricsTable extends MetricsTable<UnitMetrics> {
	private readonly resultsFilter: ResultsFilter;

	// Cached values from most recent result.
	private filter: SimResultFilter = {};
	private raidDps: number;
	private maxDps: number;

	constructor(config: ResultComponentConfig, resultsFilter: ResultsFilter) {
		config.rootCssClass = 'player-damage-metrics-root';
		super(config, [
			MetricsTable.playerNameCellConfig(),
			{
				name: 'Amount',
				tooltip: 'Player Damage / Raid Damage',
				headerCellClass: 'amount-header-cell',
				fillCell: (player: UnitMetrics, cellElem: HTMLElement, rowElem: HTMLElement) => {
					cellElem.classList.add('amount-cell');
					const filter = this.filter;
					const dps = player.getDps(filter);

					// Sized up front so the tooltip can be placed before the chart draws into it.
					const chartContainer = document.createElement('div');
					chartContainer.style.width = '600px';
					chartContainer.style.height = '400px';
					let charted = false;

					tippy(rowElem, {
						content: chartContainer,
						placement: 'bottom',
						ignoreAttributes: true,
						// Chart.js sizes itself from its container, so it can only draw once the
						// tooltip has put the container on the page.
						onShown: () => {
							if (!charted) {
								charted = true;
								new SourceChart(chartContainer, player.actions.map(action => action.forTarget(filter)));
							}
						},
					});

					cellElem.innerHTML = `
						<div class="player-damage-percent">
							<span>${(this.raidDps ? (dps / this.raidDps) * 100 : 0).toFixed(2)}%</span>
						</div>
						<div class="player-damage-bar-container">
							<div class="player-damage-bar bg-${player.classColor}" style="width:${this.maxDps ? (dps / this.maxDps) * 100 : 0}%"></div>
						</div>
						<div class="player-damage-total">
							<span>${(player.getTotalDamage(filter) / 1000).toFixed(1)}k</span>
						</div>
					`;
				},
			},
			{
				name: 'DPS',
				tooltip: 'Damage / Encounter Duration',
				sort: ColumnSortType.Descending,
				getValue: (metric: UnitMetrics) => metric.getDps(this.filter),
				getDisplayString: (metric: UnitMetrics) => metric.getDps(this.filter).toFixed(1),
			},
		]);
		this.resultsFilter = resultsFilter;
		this.raidDps = 0;
		this.maxDps = 0;
	}

	customizeRowElem(player: UnitMetrics, rowElem: HTMLElement) {
		rowElem.classList.add('player-damage-row');
		rowElem.addEventListener('click', event => {
			this.resultsFilter.setPlayer(this.getLastSimResult().eventID, player.unitIndex);
		});
	}

	getGroupedMetrics(resultData: SimResultData): Array<Array<UnitMetrics>> {
		const players = resultData.result.getPlayers(resultData.filter);
		this.filter = resultData.filter;

		const playerDps = players.map(player => player.getDps(this.filter));
		this.raidDps = resultData.filter.target == null ? resultData.result.raidMetrics.dps.avg : sum(playerDps);
		this.maxDps = Math.max(0, ...playerDps);

		return players.map(player => [player]);
	}
}
