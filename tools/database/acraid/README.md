# acraid

Exports a raid group's characters from the live AzerothCore server into a roster JSON. That covers gear
(with enchants, gems and reforges), talents, glyphs, professions and racial trait swaps. It only runs
SELECTs.

This is the CLI. The logic lives in `tools/database/azerothcore/`:
- `characters.go`: queries
- `roster.go`: conversion and the JSON shape
- `roster_dbc.go`: DBC lookups

The sim UI can't import the roster yet.

## Before exporting

Gear, talents and glyphs reach the database only when a character saves. Run `saveall` in the
worldserver console first.

## Running

Go isn't installed on the host, so the tool runs in the toolchain image. Generate the protos first if
`sim/core/proto/*.pb.go` is missing (see `docs/guide/dev-environment.md`), and mount the checkout you're
working in. The image has no Docker CLI, so copy the DBC files out on the host first. From Git Bash, with
`SCRATCH` set to a Windows-style path such as `C:/Users/me/tmp`:

```sh
mkdir -p "$SCRATCH/dbc"
for f in Talent.dbc TalentTab.dbc GlyphProperties.dbc SpellItemEnchantment.dbc; do
  MSYS_NO_PATHCONV=1 docker cp "ac-worldserver:/azerothcore/env/dist/data/dbc/$f" "$SCRATCH/dbc/$f"
done
MSYS_NO_PATHCONV=1 docker run --rm --network azerothcore-wotlk-pb_ac-network \
  -v G:/DevStuff/GitHub/wowsimwotlk:/wotlk -v "$SCRATCH:/scratch" \
  -v wotlk-gomod:/go/pkg/mod -v wotlk-gocache:/root/.cache/go-build -w /wotlk wowsims-wotlk-dev \
  go run ./tools/database/acraid -dsn "root:password@tcp(ac-database:3306)/" \
  -dbcDir /scratch/dbc -leader Deathsong -out /scratch/raid.json
```

It prints one summary line per character, plus a `!` line for each warning.

## Flags

| Flag | Default | Meaning |
|---|---|---|
| `-out` | required | the roster JSON to write |
| `-leader` | | export the group this character is in, keeping its subgroups. Fails with "isn't in a group" when there's no group; use `-names` then |
| `-names` | | comma-separated characters. Alone, they fill subgroups of 5 in order. With `-leader`, they top up the group |
| `-dsn` | `root:password@tcp(127.0.0.1:3306)/` | no database name: the queries name `acore_characters` and `acore_world` themselves |
| `-dbcDir` | empty: copy from `-acContainer` | needs Talent, TalentTab, GlyphProperties and SpellItemEnchantment |
| `-acContainer` | `ac-worldserver` | where DBCs are copied from, with `docker cp` |
| `-trees` | `ui/core/talents/trees` | the sim's talent tree layouts |
| `-simDb` | `assets/database/db.json` | decides whether a reforge does anything |
| `-minSkill` | `1` | skill level at which a profession counts as known |

Keep `-minSkill` at 1 on this server. Profession bonuses (Toughness, Master of Anatomy) are granted by GM
command or shared per account, so the skill value says nothing about who has them.

## Output (version 1)

| Level | Fields |
|---|---|
| Top level | `version`, `exportedAt`, `group` (`selector`, `leader`, `leaderIsGroupLeader`, `names`), `warnings`, `characters` |
| Each character | `name`, `classId`, `raceId`, `swapRaceId`, `level`, `subgroup`, `memberFlags`, `talents`, `glyphs` (`major` and `minor` spell ids), `professions`, `gear`, `quiver`, `warnings` |
| Each gear entry | `acSlot`, `id`, `enchant`, `gems`, `extraGem`, `reforge` |

- `reforge` is `{fromStatType, toStatType}`, readable with `ItemReforge.fromJson`.
- `quiver` is true when a quiver or ammo pouch sits in one of the character's 4 bag slots. Only hunters
  use it.
- Only the active talent spec is exported.
- Herbalism is exported as a profession even when the character doesn't know Lifeblood (55503), and
  the sim grants Lifeblood to every herbalist.

## Cross-check

`crosscheck.py` rebuilds the export independently in Python and diffs it. It reads the SQL through
`docker exec ac-database mysql`, plus the DBC files and the sim DB. It exits 1 on any difference. Pass it
the same `--names`, `--min-skill` and `--sim-db` as the export.

```sh
python tools/database/acraid/crosscheck.py "$SCRATCH/raid.json" --dbc "$SCRATCH/dbc" --leader Deathsong
python tools/database/acraid/crosscheck_test.py   # the checker's own tests, against a stubbed server
```
