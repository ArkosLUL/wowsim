# simval fixture

`simval.jsonl` is a capture of mod-sim-validation's e2e records. `tools/simval`'s `TestAgainstServerRecords`
replays it through `CheckRecord` on every `go test`, and `int-check.sh` runs it too. Recapture it
(never append) per [testing](../../../../docs/guide/testing.md#against-the-server);
`TestFixtureHasEveryE2ERecord` lists what a full run records.

`damage.jsonl` holds `TestSimvalDamage`'s `damage` records, which `TestDamageRecords` replays
([tools/simval](../../../../tools/simval/README.md#damage-probe)).

`goldchar.jsonl` holds `TestSimvalGoldenCharacters`' `info` and `damage` records, which `TestGoldenCharacters`
replays against `goldchar_setups.json` ([tools/simval](../../../../tools/simval/README.md#golden-characters)).

A record of any of these files that disagrees with the sim is a real mismatch: it's dropped from the file and
listed here, and the sim isn't changed.

`implemented_talents.json` is the talent guard's export (`TestTalentCoverage`, `sim/talent_coverage_test.go`):
every talent `sim/<class>` reads, with class, tree, `fieldName`, row, max points and its spell per rank. The
guard fails when it's stale; rewrite it with `go test ./sim -run TestTalentCoverage -update`.

`talent_paths.json` holds the talent sweep's paths, generated and kept current by `tools/simval`
([talent sweep](../../../../tools/simval/README.md#talent-sweep)). `talents.jsonl` holds a `TestSimvalTalentSweep`
run trimmed by `simval talents`, which `TestTalentSweepRecords` replays.

`spell_groups.json` is a SELECT export of `acore_world.spell_group` and `spell_group_stack_rules`, which
`TestSimSpellGroups` (`sim/spell_groups_test.go`) compares with the sim's exclusive effects. Re-export it with the
query in that test's doc comment when the server's rules change.

## Dropped records

`goldchar.jsonl`, each in `droppedGoldchar` (`tools/simval/goldchar_test.go`). A named check keeps its record
and is only logged; the others are removed.

| Setup | Record | Check | Why |
|---|---|---|---|

`talents.jsonl`, each in `droppedSweep` (`tools/simval/talents_test.go`). Records stay; a dropped step (empty field)
or field is only logged.

| Path | Talent | Field | Why |
|---|---|---|---|

## Spells missing from serverdata

`critCapable` assumes they crit, and `rangedWeaponSpell` that they're ranged weapon spells.

| Spell | Why |
|---|---|
| 75 Auto Shot | not in `sim/core/serverdata` |
