// run through cdp.py eval on the raid sim: installs window.__r, raid import and BiS Batch tab helpers.
// alert and confirm are stubbed so the importer can't block the page: messages land in __r.alerts,
// and every confirm says yes
window.__r = {
	alerts: [],
	sleep: ms => new Promise(r => setTimeout(r, ms)),
	async until(fn, ms = 30000) {
		const end = Date.now() + ms;
		while (Date.now() < end) {
			try {
				const v = fn();
				if (v) return v;
			} catch (e) {}
			await this.sleep(250);
		}
		throw new Error('timed out: ' + fn);
	},
	tab(id) {
		document.querySelector(`a[data-bs-target="#${id}"]`).click();
	},
	// runs the AzerothCore importer on an acraid roster (JSON text or object), replace or update, and
	// returns the head of its report
	async importRoster(roster, mode = 'replace') {
		const seen = this.alerts.length;
		const link = [...document.querySelectorAll('.import-dropdown .dropdown-item')].find(a => a.textContent.trim() == 'AzerothCore');
		link.click();
		const modal = await this.until(() => [...document.querySelectorAll('.modal.show')].find(m => m.querySelector('.acore-import-mode')));
		modal.querySelector(`.acore-import-mode input[value=${mode}]`).click();
		const text = modal.querySelector('.importer-textarea');
		text.value = typeof roster == 'string' ? roster : JSON.stringify(roster);
		text.dispatchEvent(new Event('input'));
		modal.querySelector('.import-button').click();
		const report = await this.until(() => this.alerts.slice(seen).find(a => a.startsWith('AzerothCore import')), 60000);
		return report.split('\n').slice(0, 3).join(' | ');
	},
	root() {
		return document.getElementById('bis-batch-tab');
	},
	rows() {
		return [...this.root().querySelectorAll('.optimizer-batch-grid table tr')].slice(1);
	},
	raider(tr) {
		return tr.children[0].querySelector('.form-check-label').firstChild.textContent;
	},
	// one entry per raider: name, ticked, and each phase cell's text
	grid() {
		return this.rows().map(tr => ({
			name: this.raider(tr),
			ticked: tr.children[0].querySelector('input').checked,
			cells: [...tr.children].slice(1).map(td => td.innerText.trim()),
		}));
	},
	phaseBoxes() {
		return [...this.root().querySelectorAll('.optimizer-batch-grid table tr:first-child th input')];
	},
	// ticks exactly these raiders and phases, unticks the rest, and returns the setup panel's text
	async pick(names, phases) {
		for (const [i, box] of this.phaseBoxes().entries()) {
			if (box.checked != phases.includes(i + 1)) {
				box.click();
				await this.sleep(100);
			}
		}
		// the grid re-renders on every click, so look each row up again
		for (let i = 0; i < this.rows().length; i++) {
			const tr = this.rows()[i];
			const box = tr.children[0].querySelector('input');
			if (box.checked != names.includes(this.raider(tr))) {
				box.click();
				await this.sleep(50);
			}
		}
		return this.setupText();
	},
	setupText() {
		return this.root().querySelector('.optimizer-setup').innerText;
	},
	button(label) {
		return [...this.root().querySelectorAll('button')].find(b => b.textContent.trim() == label);
	},
	cell(name, phase) {
		return this.rows().find(tr => this.raider(tr) == name).children[phase];
	},
	// opens a finished cell's result and returns the detail panel's text
	async open(name, phase) {
		this.cell(name, phase).querySelector('a').click();
		await this.sleep(500);
		return this.detail();
	},
	detail() {
		return this.root().querySelector('.optimizer-batch-detail').innerText;
	},
	warningTitle(name, phase) {
		return this.cell(name, phase).querySelector('i.fa-exclamation-triangle')?.title || '';
	},
	// the batches in localStorage, as [key, size in chars]
	storedBatches() {
		return Object.keys(localStorage)
			.filter(k => k.includes('optimizer-batch'))
			.map(k => [k, localStorage.getItem(k).length]);
	},
	// the first stored batch's jobs, as `raider Pn state`
	storedJobs() {
		const key = Object.keys(localStorage).find(k => k.includes('optimizer-batch'));
		const saved = JSON.parse(localStorage.getItem(key));
		return { running: saved.running, jobs: saved.jobs.map(j => `${j.raider} P${j.phase} ${j.state}`) };
	},
	// the saved gear set names under a spec's localStorage key, e.g. what a batch saved its BiS as
	savedGear(specKey) {
		return Object.keys(JSON.parse(localStorage.getItem(specKey + '__savedGear__') || '{}'));
	},
};
window.alert = msg => window.__r.alerts.push(String(msg));
window.confirm = msg => {
	window.__r.alerts.push('confirm: ' + msg);
	return true;
};
'ok';
