import { acSlotToItemSlot, RosterCharacter } from '../../../raid/acore_roster';
import { ItemReforge, ItemSlot } from '../../proto/common';
import { EquippedItem } from '../../proto_utils/equipped_item';
import { Gear } from '../../proto_utils/gear';
import { isValidReforge, LIVE_REFORGING, reforgeLabel, reforgeStatTypeName } from '../../proto_utils/reforging';

export interface ServerReforge {
	acSlot: number;
	itemId: number;
	from: number;
	to: number;
}

export interface SlotReply {
	acSlot: number;
	status: string;
	text: string;
}

export type PlanStatus = 'change' | 'remove' | 'same' | 'skip';

// One equipped slot: what the server saved against what the sim has.
export interface PlanRow {
	acSlot: number;
	itemSlot: ItemSlot;
	status: PlanStatus;
	serverLabel: string;
	simLabel: string;
	// why a slot is left out
	note: string;
	// the item to send, for a change or a removal
	send?: ServerReforge;
}

// Sorts each slot into change, remove, same or left out. Checked against the live config, not the
// page's server settings: with reforging off there, every kept reforge would read as none and Send
// would strip them all off the server.
export function planServerReforges(gear: Gear, char: RosterCharacter): Array<PlanRow> {
	const config = LIVE_REFORGING;
	const serverItems = new Map(char.gear.map(item => [item.acSlot, item]));
	const rows: Array<PlanRow> = [];
	const statNames = (reforge: { fromStatType: number; toStatType: number }) =>
		`${reforgeStatTypeName(reforge.fromStatType)} → ${reforgeStatTypeName(reforge.toStatType)}`;

	for (const acSlot of Object.keys(acSlotToItemSlot).map(Number)) {
		const itemSlot = acSlotToItemSlot[acSlot] as ItemSlot;
		const simItem = gear.getEquippedItem(itemSlot);
		const serverItem = serverItems.get(acSlot);
		if (!simItem && !serverItem) {
			continue;
		}
		const kept = simItem?.reforge && (simItem.reforge.fromStatType || simItem.reforge.toStatType) ? simItem.reforge : null;
		const simReforge = kept && isValidReforge(simItem!.item, kept, config) ? kept : null;
		const serverReforge = serverItem?.reforge ? ItemReforge.create(serverItem.reforge) : null;
		const serverReadable = !!serverReforge && simItem?.id == serverItem?.id && isValidReforge(simItem!.item, serverReforge, config);
		const label = (item: EquippedItem | null, reforge: ItemReforge | null) => (reforge && item ? reforgeLabel(item.item, reforge, config) : 'none');

		const row: PlanRow = {
			acSlot,
			itemSlot,
			status: 'skip',
			serverLabel: serverReforge ? (serverReadable ? label(simItem!, serverReforge) : statNames(serverReforge)) : 'none',
			simLabel: kept && !simReforge ? statNames(kept) : label(simItem, simReforge),
			note: '',
		};
		if (!simItem) {
			row.note = 'nothing equipped in the sim';
		} else if (!serverItem) {
			row.note = 'nothing equipped on the server';
		} else if (serverItem.id != simItem.id) {
			row.note = 'different item on the server';
		} else if (kept && !simReforge) {
			row.note = "the server can't take the sim's reforge";
		} else if (serverReforge && !serverReadable && !simReforge) {
			// the import drops a reforge the sim's item can't take, so none here isn't the user's pick
			row.note = "the sim's copy of the item can't take the server's reforge";
		} else {
			const same =
				(serverReforge?.fromStatType ?? 0) == (simReforge?.fromStatType ?? 0) && (serverReforge?.toStatType ?? 0) == (simReforge?.toStatType ?? 0);
			row.status = same ? 'same' : simReforge ? 'change' : 'remove';
			if (!same) {
				row.send = { acSlot, itemId: simItem.id, from: simReforge?.fromStatType ?? 0, to: simReforge?.toStatType ?? 0 };
			}
		}
		rows.push(row);
	}
	return rows;
}

export async function postReforges(character: string, items: Array<ServerReforge>): Promise<{ online: boolean; slots: Array<SlotReply> }> {
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
