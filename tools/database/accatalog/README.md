# accatalog

Builds `assets/database/server_catalog.json`, the server's item catalog
([ADR 0005](../../../docs/adr/0005-server-item-catalog.md)): every obtainable equippable item and gem, with
its progression tier, sources, equip limits and faction. SELECTs only, about 10 s. The resolver is
`ResolveCatalog` in `tools/database/azerothcore/catalog_*.go`; its tier tables are in `catalog_rules.go`.

## Running

In the toolchain container, with a copy of the live DBCs mounted at `/dbc`
([copying them](../../../docs/guide/azerothcore-server.md#dbcs); `azerothcore.CatalogDBCFileNames` lists the
files):

```sh
DBC_DIR=<live DBC copy> tools/acore/dock.sh run ./tools/database/accatalog
```

A rerun with nothing changed leaves the file byte-identical, date included.

| Flag | Default | Meaning |
|---|---|---|
| `-dsn` | `$AC_DSN`, else the live world DB on `host.docker.internal` | |
| `-dbcDir` | `/dbc` | the server's DBCs |
| `-classicDb` | `assets/database/db.json` | Classic phases, for the fallback and the report |
| `-out` | `assets/database/server_catalog.json` | |
| `-date` | the old date when nothing else changed, else today | `YYYY-MM-DD` |
| `-explain <ids>` | | print every way to get each item, catalog or not, with tiers; writes nothing |
| `-holders` | | list every unplaced loot holder, not just 12 |

## Output

- Protojson `ServerCatalog` laid out like `db.json`: one item or limit group per line, sorted by id, enum
  numbers, default values left out.
- Past 3 creatures, objects or vendors, a source names a count instead (`Northrend: 5 creatures`).
- Past 8 sources (3 below tier 13), sources merge by kind, tier and map, then by kind and tier.

The report on stdout:
- changes since the previous catalog
- Classic phase × catalog phase over `db.json`
- the BIS-catalog spec checks: Ulduar 10 and Onyxia source tiers, epic gems, dropped `db.json` items, T10
  through its Marks, Deathbringer's Will, ToC factions, Dragon's Eyes
- emblem tiers, fallback items, unobtainable and unresolved counts, unplaced loot holders

## How tiers resolve

Tier rules: [progression tiers](../../../docs/guide/azerothcore-server.md#progression-tiers).

- An item's tier is its lowest source's; a source's is the highest of what it needs, resolved recursively:
  - its map and difficulty
  - ScriptName `CanBeSeen` gates and IPP phase masks (in Northrend only the Argent Tournament, from 15)
  - conditions on quest `66000 + N`: loot rows (references included), vendor items, quest availability
  - the items it's bought with, opened from, prospected from, crafted from (reagents only, not recipes) or
    handed in for (mod-token-turnin)
  - for quest rewards: the starter, the previous quest, required items and kill targets
- Emblem tiers are floors: Sartharion's Satchel of Spoils gives a Triumph at 13, and Usuri Brightcoin
  trades Triumph down to Conquest.
- Holders are placed by spawns, encounter credits (DungeonEncounter.dbc; ICC and RS credits also cover
  their heroics), summon groups, SmartAI summons, transports (routed by TaxiPathNode.dbc) and
  `knownScriptSummons`, for what only C++ summons. Loot on a creature or difficulty entry nothing places
  counts as unresolved, not unobtainable, and lists under unplaced holders.
- A quest's required items that resolve nowhere (handed out by its scripts) don't count.
- PvP sources (honor, arena, PvP marks, battleground maps) count only when an item has no other. Those
  items, and any with resilience, are flagged `pvp`.
- Nothing resolves: 12 + the Classic phase, flagged `fallback_tier`; without a Classic phase the item is
  left out (unresolved). Items nothing awards are left out (unobtainable).
- mod-dungeon-master's random rewards come from `item_template` in C++, so no table read here holds them.

## Limits

- `knownScriptSummons` is kept by hand: extend it when `-holders` lists a holder that matters.
- Holiday bosses take their instance's tier (Coren Direbrew 0, Ahune 8); event windows aren't modeled.
  Their LFG reward boxes fall back.
- Blood elf and draenei starting zones are on Outland's map, so their loot reads tier 8.
