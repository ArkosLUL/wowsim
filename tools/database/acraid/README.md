# acraid

Exports a raid group's characters from the live AzerothCore server into a roster JSON. That covers gear
(with enchants, gems and reforges), talents, glyphs, professions, racial trait swaps, pets, ammo and
consumables. It only runs SELECTs.

This is the CLI. The logic lives in `tools/database/azerothcore/`:
- `characters.go`: queries
- `roster.go`: conversion and the JSON shape
- `roster_dbc.go`: DBC lookups

The sim imports it under Import → AzerothCore.

## Before exporting

Gear, talents and glyphs reach the database only when a character saves. Run `saveall` in the
worldserver console first.

## Running

Go isn't installed on the host, so the tool runs in the toolchain container through `tools/acore/dock.sh`
(see `docs/guide/dev-environment.md`; generate the protos first if `sim/core/proto/*.pb.go` is missing).
The container has no Docker CLI, so copy the DBC files out on the host first; with the worldserver
stopped, use an existing copy. From Git Bash in the checkout, with `SCRATCH` set to a Windows-style path
such as `C:/Users/me/tmp`:

```sh
mkdir -p "$SCRATCH/dbc"
for f in Talent TalentTab GlyphProperties SpellItemEnchantment CreatureFamily Spell; do
  MSYS_NO_PATHCONV=1 docker cp "ac-worldserver:/azerothcore/env/dist/data/dbc/$f.dbc" "$SCRATCH/dbc/$f.dbc"
done
DBC_DIR="$SCRATCH/dbc" bash tools/acore/dock.sh run ./tools/database/acraid \
  -dsn "root:password@tcp(host.docker.internal:3306)/" -dbcDir /dbc -leader Deathsong \
  -worldBuffMatrix /ac/configurationOverrides/Playerbot.env -out tmp/raid.json
```

dock.sh mounts `DBC_DIR` at `/dbc` and the AzerothCore checkout at `/ac`. It prints one summary line per
character, plus a `!` line for each warning.

## Flags

| Flag | Default | Meaning |
|---|---|---|
| `-out` | required | the roster JSON to write |
| `-leader` | | export the group this character is in, keeping its subgroups. Fails with "isn't in a group" when there's no group; use `-names` then |
| `-names` | | comma-separated characters. Alone, they fill subgroups of 5 in order. With `-leader`, they top up the group |
| `-worldBuffMatrix` | empty: bots' flask, elixirs and food stay as they are | the bots' `AiPlayerbot.WorldBuffMatrix`: a `Playerbot.env`, a `playerbots.conf` or the bare matrix. The live one is the override in `[ac]/configurationOverrides/Playerbot.env` |
| `-players` | empty: read off mod-playerbots | comma-separated characters someone plays; everyone else counts as a bot |
| `-dsn` | `root:password@tcp(127.0.0.1:3306)/` | no database name: the queries name `acore_characters`, `acore_world` and `acore_playerbots` themselves |
| `-dbcDir` | empty: copy from `-acContainer` | needs Talent, TalentTab, GlyphProperties, SpellItemEnchantment, CreatureFamily and Spell |
| `-acContainer` | `ac-worldserver` | where DBCs are copied from, with `docker cp` |
| `-trees` | `ui/core/talents/trees` | the sim's talent tree layouts, hunter pet trees included |
| `-simDb` | `assets/database/db.json` | decides whether a reforge does anything |
| `-minSkill` | `1` | skill level at which a profession counts as known |

Keep `-minSkill` at 1 on this server. Profession bonuses (Toughness, Master of Anatomy) are granted by GM
command or shared per account, so the skill value says nothing about who has them.

## Output (version 2)

| Level | Fields |
|---|---|
| Top level | `version`, `exportedAt`, `group` (`selector`, `leader`, `leaderIsGroupLeader`, `names`, `players`), `warnings`, `characters` |
| Each character | `name`, `classId`, `raceId`, `swapRaceId`, `level`, `subgroup`, `memberFlags`, `talents`, `glyphs` (`major` and `minor` spell ids), `professions`, `gear`, `quiver`, `bot`, `pet`, `ammo`, `consumes`, `warnings` |
| Each gear entry | `acSlot`, `id`, `enchant`, `gems`, `extraGem`, `reforge` |
| `pet` | `name`, `family`, `familyName`, then `petType` and `talents` for a hunter, `summon` for a warlock |
| `ammo` | `itemId`, `dps`, `value` |
| Each `consumes` entry | `value`, `source`, `itemId`, `spellId` |

- `reforge` is `{fromStatType, toStatType}`, readable with `ItemReforge.fromJson`.
- `quiver` is true when a quiver or ammo pouch sits in one of the character's 4 bag slots. Only hunters
  use it.
- Only the active talent spec is exported.
- Herbalism is exported as a profession even when the character doesn't know Lifeblood (55503), and
  the sim grants Lifeblood to every herbalist.
- `bot`: mod-playerbots saved strategies for the character (`playerbots_db_store`) or its account is a
  random-bot one (`playerbots_account_type` 1). `-players` overrides both.
- `pet` is the one out (`character_pet` slot 0), hunters and warlocks only. `talents` is the sim's pet
  talent string for the family's tree, read from `pet_spell`. `petType` and `summon` are
  `Hunter.Options.PetType` and `Warlock.Options.Summon` values.
- `ammo` is hunters only: `characters.ammoId` as the `Hunter.Options.Ammo` value of the same DPS.
- `consumes` is keyed by the `Consumes` proto JSON field names; `value` is that field's proto JSON value.
  `source` is `matrix` (the bot's world-buff matrix rows), `buffs` (saved auras), `bags` or `rules`
  (mod-playerbots never has a bot use it). `itemId` and `spellId` say what it came from.
  - Bots: flask, elixirs and food from the matrix; a potion from the bags, the offensive one when the
    bot is a DPS, else a mana potion when its class drinks them; nothing else.
  - Played characters: flask, elixirs and food from saved auras, else the bags; everything else from
    the bags. Pet scrolls aren't read.
  - A missing `value` means the server's item or buff has no sim value: the importer keeps the field
    as it is. A missing field does the same.
- Version 1 files have no `bot`, `pet`, `ammo` or `consumes`.

## Cross-check

`crosscheck.py` rebuilds the export independently in Python and diffs it. It reads the SQL through
`docker exec ac-database mysql`, plus the DBC files, the sim DB and the sim's own sources for consumable
items, ammo DPS and pet families. It exits 1 on any difference. Run it from the repo root, with the same
`--names`, `--min-skill`, `--sim-db`, `--world-buff-matrix` and `--players` as the export.

```sh
python tools/database/acraid/crosscheck.py tmp/raid.json --dbc "$SCRATCH/dbc" --leader Deathsong \
  --world-buff-matrix ../azerothcore-wotlk-pb/configurationOverrides/Playerbot.env
python tools/database/acraid/crosscheck_test.py   # the checker's own tests, against a stubbed server
```
