// run through tools/uicheck/cdp.py eval on an individual sim page: installs window.__pg for gen.py,
// driving the JSON import/export modals and the BiS Optimizer tab, plus download capture
(() => {
	const pg = (window.__pg = window.__pg || {});
	pg.downloads = pg.downloads || {};
	const prefix = 'data:text/json;charset=utf-8,';
	if (!HTMLAnchorElement.prototype.__pgClick) {
		const click = HTMLAnchorElement.prototype.click;
		HTMLAnchorElement.prototype.__pgClick = click;
		HTMLAnchorElement.prototype.click = function () {
			const href = this.getAttribute('href') || '';
			if (this.hasAttribute('download') && href.startsWith(prefix)) {
				window.__pg.downloads[this.getAttribute('download')] = decodeURIComponent(href.slice(prefix.length));
				return;
			}
			return click.call(this);
		};
	}
	pg.take = name => {
		const t = pg.downloads[name];
		delete pg.downloads[name];
		return t === undefined ? null : t;
	};

	pg.sleep = ms => new Promise(r => setTimeout(r, ms));
	pg.until = async (fn, ms = 30000) => {
		const end = Date.now() + ms;
		while (Date.now() < end) {
			try {
				const v = fn();
				if (v) return v;
			} catch (e) {}
			await pg.sleep(200);
		}
		throw new Error('timed out: ' + fn);
	};

	pg.tab = id => document.querySelector(`a[data-bs-target="#${id}"]`)?.click();

	// Drops the modal DOM directly instead of Bootstrap's animated hide(), which this driver
	// sometimes can't get to finish (hide.bs.modal never settling on a scripted click).
	pg.closeModals = () => {
		document.querySelectorAll('.modal').forEach(m => m.remove());
		document.querySelectorAll('.modal-backdrop').forEach(b => b.remove());
		document.body.classList.remove('modal-open');
		document.body.style.removeProperty('overflow');
		document.body.style.removeProperty('padding-right');
	};

	pg.dropdownItem = (cls, label) => [...document.querySelectorAll(`.${cls} .dropdown-item`)].find(a => a.textContent.trim() === label);

	// the header's JSON export: the modal's textarea already holds the current settings when it opens
	pg.exportSettings = async () => {
		pg.closeModals();
		pg.dropdownItem('export-dropdown', 'JSON').click();
		await pg.until(() => document.querySelector('.modal .exporter-textarea'));
		await pg.sleep(200);
		const text = document.querySelector('.modal .exporter-textarea').value;
		pg.closeModals();
		return text;
	};

	// the header's JSON import: applies talents, glyphs, race, professions, gear and everything else
	// IndividualSimSettings carries, in one shot (simUI.fromProto). onImport is async (it may fetch
	// missing item data), so this waits for the modal it closes on its own before forcing the rest shut.
	pg.importSettings = async json => {
		pg.closeModals();
		pg.dropdownItem('import-dropdown', 'JSON').click();
		await pg.until(() => document.querySelector('.modal .importer-textarea'));
		const textarea = document.querySelector('.modal .importer-textarea');
		textarea.value = json;
		textarea.dispatchEvent(new Event('input'));
		document.querySelector('.modal .import-button').click();
		await pg.until(() => !document.querySelector('.modal .importer-textarea'), 15000).catch(() => null);
		pg.closeModals();
		return 'ok';
	};

	// clicks a named preset chip on the Talents or Gear tab (SavedDataManager's '.saved-data-set-name')
	pg.clickPreset = async (tabId, name) => {
		pg.tab(tabId);
		const link = await pg.until(() =>
			[...document.querySelectorAll(`#${tabId} .saved-data-set-name`)].find(a => a.textContent.trim() === name),
		);
		link.click();
		await pg.sleep(200);
		return 'ok';
	};

	pg.field = label => [...document.querySelectorAll('#optimizer-tab .optimizer-field')].find(f => f.querySelector('.form-label')?.textContent.trim() === label);

	pg.button = label => [...document.querySelectorAll('#optimizer-tab button')].find(b => b.textContent.trim() === label);

	pg.setupOptimizer = async (phase, effort) => {
		pg.tab('optimizer-tab');
		await pg.until(() => document.querySelector('#optimizer-tab .optimizer-setup'));
		const phaseSelect = pg.field('Content phase').querySelector('select');
		if (phaseSelect.value != String(phase)) {
			phaseSelect.value = String(phase);
			phaseSelect.dispatchEvent(new Event('change'));
			await pg.sleep(200);
		}
		const effortSelect = pg.field('Effort').querySelector('select');
		const option = [...effortSelect.options].find(o => o.textContent.split(' ')[0] == effort);
		if (!option) {
			throw new Error(`no effort ${effort}`);
		}
		if (effortSelect.value != option.value) {
			effortSelect.value = option.value;
			effortSelect.dispatchEvent(new Event('change'));
			await pg.sleep(200);
		}
		return { phase: phaseSelect.value, effort: option.textContent };
	};

	// Starts Optimize and waits for it to settle. A rerun leaves the previous result (and its Export
	// JSON button) on screen until showResult() replaces it, so completion is read off the Cancel
	// button's visibility (shown while running), not off the result panel.
	pg.runOptimizer = async timeoutMs => {
		const optimize = pg.button('Optimize');
		if (!optimize) {
			throw new Error('no Optimize button (server unavailable?)');
		}
		optimize.click();
		await pg.until(() => {
			const cancel = pg.button('Cancel');
			return cancel && !cancel.hidden;
		}, 10000);
		const settled = await pg.until(() => {
			const cancel = pg.button('Cancel');
			if (cancel && !cancel.hidden) return null;
			const status = document.querySelector('#optimizer-tab .optimizer-status');
			return status && status.classList.contains('optimizer-status-error') ? 'error' : 'done';
		}, timeoutMs);
		// a failed run leaves the previous phase's result and its Export JSON button on screen
		if (settled === 'error' || !pg.button('Export JSON')) {
			const status = document.querySelector('#optimizer-tab .optimizer-status');
			throw new Error(`optimizer failed: ${status ? status.innerText.trim() : 'unknown'}`);
		}
		return 'ok';
	};

	pg.exportResult = phase => {
		pg.button('Export JSON').click();
		return pg.take(`optimizer-result-p${phase}.json`);
	};

	pg.ready = () => document.readyState == 'complete' && !!document.querySelector('.import-dropdown .dropdown-item');
})();
'ok';
