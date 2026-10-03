# simval fixture

`simval.jsonl` is a capture of mod-sim-validation's e2e records. `tools/simval`'s `TestAgainstServerRecords`
replays it through `CheckRecord` on every `go test`, and `int-check.sh` runs it too. Recapture it
(never append) per [testing](../../../../docs/guide/testing.md#against-the-server);
`TestFixtureHasEveryE2ERecord` lists what a full run records.

`damage.jsonl` holds `TestSimvalDamage`'s `damage` records, which `TestDamageRecords` replays
([tools/simval](../../../../tools/simval/README.md#damage-probe)).

`timing.jsonl` holds `TestSimvalTiming`'s `timing` records and its bots' `info` records (regen), which
`TestTimingRecords` replays ([tools/simval](../../../../tools/simval/README.md#timing-probe)).

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
| MageFire | improvedScorch | fire spell crit % | the sim gives Fireball and Frostfire Bolt the crit per spell; the server sheet shows school auras only |
| MageFire | livingBomb | fire spell crit % | the sim's school crit is its lowest fire spell's bonus, and the new Living Bomb spell has a lower one |
| WarlockDestruction | devastation | fire spell crit % | spell specific crit in the sim, a spell mod on the server |
| RogueCombat | maceSpecialization | armor penetration % | the server counts the aura without a mace, and the path carries two daggers; the sim adds it only with a mace in the proc mask |
| DeathknightBlood | bloodGorged | armor penetration % | the sim keeps it as a health-gated aura, not in its armor penetration |
| DeathknightFrost | frigidDreadplate | ranged miss taken % | the server aura is melee only; the sim lowers all three attacks |
| MageFrost | arcticWinds | mainhand, offhand and ranged miss taken % | the sim models only the frost damage, not the hit taken |

`timing.jsonl`, each in `knownTimingMismatches` (`tools/simval/timing.go`, keyed by class, spell, pass and check name;
passes are bare, talents and Bloodlust). The record stays and the check still runs; its failure is let through in the
passes named, and an entry no record fails is stale. `droppedTimingRecords` is empty.

| Class | Spell | Check | Passes | Why |
|---|---|---|---|---|
| Paladin | 31884 Avenging Wrath | cost | talents, Bloodlust | the sim costs 351, the server 315: the sim misses a discount |
| Paladin, Priest, Druid, Hunter, Shaman | 48801, 48819, 48066, 48071, 48463, 48465, 49001, 49050, 49231, 49271 | cost | talents, Bloodlust | the sim's cost is 1 above the server's |
| Shaman | 49273 Healing Wave | cost | all | the sim applies a discount of about 40% (659) the server doesn't (1099) |
| Mage, Warlock, Druid, Shaman | 42846 Arcane Missiles, 47855 Drain Soul, 48467 Hurricane, 49233 Flame Shock | channel, aura duration | Bloodlust | the sim truncates the tick period to whole ms, so 1 to 4 ms short |
| Rogue | 48672 Rupture, 6774 Slice and Dice | aura duration, e0 ticks | all | the probe has no combo points; the sim's finisher has no duration (Rupture) or its 5-point one (Slice and Dice) |
| Hunter | 49001, 49050, 49052, 63672 | ranged speed (and Steady Shot's cast time) | all | the bare server hunter has 10% ranged haste (0.9091) the sim hunter lacks |

## Spells missing from serverdata

`critCapable` assumes they crit, and `rangedWeaponSpell` that they're ranged weapon spells.

| Spell | Why |
|---|---|
| 75 Auto Shot | not in `sim/core/serverdata` |
