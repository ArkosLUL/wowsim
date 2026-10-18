export const CURRENT_PHASE = 5;

export const PHASE_LABELS: Record<number, string> = {
	1: 'P1: Naxxramas, Eye of Eternity, Obsidian Sanctum',
	2: 'P2: Ulduar',
	3: 'P3: Trial of the Crusader',
	4: 'P4: Icecrown Citadel',
	5: 'P5: Ruby Sanctum',
};

// Github pages serves our site under the /wotlk directory (because the repo name is wotlk)
export const REPO_NAME = 'wotlk';

// Get 'elemental_shaman', the pathname part after the repo name
const pathnameParts = window.location.pathname.split('/');
const repoPartIdx = pathnameParts.findIndex(part => part == REPO_NAME);
export const SPEC_DIRECTORY = repoPartIdx == -1 ? '' : pathnameParts[repoPartIdx + 1];
