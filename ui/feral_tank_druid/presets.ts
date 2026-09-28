import {
	BattleElixir,
	Conjured,
	Consumes,
	Explosive,
	Food,
	Glyphs,
	GuardianElixir,
	Potions,
	Spec,
	UnitReference,
} from '../core/proto/common.js';
import { SavedTalents } from '../core/proto/ui.js';

import {
	FeralTankDruid_Rotation as DruidRotation,
	FeralTankDruid_Options as DruidOptions,
	DruidMajorGlyph,
	DruidMinorGlyph,
} from '../core/proto/druid.js';

import * as PresetUtils from '../core/preset_utils.js';

// Preset options for this spec.
// Eventually we will import these values for the raid sim too, so its good to
// keep them in a separate file.

import P1Gear from './gear_sets/p1.gear.json';
export const P1_PRESET = PresetUtils.makePresetGear('P1', P1Gear);
import P2Gear from './gear_sets/p2.gear.json';
export const P2_PRESET = PresetUtils.makePresetGear('P2', P2Gear);
import P3Gear from './gear_sets/p3.gear.json';
export const P3_PRESET = PresetUtils.makePresetGear('P3', P3Gear);
import P4Gear from './gear_sets/p4.gear.json';
export const P4_PRESET = PresetUtils.makePresetGear('P4', P4Gear);

import BisPresets from './gear_sets/bis_presets.json';
import P1BisFeralTankGear from './gear_sets/p1_bis.gear.json';
export const P1_BIS_FERAL_TANK_PRESET = PresetUtils.makeBisPresetGear(BisPresets.p1_bis, P1BisFeralTankGear);
import P2BisFeralTankGear from './gear_sets/p2_bis.gear.json';
export const P2_BIS_FERAL_TANK_PRESET = PresetUtils.makeBisPresetGear(BisPresets.p2_bis, P2BisFeralTankGear);
import P3BisFeralTankGear from './gear_sets/p3_bis.gear.json';
export const P3_BIS_FERAL_TANK_PRESET = PresetUtils.makeBisPresetGear(BisPresets.p3_bis, P3BisFeralTankGear);
import P4BisFeralTankGear from './gear_sets/p4_bis.gear.json';
export const P4_BIS_FERAL_TANK_PRESET = PresetUtils.makeBisPresetGear(BisPresets.p4_bis, P4BisFeralTankGear);
import P5BisFeralTankGear from './gear_sets/p5_bis.gear.json';
export const P5_BIS_FERAL_TANK_PRESET = PresetUtils.makeBisPresetGear(BisPresets.p5_bis, P5BisFeralTankGear);

export const DefaultSimpleRotation = DruidRotation.create({
	maulRageThreshold: 25,
	maintainDemoralizingRoar: true,
	lacerateTime: 8.0,
});

import DefaultApl from './apls/default.apl.json';
export const ROTATION_DEFAULT = PresetUtils.makePresetAPLRotation('APL Default', DefaultApl);

export const ROTATION_PRESET_SIMPLE = PresetUtils.makePresetSimpleRotation('Simple Default', Spec.SpecFeralTankDruid, DefaultSimpleRotation);

// Default talents. Uses the wowhead calculator format, make the talents on
// https://wowhead.com/wotlk/talent-calc and copy the numbers in the url.
export const StandardTalents = {
	name: 'Standard',
	data: SavedTalents.create({
		talentsString: '-503232132322010353120300313511-20350001',
		glyphs: Glyphs.create({
			major1: DruidMajorGlyph.GlyphOfMaul,
			major2: DruidMajorGlyph.GlyphOfSurvivalInstincts,
			major3: DruidMajorGlyph.GlyphOfFrenziedRegeneration,
			minor1: DruidMinorGlyph.GlyphOfChallengingRoar,
			minor2: DruidMinorGlyph.GlyphOfThorns,
			minor3: DruidMinorGlyph.GlyphOfUnburdenedRebirth,
		}),
	}),
};

export const DefaultOptions = DruidOptions.create({
	innervateTarget: UnitReference.create(),
	startingRage: 20,
});

export const DefaultConsumes = Consumes.create({
	battleElixir: BattleElixir.GurusElixir,
	guardianElixir: GuardianElixir.GiftOfArthas,
	food: Food.FoodBlackenedDragonfin,
	prepopPotion: Potions.IndestructiblePotion,
	defaultPotion: Potions.IndestructiblePotion,
	defaultConjured: Conjured.ConjuredHealthstone,
	thermalSapper: true,
	fillerExplosive: Explosive.ExplosiveSaroniteBomb,
});
