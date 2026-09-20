import * as fs from 'fs';

import { MAX_PARTY_SIZE } from '../../core/party';
import { Party as PartyProto, Player as PlayerProto, Raid as RaidProto } from '../../core/proto/api';
import { Class } from '../../core/proto/common';
import { Database } from '../../core/proto_utils/database';
import { isTankSpec, newUnitReference, specNames } from '../../core/proto_utils/utils';
import { MAX_NUM_PARTIES } from '../../core/raid';
import { Sim } from '../../core/sim';
import { TypedEvent } from '../../core/typed_event';
import {
	activeParties,
	applyCharacter,
	assignRaidIndexes,
	buildCharacterImport,
	matchPreset,
	MEMBER_FLAG_MAIN_TANK,
	newPlayerFromPreset,
	parseRoster,
	rosterEquipmentSpec,
} from '../acore_roster';
import { playerPresets } from '../presets';

// Mirrors RaidAcoreImporter.replaceRaid, which can't be driven headlessly because it lives on a modal.
async function main() {
	const roster = parseRoster(fs.readFileSync(process.argv[2], 'utf-8'));
	const db = await Database.loadLeftoversIfNecessary(rosterEquipmentSpec(roster));
	const sim = new Sim();
	await sim.waitForInit(); // sim.db is null until this resolves
	const raid = sim.raid;
	const eventID = TypedEvent.nextEventID();
	const indexes = assignRaidIndexes(roster.characters);

	const imports = roster.characters.map((char, i) => {
		const imported = buildCharacterImport(db, char);
		return {
			imported: imported,
			preset: matchPreset(playerPresets, imported.spec, imported.talentsString),
			raidIndex: indexes[i],
			isMainTank: (char.memberFlags & MEMBER_FLAG_MAIN_TANK) != 0,
		};
	});

	const raidProto = RaidProto.create({
		parties: [...new Array(MAX_NUM_PARTIES).keys()].map(partyIdx =>
			PartyProto.create({
				players: [...new Array(MAX_PARTY_SIZE).keys()].map(() => PlayerProto.create()),
				buffs: raid.getParty(partyIdx).getBuffs(),
			}),
		),
		numActiveParties: activeParties(roster.characters),
	});
	const tanks = imports
		.filter(entry => entry.isMainTank)
		.concat(imports.filter(entry => !entry.isMainTank && isTankSpec(entry.imported.spec)))
		.map(entry => newUnitReference(entry.raidIndex));
	raidProto.tanks = tanks;

	TypedEvent.freezeAllAndDo(() => {
		imports.forEach(entry => {
			const player = newPlayerFromPreset(entry.imported.spec, entry.preset, sim, eventID);
			applyCharacter(player, entry.imported, eventID);
			const partyIdx = Math.floor(entry.raidIndex / MAX_PARTY_SIZE);
			raidProto.parties[partyIdx].players[entry.raidIndex % MAX_PARTY_SIZE] = player.toProto();
		});
		raid.clear(eventID);
		raid.fromProto(eventID, raidProto);
	});

	const players = raid.getPlayers().filter(p => p != null);
	console.log('players after fromProto:', players.length, '(expect 25)');
	console.log('numActiveParties:', raid.getNumActiveParties(), '(expect 5)');
	console.log(
		'tanks:',
		raid
			.getTanks()
			.map(t => {
				const p = raid.getPlayerFromUnitReference(t);
				return p ? `${p.getName()}@${t.index}` : `empty@${t.index}`;
			})
			.join(', '),
	);

	let specMismatch = 0;
	let reforgeTotal = 0;
	imports.forEach(entry => {
		const player = raid.getPlayer(entry.raidIndex);
		if (!player) {
			console.log(`MISSING ${entry.imported.char.name} at ${entry.raidIndex}`);
			return;
		}
		if (player.getName() != entry.imported.char.name) {
			console.log(`WRONG NAME at ${entry.raidIndex}: ${player.getName()} vs ${entry.imported.char.name}`);
		}
		if (player.spec != entry.imported.spec) {
			console.log(`SPEC DRIFT ${entry.imported.char.name}: ${specNames[player.spec]} vs ${specNames[entry.imported.spec]}`);
			specMismatch++;
		}
		reforgeTotal += player.getGear().asSpec().items.filter(item => !!item.reforge).length;
	});
	console.log('spec mismatches:', specMismatch, '(expect 0)');
	console.log('reforges surviving the proto round trip:', reforgeTotal, '(expect 146)');
	console.log('paladins:', raid.getClassCount(Class.ClassPaladin), '(expect 3)');

	const druidica = players.find(p => p!.getName() == 'Druidica')!;
	console.log('Druidica racial traits after round trip:', druidica.getRacialTraits(), '(expect 2)');
	console.log('Druidica professions after round trip:', druidica.getProfessions().length, '(expect 11)');
	console.log('party of Druidica:', Math.floor(druidica.getRaidIndex() / MAX_PARTY_SIZE) + 1, '(expect 4)');
}

main().catch(e => {
	console.error(e);
	process.exit(1);
});
