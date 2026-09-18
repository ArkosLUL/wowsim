# gen_db

Builds the item DB (`assets/database/{db,leftover_db}.{json,bin}`) and scrapes its Classic inputs into
`assets/db_inputs/`. The `-gen` modes and their commands are listed at the top of `main.go`; `-gen=db` is
the Classic-only build.

## `-gen=azerothcore` (`make items`)

The `db` build, plus a pass that takes item data from the live server
([ADR 0002](../../../docs/adr/0002-item-data-from-live-db.md)). SELECTs only.

**Overwrites** the fields `azerothcore.ConvertItem` fills, zero values and empty lists included: ilvl,
quality, stats, sockets, socket bonus, weapon damage and speed, heroic, class allowlist, set name.
- Runs after `ItemOverrides`, so overrides of those fields only count in `-gen=db`; phases still come from
  them. Runs before the filters, so the `db`/`leftover_db` split uses server ilvls and qualities.
- Skipped, keeping Wowhead data: items missing on the server, items nothing awards there (placeholder rows,
  Classic-only sources), heirlooms, random-enchant items.
- Drops gems whose id is a non-gem item on the server, e.g. 33633 Forceful Earthstorm Diamond.

**Conversion choices** (`tools/database/azerothcore/convert.go`):
- Set names are ItemSet.dbc's, normalized like Wowhead's, except where Go set bonuses need otherwise:
  - All TBC arena seasons share one set per class, named like the WotLK season sets Go registers
    (`Gladiator's Pursuit`), so Merciless, Vengeful and Brutal pieces keep their season in the name.
  - 10-man T8 mage pieces are `Kirin'dor Garb`: `sim/mage` lists it as an `AlternativeName`, and with_db
    builds panic when no item carries one.
- Equip spells limited to shapeshift forms stay effects, not stats. Otherwise pre-3.0 feral attack power
  (mod-individual-progression's TBC staves, e.g. 30883) counts as attack power for every class; feral
  druids lose it instead.
- Relics get their class from the relic subclass, since the server leaves `AllowableClass` open.

**Run** it in the toolchain container. The image has no docker CLI, so copy the live DBCs out first:

```sh
dir=<host dir>
for f in Spell SpellDuration SpellItemEnchantment ItemSet GemProperties; do
  MSYS_NO_PATHCONV=1 docker cp ac-worldserver:/azerothcore/env/dist/data/dbc/$f.dbc "$dir/"
done
DBC_DIR="$dir" tools/acore/dock.sh exec make items AC_DBC_DIR=/dbc
```

| Flag | Default |
|---|---|
| `-dsn` | `$AC_DSN`, else the live world DB on `host.docker.internal` |
| `-dbcDir` | empty: `docker cp` from `-acContainer` (`ac-worldserver`), host runs only |

The log counts the items that took server data and lists dropped gems. Check the result with
[acdiff](../acdiff/README.md): `items_diff.csv` should hold only unobtainable rows.
