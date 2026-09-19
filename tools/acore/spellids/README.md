# spellids

Lists the spell ids the sim needs server data for, checks them against the committed spell capture
(`assets/db_inputs/acore/spelldump.jsonl`), and writes the missing ones as an ids file for
`.simval spelldump ids`.

```bash
tools/acore/dock.sh run ./tools/acore/spellids [-v] [-out tmp/capture/ids.txt [-all]]
```

## Sources

| Source | Ids |
|---|---|
| sim literals | integer constants in non-test `sim/**` (except `sim/core/proto`, `sim/core/serverdata`, `sim/web`) that end up in something named like a spell or aura id (`SpellID`, `AuraID`, `spellId`, `auraIDs`, …): struct fields, variables and constants, the called function's parameters, returns of a function so named, `==`/`!=`, switch cases. Named constants, `TernaryInt32`-style calls and `[]int32{…}[i]` resolve; anything else counts as unresolved (`-v` lists them) |
| items | on-use, on-equip and chance-on-hit spells (`item_template.spellid_1..5` with trigger 0, 1, 2 or 5) of db.json's items and of the items the sim names by id (consumables, explosives) |
| item sets | ItemSet.dbc bonuses of those items' sets |
| enchants and gems | combat, equip and use spells of db.json's enchants and gems (SpellItemEnchantment.dbc, gems through GemProperties.dbc) |
| talents, glyphs | every Talent.dbc rank and GlyphProperties.dbc spell |
| triggered | what the above trigger, transitively, through the capture's `effects[].triggerSpell` |

`gen_serverdata` generates only the sim literals and what they trigger; the rest is captured for P7's audits.

Per source it counts ids in the capture, **missing** (in Spell.dbc or `spell_dbc`, not captured) and
**unknown** (in neither, so the server lacks it). It also counts the capture per SpellFamilyName.

| Flag | Default |
|---|---|
| `-dump` | `assets/db_inputs/acore/spelldump.jsonl` |
| `-db` | `assets/database/db.json` |
| `-dbc` | `/dbc/Clean`; its SpellItemEnchantment, GemProperties, ItemSet, Talent, TalentTab and GlyphProperties match the live server's |
| `-dsn` | `$AC_DSN`, else the live world DB on `host.docker.internal` (SELECT only) |
| `-out` | none; writes ids the capture lacks, one per line with a `#` comment |
| `-all` | with `-out`, every id |

## Capturing

After adding a spell id to `sim/`, rerun spellids. If it's missing, capture it, then rerun
`gen_serverdata`. Capturing needs the live server and the wave loop's server lock.

`.simval spelldump` overwrites `spelldump.jsonl` on every run, so the module's
`e2e/spelldump_capture_test.go` (`TestSpelldumpCapture`) runs the dumps one at a time as a GM bot and merges
each into one file, keyed by spell id and sorted. It puts the server's `spelldump.jsonl` back and removes
its copy of the ids file. It's skipped unless `SIMVAL_SPELLDUMP_OUT` is set:

- `SIMVAL_SPELLDUMP_OUT`: the merged capture, extended when it exists
- `SIMVAL_SPELLDUMP_FAMILIES`: comma-separated SpellFamilyNames
- `SIMVAL_SPELLDUMP_IDS`: an ids file from `-out`

`run.sh` passes none of these, so run its `docker run` line by hand with them added and the capture
directory mounted, e.g. `-v <worktree>/tmp/capture:/capture`, then
`go test -tags=e2e -run TestSpelldumpCapture -count=1 -v .`.

Repeat until spellids reports nothing missing:
1. `spellids -out tmp/capture/ids.txt` (`-all` on a first capture).
2. Run the driver with `SIMVAL_SPELLDUMP_OUT=/capture/spelldump.jsonl`, `SIMVAL_SPELLDUMP_IDS=/capture/ids.txt`
   and, on a full recapture, `SIMVAL_SPELLDUMP_FAMILIES=3,4,5,6,7,8,9,10,11,15,17` (the ten classes and pets).
   To add to the committed capture instead, copy it to `tmp/capture/spelldump.jsonl` first.
3. Copy the merged file over `assets/db_inputs/acore/spelldump.jsonl`. Triggers only show up once their
   trigger is captured, hence the repeat.

Each dump stalls the world thread for at most about 200 ms. Two ids stay unknown: 26545 (from Lightning Shield) and
53258 (from Empower Rune Weapon).
