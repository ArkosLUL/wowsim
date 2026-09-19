import { Tooltip } from 'bootstrap';

import { LIVE_SERVER_DEFAULTS } from '../constants/server_defaults_auto_gen.js';
import { Encounter } from '../encounter.js';
import { DungeonScaleModifiers, DungeonScaleSettings, RaidDifficulty, ServerSettings, SpellTweaksSettings } from '../proto/common.js';
import { EventID } from '../typed_event.js';
import { BooleanPicker } from './boolean_picker.js';
import { Component } from './component.js';
import { EnumPicker } from './enum_picker.js';
import { NumberPicker } from './number_picker.js';

type SpellTweakSwitch = Exclude<keyof SpellTweaksSettings, 'enable' | 'exoticPetDamagePct'>;

interface SpellTweakInput {
	key: SpellTweakSwitch;
	label: string;
	tooltip: string;
	// core reads the armor pen switches itself, so SpellTweaks.Enable doesn't gate them
	ungated?: boolean;
}

const SPELL_TWEAK_SWITCHES: Array<SpellTweakInput> = [
	{ key: 'hunterPetHaste', label: 'Hunter pet haste', tooltip: 'SpellTweaks.HunterPetHaste.Enable: hunter pets inherit ranged haste.' },
	{
		key: 'hunterPetArmorPen',
		label: 'Hunter pet armor pen',
		tooltip: 'SpellTweaks.HunterPetArmorPen.Enable: hunter pets inherit armor pen. Works with Spell tweaks off.',
		ungated: true,
	},
	{
		key: 'dkGhoulArmorPen',
		label: 'Ghoul armor pen',
		tooltip: 'SpellTweaks.DKGhoulArmorPen.Enable: the ghoul inherits armor pen. Works with Spell tweaks off.',
		ungated: true,
	},
	{ key: 'feralSpiritHaste', label: 'Feral Spirit haste', tooltip: 'SpellTweaks.FeralSpiritHaste.Enable: spirit wolves inherit melee haste.' },
	{
		key: 'diseaseHaste',
		label: 'Disease haste',
		tooltip: 'SpellTweaks.DiseaseHaste.Enable: melee haste speeds up Frost Fever and Blood Plague ticks, with Epidemic.',
	},
	{
		key: 'deadlyPoisonMurder',
		label: 'Deadly Poison haste',
		tooltip: 'SpellTweaks.DeadlyPoisonMurder.Enable: melee haste speeds up Deadly Poison ticks, with Murder.',
	},
	{
		key: 'ruptureWeaponExpertise',
		label: 'Rupture haste',
		tooltip: 'SpellTweaks.RuptureWeaponExpertise.Enable: melee haste speeds up Rupture ticks, with Weapon Expertise.',
	},
	{ key: 'rendTrauma', label: 'Rend haste', tooltip: 'SpellTweaks.RendTrauma.Enable: melee haste speeds up Rend ticks, with Trauma.' },
	{
		key: 'balanceDotScaling',
		label: 'Balance DoT scaling',
		tooltip: 'SpellTweaks.BalanceDotScaling.Enable: spell haste speeds up Moonfire and Insect Swarm ticks with Eclipse, and they crit with Earth and Moon.',
	},
	{
		key: 'titansGripNoDamagePenalty',
		label: "No Titan's Grip penalty",
		tooltip: "SpellTweaks.TitansGrip.NoDamagePenalty.Enable: no -10% damage from Titan's Grip.",
	},
	{
		key: 'omenClarityFaerieFire',
		label: 'Omen from Faerie Fire',
		tooltip: 'SpellTweaks.OmenClarityFaerieFire.Enable: Faerie Fire (Feral) on an NPC always procs Clearcasting with Omen of Clarity.',
	},
];

type ScaleSet = keyof DungeonScaleSettings;
type ScaleStat = keyof DungeonScaleModifiers;

// [creature, boss] for each: the difficulty's own size set, and the generic set it falls back to.
interface DifficultySets {
	size: [ScaleSet, ScaleSet];
	generic: [ScaleSet, ScaleSet];
}

const DIFFICULTIES: Array<{ difficulty: RaidDifficulty; name: string; sets: DifficultySets }> = [
	{
		difficulty: RaidDifficulty.RaidDifficulty10Normal,
		name: '10 player',
		sets: { size: ['raid10', 'raid10Boss'], generic: ['raid', 'raidBoss'] },
	},
	{
		difficulty: RaidDifficulty.RaidDifficulty25Normal,
		name: '25 player',
		sets: { size: ['raid25', 'raid25Boss'], generic: ['raid', 'raidBoss'] },
	},
	{
		difficulty: RaidDifficulty.RaidDifficulty10Heroic,
		name: '10 player heroic',
		sets: { size: ['raid10Heroic', 'raid10HeroicBoss'], generic: ['raidHeroic', 'raidHeroicBoss'] },
	},
	{
		difficulty: RaidDifficulty.RaidDifficulty25Heroic,
		name: '25 player heroic',
		sets: { size: ['raid25Heroic', 'raid25HeroicBoss'], generic: ['raidHeroic', 'raidHeroicBoss'] },
	},
];

const SCALE_STATS: Array<{ stat: ScaleStat; label: string }> = [
	{ stat: 'global', label: 'Global' },
	{ stat: 'health', label: 'Health' },
	{ stat: 'armor', label: 'Armor' },
	{ stat: 'damage', label: 'Damage' },
];

// Unknown, or a value this build doesn't know, is 25 normal, same as RaidDifficultyOrDefault in the sim.
function difficultyOrDefault(difficulty: RaidDifficulty): RaidDifficulty {
	return DIFFICULTIES.some(d => d.difficulty == difficulty) ? difficulty : RaidDifficulty.RaidDifficulty25Normal;
}

function setsFor(difficulty: RaidDifficulty): DifficultySets {
	return DIFFICULTIES.find(d => d.difficulty == difficultyOrDefault(difficulty))!.sets;
}

function spellTweak<K extends keyof SpellTweaksSettings>(settings: ServerSettings, key: K): SpellTweaksSettings[K] {
	return settings.spellTweaks?.[key] ?? LIVE_SERVER_DEFAULTS.spellTweaks?.[key];
}

function spellTweaksEnabled(settings: ServerSettings): boolean {
	return spellTweak(settings, 'enable') ?? false;
}

// A value equal to the live config clears the field, so the encounter keeps following the config.
function withSpellTweak<K extends keyof SpellTweaksSettings>(settings: ServerSettings, key: K, value: SpellTweaksSettings[K]): ServerSettings {
	const next = ServerSettings.clone(settings);
	const tweaks = next.spellTweaks ?? SpellTweaksSettings.create();
	if (value === LIVE_SERVER_DEFAULTS.spellTweaks?.[key]) {
		delete tweaks[key];
	} else {
		tweaks[key] = value;
	}

	if (SpellTweaksSettings.equals(tweaks, SpellTweaksSettings.create())) {
		delete next.spellTweaks;
	} else {
		next.spellTweaks = tweaks;
	}
	return next;
}

function mapUpdateIntervalMs(settings: ServerSettings): number {
	return settings.mapUpdateIntervalMs ?? LIVE_SERVER_DEFAULTS.mapUpdateIntervalMs ?? 0;
}

// The live value clears the field, like withSpellTweak.
function withMapUpdateIntervalMs(settings: ServerSettings, value: number): ServerSettings {
	const next = ServerSettings.clone(settings);
	if (value === LIVE_SERVER_DEFAULTS.mapUpdateIntervalMs) {
		delete next.mapUpdateIntervalMs;
	} else {
		next.mapUpdateIntervalMs = value;
	}
	return next;
}

function scaleField(settings: ServerSettings | undefined, set: ScaleSet, stat: ScaleStat): number | undefined {
	return settings?.dungeonScale?.[set]?.[stat];
}

// What the stat is without the encounter's own size key. Same order as newDungeonScale in the sim.
function scaleFallback(settings: ServerSettings, sets: DifficultySets, i: number, stat: ScaleStat): number {
	return (
		scaleField(LIVE_SERVER_DEFAULTS, sets.size[i], stat) ??
		scaleField(settings, sets.generic[i], stat) ??
		scaleField(LIVE_SERVER_DEFAULTS, sets.generic[i], stat) ??
		1
	);
}

function scaleModifier(settings: ServerSettings, difficulty: RaidDifficulty, boss: boolean, stat: ScaleStat): number {
	const sets = setsFor(difficulty);
	const i = boss ? 1 : 0;
	return scaleField(settings, sets.size[i], stat) ?? scaleFallback(settings, sets, i, stat);
}

// Edits the difficulty's own size set, since a live size key would hide a change to the generic one.
function withScaleModifier(settings: ServerSettings, difficulty: RaidDifficulty, boss: boolean, stat: ScaleStat, value: number): ServerSettings {
	const sets = setsFor(difficulty);
	const i = boss ? 1 : 0;
	const next = ServerSettings.clone(settings);
	const dungeonScale = next.dungeonScale ?? DungeonScaleSettings.create();
	const modifiers = dungeonScale[sets.size[i]] ?? DungeonScaleModifiers.create();
	if (value === scaleFallback(settings, sets, i, stat)) {
		delete modifiers[stat];
	} else {
		modifiers[stat] = value;
	}

	if (DungeonScaleModifiers.equals(modifiers, DungeonScaleModifiers.create())) {
		delete dungeonScale[sets.size[i]];
	} else {
		dungeonScale[sets.size[i]] = modifiers;
	}
	if (DungeonScaleSettings.equals(dungeonScale, DungeonScaleSettings.create())) {
		delete next.dungeonScale;
	} else {
		next.dungeonScale = dungeonScale;
	}
	return next;
}

export const SERVER_SETTINGS_TOOLTIP =
	"The AzerothCore config the sim follows. Everything starts at the live server's value and saves with the encounter.";

// The encounter's AzerothCore config. Every input starts at the live server's value, and only the
// ones changed from it end up in the encounter.
export class ServerSettingsPicker extends Component {
	constructor(parent: HTMLElement, encounter: Encounter) {
		super(parent, 'server-settings-picker-root');

		new NumberPicker<Encounter>(this.rootElem, encounter, {
			label: 'Map update interval (ms)',
			labelTooltip:
				'MapUpdateInterval, the server tick. Swings, finished casts and expiring buffs wait for the next tick. ' +
				'0 is exact timing, which no real server has.',
			inline: true,
			positive: true,
			changedEvent: encounter => encounter.serverSettingsChangeEmitter,
			getValue: encounter => mapUpdateIntervalMs(encounter.getServerSettings()),
			setValue: (eventID: EventID, encounter: Encounter, newValue: number) =>
				encounter.setServerSettings(eventID, withMapUpdateIntervalMs(encounter.getServerSettings(), newValue)),
		});

		new EnumPicker<Encounter>(this.rootElem, encounter, {
			label: 'Raid difficulty',
			labelTooltip: 'Picks the dungeon scale multipliers below.',
			inline: true,
			values: DIFFICULTIES.map(d => ({ name: d.name, value: d.difficulty })),
			changedEvent: encounter => encounter.serverSettingsChangeEmitter,
			getValue: encounter => difficultyOrDefault(encounter.getRaidDifficulty()),
			setValue: (eventID: EventID, encounter: Encounter, newValue: number) => encounter.setRaidDifficulty(eventID, newValue),
		});

		this.buildDungeonScale(encounter);
		this.buildSpellTweaks(encounter);
	}

	private buildDungeonScale(encounter: Encounter) {
		const grid = document.createElement('div');
		grid.classList.add('dungeon-scale-picker', 'mb-3');
		Object.assign(grid.style, {
			display: 'grid',
			gridTemplateColumns: 'minmax(0, 1fr) 5rem 5rem',
			columnGap: '.5rem',
			rowGap: '.25rem',
			alignItems: 'center',
		});
		this.rootElem.appendChild(grid);

		const label = (text: string) => {
			const elem = document.createElement('label');
			elem.classList.add('form-label', 'mb-0');
			elem.textContent = text;
			grid.appendChild(elem);
			return elem;
		};

		new Tooltip(label('Dungeon scale'), {
			title:
				"mod-dungeon-scale's full raid multipliers for this raid difficulty, global times each stat. " +
				'Boss is any target flagged as a world boss, which a level 83 target is unless set otherwise. ' +
				'A change only applies to this difficulty.',
			html: true,
		});
		label('Boss');
		label('Other');

		SCALE_STATS.forEach(({ stat, label: statLabel }) => {
			label(statLabel);
			[true, false].forEach(boss => {
				new NumberPicker<Encounter>(grid, encounter, {
					float: true,
					extraCssClasses: ['mb-0'],
					changedEvent: encounter => encounter.serverSettingsChangeEmitter,
					getValue: encounter => scaleModifier(encounter.getServerSettings(), encounter.getRaidDifficulty(), boss, stat),
					setValue: (eventID: EventID, encounter: Encounter, newValue: number) =>
						encounter.setServerSettings(
							eventID,
							withScaleModifier(encounter.getServerSettings(), encounter.getRaidDifficulty(), boss, stat, newValue),
						),
				});
			});
		});
	}

	private buildSpellTweaks(encounter: Encounter) {
		const enabled = (encounter: Encounter) => spellTweaksEnabled(encounter.getServerSettings());

		new BooleanPicker<Encounter>(this.rootElem, encounter, {
			label: 'Spell tweaks',
			labelTooltip:
				'SpellTweaks.Enable. Off turns off every tweak below except the two armor pen ones, and the tweaks with no switch of their own: ' +
				'CS and DS seal stacks, Glyph of Reckoning damage, Explosive Trap via Trap Launcher, and the Detonate Mana and Frost Blast nerfs.',
			inline: true,
			reverse: true,
			changedEvent: encounter => encounter.serverSettingsChangeEmitter,
			getValue: enabled,
			setValue: (eventID: EventID, encounter: Encounter, newValue: boolean) =>
				encounter.setServerSettings(eventID, withSpellTweak(encounter.getServerSettings(), 'enable', newValue)),
		});

		SPELL_TWEAK_SWITCHES.forEach(tweak => {
			new BooleanPicker<Encounter>(this.rootElem, encounter, {
				label: tweak.label,
				labelTooltip: tweak.tooltip,
				inline: true,
				reverse: true,
				changedEvent: encounter => encounter.serverSettingsChangeEmitter,
				// shows what the sim does, so a gated switch reads off while Spell tweaks is off
				getValue: encounter => (tweak.ungated || enabled(encounter)) && (spellTweak(encounter.getServerSettings(), tweak.key) ?? false),
				setValue: (eventID: EventID, encounter: Encounter, newValue: boolean) =>
					encounter.setServerSettings(eventID, withSpellTweak(encounter.getServerSettings(), tweak.key, newValue)),
				enableWhen: encounter => tweak.ungated || enabled(encounter),
			});
		});

		new NumberPicker<Encounter>(this.rootElem, encounter, {
			label: 'Exotic pet damage %',
			labelTooltip: 'SpellTweaks.ExoticPetDamage.Pct: extra damage for BM exotic pets. Below 0 counts as 0.',
			inline: true,
			float: true,
			changedEvent: encounter => encounter.serverSettingsChangeEmitter,
			getValue: encounter => spellTweak(encounter.getServerSettings(), 'exoticPetDamagePct') ?? 0,
			setValue: (eventID: EventID, encounter: Encounter, newValue: number) =>
				encounter.setServerSettings(eventID, withSpellTweak(encounter.getServerSettings(), 'exoticPetDamagePct', newValue)),
			enableWhen: enabled,
		});
	}
}
