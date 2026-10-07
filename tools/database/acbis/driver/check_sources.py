#!/usr/bin/env python3
"""Audits a BiS batch export against the server catalog: every ranked item must be one the server
hands out by the run's content phase, from a source kind the run kept.

It re-derives the rule in `ui/core/optimizer/catalog.ts` (`isAvailable`) instead of calling it, so a
pool builder that stops filtering shows up here. Faction is deliberately left out: the pool follows
the sim player's race, which a racial swap flips, so a mismatch here would say nothing.

    python tools/database/acbis/driver/check_sources.py BATCH ROSTER [--catalog PATH]

Exits 1 when a pick is outside the kept sources, 0 otherwise.
"""

import argparse
import collections
import json
import pathlib
import sys

# CatalogSourceKind, in proto order: the export writes the enum names, the catalog the numbers.
KINDS = [
    'CatalogSourceUnknown',
    'CatalogSourceDungeon',
    'CatalogSourceDungeonHeroic',
    'CatalogSourceRaid10',
    'CatalogSourceRaid10Heroic',
    'CatalogSourceRaid25',
    'CatalogSourceRaid25Heroic',
    'CatalogSourceVendor',
    'CatalogSourceCrafted',
    'CatalogSourceQuest',
    'CatalogSourceReputation',
    'CatalogSourceAchievement',
    'CatalogSourceProspecting',
    'CatalogSourceWorldDrop',
]

PHASE_TIER_OFFSET = 12
NO_TIER = 99


def kind_name(kind):
    if isinstance(kind, str):
        return kind
    kind = kind or 0
    return KINDS[kind] if 0 <= kind < len(KINDS) else f'kind{kind}'


def load_catalog(path):
    """One item per line in the generated file, so it loads without holding the whole JSON tree."""
    items = {}
    with open(path, encoding='utf-8') as handle:
        for line in handle:
            line = line.strip().rstrip(',')
            if not line.startswith('{"id"'):
                continue
            try:
                row = json.loads(line)
            except json.JSONDecodeError:
                continue
            items[row['id']] = row
    if not items:
        sys.exit(f'no catalog items parsed out of {path}')
    return items


def worn_by_raider(roster):
    worn = {}
    for character in roster['characters']:
        worn[character['name']] = {
            slot['id'] for slot in character.get('gear') or [] if slot.get('id')
        }
    return worn


def ranked_items(result):
    """Every item the entry ranks: the best loadout's slots plus each slot's alternatives."""
    ids = [slot['id'] for slot in result['best']['equipment']['items'] if slot.get('id')]
    for alternative in result.get('alternatives') or []:
        item = alternative.get('item') or {}
        if item.get('id'):
            ids.append(item['id'])
    return ids


def available(row, cap, kept):
    """Mirrors isAvailable, less the faction test. Returns (ok, reason when not)."""
    if row is None:
        return False, 'not in the catalog'
    if row.get('pvp'):
        return False, 'PvP item'
    sources = row.get('sources') or []
    if not sources:
        tier = row.get('progressionTier', NO_TIER)
        return tier <= cap, f'no source, tier {tier} over the phase cap {cap}'
    for source in sources:
        kind = kind_name(source.get('kind', 0))
        if source.get('progressionTier', NO_TIER) > cap:
            continue
        if kind == 'CatalogSourceUnknown' or kind in kept:
            return True, ''
    kinds = sorted({kind_name(s.get('kind', 0)) for s in sources})
    return False, 'sources ' + ','.join(kinds)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('batch', help="the driver's batch-stage<N>.json")
    parser.add_argument('roster', help="acraid's roster JSON, for the gear a raider already wears")
    parser.add_argument('--catalog', help='the server catalog (default: the checkout\'s)')
    args = parser.parse_args()

    repo = pathlib.Path(__file__).resolve().parents[4]
    catalog_path = args.catalog or repo / 'assets' / 'database' / 'server_catalog.json'
    catalog = load_catalog(catalog_path)

    with open(args.batch, encoding='utf-8') as handle:
        batch = json.load(handle)
    with open(args.roster, encoding='utf-8') as handle:
        roster = json.load(handle)
    worn = worn_by_raider(roster)

    kinds_used = collections.Counter()
    failures, kept_worn = [], []
    checked = 0
    for entry in batch['entries']:
        result = entry['result']
        kept = {kind_name(k) for k in result['settings'].get('sources') or []}
        cap = PHASE_TIER_OFFSET + entry['contentPhase']
        for item_id in ranked_items(result):
            checked += 1
            row = catalog.get(item_id)
            for source in (row or {}).get('sources') or []:
                kinds_used[kind_name(source.get('kind', 0))] += 1
            ok, reason = available(row, cap, kept)
            if ok:
                continue
            name = (row or {}).get('name', '?')
            where = kept_worn if item_id in worn.get(entry['raider'], ()) else failures
            where.append((entry['raider'], entry['contentPhase'], item_id, name, reason))

    print(f'{len(batch["entries"])} entries, {checked} ranked slots checked')
    print(f'catalog {batch.get("catalogDate", "?")}, sim {batch.get("simCommit", "?")}')
    print('source kinds behind the picks:', dict(kinds_used.most_common()))
    print(f'kept because the raider already wears it: {len(kept_worn)}')
    for row in kept_worn:
        print('  worn:', row)
    print(f'outside the kept sources: {len(failures)}')
    for row in failures:
        print('  !', row)
    return 1 if failures else 0


if __name__ == '__main__':
    sys.exit(main())
