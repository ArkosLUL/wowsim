import * as fs from 'fs';

import { MAX_PARTY_SIZE } from '../../core/party';
import { Player } from '../../core/player';
import { Class, Consumes, UnitReference, UnitReference_Type } from '../../core/proto/common';
import { Database } from '../../core/proto_utils/database';
import { emptyUnitReference, getTalentTree, isTankSpec, newUnitReference, specNames } from '../../core/proto_utils/utils';
import { MAX_NUM_PARTIES, Raid } from '../../core/raid';
import { Sim } from '../../core/sim';
import { TypedEvent } from '../../core/typed_event';
import {
	activeParties,
	applyCharacter,
	assignRaidIndexes,
	buildCharacterImport,
	CharacterImport,
	matchPreset,
	MEMBER_FLAG_MAIN_TANK,
	newPlayerFromPreset,
	parseRoster,
	rosterEquipmentSpec,
} from '../acore_roster';
import { playerPresets } from '../presets';

// Mirrors RaidAcoreImporter, which can't be driven headlessly because it lives on a modal.
const assignmentFields: Array<{ playerClass: Class; field: string }> = [
	{ playerClass: Class.ClassDruid, field: 'innervateTarget' },
	{ playerClass: Class.ClassPriest, field: 'powerInfusionTarget' },
	{ playerClass: Class.ClassRogue, field: 'tricksOfTheTradeTarget' },
	{ playerClass: Class.ClassDeathknight, field: 'unholyFrenzyTarget' },
	{ playerClass: Class.ClassMage, field: 'focusMagicTarget' },
];

interface RaidCharacter extends CharacterImport {
	preset: any;
	raidIndex: number;
	isMainTank: boolean;
}

function buildImports(db: Database, path: string): Array<RaidCharacter> {
	const roster = parseRoster(fs.readFileSync(path, 'utf-8'));
	const indexes = assignRaidIndexes(roster.characters);
	return roster.characters.map((char, i) => {
		const imported = buildCharacterImport(db, char);
		return {
			...imported,
			preset: matchPreset(playerPresets, imported.spec, imported.talentsString),
			raidIndex: indexes[i],
			isMainTank: (char.memberFlags & MEMBER_FLAG_MAIN_TANK) != 0,
		};
	});
}

function updateRaid(sim: Sim, imports: Array<RaidCharacter>) {
	const eventID = TypedEvent.nextEventID();
	const raid = sim.raid;
	const log: Array<string> = [];

	TypedEvent.freezeAllAndDo(() => {
		const before = raid.getPlayers().filter(p => p != null) as Array<Player<any>>;
		const byName = new Map<string, Player<any>>();
		before.forEach(p => {
			if (!byName.has(p.getName())) byName.set(p.getName(), p);
		});

		const snapshots: Array<{ player: Player<any>; field: string; target: Player<any> | null }> = [];
		raid.getPlayers().forEach(player => {
			if (!player) return;
			assignmentFields
				.filter(a => a.playerClass == player.getClass())
				.forEach(a => {
					const options = player.getSpecOptions() as any;
					const ref: UnitReference | undefined = options[a.field];
					if (!ref || ref.type != UnitReference_Type.Player) return;
					snapshots.push({ player: player, field: a.field, target: raid.getPlayerFromUnitReference(ref) });
				});
		});

		const placed = imports.map(imported => {
			const match = byName.get(imported.char.name);
			if (match) byName.delete(imported.char.name);
			const keep =
				!!match && match.getClass() == imported.playerClass && getTalentTree(match.getTalentsString()) == getTalentTree(imported.talentsString);
			let player: Player<any>;
			if (keep) {
				player = match!;
			} else {
				player = newPlayerFromPreset(imported.spec, imported.preset, sim, eventID);
				log.push(match ? `replaced ${imported.char.name}: ${specNames[match.spec]} -> ${specNames[imported.spec]}` : `added ${imported.char.name}`);
			}
			applyCharacter(player, imported, eventID);
			return { imported: imported, player: player };
		});

		for (let i = 0; i < MAX_NUM_PARTIES * MAX_PARTY_SIZE; i++) raid.setPlayer(eventID, i, null);
		placed.forEach(entry => raid.setPlayer(eventID, entry.imported.raidIndex, entry.player));

		const rosterNames = new Set(imports.map(i => i.char.name));
		before.filter(p => p.getParty() == null && !rosterNames.has(p.getName())).forEach(p => log.push(`removed ${p.getName()}`));

		const raiders = placed.map(e => ({ spec: e.player.spec, raidIndex: e.imported.raidIndex, isMainTank: e.imported.isMainTank }));
		raid.setTanks(
			eventID,
			raiders
				.filter(r => r.isMainTank)
				.concat(raiders.filter(r => !r.isMainTank && isTankSpec(r.spec)))
				.slice(0, 4)
				.map(r => newUnitReference(r.raidIndex)),
		);

		snapshots.forEach(s => {
			if (s.player.getParty() == null) return;
			const options = s.player.getSpecOptions() as any;
			if (!(s.field in options)) return;
			options[s.field] = s.target && s.target.getParty() != null ? newUnitReference(s.target.getRaidIndex()) : emptyUnitReference();
			s.player.setSpecOptions(eventID, options);
		});

		raid.setNumActiveParties(eventID, activeParties(imports.map(i => i.char)));
	});
	return log;
}

function placeFresh(sim: Sim, imports: Array<RaidCharacter>) {
	const eventID = TypedEvent.nextEventID();
	TypedEvent.freezeAllAndDo(() => {
		imports.forEach(imported => {
			const player = newPlayerFromPreset(imported.spec, imported.preset, sim, eventID);
			applyCharacter(player, imported, eventID);
			sim.raid.setPlayer(eventID, imported.raidIndex, player);
		});
	});
}

const byName = (raid: Raid, name: string) => raid.getPlayers().find(p => p != null && p.getName() == name) as Player<any> | undefined;

async function main() {
	const db = await Database.loadLeftoversIfNecessary(rosterEquipmentSpec(parseRoster(fs.readFileSync('ui/raid/acore_harness/testdata/raid.json', 'utf-8'))));
	const sim = new Sim();
	await sim.waitForInit();
	const raid = sim.raid;

	placeFresh(sim, buildImports(db, 'ui/raid/acore_harness/testdata/raid.json'));
	console.log('start:', raid.getPlayers().filter(p => p != null).length, 'players');

	// Set Druidica's innervate on Malediction, give Ecoterrorist a distinctive consume, move Angry out.
	const eventID = TypedEvent.nextEventID();
	const druidica = byName(raid, 'Druidica')!;
	const malediction = byName(raid, 'Malediction')!;
	const options = druidica.getSpecOptions() as any;
	options.innervateTarget = newUnitReference(malediction.getRaidIndex());
	druidica.setSpecOptions(eventID, options);
	const ecoterrorist = byName(raid, 'Ecoterrorist')!;
	ecoterrorist.setConsumes(eventID, Consumes.create({ food: 8, defaultPotion: 3 }));
	raid.setPlayer(eventID, 30, byName(raid, 'Angry')!);
	console.log('after fiddling: Angry at', byName(raid, 'Angry')!.getRaidIndex(), 'innervate ->', (druidica.getSpecOptions() as any).innervateTarget.index);

	console.log('\n-- update with the same roster --');
	console.log(updateRaid(sim, buildImports(db, 'ui/raid/acore_harness/testdata/raid.json')).join('\n') || '(no adds, replaces or removals)');
	console.log('Angry back at', byName(raid, 'Angry')!.getRaidIndex(), '(expect 0)');
	console.log('Ecoterrorist consumes kept:', JSON.stringify({ food: byName(raid, 'Ecoterrorist')!.getConsumes().food, pot: byName(raid, 'Ecoterrorist')!.getConsumes().defaultPotion }), '(expect food 8, pot 3)');
	console.log(
		'Druidica innervate ->',
		(byName(raid, 'Druidica')!.getSpecOptions() as any).innervateTarget.index,
		'(expect', byName(raid, 'Malediction')!.getRaidIndex() + ')',
	);
	console.log('players:', raid.getPlayers().filter(p => p != null).length, '(expect 25)');

	console.log('\n-- update with Justice reforged into a protection build --');
	console.log(updateRaid(sim, buildImports(db, 'ui/raid/acore_harness/testdata/raid-justice-prot.json')).join('\n'));
	console.log('Justice spec now', specNames[byName(raid, 'Justice')!.spec], '(expect Protection Paladin)');
	console.log('tanks:', raid.getTanks().map(t => raid.getPlayerFromUnitReference(t)?.getName() ?? 'empty').join(', '));

	console.log('\n-- update with Tree dropped from the roster --');
	console.log(updateRaid(sim, buildImports(db, 'ui/raid/acore_harness/testdata/raid-no-tree.json')).join('\n'));
	console.log('players:', raid.getPlayers().filter(p => p != null).length, '(expect 24)');
	console.log('Tree still here?', !!byName(raid, 'Tree'), '(expect false)');
	console.log('Druidica innervate ->', (byName(raid, 'Druidica')!.getSpecOptions() as any).innervateTarget.index, '(expect', byName(raid, 'Malediction')!.getRaidIndex() + ')');
}

main().catch(e => {
	console.error(e);
	process.exit(1);
});
