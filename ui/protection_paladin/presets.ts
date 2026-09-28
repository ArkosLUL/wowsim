import {
	Consumes,
	Flask,
	Food,
	Potions,
} from '../core/proto/common.js';
import { SavedTalents } from '../core/proto/ui.js';

import {
	PaladinAura as PaladinAura,
	PaladinMajorGlyph,
	PaladinMinorGlyph,
	PaladinJudgement as PaladinJudgement,
	ProtectionPaladin_Options as ProtectionPaladinOptions,
} from '../core/proto/paladin.js';

import * as PresetUtils from '../core/preset_utils.js';

// Preset options for this spec.
// Eventually we will import these values for the raid sim too, so its good to
// keep them in a separate file.

import P1Gear from './gear_sets/p1.gear.json';
export const P1_PRESET = PresetUtils.makePresetGear('P1 Preset', P1Gear);
import P2Gear from './gear_sets/p2.gear.json';
export const P2_PRESET = PresetUtils.makePresetGear('P2 Preset', P2Gear);
import P3Gear from './gear_sets/p3.gear.json';
export const P3_PRESET = PresetUtils.makePresetGear('P3 Preset', P3Gear);
import P4Gear from './gear_sets/p4.gear.json';
export const P4_PRESET = PresetUtils.makePresetGear('P4 Preset', P4Gear);

import BisPresets from './gear_sets/bis_presets.json';
import P1BisProtectionGear from './gear_sets/p1_bis.gear.json';
export const P1_BIS_PROTECTION_PRESET = PresetUtils.makeBisPresetGear(BisPresets.p1_bis, P1BisProtectionGear);
import P2BisProtectionGear from './gear_sets/p2_bis.gear.json';
export const P2_BIS_PROTECTION_PRESET = PresetUtils.makeBisPresetGear(BisPresets.p2_bis, P2BisProtectionGear);
import P3BisProtectionGear from './gear_sets/p3_bis.gear.json';
export const P3_BIS_PROTECTION_PRESET = PresetUtils.makeBisPresetGear(BisPresets.p3_bis, P3BisProtectionGear);
import P4BisProtectionGear from './gear_sets/p4_bis.gear.json';
export const P4_BIS_PROTECTION_PRESET = PresetUtils.makeBisPresetGear(BisPresets.p4_bis, P4BisProtectionGear);
import P5BisProtectionGear from './gear_sets/p5_bis.gear.json';
export const P5_BIS_PROTECTION_PRESET = PresetUtils.makeBisPresetGear(BisPresets.p5_bis, P5BisProtectionGear);

import DefaultApl from './apls/default.apl.json';
export const ROTATION_DEFAULT = PresetUtils.makePresetAPLRotation('Default (969)', DefaultApl);

// Default talents. Uses the wowhead calculator format, make the talents on
// https://wowhead.com/wotlk/talent-calc and copy the numbers in the url.

export const GenericAoeTalents = {
	name: 'Baseline Example',
	data: SavedTalents.create({
		talentsString: '-05005135200132311333312321-511302012003',
		glyphs: {
			major1: PaladinMajorGlyph.GlyphOfSealOfVengeance,
			major2: PaladinMajorGlyph.GlyphOfRighteousDefense,
			major3: PaladinMajorGlyph.GlyphOfDivinePlea,
			minor1: PaladinMinorGlyph.GlyphOfSenseUndead,
			minor2: PaladinMinorGlyph.GlyphOfLayOnHands,
			minor3: PaladinMinorGlyph.GlyphOfBlessingOfKings
		}
	}),
};

export const DefaultOptions = ProtectionPaladinOptions.create({
	aura: PaladinAura.RetributionAura,
	judgement: PaladinJudgement.JudgementOfWisdom,
});

export const DefaultConsumes = Consumes.create({
	flask: Flask.FlaskOfStoneblood,
	food: Food.FoodDragonfinFilet,
	defaultPotion: Potions.IndestructiblePotion,
	prepopPotion: Potions.IndestructiblePotion,
});
