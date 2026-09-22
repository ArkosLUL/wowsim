import { RaidSimPreset } from '../core/individual_sim_ui';
import { MAX_PARTY_SIZE } from '../core/party';
import { Player } from '../core/player';
import { Class, EquipmentSpec, Glyphs, ItemReforge, ItemSlot, ItemSpec, Profession, Race, Spec } from '../core/proto/common';
import { Hunter_Options_Quiver } from '../core/proto/hunter';
import { Database } from '../core/proto_utils/database';
import { Gear } from '../core/proto_utils/gear';
import { nameToProfession } from '../core/proto_utils/names';
import { isValidReforge, reforgeStatTypeName } from '../core/proto_utils/reforging';
import { getTalentTree, getTalentTreePoints, specNames } from '../core/proto_utils/utils';
import { MAX_NUM_PARTIES } from '../core/raid';
import { Sim } from '../core/sim';
import { playerTalentStringToProto } from '../core/talents/factory';
import { EventID } from '../core/typed_event';

// Roster JSON written by `go run ./tools/database/acraid`. Its shape is documented in
// docs/azerothcore-raid-import/azerothcore-raid-import.PLAN.md.
export const ROSTER_VERSION = 1;

// group_member.memberFlags bits: 1 assistant, 2 main tank, 4 main assist.
export const MEMBER_FLAG_MAIN_TANK = 2;

export interface RosterReforge {
	fromStatType: number;
	toStatType: number;
}

export interface RosterGearItem {
	acSlot: number;
	id: number;
	enchant: number;
	gems: Array<number>;
	extraGem: number;
	reforge?: RosterReforge;
}

export interface RosterGlyphs {
	major: Array<number>;
	minor: Array<number>;
}

export interface RosterCharacter {
	name: string;
	classId: number;
	raceId: number;
	swapRaceId: number;
	level: number;
	subgroup: number;
	memberFlags: number;
	talents: string;
	glyphs?: RosterGlyphs;
	professions: Array<string>;
	gear: Array<RosterGearItem>;
	// A quiver or ammo pouch in a bag slot. Only hunters use it.
	quiver?: boolean;
	warnings?: Array<string>;
}

export interface Roster {
	version: number;
	exportedAt?: string;
	group?: {
		selector?: string;
		leader?: string;
		leaderIsGroupLeader?: boolean;
	};
	warnings?: Array<string>;
	characters: Array<RosterCharacter>;
}

// Everything one roster character turns into, before it touches a Player.
export interface CharacterImport {
	char: RosterCharacter;
	playerClass: Class;
	spec: Spec;
	race: Race;
	racialTraits: Race;
	professions: Array<Profession>;
	talentsString: string;
	glyphs: Glyphs;
	equipment: EquipmentSpec;
	gear: Gear;
	warnings: Array<string>;
}

export function parseRoster(data: string): Roster {
	let json: any;
	try {
		json = JSON.parse(data);
	} catch (e: any) {
		throw new Error(`That isn't valid JSON: ${e?.message || e}`);
	}
	if (!json || typeof json != 'object' || Array.isArray(json)) {
		throw new Error('That JSON is not a roster export.');
	}
	if (json.version != ROSTER_VERSION) {
		throw new Error(`Roster version ${json.version} isn't supported, this sim reads version ${ROSTER_VERSION}.`);
	}
	if (!Array.isArray(json.characters) || json.characters.length == 0) {
		throw new Error('The roster has no characters in it.');
	}
	return json as Roster;
}

const acClassToClass: Record<number, Class> = {
	1: Class.ClassWarrior,
	2: Class.ClassPaladin,
	3: Class.ClassHunter,
	4: Class.ClassRogue,
	5: Class.ClassPriest,
	6: Class.ClassDeathknight,
	7: Class.ClassShaman,
	8: Class.ClassMage,
	9: Class.ClassWarlock,
	11: Class.ClassDruid,
};

const acRaceToRace: Record<number, Race> = {
	1: Race.RaceHuman,
	2: Race.RaceOrc,
	3: Race.RaceDwarf,
	4: Race.RaceNightElf,
	5: Race.RaceUndead,
	6: Race.RaceTauren,
	7: Race.RaceGnome,
	8: Race.RaceTroll,
	10: Race.RaceBloodElf,
	11: Race.RaceDraenei,
};

// character_inventory slots, minus shirt (3) and tabard (18) which the export skips.
const acSlotToItemSlot: Record<number, ItemSlot> = {
	0: ItemSlot.ItemSlotHead,
	1: ItemSlot.ItemSlotNeck,
	2: ItemSlot.ItemSlotShoulder,
	4: ItemSlot.ItemSlotChest,
	5: ItemSlot.ItemSlotWaist,
	6: ItemSlot.ItemSlotLegs,
	7: ItemSlot.ItemSlotFeet,
	8: ItemSlot.ItemSlotWrist,
	9: ItemSlot.ItemSlotHands,
	10: ItemSlot.ItemSlotFinger1,
	11: ItemSlot.ItemSlotFinger2,
	12: ItemSlot.ItemSlotTrinket1,
	13: ItemSlot.ItemSlotTrinket2,
	14: ItemSlot.ItemSlotBack,
	15: ItemSlot.ItemSlotMainHand,
	16: ItemSlot.ItemSlotOffHand,
	17: ItemSlot.ItemSlotRanged,
};

const classTankSpec: Partial<Record<Class, Spec>> = {
	[Class.ClassDruid]: Spec.SpecFeralTankDruid,
	[Class.ClassPaladin]: Spec.SpecProtectionPaladin,
	[Class.ClassWarrior]: Spec.SpecProtectionWarrior,
	[Class.ClassDeathknight]: Spec.SpecTankDeathknight,
};

export function rosterClass(classId: number): Class {
	return acClassToClass[classId] ?? Class.ClassUnknown;
}

export function rosterRace(raceId: number): Race {
	return acRaceToRace[raceId] ?? Race.RaceUnknown;
}

// The sim has one spec per talent build, the server has none, so the talent string decides.
export function inferSpec(playerClass: Class, talents: string, memberFlags: number): Spec {
	const tankSpec = classTankSpec[playerClass];
	if (memberFlags & MEMBER_FLAG_MAIN_TANK && tankSpec !== undefined) {
		return tankSpec;
	}

	const tab = getTalentTree(talents);
	switch (playerClass) {
		case Class.ClassWarrior:
			return tab == 2 ? Spec.SpecProtectionWarrior : Spec.SpecWarrior;
		case Class.ClassPaladin:
			return [Spec.SpecHolyPaladin, Spec.SpecProtectionPaladin, Spec.SpecRetributionPaladin][tab];
		case Class.ClassShaman:
			return [Spec.SpecElementalShaman, Spec.SpecEnhancementShaman, Spec.SpecRestorationShaman][tab];
		// Smite is a niche build nobody plays on this server, so healing priest covers both holy and disc.
		case Class.ClassPriest:
			return tab == 2 ? Spec.SpecShadowPriest : Spec.SpecHealingPriest;
		case Class.ClassDruid: {
			if (tab == 0) return Spec.SpecBalanceDruid;
			if (tab == 2) return Spec.SpecRestorationDruid;
			const talentProto = playerTalentStringToProto<Spec.SpecFeralDruid>(Spec.SpecFeralDruid, talents);
			const tanky = talentProto.thickHide == 3 || (talentProto.naturalReaction > 0 && talentProto.protectorOfThePack > 0);
			return tanky ? Spec.SpecFeralTankDruid : Spec.SpecFeralDruid;
		}
		case Class.ClassDeathknight: {
			// All three DK trees tank, so count survivability talents instead of looking at the top tree.
			const talentProto = playerTalentStringToProto<Spec.SpecDeathknight>(Spec.SpecDeathknight, talents);
			const tankTalents = [
				talentProto.toughness,
				talentProto.frigidDreadplate,
				talentProto.anticipation,
				talentProto.willOfTheNecropolis,
			].filter(points => points > 0).length;
			return tankTalents >= 2 ? Spec.SpecTankDeathknight : Spec.SpecDeathknight;
		}
		case Class.ClassHunter:
			return Spec.SpecHunter;
		case Class.ClassMage:
			return Spec.SpecMage;
		case Class.ClassRogue:
			return Spec.SpecRogue;
		case Class.ClassWarlock:
			return Spec.SpecWarlock;
	}
	throw new Error(`class ${playerClass} has no spec in this sim`);
}

// Closest preset by talent points per tree. Picks the rotation and consumes the player starts with.
export function matchPreset(presets: Array<RaidSimPreset<any>>, spec: Spec, talents: string): RaidSimPreset<any> {
	const matching = presets.filter(preset => preset.spec == spec);
	if (matching.length == 0) {
		throw new Error(`the sim has no ${specNames[spec]} preset`);
	}

	const points = getTalentTreePoints(talents);
	let best = matching[0];
	let bestDistance = Number.MAX_SAFE_INTEGER;
	matching.forEach(preset => {
		const presetPoints = getTalentTreePoints(preset.talents.talentsString);
		const distance = presetPoints.reduce((acc, presetTree, i) => acc + Math.abs((points[i] || 0) - presetTree), 0);
		if (distance < bestDistance) {
			best = preset;
			bestDistance = distance;
		}
	});
	return best;
}

// Every item the roster mentions, so Database.loadLeftoversIfNecessary can decide in one call.
export function rosterEquipmentSpec(roster: Roster): EquipmentSpec {
	return EquipmentSpec.create({
		items: roster.characters.map(char => (char.gear || []).map(item => ItemSpec.create({ id: item.id }))).flat(),
	});
}

export function buildCharacterImport(db: Database, char: RosterCharacter): CharacterImport {
	const warnings: Array<string> = [];

	const playerClass = rosterClass(char.classId);
	if (playerClass == Class.ClassUnknown) {
		throw new Error(`class id ${char.classId} isn't a WotLK class`);
	}
	const race = rosterRace(char.raceId);
	if (race == Race.RaceUnknown) {
		throw new Error(`race id ${char.raceId} isn't a WotLK race`);
	}

	let racialTraits = Race.RaceUnknown;
	if (char.swapRaceId) {
		racialTraits = rosterRace(char.swapRaceId);
		if (racialTraits == Race.RaceUnknown) {
			warnings.push(`racial trait swap to race id ${char.swapRaceId} is unknown, using the real race`);
		}
	}

	if (char.level != 80) {
		warnings.push(`is level ${char.level}, the sim only models level 80`);
	}

	const professions = (char.professions || []).map(name => {
		const profession = nameToProfession(name);
		if (profession == Profession.ProfessionUnknown) {
			warnings.push(`unknown profession '${name}'`);
		}
		return profession;
	});

	const equipment = buildEquipmentSpec(db, char, warnings);

	return {
		char: char,
		playerClass: playerClass,
		spec: inferSpec(playerClass, char.talents, char.memberFlags),
		race: race,
		racialTraits: racialTraits,
		professions: professions,
		talentsString: char.talents,
		glyphs: buildGlyphs(db, char, warnings),
		equipment: equipment,
		// Throws "No slots left" on gear the sim can't wear, which keeps it out of the import freeze.
		gear: db.lookupEquipmentSpec(equipment),
		warnings: warnings,
	};
}

function buildEquipmentSpec(db: Database, char: RosterCharacter, warnings: Array<string>): EquipmentSpec {
	const slotted = (char.gear || [])
		.map(item => ({ item: item, slot: acSlotToItemSlot[item.acSlot] }))
		.filter(entry => {
			if (entry.slot === undefined) {
				warnings.push(`equipment slot ${entry.item.acSlot} has no sim slot, item ${entry.item.id} dropped`);
				return false;
			}
			return true;
		});

	// lookupEquipmentSpec ignores the index and fills the first free eligible slot, so rings and
	// trinkets only land right when the list is in sim slot order.
	slotted.sort((a, b) => a.slot! - b.slot!);

	return EquipmentSpec.create({
		items: slotted.map(entry => buildItemSpec(db, entry.item, warnings)),
	});
}

function buildItemSpec(db: Database, item: RosterGearItem, warnings: Array<string>): ItemSpec {
	const spec = ItemSpec.create({ id: item.id, enchant: item.enchant });
	const equipped = db.lookupItemSpec(spec);
	if (!equipped) {
		warnings.push(`item ${item.id} isn't in the sim's item database`);
		spec.gems = trimTrailingZeros((item.gems || []).concat(item.extraGem ? [item.extraGem] : []));
		return spec;
	}

	const simItem = equipped.item;
	if (item.enchant != 0 && !equipped.enchant) {
		warnings.push(`enchant ${item.enchant} on ${simItem.name} isn't in the sim's database`);
	}

	const nativeSockets = simItem.gemSockets.length;
	const rosterGems = item.gems || [];
	if (rosterGems.length != nativeSockets) {
		warnings.push(`${simItem.name} has ${rosterGems.length} sockets on the server but ${nativeSockets} in the sim`);
	}

	const gems = rosterGems.slice(0, nativeSockets);
	while (gems.length < nativeSockets) {
		gems.push(0);
	}
	// Belt buckle or a Blacksmithing socket. EquippedItem only keeps that gem on a waist, wrist or
	// hands item; anywhere else it would add stats out of nowhere.
	if (item.extraGem) {
		if (equipped.couldHaveExtraSocket()) {
			gems.push(item.extraGem);
		} else {
			warnings.push(`dropped the extra gem on ${simItem.name}, the sim gives that slot no extra socket`);
		}
	}
	gems.forEach(gemId => {
		if (gemId != 0 && !db.lookupGem(gemId)) {
			warnings.push(`gem ${gemId} on ${simItem.name} isn't in the sim's database`);
		}
	});
	spec.gems = trimTrailingZeros(gems);

	if (item.reforge) {
		const reforge = ItemReforge.create({ fromStatType: item.reforge.fromStatType, toStatType: item.reforge.toStatType });
		if (isValidReforge(simItem, reforge)) {
			spec.reforge = reforge;
		} else {
			const label = `${reforgeStatTypeName(reforge.fromStatType)} to ${reforgeStatTypeName(reforge.toStatType)}`;
			warnings.push(`dropped the ${label} reforge on ${simItem.name}, the sim's copy of the item can't take it`);
		}
	}

	return spec;
}

function buildGlyphs(db: Database, char: RosterCharacter, warnings: Array<string>): Glyphs {
	const major = (char.glyphs?.major || []).map(spellId => glyphItemId(db, spellId, warnings));
	const minor = (char.glyphs?.minor || []).map(spellId => glyphItemId(db, spellId, warnings));
	return Glyphs.create({
		major1: major[0] || 0,
		major2: major[1] || 0,
		major3: major[2] || 0,
		minor1: minor[0] || 0,
		minor2: minor[1] || 0,
		minor3: minor[2] || 0,
	});
}

function glyphItemId(db: Database, spellId: number, warnings: Array<string>): number {
	if (!spellId) {
		return 0;
	}
	const itemId = db.glyphSpellToItemId(spellId);
	if (!itemId) {
		warnings.push(`glyph spell ${spellId} isn't in the sim's database`);
	}
	return itemId;
}

function trimTrailingZeros(gems: Array<number>): Array<number> {
	const trimmed = gems.slice();
	while (trimmed.length > 0 && !trimmed[trimmed.length - 1]) {
		trimmed.pop();
	}
	return trimmed;
}

// Everything the roster knows about a character. Rotation, consumes and spec options stay put,
// except a hunter's quiver: that's a bag slot on the server, not a choice, so it follows the gear.
export function applyCharacter(player: Player<any>, imported: CharacterImport, eventID: EventID) {
	player.setName(eventID, imported.char.name);
	player.setRace(eventID, imported.race);
	player.setRacialTraits(eventID, imported.racialTraits);
	player.setProfessions(eventID, imported.professions);
	player.setTalentsString(eventID, imported.talentsString);
	player.setGlyphs(eventID, imported.glyphs);
	player.setGear(eventID, imported.gear);
	if (imported.playerClass == Class.ClassHunter) {
		const options = player.getSpecOptions() as any;
		options.quiver = imported.char.quiver ? Hunter_Options_Quiver.Quiver15Percent : Hunter_Options_Quiver.QuiverNone;
		player.setSpecOptions(eventID, options);
	}
}

// Same starting point a preset dragged onto the raid grid gets, minus what applyCharacter sets.
export function newPlayerFromPreset(spec: Spec, preset: RaidSimPreset<any>, sim: Sim, eventID: EventID): Player<any> {
	const player = new Player(spec, sim);
	player.applySharedDefaults(eventID);
	player.setSpecOptions(eventID, preset.specOptions);
	player.setConsumes(eventID, preset.consumes);
	player.setDistanceFromTarget(eventID, preset.otherDefaults?.distanceFromTarget || 0);
	return player;
}

// Raid index = subgroup * 5 + the character's position in that subgroup. -1 when it doesn't fit.
export function assignRaidIndexes(chars: Array<RosterCharacter>): Array<number> {
	const filled: Record<number, number> = {};
	return chars.map(char => {
		const subgroup = char.subgroup || 0;
		const position = filled[subgroup] || 0;
		filled[subgroup] = position + 1;
		if (subgroup < 0 || subgroup >= MAX_NUM_PARTIES || position >= MAX_PARTY_SIZE) {
			return -1;
		}
		return subgroup * MAX_PARTY_SIZE + position;
	});
}

export function activeParties(chars: Array<RosterCharacter>): number {
	const highest = Math.max(0, ...chars.map(char => (char.subgroup || 0) + 1));
	return Math.min(MAX_NUM_PARTIES, Math.max(5, highest));
}
