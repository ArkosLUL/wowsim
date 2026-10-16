import { Spec } from '../../proto/common.js';

import { ResultComponent, ResultComponentConfig, SimResultData } from './result_component.js';
import { RaidSimResultsManager } from '../../components/raid_sim_action.js';

export class ToplineResults extends ResultComponent {
	constructor(config: ResultComponentConfig) {
		config.rootCssClass = 'topline-results-root';
		super(config);

		this.rootElem.classList.add('results-sim');
	}

	onSimResult(resultData: SimResultData) {
		let content = RaidSimResultsManager.makeToplineResultsContent(resultData.result, resultData.filter);

		const noManaSpecs = [
			Spec.SpecFeralTankDruid,
			Spec.SpecRogue,
			Spec.SpecWarrior,
			Spec.SpecProtectionWarrior,
		];

		const players = resultData.result.getPlayers(resultData.filter);
		if (players.length == 1 && !noManaSpecs.includes(players[0].spec)) {
			const player = players[0];
			const secondsOOM = player.secondsOomAvg;
			const percentOOM = secondsOOM / resultData.result.encounterMetrics.durationSeconds;
			const dangerLevel = percentOOM < 0.01 ? 'safe' : (percentOOM < 0.05 ? 'warning' : 'danger');

			content += `
				<div class="results-sim-percent-oom ${dangerLevel} damage-metrics">
					<span class="topline-result-avg">${secondsOOM.toFixed(1)}s</span>
				</div>
			`;
		}

		this.rootElem.innerHTML = content;

		// only a tank with a healing model has one
		const margin = players.length == 1 && players[0].getTargetIndex(resultData.filter) == null ? players[0].healthMargin : undefined;
		if (margin) {
			const row = document.createElement('div');
			row.className = 'results-metric results-sim-health-margin threat-metrics';
			row.innerHTML = `
				<span class="topline-result-avg">${Math.round(margin.avg)}</span>
				<span class="topline-result-stdev">health margin (<i class="fas fa-plus-minus fa-xs"></i>${margin.stdev.toFixed()})</span>
			`;
			const chanceOfDeath = this.rootElem.querySelector('.results-sim-cod');
			if (chanceOfDeath) {
				chanceOfDeath.after(row);
			} else {
				this.rootElem.appendChild(row);
			}
		}
	}
}
