import { Spec } from '../core/proto/common';
import type { FightStats, RankedBuild } from './data';

export enum Role {
	Melee = 'melee',
	Ranged = 'ranged',
	Caster = 'caster',
}

export const ROLES: Array<Role> = [Role.Melee, Role.Ranged, Role.Caster];

export const ROLE_LABELS: Record<Role, string> = {
	[Role.Melee]: 'Melee',
	[Role.Ranged]: 'Ranged physical',
	[Role.Caster]: 'Caster',
};

const SPEC_ROLES: Partial<Record<Spec, Role>> = {
	[Spec.SpecDeathknight]: Role.Melee,
	[Spec.SpecWarrior]: Role.Melee,
	[Spec.SpecRogue]: Role.Melee,
	[Spec.SpecRetributionPaladin]: Role.Melee,
	[Spec.SpecEnhancementShaman]: Role.Melee,
	[Spec.SpecFeralDruid]: Role.Melee,
	[Spec.SpecHunter]: Role.Ranged,
	[Spec.SpecMage]: Role.Caster,
	[Spec.SpecWarlock]: Role.Caster,
	[Spec.SpecShadowPriest]: Role.Caster,
	[Spec.SpecSmitePriest]: Role.Caster,
	[Spec.SpecBalanceDruid]: Role.Caster,
	[Spec.SpecElementalShaman]: Role.Caster,
};

export function roleOf(spec: Spec): Role | undefined {
	return SPEC_ROLES[spec];
}

export type TierLetter = 'S' | 'A' | 'B' | 'C' | 'D';

// Lower bound of each tier, in % of the top build.
export const TIERS: Array<{ letter: TierLetter; min: number }> = [
	{ letter: 'S', min: 95 },
	{ letter: 'A', min: 90 },
	{ letter: 'B', min: 85 },
	{ letter: 'C', min: 75 },
	{ letter: 'D', min: 0 },
];

// To one decimal, as shown, so a build at 94.96% shows 95.0% and is S, not A.
export function percentOfTop(dps: number, topDps: number): number {
	return topDps > 0 ? Math.round((1000 * dps) / topDps) / 10 : 0;
}

export function tierFor(percent: number): TierLetter {
	return (TIERS.find(t => percent >= t.min) ?? TIERS[TIERS.length - 1]).letter;
}

export interface RankedRow {
	build: RankedBuild;
	stats: FightStats;
	// 1 for the top build.
	rank: number;
	percent: number;
	tier: TierLetter;
}

// Every build with a result for this content phase and fight, best first. Percent, rank and tier are
// against all of them, so filtering the rows afterwards changes none of them.
export function rankBuilds(builds: Array<RankedBuild>, phase: number, fight: number): Array<RankedRow> {
	const rows = builds
		.map(build => ({ build, stats: build.phases.find(p => p.phase == phase)?.fights[fight] }))
		.filter((row): row is { build: RankedBuild; stats: FightStats } => row.stats !== undefined)
		.sort((a, b) => b.stats.dps - a.stats.dps || a.build.name.localeCompare(b.build.name));
	const top = rows.length ? rows[0].stats.dps : 0;
	return rows.map((row, i) => {
		const percent = percentOfTop(row.stats.dps, top);
		return { ...row, rank: i + 1, percent, tier: tierFor(percent) };
	});
}
