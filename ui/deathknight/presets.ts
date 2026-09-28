import {
	Consumes,
	Explosive,
	Flask,
	Food,
	Glyphs,
	PetFood,
	Potions,
	UnitReference,
} from '../core/proto/common.js';
import { SavedTalents } from '../core/proto/ui.js';

import {
	Deathknight_Options as DeathKnightOptions,
	DeathknightMajorGlyph,
	DeathknightMinorGlyph,
} from '../core/proto/deathknight.js';

import * as PresetUtils from '../core/preset_utils.js';

// Preset options for this spec.
// Eventually we will import these values for the raid sim too, so its good to
// keep them in a separate file.

import P1BloodGear from './gear_sets/p1_blood.gear.json';
export const P1_BLOOD_PRESET = PresetUtils.makePresetGear('P1 Blood', P1BloodGear, { talentTree: 0 });
import P3BloodGear from './gear_sets/p3_blood.gear.json';
export const P3_BLOOD_PRESET = PresetUtils.makePresetGear('P3 Blood', P3BloodGear, { talentTree: 0 });
import P1FrostGear from './gear_sets/p1_frost.gear.json';
export const P1_FROST_PRESET = PresetUtils.makePresetGear('P1 Frost', P1FrostGear, { talentTree: 1 });
import P2FrostGear from './gear_sets/p2_frost.gear.json';
export const P2_FROST_PRESET = PresetUtils.makePresetGear('P2 Frost', P2FrostGear, { talentTree: 1 });
import P3FrostGear from './gear_sets/p3_frost.gear.json';
export const P3_FROST_PRESET = PresetUtils.makePresetGear('P3 Frost', P3FrostGear, { talentTree: 1 });
import P4FrostGear from './gear_sets/p4_frost.gear.json';
export const P4_FROST_PRESET = PresetUtils.makePresetGear('P4 Frost', P4FrostGear, { talentTree: 1 });
import P1UhDwGear from './gear_sets/p1_uh_dw.gear.json';
export const P1_UNHOLY_DW_PRESET = PresetUtils.makePresetGear('P1 DW Unholy', P1UhDwGear, { talentTree: 2 });
import P2UhDwGear from './gear_sets/p2_uh_dw.gear.json';
export const P2_UNHOLY_DW_PRESET = PresetUtils.makePresetGear('P2 DW Unholy', P2UhDwGear, { talentTree: 2 });
import P3UhDwGear from './gear_sets/p3_uh_dw.gear.json';
export const P3_UNHOLY_DW_PRESET = PresetUtils.makePresetGear('P3 DW Unholy', P3UhDwGear, { talentTree: 2 });
import P4UhDwGear from './gear_sets/p4_uh_dw.gear.json';
export const P4_UNHOLY_DW_PRESET = PresetUtils.makePresetGear('P4 DW Unholy', P4UhDwGear, { talentTree: 2 });

import BisPresets from './gear_sets/bis_presets.json';
import P1BisBloodGear from './gear_sets/p1_bis_blood.gear.json';
export const P1_BIS_BLOOD_PRESET = PresetUtils.makeBisPresetGear(BisPresets.p1_bis_blood, P1BisBloodGear, { talentTree: 0 });
import P2BisBloodGear from './gear_sets/p2_bis_blood.gear.json';
export const P2_BIS_BLOOD_PRESET = PresetUtils.makeBisPresetGear(BisPresets.p2_bis_blood, P2BisBloodGear, { talentTree: 0 });
import P3BisBloodGear from './gear_sets/p3_bis_blood.gear.json';
export const P3_BIS_BLOOD_PRESET = PresetUtils.makeBisPresetGear(BisPresets.p3_bis_blood, P3BisBloodGear, { talentTree: 0 });
import P4BisBloodGear from './gear_sets/p4_bis_blood.gear.json';
export const P4_BIS_BLOOD_PRESET = PresetUtils.makeBisPresetGear(BisPresets.p4_bis_blood, P4BisBloodGear, { talentTree: 0 });
import P5BisBloodGear from './gear_sets/p5_bis_blood.gear.json';
export const P5_BIS_BLOOD_PRESET = PresetUtils.makeBisPresetGear(BisPresets.p5_bis_blood, P5BisBloodGear, { talentTree: 0 });
import P1BisFrostGear from './gear_sets/p1_bis_frost.gear.json';
export const P1_BIS_FROST_PRESET = PresetUtils.makeBisPresetGear(BisPresets.p1_bis_frost, P1BisFrostGear, { talentTree: 1 });
import P2BisFrostGear from './gear_sets/p2_bis_frost.gear.json';
export const P2_BIS_FROST_PRESET = PresetUtils.makeBisPresetGear(BisPresets.p2_bis_frost, P2BisFrostGear, { talentTree: 1 });
import P3BisFrostGear from './gear_sets/p3_bis_frost.gear.json';
export const P3_BIS_FROST_PRESET = PresetUtils.makeBisPresetGear(BisPresets.p3_bis_frost, P3BisFrostGear, { talentTree: 1 });
import P4BisFrostGear from './gear_sets/p4_bis_frost.gear.json';
export const P4_BIS_FROST_PRESET = PresetUtils.makeBisPresetGear(BisPresets.p4_bis_frost, P4BisFrostGear, { talentTree: 1 });
import P5BisFrostGear from './gear_sets/p5_bis_frost.gear.json';
export const P5_BIS_FROST_PRESET = PresetUtils.makeBisPresetGear(BisPresets.p5_bis_frost, P5BisFrostGear, { talentTree: 1 });
import P1BisUnholyGear from './gear_sets/p1_bis_unholy.gear.json';
export const P1_BIS_UNHOLY_PRESET = PresetUtils.makeBisPresetGear(BisPresets.p1_bis_unholy, P1BisUnholyGear, { talentTree: 2 });
import P2BisUnholyGear from './gear_sets/p2_bis_unholy.gear.json';
export const P2_BIS_UNHOLY_PRESET = PresetUtils.makeBisPresetGear(BisPresets.p2_bis_unholy, P2BisUnholyGear, { talentTree: 2 });
import P3BisUnholyGear from './gear_sets/p3_bis_unholy.gear.json';
export const P3_BIS_UNHOLY_PRESET = PresetUtils.makeBisPresetGear(BisPresets.p3_bis_unholy, P3BisUnholyGear, { talentTree: 2 });
import P4BisUnholyGear from './gear_sets/p4_bis_unholy.gear.json';
export const P4_BIS_UNHOLY_PRESET = PresetUtils.makeBisPresetGear(BisPresets.p4_bis_unholy, P4BisUnholyGear, { talentTree: 2 });
import P5BisUnholyGear from './gear_sets/p5_bis_unholy.gear.json';
export const P5_BIS_UNHOLY_PRESET = PresetUtils.makeBisPresetGear(BisPresets.p5_bis_unholy, P5BisUnholyGear, { talentTree: 2 });

import BloodDPSApl from './apls/blood_dps.apl.json';
export const BLOOD_DPS_ROTATION_PRESET_DEFAULT = PresetUtils.makePresetAPLRotation('Blood DPS', BloodDPSApl, { talentTree: 0 });
import BloodPestiAoeApl from './apls/blood_pesti_aoe.apl.json';
export const BLOOD_PESTI_AOE_ROTATION_PRESET_DEFAULT = PresetUtils.makePresetAPLRotation('Blood Pesti AOE', BloodPestiAoeApl, { talentTree: 0 });
import FrostBlPestiApl from './apls/frost_bl_pesti.apl.json';
export const FROST_BL_PESTI_ROTATION_PRESET_DEFAULT = PresetUtils.makePresetAPLRotation('Frost BL Pesti', FrostBlPestiApl, { talentTree: 1 });
import FrostUhPestiApl from './apls/frost_uh_pesti.apl.json';
export const FROST_UH_PESTI_ROTATION_PRESET_DEFAULT = PresetUtils.makePresetAPLRotation('Frost UH Pesti', FrostUhPestiApl, { talentTree: 1 });
import UhDwSsApl from './apls/unholy_dw_ss.apl.json';
export const UNHOLY_DW_ROTATION_PRESET_DEFAULT = PresetUtils.makePresetAPLRotation('Unholy DW SS', UhDwSsApl, { talentTree: 2 });
import Uh2hSsApl from './apls/uh_2h_ss.apl.json';
export const UNHOLY_2H_ROTATION_PRESET_DEFAULT = PresetUtils.makePresetAPLRotation('Unholy 2H SS', Uh2hSsApl, { talentTree: 2 });
import UhDndAoeApl from './apls/uh_dnd_aoe.apl.json';
export const UNHOLY_DND_AOE_ROTATION_PRESET_DEFAULT = PresetUtils.makePresetAPLRotation('Unholy DND AOE', UhDndAoeApl, { talentTree: 2 });

// Default talents. Uses the wowhead calculator format, make the talents on
// https://wotlk.wowhead.com/talent-calc and copy the numbers in the url.
export const FrostTalents = {
	name: 'Frost BL',
	data: SavedTalents.create({
		talentsString: '23050005-32005350352203012300033101351',
		glyphs: Glyphs.create({
			major1: DeathknightMajorGlyph.GlyphOfObliterate,
			major2: DeathknightMajorGlyph.GlyphOfFrostStrike,
			major3: DeathknightMajorGlyph.GlyphOfDisease,
			minor1: DeathknightMinorGlyph.GlyphOfHornOfWinter,
			minor2: DeathknightMinorGlyph.GlyphOfPestilence,
			minor3: DeathknightMinorGlyph.GlyphOfRaiseDead,
		}),
	}),
};

export const FrostUnholyTalents = {
	name: 'Frost UH',
	data: SavedTalents.create({
		talentsString: '01-32002350342203012300033101351-230200305003',
		glyphs: Glyphs.create({
			major1: DeathknightMajorGlyph.GlyphOfObliterate,
			major2: DeathknightMajorGlyph.GlyphOfFrostStrike,
			major3: DeathknightMajorGlyph.GlyphOfDisease,
			minor1: DeathknightMinorGlyph.GlyphOfHornOfWinter,
			minor2: DeathknightMinorGlyph.GlyphOfPestilence,
			minor3: DeathknightMinorGlyph.GlyphOfRaiseDead,
		}),
	}),
};

export const UnholyDualWieldTalents = {
	name: 'Unholy DW',
	data: SavedTalents.create({
		talentsString: '-320043500002-2300303050032152000150013133051',
		glyphs: Glyphs.create({
			major1: DeathknightMajorGlyph.GlyphOfTheGhoul,
			major2: DeathknightMajorGlyph.GlyphOfIcyTouch,
			major3: DeathknightMajorGlyph.GlyphOfDeathAndDecay,
			minor1: DeathknightMinorGlyph.GlyphOfHornOfWinter,
			minor2: DeathknightMinorGlyph.GlyphOfPestilence,
			minor3: DeathknightMinorGlyph.GlyphOfRaiseDead,
		}),
	}),
};

export const UnholyDualWieldSSTalents = {
	name: 'Unholy DW SS',
	data: SavedTalents.create({
		talentsString: '-320033500002-2301303050032151000150013133151',
		glyphs: Glyphs.create({
			major1: DeathknightMajorGlyph.GlyphOfTheGhoul,
			major2: DeathknightMajorGlyph.GlyphOfIcyTouch,
			major3: DeathknightMajorGlyph.GlyphOfDeathAndDecay,
			minor1: DeathknightMinorGlyph.GlyphOfHornOfWinter,
			minor2: DeathknightMinorGlyph.GlyphOfPestilence,
			minor3: DeathknightMinorGlyph.GlyphOfRaiseDead,
		}),
	}),
};

export const Unholy2HTalents = {
	name: 'Unholy 2H',
	data: SavedTalents.create({
		talentsString: '-320050500002-2302003350032052000150013133151',
		glyphs: Glyphs.create({
			major1: DeathknightMajorGlyph.GlyphOfTheGhoul,
			major2: DeathknightMajorGlyph.GlyphOfIcyTouch,
			major3: DeathknightMajorGlyph.GlyphOfDarkDeath,
			minor1: DeathknightMinorGlyph.GlyphOfHornOfWinter,
			minor2: DeathknightMinorGlyph.GlyphOfPestilence,
			minor3: DeathknightMinorGlyph.GlyphOfRaiseDead,
		}),
	}),
};

export const UnholyAoeTalents = {
	name: 'Unholy AOE',
	data: SavedTalents.create({
		talentsString: '-320050500002-2302303050032052000150013133151',
		glyphs: Glyphs.create({
			major1: DeathknightMajorGlyph.GlyphOfTheGhoul,
			major2: DeathknightMajorGlyph.GlyphOfIcyTouch,
			major3: DeathknightMajorGlyph.GlyphOfDeathAndDecay,
			minor1: DeathknightMinorGlyph.GlyphOfHornOfWinter,
			minor2: DeathknightMinorGlyph.GlyphOfPestilence,
			minor3: DeathknightMinorGlyph.GlyphOfRaiseDead,
		}),
	}),
};

export const BloodTalents = {
	name: 'Blood DPS',
	data: SavedTalents.create({
		talentsString: '2305120530003303231023001351--2302003050032',
		glyphs: Glyphs.create({
			major1: DeathknightMajorGlyph.GlyphOfDancingRuneWeapon,
			major2: DeathknightMajorGlyph.GlyphOfDeathStrike,
			major3: DeathknightMajorGlyph.GlyphOfDisease,
			minor1: DeathknightMinorGlyph.GlyphOfHornOfWinter,
			minor2: DeathknightMinorGlyph.GlyphOfPestilence,
			minor3: DeathknightMinorGlyph.GlyphOfRaiseDead,
		}),
	}),
};

export const DefaultUnholyOptions = DeathKnightOptions.create({
	startingRunicPower: 0,
	petUptime: 1,
	unholyFrenzyTarget: UnitReference.create(),
	drwPestiApply: true,
});

export const DefaultFrostOptions = DeathKnightOptions.create({
	startingRunicPower: 0,
	petUptime: 1,
	unholyFrenzyTarget: UnitReference.create(),
	drwPestiApply: true,
});

export const DefaultBloodOptions = DeathKnightOptions.create({
	startingRunicPower: 0,
	petUptime: 1,
	unholyFrenzyTarget: UnitReference.create(),
	drwPestiApply: true,
});

export const OtherDefaults = {
};

export const DefaultConsumes = Consumes.create({
	flask: Flask.FlaskOfEndlessRage,
	food: Food.FoodDragonfinFilet,
	defaultPotion: Potions.PotionOfSpeed,
	petFood: PetFood.PetFoodSpicedMammothTreats,
	prepopPotion: Potions.PotionOfSpeed,
	thermalSapper: true,
	fillerExplosive: Explosive.ExplosiveSaroniteBomb,
});
