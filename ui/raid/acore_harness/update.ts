import * as fs from 'fs';

import { Player } from '../../core/player';
import { Consumes } from '../../core/proto/common';
import { Database } from '../../core/proto_utils/database';
import { newUnitReference, specNames } from '../../core/proto_utils/utils';
import { Raid } from '../../core/raid';
import { Sim } from '../../core/sim';
import { TypedEvent } from '../../core/typed_event';
import { ImportNotes, RaidCharacter, updateRaid } from '../acore_importer';
import {
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

// The shipped updateRaid, so this checks the real thing. Its notes come back as one line each.
function runUpdate(sim: Sim, imports: Array<RaidCharacter>): Array<string> {
	const notes = new ImportNotes();
	updateRaid(sim, imports, notes);
	return notes.added
		.map(name => `added ${name}`)
		.concat(notes.replaced.map(entry => `replaced ${entry.name}: ${specNames[entry.from]} -> ${specNames[entry.to]}`))
		.concat(notes.removed.map(name => `removed ${name}`));
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

const innervate = (raid: Raid) => (byName(raid, 'Druidica')!.getSpecOptions() as any).innervateTarget.index;

function setInnervate(raid: Raid, targetName: string) {
	const druidica = byName(raid, 'Druidica')!;
	const options = druidica.getSpecOptions() as any;
	options.innervateTarget = newUnitReference(byName(raid, targetName)!.getRaidIndex());
	druidica.setSpecOptions(TypedEvent.nextEventID(), options);
}

async function main() {
	const db = await Database.loadLeftoversIfNecessary(rosterEquipmentSpec(parseRoster(fs.readFileSync('ui/raid/acore_harness/testdata/raid.json', 'utf-8'))));
	const sim = new Sim();
	await sim.waitForInit();
	const raid = sim.raid;

	placeFresh(sim, buildImports(db, 'ui/raid/acore_harness/testdata/raid.json'));
	console.log('start:', raid.getPlayers().filter(p => p != null).length, 'players');

	// Set Druidica's innervate on Malediction, give Ecoterrorist a distinctive consume, move Angry out.
	const eventID = TypedEvent.nextEventID();
	setInnervate(raid, 'Malediction');
	const ecoterrorist = byName(raid, 'Ecoterrorist')!;
	ecoterrorist.setConsumes(eventID, Consumes.create({ food: 8, defaultPotion: 3 }));
	raid.setPlayer(eventID, 30, byName(raid, 'Angry')!);
	console.log('after fiddling: Angry at', byName(raid, 'Angry')!.getRaidIndex(), 'innervate ->', innervate(raid));

	console.log('\n-- update with the same roster --');
	console.log(runUpdate(sim, buildImports(db, 'ui/raid/acore_harness/testdata/raid.json')).join('\n') || '(no adds, replaces or removals)');
	console.log('Angry back at', byName(raid, 'Angry')!.getRaidIndex(), '(expect 0)');
	console.log('Ecoterrorist consumes kept:', JSON.stringify({ food: byName(raid, 'Ecoterrorist')!.getConsumes().food, pot: byName(raid, 'Ecoterrorist')!.getConsumes().defaultPotion }), '(expect food 8, pot 3)');
	console.log('Druidica innervate ->', innervate(raid), '(expect', byName(raid, 'Malediction')!.getRaidIndex() + ')');
	console.log('players:', raid.getPlayers().filter(p => p != null).length, '(expect 25)');

	console.log('\n-- update with Justice reforged into a protection build --');
	// a replaced raider is a new Player under the same name, so the assignment has to follow the name
	setInnervate(raid, 'Justice');
	console.log(runUpdate(sim, buildImports(db, 'ui/raid/acore_harness/testdata/raid-justice-prot.json')).join('\n'));
	console.log('Justice spec now', specNames[byName(raid, 'Justice')!.spec], '(expect Protection Paladin)');
	console.log('tanks:', raid.getTanks().map(t => raid.getPlayerFromUnitReference(t)?.getName() ?? 'empty').join(', '));
	console.log('Druidica innervate ->', innervate(raid), '(expect', byName(raid, 'Justice')!.getRaidIndex() + ', the replaced Justice)');
	setInnervate(raid, 'Malediction');

	console.log('\n-- update with Tree dropped from the roster --');
	console.log(runUpdate(sim, buildImports(db, 'ui/raid/acore_harness/testdata/raid-no-tree.json')).join('\n'));
	console.log('players:', raid.getPlayers().filter(p => p != null).length, '(expect 24)');
	console.log('Tree still here?', !!byName(raid, 'Tree'), '(expect false)');
	console.log('Druidica innervate ->', innervate(raid), '(expect', byName(raid, 'Malediction')!.getRaidIndex() + ')');
}

main().catch(e => {
	console.error(e);
	process.exit(1);
});
