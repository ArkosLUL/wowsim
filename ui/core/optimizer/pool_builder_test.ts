// Node test, bundled with esbuild (see README.md): the tank request carries Icecrown's buff in the
// Lich King's phase only, whatever the page had set, and a DPS request is left alone.
import assert from 'assert';
import { readFileSync } from 'fs';
import * as path from 'path';

import { Party, Player as PlayerProto, Raid, RaidSimRequest, SimOptions } from '../proto/api.js';
import { Class, Debuffs, Encounter, ItemSlot, PartyBuffs, Race, RaidBuffs, Spec } from '../proto/common.js';
import { Deathknight_Options as DeathknightOptions } from '../proto/deathknight.js';
import { DatabaseFilters, UIEnchant, UIItem } from '../proto/ui.js';

const repoRoot = process.cwd();

// The UI modules read window.location when they load, and Database and the catalog fetch
// /wotlk/assets/..., so both need stand-ins first. Hence the dynamic imports.
(globalThis as any).window = { location: { protocol: 'http:', host: 'localhost', pathname: '/wotlk/tank_deathknight/' } };
globalThis.fetch = (async (input: RequestInfo | URL) => {
	const url = String(input);
	assert(url.startsWith('/wotlk/assets/'), `unexpected fetch ${url}`);
	return new Response(readFileSync(path.join(repoRoot, url.slice('/wotlk/'.length))));
}) as typeof fetch;

const { buildOptimizeRequest, defaultTabSettings, plainBossTarget, TANK_BOSSES } = await import('./pool_builder.js');
const { loadCatalog } = await import('./catalog.js');
const { Database } = await import('../proto_utils/database.js');
const { canEquipEnchant, canEquipItem, withSpecProto } = await import('../proto_utils/utils.js');
const Presets = await import('../../tank_deathknight/presets.js');

const db = await Database.get();
const catalog = await loadCatalog();

function filters(): DatabaseFilters {
	return DatabaseFilters.create({ oneHandedWeapons: true, twoHandedWeapons: true });
}

// The raid buffs in the request the optimizer tab builds for the tank page's blood death knight, or
// the same character as the DPS spec, alone in party 1, with Bloodlust on and Strength of Wrynn as
// the page had it.
function requestBuffs(spec: Spec, contentPhase: number, pageBuff: boolean): RaidBuffs {
	const gear = db.lookupEquipmentSpec(Presets.P4_BIS_BLOOD_PRESET.gear);
	const player = withSpecProto(
		spec,
		PlayerProto.create({
			name: 'Player',
			class: Class.ClassDeathknight,
			race: Race.RaceNightElf,
			equipment: gear.asSpec(),
			database: gear.toDatabase(),
			talentsString: Presets.BloodTalents.data.talentsString,
			glyphs: Presets.BloodTalents.data.glyphs,
			rotation: Presets.BLOOD_IT_SPAM_ROTATION_PRESET_DEFAULT.rotation.rotation,
			consumes: Presets.DefaultConsumes,
		}),
		spec == Spec.SpecTankDeathknight ? Presets.DefaultOptions : DeathknightOptions.create(),
	);
	const emptyParty = () => Party.create({ players: [0, 1, 2, 3, 4].map(() => PlayerProto.create()), buffs: PartyBuffs.create() });
	const parties = [0, 1, 2, 3, 4].map(() => emptyParty());
	parties[0].players[0] = player;
	const base = RaidSimRequest.create({
		raid: Raid.create({
			parties,
			buffs: RaidBuffs.create({ bloodlust: true, strengthOfWrynn: pageBuff }),
			debuffs: Debuffs.create(),
			numActiveParties: 5,
		}),
		encounter: Encounter.create({ duration: 180, targets: [plainBossTarget()] }),
		simOptions: SimOptions.create({ iterations: 100 }),
	});
	const { request } = buildOptimizeRequest({
		base,
		targetRaidIndex: 0,
		settings: defaultTabSettings(contentPhase),
		catalog,
		filters: filters(),
		getItems: (slot: ItemSlot) => db.getItems(slot).filter((item: UIItem) => canEquipItem(item, spec, slot)),
		getEnchants: (slot: ItemSlot) => db.getEnchants(slot).filter((enchant: UIEnchant) => canEquipEnchant(enchant, spec)),
		db,
	});
	return request.base!.raid!.buffs!;
}

function requestBuff(spec: Spec, contentPhase: number, pageBuff: boolean): boolean {
	const buffs = requestBuffs(spec, contentPhase, pageBuff);
	assert.strictEqual(buffs.bloodlust, true, `the page's other raid buffs stay, spec ${spec} phase ${contentPhase}`);
	return buffs.strengthOfWrynn;
}

assert.deepStrictEqual(
	Object.entries(TANK_BOSSES)
		.filter(([, boss]) => boss.icecrownBuff)
		.map(([phase]) => Number(phase)),
	[4],
	'only the Lich King fights inside Icecrown',
);

for (const pageBuff of [false, true]) {
	assert.strictEqual(requestBuff(Spec.SpecTankDeathknight, 4, pageBuff), true, `tank P4, page buff ${pageBuff}`);
	assert.strictEqual(requestBuff(Spec.SpecTankDeathknight, 5, pageBuff), false, `tank P5, page buff ${pageBuff}`);
	assert.strictEqual(requestBuff(Spec.SpecTankDeathknight, 3, pageBuff), false, `tank P3, page buff ${pageBuff}`);
	assert.strictEqual(requestBuff(Spec.SpecDeathknight, 4, pageBuff), pageBuff, `DPS P4 keeps the page's buff ${pageBuff}`);
}
console.log('pool_builder_test: ok');
