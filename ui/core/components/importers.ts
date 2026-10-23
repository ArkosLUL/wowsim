import { JsonObject } from '@protobuf-ts/runtime';
import pako from 'pako';

import {
	applyConsumes,
	applyPetAndAmmo,
	buildCharacterImport,
	describeConsumeSource,
	describePetAndAmmo,
	heroicPresenceSource,
	inferSpec,
	parseRoster,
	Roster,
	RosterCharacter,
	rosterClass,
	rosterEquipmentSpec,
} from '../../raid/acore_roster';
import { fetchServerRoster } from '../../raid/acore_server';
import { IndividualSimUI } from '../individual_sim_ui';
import {
	Class,
	EquipmentSpec,
	Glyphs,
	ItemSlot,
	ItemSpec,
	PartyBuffs,
	Profession,
	Race,
	Spec,
} from '../proto/common';
import { IndividualSimSettings } from '../proto/ui';
import { Database } from '../proto_utils/database';
import { classNames, nameToClass, nameToProfession,nameToRace } from '../proto_utils/names';
import { getTalentPoints, getTalentTree, isHealingSpec, specNames } from '../proto_utils/utils';
import { SimSettingCategories } from '../sim';
import { SimUI } from '../sim_ui';
import { classGlyphsConfig, classTalentsConfig, talentSpellIdsToTalentString } from '../talents/factory';
import { GlyphConfig } from '../talents/glyphs_picker';
import { TypedEvent } from '../typed_event';
import { buf2hex, getEnumValues } from '../utils';
import { BaseModal } from './base_modal';
import { simServerAvailable } from './individual_sim_ui/optimizer_tab';

export abstract class Importer extends BaseModal {
	protected readonly textElem: HTMLTextAreaElement;
	protected readonly descriptionElem: HTMLElement;
	protected readonly importButton: HTMLButtonElement;
	private readonly includeFile: boolean;

	constructor(parent: HTMLElement, simUI: SimUI, title: string, includeFile: boolean) {
		super(parent, 'importer', { title: title, footer: true });
		this.includeFile = includeFile;
		const uploadInputId = 'upload-input-' + title.toLowerCase().replaceAll(' ', '-');

		this.body.innerHTML = `
			<div class="import-description"></div>
			<textarea spellCheck="false" class="importer-textarea form-control"></textarea>
		`;
		this.footer!.innerHTML = `
			${
				this.includeFile
					? `
				<label for="${uploadInputId}" class="importer-button btn btn-primary upload-button me-2">
					<i class="fas fa-file-arrow-up"></i>
					Upload File
				</label>
				<input type="file" id="${uploadInputId}" class="importer-upload-input d-none" hidden>
			`
					: ''
			}
			<button class="importer-button btn btn-primary import-button">
				<i class="fa fa-download"></i>
				Import
			</button>
		`;

		this.textElem = this.rootElem.getElementsByClassName('importer-textarea')[0] as HTMLTextAreaElement;
		this.descriptionElem = this.rootElem.getElementsByClassName('import-description')[0] as HTMLElement;

		if (this.includeFile) {
			const uploadInput = this.rootElem.getElementsByClassName('importer-upload-input')[0] as HTMLButtonElement;
			uploadInput.addEventListener('change', async event => {
				const data: string = await (event as any).target.files[0].text();
				this.textElem.value = data;
				this.onTextChanged(data);
			});
		}
		this.textElem.addEventListener('input', () => this.onTextChanged(this.textElem.value));

		this.importButton = this.rootElem.getElementsByClassName('import-button')[0] as HTMLButtonElement;
		this.importButton.addEventListener('click', async () => {
			try {
				await this.onImport(this.textElem.value || '');
			} catch (error: any) {
				alert(`Import error:
${error?.message}`);
			}
		});
	}

	abstract onImport(data: string): Promise<void>;

	// Fires when the pasted or uploaded text changes, for importers whose controls depend on it.
	protected onTextChanged(_data: string) {
		// most importers only read the text when Import is clicked
	}

	protected async finishIndividualImport<SpecType extends Spec>(
		simUI: IndividualSimUI<SpecType>,
		charClass: Class,
		race: Race,
		equipmentSpec: EquipmentSpec,
		talentsStr: string,
		glyphs: Glyphs | null,
		professions: Array<Profession>,
	): Promise<void> {
		const { missingItems, missingEnchants } = await applyIndividualImport(simUI, charClass, race, equipmentSpec, talentsStr, glyphs, professions);
		setAcoreImport(simUI, null);

		this.close();

		if (missingItems.length == 0 && missingEnchants.length == 0) {
			alert('Import successful!');
		} else {
			alert(
				'Import successful, but the following IDs were not found in the sim database:' +
					(missingItems.length == 0 ? '' : '\n\nItems: ' + missingItems.join(', ')) +
					(missingEnchants.length == 0 ? '' : '\n\nEnchants: ' + missingEnchants.join(', ')),
			);
		}
	}
}

// Puts the parsed character on the page. Throws on the wrong class; tells which ids the sim database lacks.
async function applyIndividualImport<SpecType extends Spec>(
	simUI: IndividualSimUI<SpecType>,
	charClass: Class,
	race: Race,
	equipmentSpec: EquipmentSpec,
	talentsStr: string,
	glyphs: Glyphs | null,
	professions: Array<Profession>,
): Promise<{ missingItems: Array<number>; missingEnchants: Array<number> }> {
	const playerClass = simUI.player.getClass();
	if (charClass != playerClass) {
		throw new Error(`Wrong Class! Expected ${classNames.get(playerClass)} but found ${classNames.get(charClass)}!`);
	}

	await Database.loadLeftoversIfNecessary(equipmentSpec);

	const gear = simUI.sim.db.lookupEquipmentSpec(equipmentSpec);

	const expectedEnchantIds = equipmentSpec.items.map(item => item.enchant);
	const foundEnchantIds = gear.asSpec().items.map(item => item.enchant);
	const missingEnchants = expectedEnchantIds.filter(expectedId => expectedId != 0 && !foundEnchantIds.includes(expectedId));

	const expectedItemIds = equipmentSpec.items.map(item => item.id);
	const foundItemIds = gear.asSpec().items.map(item => item.id);
	const missingItems = expectedItemIds.filter(expectedId => !foundItemIds.includes(expectedId));

	const eventID = TypedEvent.nextEventID();
	TypedEvent.freezeAllAndDo(() => {
		simUI.player.setRace(eventID, race);
		simUI.player.setRacialTraits(eventID, Race.RaceUnknown);
		simUI.player.setGear(eventID, gear);
		if (talentsStr && talentsStr != '--') {
			simUI.player.setTalentsString(eventID, talentsStr);
		}
		if (glyphs) {
			simUI.player.setGlyphs(eventID, glyphs);
		}
		if (professions.length > 0) {
			simUI.player.setProfessions(eventID, professions);
		}
	});
	return { missingItems, missingEnchants };
}

interface UrlParseData {
	settings: IndividualSimSettings;
	categories: Array<SimSettingCategories>;
}

// For now this just holds static helpers to match the exporter, so it doesn't extend Importer.
export class IndividualLinkImporter {
	// Exclude UISettings by default, since most users don't intend to export those.
	static readonly DEFAULT_CATEGORIES = getEnumValues(SimSettingCategories).filter(c => c != SimSettingCategories.UISettings) as Array<SimSettingCategories>;

	static readonly CATEGORY_PARAM = 'i';
	static readonly CATEGORY_KEYS: Map<SimSettingCategories, string> = (() => {
		const map = new Map();
		// Use single-letter abbreviations since these will be included in sim links.
		map.set(SimSettingCategories.Gear, 'g');
		map.set(SimSettingCategories.Talents, 't');
		map.set(SimSettingCategories.Rotation, 'r');
		map.set(SimSettingCategories.Consumes, 'c');
		map.set(SimSettingCategories.Miscellaneous, 'm');
		map.set(SimSettingCategories.External, 'x');
		map.set(SimSettingCategories.Encounter, 'e');
		map.set(SimSettingCategories.UISettings, 'u');
		return map;
	})();

	static tryParseUrlLocation(location: Location): UrlParseData | null {
		let hash = location.hash;
		if (hash.length <= 1) {
			return null;
		}

		// Remove leading '#'
		hash = hash.substring(1);
		const binary = atob(hash);
		const bytes = new Uint8Array(binary.length);
		for (let i = 0; i < bytes.length; i++) {
			bytes[i] = binary.charCodeAt(i);
		}

		const settingsBytes = pako.inflate(bytes);
		const settings = IndividualSimSettings.fromBinary(settingsBytes);

		let exportCategories = IndividualLinkImporter.DEFAULT_CATEGORIES;
		const urlParams = new URLSearchParams(window.location.search);
		if (urlParams.has(IndividualLinkImporter.CATEGORY_PARAM)) {
			const categoryChars = urlParams.get(IndividualLinkImporter.CATEGORY_PARAM)!.split('');
			exportCategories = categoryChars
				.map(char => [...IndividualLinkImporter.CATEGORY_KEYS.entries()].find(e => e[1] == char))
				.filter(e => e)
				.map(e => e![0]);
		}

		return {
			settings: settings,
			categories: exportCategories,
		};
	}
}

export class IndividualJsonImporter<SpecType extends Spec> extends Importer {
	private readonly simUI: IndividualSimUI<SpecType>;
	constructor(parent: HTMLElement, simUI: IndividualSimUI<SpecType>) {
		super(parent, simUI, 'JSON Import', true);
		this.simUI = simUI;

		this.descriptionElem.innerHTML = `
			<p>Import settings from a JSON file, which can be created using the JSON Export feature.</p>
			<p>To import, upload the file or paste the text below, then click, 'Import'.</p>
		`;
	}

	async onImport(data: string) {
		const proto = IndividualSimSettings.fromJsonString(data, { ignoreUnknownFields: true });
		if (proto.player?.equipment) {
			await Database.loadLeftoversIfNecessary(proto.player.equipment);
		}
		if (this.simUI.isWithinRaidSim) {
			if (proto.player) {
				this.simUI.player.fromProto(TypedEvent.nextEventID(), proto.player);
			}
		} else {
			this.simUI.fromProto(TypedEvent.nextEventID(), proto);
		}
		setAcoreImport(this.simUI, null);
		this.close();
	}
}

export class Individual80UImporter<SpecType extends Spec> extends Importer {
	private readonly simUI: IndividualSimUI<SpecType>;
	constructor(parent: HTMLElement, simUI: IndividualSimUI<SpecType>) {
		super(parent, simUI, '80 Upgrades Import', true);
		this.simUI = simUI;

		this.descriptionElem.innerHTML = `
			<p>
				Import settings from <a href="https://eightyupgrades.com" target="_blank">Eighty Upgrades</a>.
			</p>
			<p>
				This feature imports gear, race, and (optionally) talents. It does NOT import buffs, debuffs, consumes, rotation, or custom stats.
			</p>
			<p>
				To import, paste the output from the site's export option below and click, 'Import'.
			</p>
		`;
	}

	async onImport(data: string) {
		const importJson = JSON.parse(data);

		// Parse all the settings.
		const charLevel = importJson?.character?.level;
		const hasReforge = importJson?.items?.some((item: any) => !!item?.reforge);
		const hasMastery = importJson?.stats?.masteryRating > 0;

		if ((charLevel && charLevel > 80) || hasReforge || hasMastery) {
			throwCataError();
		}

		const charClass = nameToClass((importJson?.character?.gameClass as string) || '');
		if (charClass == Class.ClassUnknown) {
			throw new Error('Could not parse Class!');
		}

		const race = nameToRace((importJson?.character?.race as string) || '');
		if (race == Race.RaceUnknown) {
			throw new Error('Could not parse Race!');
		}

		let talentsStr = '';
		if (importJson?.talents?.length > 0) {
			const talentIds = (importJson.talents as Array<any>).map(talentJson => talentJson.spellId);
			talentsStr = talentSpellIdsToTalentString(charClass, talentIds);
		}

		const equipmentSpec = EquipmentSpec.create();
		(importJson.items as Array<any>).forEach(itemJson => {
			const itemSpec = ItemSpec.create();
			itemSpec.id = itemJson.id;
			if (itemJson.enchant?.id) {
				itemSpec.enchant = itemJson.enchant.id;
			}
			if (itemJson.gems) {
				itemSpec.gems = (itemJson.gems as Array<any>).filter(gemJson => gemJson?.id).map(gemJson => gemJson.id);
			}
			equipmentSpec.items.push(itemSpec);
		});

		const gear = this.simUI.sim.db.lookupEquipmentSpec(equipmentSpec);

		await this.finishIndividualImport(this.simUI, charClass, race, equipmentSpec, talentsStr, null, []);
	}
}

export class IndividualWowheadGearPlannerImporter<SpecType extends Spec> extends Importer {
	private readonly simUI: IndividualSimUI<SpecType>;
	constructor(parent: HTMLElement, simUI: IndividualSimUI<SpecType>) {
		super(parent, simUI, 'Wowhead Import', true);
		this.simUI = simUI;

		this.descriptionElem.innerHTML = `
			<p>
				Import settings from <a href="https://www.wowhead.com/wotlk/gear-planner" target="_blank">Wowhead Gear Planner</a>.
			</p>
			<p>
				This feature imports gear, race, and (optionally) talents. It does NOT import buffs, debuffs, consumes, rotation, or custom stats.
			</p>
			<p>
				To import, paste the gear planner link below and click, 'Import'.
			</p>
		`;
	}

	async onImport(url: string) {
		const isCataUrl = url.includes('wowhead.com/cata');
		if (isCataUrl) {
			throwCataError();
		}

		const match = url.match(/www\.wowhead\.com\/wotlk\/gear-planner\/([a-z\-]+)\/([a-z\-]+)\/([a-zA-Z0-9_\-]+)/);
		if (!match) {
			throw new Error(`Invalid WowHead URL ${url}, must look like "https://www.wowhead.com/wotlk/gear-planner/CLASS/RACE/XXXX"`);
		}

		// Parse all the settings.
		const charClass = nameToClass(match[1].replaceAll('-', ''));
		if (charClass == Class.ClassUnknown) {
			throw new Error('Could not parse Class: ' + match[1]);
		}

		const race = nameToRace(match[2].replaceAll('-', ''));
		if (race == Race.RaceUnknown) {
			throw new Error('Could not parse Race: ' + match[2]);
		}

		const base64Data = match[3].replaceAll('_', '/').replaceAll('-', '+');
		//console.log('Base64: ' + base64Data);
		const data = Uint8Array.from(atob(base64Data), c => c.charCodeAt(0));
		//console.log('Hex: ' + buf2hex(data));

		// Binary schema
		// Byte 00: ??
		// Byte 01: ?? Seems related to aesthetics (e.g. body type)
		// Byte 02: 8-bit Player Level
		// Byte 03: 8-bit length of talents bytes
		// Next N Bytes: Talents in hex string format

		// Talent hex string looks like '230005232100330150323102505321f03f023203001f'
		// Just like regular wowhead talents string except 'f' instead of '-'.
		const numTalentBytes = data[3];
		const talentBytes = data.subarray(4, 4 + numTalentBytes);
		const talentsHexStr = buf2hex(talentBytes);
		//console.log('Talents hex: ' + talentsHexStr);
		const talentsStr = talentsHexStr.split('f').slice(0, 3).join('-');
		//console.log('Talents: ' + talentsStr);

		let cur = 4 + numTalentBytes;
		const numGlyphBytes = data[cur];
		cur++;
		const glyphBytes = data.subarray(cur, cur + numGlyphBytes);
		const gearBytes = data.subarray(cur + numGlyphBytes);
		//console.log(`Glyphs have ${numGlyphBytes} bytes: ${buf2hex(glyphBytes)}`);
		//console.log(`Remaining ${gearBytes.length} bytes: ${buf2hex(gearBytes)}`);

		// First byte in glyphs section seems to always be 0x30
		cur = 1;
		let hasGlyphs = false;
		const d = '0123456789abcdefghjkmnpqrstvwxyz';
		const glyphStr = String.fromCharCode(...glyphBytes);
		const glyphIds = [0, 0, 0, 0, 0, 0];
		while (cur < glyphBytes.length) {
			// First byte for each glyph is 0x3z, where z is the glyph position.
			// 0, 1, 2 are major glyphs, 3, 4, 5 are minor glyphs.
			const glyphPosition = d.indexOf(glyphStr[cur]);
			cur++;

			// For some reason, wowhead uses the spell IDs for the glyphs and
			// applies a ridiculous hashing scheme.
			const spellId =
				0 +
				(d.indexOf(glyphStr[cur + 0]) << 15) +
				(d.indexOf(glyphStr[cur + 1]) << 10) +
				(d.indexOf(glyphStr[cur + 2]) << 5) +
				(d.indexOf(glyphStr[cur + 3]) << 0);
			const itemId = this.simUI.sim.db.glyphSpellToItemId(spellId);
			//console.log(`Glyph position: ${glyphPosition}, spellID: ${spellId}`);

			hasGlyphs = true;
			glyphIds[glyphPosition] = itemId;
			cur += 4;
		}
		const glyphs = Glyphs.create({
			major1: glyphIds[0],
			major2: glyphIds[1],
			major3: glyphIds[2],
			minor1: glyphIds[3],
			minor2: glyphIds[4],
			minor3: glyphIds[5],
		});

		// Binary schema for each item:
		// 8-bit slotNumber, high bit = is enchanted
		// 8-bit upper 3 bits for gem count
		// 16-bit item id
		// if enchant bit is set:
		//   8-bit ??, possibly enchant position for multiple enchants?
		//   16-bit enchant id
		// for each gem:
		//   8-bit upper 3 bits for gem position
		//   16-bit gem item id
		const equipmentSpec = EquipmentSpec.create();
		cur = 0;
		while (cur < gearBytes.length) {
			const itemSpec = ItemSpec.create();
			const slotId = gearBytes[cur] & 0b00111111;
			const isEnchanted = Boolean(gearBytes[cur] & 0b10000000);
			const randomEnchant = Boolean(gearBytes[cur] & 0b01000000);
			cur++;

			const numGems = (gearBytes[cur] & 0b11100000) >> 5;
			const highid = gearBytes[cur] & 0b00011111;
			cur++;

			itemSpec.id = (highid << 16) + (gearBytes[cur] << 8) + gearBytes[cur + 1];
			cur += 2;
			//console.log(`Slot ID: ${slotId}, isEnchanted: ${isEnchanted}, numGems: ${numGems}, itemID: ${itemSpec.id}`);

			if (isEnchanted) {
				// Note: this is the enchant SPELL id, not the effect ID.
				const enchantSpellId = (gearBytes[cur] << 16) + (gearBytes[cur + 1] << 8) + gearBytes[cur + 2];
				itemSpec.enchant = this.simUI.sim.db.enchantSpellIdToEffectId(enchantSpellId);
				cur += 3;
				//console.log(`Enchant ID: ${itemSpec.enchant}. Spellid: ${enchantSpellId}`);
			}

			for (let gemIdx = 0; gemIdx < numGems; gemIdx++) {
				const gemPosition = (gearBytes[cur] & 0b11100000) >> 5;
				const highgemid = gearBytes[cur] & 0b00011111;
				cur++;

				const gemId = (highgemid << 16) + (gearBytes[cur] << 8) + gearBytes[cur + 1];
				cur += 2;
				//console.log(`Gem position: ${gemPosition}, gemID: ${gemId}`);

				if (!itemSpec.gems) {
					itemSpec.gems = [];
				}
				while (itemSpec.gems.length < gemPosition) {
					itemSpec.gems.push(0);
				}
				itemSpec.gems[gemPosition] = gemId;
			}

			// Ignore tabard / shirt slots
			const itemSlotEntry = Object.entries(IndividualWowheadGearPlannerImporter.slotIDs).find(e => e[1] == slotId);
			if (itemSlotEntry != null) {
				equipmentSpec.items.push(itemSpec);
			}
		}
		const gear = this.simUI.sim.db.lookupEquipmentSpec(equipmentSpec);

		await this.finishIndividualImport(this.simUI, charClass, race, equipmentSpec, talentsStr, hasGlyphs ? glyphs : null, []);
	}

	static slotIDs: Record<ItemSlot, number> = {
		[ItemSlot.ItemSlotHead]: 1,
		[ItemSlot.ItemSlotNeck]: 2,
		[ItemSlot.ItemSlotShoulder]: 3,
		[ItemSlot.ItemSlotBack]: 15,
		[ItemSlot.ItemSlotChest]: 5,
		[ItemSlot.ItemSlotWrist]: 9,
		[ItemSlot.ItemSlotHands]: 10,
		[ItemSlot.ItemSlotWaist]: 6,
		[ItemSlot.ItemSlotLegs]: 7,
		[ItemSlot.ItemSlotFeet]: 8,
		[ItemSlot.ItemSlotFinger1]: 11,
		[ItemSlot.ItemSlotFinger2]: 12,
		[ItemSlot.ItemSlotTrinket1]: 13,
		[ItemSlot.ItemSlotTrinket2]: 14,
		[ItemSlot.ItemSlotMainHand]: 16,
		[ItemSlot.ItemSlotOffHand]: 17,
		[ItemSlot.ItemSlotRanged]: 18,
	};
}

export class IndividualAddonImporter<SpecType extends Spec> extends Importer {
	private readonly simUI: IndividualSimUI<SpecType>;
	constructor(parent: HTMLElement, simUI: IndividualSimUI<SpecType>) {
		super(parent, simUI, 'Addon Import', true);
		this.simUI = simUI;

		this.descriptionElem.innerHTML = `
			<p>
				Import settings from the <a href="https://www.curseforge.com/wow/addons/wowsimsexporter" target="_blank">WoWSims Importer In-Game Addon</a>.
			</p>
			<p>
				This feature imports gear, race, talents, glyphs, and professions. It does NOT import buffs, debuffs, consumes, rotation, or custom stats.
			</p>
			<p>
				To import, paste the output from the addon below and click, 'Import'.
			</p>
		`;
	}

	async onImport(data: string) {
		const importJson = JSON.parse(data);

		// Parse all the settings.
		const charClass = nameToClass((importJson['class'] as string) || '');
		if (charClass == Class.ClassUnknown) {
			throw new Error('Could not parse Class!');
		}

		const race = nameToRace((importJson['race'] as string) || '');
		if (race == Race.RaceUnknown) {
			throw new Error('Could not parse Race!');
		}

		const professions = (importJson['professions'] as Array<{ name: string; level: number }>).map(profData => nameToProfession(profData.name));
		professions.forEach((prof, i) => {
			if (prof == Profession.ProfessionUnknown) {
				throw new Error(`Could not parse profession '${importJson['professions'][i].name}'`);
			}
		});

		if (importJson['glyphs']['prime']) {
			throwCataError();
		}

		const talentsStr = (importJson['talents'] as string) || '';
		const glyphsConfig = classGlyphsConfig[charClass];

		const db = await Database.get();
		const majorGlyphIDs = (importJson['glyphs']['major'] as Array<string | JsonObject>).map(g => glyphToID(g, db, glyphsConfig.majorGlyphs));
		const minorGlyphIDs = (importJson['glyphs']['minor'] as Array<string | JsonObject>).map(g => glyphToID(g, db, glyphsConfig.minorGlyphs));

		const glyphs = Glyphs.create({
			major1: majorGlyphIDs[0] || 0,
			major2: majorGlyphIDs[1] || 0,
			major3: majorGlyphIDs[2] || 0,
			minor1: minorGlyphIDs[0] || 0,
			minor2: minorGlyphIDs[1] || 0,
			minor3: minorGlyphIDs[2] || 0,
		});

		const gearJson = importJson['gear'];
		gearJson.items = (gearJson.items as Array<any>).filter(item => item != null);
		delete gearJson.version;
		(gearJson.items as Array<any>).forEach(item => {
			if (item.gems) {
				item.gems = (item.gems as Array<any>).map(gem => gem || 0);
			}
		});
		const equipmentSpec = EquipmentSpec.fromJson(gearJson);

		await this.finishIndividualImport(this.simUI, charClass, race, equipmentSpec, talentsStr, glyphs, professions);
	}
}

export class IndividualAcoreImporter<SpecType extends Spec> extends Importer {
	private readonly simUI: IndividualSimUI<SpecType>;
	private readonly characterPicker: HTMLSelectElement;
	private readonly serverStatus: HTMLElement;
	private fetchButton: HTMLButtonElement | null = null;
	private rosterFromUser = false;

	constructor(parent: HTMLElement, simUI: IndividualSimUI<SpecType>) {
		super(parent, simUI, 'AzerothCore Import', true);
		this.simUI = simUI;

		this.descriptionElem.innerHTML = `
			<p>
				Import one character from an AzerothCore roster file, written by
				<code>go run ./tools/database/acraid -leader &lt;name&gt; -out raid.json</code>.
			</p>
			<p>
				This brings over gear (with gems, enchants and reforges), race, racial traits, talents, glyphs,
				professions, the pet, ammo and consumables, and sets Heroic Presence from the character's party.
				It does NOT import other buffs, debuffs, rotation, or custom stats.
			</p>
			<p>
				The server only knows a character as of their last save, so run <code>saveall</code> in the
				worldserver console first.
			</p>
			<p>
				To import, upload the roster file or paste it below, pick the character, then click 'Import'.
			</p>
		`;

		const pickerElem = document.createElement('div');
		pickerElem.classList.add('acore-character-picker', 'mb-3');
		pickerElem.innerHTML = `
			<label class="form-label">Character</label>
			<select class="form-select acore-character-select"></select>
			<div class="acore-server-status form-text d-none"></div>
		`;
		this.body.prepend(pickerElem);
		this.characterPicker = pickerElem.getElementsByClassName('acore-character-select')[0] as HTMLSelectElement;
		this.serverStatus = pickerElem.getElementsByClassName('acore-server-status')[0] as HTMLElement;
		this.characterPicker.addEventListener('change', () => storePick(this.simUI, this.characterPicker.value));
		this.refreshCharacters('');

		simServerAvailable().then(available => {
			if (available) {
				this.addFetchButton();
				this.fetchFromServer(false);
			}
		});
	}

	private addFetchButton() {
		const button = document.createElement('button');
		button.classList.add('importer-button', 'btn', 'btn-primary', 'fetch-button', 'me-2');
		button.innerHTML = '<i class="fa fa-cloud-arrow-down"></i> Fetch from server';
		button.addEventListener('click', () => this.fetchFromServer(true));
		this.importButton.before(button);
		this.fetchButton = button;

		const hint = document.createElement('p');
		hint.classList.add('acore-fetch-hint');
		hint.textContent = "The core raid comes straight off the server when this opens. 'Fetch from server' reads it again.";
		this.descriptionElem.appendChild(hint);
	}

	// On open, a roster the user added meanwhile stays. The button overwrites it.
	private async fetchFromServer(overwrite: boolean) {
		this.fetchButton!.disabled = true;
		this.showStatus('Reading the core raid off the server...', false);
		try {
			const data = await fetchServerRoster();
			if (this.rosterFromUser && !overwrite) {
				this.showStatus("Kept the roster you added. 'Fetch from server' swaps in the server's.", false);
				return;
			}
			this.textElem.value = data;
			this.rosterFromUser = false;
			this.refreshCharacters(data);
			const exportedAt = JSON.parse(data)?.exportedAt;
			this.showStatus(`From the server${exportedAt ? `, exported ${exportedAt}` : ''}.`, false);
		} catch (error: any) {
			this.showStatus(`Couldn't read the roster off the server: ${error?.message}`, true);
		} finally {
			this.fetchButton!.disabled = false;
		}
	}

	private showStatus(text: string, failed: boolean) {
		this.serverStatus.textContent = text;
		this.serverStatus.classList.remove('d-none');
		this.serverStatus.classList.toggle('text-danger', failed);
	}

	protected onTextChanged(data: string) {
		this.rosterFromUser = data.trim() != '';
		this.refreshCharacters(data);
	}

	private refreshCharacters(data: string) {
		const previous = this.characterPicker.value;
		let all: Array<RosterCharacter> = [];
		try {
			all = parseRoster(data).characters;
		} catch {
			// Nothing usable pasted yet; the Import button says why.
		}
		const chars = acoreCandidates(this.simUI, all);
		const names = chars.map(char => char.name);

		// Names come out of an uploaded file, so build the options instead of templating HTML.
		this.characterPicker.innerHTML = '';
		if (names.length == 0) {
			this.characterPicker.appendChild(new Option(noAcoreCandidates(this.simUI, all, 'in this roster'), ''));
			return;
		}
		names.forEach(name => this.characterPicker.appendChild(new Option(name, name)));
		this.characterPicker.value = names.includes(previous) ? previous : this.preferredCharacter(chars);
	}

	// The last pick if the roster has them, else whoever plays this page's spec, on the page's main talent
	// tree if possible.
	private preferredCharacter(chars: Array<RosterCharacter>): string {
		const saved = loadAcorePick(this.simUI);
		if (chars.some(char => char.name == saved)) {
			return saved;
		}
		const player = this.simUI.player;
		const score = (char: RosterCharacter) => {
			try {
				const specMatch = inferSpec(player.getClass(), char.talents) == player.spec;
				const treeMatch = getTalentTree(char.talents) == player.getTalentTree();
				return (specMatch ? 2 : 0) + (treeMatch ? 1 : 0);
			} catch {
				// talents too long for the class's trees
				return 0;
			}
		};
		return chars.reduce((best, char) => (score(char) > score(best) ? char : best)).name;
	}

	async onImport(data: string) {
		const roster = parseRoster(data);
		await importAcoreCharacter(this.simUI, roster, this.characterPicker.value);
		this.close();
	}
}

// What the last server import left behind, for the Caps tab to show.
export interface AcoreImport {
	name: string;
	exportedAt: string;
	notes: Array<string>;
}

// Loads one roster character onto the page: gear, race, talents, glyphs, professions, racials, pet, ammo,
// consumes and Heroic Presence. Remembers the pick and publishes the result on the page's acoreImportEmitter.
// Refuses a healer on a DPS or tank page, and anyone but a healer on a healer page.
export async function importAcoreCharacter<SpecType extends Spec>(
	simUI: IndividualSimUI<SpecType>,
	roster: Roster,
	name: string,
): Promise<AcoreImport> {
	await simUI.sim.waitForInit();
	const db = await Database.loadLeftoversIfNecessary(rosterEquipmentSpec(roster));

	const playerClass = simUI.player.getClass();
	const classmates = roster.characters.filter(char => rosterClass(char.classId) == playerClass);
	if (classmates.length == 0) {
		throw new Error(`This roster has no ${classNames.get(playerClass)} in it.`);
	}
	const picked = name ? classmates.find(char => char.name == name) : acoreCandidates(simUI, classmates)[0];
	if (!picked) {
		throw new Error(name ? `${name} isn't in this roster.` : `${noAcoreCandidates(simUI, classmates, 'in this roster')}.`);
	}
	const misfit = roleMisfit(simUI, picked);
	if (misfit) {
		throw new Error(misfit);
	}
	storePick(simUI, picked.name);

	const imported = buildCharacterImport(db, picked);
	const { missingItems, missingEnchants } = await applyIndividualImport(
		simUI,
		imported.playerClass,
		imported.race,
		imported.equipment,
		imported.talentsString,
		imported.glyphs,
		imported.professions,
	);
	const player = simUI.player;
	player.setRacialTraits(TypedEvent.nextEventID(), imported.racialTraits);
	// The pet talents picker swaps in its own talents when the pet moves to another talent tree, so
	// the roster's go in after the pet type.
	applyPetAndAmmo(player, imported.playerClass, { ...imported.loadout, petTalents: undefined }, TypedEvent.nextEventID());
	applyPetAndAmmo(player, imported.playerClass, imported.loadout, TypedEvent.nextEventID());
	applyConsumes(player, imported, TypedEvent.nextEventID());

	const presence = heroicPresenceSource(roster, picked);
	const party = player.getParty();
	if (party) {
		party.setBuffs(TypedEvent.nextEventID(), PartyBuffs.create({ ...party.getBuffs(), heroicPresence: !!presence }));
	}

	const notes = (roster.warnings || []).concat(picked.warnings || []).concat(imported.warnings);
	if (imported.loadout.skipped.length) {
		notes.unshift(`Kept, the sim has no value for: ${imported.loadout.skipped.join(', ')}.`);
	}
	if (!presence) {
		notes.unshift(`Heroic Presence off: nobody in ${picked.name}'s party has Draenei racials.`);
	} else if (presence == picked) {
		notes.unshift(`Heroic Presence on: ${picked.name} has Draenei racials.`);
	} else {
		notes.unshift(`Heroic Presence on: ${presence.name} in ${picked.name}'s party has Draenei racials.`);
	}
	if (roster.version < 2) {
		notes.unshift('The pet, ammo and consumables stay as they are: a version 1 roster has none.');
	} else {
		const petAndAmmo = describePetAndAmmo(imported);
		const petLabel = imported.playerClass == Class.ClassWarlock ? 'Demon' : 'Pet and ammo';
		const consumes = describeConsumeSource(imported);
		notes.unshift(...[petAndAmmo && `${petLabel}: ${petAndAmmo}.`, consumes && `Consumables from ${consumes}.`].filter(line => line));
	}
	if (imported.spec != player.spec) {
		notes.unshift(`${picked.name}'s talents look like ${specNames[imported.spec]}, but this is the ${specNames[player.spec]} page.`);
	}
	if (missingItems.length || missingEnchants.length) {
		notes.push(
			'Not in the sim database:' +
				(missingItems.length ? ` items ${missingItems.join(', ')}` : '') +
				(missingEnchants.length ? ` enchants ${missingEnchants.join(', ')}` : '') +
				'.',
		);
	}

	const result = { name: picked.name, exportedAt: roster.exportedAt || '', notes };
	setAcoreImport(simUI, result);
	return result;
}

// The Caps tab sends reforges to this character, so any other import has to clear it.
function setAcoreImport<SpecType extends Spec>(simUI: IndividualSimUI<SpecType>, result: AcoreImport | null) {
	if (!result && !simUI.lastAcoreImport) {
		return;
	}
	simUI.lastAcoreImport = result;
	simUI.acoreImportEmitter.emit(TypedEvent.nextEventID(), undefined);
}

const HEALING_TREES: Partial<Record<Class, Array<number>>> = {
	[Class.ClassPaladin]: [0],
	[Class.ClassPriest]: [0, 1],
	[Class.ClassShaman]: [2],
	[Class.ClassDruid]: [2],
};

// The tree with the most points, e.g. 'Discipline', or '' without talents.
export function mainTreeName(char: RosterCharacter): string {
	if (getTalentPoints(char.talents) == 0) {
		return '';
	}
	return classTalentsConfig[rosterClass(char.classId)][getTalentTree(char.talents)]?.name ?? '';
}

function isAcoreHealer(char: RosterCharacter): boolean {
	return getTalentPoints(char.talents) > 0 && !!HEALING_TREES[rosterClass(char.classId)]?.includes(getTalentTree(char.talents));
}

// Healers load only on healer pages, and only healers load there. Returns why a character can't, or ''.
function roleMisfit<SpecType extends Spec>(simUI: IndividualSimUI<SpecType>, char: RosterCharacter): string {
	const healerPage = isHealingSpec(simUI.player.spec);
	if (isAcoreHealer(char) == healerPage) {
		return '';
	}
	const tree = mainTreeName(char);
	return `${char.name} ${tree ? `is specced ${tree}` : 'has no talents'}: load them on a ${healerPage ? 'DPS or tank' : 'healer'} page.`;
}

export function acoreCandidates<SpecType extends Spec>(simUI: IndividualSimUI<SpecType>, characters: Array<RosterCharacter>): Array<RosterCharacter> {
	const playerClass = simUI.player.getClass();
	return characters.filter(char => rosterClass(char.classId) == playerClass && !roleMisfit(simUI, char));
}

// Why acoreCandidates found no one, e.g. 'No Priest healers in this roster'. No final period, since the
// window shows it as an option.
export function noAcoreCandidates<SpecType extends Spec>(
	simUI: IndividualSimUI<SpecType>,
	characters: Array<RosterCharacter>,
	where: string,
): string {
	const playerClass = simUI.player.getClass();
	const className = classNames.get(playerClass);
	if (!characters.some(char => rosterClass(char.classId) == playerClass)) {
		return `No ${className} ${where}`;
	}
	return isHealingSpec(simUI.player.spec) ? `No ${className} healers ${where}` : `Only ${className} healers ${where}: they load on a healer page`;
}

function pickKey(simUI: SimUI): string {
	return simUI.getStorageKey('acore-import.v1');
}

// The character the user last loaded on this spec page, or ''.
export function loadAcorePick(simUI: SimUI): string {
	try {
		const saved = JSON.parse(window.localStorage.getItem(pickKey(simUI)) || '{}');
		return typeof saved?.character == 'string' ? saved.character : '';
	} catch {
		return '';
	}
}

function storePick(simUI: SimUI, name: string) {
	if (!name) {
		return;
	}
	try {
		window.localStorage.setItem(pickKey(simUI), JSON.stringify({ character: name }));
	} catch {
		// storage off: the spec match picks next time
	}
}

const throwCataError = () => {
	throw new Error(`WowSims does not support the Cata Pre-patch.
Please use: https://wowsims.github.io/cata/ instead`);
};

function glyphNameToID(glyphName: string, glyphsConfig: Record<number, GlyphConfig>): number {
	if (!glyphName) {
		return 0;
	}

	for (const glyphIDStr in glyphsConfig) {
		if (glyphsConfig[glyphIDStr].name == glyphName) {
			return parseInt(glyphIDStr);
		}
	}
	throw new Error(`Unknown glyph name '${glyphName}'`);
}

function glyphToID(glyph: string | JsonObject, db: Database, glyphsConfig: Record<number, GlyphConfig>): number {
	if (typeof glyph === 'string') {
		// Legacy version: AddOn exports Glyphs by name (string) only. Names must be in English.
		return glyphNameToID(glyph, glyphsConfig);
	}
	// New version exports glyph information in a table that includes the name and the glyph spell ID.
	return db.glyphSpellToItemId(glyph['spellID'] as number);
}
