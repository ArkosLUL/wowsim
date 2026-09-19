# gen_serverdata

Generates `sim/core/serverdata`'s tables from the committed spell capture
(`assets/db_inputs/acore/spelldump.jsonl`) and the live world DB. Runs offline apart from the DB, which
it only SELECTs from.

```bash
tools/acore/dock.sh run ./tools/acore/gen_serverdata
```

| File | Rows |
|---|---|
| `spells_auto_gen.go` | `Spell`: SpellInfo from the capture, plus `Flags` worked out as the server reads the attributes |
| `procs_auto_gen.go` | `Proc`: `spell_proc` resolved like `SpellMgr::LoadSpellProcs` (a negative SpellId covers its rank chain and beats a rank's own row; Spell.dbc fills unset ProcFlags, Charges and Chance), or the entry the server builds for a proc aura without a row, which only the capture has |
| `bonus_auto_gen.go` | `Bonus`: `spell_bonus_data`, the spell's row else its first rank's |
| `enchant_procs_auto_gen.go` | `EnchantProc`: every `spell_enchant_proc_data` row |

The spells, procs and bonuses cover what `tools/acore/spellids` counts as sim literals plus the spells
they trigger, not the whole capture: the tables ship in the wasm binary. A constant passed to `SpellByID`,
`ProcBySpellID` or `BonusBySpellID` counts too: their parameter is named `spellID`. Ids missing from the capture are
logged; see the [spellids README](../spellids/README.md#capturing) to capture them.

The capture already holds each spell's resolved proc entry and bonus data, so every row resolved from the
DB is checked against it. A disagreement means the DB changed since the server loaded it or since the
capture: the tool lists them all and writes nothing. Recapture, restarting the server first if it hasn't
loaded the change.

| Flag | Default |
|---|---|
| `-dump` | `assets/db_inputs/acore/spelldump.jsonl` |
| `-dsn` | `$AC_DSN`, else the live world DB on `host.docker.internal` |
| `-out` | `sim/core/serverdata` |

It imports `sim/core/serverdata` for the row types, so that package must compile first. To change a
table's shape, change `types.go`, hand-edit the generated files to match, then regenerate.
