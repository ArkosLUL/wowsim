"""Runs crosscheck.py against a stubbed server, so its queries and the subgroups it expects can be
checked without an AzerothCore instance.

  python tools/database/acraid/crosscheck_test.py

The fixture is the case that's easiest to get wrong: a 24-strong group with subgroups 0-3 full, topped
up by one character from -names, whom acraid puts in the first subgroup with room rather than in 0.
Each case then tampers with the export and expects crosscheck to catch it, so a passing run says the
check is doing something.
"""
import contextlib
import io
import json
import os
import re
import runpy
import struct
import subprocess
import sys
import tempfile
import types

CROSSCHECK = os.path.join(os.path.dirname(os.path.abspath(__file__)), 'crosscheck.py')
# crosscheck batches per table, so a 25 character raid costs a handful of queries, not 5 per character
MAX_QUERIES = 10
EMPTY_ENCHANTMENTS = ' '.join(['0'] * 36) + ' '
GROUP = [('Raider%02d' % i, i // 5, 0, 100 + i) for i in range(24)]
EXTRA_NAME, EXTRA_GUID, EXTRA_SUBGROUP = 'Felesta', 900, 4


def write_wdbc(path, fields, rows):
    data = struct.pack('<4sIIII', b'WDBC', len(rows), fields, fields * 4, 1)
    for row in rows:
        data += struct.pack('<%dI' % fields, *row)
    open(path, 'wb').write(data + b'\0')


def write_fixtures(directory):
    # SpellItemEnchantment: id at field 0, the gem's item id at field 33
    write_wdbc(os.path.join(directory, 'SpellItemEnchantment.dbc'), 34,
               [tuple([3563] + [0] * 32 + [40111]), tuple([3520] + [0] * 32 + [40155])])
    # GlyphProperties: id, spell id, flags whose bit 0 marks a minor glyph
    write_wdbc(os.path.join(directory, 'GlyphProperties.dbc'), 3, [(170, 54733, 0), (399, 58386, 1)])
    json.dump({'items': [{'id': 47112, 'stats': [0] * 35}]}, open(os.path.join(directory, 'db.json'), 'w'))


def server_rows(query, guids):
    if 'FROM acore_characters.group_member gm' in query and 'c.level' in query:
        return [[name, str(subgroup), str(flags), str(guid), '1', '2', '80'] for name, subgroup, flags, guid in GROUP]
    if query.startswith('SELECT name, guid, race, class, level FROM acore_characters.characters'):
        return [[EXTRA_NAME, str(EXTRA_GUID), '1', '2', '80']]
    if 'character_skills' in query:
        return [[str(guid), '171', '450'] for guid in guids]
    if 'character_racial_swap' in query or 'character_reforging' in query:
        return []
    if 'character_inventory' in query:
        return [[str(guid), '0', str(guid * 10), '47112', EMPTY_ENCHANTMENTS, '0', '1', '0'] for guid in guids]
    if 'character_glyphs' in query:
        return [[str(guid), '170', '399', '0', '0', '0', '0'] for guid in guids]
    raise AssertionError('the stub has no answer for: ' + query)


def stub_server(queries):
    def run(command, **_):
        query = ' '.join(command[command.index('-e') + 1].split())
        queries.append(query)
        # the guid list is the last IN (...), after character_inventory's own NOT IN (3, 18). The
        # lookup by name is the one query whose IN list holds names instead.
        in_lists = re.findall(r'IN \((\d[\d, ]*)\)', query)
        guids = [int(guid) for guid in in_lists[-1].split(',')] if in_lists else []
        rows = server_rows(query, guids)
        return types.SimpleNamespace(returncode=0, stdout='\n'.join('\t'.join(row) for row in rows) + '\n', stderr='')
    return run


def export():
    """What acraid would have written for the stubbed server."""
    members = GROUP + [(EXTRA_NAME, EXTRA_SUBGROUP, 0, EXTRA_GUID)]
    return {'version': 1, 'characters': [
        {'name': name, 'classId': 2, 'raceId': 1, 'level': 80, 'subgroup': subgroup, 'memberFlags': flags,
         'swapRaceId': 0, 'professions': ['Alchemy'], 'glyphs': {'major': [54733], 'minor': [58386]},
         'gear': [{'acSlot': 0, 'id': 47112, 'enchant': 0, 'gems': [], 'extraGem': 0}]}
        for name, subgroup, flags, guid in members]}


def crosscheck(directory, roster):
    """crosscheck.py's exit code, output and query count over that roster."""
    path = os.path.join(directory, 'raid.json')
    json.dump(roster, open(path, 'w'))
    queries = []
    real_run, subprocess.run = subprocess.run, stub_server(queries)
    sys.argv = ['crosscheck.py', path, '--dbc', directory, '--leader', GROUP[0][0],
                '--names', EXTRA_NAME, '--sim-db', os.path.join(directory, 'db.json')]
    output = io.StringIO()
    try:
        with contextlib.redirect_stdout(output):
            runpy.run_path(CROSSCHECK, run_name='__main__')
        code = 0
    except SystemExit as exit:
        code = exit.code
    finally:
        subprocess.run = real_run
    return code, output.getvalue(), len(queries)


def extra(roster):
    return next(character for character in roster['characters'] if character['name'] == EXTRA_NAME)


def tamper(mutate):
    roster = export()
    mutate(extra(roster))
    return roster


CASES = [
    ('an untouched export', export(), None),
    # the bug this covers: -names characters used to be checked against subgroup 0 whatever acraid did
    ('the topped-up character parked in subgroup 0', tamper(lambda c: c.update(subgroup=0)), 'subgroup'),
    ('a gem that was never socketed', tamper(lambda c: c['gear'][0].update(gems=[40111])), 'slot 0'),
    ('a dropped profession', tamper(lambda c: c.update(professions=[])), 'professions'),
    ('a dropped glyph', tamper(lambda c: c['glyphs'].update(major=[])), 'glyphs'),
]


def main():
    failures = []
    with tempfile.TemporaryDirectory() as directory:
        write_fixtures(directory)
        for label, roster, want in CASES:
            code, output, queries = crosscheck(directory, roster)
            problems = []
            if want is None and code != 0:
                problems.append(f'reported differences:\n{output}')
            elif want is not None and (code == 0 or want not in output):
                problems.append(f'expected a difference mentioning {want!r}, got:\n{output}')
            if queries > MAX_QUERIES:
                problems.append(f'{queries} queries for {len(GROUP) + 1} characters, expected {MAX_QUERIES} or fewer')
            print(f'{"FAIL" if problems else "ok  "} {label} ({queries} queries)')
            failures += [f'{label}: {problem}' for problem in problems]

    print('\n'.join(failures) if failures else '\nall cases passed')
    return 1 if failures else 0


if __name__ == '__main__':
    sys.exit(main())
