"""Merges the runner's --out/results/*.json into one --out/weights.json.

    python tools/statweights/merge.py --out tmp/statweights
"""
import argparse
import datetime
import hashlib
import json
import subprocess
import sys
from pathlib import Path

sys.dont_write_bytecode = True
REPO = Path(__file__).resolve().parents[2]
sys.path.insert(0, str(REPO / 'tools' / 'presetgen'))
import gen  # noqa: E402

# what the runner's numbers depend on; other sessions' doc edits don't make a run dirty
SIM_PATHS = ['sim', 'proto', 'assets']


def git(*args):
    return subprocess.run(['git', '-C', str(REPO), *args], capture_output=True, encoding='utf-8',
                          check=True).stdout.strip()


def sim_commit():
    commit = git('rev-parse', 'HEAD')
    if git('status', '--porcelain', '--', *SIM_PATHS):
        commit += '-dirty'
    return commit


def stale(out, key, phase, result):
    """Why a result no longer matches its settings file or the phase's gear preset, else None."""
    settings = out / 'settings' / f'{key}_p{phase}.bin'
    if not settings.exists() or hashlib.sha256(settings.read_bytes()).hexdigest() != result['settings_sha256']:
        return 'settings changed since the run: rerun the runner'
    items = json.loads(gen.gear_path(gen.BUILDS_BY_KEY[key], phase).read_text(encoding='utf-8'))['items']
    ids = [item.get('id', 0) for item in items]
    if ids + [0] * (len(result['items']) - len(ids)) != result['items']:
        return 'gear preset changed since the capture: delete its settings file, recapture and rerun'
    return None


def only(values, what):
    values = set(values)
    if len(values) != 1:
        raise SystemExit(f'results mix {what}: {sorted(values)}; rerun them with one')
    return values.pop()


def main():
    parser = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    parser.add_argument('--out', required=True, help='work directory: reads results/, writes weights.json')
    parser.add_argument('--builds', default='all', help="comma-separated keys from presetgen's BUILDS, or all (default)")
    parser.add_argument('--phases', default='1-5', help='e.g. 1-5 or 1,5 (default 1-5)')
    args = parser.parse_args()

    build_list = gen.parse_list(args.builds, [b.key for b in gen.BUILDS], 'build')
    phases = gen.parse_phases(args.phases)
    out = Path(args.out)
    results, missing = {}, []
    for key in build_list:
        for phase in phases:
            path = out / 'results' / f'{key}_p{phase}.json'
            if path.exists():
                results[key, phase] = json.loads(path.read_text(encoding='utf-8'))
            else:
                missing.append(f'{key} P{phase}')
    if missing:
        raise SystemExit(f'no results for {", ".join(missing)}: run the runner first')
    problems = [f'{key} P{phase}: {why}' for (key, phase), r in results.items() if (why := stale(out, key, phase, r))]
    if problems:
        raise SystemExit('stale results:\n' + '\n'.join(problems))

    builds = []
    for key in build_list:
        rows = [results[key, p] for p in phases]
        builds.append({
            'key': key,
            'class': only((r['class'] for r in rows), f'{key} classes'),
            'spec': only((r['spec'] for r in rows), f'{key} specs'),
            'phases': [{'phase': r['phase'], 'items': r['items'], 'dps': r['dps'], 'tps': r['tps'], 'tmi': r['tmi'],
                        'slopes': dict(sorted(r['slopes'].items()))} for r in rows],
        })
    weights = {
        'sim_commit': sim_commit(),
        'generated_at': datetime.datetime.now(datetime.timezone.utc).strftime('%Y-%m-%dT%H:%M:%SZ'),
        'iterations': only((r['iterations'] for r in results.values()), 'iteration counts'),
        'seed': only((r['seed'] for r in results.values()), 'seeds'),
        'builds': builds,
    }
    dest = out / 'weights.json'
    tmp = dest.with_suffix('.json.tmp')
    tmp.write_text(json.dumps(weights, indent=1) + '\n', encoding='utf-8')
    tmp.replace(dest)
    print(f'{dest}: {len(builds)} builds, {len(results)} phases, sim {weights["sim_commit"]}')


if __name__ == '__main__':
    main()
