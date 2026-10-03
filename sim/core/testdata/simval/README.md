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

`spellmod_spells.json` lists, per spell-mod talent, the spells the sim names that its mods change on the server;
the spell-mod sweep probes them. Rewrite it with [spellmods](../../../../tools/acore/spellmods/README.md).
`spellmods.jsonl` holds a `TestSimvalSpellModSweep` run's `SPELLMOD` lines, which `TestSpellModRecords` replays
([spell-mod sweep](../../../../tools/simval/README.md#spell-mod-sweep)).

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

`spellmods.jsonl`, each in `droppedSpellMods` (`tools/simval/spellmods_test.go`). Lines stay; a dropped step (spell 0),
spell (empty check) or check is only logged.

| Path | Talent | Spell | Check | Why |
|---|---|---|---|---|
| MageArcane | arcaneFocus, arcanePower | 42921; 42833, 42842, 42846, 42891, 42897, 42945, 42950, 44781, 47610, 55360 | cost | the sim's cost is 1 above (Arcane Blast 2): the server truncates the base cost before the percent mods |
| PaladinProtection | benediction | 48801, 48819, 48827, 53595, 61411 | cost | the same 1 above |
| WarlockDemonology, WarlockAffliction | cataclysm, suppression | 17962, 47827; 47813, 47855, 47864, 47865, 50511 | cost | the same 1 above |
| ShamanElemental, ShamanEnhancement, ShamanRestoration | convection, mentalQuickness, shamanisticFocus, totemicFocus | 49231, 49236, 49271; 2825, 8143, 49233, 57722, 58643, 58704, 58734, 58753, 58774; 49233; 58734 | cost | the same 1 above |
| HunterMarksmanship, HunterSurvival | efficiency, resourcefulness | 34490, 49001, 49048, 49050, 63672; 63672 | cost | the same 1 above |
| PriestDiscipline, DruidBalance | improvedFlashHeal, mentalAgility, moonglow | 48071; 48068, 48089, 48158, 48300; 48463, 48465, 53201 | cost | the same 1 above |
| PaladinProtection | benediction | 498 Divine Protection, 31884 Avenging Wrath, 48952 Holy Shield | cost | the sim leaves Benediction out; Divine Protection costs nothing in the sim |
| HunterMarksmanship | efficiency | 58434 Volley | cost | the sim leaves Efficiency out |
| WarriorProtection | focusedRage | 23881 Bloodthirst, 47486 Mortal Strike | cost | the sim leaves Focused Rage out |
| ShamanEnhancement | mentalQuickness | 17364, 49284, 51533, 60103, 61301, 61657 | cost | the sim leaves Mental Quickness out; Earth Shield costs nothing in the sim |
| PriestShadow | shadowFocus | 48127 Mind Blast, 53023 Mind Sear | cost | the sim leaves Shadow Focus's cost cut out |
| HunterMarksmanship | efficiency | 3043 Scorpid Sting | cost | base cost 9% of base mana in the sim, 11% on the server |
| ShamanRestoration | tidalFocus | 49273 Healing Wave, 49284 Earth Shield | cost | Healing Wave's base cost is 15% of base mana in the sim, 25% on the server; Earth Shield costs nothing in the sim |
| PriestShadow | focusedMind | 53023 Mind Sear | cost | its base differs after the Shadow Focus row |
| DruidFeralCat | naturalShapeshifter, berserk | 768, 9634; 48566, 48572 | cost | the server adds percent cost mods and puts them on the base cost before flat ones; the sim multiplies them onto the reduced cost |
| DruidFeralCat | berserk | 48564 Mangle (Bear) | cooldown | not a sim gap: under Berserk the sim resets the cooldown after each cast instead of zeroing it |
| DeathknightFrost | annihilation, viciousStrikes | 55271 Scourge Strike; 49924 Death Strike | crit chance | the sim leaves the talent's crit out |
| ShamanRestoration | tidalMastery | 59159 Thunderstorm | crit chance | the sim leaves Tidal Mastery's crit out |
| WarlockDestruction | improvedSearingPain | 47815 Searing Pain | crit chance | the sim adds 0.1% instead of 10% (a factor of 100 short) |
| PriestDiscipline | innerFocus | 48089 Circle of Healing | crit chance | the sim adds Inner Focus's crit; the server's crit mask leaves Circle of Healing out |
| ShamanRestoration, DruidFeralBear | naturesSwiftness | 49273, 49276, 55459; 48477 | cast time, cast time before haste | the sim's Nature's Swiftness makes only its damage spells instant, not heals or Rebirth |
| WarriorFury | intensifyRage | 2687, 12292, 18499 | cooldown | the server's float math truncates 1 ms lower |
| DruidBalance, MageFire | celestialFocus, icyVeins | 48467; 12051, 42846, 42940 | aura duration, channel | the sim truncates the hasted tick period to whole ms |
| MageFire | permafrost | 47610 Frostfire Bolt | aura duration, e2 ticks | the sim leaves Permafrost's +3 s out |
| ShamanRestoration | improvedEarthShield | 49284 Earth Shield | aura duration | the sim models charges as ticks, so its aura grows; the server adds charges |
| PriestShadow | shadowFocus | 47585 Dispersion | miss | not a sim gap: Dispersion, a self buff, never rolls to hit; the sim's Shadow Focus hit leaves it out |

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
