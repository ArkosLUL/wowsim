const TANK_HEALTH_PHASE_1 = 40000;
const RAID_HEALTH_PHASE_1 = 20000;
const TANK_HEALTH_PER_PHASE = 5000;
const RAID_HEALTH_PER_PHASE = 2500;

// Max health of the tank dummy and of each raid dummy in a healer sim at a content phase (1 to 5).
export function dummyHealthForPhase(phase: number): { tank: number; raid: number } {
	const later = Math.max(phase, 1) - 1;
	return {
		tank: TANK_HEALTH_PHASE_1 + TANK_HEALTH_PER_PHASE * later,
		raid: RAID_HEALTH_PHASE_1 + RAID_HEALTH_PER_PHASE * later,
	};
}

// A healer page's default incoming damage per variant (a priest's tree, 'default' for the other pages) and
// phase: the page's incoming_damage.json.
export type IncomingDamageDefaults = Record<string, Record<string, number>>;

export function defaultIncomingDamage(defaults: IncomingDamageDefaults, variant: string, phase: number): number {
	return defaults[variant]?.[String(phase)] ?? 0;
}
