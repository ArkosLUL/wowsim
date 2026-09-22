// run through tools/uicheck/cdp.py eval on the raid sim, after tools/uicheck/raid.js: installs
// window.__bis for driver.py, the BiS Batch tab's state and controls plus download capture
(() => {
	const bis = (window.__bis = window.__bis || {});
	bis.downloads = bis.downloads || {};
	const isBatchKey = key => String(key).includes('optimizer-batch.v1.');
	const settledCount = text => {
		try {
			return JSON.parse(text).jobs.filter(j => j.state == 'done' || j.state == 'failed').length;
		} catch (e) {
			return -1;
		}
	};

	// downloadString clicks a data: link, which headless Chrome would save somewhere we can't see
	if (!HTMLAnchorElement.prototype.__bisClick) {
		const click = HTMLAnchorElement.prototype.click;
		HTMLAnchorElement.prototype.__bisClick = click;
		HTMLAnchorElement.prototype.click = function () {
			const prefix = 'data:text/json;charset=utf-8,';
			const href = this.getAttribute('href') || '';
			if (this.hasAttribute('download') && href.startsWith(prefix)) {
				window.__bis.downloads[this.getAttribute('download')] = decodeURIComponent(href.slice(prefix.length));
				return;
			}
			return click.call(this);
		};
	}

	// Keeps the page's last batch write even when localStorage refuses it: past the quota the run goes
	// on in memory, and the driver reads its progress from this copy
	if (!Storage.prototype.__bisSetItem) {
		const setItem = Storage.prototype.setItem;
		Storage.prototype.__bisSetItem = setItem;
		Storage.prototype.setItem = function (key, value) {
			if (this !== window.localStorage || !isBatchKey(key)) {
				return setItem.call(this, key, value);
			}
			const bis = window.__bis;
			bis.latest = { key: String(key), text: String(value) };
			try {
				setItem.call(this, key, value);
				bis.writeFailed = false;
			} catch (e) {
				bis.writeFailed = true;
				throw e;
			} finally {
				bis.checkStage1Stop(String(value));
			}
		};
	}

	bis.take = name => {
		const text = bis.downloads[name];
		delete bis.downloads[name];
		return text === undefined ? null : text;
	};

	// the raid sim's keys, with the batch as the page last wrote it
	bis.raidStorage = () => {
		const out = Object.fromEntries(Object.keys(localStorage).filter(k => k.startsWith('__wotlk_raid__')).map(k => [k, localStorage.getItem(k)]));
		if (bis.latest) {
			out[bis.latest.key] = bis.latest.text;
		}
		return JSON.stringify(out);
	};

	// Runs on another page of the sim's origin before the raid page loads. Chrome writes localStorage
	// to disk lazily, so a killed Chrome comes back with an older batch or none: this puts back what
	// the snapshot has more of. Clearing the running flag keeps the page from resuming a batch on its
	// own, which would run whatever the last pass set up before the driver could step in.
	bis.prepareStorage = text => {
		const out = { restored: [], stopped: [], failed: [] };
		const write = (key, value, done) => {
			try {
				localStorage.setItem(key, value);
				done.push(key);
			} catch (e) {
				out.failed.push(`${key}: ${e.name}`);
			}
		};
		for (const [key, value] of Object.entries(text ? JSON.parse(text) : {})) {
			const current = localStorage.getItem(key);
			if (isBatchKey(key) ? settledCount(value) > settledCount(current) : current != value) {
				write(key, value, out.restored);
			}
		}
		for (const key of Object.keys(localStorage).filter(isBatchKey)) {
			try {
				const saved = JSON.parse(localStorage.getItem(key));
				if (saved.running) {
					saved.running = false;
					write(key, JSON.stringify(saved), out.stopped);
				}
			} catch (e) {}
		}
		return JSON.stringify(out);
	};

	bis.root = () => document.getElementById('bis-batch-tab');

	bis.button = label => [...bis.root().querySelectorAll('button')].find(b => b.textContent.trim() == label);

	// the batch the page last wrote, else the newest stored one, which can belong to another roster
	// until the page writes this roster's (touch)
	bis.stored = () => {
		if (bis.latest) {
			if (bis.parsed?.text !== bis.latest.text) {
				try {
					bis.parsed = { text: bis.latest.text, saved: JSON.parse(bis.latest.text) };
				} catch (e) {
					bis.parsed = { text: bis.latest.text, saved: null };
				}
			}
			if (bis.parsed.saved) {
				return { key: bis.latest.key, chars: bis.latest.text.length, saved: bis.parsed.saved };
			}
		}
		let best = null;
		for (const key of Object.keys(localStorage)) {
			if (!isBatchKey(key)) {
				continue;
			}
			try {
				const text = localStorage.getItem(key);
				const saved = JSON.parse(text);
				if (!best || (saved.savedAt || 0) > best.saved.savedAt) {
					best = { key, chars: text.length, saved };
				}
			} catch (e) {}
		}
		return best;
	};

	// makes the page write the current roster's batch, through a change event that keeps the effort
	bis.touch = () => {
		bis.effortSelect().dispatchEvent(new Event('change'));
		return 'ok';
	};

	bis.raiders = () =>
		window.__r.rows().map(tr => {
			const label = tr.children[0].querySelector('.form-check-label');
			const spec = label.querySelector('.optimizer-hint')?.textContent || '';
			return { name: window.__r.raider(tr), spec, tank: spec.endsWith('(tank)') };
		});

	bis.state = () => {
		const root = bis.root();
		const stop = bis.button('Stop');
		const start = bis.button('Start') || bis.button('Resume');
		const stored = bis.stored();
		const settings = stored?.saved.settings || {};
		const jobs = (stored?.saved.jobs || []).map(job => ({
			raider: job.raider,
			raidIndex: job.raidIndex,
			phase: job.phase,
			stage: job.stage,
			state: job.state,
			error: job.error || '',
			elapsed: job.result?.elapsedSeconds || 0,
			sims: job.result?.totalSims || 0,
			commit: job.result?.simCommit || '',
		}));
		// what the page needs stored: the other keys plus the batch as it last wrote it
		const needChars = Object.keys(localStorage)
			.filter(k => k != stored?.key)
			.reduce((n, k) => n + k.length + localStorage.getItem(k).length, stored ? stored.key.length + stored.chars : 0);
		return {
			ready: !!root?.querySelector('.optimizer-setup'),
			running: !!stop && !stop.hidden,
			start: start ? { label: start.textContent.trim(), disabled: start.disabled } : null,
			retry: !!bis.button('Run failed ones again') && !bis.button('Run failed ones again').hidden,
			wasm: !!root?.querySelector('.optimizer-availability'),
			status: root?.querySelector('.optimizer-status')?.innerText.trim() || '',
			storedKey: stored?.key || '',
			storedChars: stored?.chars || 0,
			needChars,
			writeFailed: !!bis.writeFailed,
			settings,
			jobs,
		};
	};

	bis.effortSelect = () =>
		[...bis.root().querySelectorAll('.optimizer-setup .optimizer-field')].find(f => f.querySelector('label')?.textContent.trim() == 'Effort').querySelector('select');

	// effort by its label's first word: Quick, Normal or Thorough
	bis.setEffort = word => {
		const select = bis.effortSelect();
		const option = [...select.options].find(o => o.textContent.split(' ')[0] == word);
		if (!option) {
			throw new Error(`no effort ${word}`);
		}
		if (select.value != option.value) {
			select.value = option.value;
			select.dispatchEvent(new Event('change'));
		}
		return option.textContent;
	};

	bis.click = label => {
		const b = bis.button(label);
		if (!b || b.hidden || b.disabled) {
			return false;
		}
		b.click();
		return true;
	};

	// The Stop button is still hidden while a run's first write lands, and clicking it works anyway:
	// the page only stops a run that exists
	bis.stop = () => {
		bis.button('Stop')?.click();
		return 'ok';
	};

	// The batch always follows a phase's stage 1 with its stage 2 and has no switch for it. Stopping
	// from inside the write that settles the last stage 1 job ends the run loop before it starts
	// stage 2, so no server run gets started and cancelled.
	bis.stopAfterStage1 = (phase, names) => {
		bis.stage1Stop = { phase, names };
		return 'ok';
	};

	bis.checkStage1Stop = text => {
		const want = bis.stage1Stop;
		if (!want) {
			return;
		}
		try {
			const saved = JSON.parse(text);
			const settled = want.names.every(name =>
				saved.jobs.some(j => j.raider == name && j.phase == want.phase && j.stage == 1 && (j.state == 'done' || j.state == 'failed')),
			);
			if (saved.running && settled) {
				bis.stage1Stop = null;
				bis.stop();
			}
		} catch (e) {}
	};
})();
'ok';
