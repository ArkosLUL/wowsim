import * as fs from 'fs';

import { Player } from '../../core/player';
import { Spec } from '../../core/proto/common';
import { Database } from '../../core/proto_utils/database';
import { specNames } from '../../core/proto_utils/utils';
import { Sim } from '../../core/sim';
import { TypedEvent } from '../../core/typed_event';
import { applyCharacter, assignRaidIndexes, buildCharacterImport, matchPreset, newPlayerFromPreset, parseRoster, rosterEquipmentSpec } from '../acore_roster';
import { playerPresets } from '../presets';

async function main() {
	const roster = parseRoster(fs.readFileSync(process.argv[2], 'utf-8'));
	const db = await Database.loadLeftoversIfNecessary(rosterEquipmentSpec(roster));
	const sim = new Sim();
	await sim.waitForInit(); // sim.db is null until this resolves
	const raid = sim.raid;
	const eventID = TypedEvent.nextEventID();
	const indexes = assignRaidIndexes(roster.characters);

	const placed: Array<{ name: string; index: number; player: Player<any>; spec: Spec }> = [];
	TypedEvent.freezeAllAndDo(() => {
		roster.characters.forEach((char, i) => {
			const imported = buildCharacterImport(db, char);
			const preset = matchPreset(playerPresets, imported.spec, imported.talentsString);
			const player = newPlayerFromPreset(imported.spec, preset, sim, eventID);
			applyCharacter(player, imported, eventID);
			raid.setPlayer(eventID, indexes[i], player);
			placed.push({ name: char.name, index: indexes[i], player: player, spec: imported.spec });
		});
	});

	console.log('players in raid:', raid.getPlayers().filter(p => p != null).length);
	let wrong = 0;
	placed.forEach(entry => {
		if (entry.player.getRaidIndex() != entry.index) {
			console.log(`MISPLACED ${entry.name}: wanted ${entry.index}, got ${entry.player.getRaidIndex()}`);
			wrong++;
		}
	});
	console.log('misplaced:', wrong);

	const bulwark = placed.find(p => p.name == 'Bulwark')!;
	console.log(`Bulwark at raid index ${bulwark.player.getRaidIndex()}, spec ${specNames[bulwark.player.spec]}, gear slots ${bulwark.player.getGear().asSpec().items.filter(i => i.id != 0).length}`);
	const mainHand = bulwark.player.getGear().asSpec().items.find(i => i.id == 45442);
	console.log('Bulwark main hand enchant in gear:', mainHand?.enchant, '(expect 3851, Titanguard)');

	const angry = placed.find(p => p.name == 'Angry')!;
	const angryGear = angry.player.getGear().asSpec();
	console.log('Angry reforges in gear:', angryGear.items.filter(i => !!i.reforge).length, '(expect 3)');
	console.log('Angry professions:', angry.player.getProfessions().length, '(expect 11)');
	console.log('Angry racial traits:', angry.player.getRacialTraits(), 'race:', angry.player.getRace());

	const druidica = placed.find(p => p.name == 'Druidica')!;
	console.log('Druidica effective racial traits:', druidica.player.getEffectiveRacialTraits(), '(expect 2 = Draenei)');
	console.log('Druidica talents:', druidica.player.getTalentsString().slice(0, 20) + '...');

	// Move Angry out of his party, then put everyone back where the roster says, like Update does.
	raid.setPlayer(eventID, 30, angry.player);
	console.log('Angry after a manual move:', angry.player.getRaidIndex(), '(expect 30)');
	TypedEvent.freezeAllAndDo(() => {
		for (let i = 0; i < 40; i++) {
			raid.setPlayer(eventID, i, null);
		}
		placed.forEach(entry => raid.setPlayer(eventID, entry.index, entry.player));
	});
	console.log('Angry after the re-place:', angry.player.getRaidIndex(), '(expect 0)');
	console.log('players after the re-place:', raid.getPlayers().filter(p => p != null).length, '(expect 25)');
}

main().catch(e => {
	console.error(e);
	process.exit(1);
});
