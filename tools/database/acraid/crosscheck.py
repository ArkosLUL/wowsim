"""Checks an acraid export against the server it came from.

Rebuilds gear, gems, reforges, glyphs, professions, races, subgroups, quivers, the bot flag, pets, ammo
and consumables from SQL, the DBC files and the sim's own sources, then diffs that against the JSON.
It's a second implementation on purpose: the Go tests cover the pure conversions, this covers the
queries and the DBC field indices they run on. Where acraid keeps a table of the sim's values, this
reads them from the sim instead: consumable items from its pickers, ammo DPS from sim/hunter, pet
families from the protos, and each consumable's buff from item_template and Spell.dbc.

  python tools/database/acraid/crosscheck.py raid.json --dbc <dir> [--leader Deathsong] [--names a,b]
      [--world-buff-matrix <file>] [--players a,b]

--dbc takes the directory acraid read, e.g. what `docker cp ac-worldserver:/azerothcore/env/dist/data/dbc`
gives. The queries go through `docker exec ac-database mysql`, so they need that container running. The
sim's files are read relative to the working directory, so run it from the repo root.
"""
import argparse
import collections
import json
import math
import re
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

HUNTER, WARLOCK = 3, 9
RANGED_SLOT, THORIDAL = 17, 34334
ALLIANCE_RACES = {1, 3, 4, 7, 11}
NO_MANA_CLASSES = {1, 4, 6}  # warrior, rogue, death knight
# The consumables the sim's pickers show one item for, where the server has others that give the same.
EXTRA_ITEMS = {20520: ('defaultConjured', 'ConjuredDarkRune'), 33874: ('petFood', 'PetFoodKiblersBits'),
               **{item: ('defaultConjured', 'ConjuredHealthstone') for item in (36892, 36893, 36894, 22103, 22104)}}
# A feast sets down a table, and the table's spell leaves the buff.
FEAST_BUFFS = {43015: 57399, 34753: 57294}
CONJURED_ORDER = ['ConjuredDarkRune', 'ConjuredFlameCap', 'ConjuredRogueThistleTea', 'ConjuredHealthstone']
PET_FOOD_ORDER = ['PetFoodSpicedMammothTreats', 'PetFoodKiblersBits']
POTION_KINDS = [{'PotionOfSpeed', 'PotionOfWildMagic', 'HastePotion', 'DestructionPotion', 'InsaneStrengthPotion',
                 'HeroicPotion', 'MightyRagePotion'},
                {'IndestructiblePotion', 'IronshieldPotion'},
                {'RunicManaInjector', 'RunicManaPotion', 'FelManaPotion', 'SuperManaPotion'},
                {'RunicHealingInjector', 'RunicHealingPotion'}]
OFFENSIVE, DEFENSIVE, MANA, HEALING = POTION_KINDS
BOT_OFFENSIVE_POTIONS = {22838, 22828, 22839, 40211, 40212}  # OFFENSIVE_POTION_IDS in mod-playerbots' PlayerbotAI.h
# Strategies typed STRATEGY_TYPE_DPS in mod-playerbots, which is how a bot's IsDps reads its role.
DPS_STRATEGIES = {0: {'dps assist', 'grind'}, 1: {'arms', 'fury'}, 2: {'dps', 'offheal'}, 3: {'bm', 'mm', 'surv'},
                  4: {'combat', 'assassin', 'subtlety'}, 5: {'shadow', 'dps', 'holy dps'}, 6: {'frost', 'unholy'},
                  7: {'ele', 'caster', 'enh', 'melee', 'dps'}, 8: {'arcane', 'fire', 'frost', 'frostfire'},
                  9: {'affli', 'demo', 'destro', 'tank'}, 11: {'balance'}}
DEFAULT_SPEC_TABS = {8: 2, 2: 2, 5: 1, 9: 1, 6: 1}
THICK_HIDE_3, BLADE_BARRIER_5, SHAMANISTIC_RAGE, SHADOWFIEND = 16931, 55226, 30823, 34433
BOT_NEVER = ['prepopPotion', 'defaultConjured', 'thermalSapper', 'explosiveDecoy', 'fillerExplosive', 'petFood',
             'petScrollOfAgility', 'petScrollOfStrength']
BUFF_FIELDS = ['flask', 'battleElixir', 'guardianElixir', 'food']

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


def dbc_table(name, fields=None):
    """The file's rows, whole or just the fields asked for, and a reader for its string fields."""
    data = open(f'{args.dbc}/{name}', 'rb').read()
    magic, count, width, size, _ = struct.unpack('<4sIIII', data[:20])
    if magic != b'WDBC':
        sys.exit(f'{name} is not a WDBC file')
    strings = data[20 + count * size:]
    rows = []
    for row in range(count):
        base = 20 + row * size
        if fields is None:
            rows.append(struct.unpack_from('<%dI' % width, data, base))
        else:
            rows.append(tuple(struct.unpack_from('<I', data, base + 4 * field)[0] for field in fields))
    return rows, lambda offset: strings[offset:strings.index(b'\0', offset)].decode()


def dbc_rows(name):
    return dbc_table(name)[0]


def signed(value):
    return value - (1 << 32) if value >= 1 << 31 else value


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


# --- the sim's own values -------------------------------------------------------------------------

def enum_values(proto_file, enum):
    """An enum's value names in order, from the proto."""
    body = re.search(r'enum %s \{(.*?)\}' % enum, open(f'{args.proto}/{proto_file}').read(), re.S).group(1)
    return re.findall(r'(\w+) = \d+;', body)


def sim_consumables():
    """Item id -> (Consumes field, value) from the sim's consumable pickers, plus the fields' empty values."""
    text = open(args.consumables).read()
    fields = {'Flask': 'flask', 'BattleElixir': 'battleElixir', 'GuardianElixir': 'guardianElixir', 'Food': 'food',
              'Potions': 'defaultPotion', 'Conjured': 'defaultConjured', 'Explosive': 'fillerExplosive',
              'PetFood': 'petFood'}
    items = dict(EXTRA_ITEMS)
    for item, enum, value in re.findall(r'fromItemId\((\d+)\),\s*value:\s*(\w+)\.(\w+)', text):
        items[int(item)] = (fields[enum], value)
    for item, field, value in re.findall(r"fromItemId\((\d+)\), fieldName: '(\w+)'(?:, value: \w+\.(\w+))?", text):
        if field in ('thermalSapper', 'explosiveDecoy', 'petFood'):
            items[int(item)] = (field, value or True)
    empty = {field: enum_values('common.proto', enum)[0] for enum, field in fields.items()}
    empty.update(prepopPotion=empty['defaultPotion'], thermalSapper=False, explosiveDecoy=False,
                 petScrollOfAgility=0, petScrollOfStrength=0)
    prepop = set(re.findall(r'config: (\w+)', re.search(r'PRE_POTIONS_CONFIG = \[(.*?)\] as', text, re.S).group(1)))
    return items, empty, prepop


def sim_ammo():
    """(value, DPS) in the order NewHunter checks them."""
    text = open(args.hunter_go).read()
    return [(value, float(dps)) for value, dps in
            re.findall(r'case proto\.Hunter_Options_(\w+):\s*hunter\.AmmoDPS = ([\d.]+)', text)]


def food_buff(spell):
    """The buff a food's spell leaves: whichever spell it triggers that isn't the eating or drinking."""
    for trigger in spell_triggers.get(spell, ()):
        if trigger and spell_names.get(trigger) not in ('Food', 'Drink', None):
            return trigger
    return 0


def matrix_text(text):
    """The world-buff matrix out of Playerbot.env, playerbots.conf, or a file holding just the matrix."""
    quoted = re.search(r'^[ \t]*AC_AI_PLAYERBOT_WORLD_BUFF_MATRIX[ \t]*[=:][ \t]*(["\'])(.*?)\1', text, re.M | re.S)
    if quoted:
        return quoted.group(2)
    line = re.search(r'^[ \t]*(?:AC_AI_PLAYERBOT_WORLD_BUFF_MATRIX|AiPlayerbot\.WorldBuffMatrix)[ \t]*[=:](.*)$', text, re.M)
    return line.group(1) if line else text


def world_buffs():
    """(faction, class, spec, min level, max level, spell) for every spell of every well-formed entry."""
    buffs = []
    for entry in matrix_text(open(args.world_buff_matrix).read()).split(';'):
        parts = entry.strip().split(':', 2)
        if len(parts) < 3:
            continue
        meta = []
        for token in parts[1].split(','):
            number = re.match(r'\s*([+-]?\d+)', token)
            if not number:
                break
            meta.append(int(number.group(1)))
        if len(meta) != 5:
            continue
        for token in parts[2].split(','):
            number = re.match(r'\s*([+-]?\d+)', token)
            if number:
                buffs.append((*meta, int(number.group(1))))
    return buffs


# --- the rules acraid applies, written out again --------------------------------------------------

def spec_tab(class_id, level, spells):
    points = [0, 0, 0]
    for spell in spells:
        if spell in talent_ranks:
            tab, rank = talent_ranks[spell]
            class_mask, _, page, _ = talent_tabs.get(tab, (0, 0, -1, ''))
            if class_mask & (1 << (class_id - 1)) and 0 <= page < 3:
                points[page] += rank
    if level < 10 or not sum(points):
        return DEFAULT_SPEC_TABS.get(class_id, 0)
    return points.index(max(points))


def is_dps_by_spec(class_id, tab, spells):
    if class_id in (3, 4, 8, 9):
        return True
    return {5: tab == 2, 11: tab == 0 or (tab == 1 and THICK_HIDE_3 not in spells), 7: tab != 2, 2: tab == 2,
            6: tab != 0, 1: tab != 2}.get(class_id, False)


def is_heal(class_id, tab):
    return {5: tab in (0, 1), 11: tab == 2, 7: tab == 2, 2: tab == 0}.get(class_id, False)


def skips_mana_potions(class_id, level, tab, spells, knows_shadowfiend):
    if class_id == HUNTER:
        return level >= 5
    if class_id == WARLOCK:
        return level >= 6
    if class_id == 8:
        return level >= 20
    if class_id == 5:
        return not is_heal(5, tab) and knows_shadowfiend
    if class_id == 7 and not is_heal(7, tab):
        return SHAMANISTIC_RAGE in spells if tab == 1 else level >= 60
    return False


def bot_rank(item):
    """mod-playerbots' compare_items: potions before flasks, lower quality first, higher item level first."""
    return item['subclass'], item['quality'], -item['level']


def consume(value=None, source='bags', item=0, spell=0):
    entry = {'source': source}
    if value is not None:
        entry['value'] = value
    if item:
        entry['itemId'] = item
    if spell:
        entry['spellId'] = spell
    return entry


def sort_buffs(found):
    """What a list of (item, buff, item subclass) says about the flask, elixir and food fields: a buff
    alone for a saved or matrix buff, an item alone for one in the bags. Whatever the sim has no value
    for lands under 'other ...'."""
    kinds = collections.defaultdict(list)
    for item, buff, subclass in found:
        if not item and buff in sim_buffs:
            item = sim_buffs[buff]
        known = consumable_items.get(item)
        if known:
            if known[0] in BUFF_FIELDS:
                kinds[known[0]].append(consume(known[1], item=item, spell=buff))
            continue
        if not item and buff in buff_items:
            item, subclass = buff_items[buff]
        if subclass in (2, 3):
            kinds['other flask' if subclass == 3 else 'other elixir'].append(consume(item=item, spell=buff))
        elif buff in well_fed:
            kinds['other food'].append(consume(item=item, spell=buff))
    return kinds


def fill_buffs(consumes, kinds, source):
    def at(entry):
        return {**entry, 'source': source}
    if kinds['flask'] or kinds['other flask']:
        consumes['flask'] = at((kinds['flask'] or kinds['other flask'])[0])
        consumes['battleElixir'] = consume(empty['battleElixir'], source)
        consumes['guardianElixir'] = consume(empty['guardianElixir'], source)
    else:
        consumes['flask'] = consume(empty['flask'], source)
        for field in ('battleElixir', 'guardianElixir'):
            options = kinds[field] or kinds['other elixir']
            consumes[field] = at(options[0]) if options else consume(empty[field], source)


def fill_food(consumes, kinds, source):
    options = kinds['food'] or kinds['other food']
    consumes['food'] = {**options[0], 'source': source} if options else consume(empty['food'], source)


def bag_buff_kinds(bags):
    """The bags' flasks, elixirs and buff food, highest item level first. Food only counts when one of
    its spells leaves a Well Fed buff."""
    found = []
    for item in sorted(bags, key=lambda item: (-item['level'], item['id'])):
        buff = 0
        if item['subclass'] == 5 and item['id'] not in consumable_items:
            buff = next((trigger for spell in item['spells'] for trigger in spell_triggers.get(spell, ())
                         if spell and trigger in well_fed), 0)
        found.append((item['id'], buff, item['subclass']))
    return sort_buffs(found)


def first_in_bags(bags, field, order, class_id):
    """The first value in order the bags hold an item for, the highest item id when several give it."""
    held = {item['id'] for item in bags}
    for value in order:
        if value == 'ConjuredRogueThistleTea' and class_id != 4:
            continue
        for item, known in sorted(consumable_items.items(), reverse=True):
            if known == (field, value) and item in held:
                return consume(value, item=item)
    return consume(empty[field])


def player_consumes(member, auras, bags):
    consumes = {}
    buffs = sort_buffs([(0, aura, 0) for aura in auras])
    from_bags = bag_buff_kinds(bags)
    has_flask = any(buffs[kind] for kind in ('flask', 'battleElixir', 'guardianElixir', 'other flask', 'other elixir'))
    fill_buffs(consumes, buffs if has_flask else from_bags, 'buffs' if has_flask else 'bags')
    has_food = buffs['food'] or buffs['other food']
    fill_food(consumes, buffs if has_food else from_bags, 'buffs' if has_food else 'bags')

    potions = sorted((item for item in bags if consumable_items.get(item['id'], ('',))[0] == 'defaultPotion'),
                     key=lambda item: (*bot_rank(item), item['id']))
    kinds = [OFFENSIVE, DEFENSIVE] + ([MANA] if member.class_id not in NO_MANA_CLASSES else []) + [HEALING]
    picked = next((item for kind in kinds for item in potions if consumable_items[item['id']][1] in kind), None)
    others = sorted((item for item in bags if item['subclass'] == 1 and item['id'] not in consumable_items),
                    key=lambda item: (*bot_rank(item), item['id']))
    if picked:
        consumes['defaultPotion'] = consume(consumable_items[picked['id']][1], item=picked['id'])
    elif others:
        consumes['defaultPotion'] = consume(item=others[0]['id'])
    else:
        consumes['defaultPotion'] = consume(empty['defaultPotion'])
    prepop = next((item for kind in (OFFENSIVE, DEFENSIVE) for item in potions
                   if consumable_items[item['id']][1] in kind & prepop_values), None)
    consumes['prepopPotion'] = consume(consumable_items[prepop['id']][1], item=prepop['id']) if prepop \
        else consume(empty['prepopPotion'])

    consumes['defaultConjured'] = first_in_bags(bags, 'defaultConjured', CONJURED_ORDER, member.class_id)
    consumes['fillerExplosive'] = first_in_bags(bags, 'fillerExplosive', explosive_order, member.class_id)
    consumes['petFood'] = first_in_bags(bags, 'petFood', PET_FOOD_ORDER, member.class_id)
    for field in ('thermalSapper', 'explosiveDecoy'):
        consumes[field] = first_in_bags(bags, field, [True], member.class_id)
    return consumes


def bot_consumes(member, spells, strategies, bags, knows_shadowfiend):
    consumes = {}
    tab = spec_tab(member.class_id, member.level, spells)
    saved = strategies is not None
    if saved and 'worldbuff' not in strategies['nc']:
        for field in BUFF_FIELDS:
            consumes[field] = consume(empty[field], 'rules')
    elif not args.world_buff_matrix:
        for field in BUFF_FIELDS:
            consumes[field] = consume(source='matrix')
    else:
        spec = tab
        if member.class_id == 11 and tab == 1 and THICK_HIDE_3 not in spells:
            spec = 3
        if member.class_id == 6 and tab == 0 and BLADE_BARRIER_5 not in spells:
            spec = 3
        faction = 1 if member.race in ALLIANCE_RACES else 2
        buffs = []
        for buff_faction, class_id, buff_spec, low, high, spell in matrix:
            if (buff_faction in (0, faction) and class_id in (0, member.class_id) and buff_spec == spec
                    and (not low or low <= member.level) and (not high or high >= member.level) and spell not in buffs):
                buffs.append(spell)
        kinds = sort_buffs([(0, buff, 0) for buff in buffs])
        fill_buffs(consumes, kinds, 'matrix')
        fill_food(consumes, kinds, 'matrix')

    if saved:
        dps = any(strategy in DPS_STRATEGIES[0] | DPS_STRATEGIES.get(member.class_id, set())
                  for strategy in strategies['co'] + strategies['nc'] + strategies['dead'])
    else:
        dps = is_dps_by_spec(member.class_id, tab, spells)
    mana = member.class_id not in NO_MANA_CLASSES and not skips_mana_potions(
        member.class_id, member.level, tab, spells, knows_shadowfiend)
    if (saved and 'potions' not in strategies['co']) or not (dps or mana):
        consumes['defaultPotion'] = consume(empty['defaultPotion'], 'rules')
    else:
        offensive = [item for item in bags if item['id'] in BOT_OFFENSIVE_POTIONS] if dps else []
        drinks = offensive or ([item for item in bags if item['subclass'] in (1, 3) and energizes(item)] if mana else [])
        if drinks:
            picked = min(drinks, key=lambda item: (*bot_rank(item), item['id']))
            known = consumable_items.get(picked['id'])
            consumes['defaultPotion'] = consume(known[1] if known and known[0] == 'defaultPotion' else None,
                                                item=picked['id'])
        else:
            consumes['defaultPotion'] = consume(empty['defaultPotion'])
    for field in BOT_NEVER:
        consumes[field] = consume(empty[field], 'rules')
    return consumes


def energizes(item):
    """mod-playerbots' mana potion test: an energize effect in the item's spells, up to the first empty one."""
    for spell in item['spells']:
        if not spell:
            return False
        if spell in energize_spells:
            return True
    return False


def pet_entry(member, pet, pet_spells):
    family_name, talent_type = families.get(pet['family'], ('', -1))
    entry = {'name': pet['name'], 'family': pet['family'], 'familyName': family_name}
    key = family_name.replace(' ', '').lower()
    if member.class_id == WARLOCK:
        summon = next((name for name in summons[1:] if name.lower() == key), None)
        if summon:
            entry['summon'] = summon
        return entry
    pet_type = next((name for name in pet_types[1:] if name.lower() == key), None)
    if not pet_type:
        return entry
    entry['petType'] = pet_type
    tab = min((tab for tab, (_, mask, _, _) in talent_tabs.items() if talent_type >= 0 and mask == 1 << talent_type),
              default=None)
    if tab is None:
        return entry
    tree = json.load(open(f'{args.trees}/hunter_{talent_tabs[tab][3].lower()}.json'))[0]['talents']
    ranks = [0] * len(tree)
    for spell in pet_spells:
        for talent_tab, row, col, rank in pet_talents.get(spell, ()):
            index = next((i for i, talent in enumerate(tree)
                          if (talent['location']['rowIdx'], talent['location']['colIdx']) == (row, col)), None)
            if talent_tab == tab and index is not None:
                ranks[index] = max(ranks[index], rank)
    entry['talents'] = ''.join(map(str, ranks)).rstrip('0')
    return entry


def ammo_entry(ammo_id, dps, ranged_id):
    if not ammo_id:
        # Thori'dal makes its own arrows, so acraid leaves the player's ammo as it is
        return None if ranged_id == THORIDAL else {'itemId': 0, 'dps': 0, 'value': 'AmmoNone'}
    entry = {'itemId': ammo_id, 'dps': dps}
    value = next((value for value, known in ammo_dps if abs(known - dps) < 0.01), None)
    if value:
        entry['value'] = value
    return entry


def same_ammo(got, want):
    # both sides average the DPS from item_template floats, so it gets ammo_entry's tolerance
    if got is None or want is None:
        return got == want
    return {**got, 'dps': 0} == {**want, 'dps': 0} and abs(got.get('dps', 0) - want['dps']) < 0.01


parser = argparse.ArgumentParser()
parser.add_argument('roster', help='the JSON acraid wrote')
parser.add_argument('--dbc', required=True, help='directory holding the server DBC files')
parser.add_argument('--leader', help='the character acraid was run with')
parser.add_argument('--names', help='comma separated names, when acraid was run with -names')
parser.add_argument('--sim-db', default='assets/database/db.json', help='what acraid was run with as -simDb')
parser.add_argument('--min-skill', type=int, default=1, help='what acraid was run with as -minSkill')
parser.add_argument('--world-buff-matrix', help='what acraid was run with as -worldBuffMatrix')
parser.add_argument('--players', help='what acraid was run with as -players')
parser.add_argument('--trees', default='ui/core/talents/trees', help='what acraid was run with as -trees')
parser.add_argument('--consumables', default='ui/core/components/inputs/consumables.ts')
parser.add_argument('--hunter-go', default='sim/hunter/hunter.go')
parser.add_argument('--proto', default='proto')
parser.add_argument('--container', default='ac-database')
parser.add_argument('--user', default='root')
parser.add_argument('--password', default='password')
args = parser.parse_args()
if not args.leader and not args.names:
    parser.error('pass --leader or --names, whichever acraid used')

gem_items = {row[0]: row[33] for row in dbc_rows('SpellItemEnchantment.dbc') if row[33]}
glyphs = {row[0]: (row[1], row[2] & 1) for row in dbc_rows('GlyphProperties.dbc')}
sim_items = {item['id']: item.get('stats') or [] for item in json.load(open(args.sim_db))['items']}

tab_rows, tab_text = dbc_table('TalentTab.dbc', (0, 1, 20, 21, 22))
talent_tabs = {tab: (class_mask, pet_mask, page, tab_text(name)) for tab, name, class_mask, pet_mask, page in tab_rows}
talent_ranks, pet_talents = {}, collections.defaultdict(list)
for talent in dbc_rows('Talent.dbc'):
    for rank, spell in enumerate(talent[4:9], 1):
        if spell and talent_tabs.get(talent[1], (0, 0))[1]:
            pet_talents[spell].append((talent[1], talent[2], talent[3], rank))
        elif spell:
            talent_ranks[spell] = (talent[1], rank)
family_rows, family_text = dbc_table('CreatureFamily.dbc', (0, 8, 10))
families = {family: (family_text(name), signed(talent_type)) for family, talent_type, name in family_rows}
spell_rows, spell_text = dbc_table('Spell.dbc', (0, 71, 72, 73, 116, 117, 118, 136))
spell_names = {row[0]: spell_text(row[7]) for row in spell_rows}
spell_triggers = {row[0]: row[4:7] for row in spell_rows}
energize_spells = {row[0] for row in spell_rows if 30 in row[1:4]}
well_fed = {spell for spell, name in spell_names.items() if name == 'Well Fed'} | set(FEAST_BUFFS.values())
pet_types = enum_values('hunter.proto', 'PetType')
summons = enum_values('warlock.proto', 'Summon')
consumable_items, empty, prepop_values = sim_consumables()
explosive_order = enum_values('common.proto', 'Explosive')[1:]
ammo_dps = sim_ammo()
matrix = world_buffs() if args.world_buff_matrix else []
players = {name.strip().lower() for name in (args.players or '').split(',') if name.strip()}

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
active_talents = by_guid("""SELECT t.guid, t.spell FROM acore_characters.character_talent t
                            JOIN acore_characters.characters c ON c.guid = t.guid
                            WHERE t.specMask & (1 << c.activeTalentGroup) AND t.guid IN (%s)""", guids)
ammo = by_guid("""SELECT c.guid, c.ammoId, it.dmg_min1, it.dmg_max1 FROM acore_characters.characters c
                  LEFT JOIN acore_world.item_template it ON it.entry = c.ammoId WHERE c.guid IN (%s)""", guids)
pets = by_guid("""SELECT p.owner, p.id, p.name, COALESCE(ct.family, 0) FROM acore_characters.character_pet p
                  LEFT JOIN acore_world.creature_template ct ON ct.entry = p.entry
                  WHERE p.slot = 0 AND p.owner IN (%s) ORDER BY p.id""", guids)
pet_spell_rows = by_guid("""SELECT p.owner, ps.guid, ps.spell FROM acore_characters.pet_spell ps
                            JOIN acore_characters.character_pet p ON p.id = ps.guid
                            WHERE p.slot = 0 AND p.owner IN (%s)""", guids)
auras = by_guid('SELECT guid, spell FROM acore_characters.character_aura WHERE guid IN (%s)', guids)
# the backpack's 16 slots and whatever sits in the 4 bags, never the bank
bag_rows = by_guid("""SELECT ci.guid, it.entry, it.subclass, it.Quality, it.ItemLevel,
                        it.spellid_1, it.spellid_2, it.spellid_3, it.spellid_4, it.spellid_5
                      FROM acore_characters.character_inventory ci
                      JOIN acore_characters.item_instance ii ON ii.guid = ci.item
                      JOIN acore_world.item_template it ON it.entry = ii.itemEntry
                      WHERE it.class = 0 AND ci.guid IN (%s) AND (ci.slot BETWEEN 23 AND 38 AND ci.bag = 0
                        OR ci.bag IN (SELECT held.item FROM acore_characters.character_inventory held
                                      WHERE held.guid = ci.guid AND held.bag = 0 AND held.slot BETWEEN 19 AND 22))""",
                   guids)
shadowfiends = by_guid(f'SELECT guid, spell FROM acore_characters.character_spell WHERE spell = {SHADOWFIEND} '
                       'AND guid IN (%s)', guids)
bot_store = by_guid('SELECT guid, `key`, value FROM acore_playerbots.playerbots_db_store WHERE guid IN (%s)', guids)
random_bots = by_guid("""SELECT c.guid, t.account_type FROM acore_characters.characters c
                         JOIN acore_playerbots.playerbots_account_type t ON t.account_id = c.account
                         WHERE t.account_type = 1 AND c.guid IN (%s)""", guids)
# any flask's or elixir's buff -> (the lowest item giving it, its subclass)
buff_items = {}
for spell, item, subclass in sorted(sql("""SELECT spellid_1, entry, subclass FROM acore_world.item_template
                                           WHERE class = 0 AND subclass IN (2, 3) AND spellid_1 <> 0"""),
                                    key=lambda row: int(row[1])):
    buff_items.setdefault(int(spell), (int(item), int(subclass)))
# the sim's flasks, elixirs and food -> the buff each leaves: a flask's or elixir's own spell, what a
# food's spell triggers, or the feast table's buff
sim_buffs = {}
for item, spell in sql('SELECT entry, spellid_1 FROM acore_world.item_template WHERE entry IN (%s)' %
                       ', '.join(str(item) for item, known in consumable_items.items() if known[0] in BUFF_FIELDS)):
    item, spell = int(item), int(spell)
    buff = FEAST_BUFFS.get(item) or (food_buff(spell) if consumable_items[item][0] == 'food' else spell)
    if buff:
        sim_buffs[buff] = item

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

    strategies = None
    for key, value in bot_store[member.guid]:
        if key in ('co', 'nc', 'dead'):
            strategies = strategies or {'co': [], 'nc': [], 'dead': []}
            strategies[key] = [strategy.strip().lstrip('+') for strategy in value.split(',') if strategy.strip()]
    bot = bool(bot_store[member.guid] or random_bots[member.guid])
    if args.players:
        bot = name.lower() not in players
    if character.get('bot') != bot:
        problems.append(f'{name}.bot = {character.get("bot")}, server says {bot}')

    want_pet = None
    if member.class_id in (HUNTER, WARLOCK) and pets[member.guid]:
        pet_id, pet_name, family = pets[member.guid][0]
        spells = [int(spell) for owner_pet, spell in pet_spell_rows[member.guid] if owner_pet == pet_id]
        want_pet = pet_entry(member, {'name': pet_name, 'family': int(family)}, spells)
    if character.get('pet') != want_pet:
        problems.append(f'{name}.pet = {character.get("pet")}, server {want_pet}')

    want_ammo = None
    if member.class_id == HUNTER:
        ammo_id, low, high = ammo[member.guid][0]
        dps = (float(low) + float(high)) / 2 if low != 'NULL' else 0
        want_ammo = ammo_entry(int(ammo_id), dps, want_gear.get(RANGED_SLOT, {}).get('id'))
    if not same_ammo(character.get('ammo'), want_ammo):
        problems.append(f'{name}.ammo = {character.get("ammo")}, server {want_ammo}')

    bags = [{'id': int(row[0]), 'subclass': int(row[1]), 'quality': int(row[2]), 'level': int(row[3]),
             'spells': [int(spell) for spell in row[4:9]]} for row in bag_rows[member.guid]]
    bags = list({item['id']: item for item in bags}.values())
    spells = [int(row[0]) for row in active_talents[member.guid]]
    if bot:
        want_consumes = bot_consumes(member, spells, strategies, bags, bool(shadowfiends[member.guid]))
    else:
        want_consumes = player_consumes(member, [int(row[0]) for row in auras[member.guid]], bags)
    got_consumes = character.get('consumes') or {}
    for field in sorted(set(got_consumes) | set(want_consumes)):
        if got_consumes.get(field) != want_consumes.get(field):
            problems.append(f'{name} {field}: export {got_consumes.get(field)}, server {want_consumes.get(field)}')

print(f'checked {len(members)} characters, {items} items')
print('\n'.join(problems) if problems else 'no differences')
sys.exit(1 if problems else 0)
