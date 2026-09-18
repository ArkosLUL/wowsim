import { REPO_NAME } from './constants/other.js'
import { GemColor } from './proto/common.js';
import { ItemQuality } from './proto/common.js';
import { ItemSlot } from './proto/common.js';
import { ItemSpec } from './proto/common.js';
import { ItemType } from './proto/common.js';
import { Stat } from './proto/common.js';

import { ComputeStatsRequest, ComputeStatsResult } from './proto/api.js';
import { RaidSimRequest, RaidSimResult, ProgressMetrics } from './proto/api.js';
import { StatWeightsRequest, StatWeightsResult } from './proto/api.js';
import { BulkSimRequest, BulkSimResult, OptimizeGearRequest } from './proto/api.js';
import { OptimizerResult } from './proto/optimizer.js';

import { wait } from './utils.js';

const SIM_WORKER_URL = `/${REPO_NAME}/sim_worker.js`;

export class WorkerPool {
	private workers: Array<SimWorker>;

	constructor(numWorkers: number) {
		this.workers = [];
		for (let i = 0; i < numWorkers; i++) {
			this.workers.push(new SimWorker());
		}
	}

	private getLeastBusyWorker(): SimWorker {
		return this.workers.reduce(
			(curMinWorker, nextWorker) => curMinWorker.numTasksRunning < nextWorker.numTasksRunning ?
				curMinWorker : nextWorker);
	}

	async makeApiCall(requestName: string, request: Uint8Array): Promise<Uint8Array> {
		return await this.getLeastBusyWorker().doApiCall(requestName, request, "");
	}

	async computeStats(request: ComputeStatsRequest): Promise<ComputeStatsResult> {
		const result = await this.makeApiCall('computeStats', ComputeStatsRequest.toBinary(request));
		return ComputeStatsResult.fromBinary(result);
	}

	async statWeightsAsync(request: StatWeightsRequest, onProgress: Function): Promise<StatWeightsResult> {
		console.log('Stat weights request: ' + StatWeightsRequest.toJsonString(request));
		const worker = this.getLeastBusyWorker();
		const id = worker.makeTaskId();
		// Add handler for the progress events
		worker.addPromiseFunc(id + "progress", this.newProgressHandler(id, worker, onProgress), (err) => { })

		// Now start the async sim
		const resultData = await worker.doApiCall('statWeightsAsync', StatWeightsRequest.toBinary(request), id);
		const result = ProgressMetrics.fromBinary(resultData)
		console.log('Stat weights result: ' + StatWeightsResult.toJsonString(result.finalWeightResult!));
		return result.finalWeightResult!;
	}

	async bulkSimAsync(request: BulkSimRequest, onProgress: Function): Promise<BulkSimResult> {
		console.log('bulk sim request: ' + BulkSimRequest.toJsonString(request, { enumAsInteger: true }));
		const worker = this.getLeastBusyWorker();
		const id = worker.makeTaskId();
		// Add handler for the progress events
		worker.addPromiseFunc(id + "progress", this.newProgressHandler(id, worker, onProgress), (err) => { })

		// Now start the async sim
		const resultData = await worker.doApiCall('bulkSimAsync', BulkSimRequest.toBinary(request), id);
		const result = ProgressMetrics.fromBinary(resultData)
		const resultJson = BulkSimResult.toJson(result.finalBulkResult!) as any;
		console.log('bulk sim result: ' + JSON.stringify(resultJson));
		return result.finalBulkResult!;
	}

	async raidSimAsync(request: RaidSimRequest, onProgress: Function): Promise<RaidSimResult> {
		console.log('Raid sim request: ' + RaidSimRequest.toJsonString(request));
		const worker = this.getLeastBusyWorker();
		const id = worker.makeTaskId();
		// Add handler for the progress events
		worker.addPromiseFunc(id + "progress", this.newProgressHandler(id, worker, onProgress), (err) => { })

		// Now start the async sim
		const resultData = await worker.doApiCall('raidSimAsync', RaidSimRequest.toBinary(request), id);
		const result = ProgressMetrics.fromBinary(resultData)

		// Don't print the logs because it just clogs the console.
		const resultJson = RaidSimResult.toJson(result.finalRaidResult!) as any;
		delete resultJson!['logs'];
		console.log('Raid sim result: ' + JSON.stringify(resultJson));
		return result.finalRaidResult!;
	}

	// The server runs one optimization at a time; a second one comes back with an errorResult.
	// Aborting signal cancels the run on the web server (not under wasm), and the promise still
	// resolves, with the result marked cancelled.
	async optimizeGearAsync(request: OptimizeGearRequest, onProgress: (progress: ProgressMetrics) => void, signal?: AbortSignal): Promise<OptimizerResult> {
		console.log('Optimize gear request: ' + OptimizeGearRequest.toJsonString(request, { enumAsInteger: true }));
		const worker = this.getLeastBusyWorker();
		const id = worker.makeTaskId();
		worker.addPromiseFunc(id + 'progress', this.newProgressHandler(id, worker, onProgress), console.error);

		// start the call before hooking up cancel, so the worker sees the task before any cancel for it
		const resultPromise = worker.doApiCall('optimizeGearAsync', OptimizeGearRequest.toBinary(request), id);
		const cancel = () => worker.cancelTask(id);
		if (signal?.aborted) {
			cancel();
		} else {
			signal?.addEventListener('abort', cancel, { once: true });
		}
		try {
			const result = ProgressMetrics.fromBinary(await resultPromise).finalOptimizeResult;
			if (!result) {
				throw new Error('The optimizer returned no result. The server may have rejected the request.');
			}
			console.log('Optimize gear result: ' + OptimizerResult.toJsonString(result, { enumAsInteger: true }));
			return result;
		} finally {
			signal?.removeEventListener('abort', cancel);
		}
	}

	newProgressHandler(id: string, worker: SimWorker, onProgress: Function): (progressData: any) => void {
		return (progressData: any) => {
			var progress = ProgressMetrics.fromBinary(progressData);
			onProgress(progress);
			// If we are done, stop adding the handler.
			if (progress.finalRaidResult != null || progress.finalWeightResult != null || progress.finalBulkResult != null || progress.finalOptimizeResult != null) {
				return;
			}

			worker.addPromiseFunc(id + "progress", this.newProgressHandler(id, worker, onProgress), (err) => { });
		};
	}
}

class SimWorker {
	numTasksRunning: number;
	private taskIdsToPromiseFuncs: Record<string, [(result: any) => void, (error: any) => void]>;
	private worker: Worker;
	private onReady: Promise<void>;

	constructor() {
		this.numTasksRunning = 0;
		this.taskIdsToPromiseFuncs = {};
		this.worker = new window.Worker(SIM_WORKER_URL);

		let resolveReady: (() => void) | null = null;
		this.onReady = new Promise((_resolve, _reject) => {
			resolveReady = _resolve;
		});

		this.worker.onmessage = event => {
			if (event.data.msg == 'ready') {
				this.worker.postMessage({ msg: 'setID', id: '1' });
				resolveReady!();
			} else if (event.data.msg == 'idconfirm') {
				// Do nothing
			} else {
				const id = event.data.id;
				if (!this.taskIdsToPromiseFuncs[id]) {
					console.warn('Unrecognized result id: ' + id);
					return;
				}

				const promiseFuncs = this.taskIdsToPromiseFuncs[id];
				delete this.taskIdsToPromiseFuncs[id];
				this.numTasksRunning--;

				promiseFuncs[0](event.data.outputData);
			}
		};
	}

	addPromiseFunc(id: string, callback: (result: any) => void, onError: (error: any) => void) {
		this.taskIdsToPromiseFuncs[id] = [callback, onError];
	}

	// Asks the worker to cancel an async task. No reply: the task ends with its own final result.
	// Waits on onReady like doApiCall does, so it can't overtake a start message still waiting there.
	async cancelTask(id: string) {
		await this.onReady;
		this.worker.postMessage({ msg: 'cancelAsync', id: id });
	}

	makeTaskId(): string {
		let id = '';
		const characters = 'ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789';
		for (let i = 0; i < 16; i++) {
			id += characters.charAt(Math.floor(Math.random() * characters.length));
		}
		return id;
	}

	async doApiCall(requestName: string, request: Uint8Array, id: string): Promise<Uint8Array> {
		this.numTasksRunning++;
		await this.onReady;

		const taskPromise = new Promise<Uint8Array>((resolve, reject) => {
			if (!id) {
				id = this.makeTaskId();
			}
			this.taskIdsToPromiseFuncs[id] = [resolve, reject];

			this.worker.postMessage({
				msg: requestName,
				id: id,
				inputData: request,
			});
		});
		return await taskPromise;
	}
}
