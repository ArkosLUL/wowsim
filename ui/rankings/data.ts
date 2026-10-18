import pako from 'pako';

import { REPO_NAME } from '../core/constants/other';
import { Class, Spec } from '../core/proto/common';
import { IndividualSimSettings } from '../core/proto/ui';

export const RANKINGS_URL = '/wotlk/assets/rankings/rankings.json';

// Every IndividualLinkImporter category letter, UI settings ('u') included: without it the sim
// page ignores the link's content phase and iterations.
const LINK_CATEGORIES = 'gtrcmxeu';

export interface FightStats {
	dps: number;
	stdev: number;
	min: number;
	max: number;
}

export interface BuildPhase {
	phase: number;
	// By fight length in seconds.
	fights: Record<number, FightStats>;
	link: string;
}

export interface RankedBuild {
	key: string;
	name: string;
	class: Class;
	spec: Spec;
	dir: string;
	talents: string;
	phases: Array<BuildPhase>;
}

export interface Rankings {
	simCommit: string;
	generatedAt: string;
	catalogDate: string;
	iterations: number;
	seed: number;
	fights: Array<number>;
	builds: Array<RankedBuild>;
}

function parseRankings(json: any): Rankings {
	return {
		simCommit: String(json.sim_commit ?? ''),
		generatedAt: String(json.generated_at ?? ''),
		catalogDate: String(json.catalog_date ?? ''),
		iterations: Number(json.iterations ?? 0),
		seed: Number(json.seed ?? 0),
		fights: (json.fights ?? []).map(Number),
		builds: (json.builds ?? []).map(
			(b: any): RankedBuild => ({
				key: b.key,
				name: b.name,
				class: Class[b.class as keyof typeof Class] ?? Class.ClassUnknown,
				spec: Spec[b.spec as keyof typeof Spec],
				dir: b.dir,
				talents: b.talents ?? '',
				phases: (b.phases ?? []).map(
					(p: any): BuildPhase => ({
						phase: Number(p.phase),
						fights: Object.fromEntries(Object.entries(p.fights ?? {}).map(([fight, stats]) => [Number(fight), stats as FightStats])),
						link: p.link,
					}),
				),
			}),
		),
	};
}

let loadPromise: Promise<Rankings> | null = null;

export function loadRankings(): Promise<Rankings> {
	if (!loadPromise) {
		loadPromise = fetch(RANKINGS_URL)
			.then(response => {
				if (!response.ok) {
					throw new Error(`Couldn't load the rankings (${RANKINGS_URL}): HTTP ${response.status}`);
				}
				return response.json();
			})
			.then(parseRankings);
		loadPromise.catch(() => (loadPromise = null));
	}
	return loadPromise;
}

export function contentPhases(rankings: Rankings): Array<number> {
	const phases = new Set<number>();
	rankings.builds.forEach(build => build.phases.forEach(p => phases.add(p.phase)));
	return [...phases].sort((a, b) => a - b);
}

export function buildPhase(build: RankedBuild, phase: number): BuildPhase | undefined {
	return build.phases.find(p => p.phase == phase);
}

function fromBase64(text: string): Uint8Array {
	const binary = atob(text);
	const bytes = new Uint8Array(binary.length);
	for (let i = 0; i < binary.length; i++) {
		bytes[i] = binary.charCodeAt(i);
	}
	return bytes;
}

function toBase64(bytes: Uint8Array): string {
	let binary = '';
	for (let i = 0; i < bytes.length; i++) {
		binary += String.fromCharCode(bytes[i]);
	}
	return btoa(binary);
}

const linkCache = new Map<string, string>();

// The spec's sim page with the simmed setup at this fight length, as a share link.
export function simLink(build: RankedBuild, phase: BuildPhase, fightSeconds: number): string {
	const cacheKey = `${build.key}|${phase.phase}|${fightSeconds}`;
	let href = linkCache.get(cacheKey);
	if (href === undefined) {
		const settings = IndividualSimSettings.fromBinary(pako.inflate(fromBase64(phase.link)));
		if (settings.encounter) {
			settings.encounter.duration = fightSeconds;
		}
		const encoded = toBase64(pako.deflate(IndividualSimSettings.toBinary(settings)));
		href = `/${REPO_NAME}/${build.dir}/?i=${LINK_CATEGORIES}#${encoded}`;
		linkCache.set(cacheKey, href);
	}
	return href;
}
