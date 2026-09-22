"""Checks an acraid export against the server it came from.

Rebuilds gear, gems, reforges, glyphs, professions, races, subgroups and quivers from SQL, the DBC files
and the sim's item database, then diffs that against the JSON. It's a second implementation on purpose:
the Go tests cover the pure conversions, this covers the queries and the DBC field indices they run on.

  python tools/database/acraid/crosscheck.py raid.json --dbc <dir> [--leader Deathsong] [--names a,b]

--dbc takes the directory acraid read, e.g. what `docker cp ac-worldserver:/azerothcore/env/dist/data/dbc`
gives. The queries go through `docker exec ac-database mysql`, so they need that container running.
"""
import argparse
import collections
import json
import math
import struct
import subprocess
import sys

MAX_GEM_SOCKETS = 3
MAX_RAID_SIZE = 40
ITEM_CLASS_QUIVER = 11  # item_template.class for quivers and ammo pouches
PRIMARY_PROFESSIONS = {164: 'Blacksmithing', 165: 'Leatherworking', 171: 'Alchemy', 182: 'Herbalism', 186: 'Mining',
                       197: 'Tailoring', 202: 'Engineering', 333: 'Enchanting', 393: 'Skinning', 755: 'Jewelcrafting',
                       773: 'Inscription'}
# ItemModType -> the sim's stat indices, and the server's Reforging.Percentage.
REFORGE_STATS = {6: (4,), 13: (25,), 14: (26,), 31: (12, 7), 32: (13, 8), 36: (14, 9), 37: (16,)}
REFORGE_PERCENTAGE = 0.4

Member = collections.namedtuple('Member', 'name subgroup flags guid race class_id level')


def sql(query):
    result = subprocess.run(['docker', 'exec', args.container, 'mysql', f'-u{args.user}', f'-p{args.password}',
                             '-N', '-B', '-e', query], capture_output=True, text=True)
    if result.returncode:
        sys.exit(result.stderr)
    return [line.split('\t') for line in result.stdout.splitlines() if line.strip()]


def by_guid(query, guids):
    """One query for every character at once, its rows grouped by the guid they start with."""
    rows = sql(query % ', '.join(str(guid) for guid in guids))
    grouped = {guid: [] for guid in guids}
    for row in rows:
        grouped[int(row[0])].append(row[1:])
    return grouped


def dbc_rows(name):
    data = open(f'{args.dbc}/{name}', 'rb').read()
    magic, count, fields, size, _ = struct.unpack('<4sIIII', data[:20])
    if magic != b'WDBC':
        sys.exit(f'{name} is not a WDBC file')
    for row in range(count):
        yield struct.unpack_from('<%dI' % fields, data, 20 + row * size)


def named_rows():
    """The characters --names lists, in the order acraid would have taken them."""
    names = [name.strip() for name in args.names.split(',') if name.strip()]
    quoted = ', '.join("'%s'" % name.replace("'", "''") for name in names)
    found = {row[0].lower(): row for row in
             sql(f"""SELECT name, guid, race, class, level FROM acore_characters.characters
                     WHERE name IN ({quoted})""")}
    missing = [name for name in names if name.lower() not in found]
    if missing:
        sys.exit(f'no character named {", ".join(missing)}')
    return [found[name.lower()] for name in names]


def group_rows():
    leader = args.leader.replace("'", "''")
    return sql(f"""SELECT c.name, gm.subgroup, gm.memberFlags, c.guid, c.race, c.class, c.level
                   FROM acore_characters.group_member gm
                   JOIN acore_characters.characters c ON c.guid = gm.memberGuid
                   WHERE gm.guid = (SELECT guid FROM acore_characters.group_member WHERE memberGuid =
                       (SELECT guid FROM acore_characters.characters WHERE name = '{leader}'))""")


def select_members():
    """The group, the characters named, or the group topped up with the ones not in it. The subgroups
    have to match how acraid assigns them: the group keeps its own, --names fills subgroups of 5 in
    order, and a character topping up a group goes in the first subgroup with room."""
    found = [Member(row[0], int(row[1]), int(row[2]), int(row[3]), int(row[4]), int(row[5]), int(row[6]))
             for row in (group_rows() if args.leader else [])]
    if not args.names:
        return found

    sizes = collections.Counter(member.subgroup for member in found)
    known = {member.guid for member in found}
    for name, guid, race, class_id, level in named_rows():
        if int(guid) in known:
            continue
        if args.leader:
            subgroup = next((group for group in range(MAX_RAID_SIZE // 5) if sizes[group] < 5), None)
            if subgroup is None:
                sys.exit('the group is full, so acraid had nowhere to put the characters named')
        else:
            subgroup = len(found) // 5
        sizes[subgroup] += 1
        found.append(Member(name, subgroup, 0, int(guid), int(race), int(class_id), int(level)))
    return found


def stat_value(base, index):
    return base[index] if index < len(base) else 0


def can_reforge(base, from_stat, to_stat):
    """The server's rule as sim/core/reforging.go applies it. base is the item's stats in the sim's
    database, None when it has no row for the item, which leaves only the stat types to go on."""
    if from_stat == to_stat or from_stat not in REFORGE_STATS or to_stat not in REFORGE_STATS:
        return False
    if base is None:
        return True
    if max(stat_value(base, index) for index in REFORGE_STATS[to_stat]) > 0:
        return False
    return math.floor(REFORGE_PERCENTAGE * max(stat_value(base, index) for index in REFORGE_STATS[from_stat])) >= 1


def socketed_gems(tokens, sockets, known_template):
    """What acraid exports for one item's sockets: the item_template count, or, without a template row,
    the filled sockets with the socket-adding enchant's gem in the last of them."""
    filled = max((i + 1 for i in range(MAX_GEM_SOCKETS) if tokens[6 + 3 * i]), default=0)
    prismatic = tokens[18]
    native = min(sockets, MAX_GEM_SOCKETS)
    if not known_template:
        native = filled - 1 if prismatic and filled else filled
    gems = [gem_items.get(tokens[6 + 3 * i], 0) for i in range(native)]
    extra = gem_items.get(tokens[6 + 3 * native], 0) if prismatic and native < MAX_GEM_SOCKETS else 0
    return gems, extra


parser = argparse.ArgumentParser()
parser.add_argument('roster', help='the JSON acraid wrote')
parser.add_argument('--dbc', required=True, help='directory holding the server DBC files')
parser.add_argument('--leader', help='the character acraid was run with')
parser.add_argument('--names', help='comma separated names, when acraid was run with -names')
parser.add_argument('--sim-db', default='assets/database/db.json', help='what acraid was run with as -simDb')
parser.add_argument('--min-skill', type=int, default=1, help='what acraid was run with as -minSkill')
parser.add_argument('--container', default='ac-database')
parser.add_argument('--user', default='root')
parser.add_argument('--password', default='password')
args = parser.parse_args()
if not args.leader and not args.names:
    parser.error('pass --leader or --names, whichever acraid used')

gem_items = {row[0]: row[33] for row in dbc_rows('SpellItemEnchantment.dbc') if row[33]}
glyphs = {row[0]: (row[1], row[2] & 1) for row in dbc_rows('GlyphProperties.dbc')}
sim_items = {item['id']: item.get('stats') or [] for item in json.load(open(args.sim_db))['items']}

roster = json.load(open(args.roster))
exported = {character['name']: character for character in roster['characters']}
members = select_members()
guids = [member.guid for member in members]
if not guids:
    sys.exit('no characters selected, so there is nothing acraid could have exported')

problems = []
if sorted(member.name for member in members) != sorted(exported):
    problems.append(f'members differ: server {sorted(member.name for member in members)}, export {sorted(exported)}')

skills = by_guid('SELECT guid, skill, value FROM acore_characters.character_skills WHERE guid IN (%s)', guids)
swaps = by_guid('SELECT guid, selected_race FROM acore_characters.character_racial_swap WHERE guid IN (%s)', guids)
reforges = by_guid("""SELECT guid, item_guid, stat_decrease, stat_increase
                      FROM acore_characters.character_reforging WHERE guid IN (%s)""", guids)
equipped = by_guid("""SELECT ci.guid, ci.slot, ii.guid, ii.itemEntry, ii.enchantments, COALESCE(ii.randomPropertyId, 0),
                        it.entry IS NOT NULL,
                        (COALESCE(it.socketColor_1,0)<>0)+(COALESCE(it.socketColor_2,0)<>0)+(COALESCE(it.socketColor_3,0)<>0)
                      FROM acore_characters.character_inventory ci
                      JOIN acore_characters.item_instance ii ON ii.guid = ci.item
                      LEFT JOIN acore_world.item_template it ON it.entry = ii.itemEntry
                      WHERE ci.bag = 0 AND ci.slot < 19 AND ci.slot NOT IN (3, 18) AND ci.guid IN (%s)""", guids)
quivers = by_guid("""SELECT ci.guid, it.class
                     FROM acore_characters.character_inventory ci
                     JOIN acore_characters.item_instance ii ON ii.guid = ci.item
                     JOIN acore_world.item_template it ON it.entry = ii.itemEntry
                     WHERE ci.bag = 0 AND ci.slot BETWEEN 19 AND 22 AND ci.guid IN (%s)""", guids)
# the character's active spec only, which is what the server would load
equipped_glyphs = by_guid("""SELECT cg.guid, cg.glyph1, cg.glyph2, cg.glyph3, cg.glyph4, cg.glyph5, cg.glyph6
                             FROM acore_characters.character_glyphs cg
                             JOIN acore_characters.characters c ON c.guid = cg.guid AND c.activeTalentGroup = cg.talentGroup
                             WHERE cg.guid IN (%s)""", guids)

items = 0
for member in members:
    name = member.name
    character = exported.get(name)
    if character is None:
        continue
    for field, want in [('raceId', member.race), ('classId', member.class_id), ('level', member.level),
                        ('subgroup', member.subgroup), ('memberFlags', member.flags)]:
        if character[field] != want:
            problems.append(f'{name}.{field} = {character[field]}, server says {want}')

    learned = sorted(((int(value), PRIMARY_PROFESSIONS[int(skill)]) for skill, value in skills[member.guid]
                      if int(skill) in PRIMARY_PROFESSIONS and int(value) >= args.min_skill),
                     key=lambda p: (-p[0], p[1]))
    want_professions = [profession for _, profession in learned]
    if character['professions'] != want_professions:
        problems.append(f'{name}.professions = {character["professions"]}, server says {want_professions}')

    swap = swaps[member.guid]
    want_swap = int(swap[0][0]) if swap else 0
    if character['swapRaceId'] != want_swap:
        problems.append(f'{name}.swapRaceId = {character["swapRaceId"]}, server says {want_swap}')

    want_quiver = any(int(row[0]) == ITEM_CLASS_QUIVER for row in quivers[member.guid])
    if character.get('quiver', False) != want_quiver:
        problems.append(f'{name}.quiver = {character.get("quiver", False)}, server says {want_quiver}')

    item_reforges = {int(row[0]): (int(row[1]), int(row[2])) for row in reforges[member.guid]}

    want_gear = {}
    for slot, item_guid, entry, enchantments, random_property, known_template, sockets in equipped[member.guid]:
        tokens = [int(token) for token in enchantments.split()]
        if len(tokens) != 36:
            problems.append(f'{name} slot {slot}: {len(tokens)} enchantment tokens')
            continue
        gems, extra = socketed_gems(tokens, int(sockets), int(known_template))
        item = {'acSlot': int(slot), 'id': int(entry), 'enchant': tokens[0], 'gems': gems, 'extraGem': extra}
        reforge = item_reforges.get(int(item_guid))
        if reforge and can_reforge(sim_items.get(int(entry)), reforge[0], reforge[1]):
            item['reforge'] = {'fromStatType': reforge[0], 'toStatType': reforge[1]}
        if int(random_property):
            problems.append(f'{name} slot {slot}: random property {random_property}, which nothing exports')
        want_gear[int(slot)] = item

    got_gear = {item['acSlot']: item for item in character['gear']}
    items += len(got_gear)
    for slot in sorted(set(got_gear) | set(want_gear)):
        if got_gear.get(slot) != want_gear.get(slot):
            problems.append(f'{name} slot {slot}: export {got_gear.get(slot)}, server {want_gear.get(slot)}')

    rows = equipped_glyphs[member.guid]
    want_glyphs = {'major': [], 'minor': []}
    for glyph in (int(glyph) for glyph in rows[0]) if rows else []:
        if glyph in glyphs:
            want_glyphs['minor' if glyphs[glyph][1] else 'major'].append(glyphs[glyph][0])
    for kind in want_glyphs:
        if sorted(character['glyphs'][kind]) != sorted(want_glyphs[kind]):
            problems.append(f'{name} {kind} glyphs: export {character["glyphs"][kind]}, server {want_glyphs[kind]}')

print(f'checked {len(members)} characters, {items} items')
print('\n'.join(problems) if problems else 'no differences')
sys.exit(1 if problems else 0)
