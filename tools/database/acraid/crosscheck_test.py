"""Runs crosscheck.py against a stubbed server, so its queries and the subgroups it expects can be
checked without an AzerothCore instance.

  python tools/database/acraid/crosscheck_test.py

The fixture is the case that's easiest to get wrong: a 24-strong group with subgroups 0-3 full, topped
up by one character from -names, whom acraid puts in the first subgroup with room rather than in 0.
The group are bots, led by a hunter with a pet and ammo and a warlock with a demon, and the topped-up
character is played. Each case then tampers with the export and expects crosscheck to catch it, so a
passing run says the check is doing something.
"""
import contextlib
import copy
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

HERE = os.path.dirname(os.path.abspath(__file__))
CROSSCHECK = os.path.join(HERE, 'crosscheck.py')
REPO = os.path.normpath(os.path.join(HERE, '..', '..', '..'))
# crosscheck batches per table, so a 25 character raid costs a fixed number of queries, not a few per character
MAX_QUERIES = 20
EMPTY_ENCHANTMENTS = ' '.join(['0'] * 36) + ' '
GROUP = [('Raider%02d' % i, i // 5, 0, 100 + i) for i in range(24)]
EXTRA_NAME, EXTRA_GUID, EXTRA_SUBGROUP = 'Felesta', 900, 4
HUNTER_GUID, WARLOCK_GUID = 100, 101
CLASSES = {HUNTER_GUID: '3', WARLOCK_GUID: '9'}  # the rest are paladins
QUIVER_GUID = HUNTER_GUID  # only this raider carries a quiver
MATRIX = 'AC_AI_PLAYERBOT_WORLD_BUFF_MATRIX="1:0,3,0,80,80:53760,57371;\n  2:0,3,1,80,80:53758"\n'


def write_wdbc(path, fields, rows, strings=b'\0'):
    data = struct.pack('<4sIIII', b'WDBC', len(rows), fields, fields * 4, len(strings))
    for row in rows:
        data += struct.pack('<%dI' % fields, *row)
    open(path, 'wb').write(data + strings)


def record(fields, values):
    row = [0] * fields
    for index, value in values.items():
        row[index] = value
    return tuple(row)


def write_fixtures(directory):
    # SpellItemEnchantment: id at field 0, the gem's item id at field 33
    write_wdbc(os.path.join(directory, 'SpellItemEnchantment.dbc'), 34,
               [tuple([3563] + [0] * 32 + [40111]), tuple([3520] + [0] * 32 + [40155])])
    # GlyphProperties: id, spell id, flags whose bit 0 marks a minor glyph
    write_wdbc(os.path.join(directory, 'GlyphProperties.dbc'), 3, [(170, 54733, 0), (399, 58386, 1)])
    # TalentTab: id, name, class mask at 20, pet talent mask at 21, tab page at 22
    write_wdbc(os.path.join(directory, 'TalentTab.dbc'), 24,
               [record(24, {0: 409, 1: 1, 21: 2}), record(24, {0: 410, 1: 10, 21: 1})], b'\0Tenacity\0Ferocity\0')
    # Talent: id, tab, row, col, rank spells; Cobra Reflexes sits top left in both pet trees
    write_wdbc(os.path.join(directory, 'Talent.dbc'), 23,
               [record(23, {0: 2118, 1: 409, 4: 61682, 5: 61683}), record(23, {0: 2203, 1: 410, 4: 61682, 5: 61683})])
    # CreatureFamily: id, pet talent type at 8, name at 10
    write_wdbc(os.path.join(directory, 'CreatureFamily.dbc'), 28,
               [record(28, {0: 42, 8: 1, 10: 1}), record(28, {0: 15, 8: 0xFFFFFFFF, 10: 6})], b'\0Worm\0Felhunter\0')
    # Spell: id, effects at 71, triggered spells at 116, name at 136
    write_wdbc(os.path.join(directory, 'Spell.dbc'), 137,
               [record(137, {0: 57370, 71: 6, 116: 57371, 136: 1}), record(137, {0: 57371, 71: 6, 136: 13}),
                record(137, {0: 43186, 71: 30, 136: 22})], b'\0Refreshment\0Well Fed\0Restore Mana\0')
    json.dump({'items': [{'id': 47112, 'stats': [0] * 35}]}, open(os.path.join(directory, 'db.json'), 'w'))
    open(os.path.join(directory, 'Playerbot.env'), 'w').write(MATRIX)


def server_rows(query, guids):
    if 'FROM acore_characters.group_member gm' in query and 'c.level' in query:
        return [[name, str(subgroup), str(flags), str(guid), '1', CLASSES.get(guid, '2'), '80']
                for name, subgroup, flags, guid in GROUP]
    if query.startswith('SELECT name, guid, race, class, level FROM acore_characters.characters'):
        return [[EXTRA_NAME, str(EXTRA_GUID), '1', '2', '80']]
    if 'character_skills' in query:
        return [[str(guid), '171', '450'] for guid in guids]
    if any(table in query for table in ('character_racial_swap', 'character_reforging', 'character_talent',
                                        'character_spell', 'playerbots_account_type')):
        return []
    if 'BETWEEN 23 AND 38' in query:
        # bags: entry, subclass, quality, item level, spells
        speed = ['40211', '1', '1', '80', '53908', '0', '0', '0', '0']
        filet = ['43000', '5', '1', '80', '57370', '0', '0', '0', '0']
        return ([[str(HUNTER_GUID)] + speed] if HUNTER_GUID in guids else []) + \
            ([[str(EXTRA_GUID)] + speed, [str(EXTRA_GUID)] + filet] if EXTRA_GUID in guids else [])
    if 'character_inventory' in query and 'BETWEEN 19 AND 22' in query:
        return [[str(QUIVER_GUID), '11']] if QUIVER_GUID in guids else []
    if 'character_inventory' in query:
        return [[str(guid), '0', str(guid * 10), '47112', EMPTY_ENCHANTMENTS, '0', '1', '0'] for guid in guids]
    if 'character_glyphs' in query:
        return [[str(guid), '170', '399', '0', '0', '0', '0'] for guid in guids]
    if 'c.ammoId' in query:
        return [[str(guid), '52021', '91', '92'] if guid == HUNTER_GUID else [str(guid), '0', 'NULL', 'NULL']
                for guid in guids]
    if 'pet_spell' in query:
        return [[str(HUNTER_GUID), '5000', '61683'], [str(HUNTER_GUID), '5000', '52474']]
    if 'character_pet' in query:
        return [[str(HUNTER_GUID), '5000', 'Grub', '42'], [str(WARLOCK_GUID), '5001', 'Khiigrom', '15']]
    if 'character_aura' in query:
        return [[str(EXTRA_GUID), '53760'], [str(EXTRA_GUID), '25898']]
    if 'playerbots_db_store' in query:
        return [row for guid in guids if guid != EXTRA_GUID
                for row in ([str(guid), 'co', '+dps assist,+potions'], [str(guid), 'nc', '+worldbuff'])]
    if 'subclass IN (2, 3)' in query:
        return [['53760', '46377', '3']]
    if query.startswith('SELECT entry, spellid_1 FROM acore_world.item_template'):
        return [['46377', '53760'], ['43000', '57370']]
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


NONE = {'flask': 'FlaskUnknown', 'battleElixir': 'BattleElixirUnknown', 'guardianElixir': 'GuardianElixirUnknown',
        'food': 'FoodUnknown', 'defaultPotion': 'UnknownPotion', 'prepopPotion': 'UnknownPotion',
        'defaultConjured': 'ConjuredUnknown', 'thermalSapper': False, 'explosiveDecoy': False,
        'fillerExplosive': 'ExplosiveUnknown', 'petFood': 'PetFoodUnknown', 'petScrollOfAgility': 0,
        'petScrollOfStrength': 0}
ENDLESS_RAGE = {'value': 'FlaskOfEndlessRage', 'itemId': 46377, 'spellId': 53760}
SPEED = {'value': 'PotionOfSpeed', 'source': 'bags', 'itemId': 40211}


def consumes(bot, buffs_source, **fields):
    """Every field as acraid fills it: nothing, unless fields says otherwise."""
    wanted = {field: {'value': value, 'source': 'rules' if bot else 'bags'} for field, value in NONE.items()
              if bot or not field.startswith('petScroll')}
    for field in ('flask', 'battleElixir', 'guardianElixir'):
        wanted[field]['source'] = buffs_source
    wanted['food']['source'] = 'matrix' if bot else 'bags'
    wanted['defaultPotion']['source'] = 'bags'
    for field, value in fields.items():
        wanted[field] = value
    return wanted


def character(name, subgroup, flags, guid):
    entry = {'name': name, 'classId': int(CLASSES.get(guid, '2')), 'raceId': 1, 'level': 80, 'subgroup': subgroup,
             'memberFlags': flags, 'swapRaceId': 0, 'professions': ['Alchemy'],
             'glyphs': {'major': [54733], 'minor': [58386]},
             'gear': [{'acSlot': 0, 'id': 47112, 'enchant': 0, 'gems': [], 'extraGem': 0}],
             'quiver': guid == QUIVER_GUID, 'bot': guid != EXTRA_GUID, 'consumes': consumes(True, 'matrix')}
    if guid == HUNTER_GUID:
        entry['pet'] = {'name': 'Grub', 'family': 42, 'familyName': 'Worm', 'petType': 'Worm', 'talents': '2'}
        entry['ammo'] = {'itemId': 52021, 'dps': 91.5, 'value': 'IcebladeArrow'}
        entry['consumes'] = consumes(True, 'matrix', flask={**ENDLESS_RAGE, 'source': 'matrix'},
                                     food={'value': 'FoodDragonfinFilet', 'source': 'matrix', 'itemId': 43000,
                                           'spellId': 57371}, defaultPotion=SPEED)
    if guid == WARLOCK_GUID:
        entry['pet'] = {'name': 'Khiigrom', 'family': 15, 'familyName': 'Felhunter', 'summon': 'Felhunter'}
    if guid == EXTRA_GUID:
        entry['consumes'] = consumes(False, 'buffs', flask={**ENDLESS_RAGE, 'source': 'buffs'},
                                     food={'value': 'FoodDragonfinFilet', 'source': 'bags', 'itemId': 43000},
                                     defaultPotion=SPEED, prepopPotion=SPEED)
    return entry


def export():
    """What acraid would have written for the stubbed server."""
    members = GROUP + [(EXTRA_NAME, EXTRA_SUBGROUP, 0, EXTRA_GUID)]
    return {'version': 2, 'characters': [character(*member) for member in members]}


def crosscheck(directory, roster):
    """crosscheck.py's exit code, output and query count over that roster."""
    path = os.path.join(directory, 'raid.json')
    json.dump(roster, open(path, 'w'))
    queries = []
    real_run, subprocess.run = subprocess.run, stub_server(queries)
    sys.argv = ['crosscheck.py', path, '--dbc', directory, '--leader', GROUP[0][0],
                '--names', EXTRA_NAME, '--sim-db', os.path.join(directory, 'db.json'),
                '--world-buff-matrix', os.path.join(directory, 'Playerbot.env'),
                '--trees', os.path.join(REPO, 'ui', 'core', 'talents', 'trees'),
                '--consumables', os.path.join(REPO, 'ui', 'core', 'components', 'inputs', 'consumables.ts'),
                '--hunter-go', os.path.join(REPO, 'sim', 'hunter', 'hunter.go'), '--proto', os.path.join(REPO, 'proto')]
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


def tamper(mutate, name=EXTRA_NAME):
    roster = copy.deepcopy(export())
    mutate(next(character for character in roster['characters'] if character['name'] == name))
    return roster


HUNTER_NAME = GROUP[HUNTER_GUID - 100][0]
CASES = [
    ('an untouched export', export(), None),
    # the bug this covers: -names characters used to be checked against subgroup 0 whatever acraid did
    ('the topped-up character parked in subgroup 0', tamper(lambda c: c.update(subgroup=0)), 'subgroup'),
    ('a gem that was never socketed', tamper(lambda c: c['gear'][0].update(gems=[40111])), 'slot 0'),
    ('a dropped profession', tamper(lambda c: c.update(professions=[])), 'professions'),
    ('a dropped glyph', tamper(lambda c: c['glyphs'].update(major=[])), 'glyphs'),
    ('a quiver the server does not have', tamper(lambda c: c.update(quiver=True)), 'quiver'),
    ('a played character counted as a bot', tamper(lambda c: c.update(bot=True)), 'bot'),
    ('pet talents the pet does not have', tamper(lambda c: c['pet'].update(talents='1'), HUNTER_NAME), 'pet'),
    ('ammo of another DPS', tamper(lambda c: c['ammo'].update(value='SaroniteRazorheads'), HUNTER_NAME), 'ammo'),
    ('a flask the matrix does not give', tamper(lambda c: c['consumes']['flask'].update(value='FlaskOfStoneblood'),
                                                 HUNTER_NAME), 'flask'),
    ('the potion from another bag item', tamper(lambda c: c['consumes']['defaultPotion'].update(itemId=40212)),
     'defaultPotion'),
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
