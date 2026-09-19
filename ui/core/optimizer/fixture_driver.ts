// Node script, bundled with esbuild (see README.md). It writes the replay fixtures sim/optimizer's
// TestReplayFixtures checks, straight from the tab's pool builder, and posts a request to a running
// sim server the way net_worker.js does.
import { mkdirSync, readFileSync, writeFileSync } from 'fs';
import * as path from 'path';
import { gunzipSync, gzipSync } from 'zlib';

import { AsyncAPIResult, OptimizeGearRequest, Party, Player as PlayerProto, ProgressMetrics, Raid, RaidSimRequest, SimOptions } from '../proto/api.js';
import {
	ArmorType,
	Class,
	Debuffs,
	Encounter,
	EquipmentSpec,
	IndividualBuffs,
	ItemSlot,
	PartyBuffs,
	Profession,
	Race,
	RaidBuffs,
	RangedWeaponType,
	Spec,
	Stat,
	TristateEffect,
	WeaponType,
} from '../proto/common.js';
import { OptimizerResult, StatMinimum } from '../proto/optimizer.js';
import { DatabaseFilters, RaidFilterOption, SourceFilterOption, UIEnchant, UIItem } from '../proto/ui.js';
import type { Database as DatabaseType } from '../proto_utils/database.js';
import { getEnumValues } from '../utils.js';
import type { OptimizerTabSettings } from './pool_builder.js';

const repoRoot = process.cwd();

// The UI modules read window.location when they load, and Database and the catalog fetch
// /wotlk/assets/..., so both need stand-ins first. Hence the dynamic imports.
(globalThis as any).window = { location: { protocol: 'http:', host: 'localhost', pathname: '/wotlk/retribution_paladin/' } };
const nodeFetch = globalThis.fetch;
globalThis.fetch = (async (input: RequestInfo | URL, init?: RequestInit) => {
	const url = String(input);
	if (url.startsWith('/wotlk/assets/')) {
		return new Response(readFileSync(path.join(repoRoot, url.slice('/wotlk/'.length))));
	}
	return nodeFetch(input, init);
}) as typeof fetch;

const { buildOptimizeRequest, defaultTabSettings, plainBossTarget } = await import('./pool_builder.js');
const { loadCatalog } = await import('./catalog.js');
const { Database } = await import('../proto_utils/database.js');
const { canEquipEnchant, canEquipItem, withSpecProto } = await import('../proto_utils/utils.js');
const Presets = await import('../../retribution_paladin/presets.js');

const SPEC = Spec.SpecRetributionPaladin;
const PROFESSIONS = [Profession.Blacksmithing, Profession.Jewelcrafting];

// The Retribution sim's defaults (ui/retribution_paladin/sim.ts), which can't load under Node.
const RAID_BUFFS = RaidBuffs.create({
	arcaneBrilliance: true,
	divineSpirit: true,
	giftOfTheWild: TristateEffect.TristateEffectImproved,
	bloodlust: true,
	manaSpringTotem: TristateEffect.TristateEffectRegular,
	hornOfWinter: true,
	battleShout: TristateEffect.TristateEffectImproved,
	sanctifiedRetribution: true,
	swiftRetribution: true,
	elementalOath: true,
	rampage: true,
	trueshotAura: true,
	icyTalons: true,
	totemOfWrath: true,
	wrathOfAirTotem: true,
	demonicPactSp: 500,
});
const INDIVIDUAL_BUFFS = IndividualBuffs.create({
	judgementsOfTheWise: true,
	blessingOfKings: true,
	blessingOfMight: TristateEffect.TristateEffectImproved,
});
const DEBUFFS = Debuffs.create({
	shadowMastery: true,
	totemOfWrath: true,
	judgementOfWisdom: true,
	judgementOfLight: true,
	misery: true,
	curseOfElements: true,
	bloodFrenzy: true,
	exposeArmor: true,
	sunderArmor: true,
	faerieFire: TristateEffect.TristateEffectImproved,
	curseOfWeakness: TristateEffect.TristateEffectRegular,
});

// Sim.fromProto fills empty filter lists with every value, so this is what the gear picker starts with.
function allFilters(): DatabaseFilters {
	const all = <E>(e: any) => (getEnumValues(e) as Array<E>).filter(v => (v as unknown as number) != 0);
	return DatabaseFilters.create({
		armorTypes: all<ArmorType>(ArmorType),
		weaponTypes: all<WeaponType>(WeaponType),
		rangedWeaponTypes: all<RangedWeaponType>(RangedWeaponType),
		sources: all<SourceFilterOption>(SourceFilterOption),
		raids: all<RaidFilterOption>(RaidFilterOption),
		oneHandedWeapons: true,
		twoHandedWeapons: true,
	});
}

// What Sim.makeRaidSimRequest gives for a Retribution Paladin wearing gear: the gear looked up and
// back, an inactive meta dropped, alone in party 1.
function raidSimRequest(db: DatabaseType, gear: EquipmentSpec): RaidSimRequest {
	let equipped = db.lookupEquipmentSpec(gear);
	if (equipped.hasInactiveMetaGem(PROFESSIONS.includes(Profession.Blacksmithing))) {
		equipped = equipped.withoutMetaGem();
	}
	const player = withSpecProto(
		SPEC,
		PlayerProto.create({
			name: 'Player',
			class: Class.ClassPaladin,
			race: Race.RaceHuman,
			equipment: equipped.asSpec(),
			database: equipped.toDatabase(),
			talentsString: Presets.AuraMasteryTalents.data.talentsString,
			glyphs: Presets.AuraMasteryTalents.data.glyphs,
			rotation: Presets.ROTATION_PRESET_DEFAULT.rotation.rotation,
			consumes: Presets.DefaultConsumes,
			buffs: INDIVIDUAL_BUFFS,
			professions: PROFESSIONS,
			profession1: PROFESSIONS[0],
			profession2: PROFESSIONS[1],
			reactionTimeMs: 200,
		}),
		Presets.DefaultOptions,
	);
	const emptyParty = () => Party.create({ players: [0, 1, 2, 3, 4].map(() => PlayerProto.create()), buffs: PartyBuffs.create() });
	const parties = [0, 1, 2, 3, 4].map(() => emptyParty());
	parties[0].players[0] = player;
	return RaidSimRequest.create({
		raid: Raid.create({ parties, buffs: RAID_BUFFS, debuffs: DEBUFFS, numActiveParties: 5 }),
		encounter: Encounter.create({
			duration: 180,
			durationVariation: 5,
			executeProportion20: 0.2,
			executeProportion25: 0.25,
			executeProportion35: 0.35,
			targets: [plainBossTarget()],
		}),
		simOptions: SimOptions.create({ iterations: 3000, randomSeed: BigInt(101), debugFirstIteration: true }),
	});
}

interface Fixture {
	name: string;
	gear: EquipmentSpec;
	contentPhase: number;
	change?: (settings: OptimizerTabSettings, gear: EquipmentSpec) => void;
}

const FIXTURES: Array<Fixture> = [
	{ name: 'ret_p1', gear: Presets.P1_PRESET.gear, contentPhase: 1 },
	// most of the P2 gear isn't out yet, so the seed loses it and its second trinket moves up
	{ name: 'ret_p1_from_p2', gear: Presets.P2_PRESET.gear, contentPhase: 1 },
	{ name: 'ret_p2', gear: Presets.P2_PRESET.gear, contentPhase: 2 },
	{ name: 'ret_p3', gear: Presets.P3_PRESET.gear, contentPhase: 3 },
	{
		name: 'ret_p4',
		gear: Presets.P4_PRESET.gear,
		contentPhase: 4,
		change: (settings, gear) => {
			settings.lockedSlots = [ItemSlot.ItemSlotTrinket1, ItemSlot.ItemSlotTrinket2];
			settings.excludedItemIds = [gear.items[ItemSlot.ItemSlotHead].id];
			settings.statMinimums = [StatMinimum.create({ stat: Stat.StatMeleeHit, minValue: 263 })];
		},
	},
	{ name: 'ret_p5', gear: Presets.P5_PRESET.gear, contentPhase: 5 },
];

async function writeFixtures(outDir: string) {
	const db = await Database.get();
	const catalog = await loadCatalog();
	mkdirSync(outDir, { recursive: true });
	for (const fixture of FIXTURES) {
		const settings = defaultTabSettings(fixture.contentPhase);
		fixture.change?.(settings, fixture.gear);
		const { request, seedChanges } = buildOptimizeRequest({
			base: raidSimRequest(db, fixture.gear),
			targetRaidIndex: 0,
			settings,
			catalog,
			filters: allFilters(),
			getItems: (slot: ItemSlot) => db.getItems(slot).filter((item: UIItem) => canEquipItem(item, SPEC, slot)),
			getEnchants: (slot: ItemSlot) => db.getEnchants(slot).filter((enchant: UIEnchant) => canEquipEnchant(enchant, SPEC)),
			db,
		});
		const file = path.join(outDir, `${fixture.name}.json.gz`);
		const gz = gzipSync(OptimizeGearRequest.toJsonString(request) + '\n', { level: 9 });
		// header's OS byte depends on the platform's zlib build; pin it so reruns match byte for byte
		gz[9] = 255;
		writeFileSync(file, gz);
		const pool = request.pool!;
		console.log(
			`${file}: ${pool.slots.reduce((n, s) => n + s.itemIds.length, 0)} slot candidates, ${pool.gemIds.length} gems, ` +
				`${pool.database!.items.length} items in the database`,
		);
		seedChanges.forEach(change => console.log(`  seed: ${change}`));
	}
}

// Same calls as net_worker.js: start, then poll every 500 ms until the final result.
async function postFixture(server: string, file: string) {
	const data = readFileSync(file);
	const request = OptimizeGearRequest.fromJsonString((file.endsWith('.gz') ? gunzipSync(data) : data).toString('utf8'));
	const post = (route: string, body: Uint8Array) =>
		nodeFetch(server + route, { method: 'POST', headers: { 'Content-Type': 'application/x-protobuf' }, body });
	const started = await post('/optimizeGearAsync', OptimizeGearRequest.toBinary(request));
	const handle = new Uint8Array(await started.arrayBuffer());
	console.log(`/optimizeGearAsync: HTTP ${started.status}, progress id ${AsyncAPIResult.fromBinary(handle).progressId}`);
	for (;;) {
		const polled = await post('/asyncProgress', handle);
		if (polled.status == 204) {
			throw new Error('/asyncProgress: the run is gone without a final result');
		}
		const progress = ProgressMetrics.fromBinary(new Uint8Array(await polled.arrayBuffer()));
		if (progress.optimizerProgress) {
			console.log(`progress: ${JSON.stringify(progress.optimizerProgress)}`);
		}
		const result = progress.finalOptimizeResult;
		if (result) {
			console.log(OptimizerResult.toJsonString(result, { prettySpaces: 2 }));
			if (result.errorResult) {
				process.exitCode = 1;
			}
			return;
		}
		await new Promise(resolve => setTimeout(resolve, 500));
	}
}

const [command, ...args] = process.argv.slice(2);
if (command == 'write' && args.length == 1) {
	await writeFixtures(args[0]);
} else if (command == 'post' && args.length == 2) {
	await postFixture(args[0].replace(/\/$/, ''), args[1]);
} else {
	console.error('usage: fixture_driver.mjs write <out dir> | post <server url> <fixture.json[.gz]>');
	process.exitCode = 2;
}
