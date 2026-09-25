import * as fs from 'fs';

import { Player } from '../../core/player';
import { Consumes, Flask, Food, Spec } from '../../core/proto/common';
import { Hunter_Options, Hunter_Options_Ammo as Ammo, Hunter_Options_PetType as PetType, HunterPetTalents } from '../../core/proto/hunter';
import { Warlock_Options, Warlock_Options_Summon as Summon } from '../../core/proto/warlock';
import { Database } from '../../core/proto_utils/database';
import { Sim } from '../../core/sim';
import { protoToTalentString } from '../../core/talents/factory';
import { getPetTalentsConfig } from '../../core/talents/hunter_pet';
import { TypedEvent } from '../../core/typed_event';
import { ImportNotes, RaidCharacter, updateRaid } from '../acore_importer';
import {
	applyCharacter,
	applyConsumes,
	assignRaidIndexes,
	buildCharacterImport,
	CharacterImport,
	describeConsumeSource,
	describePetAndAmmo,
	matchPreset,
	MEMBER_FLAG_MAIN_TANK,
	newPlayerFromPreset,
	parseRoster,
	Roster,
	rosterEquipmentSpec,
} from '../acore_roster';
import { playerPresets } from '../presets';

// The version 2 fields: pets, ammo and consumables, through Replace, Update and a version 1 roster.
const FIXTURE = 'ui/raid/acore_harness/testdata/loadout.json';

let failures = 0;
function check(label: string, got: unknown, want: unknown) {
	const same = JSON.stringify(got) == JSON.stringify(want);
	if (!same) {
		failures++;
	}
	console.log(`${same ? 'ok  ' : 'FAIL'} ${label}: ${JSON.stringify(got)}${same ? '' : ` (expect ${JSON.stringify(want)})`}`);
}

const readFixture = (): Roster => JSON.parse(fs.readFileSync(FIXTURE, 'utf-8'));

function buildImports(db: Database, roster: Roster): Array<RaidCharacter> {
	const parsed = parseRoster(JSON.stringify(roster));
	const indexes = assignRaidIndexes(parsed.characters);
	return parsed.characters.map((char, i) => {
		const imported = buildCharacterImport(db, char);
		return {
			...imported,
			preset: matchPreset(playerPresets, imported.spec, imported.talentsString),
			raidIndex: indexes[i],
			isMainTank: (char.memberFlags & MEMBER_FLAG_MAIN_TANK) != 0,
		};
	});
}

// Replace's per-player steps, without the modal.
function placeFresh(sim: Sim, imports: Array<RaidCharacter>) {
	const eventID = TypedEvent.nextEventID();
	TypedEvent.freezeAllAndDo(() => {
		imports.forEach(imported => {
			const player = newPlayerFromPreset(imported.spec, imported.preset, sim, eventID);
			applyCharacter(player, imported, eventID);
			applyConsumes(player, imported, eventID);
			sim.raid.setPlayer(eventID, imported.raidIndex, player);
		});
	});
}

const byName = (sim: Sim, name: string) => sim.raid.getPlayers().find(p => p != null && p.getName() == name) as Player<any>;
const hunterOptions = (sim: Sim, name: string) => byName(sim, name).getSpecOptions() as Hunter_Options;
const petTalents = (options: Hunter_Options) => protoToTalentString(options.petTalents || HunterPetTalents.create(), getPetTalentsConfig(options.petType));

function presetOf(spec: Spec, talents: string) {
	return matchPreset(playerPresets, spec, talents);
}

async function main() {
	const fixture = readFixture();
	const db = await Database.loadLeftoversIfNecessary(rosterEquipmentSpec(fixture));
	const sim = new Sim();
	await sim.waitForInit();

	console.log('-- parsing --');
	check('version 2 accepted', parseRoster(JSON.stringify(fixture)).version, 2);
	check('version 1 accepted', parseRoster(JSON.stringify({ ...fixture, version: 1 })).version, 1);
	let rejected = '';
	try {
		parseRoster(JSON.stringify({ ...fixture, version: 3 }));
	} catch (e: any) {
		rejected = e.message;
	}
	check('version 3 rejected', rejected, "Roster version 3 isn't supported, this sim reads versions 1 and 2.");

	console.log('\n-- what each character turns into --');
	const imports = buildImports(db, fixture);
	const imp = (name: string) => imports.find(i => i.char.name == name) as CharacterImport;
	check('Fletcher pet and ammo', describePetAndAmmo(imp('Fletcher')), 'Cat with its talents, Iceblade Arrow');
	check('Burrower pet and ammo', describePetAndAmmo(imp('Burrower')), 'Worm, no talents');
	check('Grimoire demon', describePetAndAmmo(imp('Grimoire')), 'Succubus');
	check('Pactbound demon', describePetAndAmmo(imp('Pactbound')), '');
	check('Anvil pet and ammo', describePetAndAmmo(imp('Anvil')), '');
	check('Fletcher consumables', describeConsumeSource(imp('Fletcher')), 'bot rules');
	check('Grimoire consumables', describeConsumeSource(imp('Grimoire')), 'saved buffs and bags');
	check('Frostfinger consumables', describeConsumeSource(imp('Frostfinger')), 'bags');
	check('Burrower skipped', imp('Burrower').loadout.skipped, ['ammo item 99999', 'flask (spell 67890, item 12345)', 'potion (item 88888)']);
	check('Pactbound skipped', imp('Pactbound').loadout.skipped, ['pet Doomguard (family 19)']);
	check('Burrower consumes set', Object.keys(imp('Burrower').loadout.consumes).length, 10);
	check('no UI-side warnings on the fixture', imports.flatMap(i => i.warnings), []);

	console.log('\n-- Replace --');
	placeFresh(sim, imports);
	const fletcher = hunterOptions(sim, 'Fletcher');
	check('Fletcher pet', PetType[fletcher.petType], 'Cat');
	check('Fletcher pet talents', petTalents(fletcher), '21000230300030101');
	check('Fletcher ammo', Ammo[fletcher.ammo], 'IcebladeArrow');
	const fletcherConsumes = byName(sim, 'Fletcher').getConsumes();
	check('Fletcher flask', Flask[fletcherConsumes.flask], 'FlaskOfEndlessRage');
	check('Fletcher food', Food[fletcherConsumes.food], 'FoodHeartyRhino');
	check('Fletcher pet food, the preset has one', fletcherConsumes.petFood, 0);

	const burrower = hunterOptions(sim, 'Burrower');
	const burrowerPreset = presetOf(Spec.SpecHunter, imp('Burrower').talentsString);
	check('Burrower pet', PetType[burrower.petType], 'Worm');
	check('Burrower pet talents', petTalents(burrower), '');
	check('Burrower ammo stays the preset', burrower.ammo, (burrowerPreset.specOptions as Hunter_Options).ammo);
	const burrowerConsumes = byName(sim, 'Burrower').getConsumes();
	check('Burrower flask stays the preset', burrowerConsumes.flask, burrowerPreset.consumes.flask);
	check('Burrower potion stays the preset', burrowerConsumes.defaultPotion, burrowerPreset.consumes.defaultPotion);
	check('Burrower food, not in the roster, stays the preset', burrowerConsumes.food, burrowerPreset.consumes.food);

	const grimoire = byName(sim, 'Grimoire');
	check('Grimoire summon', Summon[(grimoire.getSpecOptions() as Warlock_Options).summon], 'Succubus');
	check(
		'Grimoire bags',
		Consumes.toJson(grimoire.getConsumes()),
		Consumes.toJson(
			Consumes.fromJson({
				flask: 'FlaskOfTheFrostWyrm',
				food: 'FoodFirecrackerSalmon',
				defaultPotion: 'RunicManaPotion',
				prepopPotion: 'PotionOfWildMagic',
				defaultConjured: 'ConjuredHealthstone',
				thermalSapper: true,
				fillerExplosive: 'ExplosiveSaroniteBomb',
			}),
		),
	);
	const pactbound = byName(sim, 'Pactbound');
	check(
		'Pactbound summon stays the preset',
		(pactbound.getSpecOptions() as Warlock_Options).summon,
		(presetOf(Spec.SpecWarlock, imp('Pactbound').talentsString).specOptions as Warlock_Options).summon,
	);

	const replaceNotes = new ImportNotes();
	console.log('\nReplace summary, as the alert shows it:');
	console.log(replaceNotes.summarize('replace', fixture, imports, [], true));

	console.log('\n-- Update without refreshing consumables --');
	const changed = readFixture();
	const fletcherChar = changed.characters.find(c => c.name == 'Fletcher')!;
	fletcherChar.ammo = { itemId: 41165, dps: 67.5, value: 'SaroniteRazorheads' };
	fletcherChar.pet = { name: 'Whiskers', family: 2, familyName: 'Cat', petType: 'Cat', talents: '2100203' };
	fletcherChar.consumes!.flask = { value: 'FlaskOfPureMojo', source: 'matrix', itemId: 46378, spellId: 54212 };
	const newcomer = JSON.parse(JSON.stringify(fletcherChar));
	newcomer.name = 'Newcomer';
	newcomer.subgroup = 2;
	changed.characters.push(newcomer);

	let notes = new ImportNotes();
	updateRaid(sim, buildImports(db, changed), notes, false);
	check('Fletcher ammo follows the server', Ammo[hunterOptions(sim, 'Fletcher').ammo], 'SaroniteRazorheads');
	check('Fletcher pet talents follow the server', petTalents(hunterOptions(sim, 'Fletcher')), '2100203');
	check('Fletcher flask kept', Flask[byName(sim, 'Fletcher').getConsumes().flask], 'FlaskOfEndlessRage');
	check('Newcomer gets the roster flask', Flask[byName(sim, 'Newcomer').getConsumes().flask], 'FlaskOfPureMojo');
	check('kept consumables', notes.consumesKept, ['Anvil', 'Fletcher', 'Burrower', 'Grimoire', 'Pactbound', 'Frostfinger']);
	console.log('\nUpdate summary, full:');
	console.log(notes.summarize('update', changed, buildImports(db, changed), []));

	console.log('\n-- Update refreshing consumables --');
	notes = new ImportNotes();
	updateRaid(sim, buildImports(db, changed), notes, true);
	check('Fletcher flask refreshed', Flask[byName(sim, 'Fletcher').getConsumes().flask], 'FlaskOfPureMojo');
	check('nothing kept', notes.consumesKept, []);

	console.log('\n-- Update from a version 1 roster --');
	const oldFormat: any = readFixture();
	oldFormat.version = 1;
	oldFormat.characters.forEach((char: any) => {
		delete char.bot;
		delete char.pet;
		delete char.ammo;
		delete char.consumes;
	});
	const custom = byName(sim, 'Frostfinger');
	custom.setConsumes(TypedEvent.nextEventID(), Consumes.create({ flask: Flask.FlaskOfPureMojo, food: Food.FoodFishFeast }));
	notes = new ImportNotes();
	updateRaid(sim, buildImports(db, oldFormat), notes, true);
	check('Frostfinger consumes untouched', Consumes.toJson(byName(sim, 'Frostfinger').getConsumes()), { flask: 'FlaskOfPureMojo', food: 'FoodFishFeast' });
	check('Fletcher pet untouched', petTalents(hunterOptions(sim, 'Fletcher')), '2100203');
	check('Fletcher ammo untouched', Ammo[hunterOptions(sim, 'Fletcher').ammo], 'SaroniteRazorheads');
	const oldSummary = notes.summarize('update', oldFormat, buildImports(db, oldFormat), [], true);
	check('version 1 summary line', oldSummary.split('\n').filter(line => line.startsWith('Pets, ammo')), [
		"Pets, ammo and consumables: a version 1 roster has none, so they're the sim's.",
	]);

	console.log('\n-- values the sim does not have --');
	const odd = readFixture();
	const oddHunter = odd.characters.find(c => c.name == 'Fletcher')!;
	oddHunter.pet = { name: 'X', family: 99, familyName: 'Dragon', petType: 'Dragon', talents: '123' };
	oddHunter.ammo = { itemId: 1, dps: 1, value: 'MithrilSlug' };
	oddHunter.consumes = {
		flask: { value: 'FlaskOfTheFuture', source: 'matrix', itemId: 5 },
		thermalSapper: { value: 'yes', source: 'bags' },
		pocketLint: { value: 1, source: 'bags' },
		food: { value: 'FoodFishFeast', source: 'bags' },
	};
	const oddWarlock = odd.characters.find(c => c.name == 'Grimoire')!;
	oddWarlock.pet = { name: 'Y', family: 16, familyName: 'Voidwalker', summon: 'Infernal' };
	const oddImports = buildImports(db, odd);
	const oddFletcher = oddImports.find(i => i.char.name == 'Fletcher')!;
	check('odd hunter loadout', { petType: oddFletcher.loadout.petType, ammo: oddFletcher.loadout.ammo, consumes: oddFletcher.loadout.consumes }, {
		consumes: { food: Food.FoodFishFeast },
	});
	check('odd hunter skipped', oddFletcher.loadout.skipped, ['pet Dragon (family 99)', 'ammo item 1', 'flask (item 5)', 'thermal sapper']);
	oddFletcher.warnings.forEach(warning => console.log(`     warning: ${warning}`));
	const oddGrimoire = oddImports.find(i => i.char.name == 'Grimoire')!;
	check('odd warlock summon', oddGrimoire.loadout.summon, undefined);
	oddGrimoire.warnings.forEach(warning => console.log(`     warning: ${warning}`));

	const badTalents = readFixture();
	badTalents.characters.find(c => c.name == 'Fletcher')!.pet!.talents = '2100-203';
	const badImport = buildImports(db, badTalents).find(i => i.char.name == 'Fletcher')!;
	check('pet talents across two trees', { petType: badImport.loadout.petType, talents: badImport.loadout.petTalents }, { petType: PetType.Cat });
	badImport.warnings.forEach(warning => console.log(`     warning: ${warning}`));

	const noPet = readFixture();
	noPet.characters.find(c => c.name == 'Fletcher')!.pet = { name: 'Z', family: 0, familyName: '', petType: 'PetNone', talents: '' };
	noPet.characters.find(c => c.name == 'Fletcher')!.ammo = { itemId: 0, dps: 0, value: 'AmmoNone' };
	check('PetNone and AmmoNone', describePetAndAmmo(buildImports(db, noPet).find(i => i.char.name == 'Fletcher')!), 'no pet, no ammo');

	console.log(`\n${failures} failures`);
	if (failures) {
		process.exit(1);
	}
}

main().catch(e => {
	console.error(e);
	process.exit(1);
});
