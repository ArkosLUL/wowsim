import * as fs from 'fs';

import { ItemSlot } from '../../core/proto/common';
import { Database } from '../../core/proto_utils/database';
import { specNames } from '../../core/proto_utils/utils';
import {
	activeParties,
	assignRaidIndexes,
	buildCharacterImport,
	matchPreset,
	parseRoster,
	rosterEquipmentSpec,
	rosterRace,
} from '../acore_roster';
import { playerPresets } from '../presets';

async function main() {
	const roster = parseRoster(fs.readFileSync(process.argv[2], 'utf-8'));
	const indexes = assignRaidIndexes(roster.characters);
	const db = await Database.loadLeftoversIfNecessary(rosterEquipmentSpec(roster));

	console.log(`version ${roster.version}, ${roster.characters.length} characters, ${activeParties(roster.characters)} parties`);
	console.log(`top-level warnings: ${JSON.stringify(roster.warnings || [])}`);

	let totalReforges = 0;
	let keptReforges = 0;
	let totalGems = 0;
	let keptGems = 0;
	const allWarnings: Array<string> = [];

	roster.characters.forEach((char, i) => {
		const imported = buildCharacterImport(db, char);
		const preset = matchPreset(playerPresets, imported.spec, imported.talentsString);
		const gearSpec = imported.gear.asSpec();
		const equipped = gearSpec.items.filter(item => item.id != 0).length;
		const reforges = gearSpec.items.filter(item => !!item.reforge).length;
		const gems = gearSpec.items.map(item => item.gems.filter(gem => gem != 0).length).reduce((a, b) => a + b, 0);
		const rosterReforges = char.gear.filter(item => !!item.reforge).length;
		const rosterGems = char.gear.map(item => item.gems.filter(g => g != 0).length + (item.extraGem ? 1 : 0)).reduce((a, b) => a + b, 0);
		totalReforges += rosterReforges;
		keptReforges += reforges;
		totalGems += rosterGems;
		keptGems += gems;
		imported.warnings.forEach(w => allWarnings.push(`${char.name}: ${w}`));

		const mainHand = imported.gear.getEquippedItem(ItemSlot.ItemSlotMainHand);
		const ranged = imported.gear.getEquippedItem(ItemSlot.ItemSlotRanged);
		console.log(
			[
				char.name.padEnd(13),
				`idx ${String(indexes[i]).padStart(2)}`,
				specNames[imported.spec].padEnd(21),
				`preset=${preset.defaultName.padEnd(14)}`,
				`race=${imported.race}`,
				`traits=${imported.racialTraits}`,
				`profs=${imported.professions.length}`,
				`glyphs=${[imported.glyphs.major1, imported.glyphs.major2, imported.glyphs.major3, imported.glyphs.minor1, imported.glyphs.minor2, imported.glyphs.minor3].filter(g => g != 0).length}`,
				`items=${equipped}/${char.gear.length}`,
				`gems=${gems}/${rosterGems}`,
				`reforges=${reforges}/${rosterReforges}`,
				`mh=${mainHand ? mainHand.id : 'none'}`,
				`ranged=${ranged ? ranged.id : 'none'}`,
			].join(' '),
		);
	});

	console.log(`\ngems kept ${keptGems}/${totalGems}, reforges kept ${keptReforges}/${totalReforges}`);
	console.log(`per-character warnings (${allWarnings.length}):`);
	allWarnings.forEach(w => console.log(`  ${w}`));

	// Druidica is the racial-swap case: a Night Elf with Draenei traits.
	const druidica = roster.characters.find(c => c.name == 'Druidica')!;
	const druidicaImport = buildCharacterImport(db, druidica);
	console.log(
		`\nDruidica race=${druidicaImport.race} (expect ${rosterRace(4)}) traits=${druidicaImport.racialTraits} (expect ${rosterRace(11)})`,
	);
}

main().catch(e => {
	console.error(e);
	process.exit(1);
});
