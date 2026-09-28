import {
	APLRotation,
	APLRotation_Type as APLRotationType,
} from './proto/apl';
import {
	EquipmentSpec,
    Faction,
    Race,
    Spec,
} from './proto/common';
import {
    SavedRotation,
} from './proto/ui';

import { Player } from './player';
import { raceNames } from './proto_utils/names';
import {
    SpecRotation,
	specTypeFunctions,
} from './proto_utils/utils';

import * as Tooltips from './constants/tooltips.js';

export interface PresetGear {
	name: string;
	gear: EquipmentSpec;
	tooltip?: string;
	enableWhen?: (obj: Player<any>) => boolean;
}

export interface PresetRotation {
	name: string;
	rotation: SavedRotation;
	tooltip?: string;
	enableWhen?: (obj: Player<any>) => boolean;
}

export interface PresetGearOptions {
    talentTree?: number,
    talentTrees?: Array<number>,
    faction?: Faction,
    customCondition?: (player: Player<any>) => boolean,

    tooltip?: string,
}

export interface PresetRotationOptions {
    talentTree?: number,
}

export function makePresetGear(name: string, gearJson: any, options?: PresetGearOptions): PresetGear {
    const gear = EquipmentSpec.fromJson(gearJson);
    return makePresetGearHelper(name, gear, options || {});
}

// an entry of a spec's gear_sets/bis_presets.json, which tools/presetgen writes with each gear file
export interface BisPresetInfo {
    name: string,
    build: string,
    phase: number,
    effort: string,
    racialTraits: string,
    simCommit: string,
    catalogDate: string,
}

export function makeBisPresetGear(info: BisPresetInfo, gearJson: any, options?: PresetGearOptions): PresetGear {
    const race = raceNames.get(Race[info.racialTraits as keyof typeof Race]) || info.racialTraits;
    const tooltip = `<p>Phase ${info.phase} BiS for ${info.build}, picked by the BiS optimizer at ${info.effort} effort. It assumes ${race} racial traits.</p>`
        + `<p>Sim commit ${info.simCommit}, item catalog from ${info.catalogDate}.</p>`;
    return makePresetGear(info.name, gearJson, { ...options, tooltip });
}

function makePresetGearHelper(name: string, gear: EquipmentSpec, options: PresetGearOptions): PresetGear {
    let conditions: Array<(player: Player<any>) => boolean> = [];
    if (options.talentTree != undefined) {
        conditions.push((player: Player<any>) => player.getTalentTree() == options.talentTree);
    }
    if (options.talentTrees != undefined) {
        conditions.push((player: Player<any>) => (options.talentTrees || []).includes(player.getTalentTree()));
    }
    if (options.faction != undefined) {
        conditions.push((player: Player<any>) => player.getFaction() == options.faction);
    }
    if (options.customCondition != undefined) {
        conditions.push(options.customCondition);
    }

    return {
        name: name,
        tooltip: options.tooltip || Tooltips.BASIC_BIS_DISCLAIMER,
        gear: gear,
        enableWhen: conditions.length > 0
            ? (player: Player<any>) => conditions.every(cond => cond(player))
            : undefined,
    };
}

export function makePresetAPLRotation(name: string, rotationJson: any, options?: PresetRotationOptions): PresetRotation {
    const rotation = SavedRotation.create({
        rotation: APLRotation.fromJson(rotationJson),
    });
    return makePresetRotationHelper(name, rotation, options);
}

export function makePresetSimpleRotation<SpecType extends Spec>(name: string, spec: SpecType, simpleRotation: SpecRotation<SpecType>, options?: PresetRotationOptions): PresetRotation {
    const rotation = SavedRotation.create({
		rotation: {
			type: APLRotationType.TypeSimple,
			simple: {
				specRotationJson: JSON.stringify(specTypeFunctions[spec].rotationToJson(simpleRotation)),
			},
		},
    });
    return makePresetRotationHelper(name, rotation, options);
}

function makePresetRotationHelper(name: string, rotation: SavedRotation, options?: PresetRotationOptions): PresetRotation {
    let conditions: Array<(player: Player<any>) => boolean> = [];
    if (options?.talentTree != undefined) {
        conditions.push((player: Player<any>) => player.getTalentTree() == options.talentTree);
    }

    return {
        name: name,
        rotation: rotation,
        enableWhen: conditions.length > 0
            ? (player: Player<any>) => conditions.every(cond => cond(player))
            : undefined,
    };
}