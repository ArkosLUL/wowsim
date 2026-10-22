"""Writes the Caps & Priority tab's shipped stat values, assets/statweights/stat_values.json, from the
merged --out/weights.json.

    python tools/statweights/publish.py --out tmp/statweights
"""
import argparse
import json
import sys
from pathlib import Path

sys.dont_write_bytecode = True
REPO = Path(__file__).resolve().parents[2]
sys.path.insert(0, str(REPO / 'tools' / 'presetgen'))
import gen  # noqa: E402

DEST = REPO / 'assets' / 'statweights' / 'stat_values.json'
PHASES = [1, 2, 3, 4, 5]
SIGNIFICANT_DIGITS = 4

# the tree with the most points in the build's talent preset, as player.getTalentTree() counts it;
# the tab picks a page's build by spec, then this, then the two-hand flag
TREES = {
    'dk_blood': 0, 'dk_frost': 1, 'dk_unholy': 2, 'dk_unholy_2h': 2,
    'hunter_bm': 0, 'hunter_mm': 1, 'hunter_sv': 2,
    'mage_arcane': 0, 'mage_fire': 1, 'mage_frost': 2,
    'rogue_assassination': 0, 'rogue_combat': 1, 'rogue_subtlety': 2,
    'warlock_affliction': 0, 'warlock_demonology': 1, 'warlock_destruction': 2,
    'warrior_arms': 0, 'warrior_fury': 1,
    'tank_dk_blood': 0, 'tank_dk_frost': 1,
    'balance_druid': 0, 'feral_druid': 1, 'feral_tank_druid': 1,
    'elemental_shaman': 0, 'enhancement_shaman': 1,
    'protection_paladin': 1, 'retribution_paladin': 2,
    'protection_warrior': 2,
    'shadow_priest': 2, 'smite_priest': 0,
}
# builds that share their spec and tree with a dual wield build
TWO_HAND = {'dk_unholy_2h'}


def rounded(value):
    value = float(f'{value:.{SIGNIFICANT_DIGITS}g}')
    return int(value) if value.is_integer() else value


def main():
    parser = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    parser.add_argument('--out', required=True, help='work directory: reads weights.json')
    parser.add_argument('--dest', default=str(DEST), help='file to write (default assets/statweights/stat_values.json)')
    args = parser.parse_args()

    weights = json.loads((Path(args.out) / 'weights.json').read_text(encoding='utf-8'))
    measured = {b['key']: b for b in weights['builds']}
    problems = [f'{b.key}: no tree in TREES' for b in gen.BUILDS if b.key not in TREES]
    problems += [f'{key}: not in presetgen\'s BUILDS' for key in measured if key not in gen.BUILDS_BY_KEY]
    for b in gen.BUILDS:
        phases = sorted(p['phase'] for p in measured.get(b.key, {}).get('phases', []))
        if phases != PHASES:
            problems.append(f'{b.key}: weights.json has phases {phases}, the tab needs {PHASES}')
    if problems:
        raise SystemExit('can\'t publish:\n' + '\n'.join(problems))

    builds = []
    for b in gen.BUILDS:
        row = measured[b.key]
        builds.append({
            'key': b.key,
            'name': b.label,
            'class': row['class'],
            'spec': row['spec'],
            'tree': TREES[b.key],
            'two_hand': b.key in TWO_HAND,
            'phases': [{'phase': p['phase'],
                        'slopes': {stat: {metric: rounded(s[metric]) for metric in ('dps', 'tps', 'tmi')}
                                   for stat, s in sorted(p['slopes'].items())}}
                       for p in sorted(row['phases'], key=lambda p: p['phase'])],
        })
    values = {'sim_commit': weights['sim_commit'], 'generated_at': weights['generated_at'], 'builds': builds}
    dest = Path(args.dest)
    dest.parent.mkdir(parents=True, exist_ok=True)
    tmp = dest.with_suffix('.json.tmp')
    tmp.write_text(json.dumps(values, indent=1) + '\n', encoding='utf-8', newline='\n')
    tmp.replace(dest)
    print(f'{dest}: {len(builds)} builds, sim {values["sim_commit"]}, {dest.stat().st_size // 1024} KB')


if __name__ == '__main__':
    main()
