import {
	BattleElixir,
	Consumes,
	Explosive,
	Food,
	Glyphs,
	GuardianElixir,
	Potions,
	Spec,
} from '../core/proto/common.js';
import { SavedTalents } from '../core/proto/ui.js';

import {
	WarriorShout,
	ProtectionWarrior_Rotation as ProtectionWarriorRotation,
	ProtectionWarrior_Options as ProtectionWarriorOptions,
	WarriorMajorGlyph,
	WarriorMinorGlyph,
} from '../core/proto/warrior.js';

import * as PresetUtils from '../core/preset_utils.js';

// Preset options for this spec.
// Eventually we will import these values for the raid sim too, so its good to
// keep them in a separate file.

import P1BalancedGear from './gear_sets/p1_balanced.gear.json';
export const P1_BALANCED_PRESET = PresetUtils.makePresetGear('P1 Preset', P1BalancedGear);
import P2SurvivalGear from './gear_sets/p2_survival.gear.json';
export const P2_SURVIVAL_PRESET = PresetUtils.makePresetGear('P2 Preset', P2SurvivalGear);
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
export const ROTATION_DEFAULT = PresetUtils.makePresetAPLRotation('Default APL', DefaultApl);
export const ROTATION_PRESET_SIMPLE = PresetUtils.makePresetSimpleRotation('Simple Cooldowns', Spec.SpecProtectionWarrior, ProtectionWarriorRotation.create());


// Default talents. Uses the wowhead calculator format, make the talents on
// https://wowhead.com/wotlk/talent-calc and copy the numbers in the url.
export const StandardTalents = {
	name: 'Standard',
	data: SavedTalents.create({
		talentsString: '2500030023-302-053351225000012521030113321',
		glyphs: Glyphs.create({
			major1: WarriorMajorGlyph.GlyphOfBlocking,
			major2: WarriorMajorGlyph.GlyphOfVigilance,
			major3: WarriorMajorGlyph.GlyphOfDevastate,
			minor1: WarriorMinorGlyph.GlyphOfCharge,
			minor2: WarriorMinorGlyph.GlyphOfThunderClap,
			minor3: WarriorMinorGlyph.GlyphOfCommand,
		}),
	}),
};

export const UATalents = {
	name: 'UA',
	data: SavedTalents.create({
		talentsString: '35023301230051002020120002-2-05035122500000252',
		glyphs: Glyphs.create({
			major1: WarriorMajorGlyph.GlyphOfRevenge,
			major2: WarriorMajorGlyph.GlyphOfHeroicStrike,
			major3: WarriorMajorGlyph.GlyphOfSweepingStrikes,
			minor1: WarriorMinorGlyph.GlyphOfCharge,
			minor2: WarriorMinorGlyph.GlyphOfThunderClap,
			minor3: WarriorMinorGlyph.GlyphOfCommand,
		}),
	}),
};

export const DefaultOptions = ProtectionWarriorOptions.create({
	shout: WarriorShout.WarriorShoutCommanding,
	useShatteringThrow: false,
	startingRage: 0,
});

export const DefaultConsumes = Consumes.create({
	battleElixir: BattleElixir.ElixirOfExpertise,
	guardianElixir: GuardianElixir.ElixirOfProtection,
	food: Food.FoodDragonfinFilet,
	defaultPotion: Potions.IndestructiblePotion,
	prepopPotion: Potions.IndestructiblePotion,
	thermalSapper: true,
	fillerExplosive: Explosive.ExplosiveSaroniteBomb,
});
