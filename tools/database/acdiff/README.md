# acdiff

Compares the sim's item data and hardcoded Go effects with the live AzerothCore server. It covers item
levels and stats, obtainability, item/gem/enchant spell effects (values, cooldowns and PPM), set bonuses,
gems and enchants. It only runs SELECTs, and a run takes about 10 seconds. The findings, and what to do
about them, are in `docs/azerothcore-item-diff/azerothcore-item-diff.INVESTIGATION.md`.

The readers it uses live in `tools/database/azerothcore/`.

## Running

Go isn't installed on the host, so run it through `tools/acore/dock.sh`, which mounts the checkout it
lives in and the AzerothCore checkout at `/ac`. Point `DBC_DIR` at a copy of the live DBCs (the
[gen_db README](../gen_db/README.md) has the copy loop): dock.sh's default isn't the live copy.

```sh
DBC_DIR=<copy of the live DBCs> tools/acore/dock.sh run ./tools/database/acdiff -dbcDir /dbc -acRepo /ac
```

## Flags

| Flag | Default | Meaning |
|---|---|---|
| `-acRepo` | required | AzerothCore checkout, scanned for module and custom SQL that touches items or spells |
| `-dsn` | `$AC_DSN`, else the live world DB on `host.docker.internal` | world database |
| `-dbcDir` | empty: copy from `-acContainer`, host runs only | the server's DBC files |
| `-acContainer` | `ac-worldserver` | where DBCs are copied from, with `docker cp` |
| `-simDb`, `-leftoverDb` | `assets/database/db.json`, `assets/database/leftover_db.json` | sim item databases |
| `-spellTooltips` | `assets/db_inputs/wowhead_spell_tooltips.csv` | Classic spell tooltips |
| `-simSrc` | `sim` | Go sources searched for hardcoded effects and set bonuses |
| `-outDir` | `docs/azerothcore-item-diff/data` | where the reports are written |

## Output

Diffs read server → sim.

| File | Contents |
|---|---|
| `summary.md` | counts per category |
| `items_diff.csv` | item differences, with `category` set to classic, module or unobtainable |
| `items_not_comparable.csv` | heirlooms |
| `items_missing_on_server.csv`, `items_missing_in_sim.csv` | items only one side has |
| `items_unobtainable_on_server.csv` | sim items the server has but nothing awards |
| `effects_diff.csv` | hardcoded effect values vs server spell data |
| `sets_diff.csv`, `sets_issues.csv` | set bonuses |
| `gems_diff.csv`, `enchants_diff.csv` | gem and enchant stats |

`effects_review.csv` and `sets_review.csv` hold hand-reviewed verdicts: real, false_positive or
not_modeled. The tool doesn't write them. Update them by hand after a re-run, so reviewed rows don't get
reviewed twice.

## Limits

Effect matching is heuristic. It misses:
- values computed in helper functions (Darkmoon Card: Greatness)
- spells applied by scripts or area auras (Val'anyr, Shifting Naaru Sliver)
- internal cooldowns that only exist in Go
- coincidental number matches (Ashen Band's 10% chance vs its 10 s duration)

Effects that are only registered per character (death knight items) or checked by item id are still
compared. They're marked `(not registered)`.
