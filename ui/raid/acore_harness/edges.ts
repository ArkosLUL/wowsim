import { Class, Spec } from '../../core/proto/common';
import { specNames } from '../../core/proto_utils/utils';
import { tankReferences } from '../acore_importer';
import { activeParties, assignRaidIndexes, inferSpec, parseRoster, RosterCharacter } from '../acore_roster';

let failures = 0;
function check(what: string, got: string, want: string) {
	console.log(got == want ? `ok   ${what}: ${got}` : `FAIL ${what}: ${got}, expected ${want}`);
	if (got != want) {
		failures++;
	}
}

const char = (name: string, subgroup: number): RosterCharacter =>
	({ name, classId: 1, raceId: 1, swapRaceId: 0, level: 80, subgroup, memberFlags: 0, talents: '', professions: [], gear: [] } as RosterCharacter);

// six in one subgroup: the sixth has nowhere to go
const overfull = [0, 0, 0, 0, 0, 0].map((sub, i) => char(`over${i}`, sub));
check('overfull subgroup indexes', assignRaidIndexes(overfull).join(','), '0,1,2,3,4,-1');

// subgroups past the raid's 8 parties
const wide = [5, 6, 7, 8, -1].map((sub, i) => char(`wide${i}`, sub));
check('wide subgroup indexes', assignRaidIndexes(wide).join(','), '25,30,35,-1,-1');
check('activeParties(subgroup 7)', `${activeParties([char('a', 7)])}`, '8');
check('activeParties(subgroup 0)', `${activeParties([char('a', 0)])}`, '5');
check('activeParties(subgroup 9)', `${activeParties([char('a', 9)])}`, '8');

// The talents decide tank versus DPS on their own: the group's main-tank flag isn't an input, since
// raiders here carry it while playing a DPS build.
const spec = (playerClass: Class, talents: string) => specNames[inferSpec(playerClass, talents)];
check('cat druid flagged as main tank', spec(Class.ClassDruid, '-553202032322010053120030310511-203503012'), 'Feral DPS Druid');
check('bear druid', spec(Class.ClassDruid, '-503232132322010353120300313511-20350001'), 'Feral Tank Druid');
check('unflagged blood tank death knight', spec(Class.ClassDeathknight, '005512153330030320102013-3050505000023-005'), 'Tank Death Knight');
check('blood dps death knight', spec(Class.ClassDeathknight, '2305120530003303231023001351--2302003050032'), 'DPS Death Knight');
check('fury warrior flagged as main tank', spec(Class.ClassWarrior, '32002300233-305053000500310153120511351'), 'DPS Warrior');
check('protection paladin', spec(Class.ClassPaladin, '-05005135203102311333312321-502302012003'), 'Protection Paladin');
check('mage, a class with no tank spec', spec(Class.ClassMage, '23000512310035015032310250532-03-023203'), 'Mage');

// The flag's remaining job: it orders the tank slots, and never puts a DPS build in one, since the
// boss swings at whoever holds the first.
const slots = (raiders: Array<[Spec, number, boolean]>) =>
	tankReferences(raiders.map(([spec, raidIndex, isMainTank]) => ({ spec, raidIndex, isMainTank })))
		.map(ref => ref.index)
		.join(',');
const tank = (index: number, flagged = false): [Spec, number, boolean] => [Spec.SpecProtectionWarrior, index, flagged];
check('the flagged tank leads', slots([tank(3), tank(7, true)]), '7,3');
check('a flagged cat druid takes no tank slot', slots([[Spec.SpecFeralDruid, 2, true], tank(7)]), '7');
check('a raid whose only flag is a DPS build has no tanks', slots([[Spec.SpecFeralDruid, 2, true]]), '');
check('tank slots stop at MAX_TANKS', slots([tank(1), tank(2), tank(3), tank(4), tank(5)]), '1,2,3,4');

// version and shape guards
const bad = ['', 'not json', '{}', '{"version":3,"characters":[]}', '{"version":1,"characters":[]}', '[]'];
bad.forEach(data => {
	const label = `parseRoster(${JSON.stringify(data).slice(0, 34)})`;
	try {
		parseRoster(data);
		console.log(`FAIL ${label}: accepted, expected a rejection`);
		failures++;
	} catch (e: any) {
		console.log(`ok   ${label}: ${e.message}`);
	}
});

if (failures > 0) {
	console.log(`${failures} checks failed`);
	process.exit(1);
}
