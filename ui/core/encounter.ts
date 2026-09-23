import {
	Encounter as EncounterProto,
	MobType,
	RaidDifficulty,
	ServerSettings,
	SpellSchool,
	Stat,
	Target as TargetProto,
	TargetInput,
	PresetEncounter,
	PresetTarget,
} from './proto/common.js';
import { Stats } from './proto_utils/stats.js';
import * as Mechanics from './constants/mechanics.js';

import { Sim } from './sim.js';
import { UnitMetadataList } from './player.js';
import { EventID, TypedEvent } from './typed_event.js';

// Manages all the settings for an Encounter.
export class Encounter {
	readonly sim: Sim;

	private duration: number = 180;
	private durationVariation: number = 5;
	private executeProportion20: number = 0.2;
	private executeProportion25: number = 0.25;
	private executeProportion35: number = 0.35;
	private useHealth: boolean = false;
	private raidDifficulty: RaidDifficulty = RaidDifficulty.RaidDifficultyUnknown;
	// Only what the user changed: an unset field is the live server's value.
	private serverSettings: ServerSettings = ServerSettings.create();
	targets: Array<TargetProto>;
	targetsMetadata: UnitMetadataList;

	readonly targetsChangeEmitter = new TypedEvent<void>();
	readonly durationChangeEmitter = new TypedEvent<void>();
	readonly executeProportionChangeEmitter = new TypedEvent<void>();
	// Raid difficulty or server settings.
	readonly serverSettingsChangeEmitter = new TypedEvent<void>();

	// Emits when any of the above emitters emit.
	readonly changeEmitter = new TypedEvent<void>();

	constructor(sim: Sim) {
		this.sim = sim;
		this.targets = [Encounter.defaultTargetProto()];
		this.targetsMetadata = new UnitMetadataList();

		[
			this.targetsChangeEmitter,
			this.durationChangeEmitter,
			this.executeProportionChangeEmitter,
			this.serverSettingsChangeEmitter,
		].forEach(emitter => emitter.on(eventID => this.changeEmitter.emit(eventID)));
	}

	// blank once every target is deleted, which Simulate reports to the user
	get primaryTarget(): TargetProto {
		return TargetProto.clone(this.targets[0] || TargetProto.create());
	}

	getDurationVariation(): number {
		return this.durationVariation;
	}
	setDurationVariation(eventID: EventID, newDuration: number) {
		if (newDuration == this.durationVariation)
			return;

		this.durationVariation = newDuration;
		this.durationChangeEmitter.emit(eventID);
	}

	getDuration(): number {
		return this.duration;
	}
	setDuration(eventID: EventID, newDuration: number) {
		if (newDuration == this.duration)
			return;

		this.duration = newDuration;
		this.durationChangeEmitter.emit(eventID);
	}

	getExecuteProportion20(): number {
		return this.executeProportion20;
	}
	setExecuteProportion20(eventID: EventID, newExecuteProportion20: number) {
		if (newExecuteProportion20 == this.executeProportion20)
			return;

		this.executeProportion20 = newExecuteProportion20;
		this.executeProportionChangeEmitter.emit(eventID);
	}
	getExecuteProportion25(): number {
		return this.executeProportion25;
	}
	setExecuteProportion25(eventID: EventID, newExecuteProportion25: number) {
		if (newExecuteProportion25 == this.executeProportion25)
			return;

		this.executeProportion25 = newExecuteProportion25;
		this.executeProportionChangeEmitter.emit(eventID);
	}
	getExecuteProportion35(): number {
		return this.executeProportion35;
	}
	setExecuteProportion35(eventID: EventID, newExecuteProportion35: number) {
		if (newExecuteProportion35 == this.executeProportion35)
			return;

		this.executeProportion35 = newExecuteProportion35;
		this.executeProportionChangeEmitter.emit(eventID);
	}

	getUseHealth(): boolean {
		return this.useHealth;
	}
	setUseHealth(eventID: EventID, newUseHealth: boolean) {
		if (newUseHealth == this.useHealth)
			return;

		this.useHealth = newUseHealth;
		this.durationChangeEmitter.emit(eventID);
		this.executeProportionChangeEmitter.emit(eventID);
	}

	getRaidDifficulty(): RaidDifficulty {
		return this.raidDifficulty;
	}
	setRaidDifficulty(eventID: EventID, newRaidDifficulty: RaidDifficulty) {
		if (newRaidDifficulty == this.raidDifficulty)
			return;

		this.raidDifficulty = newRaidDifficulty;
		this.serverSettingsChangeEmitter.emit(eventID);
	}

	getServerSettings(): ServerSettings {
		return ServerSettings.clone(this.serverSettings);
	}
	setServerSettings(eventID: EventID, newServerSettings: ServerSettings) {
		if (ServerSettings.equals(newServerSettings, this.serverSettings))
			return;

		this.serverSettings = ServerSettings.clone(newServerSettings);
		this.serverSettingsChangeEmitter.emit(eventID);
	}

	matchesPreset(preset: PresetEncounter): boolean {
		return preset.targets.length == this.targets.length && this.targets.every((t, i) => TargetProto.equals(t, preset.targets[i].target));
	}

	// Targets are edited in place, so they're copied in: an edit must never reach a preset or a saved encounter.
	applyPreset(eventID: EventID, preset: PresetEncounter) {
		this.targets = preset.targets.map(presetTarget => TargetProto.clone(presetTarget.target || TargetProto.create()));
		this.targetsChangeEmitter.emit(eventID);
	}

	applyPresetTarget(eventID: EventID, preset: PresetTarget, index: number) {
		this.targets[index] = TargetProto.clone(preset.target || TargetProto.create());
		this.targetsChangeEmitter.emit(eventID);
	}

	toProto(): EncounterProto {
		return EncounterProto.create({
			duration: this.duration,
			durationVariation: this.durationVariation,
			executeProportion20: this.executeProportion20,
			executeProportion25: this.executeProportion25,
			executeProportion35: this.executeProportion35,
			useHealth: this.useHealth,
			targets: this.targets,
			raidDifficulty: this.raidDifficulty,
			// unset when empty, so it still equals encounters saved without it
			serverSettings: ServerSettings.equals(this.serverSettings, ServerSettings.create()) ? undefined : this.getServerSettings(),
		});
	}

	fromProto(eventID: EventID, proto: EncounterProto) {
		TypedEvent.freezeAllAndDo(() => {
			this.setDuration(eventID, proto.duration);
			this.setDurationVariation(eventID, proto.durationVariation);
			this.setExecuteProportion20(eventID, proto.executeProportion20);
			this.setExecuteProportion25(eventID, proto.executeProportion25);
			this.setExecuteProportion35(eventID, proto.executeProportion35);
			this.setUseHealth(eventID, proto.useHealth);
			this.setRaidDifficulty(eventID, proto.raidDifficulty);
			this.setServerSettings(eventID, proto.serverSettings || ServerSettings.create());
			this.targets = proto.targets.map(target => TargetProto.clone(target));
			this.targetsChangeEmitter.emit(eventID);
		});
	}

	applyDefaults(eventID: EventID) {
		this.fromProto(eventID, EncounterProto.create({
			duration: 180,
			durationVariation: 5,
			executeProportion20: 0.2,
			executeProportion25: 0.25,
			executeProportion35: 0.35,
			targets: [Encounter.defaultTargetProto()],
		}));
	}

	static defaultTargetProto(): TargetProto {
		return TargetProto.create({
			level: Mechanics.BOSS_LEVEL,
			mobType: MobType.MobTypeGiant,
			tankIndex: 0,
			swingSpeed: 1.5,
			minBaseDamage: 65000,
			dualWield: false,
			dualWieldPenalty: false,
			suppressDodge: false,
			parryHaste: true,
			spellSchool: SpellSchool.SpellSchoolPhysical,
			// Block value is left out on purpose: the sim derives a creature's from
			// its level and strength, which is 41 for a level 83 boss.
			stats: Stats.fromMap({
				[Stat.StatArmor]: 10643,
				[Stat.StatAttackPower]: 805,
			}).asArray(),
			targetInputs: new Array<TargetInput>(0),
		});
	}
}
