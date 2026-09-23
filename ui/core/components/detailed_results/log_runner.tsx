// eslint-disable-next-line @typescript-eslint/no-unused-vars
import { element, fragment } from 'tsx-vanilla';

import { Entity, SimLog } from '../../proto_utils/logs_parser.js';
import { SimResult } from '../../proto_utils/sim_result.js';
import { TypedEvent } from '../../typed_event.js';
import { BooleanPicker } from '../boolean_picker.js';
import { ResultComponent, ResultComponentConfig, SimResultData } from './result_component.js';

interface LogRow {
	log: SimLog;
	elem: HTMLElement;
	text: string;
	isDebug: boolean;
}

const SEARCH_DELAY_MS = 250;

export class LogRunner extends ResultComponent {
	private logsContainer: HTMLElement;
	private searchInput: HTMLInputElement;

	private showDebug = false;
	private search = '';

	// Built once per result, since a raid's log runs to tens of thousands of lines. The filters,
	// search and debug toggle only pick which rows show.
	private rows: Array<LogRow> = [];
	private rowsFor: SimResult | null = null;
	// Rows are only built while the log tab is open, and the first change after it closes takes them
	// off the page: that many elements slow every other tab down.
	private tabOpen = false;

	readonly showDebugChangeEmitter = new TypedEvent<void>('Show Debug');

	constructor(config: ResultComponentConfig) {
		config.rootCssClass = 'log-runner-root';
		super(config);

		this.rootElem.appendChild(
			<>
				<div className="log-runner-controls">
					<input className="log-search-input form-control" type="search" placeholder="Search the log"></input>
					<div className="show-debug-container"></div>
				</div>
				<table className="metrics-table log-runner-table">
					<thead>
						<tr className="metrics-table-header-row">
							<th>Time</th>
							<th>
								<div className="d-flex align-items-end">Event</div>
							</th>
						</tr>
					</thead>
					<tbody className="log-runner-logs"></tbody>
				</table>
			</>,
		);
		this.logsContainer = this.rootElem.querySelector('.log-runner-logs')!;
		this.searchInput = this.rootElem.querySelector('.log-search-input')!;

		new BooleanPicker<LogRunner>(this.rootElem.querySelector('.show-debug-container')!, this, {
			extraCssClasses: ['show-debug-picker'],
			label: 'Show Debug Statements',
			inline: true,
			reverse: true,
			changedEvent: () => this.showDebugChangeEmitter,
			getValue: () => this.showDebug,
			setValue: (eventID, _logRunner, newValue) => {
				this.showDebug = newValue;
				this.showDebugChangeEmitter.emit(eventID);
			},
		});
		this.showDebugChangeEmitter.on(() => this.render());

		let searchTimeout: ReturnType<typeof setTimeout> | undefined;
		this.searchInput.addEventListener('input', () => {
			clearTimeout(searchTimeout);
			searchTimeout = setTimeout(() => {
				this.search = this.searchInput.value.trim().toLowerCase();
				this.render();
			}, SEARCH_DELAY_MS);
		});
	}

	// Closing leaves the rows up, so the tab doesn't fade out empty.
	setTabOpen(open: boolean) {
		this.tabOpen = open;
		if (open) {
			this.render();
		}
	}

	onSimResult(_resultData: SimResultData): void {
		this.render();
	}

	private render() {
		if (!this.hasLastSimResult()) {
			return;
		}
		const resultData = this.getLastSimResult();
		if (!this.tabOpen) {
			this.logsContainer.replaceChildren();
			if (resultData.result !== this.rowsFor) {
				this.rows = [];
				this.rowsFor = null;
			}
			return;
		}
		if (resultData.result !== this.rowsFor) {
			this.rows = this.buildRows(resultData.result);
			this.rowsFor = resultData.result;
		}
		const isAbout = this.unitMatcher(resultData);

		const shown = document.createDocumentFragment();
		this.rows
			.filter(row => (this.showDebug || !row.isDebug) && isAbout(row.log) && (!this.search || row.text.includes(this.search)))
			.forEach(row => shown.appendChild(row.elem));
		this.logsContainer.replaceChildren(shown);
	}

	private buildRows(result: SimResult): Array<LogRow> {
		return result.logs
			.filter(log => !log.isCastCompleted())
			.filter(log => log.raw.length > 0)
			.map(log => {
				const elem = (
					<tr>
						<td className="log-timestamp">{log.formattedTimestamp()}</td>
						<td className="log-event">{this.newEventFrom(log)}</td>
					</tr>
				) as HTMLElement;
				return {
					log,
					elem,
					text: elem.textContent!.toLowerCase(),
					isDebug: log.raw.includes('[DEBUG]'),
				};
			});
	}

	// Lines where the picked raider (or one of their pets) and the picked target take part, as source
	// or target.
	private unitMatcher(resultData: SimResultData): (log: SimLog) => boolean {
		const { result, filter } = resultData;
		const player = filter.player != null ? result.getPlayerWithIndex(filter.player) : null;
		const target = filter.target != null ? result.getTargetWithIndex(filter.target) : null;

		const involves = (log: SimLog, matches: (entity: Entity) => boolean) =>
			(log.source != null && matches(log.source)) || (log.target != null && matches(log.target));

		return log =>
			(player == null || involves(log, entity => !entity.isTarget && entity.index == player.index)) &&
			(target == null || involves(log, entity => entity.isTarget && entity.index == target.index));
	}

	private newEventFrom(log: SimLog): Element {
		const eventString = log.toString(false).trim();
		const wrapper = <span></span>;
		wrapper.innerHTML = eventString;
		return wrapper;
	}
}
