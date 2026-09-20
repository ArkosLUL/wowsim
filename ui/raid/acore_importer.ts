import { Importer } from '../core/components/importers';
import { RaidSimPreset } from '../core/individual_sim_ui';
import { MAX_PARTY_SIZE } from '../core/party';
import { Player } from '../core/player';
import { Party as PartyProto, Player as PlayerProto, Raid as RaidProto } from '../core/proto/api';
import { Class, Spec, UnitReference, UnitReference_Type } from '../core/proto/common';
import { RaidSimSettings } from '../core/proto/ui';
import { Database } from '../core/proto_utils/database';
import { emptyUnitReference, getTalentTree, isTankSpec, makeDefaultBlessings, newUnitReference, specNames } from '../core/proto_utils/utils';
import { MAX_NUM_PARTIES, Raid } from '../core/raid';
import { Sim } from '../core/sim';
import { EventID, TypedEvent } from '../core/typed_event';
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
	Roster,
	rosterEquipmentSpec,
} from './acore_roster';
import { playerPresets } from './presets';
import { RaidSimUI } from './raid_sim_ui';

// tanks_picker.ts shows this many tank slots, and nothing past them is editable.
const MAX_TANKS = 4;

// Past this the alert gets unreadable, so the rest only goes to the console.
const MAX_SUMMARY_WARNINGS = 20;

// Keeps the two radios apart when more than one import modal has been opened.
let modeGroupCounter = 0;

type ImportMode = 'replace' | 'update';

export interface RaidCharacter extends CharacterImport {
	preset: RaidSimPreset<any>;
	raidIndex: number;
	isMainTank: boolean;
}

interface AssignmentSnapshot {
	player: Player<any>;
	field: string;
	target: Player<any> | null;
}

// Spec options that hold a raid index, keyed by class. Not every spec of a class has the field, so
// both the read and the write check first.
const assignmentFields: Array<{ playerClass: Class; field: string }> = [
	{ playerClass: Class.ClassDruid, field: 'innervateTarget' },
	{ playerClass: Class.ClassPriest, field: 'powerInfusionTarget' },
	{ playerClass: Class.ClassRogue, field: 'tricksOfTheTradeTarget' },
	{ playerClass: Class.ClassDeathknight, field: 'unholyFrenzyTarget' },
	{ playerClass: Class.ClassMage, field: 'focusMagicTarget' },
];

export class RaidAcoreImporter extends Importer {
	private readonly simUI: RaidSimUI;
	private mode: ImportMode;

	constructor(parent: HTMLElement, simUI: RaidSimUI) {
		super(parent, simUI, 'AzerothCore Import', true);
		this.simUI = simUI;

		// Update is only useful once there's a raid to update.
		this.mode = simUI.sim.raid.getPlayers().some(player => player != null) ? 'update' : 'replace';

		this.descriptionElem.innerHTML = `
			<p>
				Imports a raid from an AzerothCore server, using the roster file written by
				<code>go run ./tools/database/acraid -leader &lt;name&gt; -out raid.json</code>.
			</p>
			<p>
				Gear (with gems, enchants and reforges), talents, glyphs, race, racial traits and professions come
				from the server. Rotations, consumes, spec options, raid buffs and the encounter don't: those stay
				yours.
			</p>
			<p>
				To import, upload the roster file or paste it below, pick a mode, then click 'Import'.
			</p>
		`;

		this.body.prepend(this.buildModePicker());
	}

	private buildModePicker(): HTMLElement {
		const groupName = `acore-import-mode-${modeGroupCounter++}`;
		const container = document.createElement('div');
		container.classList.add('acore-import-mode', 'mb-3');
		container.innerHTML = `
			<div class="form-check">
				<input class="form-check-input" type="radio" name="${groupName}" id="${groupName}-update" value="update">
				<label class="form-check-label" for="${groupName}-update">
					<strong>Update</strong> the raid: match raiders by name and refresh what the server knows, keeping
					their rotation, consumes and spec options.
				</label>
			</div>
			<div class="form-check">
				<input class="form-check-input" type="radio" name="${groupName}" id="${groupName}-replace" value="replace">
				<label class="form-check-label" for="${groupName}-replace">
					<strong>Replace</strong> the raid: throw away every raider and build the roster from presets.
				</label>
			</div>
		`;

		container.querySelectorAll('input[type=radio]').forEach(elem => {
			const radio = elem as HTMLInputElement;
			radio.checked = radio.value == this.mode;
			radio.addEventListener('change', () => {
				if (radio.checked) {
					this.mode = radio.value as ImportMode;
				}
			});
		});
		return container;
	}

	async onImport(data: string) {
		const roster = parseRoster(data);
		await this.simUI.sim.waitForInit();
		const db = await Database.loadLeftoversIfNecessary(rosterEquipmentSpec(roster));

		const raidIndexes = assignRaidIndexes(roster.characters);
		const imports: Array<RaidCharacter> = [];
		const skipped: Array<string> = [];
		roster.characters.forEach((char, i) => {
			try {
				if (raidIndexes[i] == -1) {
					throw new Error(`subgroup ${char.subgroup} has no room in the raid`);
				}
				const imported = buildCharacterImport(db, char);
				imports.push({
					...imported,
					preset: matchPreset(playerPresets, imported.spec, imported.talentsString),
					raidIndex: raidIndexes[i],
					isMainTank: (char.memberFlags & MEMBER_FLAG_MAIN_TANK) != 0,
				});
			} catch (e: any) {
				skipped.push(`${char.name}: ${e?.message || e}`);
			}
		});
		if (imports.length == 0) {
			throw new Error(`Nothing could be imported.\n\n${skipped.join('\n')}`);
		}

		const notes = new ImportNotes();
		if (this.mode == 'replace') {
			this.replaceRaid(imports, notes);
		} else {
			updateRaid(this.simUI.sim, imports, notes);
		}

		this.close();
		const summary = notes.summarize(this.mode, roster, imports, skipped);
		console.log(summary);
		const warnings = collectWarnings(roster, imports);
		if (warnings.length > MAX_SUMMARY_WARNINGS) {
			console.log(`All ${warnings.length} roster notes:\n${warnings.map(warning => `  ${warning}`).join('\n')}`);
		}
		// chrome cuts a long alert off mid-text, so the full spec list stays in the console
		alert(notes.summarize(this.mode, roster, imports, skipped, true));
	}

	private replaceRaid(imports: Array<RaidCharacter>, notes: ImportNotes) {
		const eventID = TypedEvent.nextEventID();
		const raid = this.simUI.sim.raid;

		// Everyone is a fresh Player here, so the only interesting move is who joined and who left.
		const previousNames = new Set(raid.getPlayers().map(player => player?.getName()).filter(name => name !== undefined) as Array<string>);
		if (previousNames.size > 0) {
			const rosterNames = new Set(imports.map(imported => imported.char.name));
			imports.filter(imported => !previousNames.has(imported.char.name)).forEach(imported => notes.added.push(imported.char.name));
			previousNames.forEach(name => {
				if (!rosterNames.has(name)) {
					notes.removed.push(name);
				}
			});
		}

		const raidProto = RaidProto.create({
			parties: [...new Array(MAX_NUM_PARTIES).keys()].map(partyIdx =>
				PartyProto.create({
					players: [...new Array(MAX_PARTY_SIZE).keys()].map(() => PlayerProto.create()),
					buffs: raid.getParty(partyIdx).getBuffs(),
				}),
			),
			buffs: raid.getBuffs(),
			debuffs: raid.getDebuffs(),
			targetDummies: raid.getTargetDummies(),
			tanks: tankReferences(imports),
			numActiveParties: activeParties(imports.map(imported => imported.char)),
		});
		const numPaladins = imports.filter(imported => imported.playerClass == Class.ClassPaladin).length;

		TypedEvent.freezeAllAndDo(() => {
			imports.forEach(imported => {
				const player = newPlayerFromPreset(imported.spec, imported.preset, this.simUI.sim, eventID);
				applyCharacter(player, imported, eventID);
				const partyIdx = Math.floor(imported.raidIndex / MAX_PARTY_SIZE);
				raidProto.parties[partyIdx].players[imported.raidIndex % MAX_PARTY_SIZE] = player.toProto();
			});

			// Clear first, so nothing of the old raid lingers in a slot the roster doesn't fill.
			this.simUI.clearRaid(eventID);
			this.simUI.fromProto(
				eventID,
				RaidSimSettings.create({
					raid: raidProto,
					encounter: this.simUI.sim.encounter.toProto(),
					blessings: makeDefaultBlessings(numPaladins),
				}),
			);
		});
	}
}

// Matches raiders by name, moves them to their roster subgroups, adds and drops the difference.
// Sits outside the modal so a test can drive it.
export function updateRaid(sim: Sim, imports: Array<RaidCharacter>, notes: ImportNotes) {
	const eventID = TypedEvent.nextEventID();
	const raid = sim.raid;

	TypedEvent.freezeAllAndDo(() => {
		const before = raid.getPlayers().filter(player => player != null) as Array<Player<any>>;
		const byName = new Map<string, Player<any>>();
		before.forEach(player => {
			if (!byName.has(player.getName())) {
				byName.set(player.getName(), player);
			}
		});
		// Raid indexes move around below, so remember who the assignments point at, not where.
		const assignments = snapshotAssignments(raid);

		const placed = imports.map(imported => {
			const match = byName.get(imported.char.name);
			if (match) {
				byName.delete(imported.char.name);
			}
			// A new top tree means a new spec, and a spec change means a new Player.
			const keep =
				!!match &&
				match.getClass() == imported.playerClass &&
				getTalentTree(match.getTalentsString()) == getTalentTree(imported.talentsString);

			let player: Player<any>;
			if (keep) {
				player = match!;
				notes.kept.push({ name: imported.char.name, spec: player.spec, inferred: imported.spec });
			} else {
				player = newPlayerFromPreset(imported.spec, imported.preset, sim, eventID);
				if (match) {
					notes.replaced.push({ name: imported.char.name, from: match.spec, to: imported.spec });
				} else {
					notes.added.push(imported.char.name);
				}
			}
			applyCharacter(player, imported, eventID);
			return { imported: imported, player: player };
		});

		for (let i = 0; i < MAX_NUM_PARTIES * MAX_PARTY_SIZE; i++) {
			raid.setPlayer(eventID, i, null);
		}
		placed.forEach(entry => raid.setPlayer(eventID, entry.imported.raidIndex, entry.player));

		// A replaced raider also loses their party, so go by name: only roster no-shows are gone.
		const rosterNames = new Set(imports.map(imported => imported.char.name));
		before
			.filter(player => player.getParty() == null && !rosterNames.has(player.getName()))
			.forEach(player => notes.removed.push(player.getName()));

		raid.setTanks(
			eventID,
			tankReferences(placed.map(entry => ({ spec: entry.player.spec, raidIndex: entry.imported.raidIndex, isMainTank: entry.imported.isMainTank }))),
		);
		const placedByName = new Map<string, Player<any>>();
		placed.forEach(entry => placedByName.set(entry.imported.char.name, entry.player));
		remapAssignments(assignments, placedByName, eventID);
		raid.setNumActiveParties(eventID, activeParties(imports.map(imported => imported.char)));
	});
}

// Main-tank flagged raiders first, then whoever else brought a tank spec.
function tankReferences(raiders: Array<{ spec: Spec; raidIndex: number; isMainTank: boolean }>): Array<UnitReference> {
	const mainTanks = raiders.filter(raider => raider.isMainTank);
	const otherTanks = raiders.filter(raider => !raider.isMainTank && isTankSpec(raider.spec));
	return mainTanks
		.concat(otherTanks)
		.slice(0, MAX_TANKS)
		.map(raider => newUnitReference(raider.raidIndex));
}

function snapshotAssignments(raid: Raid): Array<AssignmentSnapshot> {
	const snapshots: Array<AssignmentSnapshot> = [];
	raid.getPlayers().forEach(player => {
		if (!player) {
			return;
		}
		assignmentFields
			.filter(assignment => assignment.playerClass == player.getClass())
			.forEach(assignment => {
				const options = player.getSpecOptions() as any;
				const reference: UnitReference | undefined = options[assignment.field];
				if (!reference || reference.type != UnitReference_Type.Player) {
					return;
				}
				snapshots.push({
					player: player,
					field: assignment.field,
					target: raid.getPlayerFromUnitReference(reference),
				});
			});
	});
	return snapshots;
}

function remapAssignments(snapshots: Array<AssignmentSnapshot>, placedByName: Map<string, Player<any>>, eventID: EventID) {
	snapshots.forEach(snapshot => {
		if (snapshot.player.getParty() == null) {
			return;
		}
		const options = snapshot.player.getSpecOptions() as any;
		if (!(snapshot.field in options)) {
			return;
		}
		// A respecced raider is a new Player object under the same name, so follow the name. Only
		// someone who left the raid clears the assignment.
		let target = snapshot.target;
		if (target && target.getParty() == null) {
			target = placedByName.get(target.getName()) || null;
		}
		options[snapshot.field] = target && target.getParty() != null ? newUnitReference(target.getRaidIndex()) : emptyUnitReference();
		snapshot.player.setSpecOptions(eventID, options);
	});
}

export class ImportNotes {
	readonly added: Array<string> = [];
	readonly removed: Array<string> = [];
	readonly replaced: Array<{ name: string; from: Spec; to: Spec }> = [];
	readonly kept: Array<{ name: string; spec: Spec; inferred: Spec }> = [];

	summarize(mode: ImportMode, roster: Roster, imports: Array<RaidCharacter>, skipped: Array<string>, brief = false): string {
		const lines: Array<string> = [];
		const group = roster.group?.leader ? `${roster.group.leader}'s raid` : 'roster';
		lines.push(`AzerothCore import (${mode == 'replace' ? 'Replace' : 'Update'}): ${group}, exported ${roster.exportedAt || 'at an unknown time'}.`);
		lines.push(`${imports.length} of ${roster.characters.length} characters in ${activeParties(imports.map(imported => imported.char))} parties.`);

		const specLine = (imported: RaidCharacter) => {
			const kept = this.kept.find(entry => entry.name == imported.char.name);
			const tags: Array<string> = [];
			if (imported.isMainTank) {
				tags.push('main tank');
			}
			if (kept && kept.spec != kept.inferred) {
				tags.push(`kept, the roster's talents look like ${specNames[kept.inferred]}`);
			}
			return `  ${imported.char.name}: ${specNames[kept ? kept.spec : imported.spec]}${tags.length ? ` (${tags.join('; ')})` : ''}`;
		};
		if (brief) {
			// just the ones worth a second look, since the console has every spec
			const odd = imports.filter(imported => {
				const kept = this.kept.find(entry => entry.name == imported.char.name);
				return imported.isMainTank || (kept && kept.spec != kept.inferred);
			});
			lines.push('', `Specs: ${imports.length} read off the roster, all of them in the browser console.`);
			odd.forEach(imported => lines.push(specLine(imported)));
		} else {
			lines.push('', 'Specs:');
			imports.forEach(imported => lines.push(specLine(imported)));
		}

		if (this.added.length) {
			lines.push('', `Added: ${this.added.join(', ')}`);
		}
		if (this.replaced.length) {
			lines.push('', 'Replaced, their top talent tree changed:');
			this.replaced.forEach(entry => lines.push(`  ${entry.name}: ${specNames[entry.from]} -> ${specNames[entry.to]}`));
		}
		if (this.removed.length) {
			const inRoster = new Set(roster.characters.map(char => char.name));
			const missing = this.removed.filter(name => !inRoster.has(name));
			const unbuildable = this.removed.filter(name => inRoster.has(name));
			if (missing.length) {
				lines.push('', `Removed, they aren't in the roster: ${missing.join(', ')}`);
			}
			if (unbuildable.length) {
				lines.push('', `Removed, their roster entry couldn't be imported: ${unbuildable.join(', ')}`);
			}
		}
		if (skipped.length) {
			lines.push('', 'Skipped:');
			skipped.forEach(entry => lines.push(`  ${entry}`));
		}

		const warnings = collectWarnings(roster, imports);
		if (warnings.length) {
			lines.push('', 'Roster notes:');
			warnings.slice(0, MAX_SUMMARY_WARNINGS).forEach(warning => lines.push(`  ${warning}`));
			if (warnings.length > MAX_SUMMARY_WARNINGS) {
				lines.push(`  ...and ${warnings.length - MAX_SUMMARY_WARNINGS} more, the browser console has all of them.`);
			}
		}

		lines.push(
			'',
			'Keep in mind:',
			"  Item stats come from the server, but a few item and set effects are still Classic's.",
			"  mod-spell-tweaks and mod-individual-progression aren't modelled.",
			"  Rotations, consumes, spec options and raid buffs are the sim's, not what you run in game.",
		);
		return lines.join('\n');
	}
}

// One line per distinct message, with the characters it hit, so 25 copies of the same note stay readable.
function collectWarnings(roster: Roster, imports: Array<RaidCharacter>): Array<string> {
	const lines: Array<string> = (roster.warnings || []).slice();
	const byMessage = new Map<string, Array<string>>();
	imports.forEach(imported => {
		const messages = (imported.char.warnings || []).concat(imported.warnings);
		messages.forEach(message => {
			const names = byMessage.get(message) || [];
			names.push(imported.char.name);
			byMessage.set(message, names);
		});
	});
	byMessage.forEach((names, message) => {
		const who = names.length > 3 ? `${names.slice(0, 3).join(', ')} and ${names.length - 3} more` : names.join(', ');
		lines.push(`${who}: ${message}`);
	});
	return lines;
}
