var workerID = "";

// Async calls in flight by task id, with the server's progress handle once it's known.
const asyncTasks = new Map();

function cancelAsync(progressHandle) {
	fetch("/cancelAsync", {
		method: 'POST',
		headers: {
			'Content-Type': 'application/x-protobuf'
		},
		body: progressHandle,
	}).catch(err => console.warn('cancelAsync failed: ' + err));
}

addEventListener('message', async (e) => {
	const msg = e.data.msg;
	const id = e.data.id;

	if (msg == "setID") {
		workerID = id;
		postMessage({ msg: "idconfirm" })
		return;
	}

	// id is the task to cancel. No reply: the task still ends with its own final result.
	if (msg == "cancelAsync") {
		const task = asyncTasks.get(id);
		if (task && task.handle) {
			cancelAsync(task.handle);
		} else if (task) {
			task.cancelRequested = true;
		}
		return;
	}

	const isAsync = msg == "raidSimAsync" || msg == "statWeightsAsync" || msg == "bulkSimAsync" || msg == "optimizeGearAsync";
	const task = { handle: null, cancelRequested: false };
	if (isAsync) {
		asyncTasks.set(id, task);
	}

	// A fetch here throws when the server's gone (a dropped connection, a restarted container): without
	// this, the caller's promise never settles, so a run hangs and Stop can't free it.
	try {
		var url = "/" + msg;
		let response = await fetch(url, {
			method: 'POST',
			headers: {
				'Content-Type': 'application/x-protobuf'
			},
			body: e.data.inputData
		});

		var content = await response.arrayBuffer();
		var outputData;
		if (isAsync) {
			task.handle = content;
			if (task.cancelRequested) {
				cancelAsync(content);
			}
			while (true) {
				let progressResponse = await fetch("/asyncProgress", {
					method: 'POST',
					headers: {
						'Content-Type': 'application/x-protobuf'
					},
					body: content,
				});

				// If no new data available, stop querying.
				if (progressResponse.status == 204) {
					break
				}

				outputData = await progressResponse.arrayBuffer();
				var uint8View = new Uint8Array(outputData);
				postMessage({
					msg: msg,
					outputData: uint8View,
					id: id + "progress",
				});
				await new Promise(resolve => setTimeout(resolve, 500));
			}
		} else {
			outputData = content;
		}

		var uint8View = new Uint8Array(outputData);
		postMessage({
			msg: msg,
			outputData: uint8View,
			id: id,
		});
	} catch (err) {
		const error = (err && err.message) || String(err);
		postMessage({ msg: msg, id: id, error: error });
		if (isAsync) {
			// a progress handler could be pending on this id too
			postMessage({ msg: msg, id: id + "progress", error: error });
		}
	} finally {
		if (isAsync) {
			asyncTasks.delete(id);
		}
	}
}, false);

// Let UI know worker is ready.
postMessage({
	msg: "ready"
});
