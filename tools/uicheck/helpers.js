// run through cdp.py eval: installs window.__t, waits plus gear tab helpers
window.__t = {
	sleep: ms => new Promise(r => setTimeout(r, ms)),
	async until(fn, ms = 20000) {
		const end = Date.now() + ms;
		while (Date.now() < end) {
			try {
				const v = fn();
				if (v) return v;
			} catch (e) {}
			await this.sleep(200);
		}
		throw new Error('timed out: ' + fn);
	},
	catalogFetches() {
		return performance.getEntriesByType('resource').filter(e => e.name.includes('server_catalog')).map(e => e.name);
	},
	tab(id) {
		document.querySelector(`a[data-bs-target="#${id}"]`).click();
	},
	// opens the i-th picker's selector modal, in page order, not slot order: left column 0-8, right 9-16
	async openSlot(i) {
		document.querySelectorAll('.modal.show').forEach(m => m.querySelector('.close-button')?.click());
		await this.until(() => !document.querySelector('.modal.show')); await this.sleep(300);
		const pickers = document.querySelectorAll('#gear-tab .gear-picker-root .item-picker-root');
		pickers[i].querySelector('.item-picker-icon').click();
		await this.until(() => document.querySelector('.modal.show .selector-modal'));
		await this.sleep(500);
		return document.querySelector('.modal.show .selector-modal .selector-modal-tabs').innerText;
	},
	pane(contentId) {
		return document.querySelector(`.modal.show .selector-modal #${contentId}`);
	},
	async showTab(contentId) {
		document.querySelector(`.modal.show .selector-modal a[data-content-id="${contentId}"]`).click();
		await this.sleep(500);
	},
	async setPhase(contentId, phase) {
		const select = this.pane(contentId).querySelector('.selector-modal-phase-selector select');
		select.value = String(phase);
		select.dispatchEvent(new Event('change'));
		await this.sleep(600);
		return select.value;
	},
	async search(contentId, text) {
		const input = this.pane(contentId).querySelector('.selector-modal-search');
		input.value = text;
		input.dispatchEvent(new Event('input'));
		await this.sleep(600);
		return Array.from(this.pane(contentId).querySelectorAll('li.selector-modal-list-item')).map(
			li => li.querySelector('.selector-modal-list-item-name').innerText.trim() + (li.classList.contains('active') ? ' [equipped]' : ''),
		);
	},
	// rows in the virtual list, from its scroll height (56px rows)
	listSize(contentId) {
		const list = this.pane(contentId).querySelector('.selector-modal-list');
		return Math.round(list.scrollHeight / 56);
	},
};
'ok';
