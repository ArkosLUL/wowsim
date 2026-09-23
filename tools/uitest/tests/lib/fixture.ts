import { readFileSync } from 'node:fs';
import path from 'node:path';
import { gunzipSync } from 'node:zlib';

import type { Page } from '@playwright/test';

const FIXTURE_DIR = path.join(__dirname, '..', '..', 'fixtures');

export interface FixturePlayer {
	name: string;
	// The raid slot, which is what the "(#N)" in a unit's label counts.
	raidIndex: number;
	// The sim's own numbering, handed out over targets first and then the raid.
	unitIndex: number;
	dps: number;
}

export interface Fixture {
	// The SimRun as protojson, ready to post at the page.
	run: any;
	players: FixturePlayer[];
	targets: { name: string; unitIndex: number }[];
}

export function loadFixture(name = 'raid25'): Fixture {
	const raw = gunzipSync(readFileSync(path.join(FIXTURE_DIR, `${name}.simrun.json.gz`)));
	const run = JSON.parse(raw.toString('utf-8'));

	// PartyMetrics pairs request players with metrics players by position, skipping empty slots, so
	// the expectations have to be derived the same way.
	const players: FixturePlayer[] = [];
	const requestParties = run.request?.raid?.parties ?? [];
	const metricParties = run.result?.raidMetrics?.parties ?? [];
	for (let party = 0; party < Math.min(requestParties.length, metricParties.length); party++) {
		const requestPlayers = requestParties[party].players ?? [];
		const metricPlayers = metricParties[party].players ?? [];
		for (let slot = 0; slot < Math.min(requestPlayers.length, metricPlayers.length); slot++) {
			const spec = requestPlayers[slot]?.class;
			if (!spec || spec === 'ClassUnknown') continue;
			const metrics = metricPlayers[slot];
			players.push({
				name: metrics.name,
				raidIndex: party * 5 + slot,
				unitIndex: metrics.unitIndex ?? 0,
				dps: metrics.dps?.avg ?? 0,
			});
		}
	}

	const targets = (run.result?.encounterMetrics?.targets ?? []).map((target: any) => ({
		name: target.name,
		unitIndex: target.unitIndex ?? 0,
	}));

	return { run, players, targets };
}

// The healing, threat and experimental tabs stay hidden until settings turn them on, so the page
// only shows everything once it has had a settings message as well as a run.
const ALL_METRICS = {
	showDamageMetrics: true,
	showThreatMetrics: true,
	showHealingMetrics: true,
	showExperimental: true,
};

// Opens the standalone results page and hands it the fixture the way the sim page does.
export async function showFixture(page: Page, fixture: Fixture, settings: Record<string, boolean> = ALL_METRICS) {
	await page.goto('/wotlk/detailed_results/index.html');
	await page.evaluate(s => window.postMessage({ settings: s }, '*'), settings);
	await page.evaluate(run => window.postMessage({ runData: { run } }, '*'), fixture.run);
	await page.locator('.dr-root:not(.dr-no-results)').waitFor();
}

export const label = (player: FixturePlayer) => `${player.name} (#${player.raidIndex + 1})`;
