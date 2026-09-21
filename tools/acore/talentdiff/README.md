# talentdiff

Compares the talent and glyph client tables of two DBC sets. The sim keeps stock tree positions and
the raid importer maps talents by them, so a talent the user's client moved — or a talent or glyph
spell it changed — has to be a decision rather than a surprise.

```bash
tools/acore/dock.sh run ./tools/acore/talentdiff
DBC_DIR=<dir holding both sets> tools/acore/dock.sh run ./tools/acore/talentdiff -a /dbc/Clean -b /dbc/live -blabel live
```

| Flag | Default |
|---|---|
| `-a`, `-b` | `/dbc/Clean`, `/dbc/Changed` |
| `-alabel`, `-blabel` | `clean`, `changed`; the two value column names |
| `-out` | `docs/azerothcore-parity/audit/talents_diff.csv` |
| `-spells` | `true`; also compare the Spell.dbc rows of every talent and glyph spell |

dock.sh mounts one DBC directory, so both sides must sit under it. The default `A:\WOW\dbc` holds
`Clean` and `Changed`; to bring the live copy in, put it beside them and point `DBC_DIR` at the parent.

It reads DBC files only, but the server lays mod-spell-tweaks' `spell_dbc` and `talent_dbc` rows over them. For
the value the server uses, read the spelldump (`assets/db_inputs/acore/spelldump.jsonl`) or the live DB: Rage of
Rivendare, Virulence and Nerves of Cold Steel differ there while the live DBC files are stock.

## Output

One CSV row per difference: `table`, `id`, `change` (added, removed, changed), `field`, both values,
both values read as strings, and a context column that locates a talent in its tree or names a spell.
`table` is `Talent.dbc`, `TalentTab.dbc`, `GlyphProperties.dbc`, `talent_spell` or `glyph_spell`.

Records are compared field by field rather than through a struct, so a layout this tool got wrong
can't hide a difference; the named fields come from AzerothCore's `DBCStructure.h` and the rest read as
`field_<n>`. A string field holds an offset into its own file's string block, so the same text lands on
different numbers — a field whose numbers differ but whose strings are equal and non-empty is left out.
