import { acSlotToItemSlot, parseRoster, RosterCharacter, rosterClass } from '../../../raid/acore_roster';
import { fetchServerRoster } from '../../../raid/acore_server';
import { IndividualSimUI } from '../../individual_sim_ui';
import { ItemReforge, ItemSlot, Spec } from '../../proto/common';
import { EquippedItem } from '../../proto_utils/equipped_item';
import { slotNames } from '../../proto_utils/names';
import { isValidReforge, LIVE_REFORGING, reforgeLabel, reforgeStatTypeName } from '../../proto_utils/reforging';
import { BaseModal } from '../base_modal';
import { button, newElement } from './optimizer_tab';

interface SlotReply {
	acSlot: number;
	status: string;
	text: string;
}

// One equipped slot: what the sim has against what the server saved.
interface Row {
	acSlot: number;
	itemName: string;
	serverLabel: string;
	simLabel: string;
	// the item to send, or undefined when the row is left out
	send?: { acSlot: number; itemId: number; from: number; to: number };
	note: string;
	statusCell: HTMLElement;
}

// Sends the sim's reforges for one core-raid character to the live server, through /acoreReforge.
export class AcoreReforgeDialog extends BaseModal {
	private readonly simUI: IndividualSimUI<Spec>;
	private readonly picker: HTMLSelectElement;
	private readonly status: HTMLElement;
	private readonly table: HTMLTableElement;
	private readonly sendButton: HTMLButtonElement;
	private characters: Array<RosterCharacter> = [];
	private rows: Array<Row> = [];

	constructor(parent: HTMLElement, simUI: IndividualSimUI<any>) {
		super(parent, 'acore-reforge-dialog', { title: 'Apply reforges on the server', footer: true, size: 'xl' });
		this.simUI = simUI;

		this.picker = newElement('select', 'form-select acore-reforge-character');
		this.picker.addEventListener('change', () => this.renderRows());
		this.status = newElement('p', 'acore-reforge-status');
		this.table = newElement('table', 'table table-sm acore-reforge-table');
		this.body.append(
			newElement('label', 'form-label', 'Character'),
			this.picker,
			this.status,
			this.table,
			newElement(
				'p',
				'form-text acore-reforge-notes',
				'The server list is the character as of their last save. A character who is logged out gets the reforges at their next ' +
					'login. A bot redoes its own reforges when it refreshes its gear.',
			),
		);
		this.sendButton = button('Send to server', 'btn-primary acore-reforge-send', () => this.send());
		this.sendButton.disabled = true;
		this.footer!.appendChild(this.sendButton);
		this.load();
	}

	private async load() {
		this.showStatus('Reading the core raid off the server…', false);
		try {
			const roster = parseRoster(await fetchServerRoster());
			const playerClass = this.simUI.player.getClass();
			this.characters = roster.characters.filter(char => rosterClass(char.classId) == playerClass);
		} catch (error: any) {
			this.showStatus(`Couldn't read the roster off the server: ${error?.message}`, true);
			return;
		}
		if (this.characters.length == 0) {
			this.showStatus('The core raid has no character of this class.', true);
			return;
		}
		// names come off the server, so build the options instead of templating HTML
		this.characters.forEach(char => this.picker.appendChild(new Option(char.name, char.name)));
		const saved = this.savedPick();
		this.picker.value = this.characters.some(char => char.name == saved) ? saved : this.characters[0].name;
		this.showStatus('', false);
		this.renderRows();
	}

	// the importer's remembered character
	private savedPick(): string {
		try {
			const saved = JSON.parse(window.localStorage.getItem(this.simUI.getStorageKey('acore-import.v1')) || '{}');
			return typeof saved?.character == 'string' ? saved.character : '';
		} catch {
			return '';
		}
	}

	private showStatus(text: string, failed: boolean) {
		this.status.textContent = text;
		this.status.hidden = text == '';
		this.status.classList.toggle('text-danger', failed);
	}

	private renderRows() {
		const char = this.characters.find(c => c.name == this.picker.value);
		this.rows = char ? this.buildRows(char) : [];
		this.table.innerHTML = '';
		const head = this.table.createTHead().insertRow();
		['Slot', 'Item', 'On the server', 'In the sim', 'Status'].forEach(label => head.appendChild(newElement('th', '', label)));
		const body = this.table.createTBody();
		for (const row of this.rows) {
			const tr = body.insertRow();
			tr.dataset.acSlot = String(row.acSlot);
			tr.append(
				newElement('td', '', slotNames.get(acSlotToItemSlot[row.acSlot]) ?? `Slot ${row.acSlot}`),
				newElement('td', '', row.itemName),
				newElement('td', '', row.serverLabel),
				newElement('td', '', row.simLabel),
			);
			row.statusCell = newElement('td', 'acore-reforge-row-status', row.note);
			tr.appendChild(row.statusCell);
		}
		this.sendButton.disabled = !this.rows.some(row => row.send);
	}

	private buildRows(char: RosterCharacter): Array<Row> {
		const gear = this.simUI.player.getGear();
		// the live config, not the page's server settings: with reforging off there, every kept reforge
		// would read as none and Send would strip them all off the server
		const config = LIVE_REFORGING;
		const serverItems = new Map(char.gear.map(item => [item.acSlot, item]));
		const rows: Array<Row> = [];
		const statNames = (reforge: { fromStatType: number; toStatType: number }) =>
			`${reforgeStatTypeName(reforge.fromStatType)} → ${reforgeStatTypeName(reforge.toStatType)}`;

		for (const acSlot of Object.keys(acSlotToItemSlot).map(Number)) {
			const simItem = gear.getEquippedItem(acSlotToItemSlot[acSlot] as ItemSlot);
			const serverItem = serverItems.get(acSlot);
			if (!simItem && !serverItem) {
				continue;
			}
			const kept = simItem?.reforge && (simItem.reforge.fromStatType || simItem.reforge.toStatType) ? simItem.reforge : null;
			const simReforge = kept && isValidReforge(simItem!.item, kept, config) ? kept : null;
			const serverReforge = serverItem?.reforge ? ItemReforge.create(serverItem.reforge) : null;
			const serverReadable = !!serverReforge && simItem?.id == serverItem?.id && isValidReforge(simItem!.item, serverReforge, config);
			const label = (item: EquippedItem | null, reforge: ItemReforge | null) =>
				reforge && item ? reforgeLabel(item.item, reforge, config) : 'none';

			const row: Row = {
				acSlot,
				itemName: simItem?.item.name ?? `Item ${serverItem?.id}`,
				serverLabel: serverReforge ? (serverReadable ? label(simItem!, serverReforge) : statNames(serverReforge)) : 'none',
				simLabel: kept && !simReforge ? statNames(kept) : label(simItem, simReforge),
				note: '',
				statusCell: document.createElement('td'),
			};
			if (!simItem) {
				row.note = 'Left out: nothing equipped in the sim';
			} else if (!serverItem) {
				row.note = 'Left out: nothing equipped on the server';
			} else if (serverItem.id != simItem.id) {
				row.note = 'Different item on the server, left out';
			} else if (kept && !simReforge) {
				row.note = "Left out: the server can't take the sim's reforge";
			} else if (serverReforge && !serverReadable && !simReforge) {
				// the import drops a reforge the sim's item can't take, so none here isn't the user's pick
				row.note = "Left out: the sim's copy of the item can't take the server's reforge";
			} else {
				const same =
					(serverReforge?.fromStatType ?? 0) == (simReforge?.fromStatType ?? 0) && (serverReforge?.toStatType ?? 0) == (simReforge?.toStatType ?? 0);
				row.note = same ? 'Same' : simReforge ? 'Will change' : 'Will remove';
				row.send = { acSlot, itemId: simItem.id, from: simReforge?.fromStatType ?? 0, to: simReforge?.toStatType ?? 0 };
			}
			rows.push(row);
		}
		return rows;
	}

	private async send() {
		const sending = this.rows.filter(row => row.send);
		if (sending.length == 0) {
			return;
		}
		this.sendButton.disabled = true;
		this.picker.disabled = true;
		this.showStatus('Sending to the server…', false);
		try {
			const result = await postReforges(this.picker.value, sending.map(row => row.send!));
			const bySlot = new Map(result.slots.map(slot => [slot.acSlot, slot]));
			for (const row of sending) {
				const slot = bySlot.get(row.acSlot);
				row.statusCell.textContent = slot ? `${slot.status}: ${slot.text}` : 'No answer for this slot';
				row.statusCell.dataset.status = slot?.status ?? 'none';
			}
			this.showStatus(
				`${this.picker.value} was ${result.online ? 'online, so the reforges are in effect now' : 'offline, so they get the reforges at their next login'}.`,
				false,
			);
		} catch (error: any) {
			this.showStatus(error?.message || "The sim server didn't answer", true);
		} finally {
			this.sendButton.disabled = false;
			this.picker.disabled = false;
		}
	}
}

async function postReforges(
	character: string,
	items: Array<{ acSlot: number; itemId: number; from: number; to: number }>,
): Promise<{ online: boolean; slots: Array<SlotReply> }> {
	let response: Response;
	let text: string;
	try {
		response = await fetch('/acoreReforge', {
			method: 'POST',
			headers: { 'Content-Type': 'application/json' },
			body: JSON.stringify({ character, items }),
		});
		text = await response.text();
	} catch (e) {
		throw new Error("The sim server didn't answer");
	}
	let reply: any;
	try {
		reply = JSON.parse(text);
	} catch (e) {
		// a server built before the route existed answers with its 404 page
		throw new Error("This sim server can't reforge on AzerothCore");
	}
	if (!response.ok) {
		throw new Error(typeof reply?.error == 'string' && reply.error ? reply.error : "This sim server can't reforge on AzerothCore");
	}
	return reply;
}
