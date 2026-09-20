import { Class } from '../../core/proto/common';
import { specNames } from '../../core/proto_utils/utils';
import { activeParties, assignRaidIndexes, inferSpec, parseRoster, RosterCharacter } from '../acore_roster';

const char = (name: string, subgroup: number): RosterCharacter =>
	({ name, classId: 1, raceId: 1, swapRaceId: 0, level: 80, subgroup, memberFlags: 0, talents: '', professions: [], gear: [] } as RosterCharacter);

// six in one subgroup: the sixth has nowhere to go
const overfull = [0, 0, 0, 0, 0, 0].map((sub, i) => char(`over${i}`, sub));
console.log('overfull subgroup indexes:', assignRaidIndexes(overfull).join(','), '(expect 0,1,2,3,4,-1)');

// subgroups past the raid's 8 parties
const wide = [5, 6, 7, 8, -1].map((sub, i) => char(`wide${i}`, sub));
console.log('wide subgroup indexes:', assignRaidIndexes(wide).join(','), '(expect 25,30,35,-1,-1)');
console.log('activeParties(subgroup 7):', activeParties([char('a', 7)]), '(expect 8)');
console.log('activeParties(subgroup 0):', activeParties([char('a', 0)]), '(expect 5)');
console.log('activeParties(subgroup 9):', activeParties([char('a', 9)]), '(expect 8)');

// main-tank flag beats the talent string, but only for classes with a tank spec
const prot = inferSpec(Class.ClassWarrior, '32002300233-305053000500310153120511351', 2);
const noTankSpec = inferSpec(Class.ClassMage, '23000512310035015032310250532-03-023203', 2);
console.log('main-tank warrior:', specNames[prot], '(expect Protection Warrior)');
console.log('main-tank mage:', specNames[noTankSpec], '(expect Mage)');

// version and shape guards
const bad = ['', 'not json', '{}', '{"version":2,"characters":[]}', '{"version":1,"characters":[]}', '[]'];
bad.forEach(data => {
	try {
		parseRoster(data);
		console.log(`parseRoster(${JSON.stringify(data).slice(0, 34)}) -> accepted (unexpected)`);
	} catch (e: any) {
		console.log(`parseRoster(${JSON.stringify(data).slice(0, 34)}) -> ${e.message}`);
	}
});
