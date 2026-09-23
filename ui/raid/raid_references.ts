import { Player } from '../core/player';
import { UnitReference, UnitReference_Type } from '../core/proto/common';
import { emptyUnitReference } from '../core/proto_utils/utils';
import { Raid } from '../core/raid';
import { EventID, TypedEvent } from '../core/typed_event';

// Spec options that hold a raid slot. Not every spec has them, so check the field is there.
export const RAID_TARGET_OPTIONS = ['innervateTarget', 'powerInfusionTarget', 'tricksOfTheTradeTarget', 'unholyFrenzyTarget', 'focusMagicTarget'];

interface Snapshot {
	before: UnitReference;
	target: Player<any> | null;
}

const referenceTo = (target: Player<any> | null) => (target?.getParty() ? target.makeUnitReference() : emptyUnitReference());

// Tanks and raid-target spec options name a raid slot, not a raider. Runs the edit, then points each
// one the edit left alone back at its raider, or clears it if they left the raid.
export function keepRaidReferences<T>(raid: Raid, eventID: EventID, edit: () => T): T {
	const snapshot = (reference: UnitReference): Snapshot => ({
		before: UnitReference.clone(reference),
		target: reference.type == UnitReference_Type.Player ? raid.getPlayerFromUnitReference(reference) : null,
	});
	const tanks = raid.getTanks().map(snapshot);
	const options = raid
		.getPlayers()
		.filter((player): player is Player<any> => player != null)
		.flatMap(player => {
			const specOptions = player.getSpecOptions() as any;
			return RAID_TARGET_OPTIONS.filter(field => specOptions[field]?.type == UnitReference_Type.Player).map(field => ({
				player,
				field,
				...snapshot(specOptions[field]),
			}));
		});

	let result: T;
	TypedEvent.freezeAllAndDo(() => {
		result = edit();

		const newTanks = raid
			.getTanks()
			.map((tank, i) => (i < tanks.length && UnitReference.equals(tank, tanks[i].before) ? referenceTo(tanks[i].target) : tank));
		raid.setTanks(eventID, newTanks);

		options
			.filter(option => option.player.getParty() != null)
			.forEach(option => {
				const specOptions = option.player.getSpecOptions() as any;
				const reference = referenceTo(option.target);
				if (UnitReference.equals(specOptions[option.field], option.before) && !UnitReference.equals(reference, option.before)) {
					specOptions[option.field] = reference;
					option.player.setSpecOptions(eventID, specOptions);
				}
			});
	});
	return result!;
}
