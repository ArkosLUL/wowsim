import { StatSlopeMetrics, StatSlopesResult } from './proto/api.js';
import { Class, PseudoStat, Spec, Stat } from './proto/common.js';

export const STAT_VALUES_URL = '/wotlk/assets/statweights/stat_values.json';

// One stat's value per point. tmi is TMI's change, so lower is better. The shipped file has no hps.
export interface Slope {
	dps: number;
	tps: number;
	tmi: number;
	hps: number;
}

export interface SlopeSet {
	stats: Map<Stat, Slope>;
	pseudoStats: Map<PseudoStat, Slope>;
}

// measured at the phase's BiS gear
export interface ShippedPhase extends SlopeSet {
	phase: number;
}

// Measured at the player's own gear, one-sided: gain is what one more point adds, loss what one
// fewer costs. A stat the player has none of has no loss.
export interface LiveValues {
	gain: SlopeSet;
	loss: SlopeSet;
}

function slopeFrom(metrics: StatSlopeMetrics): Slope {
	return { dps: metrics.dps, tps: metrics.tps, tmi: metrics.tmi, hps: metrics.hps };
}

export function liveValues(result: StatSlopesResult): LiveValues {
	const gain: SlopeSet = { stats: new Map(), pseudoStats: new Map() };
	const loss: SlopeSet = { stats: new Map(), pseudoStats: new Map() };
	const put = (set: SlopeSet, pseudo: boolean, stat: Stat, pseudoStat: PseudoStat, slope: Slope) =>
		pseudo ? set.pseudoStats.set(pseudoStat, slope) : set.stats.set(stat, slope);
	for (const s of result.slopes) {
		if (s.gain) {
			put(gain, s.pseudo, s.stat, s.pseudoStat, slopeFrom(s.gain));
		}
		if (s.loss && s.lowDelta > 0) {
			put(loss, s.pseudo, s.stat, s.pseudoStat, slopeFrom(s.loss));
		}
	}
	return { gain, loss };
}

export interface ShippedBuild {
	key: string;
	name: string;
	class: Class;
	spec: Spec;
	// the talent tree with the most points, as Player.getTalentTree() counts it
	tree: number;
	twoHand: boolean;
	phases: Array<ShippedPhase>;
}

export interface StatValues {
	simCommit: string;
	generatedAt: string;
	builds: Array<ShippedBuild>;
}

function parseSlope(json: any): Slope {
	return { dps: Number(json?.dps ?? 0), tps: Number(json?.tps ?? 0), tmi: Number(json?.tmi ?? 0), hps: Number(json?.hps ?? 0) };
}

function parsePhase(json: any): ShippedPhase {
	const phase: ShippedPhase = { phase: Number(json.phase), stats: new Map(), pseudoStats: new Map() };
	for (const [name, slope] of Object.entries(json.slopes ?? {})) {
		const stat = Stat[name as keyof typeof Stat];
		const pseudoStat = PseudoStat[name as keyof typeof PseudoStat];
		if (stat !== undefined) {
			phase.stats.set(stat, parseSlope(slope));
		} else if (pseudoStat !== undefined) {
			phase.pseudoStats.set(pseudoStat, parseSlope(slope));
		}
	}
	return phase;
}

export function parseStatValues(json: any): StatValues {
	return {
		simCommit: String(json.sim_commit ?? ''),
		generatedAt: String(json.generated_at ?? ''),
		builds: (json.builds ?? [])
			.map(
				(b: any): ShippedBuild => ({
					key: String(b.key),
					name: String(b.name ?? b.key),
					class: Class[b.class as keyof typeof Class] ?? Class.ClassUnknown,
					spec: Spec[b.spec as keyof typeof Spec],
					tree: Number(b.tree ?? 0),
					twoHand: !!b.two_hand,
					phases: (b.phases ?? []).map(parsePhase),
				}),
			)
			.filter((b: ShippedBuild) => b.spec !== undefined),
	};
}

let loadPromise: Promise<StatValues> | null = null;

// Fetches the file once per page. A failed fetch isn't cached, so a retry fetches again.
export function loadStatValues(): Promise<StatValues> {
	if (!loadPromise) {
		loadPromise = fetch(STAT_VALUES_URL)
			.then(response => {
				if (!response.ok) {
					throw new Error(`Couldn't load the shipped stat values (${STAT_VALUES_URL}): HTTP ${response.status}`);
				}
				return response.json();
			})
			.then(parseStatValues);
		loadPromise.catch(() => (loadPromise = null));
	}
	return loadPromise;
}
