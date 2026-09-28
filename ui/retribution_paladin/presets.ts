import {
	Conjured,
	Consumes,
	Flask,
	Food,
	Glyphs,
	Potions,
} from '../core/proto/common.js';
import { SavedTalents } from '../core/proto/ui.js';

import {
	PaladinAura as PaladinAura,
	PaladinJudgement as PaladinJudgement,
	RetributionPaladin_Options as RetributionPaladinOptions,
	PaladinMajorGlyph,
	PaladinMinorGlyph,
} from '../core/proto/paladin.js';

import * as PresetUtils from '../core/preset_utils.js';

// Preset options for this spec.
// Eventually we will import these values for the raid sim too, so its good to
// keep them in a separate file.

import P1Gear from './gear_sets/p1.gear.json';
export const P1_PRESET = PresetUtils.makePresetGear('P1 Preset', P1Gear);
import P2Gear from './gear_sets/p2.gear.json';
export const P2_PRESET = PresetUtils.makePresetGear('P2 Preset', P2Gear);
import P3MaceGear from './gear_sets/p3_mace.gear.json';
export const P3_PRESET = PresetUtils.makePresetGear('P3 Mace Preset', P3MaceGear);
import P4Gear from './gear_sets/p4.gear.json';
export const P4_PRESET = PresetUtils.makePresetGear('P4 Preset', P4Gear);
import P5Gear from './gear_sets/p5.gear.json';
export const P5_PRESET = PresetUtils.makePresetGear('P5 Preset', P5Gear);

import BisPresets from './gear_sets/bis_presets.json';
import P1BisRetributionGear from './gear_sets/p1_bis.gear.json';
export const P1_BIS_RETRIBUTION_PRESET = PresetUtils.makeBisPresetGear(BisPresets.p1_bis, P1BisRetributionGear);
import P2BisRetributionGear from './gear_sets/p2_bis.gear.json';
export const P2_BIS_RETRIBUTION_PRESET = PresetUtils.makeBisPresetGear(BisPresets.p2_bis, P2BisRetributionGear);
import P3BisRetributionGear from './gear_sets/p3_bis.gear.json';
export const P3_BIS_RETRIBUTION_PRESET = PresetUtils.makeBisPresetGear(BisPresets.p3_bis, P3BisRetributionGear);
import P4BisRetributionGear from './gear_sets/p4_bis.gear.json';
export const P4_BIS_RETRIBUTION_PRESET = PresetUtils.makeBisPresetGear(BisPresets.p4_bis, P4BisRetributionGear);
import P5BisRetributionGear from './gear_sets/p5_bis.gear.json';
export const P5_BIS_RETRIBUTION_PRESET = PresetUtils.makeBisPresetGear(BisPresets.p5_bis, P5BisRetributionGear);

import DefaultApl from './apls/default.apl.json';
export const ROTATION_PRESET_DEFAULT = PresetUtils.makePresetAPLRotation('Default', DefaultApl);

// Default talents. Uses the wowhead calculator format, make the talents on
// https://wowhead.com/wotlk/talent-calc and copy the numbers in the url.
export const AuraMasteryTalents = {
	name: 'Aura Mastery',
	data: SavedTalents.create({
		talentsString: '050501-05-05232051203331302133231331',
		glyphs: Glyphs.create({
			major1: PaladinMajorGlyph.GlyphOfSealOfVengeance,
			major2: PaladinMajorGlyph.GlyphOfJudgement,
			major3: PaladinMajorGlyph.GlyphOfConsecration,
			minor1: PaladinMinorGlyph.GlyphOfSenseUndead,
			minor2: PaladinMinorGlyph.GlyphOfLayOnHands,
			minor3: PaladinMinorGlyph.GlyphOfBlessingOfKings
		})
	}),
};


export const DivineSacTalents = {
	name: 'Divine Sacrifice & Guardian',
	data: SavedTalents.create({
		talentsString: '03-453201002-05222051203331302133201331',
		glyphs: Glyphs.create({
			major1: PaladinMajorGlyph.GlyphOfSealOfVengeance,
			major2: PaladinMajorGlyph.GlyphOfJudgement,
			major3: PaladinMajorGlyph.GlyphOfConsecration,
			minor1: PaladinMinorGlyph.GlyphOfSenseUndead,
			minor2: PaladinMinorGlyph.GlyphOfLayOnHands,
			minor3: PaladinMinorGlyph.GlyphOfBlessingOfKings
		})
	}),
};

export const DefaultOptions = RetributionPaladinOptions.create({
	aura: PaladinAura.RetributionAura,
	judgement: PaladinJudgement.JudgementOfWisdom,
});

export const DefaultConsumes = Consumes.create({
	defaultPotion: Potions.PotionOfSpeed,
	defaultConjured: Conjured.ConjuredDarkRune,
	flask: Flask.FlaskOfEndlessRage,
	food: Food.FoodDragonfinFilet,
});