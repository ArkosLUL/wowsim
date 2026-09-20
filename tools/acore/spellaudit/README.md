# spellaudit

Exports what the live server knows about every spell the sim names, so a class work item can review
its spells against the server instead of against Classic research. It joins the sim's spell-id scan
(the one [spellids](../spellids/README.md) does), the committed `.simval spelldump` capture and the
generated `sim/core/serverdata` tables. Offline: no server, no DB.

```bash
tools/acore/dock.sh run ./tools/acore/spellaudit [-area hunter]
```

| Flag | Default |
|---|---|
| `-root` | `.`, the checkout to scan |
| `-dump` | `assets/db_inputs/acore/spelldump.jsonl` |
| `-out` | `docs/azerothcore-parity/audit` |
| `-area` | all; one sim area, e.g. `hunter`, `core`, `common/wotlk` |

`area` comes from the source path a spell is named in (`sim/<class>/…`, `sim/common/<x>/…`), so a class
item can filter to its own rows. A spell named from several areas carries them all.

## Output

| File | Rows |
|---|---|
| `spells_audit.csv` | every sim-named spell in the capture: family, damage class, school, cast, GCD, cooldowns, duration, stacks, tick interval, the P3-2/P3-3 flags (binary, channeled, ranged slot, auto-repeat, resets swing, haste GCD, haste periodic, no active defense, always hit), proc PPM/chance/ICD, what it triggers, its server script, and up to three places the sim names it |
| `procs_audit.csv` | the subset with a `spell_proc` entry: PPM, chance, ICD, charges, the proc/type/phase/hit masks, and the `PROC_ATTR_*` bits spelled out |
| `spells_missing.csv` | sim-named ids the capture lacks, positions the scan couldn't resolve to a constant, and type errors that cost it constants |

A boolean cell is `yes` or `no`; empty means the generated tables don't cover that spell, never "no".
An id in `spells_missing.csv` as "not captured" means the sim asks the server for a spell it has never
seen — capture it (spellids README) and rerun `gen_serverdata`.
