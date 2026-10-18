"""Merges the runner's --out/results/*.json into assets/rankings/rankings.json.

    python tools/rankings/merge.py --out tmp/rankings
"""
import argparse
import datetime
import importlib.util
import json
import sys
from pathlib import Path

sys.dont_write_bytecode = True
REPO = Path(__file__).resolve().parents[2]
sys.path.insert(0, str(REPO / 'tools' / 'presetgen'))
import gen  # noqa: E402

# statweights' merge.py shares this file's name, so it loads by path
_spec = importlib.util.spec_from_file_location('statweights_merge', REPO / 'tools' / 'statweights' / 'merge.py')
statweights_merge = importlib.util.module_from_spec(_spec)
_spec.loader.exec_module(statweights_merge)

DEST = REPO / 'assets' / 'rankings' / 'rankings.json'


def sim_commit():
    """HEAD, with -dirty when sim/, proto/ or assets/ changed. The rankings file itself doesn't count."""
    commit = statweights_merge.git('rev-parse', 'HEAD')
    paths = [*statweights_merge.SIM_PATHS, ':(exclude)assets/rankings']
    if statweights_merge.git('status', '--porcelain', '--', *paths):
        commit += '-dirty'
    return commit


def main():
    parser = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    parser.add_argument('--out', required=True, help='work directory: reads settings/ and results/')
    parser.add_argument('--dest', default=str(DEST), help='file to write (default assets/rankings/rankings.json)')
    args = parser.parse_args()

    out = Path(args.out)
    builds = [b for b in gen.BUILDS if not b.tank]
    phases = range(1, 6)
    results, missing = {}, []
    for b in builds:
        for phase in phases:
            path = out / 'results' / f'{b.key}_p{phase}.json'
            if path.exists():
                results[b.key, phase] = json.loads(path.read_text(encoding='utf-8'))
            else:
                missing.append(f'{b.key} P{phase}')
    if missing:
        raise SystemExit(f'no results for {", ".join(missing)}: run the runner first')
    problems = [f'{key} P{phase}: {why}' for (key, phase), r in results.items()
                if (why := statweights_merge.stale(out, key, phase, r))]
    if problems:
        raise SystemExit('stale results:\n' + '\n'.join(problems))

    fights = statweights_merge.only((tuple(sorted(map(int, r['fights']))) for r in results.values()), 'fight lengths')
    iterations = statweights_merge.only((r['iterations'] for r in results.values()), 'iteration counts')
    seed = statweights_merge.only((r['seed'] for r in results.values()), 'seeds')
    statweights_merge.only((r.get('sim_sha256', '') for r in results.values()), 'sim builds')
    catalog_dates = set()
    for b in builds:
        meta = json.loads((REPO / 'ui' / b.dir / 'gear_sets' / gen.META_FILE).read_text(encoding='utf-8'))
        for phase in phases:
            catalog_dates.add(meta[gen.gear_stem(b, phase)]['catalogDate'])
    catalog_date = statweights_merge.only(catalog_dates, 'catalog dates in the BiS presets')

    rows = []
    for b in builds:
        rs = [results[b.key, p] for p in phases]
        rows.append({
            'key': b.key,
            'name': b.label,
            'class': statweights_merge.only((r['class'] for r in rs), f'{b.key} classes'),
            'spec': statweights_merge.only((r['spec'] for r in rs), f'{b.key} specs'),
            'dir': b.dir,
            'talents': statweights_merge.only((r['talents'] for r in rs), f'{b.key} talents'),
            'phases': [{'phase': r['phase'], 'fights': r['fights'], 'link': r['link']} for r in rs],
        })
    rankings = {
        'sim_commit': sim_commit(),
        'generated_at': datetime.datetime.now(datetime.timezone.utc).strftime('%Y-%m-%dT%H:%M:%SZ'),
        'catalog_date': catalog_date,
        'iterations': iterations,
        'seed': seed,
        'fights': list(fights),
        'builds': rows,
    }
    dest = Path(args.dest)
    dest.parent.mkdir(parents=True, exist_ok=True)
    tmp = dest.with_suffix('.json.tmp')
    tmp.write_text(json.dumps(rankings, indent=1) + '\n', encoding='utf-8')
    tmp.replace(dest)
    print(f'{dest}: {len(rows)} builds, {len(results)} phases, sim {rankings["sim_commit"]}, '
          f'{dest.stat().st_size // 1024} KB')


if __name__ == '__main__':
    main()
